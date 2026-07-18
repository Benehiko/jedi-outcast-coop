package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Benehiko/jedi-outcast-coop/internal/config"
	"github.com/Benehiko/jedi-outcast-coop/internal/gfx"
	"github.com/Benehiko/jedi-outcast-coop/internal/install"
	"github.com/Benehiko/jedi-outcast-coop/internal/project"
	"github.com/Benehiko/jedi-outcast-coop/internal/workdir"
)

func newInstallCmd() *cobra.Command {
	var (
		repo, buildDir, gamedata string
		yes                      bool
	)

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the co-op engine + your settings",
		Long: "Stage the engine data directory (symlinks to the retail assets and the\n" +
			"built co-op gamecode), install the jk2coop-host / jk2coop-join launchers,\n" +
			"and apply your config (jk2coop game / jk2coop graphics): the autoexec\n" +
			"cvars, the patch-backed graphics features (rebuilding the engine if they\n" +
			"changed), and any optional texture paks.\n\n" +
			"It never copies or modifies retail files, and re-running is idempotent.\n" +
			"Use `jk2coop uninstall` to remove exactly what was installed.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if repo != "" {
				root, err := project.Root(repo)
				if err != nil {
					return err
				}
				return runInstall(cmd, root, buildDir, gamedata, yes)
			}
			// Prefer the repo when the cwd is inside a checkout (dev), else fall
			// back to the standalone embedded-source install.
			if root, err := project.Root(""); err == nil {
				return runInstall(cmd, root, buildDir, gamedata, yes)
			}
			wd, err := workdir.Resolve()
			if err != nil {
				return err
			}
			return runInstallStandalone(cmd, wd, buildDir, gamedata, yes)
		},
	}

	f := cmd.Flags()
	f.StringVar(&repo, "repo", "", "dev only: install from this repo checkout instead of the embedded source")
	f.StringVar(&buildDir, "build", "", "OpenJK CMake build dir (default: <workdir>/src/build or $JK2_BUILD)")
	f.StringVar(&gamedata, "gamedata", "", "path to your Jedi Outcast GameData dir (default: Steam autodetect)")
	f.BoolVarP(&yes, "yes", "y", false, "assume \"yes\" to prompts (non-interactive)")
	return cmd
}

// runInstall builds the install Options and runs the installer. Shared by the
// `install` command and by `setup` (which runs it after building the engine),
// so the two paths stage the data dir and launchers identically. An empty
// buildDir defaults to <root>/openjk/build (or $JK2_BUILD).
func runInstall(cmd *cobra.Command, root, buildDir, gamedata string, yes bool) error {
	if buildDir == "" {
		buildDir = install.EnvOr("JK2_BUILD", filepath.Join(root, "openjk", "build"))
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	p := install.DetectPlatform(buildDir)
	opts := &install.Options{
		RepoRoot:  root,
		BuildDir:  buildDir,
		GameData:  gamedata,
		Config:    &cfg,
		AssumeYes: yes,
		Out:       cmd.OutOrStdout(),
		Prompt:    stdinPrompt(cmd),
	}
	return install.Install(cmd.Context(), p, opts)
}

// runInstallStandalone installs from the embedded work dir: the co-op UI assets
// come from <workdir>/coop-ui, and the engine-sync step extracts+patches+builds
// the embedded source via an EmbedManager (no repo, no git).
func runInstallStandalone(cmd *cobra.Command, wd workdir.Dir, buildDir, gamedata string, yes bool) error {
	if buildDir == "" {
		buildDir = install.EnvOr("JK2_BUILD", wd.Build())
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	// Ensure the co-op UI + blaster-FX assets are on disk for the installer to pak
	// (setup extracts them too; a bare `install` run must self-provision them).
	if err := embedCoopUI(wd); err != nil {
		return err
	}
	if err := embedBlasterFX(wd); err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	p := install.DetectPlatform(buildDir)
	opts := &install.Options{
		CoopUIDir:    wd.CoopUI(),
		BlasterFXDir: wd.BlasterFX(),
		BuildDir:     buildDir,
		GameData:     gamedata,
		Config:       &cfg,
		AssumeYes:    yes,
		Out:          out,
		Prompt:       stdinPrompt(cmd),
		EngineSync:   standaloneEngineSync(wd, buildDir, out),
	}
	return install.Install(cmd.Context(), p, opts)
}

// standaloneEngineSync returns an install.Options.EngineSync closure that brings
// the work-dir engine in line with the graphics config: re-extract + re-patch
// the embedded source only when the selection or pin changed, then rebuild.
func standaloneEngineSync(wd workdir.Dir, buildDir string, out interface{ Write([]byte) (int, error) }) func(context.Context) error {
	return func(ctx context.Context) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		mgr := &gfx.EmbedManager{Dir: wd}
		changed, err := mgr.EnsureApplied(ctx, cfg.GfxSelection())
		if err != nil {
			return err
		}
		if !changed {
			// Tree already matches; a build only runs if the output is missing.
			if _, statErr := os.Stat(filepath.Join(buildDir, "CMakeCache.txt")); statErr == nil {
				return nil
			}
		} else {
			_, _ = fmt.Fprintf(out, "Rebuilding engine to match graphics config (%s)…\n", gfx.SummaryLine(cfg.GfxSelection()))
		}
		return gfx.Build(ctx, wd.Src(), buildDir, out)
	}
}

// stdinPrompt returns a y/N prompt bound to stdin, or nil when stdin is not a
// terminal (so prompts resolve to "no" non-interactively).
func stdinPrompt(cmd *cobra.Command) func(string) (bool, error) {
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return nil
	}
	reader := bufio.NewReader(os.Stdin)
	return func(question string) (bool, error) {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s [y/N] ", question)
		line, err := reader.ReadString('\n')
		if err != nil {
			return false, nil //nolint:nilerr // EOF/interrupt means "no"
		}
		ans := strings.ToLower(strings.TrimSpace(line))
		return ans == "y" || ans == "yes", nil
	}
}
