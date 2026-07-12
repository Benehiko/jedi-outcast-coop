package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Benehiko/jedi-outcast-coop/internal/install"
	"github.com/Benehiko/jedi-outcast-coop/internal/project"
)

func newInstallCmd() *cobra.Command {
	var (
		repo, buildDir, gamedata          string
		uninstall, all, noOptional, yes   bool
		withWide, withTextures, withUpscl bool
	)

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Stage the co-op engine data dir and install the launchers",
		Long: "Stage the engine data directory (symlinks to the retail assets and the\n" +
			"built co-op gamecode) and install the jk2coop-host / jk2coop-join launchers.\n\n" +
			"It never copies or modifies retail files. --uninstall removes exactly what\n" +
			"was installed, and re-running is idempotent.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := project.Root(repo)
			if err != nil {
				return err
			}
			if buildDir == "" {
				buildDir = install.EnvOr("JK2_BUILD", filepath.Join(root, "openjk", "build"))
			}

			p := install.DetectPlatform(buildDir)
			opts := &install.Options{
				RepoRoot:   root,
				BuildDir:   buildDir,
				GameData:   gamedata,
				Widescreen: resolveState(all, noOptional, withWide),
				Textures:   resolveState(all, noOptional, withTextures),
				Upscale:    resolveState(all, noOptional, withUpscl),
				AssumeYes:  yes,
				Out:        cmd.OutOrStdout(),
				Prompt:     stdinPrompt(cmd),
			}

			if uninstall {
				return install.Uninstall(p, opts)
			}
			return install.Install(cmd.Context(), p, opts)
		},
	}

	f := cmd.Flags()
	f.StringVar(&repo, "repo", "", "repository root (default: autodetect from cwd)")
	f.StringVar(&buildDir, "build", "", "OpenJK CMake build dir (default: <repo>/openjk/build or $JK2_BUILD)")
	f.StringVar(&gamedata, "gamedata", "", "path to your Jedi Outcast GameData dir (default: Steam autodetect)")
	f.BoolVar(&uninstall, "uninstall", false, "remove everything this installer created")
	f.BoolVar(&withWide, "with-widescreen", false, "enable the widescreen/QHD/ultrawide video-menu mod")
	f.BoolVar(&withTextures, "with-textures", false, "generate the AI material-texture pak (GPU + container)")
	f.BoolVar(&withUpscl, "with-upscale", false, "build the Real-ESRGAN hi-res texture pak (GPU + container)")
	f.BoolVar(&all, "all", false, "enable every optional mod")
	f.BoolVar(&noOptional, "no-optional", false, "skip all optional-mod prompts (core install only)")
	f.BoolVarP(&yes, "yes", "y", false, "assume \"yes\" to prompts (non-interactive)")
	return cmd
}

// resolveState maps the mutually-influencing flags to an install.OptState.
// --all/--no-optional win over an individual --with-*; otherwise --with-* forces
// yes and the default is "ask".
func resolveState(all, noOptional, with bool) install.OptState {
	switch {
	case noOptional:
		return install.OptNo
	case all || with:
		return install.OptYes
	default:
		return install.OptAsk
	}
}

// stdinPrompt returns a y/N prompt bound to stdin, or nil when stdin is not a
// terminal (so "ask" resolves to "no" non-interactively).
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
