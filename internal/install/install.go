package install

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Benehiko/jedi-outcast-coop/internal/paks"
)

// OptState is a tri-state opt-in decision for an optional mod.
type OptState int

const (
	// OptAsk prompts on a TTY, resolves to "no" off a TTY.
	OptAsk OptState = iota
	// OptYes force-enables.
	OptYes
	// OptNo force-disables.
	OptNo
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

	Widescreen OptState
	Textures   OptState
	Upscale    OptState

	// Combat is the combat feel written to autoexec_sp.cfg: "modern" (default)
	// or "classic". SkipCutscenes controls the map-intro cutscene auto-skip.
	// Sensitivity is the base mouse sensitivity written in modern mode.
	Combat        string
	Sensitivity   string
	SkipCutscenes OptState

	// AssumeYes auto-confirms prompts that would otherwise be shown.
	AssumeYes bool

	// Out receives progress text. Prompt reads y/N answers (nil = never
	// interactive; "ask" resolves to "no").
	Out    io.Writer
	Prompt func(question string) (bool, error)
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

// Install stages the data dir and installs the launchers for the current
// platform, then applies the selected optional mods.
func Install(ctx context.Context, p Platform, opts *Options) error {
	engineBin, engineDir := resolveEngine(opts.BuildDir, p)

	opts.sayf("Installing JK2 co-op (%s, %s)…", p.OS, p.Arch)

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
	opts.sayf("Installed. Try:")
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

// resolveOpt resolves a tri-state opt-in into a decision, prompting if "ask".
func (o *Options) resolveOpt(state OptState, question string) (bool, error) {
	switch state {
	case OptYes:
		return true, nil
	case OptNo:
		return false, nil
	default:
		if o.Prompt == nil {
			return false, nil
		}
		if o.AssumeYes {
			o.infof("%s [y/N] y (--yes)", question)
			return true, nil
		}
		return o.Prompt(question)
	}
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
