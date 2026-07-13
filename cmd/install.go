package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Benehiko/jedi-outcast-coop/internal/config"
	"github.com/Benehiko/jedi-outcast-coop/internal/install"
	"github.com/Benehiko/jedi-outcast-coop/internal/project"
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
			root, err := project.Root(repo)
			if err != nil {
				return err
			}
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
		},
	}

	f := cmd.Flags()
	f.StringVar(&repo, "repo", "", "repository root (default: autodetect from cwd)")
	f.StringVar(&buildDir, "build", "", "OpenJK CMake build dir (default: <repo>/openjk/build or $JK2_BUILD)")
	f.StringVar(&gamedata, "gamedata", "", "path to your Jedi Outcast GameData dir (default: Steam autodetect)")
	f.BoolVarP(&yes, "yes", "y", false, "assume \"yes\" to prompts (non-interactive)")
	return cmd
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
