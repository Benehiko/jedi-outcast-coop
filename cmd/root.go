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
		Short: "Tooling for the Jedi Outcast co-op rebuild",
		Long: "jk2coop is the build/install tooling for the Jedi Outcast co-op rebuild.\n" +
			"It applies the OpenJK patch set, builds the override paks, and installs\n" +
			"the co-op engine data dir and launchers.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(newVersionCmd())
	root.AddCommand(newPatchesCmd())
	root.AddCommand(newPk3Cmd())
	root.AddCommand(newInstallCmd())

	return root
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
