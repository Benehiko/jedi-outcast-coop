package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Benehiko/jedi-outcast-coop/internal/dockerbuild"
	veepkg "github.com/Benehiko/jedi-outcast-coop/internal/vee"
	"github.com/Benehiko/jedi-outcast-coop/internal/vmbuild"
)

// newVeeCmd groups the commands that manage vee and the throwaway build VM
// jk2coop uses to compile the engine without a host toolchain. It makes the
// otherwise-invisible machinery (where vee lives, which VM exists, how to remove
// it) inspectable and controllable.
func newVeeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vee",
		Short: "Manage vee and the build VM jk2coop uses to compile the engine",
		Long: "jk2coop builds the engine inside a throwaway VM managed by `vee`, so you\n" +
			"install no C/C++ toolchain and no Docker on this machine. These commands\n" +
			"let you inspect and manage that machinery:\n\n" +
			"  jk2coop vee status         show where vee lives and whether a build VM exists\n" +
			"  jk2coop vee download       download jk2coop's managed vee copy now\n" +
			"  jk2coop vee vm delete      delete the build VM (frees disk; next build recreates it)\n\n" +
			"The build VM is kept between runs so a rebuild (a graphics change, a\n" +
			"re-install) reuses the warm VM instead of recreating it. Deleting it is\n" +
			"safe — the next build recreates it, only slower.\n\n" +
			"See docs/build-vm.md for how the vee/VM setup works end to end.",
	}
	cmd.AddCommand(newVeeStatusCmd())
	cmd.AddCommand(newVeeDownloadCmd())
	cmd.AddCommand(newVeeVMCmd())
	return cmd
}

func newVeeStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show vee location and build-VM state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			path, ok := veepkg.Resolve()
			if ok {
				_, _ = fmt.Fprintf(out, "vee: %s\n", path)
			} else {
				managed, _ := veepkg.ManagedPath()
				_, _ = fmt.Fprintf(out, "vee: not found (would download %s to %s)\n", veepkg.Version, managed)
			}
			_, _ = fmt.Fprintf(out, "docker build VM (%s): %s\n", dockerbuild.VMName, vmState(cmd.Context(), dockerbuild.VMName))
			_, _ = fmt.Fprintf(out, "plain build VM  (%s): %s\n", vmbuild.VMName, vmState(cmd.Context(), vmbuild.VMName))
			return nil
		},
	}
}

func newVeeDownloadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "download",
		Short: "Download jk2coop's managed vee copy into the config dir",
		Long: "Download the pinned vee release into os.UserConfigDir()/jk2coop/bin so\n" +
			"later builds reuse it. A no-op if vee is already on PATH or already\n" +
			"downloaded. `jk2coop setup` does this for you; this is for doing it ahead\n" +
			"of time or re-fetching after a manual delete.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := veepkg.Ensure(cmd.Context(), cmd.OutOrStdout())
			return err
		},
	}
}

func newVeeVMCmd() *cobra.Command {
	vm := &cobra.Command{
		Use:   "vm",
		Short: "Manage the throwaway build VM",
	}
	vm.AddCommand(&cobra.Command{
		Use:   "delete",
		Short: "Delete the build VM (frees disk; the next build recreates it)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			ctx := cmd.Context()
			var deleted bool
			for _, name := range []string{dockerbuild.VMName, vmbuild.VMName} {
				if vmState(ctx, name) == "absent" {
					continue
				}
				deleted = true
				if err := veeRun(ctx, out, "delete", name, "--yes"); err != nil {
					return err
				}
			}
			if !deleted {
				_, _ = fmt.Fprintln(out, "No build VM to delete.")
			}
			return nil
		},
	})
	return vm
}

// vmState reports "present" or "absent" for a vee VM by name, or "vee not found"
// when vee itself is unavailable.
func vmState(ctx context.Context, name string) string {
	bin, ok := veepkg.Resolve()
	if !ok {
		return "vee not found"
	}
	b, err := exec.CommandContext(ctx, bin, "list").Output()
	if err != nil {
		return "unknown"
	}
	// `vee list` prints one VM per line; match the name as a whole field.
	for line := range strings.SplitSeq(string(b), "\n") {
		if slices.Contains(strings.Fields(line), name) {
			return "present"
		}
	}
	return "absent"
}

// veeRun shells out to the resolved vee binary, streaming output.
func veeRun(ctx context.Context, out interface{ Write([]byte) (int, error) }, args ...string) error {
	bin, ok := veepkg.Resolve()
	if !ok {
		return fmt.Errorf("vee not found; run `jk2coop vee download` or `jk2coop setup` first")
	}
	c := exec.CommandContext(ctx, bin, args...)
	c.Stdout, c.Stderr = out, out
	if err := c.Run(); err != nil {
		return fmt.Errorf("vee %s: %w", args[0], err)
	}
	return nil
}
