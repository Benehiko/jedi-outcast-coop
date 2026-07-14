package dockerbuild

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strings"
)

// VMName is the fixed name of the docker-build VM. Reusing one name lets a
// re-run reuse a warm VM (image already built, deps cached) instead of
// recreating it.
const VMName = "jk2coop-docker"

// Mount tag / guest path for the virtiofs-shared host source tree.
const (
	virtiofsTag = "share"
	guestMount  = "/mnt/jk2coop"
)

// lookPath and execCommand are indirected for testing.
var (
	lookPath    = exec.LookPath
	execCommand = exec.CommandContext
)

// Available reports whether the `vee` tool is on PATH (a precondition for the
// docker build path — the host needs no Docker of its own, only vee).
func Available() bool {
	_, err := lookPath("vee")
	return err == nil
}

// vee runs `vee <args…>`, streaming combined output to out.
func vee(ctx context.Context, out io.Writer, args ...string) error {
	cmd := execCommand(ctx, "vee", args...)
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("vee %s: %w", args[0], err)
	}
	return nil
}

// veeCreate starts the VM if it exists, otherwise creates the docker-template VM
// with shareDir shared over virtiofs. `vee create` would wipe a warm VM, so we
// only create when absent.
func veeCreate(ctx context.Context, shareDir string, out io.Writer) error {
	if vmExists(ctx) {
		return vee(ctx, out, "start", VMName)
	}
	return vee(ctx, out,
		"create", VMName,
		"--template", "docker",
		"--headless",
		"--virtiofs-dir", shareDir,
		"--virtiofs-tag", virtiofsTag,
	)
}

// vmExists reports whether a VM named VMName is already registered with vee.
func vmExists(ctx context.Context) bool {
	cmd := execCommand(ctx, "vee", "list")
	b, err := cmd.Output()
	if err != nil {
		return false
	}
	for line := range strings.SplitSeq(string(b), "\n") {
		if slices.Contains(strings.Fields(line), VMName) {
			return true
		}
	}
	return false
}

// veeSSH runs a script inside the guest over `vee ssh`. Everything after `--`
// is the remote command; ssh flattens and re-parses it through the guest shell,
// so the script is base64-encoded to a single quote-free token (same technique
// as internal/vmbuild).
func veeSSH(ctx context.Context, out io.Writer, script string) error {
	enc := base64.StdEncoding.EncodeToString([]byte(script))
	remote := "echo " + enc + " | base64 -d | sh"
	return vee(ctx, out, "ssh", VMName, "--", remote)
}

// prepareGuest mounts the virtiofs share and ensures dockerd is running inside
// the guest. The vee docker template installs Docker via cloud-init but does not
// reliably leave it running (the boot-time `service docker start` races the
// package install), so we (re)start it and the caller then waits on /_ping.
func prepareGuest(ctx context.Context, out io.Writer) error {
	script := strings.Join([]string{
		"set -eu",
		"doas mkdir -p " + guestMount,
		"mountpoint -q " + guestMount + " || doas mount -t virtiofs " + virtiofsTag + " " + guestMount,
		// Start dockerd if the API socket is not already up.
		"doas rc-service docker status >/dev/null 2>&1 || doas rc-service docker start",
	}, "\n")
	return veeSSH(ctx, out, script)
}

// Delete removes the build VM and its disks. Callers offer this as a prompt
// after a successful build; keeping the VM speeds up re-runs.
func Delete(ctx context.Context, out io.Writer) error {
	_, _ = fmt.Fprintf(out, "Deleting build VM %q…\n", VMName)
	return vee(ctx, out, "delete", VMName, "--yes")
}
