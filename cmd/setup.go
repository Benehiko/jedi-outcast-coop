package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/Benehiko/jedi-outcast-coop/internal/config"
	"github.com/Benehiko/jedi-outcast-coop/internal/dockerbuild"
	embedpkg "github.com/Benehiko/jedi-outcast-coop/internal/embed"
	"github.com/Benehiko/jedi-outcast-coop/internal/gfx"
	"github.com/Benehiko/jedi-outcast-coop/internal/install"
	"github.com/Benehiko/jedi-outcast-coop/internal/prereq"
	"github.com/Benehiko/jedi-outcast-coop/internal/project"
	veepkg "github.com/Benehiko/jedi-outcast-coop/internal/vee"
	"github.com/Benehiko/jedi-outcast-coop/internal/vmbuild"
	"github.com/Benehiko/jedi-outcast-coop/internal/workdir"
)

// buildMethod selects where the engine is compiled.
type buildMethod int

const (
	// buildHost compiles on this machine (needs the native toolchain).
	buildHost buildMethod = iota
	// buildVM compiles inside a throwaway QEMU VM via vee (internal/vmbuild).
	buildVM
	// buildDocker compiles inside a container in a vee docker-template VM
	// (internal/dockerbuild); the host needs neither a toolchain nor Docker.
	buildDocker
)

func newSetupCmd() *cobra.Command {
	var (
		repo, buildDir, gamedata       string
		yes, useVM, useHost, useDocker bool
	)

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "First-time setup: fetch, build, and install in one step",
		Long: "One command to go from a fresh clone to a playable game:\n\n" +
			"  1. initialise the OpenJK submodule (if not already)\n" +
			"  2. apply the co-op patches for your graphics config\n" +
			"  3. build the engine — by default in a container inside a throwaway\n" +
			"     VM (via `vee`), so you install no compiler and no Docker\n" +
			"  4. install (stage assets, launchers, and your settings)\n\n" +
			"By default the build runs in a container in a vee VM (--docker), so you\n" +
			"install no compiler and no Docker. If `vee` is not already on PATH, setup\n" +
			"downloads a pinned, checksum-verified copy into the config dir\n" +
			"(~/.config/jk2coop/bin) and keeps it for later rebuilds. Manage the vee/VM\n" +
			"machinery with `jk2coop vee`.\n\n" +
			"On macOS the default is instead a native host build: a Linux container/VM\n" +
			"cannot emit a macOS (Mach-O) engine, so setup builds with your local\n" +
			"cmake/ninja/compiler toolchain and never downloads vee (--docker/--vm are\n" +
			"rejected there). Use the 'jk2coop-macos' CI artifact to skip building.\n\n" +
			"Pass --host to build on this machine (needs the cmake/ninja/compiler\n" +
			"toolchain) or --vm for a plain VM build. When vee cannot be obtained,\n" +
			"setup falls back to a host build.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := exclusiveBuildFlags(useVM, useHost, useDocker); err != nil {
				return err
			}
			if repo != "" {
				// Dev flow: build from the repo checkout + git submodule.
				return setupFromRepo(cmd, repo, buildDir, gamedata, useVM, useHost, useDocker, yes)
			}
			// Standalone flow: extract the embedded source into the work dir,
			// patch it in pure Go, build, and install — no repo, no git.
			return setupStandalone(cmd, buildDir, gamedata, useVM, useHost, useDocker, yes)
		},
	}

	f := cmd.Flags()
	f.StringVar(&repo, "repo", "", "dev only: build from this repo checkout + git submodule instead of the embedded source")
	f.StringVar(&buildDir, "build", "", "OpenJK CMake build dir (default: <workdir>/src/build or $JK2_BUILD)")
	f.StringVar(&gamedata, "gamedata", "", "path to your Jedi Outcast GameData dir (default: Steam autodetect)")
	f.BoolVar(&useVM, "vm", false, "build inside a throwaway VM via vee (no local toolchain needed)")
	f.BoolVar(&useDocker, "docker", false, "build inside a container in a vee docker VM (no local toolchain or Docker needed)")
	f.BoolVar(&useHost, "host", false, "build on this machine (requires the build toolchain)")
	f.BoolVarP(&yes, "yes", "y", false, "assume \"yes\" to prompts (non-interactive)")
	return cmd
}

// setupStandalone runs the embedded-source setup: resolve the work dir, extract
// + patch the baked-in OpenJK source for the configured graphics selection,
// build (host or VM), then install. Needs neither a repo checkout nor git.
func setupStandalone(cmd *cobra.Command, buildDir, gamedata string, useVM, useHost, useDocker, yes bool) error {
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
	method, err := chooseBuildMethod(cmd, useVM, useHost, useDocker)
	if err != nil {
		return err
	}

	// Extract the embedded source and apply the co-op patches for the selection.
	mgr := &gfx.EmbedManager{Dir: wd}
	_, _ = fmt.Fprintf(out, "Preparing engine source (%s)…\n", gfx.SummaryLine(cfg.GfxSelection()))
	if _, err := mgr.Apply(ctx, cfg.GfxSelection()); err != nil {
		return err
	}

	// Build — host, VM, or docker. The VM/docker paths share the work-dir root
	// over virtiofs and build its "src" subdir (already patched on the host).
	switch method {
	case buildVM:
		if err := vmbuild.Build(ctx, wd.Root, "src", out); err != nil {
			return err
		}
		if err := maybeDeleteVM(cmd, yes); err != nil {
			return err
		}
	case buildDocker:
		if err := dockerbuild.Build(ctx, wd.Root, "src", out); err != nil {
			return err
		}
		if err := maybeDeleteDockerVM(cmd, yes); err != nil {
			return err
		}
	default:
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
func setupFromRepo(cmd *cobra.Command, repo, buildDir, gamedata string, useVM, useHost, useDocker, yes bool) error {
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
	method, err := chooseBuildMethod(cmd, useVM, useHost, useDocker)
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
	switch method {
	case buildVM:
		if err := vmbuild.Build(ctx, root, "openjk", out); err != nil {
			return err
		}
		if err := maybeDeleteVM(cmd, yes); err != nil {
			return err
		}
	case buildDocker:
		if err := dockerbuild.Build(ctx, root, "openjk", out); err != nil {
			return err
		}
		if err := maybeDeleteDockerVM(cmd, yes); err != nil {
			return err
		}
	default:
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

// embedBlasterFX extracts the baked-in blaster impact-FX source into the work
// dir so the installer can build zz-blaster-fx.pk3 from it. ExtractBlasterFX
// strips the embed root, so passing wd.BlasterFX() lands the effects/ tree at
// wd.BlasterFX()/effects — exactly where install.BuildBlasterFX looks.
func embedBlasterFX(wd workdir.Dir) error {
	if err := os.MkdirAll(wd.BlasterFX(), 0o755); err != nil {
		return err
	}
	return embedpkg.ExtractBlasterFX(wd.BlasterFX())
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

// exclusiveBuildFlags rejects more than one of --vm/--docker/--host at once.
func exclusiveBuildFlags(useVM, useHost, useDocker bool) error {
	n := 0
	for _, b := range []bool{useVM, useHost, useDocker} {
		if b {
			n++
		}
	}
	if n > 1 {
		return fmt.Errorf("--vm, --docker, and --host are mutually exclusive")
	}
	return nil
}

// chooseBuildMethod decides where to build, gathering the live host state (vee
// availability, missing toolchain) and delegating the pure decision to
// decideBuildMethod. Unless the user forced --host (or is on macOS, where the
// container build is impossible), it first tries to make vee available — using
// an existing install or downloading jk2coop's own managed copy from GitHub
// releases — so the default container build works with no host toolchain. The
// downloaded vee is kept under the config dir for later rebuilds.
func chooseBuildMethod(cmd *cobra.Command, useVM, useHost, useDocker bool) (buildMethod, error) {
	out := cmd.OutOrStdout()
	veeAvail := ensureVee(cmd, useHost)
	toolMissing := len(prereq.Missing()) > 0
	method, err := decideBuildMethod(useVM, useHost, useDocker, veeAvail, toolMissing, runtime.GOOS)
	if err != nil {
		return buildHost, err
	}
	// The "install vee" nudge only applies where the container build is a real
	// option. On macOS it never is (a Linux container can't emit a Mach-O
	// binary), so suggesting vee there would be misleading — the host build is
	// the intended default, not a fallback.
	if method == buildHost && !useHost && !veeAvail && !toolMissing && runtime.GOOS != "darwin" {
		_, _ = fmt.Fprintln(out,
			"vee unavailable; building on this machine (install vee for the default container build).")
	}
	return method, nil
}

// ensureVee reports whether vee is usable, downloading jk2coop's managed copy
// when none is present. It never downloads for an explicit --host build (no vee
// needed) and treats a download failure as "vee unavailable" (a warning, not a
// fatal error) so setup can still fall back to a host build.
func ensureVee(cmd *cobra.Command, useHost bool) bool {
	// A host build needs no vee, and on macOS the container/VM paths cannot
	// produce a runnable (Mach-O) engine at all — so vee is never useful there.
	// In both cases skip the network entirely.
	if useHost || runtime.GOOS == "darwin" {
		return false
	}
	out := cmd.OutOrStdout()
	if _, err := veepkg.Ensure(cmd.Context(), out); err != nil {
		_, _ = fmt.Fprintf(out, "Could not obtain vee automatically: %v\n", err)
		return false
	}
	return true
}

// decideBuildMethod is the pure build-method decision. Explicit flags win. With
// no flag the default is the docker path (a container in a vee VM — no host
// toolchain and no host Docker), so a fresh clone builds with only vee
// installed. When vee is unavailable the default falls back to a host build (if
// the toolchain is present) or a fatal error guiding the user to install vee or
// the toolchain. veeAvail reports whether vee is on PATH (it powers both the
// docker and vm paths).
//
// hostGOOS makes the mapping OS-aware (and testable). On macOS the container/VM
// paths cannot emit a Mach-O binary (see dockerbuild.TargetForHost), so the
// only build that yields a runnable engine is the host (Xcode) build: the
// default is host rather than the doomed container path, and an explicit
// --docker/--vm is rejected up front — before any vee download or source
// extract — instead of failing deep inside the container build.
func decideBuildMethod(useVM, useHost, useDocker, veeAvail, toolMissing bool, hostGOOS string) (buildMethod, error) {
	if hostGOOS == "darwin" {
		switch {
		case useDocker:
			return buildHost, fmt.Errorf("a macOS engine cannot be built in a container; drop --docker and build on this Mac (jk2coop setup --host) or use the 'jk2coop-macos' CI artifact")
		case useVM:
			return buildHost, fmt.Errorf("a macOS engine cannot be built in a Linux VM; drop --vm and build on this Mac (jk2coop setup --host) or use the 'jk2coop-macos' CI artifact")
		}
		// --host or no flag: the macOS default is a native host build.
		if toolMissing {
			return buildHost, fmt.Errorf("build toolchain missing:\n%s", prereq.Guidance(prereq.Missing()))
		}
		return buildHost, nil
	}

	switch {
	case useHost:
		if toolMissing {
			return buildHost, fmt.Errorf("build toolchain missing:\n%s", prereq.Guidance(prereq.Missing()))
		}
		return buildHost, nil
	case useVM:
		if !veeAvail {
			return buildHost, fmt.Errorf("--vm needs vee on PATH; install vee or use --host")
		}
		return buildVM, nil
	case useDocker:
		if !veeAvail {
			return buildHost, fmt.Errorf("--docker needs vee on PATH (it runs the Docker daemon inside a vee VM); install vee or use --host")
		}
		return buildDocker, nil
	}

	// No flag: default to the docker path when vee is available.
	if veeAvail {
		return buildDocker, nil
	}
	// vee absent — fall back to a host build when possible, else guide the user
	// to install vee (for the default container build) or the toolchain.
	if !toolMissing {
		return buildHost, nil
	}
	return buildHost, fmt.Errorf(
		"cannot build: vee is not installed (needed for the default container build) "+
			"and the host build toolchain is missing.\n\n"+
			"Install vee (https://github.com/Benehiko/vee) to build with no toolchain, or install the toolchain:\n%s",
		prereq.Guidance(prereq.Missing()))
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

// maybeDeleteDockerVM offers to delete the docker-build VM after a successful
// build. Like maybeDeleteVM, the default is to keep it for faster rebuilds.
func maybeDeleteDockerVM(cmd *cobra.Command, yes bool) error {
	prompt := stdinPrompt(cmd)
	if yes || prompt == nil {
		return nil // keep the VM (non-destructive default)
	}
	del, err := prompt("Delete the docker-build VM now? (keep it for faster rebuilds)")
	if err != nil || !del {
		return err
	}
	return dockerbuild.Delete(cmd.Context(), cmd.OutOrStdout())
}
