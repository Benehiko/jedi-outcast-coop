package install

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Benehiko/jedi-outcast-coop/internal/config"
	"github.com/Benehiko/jedi-outcast-coop/internal/gfx"
	"github.com/Benehiko/jedi-outcast-coop/internal/paks"
)

// Options configures an install run.
type Options struct {
	// RepoRoot is the checked-out repository (for tools/ and assets/).
	RepoRoot string
	// BuildDir is the OpenJK CMake build dir (openjk/build).
	BuildDir string
	// GameData overrides Steam autodetection when set (the dir with
	// base/assets0.pk3).
	GameData string

	// Config is the single source of truth for every user setting: the autoexec
	// cvars, which patch-backed graphics features the engine is built with, and
	// which optional GPU paks to build.
	Config *config.Config

	// AssumeYes auto-confirms prompts that would otherwise be shown.
	AssumeYes bool

	// Out receives progress text. Prompt reads y/N answers (nil = never
	// interactive).
	Out    io.Writer
	Prompt func(question string) (bool, error)

	// warnings accumulates one-line summaries of non-fatal failures (an optional
	// mod that was requested but could not be built). They are reprinted in the
	// final summary so a failure buried in the scrolling install log is not
	// missed. Populated via warnf.
	warnings []string
}

func (o *Options) sayf(format string, a ...any) {
	if o.Out != nil {
		_, _ = fmt.Fprintf(o.Out, format+"\n", a...)
	}
}

func (o *Options) infof(format string, a ...any) {
	if o.Out != nil {
		_, _ = fmt.Fprintf(o.Out, "  "+format+"\n", a...)
	}
}

// warnf reports a non-fatal failure prominently at the point it happens and
// records a one-line summary for the end-of-run recap. summary is the short line
// shown in the recap; detail lines (the underlying error, a docs pointer) are
// printed now, indented under the warning. Install continues — these are
// optional mods, not hard errors.
func (o *Options) warnf(summary string, detail ...string) {
	o.warnings = append(o.warnings, summary)
	if o.Out == nil {
		return
	}
	_, _ = fmt.Fprintf(o.Out, "\n  ⚠ WARNING: %s\n", summary)
	for _, d := range detail {
		_, _ = fmt.Fprintf(o.Out, "      %s\n", d)
	}
}

// summarize prints the closing "Installed" recap and a "Try:" lead-in. When any
// optional mod failed (recorded via warnf) it says so up front and re-lists the
// failures, so a warning buried in the scrolling log is impossible to miss. The
// install itself still succeeded — the failed extras are optional.
func (o *Options) summarize() {
	n := len(o.warnings)
	if n == 0 {
		o.sayf("Installed. Try:")
		return
	}
	plural := "warning"
	if n > 1 {
		plural = "warnings"
	}
	o.sayf("Installed, but %d %s — some requested extras did not build:", n, plural)
	for _, w := range o.warnings {
		o.sayf("    ✗ %s", w)
	}
	o.sayf("")
	o.sayf("The game is installed and playable; the failed extras above are optional.")
	o.sayf("Try:")
}

// Install stages the data dir and installs the launchers for the current
// platform, then applies the selected optional mods.
func Install(ctx context.Context, p Platform, opts *Options) error {
	if opts.Config == nil {
		cfg := config.Defaults()
		opts.Config = &cfg
	}

	opts.sayf("Installing JK2 co-op (%s, %s)…", p.OS, p.Arch)

	// Ensure the engine is built with the patch-backed graphics features the
	// config wants (widescreen / render fidelity). Rebuilds only when they
	// differ from what's already built, so a plain reinstall is fast.
	if err := ensureEngineMatchesConfig(ctx, opts); err != nil {
		return err
	}

	engineBin, engineDir := resolveEngine(opts.BuildDir, p)

	// Preconditions: the build must exist.
	if engineBin == "" || !fileExists(engineBin) {
		return fmt.Errorf("engine not built in %s (build it per README first)", opts.BuildDir)
	}
	gamecode := filepath.Join(opts.BuildDir, "codeJK2", "game", p.GamecodeName)
	if !fileExists(gamecode) {
		return fmt.Errorf("gamecode not built: %s", gamecode)
	}
	renderer := filepath.Join(engineDir, p.RendererName)
	if !fileExists(renderer) {
		return fmt.Errorf("renderer not built beside engine: %s", renderer)
	}
	opts.infof("engine: %s", engineBin)

	gamedata := opts.GameData
	if gamedata == "" {
		opts.sayf("Locating your Jedi Outcast GameData…")
		gd, err := DetectGameData()
		if err != nil {
			return fmt.Errorf("%w.\n       Pass it explicitly: jk2coop install --gamedata '/path/to/Jedi Outcast/GameData'", err)
		}
		gamedata = gd
	}
	if !fileExists(filepath.Join(gamedata, "base", "assets0.pk3")) {
		return fmt.Errorf("invalid --gamedata: no base/assets0.pk3 under: %s", gamedata)
	}
	opts.infof("GameData: %s", gamedata)

	man, err := LoadManifest(p.ManifestPath())
	if err != nil {
		return err
	}

	// Persist the config so its values survive across installs and are removed on
	// uninstall, and track it in the manifest.
	if cfgPath, err := opts.Config.Save(); err != nil {
		opts.infof("could not save config: %v", err)
	} else if err := man.Add(cfgPath); err != nil {
		return err
	} else {
		opts.infof("config: %s", cfgPath)
	}

	baseDir := p.BaseDir()
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return err
	}
	if err := man.Add(baseDir); err != nil {
		return err
	}
	if err := man.Add(p.DataDir); err != nil {
		return err
	}

	// Link the retail asset paks.
	opts.sayf("Staging %s", baseDir)
	assetPaks, err := filepath.Glob(filepath.Join(gamedata, "base", "assets*.pk3"))
	if err != nil {
		return err
	}
	if len(assetPaks) == 0 {
		return fmt.Errorf("no assets*.pk3 found in %s/base", gamedata)
	}
	for _, pk := range assetPaks {
		if err := linkTracked(man, pk, filepath.Join(baseDir, filepath.Base(pk))); err != nil {
			return err
		}
	}
	opts.infof("linked %d asset pak(s)", len(assetPaks))

	// Link the co-op gamecode.
	if err := linkTracked(man, gamecode, filepath.Join(baseDir, p.GamecodeName)); err != nil {
		return err
	}
	opts.infof("linked gamecode %s", p.GamecodeName)

	// Build (if needed) + link the co-op UI overlay.
	coopPak := filepath.Join(opts.RepoRoot, "assets", "coop-ui", "zz-coop-ui.pk3")
	if !fileExists(coopPak) {
		if _, err := paks.BuildCoopUI(filepath.Join(opts.RepoRoot, "assets", "coop-ui"), coopPak); err != nil {
			opts.infof("could not build co-op UI overlay: %v", err)
		}
	}
	if fileExists(coopPak) {
		if err := linkTracked(man, coopPak, filepath.Join(baseDir, "zz-coop-ui.pk3")); err != nil {
			return err
		}
		opts.infof("linked co-op UI overlay zz-coop-ui.pk3")
	}

	// Launchers.
	if err := os.MkdirAll(p.BinDir, 0o755); err != nil {
		return err
	}
	if err := man.Add(p.BinDir); err != nil {
		return err
	}
	opts.sayf("Installing launchers in %s", p.BinDir)
	if err := writeLaunchers(p, man, engineBin, gamecode); err != nil {
		return err
	}
	opts.infof("jk2coop-host")
	opts.infof("jk2coop-join")

	// Optional mods.
	opts.sayf("")
	opts.sayf("Optional mods:")
	if err := installOptionalMods(ctx, man, opts, gamedata, baseDir); err != nil {
		return err
	}

	opts.sayf("")
	opts.summarize()
	opts.sayf("    jk2coop-host                      # host on port %d", defaultPort)
	opts.sayf("    jk2coop-join 127.0.0.1 --second   # join from a second local client")
	if !pathContains(p.BinDir) {
		opts.sayf("")
		opts.sayf("note: %s is not on your PATH; add it or call the launchers by full path.", p.BinDir)
	}
	return nil
}

// Uninstall removes everything the manifest tracks.
func Uninstall(p Platform, opts *Options) error {
	opts.sayf("Uninstalling JK2 co-op…")
	man, err := LoadManifest(p.ManifestPath())
	if err != nil {
		return err
	}
	if !man.Exists() {
		opts.infof("no install manifest at %s — nothing tracked to remove.", p.ManifestPath())
		return nil
	}
	for _, r := range man.Uninstall() {
		if !r.Removed {
			continue
		}
		switch r.Kind {
		case "manifest":
			opts.infof("removed manifest")
		case "dir":
			opts.infof("removed dir %s", r.Path)
		default:
			opts.infof("removed %s", r.Path)
		}
	}
	opts.sayf("Done. Retail files and your Steam install were never touched.")
	return nil
}

// ensureEngineMatchesConfig rebuilds the engine when the patch-backed graphics
// features it is currently built with differ from what the config wants. It
// resets the OpenJK submodule, reapplies the co-op base plus the selected
// features, and rebuilds — the same operation the graphics menu performs. When
// the built selection already matches, it is a no-op (no reset, no rebuild).
func ensureEngineMatchesConfig(ctx context.Context, opts *Options) error {
	if opts.RepoRoot == "" {
		// No repo (e.g. installing from a prebuilt drop): trust what's built.
		return nil
	}
	mgr := &gfx.Manager{
		Submodule:  filepath.Join(opts.RepoRoot, "openjk"),
		PatchesDir: filepath.Join(opts.RepoRoot, "patches"),
	}
	have, err := mgr.Detect(ctx)
	if err != nil {
		// A missing/uninitialised submodule is not fatal for an install from a
		// prebuilt engine; skip the match step.
		opts.infof("skipping engine feature check: %v", err)
		return nil
	}
	want := opts.Config.GfxSelection()
	if !gfxSelectionDiffers(have, want) {
		return nil
	}
	opts.sayf("Rebuilding engine to match graphics config (%s)…", gfx.SummaryLine(want))
	if _, err := mgr.Apply(ctx, want); err != nil {
		return err
	}
	return gfx.Build(ctx, filepath.Join(opts.RepoRoot, "openjk"), opts.BuildDir, opts.Out)
}

// gfxSelectionDiffers reports whether the built feature set (have) differs from
// the wanted one for any known gfx feature key.
func gfxSelectionDiffers(have, want map[string]bool) bool {
	for _, f := range gfx.Features {
		if have[f.Key] != want[f.Key] {
			return true
		}
	}
	return false
}

func linkTracked(man *Manifest, target, linkPath string) error {
	if err := replaceSymlink(target, linkPath); err != nil {
		return err
	}
	return man.Add(linkPath)
}

func pathContains(dir string) bool {
	sep := string(os.PathListSeparator)
	return slices.Contains(strings.Split(os.Getenv("PATH"), sep), dir)
}
