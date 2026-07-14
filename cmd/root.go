// Package cmd wires the jk2coop cobra command tree.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version metadata, injected at build time via -ldflags (see the Makefile).
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "jk2coop",
		Short: "Jedi Outcast co-op — install, play, and tune",
		Long: "jk2coop installs and runs the Jedi Outcast co-op rebuild.\n\n" +
			"  jk2coop setup              first-time: fetch, build, and install\n" +
			"  jk2coop install            install the engine + your settings\n" +
			"  jk2coop launch             play (hosts co-op by default)\n" +
			"  jk2coop host               host a co-op game\n" +
			"  jk2coop join <IP>          join a co-op game by IP\n" +
			"  jk2coop game               Game Settings (mouse, blaster, aim)\n" +
			"  jk2coop graphics           Graphics Settings (widescreen, MSAA, …)\n" +
			"  jk2coop uninstall          remove everything jk2coop installed",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// The user-facing actions.
	root.AddCommand(newSetupCmd())
	root.AddCommand(newInstallCmd())
	root.AddCommand(newLaunchCmd())
	root.AddCommand(newHostCmd())
	root.AddCommand(newJoinCmd())
	root.AddCommand(newGameCmd())
	root.AddCommand(newGraphicsCmd())
	root.AddCommand(newUninstallCmd())

	root.AddCommand(newVersionCmd())

	// Low-level developer/CI tooling, hidden from the main help.
	root.AddCommand(newDevCmd())

	return root
}

// newDevCmd groups the low-level patch/pak plumbing that install now runs for
// you. Hidden from the main help but still callable (CI, e2e, debugging).
func newDevCmd() *cobra.Command {
	dev := &cobra.Command{
		Use:    "dev",
		Short:  "Low-level patch/pak tooling (used by install; for CI/debugging)",
		Hidden: true,
	}
	dev.AddCommand(newPatchesCmd())
	dev.AddCommand(newPk3Cmd())
	dev.AddCommand(newGfxCmd())
	return dev
}

// Execute runs the root command and exits non-zero on error.
func Execute() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "jk2coop %s (commit %s, built %s)\n", version, commit, date)
		},
	}
}
