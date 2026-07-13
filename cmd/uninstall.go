package cmd

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Benehiko/jedi-outcast-coop/internal/install"
	"github.com/Benehiko/jedi-outcast-coop/internal/project"
)

func newUninstallCmd() *cobra.Command {
	var repo, buildDir string

	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove everything jk2coop installed",
		Long: "Remove exactly what `jk2coop install` created (staged data dir, launchers,\n" +
			"override paks, autoexec, config), tracked in the install manifest. Retail\n" +
			"files and your Steam install are never touched.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Repo root is optional for uninstall; fall back to cwd-independent defaults.
			root, _ := project.Root(repo)
			if buildDir == "" {
				base := root
				if base == "" {
					base = "."
				}
				buildDir = install.EnvOr("JK2_BUILD", filepath.Join(base, "openjk", "build"))
			}
			p := install.DetectPlatform(buildDir)
			opts := &install.Options{Out: cmd.OutOrStdout()}
			return install.Uninstall(p, opts)
		},
	}

	f := cmd.Flags()
	f.StringVar(&repo, "repo", "", "repository root (default: autodetect from cwd)")
	f.StringVar(&buildDir, "build", "", "OpenJK CMake build dir (default: <repo>/openjk/build or $JK2_BUILD)")
	return cmd
}
