package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Benehiko/jedi-outcast-coop/internal/config"
	embedpkg "github.com/Benehiko/jedi-outcast-coop/internal/embed"
	"github.com/Benehiko/jedi-outcast-coop/internal/gfx"
	"github.com/Benehiko/jedi-outcast-coop/internal/install"
	"github.com/Benehiko/jedi-outcast-coop/internal/prereq"
	"github.com/Benehiko/jedi-outcast-coop/internal/project"
	"github.com/Benehiko/jedi-outcast-coop/internal/vmbuild"
	"github.com/Benehiko/jedi-outcast-coop/internal/workdir"
)

func newSetupCmd() *cobra.Command {
	var (
		repo, buildDir, gamedata string
		yes, useVM, useHost      bool
	)

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "First-time setup: fetch, build, and install in one step",
		Long: "One command to go from a fresh clone to a playable game:\n\n" +
			"  1. initialise the OpenJK submodule (if not already)\n" +
			"  2. apply the co-op patches for your graphics config\n" +
			"  3. build the engine — on this machine, or in a clean throwaway VM\n" +
			"  4. install (stage assets, launchers, and your settings)\n\n" +
			"If the build toolchain (cmake, ninja, a C compiler) is missing, setup\n" +
			"prints the exact command to install it, or offers to build inside a VM\n" +
			"(via `vee`) so you never install a compiler at all.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if useVM && useHost {
				return fmt.Errorf("--vm and --host are mutually exclusive")
			}
			if repo != "" {
				// Dev flow: build from the repo checkout + git submodule.
				return setupFromRepo(cmd, repo, buildDir, gamedata, useVM, useHost, yes)
			}
			// Standalone flow: extract the embedded source into the work dir,
			// patch it in pure Go, build, and install — no repo, no git.
			return setupStandalone(cmd, buildDir, gamedata, useVM, useHost, yes)
		},
	}

	f := cmd.Flags()
	f.StringVar(&repo, "repo", "", "dev only: build from this repo checkout + git submodule instead of the embedded source")
	f.StringVar(&buildDir, "build", "", "OpenJK CMake build dir (default: <workdir>/src/build or $JK2_BUILD)")
	f.StringVar(&gamedata, "gamedata", "", "path to your Jedi Outcast GameData dir (default: Steam autodetect)")
	f.BoolVar(&useVM, "vm", false, "build inside a throwaway VM via vee (no local toolchain needed)")
	f.BoolVar(&useHost, "host", false, "build on this machine (requires the build toolchain)")
	f.BoolVarP(&yes, "yes", "y", false, "assume \"yes\" to prompts (non-interactive)")
	return cmd
}

// setupStandalone runs the embedded-source setup: resolve the work dir, extract
// + patch the baked-in OpenJK source for the configured graphics selection,
// build (host or VM), then install. Needs neither a repo checkout nor git.
func setupStandalone(cmd *cobra.Command, buildDir, gamedata string, useVM, useHost, yes bool) error {
	out := cmd.OutOrStdout()
	ctx := cmd.Context()

	wd, err := workdir.Resolve()
	if err != nil {
		return err
	}
	if buildDir == "" {
		buildDir = install.EnvOr("JK2_BUILD", wd.Build())
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Decide the build method up front so a doomed host build (missing
	// toolchain, no VM) fails fast before we extract and patch.
	buildInVM, err := chooseVMBuild(cmd, useVM, useHost, yes)
	if err != nil {
		return err
	}

	// Extract the embedded source and apply the co-op patches for the selection.
	mgr := &gfx.EmbedManager{Dir: wd}
	_, _ = fmt.Fprintf(out, "Preparing engine source (%s)…\n", gfx.SummaryLine(cfg.GfxSelection()))
	if _, err := mgr.Apply(ctx, cfg.GfxSelection()); err != nil {
		return err
	}

	// Build — host or VM. The VM shares the work-dir root over virtiofs and
	// builds its "src" subdir (already patched on the host).
	if buildInVM {
		if err := vmbuild.Build(ctx, wd.Root, "src", out); err != nil {
			return err
		}
		if err := maybeDeleteVM(cmd, yes); err != nil {
			return err
		}
	} else {
		_, _ = fmt.Fprintln(out, "Building the engine on this machine…")
		if err := gfx.Build(ctx, wd.Src(), buildDir, out); err != nil {
			return err
		}
	}

	// Extract the co-op UI assets for the installer to pak, then install.
	if err := embedCoopUI(wd); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, "Installing…")
	return runInstallStandalone(cmd, wd, buildDir, gamedata, yes)
}

// setupFromRepo is the legacy dev flow: build from the repo checkout and git
// submodule (git-based patch apply, submodule reset), unchanged behaviour.
func setupFromRepo(cmd *cobra.Command, repo, buildDir, gamedata string, useVM, useHost, yes bool) error {
	root, err := project.Root(repo)
	if err != nil {
		return err
	}
	if buildDir == "" {
		buildDir = install.EnvOr("JK2_BUILD", filepath.Join(root, "openjk", "build"))
	}
	out := cmd.OutOrStdout()
	ctx := cmd.Context()

	if err := ensureSubmodule(ctx, root, out); err != nil {
		return err
	}
	buildInVM, err := chooseVMBuild(cmd, useVM, useHost, yes)
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	mgr := &gfx.Manager{
		Submodule:  filepath.Join(root, "openjk"),
		PatchesDir: filepath.Join(root, "patches"),
	}
	_, _ = fmt.Fprintf(out, "Applying co-op patches (%s)…\n", gfx.SummaryLine(cfg.GfxSelection()))
	if _, err := mgr.Apply(ctx, cfg.GfxSelection()); err != nil {
		return err
	}
	if buildInVM {
		if err := vmbuild.Build(ctx, root, "openjk", out); err != nil {
			return err
		}
		if err := maybeDeleteVM(cmd, yes); err != nil {
			return err
		}
	} else {
		_, _ = fmt.Fprintln(out, "Building the engine on this machine…")
		if err := gfx.Build(ctx, filepath.Join(root, "openjk"), buildDir, out); err != nil {
			return err
		}
	}
	_, _ = fmt.Fprintln(out, "Installing…")
	return runInstall(cmd, root, buildDir, gamedata, yes)
}

// embedCoopUI extracts the baked-in co-op UI assets into the work dir so the
// installer can build zz-coop-ui.pk3 from them. ExtractCoopUI writes a coop-ui/
// subdir under its argument, so passing the work-dir root lands the assets at
// wd.CoopUI() (<root>/coop-ui).
func embedCoopUI(wd workdir.Dir) error {
	if err := os.MkdirAll(wd.Root, 0o755); err != nil {
		return err
	}
	return embedpkg.ExtractCoopUI(wd.Root)
}

// ensureSubmodule initialises the OpenJK submodule when it is not yet a git
// checkout, fixing the most common fresh-clone mistake (forgetting
// --recurse-submodules) instead of failing later.
func ensureSubmodule(ctx context.Context, root string, out interface{ Write([]byte) (int, error) }) error {
	sub := filepath.Join(root, "openjk")
	if _, err := os.Stat(filepath.Join(sub, ".git")); err == nil {
		return nil // already a checkout
	}
	_, _ = fmt.Fprintln(out, "Initialising the OpenJK submodule…")
	cmd := exec.CommandContext(ctx, "git", "-C", root, "submodule", "update", "--init", "--recursive")
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git submodule update --init: %w", err)
	}
	return nil
}

// chooseVMBuild decides whether to build in a VM, gathering the live host state
// (missing toolchain, vee availability, interactivity) and delegating the
// decision to decideBuild.
func chooseVMBuild(cmd *cobra.Command, useVM, useHost, yes bool) (bool, error) {
	missing := prereq.Missing()
	prompt := stdinPrompt(cmd)
	// Interactive when we have a real stdin prompt and the user did not pass -y.
	interactive := prompt != nil && !yes
	out := cmd.OutOrStdout()
	if len(missing) > 0 && !useHost {
		_, _ = fmt.Fprintf(out, "\nBuild tools not found on this machine:\n%s\n\n", prereq.Guidance(missing))
	}
	return decideBuild(useVM, useHost, len(missing) > 0, vmbuild.Available(), interactive, prompt)
}

// decideBuild is the pure build-location decision. Flags win; otherwise, when
// vee is available it offers the choice (interactively) or picks the sensible
// default (non-interactively: VM iff the host lacks the toolchain). When vee is
// absent the answer is always host, and a missing toolchain is a fatal error
// carrying install guidance so the user can install it and re-run.
func decideBuild(useVM, useHost, toolMissing, veeAvail, interactive bool, prompt func(string) (bool, error)) (bool, error) {
	switch {
	case useVM:
		return true, nil
	case useHost:
		if toolMissing {
			return false, fmt.Errorf("build toolchain missing:\n%s", prereq.Guidance(prereq.Missing()))
		}
		return false, nil
	}

	if !veeAvail {
		if toolMissing {
			return false, fmt.Errorf("build toolchain missing (and vee not installed for a VM build):\n%s", prereq.Guidance(prereq.Missing()))
		}
		return false, nil
	}

	// vee is available: offer the choice, defaulting to a VM build only when the
	// host is missing the toolchain.
	if !interactive {
		return toolMissing, nil
	}
	q := "Build inside a clean throwaway VM (via vee)?"
	if toolMissing {
		q = "Build inside a clean throwaway VM (via vee)? (recommended — no compiler needed)"
	}
	return prompt(q)
}

// maybeDeleteVM offers to delete the build VM after a successful VM build.
// Default is to keep it (a re-run reuses the warm VM). --yes keeps it.
func maybeDeleteVM(cmd *cobra.Command, yes bool) error {
	prompt := stdinPrompt(cmd)
	if yes || prompt == nil {
		return nil // keep the VM (non-destructive default)
	}
	del, err := prompt("Delete the build VM now? (keep it for faster rebuilds)")
	if err != nil || !del {
		return err
	}
	return vmbuild.Delete(cmd.Context(), cmd.OutOrStdout())
}
