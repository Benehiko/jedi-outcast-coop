// Package vmbuild builds the OpenJK engine inside a disposable QEMU VM managed
// by the user's `vee` tool, for hosts that do not have (and do not want to
// install) the native C/C++ build toolchain.
//
// The host repository is shared into the guest over virtiofs, so the three
// build artifacts the guest writes under openjk/build/ appear back on the host
// automatically — no copy-out step. After the build the host-side install runs
// normally (it only symlinks files and needs no toolchain).
//
// Every step shells out to `vee`; nothing here needs cgo, a build toolchain, or
// network access of its own.
package vmbuild

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strings"

	"github.com/Benehiko/jedi-outcast-coop/internal/gfx"
	"github.com/Benehiko/jedi-outcast-coop/internal/prereq"
	veepkg "github.com/Benehiko/jedi-outcast-coop/internal/vee"
)

// VMName is the fixed name of the build VM. Reusing one name lets a re-run reuse
// a warm VM (deps already installed) instead of recreating it.
const VMName = "jk2coop-build"

// virtiofsTag is the mount tag for the shared host repo; guestMount is where the
// remote build script mounts it inside the guest.
const (
	virtiofsTag = "share"
	guestMount  = "/mnt/jk2coop"
)

// veeResolve is indirected for testing.
var veeResolve = veepkg.Resolve

// Available reports whether vee is present (on PATH or downloaded into the
// jk2coop config dir) — a precondition for any VM build path.
func Available() bool {
	_, ok := veeResolve()
	return ok
}

// veeBin resolves the vee binary path (PATH first, then the managed config-dir
// copy), falling back to the bare name so a command failure is legible.
func veeBin() string {
	if p, ok := veeResolve(); ok {
		return p
	}
	return "vee"
}

// Build creates (or reuses) the build VM with shareDir shared over virtiofs,
// installs the build dependencies inside it, and runs the engine build. srcSub
// is the OpenJK source directory RELATIVE to shareDir (e.g. "openjk" for the dev
// repo flow, "src" for the standalone work-dir flow); the build writes to
// <srcSub>/build. On success the artifacts appear under shareDir/<srcSub>/build
// on the host (the guest writes them back through the virtiofs share). Progress
// streams to out. The co-op patches must already be applied to the shared tree
// on the host.
func Build(ctx context.Context, shareDir, srcSub string, out io.Writer) error {
	if !Available() {
		return fmt.Errorf("vee not found on PATH; install it or build on this machine instead")
	}

	_, _ = fmt.Fprintf(out, "Creating build VM %q (sharing %s over virtiofs)…\n", VMName, shareDir)
	if err := create(ctx, shareDir, out); err != nil {
		return err
	}

	_, _ = fmt.Fprintln(out, "Building the engine inside the VM (installing deps, then cmake)…")
	return run(ctx, out, remoteScript(srcSub)...)
}

// Delete removes the build VM and its disks. Callers offer this as a prompt
// after a successful build; keeping the VM speeds up re-runs.
func Delete(ctx context.Context, out io.Writer) error {
	_, _ = fmt.Fprintf(out, "Deleting build VM %q…\n", VMName)
	return run(ctx, out, "delete", VMName, "--yes")
}

// create starts the VM if it already exists, otherwise creates it. `vee create`
// with --reinstall would wipe a warm VM, so we only create when absent.
func create(ctx context.Context, repoRoot string, out io.Writer) error {
	if vmExists(ctx) {
		// Already created on a prior run: just make sure it is running.
		return run(ctx, out, "start", VMName)
	}
	return run(ctx, out,
		"create", VMName,
		"--template", "devbox",
		"--distro", "ubuntu",
		"--headless",
		"--virtiofs-dir", repoRoot,
		"--virtiofs-tag", virtiofsTag,
	)
}

// vmExists reports whether a VM named VMName is already registered with vee.
func vmExists(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, veeBin(), "list")
	b, err := cmd.Output()
	if err != nil {
		return false
	}
	// `vee list` prints one VM per line; match the name as a whole field.
	for line := range strings.SplitSeq(string(b), "\n") {
		if slices.Contains(strings.Fields(line), VMName) {
			return true
		}
	}
	return false
}

// run executes `vee <args…>`, streaming combined output to out.
func run(ctx context.Context, out io.Writer, args ...string) error {
	cmd := exec.CommandContext(ctx, veeBin(), args...)
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("vee %s: %w", args[0], err)
	}
	return nil
}

// buildScript is the remote build: mount the shared repo, install the exact CI
// dependency set, then configure and build with the same flags as the host build
// (gfx.ConfigureArgs). The co-op patches are already applied on the host to the
// shared tree, so the VM only compiles — no patch step here.
func buildScript(srcSub string) string {
	build := srcSub + "/build"
	cmake := "cmake -S " + srcSub + " -B " + build + " " + strings.Join(gfx.ConfigureArgs, " ")
	return strings.Join([]string{
		"set -eu",
		// Mount the shared host dir if the template did not already.
		"sudo mkdir -p " + guestMount,
		"mountpoint -q " + guestMount + " || sudo mount -t virtiofs " + virtiofsTag + " " + guestMount,
		"export DEBIAN_FRONTEND=noninteractive",
		"sudo apt-get update",
		"sudo apt-get install -y --no-install-recommends " + prereq.AptPackages,
		"cd " + guestMount,
		cmake,
		"cmake --build " + build,
	}, "\n")
}

// remoteScript builds the `vee ssh <name> -- <ssh args>` invocation. Everything
// after `--` is passed to ssh(1) as the remote command. ssh flattens the command
// args and re-parses them through the guest's login shell, which mangles spaces,
// quotes, and `&&`. To be immune to that, base64-encode the whole script and
// decode+run it remotely — the transmitted command is a single quote-free token.
func remoteScript(srcSub string) []string {
	enc := base64.StdEncoding.EncodeToString([]byte(buildScript(srcSub)))
	remote := "echo " + enc + " | base64 -d | bash"
	return []string{"ssh", VMName, "--", remote}
}
