package dockerbuild

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strings"
	"time"

	veepkg "github.com/Benehiko/jedi-outcast-coop/internal/vee"
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

// guestUser is the login user inside the docker-template VM. The vee docker
// template is Alpine and provisions the ssh key onto Alpine's cloud-init
// default user "alpine". vee's `ssh` otherwise defaults the login to the host
// $USER, which does not exist in the guest — so every `vee ssh` must pass
// `--user alpine` explicitly or auth fails with "Permission denied (publickey)".
const guestUser = "alpine"

// execCommand, veeResolve and runGuestPrep are indirected for testing.
var (
	execCommand = exec.CommandContext
	veeResolve  = veepkg.Resolve
	// runGuestPrep runs the guest-prep script once over ssh. Indirected so the
	// retry loop in prepareGuest can be tested without a real VM.
	runGuestPrep = veeSSH
)

// Available reports whether vee is present (on PATH or downloaded into the
// jk2coop config dir) — a precondition for the docker build path, since the host
// needs no Docker of its own, only vee.
func Available() bool {
	_, ok := veeResolve()
	return ok
}

// veeBin resolves the vee binary path (PATH first, then the managed config-dir
// copy). Callers reach the docker path only after Available() / vee.Ensure, so a
// missing binary here is a logic error surfaced as a clear command failure.
func veeBin() string {
	if p, ok := veeResolve(); ok {
		return p
	}
	return "vee"
}

// vee runs `vee <args…>`, streaming combined output to out.
func vee(ctx context.Context, out io.Writer, args ...string) error {
	cmd := execCommand(ctx, veeBin(), args...)
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("vee %s: %w", args[0], err)
	}
	return nil
}

// veeCreate ensures the docker-build VM is up:
//   - already running   → nothing to do (a warm re-run reuses it);
//   - exists but stopped → start it;
//   - absent            → create the docker-template VM with shareDir shared.
//
// `vee create` would wipe a warm VM and `vee start` errors on an
// already-running one, so both are guarded — keeping setup idempotent across
// re-runs.
func veeCreate(ctx context.Context, shareDir string, out io.Writer) error {
	switch vmState(ctx) {
	case vmRunning:
		return nil
	case vmStopped:
		return vee(ctx, out, "start", VMName)
	default:
		return vee(ctx, out,
			"create", VMName,
			"--template", "docker",
			"--headless",
			"--virtiofs-dir", shareDir,
			"--virtiofs-tag", virtiofsTag,
		)
	}
}

// vmStateKind is the registration/run state of the build VM as seen by vee.
type vmStateKind int

const (
	vmAbsent  vmStateKind = iota // not registered with vee
	vmStopped                    // registered but not running
	vmRunning                    // registered and running
)

// vmState reports whether VMName is absent, stopped, or running by parsing
// `vee list`. A row for VMName whose fields include "running" is running; any
// other row for VMName is stopped; no row is absent. A failed list is treated
// as absent so the caller falls through to create.
func vmState(ctx context.Context) vmStateKind {
	cmd := execCommand(ctx, veeBin(), "list")
	b, err := cmd.Output()
	if err != nil {
		return vmAbsent
	}
	for line := range strings.SplitSeq(string(b), "\n") {
		fields := strings.Fields(line)
		if !slices.Contains(fields, VMName) {
			continue
		}
		if slices.Contains(fields, "running") {
			return vmRunning
		}
		return vmStopped
	}
	return vmAbsent
}

// veeSSH runs a script inside the guest over `vee ssh`. Everything after `--`
// is the remote command; ssh flattens and re-parses it through the guest shell,
// so the script is base64-encoded to a single quote-free token (same technique
// as internal/vmbuild).
func veeSSH(ctx context.Context, out io.Writer, script string) error {
	enc := base64.StdEncoding.EncodeToString([]byte(script))
	remote := "echo " + enc + " | base64 -d | sh"
	return vee(ctx, out, "ssh", VMName, "--user", guestUser, "--", remote)
}

// guestPrepScript is the shell run inside the guest to mount the virtiofs share
// and ensure dockerd is running. It is idempotent: mounting is guarded by
// mountpoint, and docker is only (re)started when not already up, so re-running
// it after a partial setup is safe.
func guestPrepScript() string {
	return strings.Join([]string{
		"set -eu",
		"doas mkdir -p " + guestMount,
		"mountpoint -q " + guestMount + " || doas mount -t virtiofs " + virtiofsTag + " " + guestMount,
		// Start dockerd if the API socket is not already up.
		"doas rc-service docker status >/dev/null 2>&1 || doas rc-service docker start",
	}, "\n")
}

// guestPrepTimeout bounds how long we retry the guest-prep SSH while the freshly
// booted guest's sshd is not yet accepting connections. guestPrepInterval is the
// gap between attempts. Both are vars so tests can shrink them.
var (
	guestPrepTimeout  = 90 * time.Second
	guestPrepInterval = 3 * time.Second
)

// prepareGuest mounts the virtiofs share and ensures dockerd is running inside
// the guest. The vee docker template installs Docker via cloud-init but does not
// reliably leave it running (the boot-time `service docker start` races the
// package install), so we (re)start it and the caller then waits on /_ping.
//
// A just-created/started VM's sshd is not immediately accepting connections, so
// the first `vee ssh` often fails with "connection reset by peer" (exit 255).
// The prep script is idempotent, so we retry it on transient failures until it
// succeeds or the deadline passes — otherwise a boot-time race would abort setup
// even though the VM is healthy and the daemon comes up moments later.
func prepareGuest(ctx context.Context, out io.Writer) error {
	script := guestPrepScript()
	waitCtx, cancel := context.WithTimeout(ctx, guestPrepTimeout)
	defer cancel()

	var lastErr error
	for {
		// Buffer this attempt's output so a failed (retried) attempt does not spew
		// ssh reset noise; only the succeeding attempt's output reaches the user.
		var buf strings.Builder
		if err := runGuestPrep(waitCtx, &buf, script); err == nil {
			_, _ = io.WriteString(out, buf.String())
			return nil
		} else {
			lastErr = err
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("guest not reachable over ssh within %s: %w", guestPrepTimeout, lastErr)
		case <-time.After(guestPrepInterval):
			_, _ = fmt.Fprint(out, ".")
		}
	}
}

// Delete removes the build VM and its disks. Callers offer this as a prompt
// after a successful build; keeping the VM speeds up re-runs.
func Delete(ctx context.Context, out io.Writer) error {
	_, _ = fmt.Fprintf(out, "Deleting build VM %q…\n", VMName)
	return vee(ctx, out, "delete", VMName, "--yes")
}
