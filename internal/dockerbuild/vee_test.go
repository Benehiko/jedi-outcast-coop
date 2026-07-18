package dockerbuild

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestVMStateParsesListOutput(t *testing.T) {
	restore := execCommand
	t.Cleanup(func() { execCommand = restore })

	cases := []struct {
		name string
		list string
		want vmStateKind
	}{
		{"running", "NAME            STATUS\n" + VMName + "  docker  2G  2  running  1234  -\n", vmRunning},
		{"stopped", "NAME            STATUS\n" + VMName + "  docker  2G  2  stopped  -     -\n", vmStopped},
		{"absent", "NAME            STATUS\nother-vm  docker  2G  2  running  1  -\n", vmAbsent},
		// "running" appearing for a *different* VM must not mark ours running.
		{"other-running", "NAME\nother  x  running\n" + VMName + "  x  stopped\n", vmStopped},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			execCommand = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
				// Emit tc.list on stdout via a shell echo.
				return exec.CommandContext(ctx, "printf", "%s", tc.list)
			}
			if got := vmState(context.Background()); got != tc.want {
				t.Errorf("vmState = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDeleteStopsRunningVMThenDeletesWithoutFlag(t *testing.T) {
	// vee delete takes no --yes flag (an unknown flag aborts it) and silently
	// no-ops on a running VM, so Delete must stop first, then `delete <VMName>`
	// with no extra args — and in that order.
	restore := execCommand
	t.Cleanup(func() { execCommand = restore })

	var order []string
	execCommand = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		switch args[0] {
		case "list":
			return exec.CommandContext(ctx, "printf", "%s", VMName+"  docker  2G  2  running  1  -\n")
		case "stop":
			order = append(order, "stop")
		case "delete":
			order = append(order, "delete")
			if len(args) != 2 || args[1] != VMName {
				t.Errorf("delete args = %v, want [delete %s]", args, VMName)
			}
		}
		return exec.CommandContext(ctx, "true")
	}

	if err := Delete(context.Background(), io.Discard); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if strings.Join(order, ",") != "stop,delete" {
		t.Errorf("call order = %v, want [stop delete]", order)
	}
}

func TestDeleteSkipsWhenAbsent(t *testing.T) {
	restore := execCommand
	t.Cleanup(func() { execCommand = restore })

	execCommand = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		if args[0] == "delete" {
			t.Error("Delete must not call `vee delete` when the VM is absent")
		}
		// `vee list` returns no matching row → vmAbsent.
		return exec.CommandContext(ctx, "printf", "%s", "other-vm  x  running\n")
	}

	if err := Delete(context.Background(), io.Discard); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestVeeSSHTargetsAlpineUser(t *testing.T) {
	// The docker template provisions the ssh key onto the "alpine" user, so every
	// vee ssh must pass --user alpine or auth fails with Permission denied.
	restore := execCommand
	t.Cleanup(func() { execCommand = restore })

	var gotArgs []string
	execCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		gotArgs = args
		// A no-op command that exits 0, so veeSSH reports success.
		return exec.CommandContext(ctx, "true")
	}

	if err := veeSSH(context.Background(), io.Discard, "echo hi"); err != nil {
		t.Fatalf("veeSSH: %v", err)
	}

	joined := strings.Join(gotArgs, " ")
	if !strings.Contains(joined, "--user "+guestUser) {
		t.Errorf("vee ssh args missing --user %s; got %q", guestUser, joined)
	}
	if gotArgs[0] != "ssh" || !strings.Contains(joined, VMName) {
		t.Errorf("expected `ssh %s …`; got %q", VMName, joined)
	}
	if guestUser != "alpine" {
		t.Errorf("guestUser must be the docker template's cloud-init user; got %q", guestUser)
	}
}

func TestPrepareGuestScript(t *testing.T) {
	// prepareGuest hands guestPrepScript() to ssh; assert its key steps directly
	// through the shared builder.
	script := guestPrepScript()
	for _, want := range []string{
		"mount -t virtiofs " + virtiofsTag,
		guestMount,
		"rc-service docker start",
		"doas", // Alpine has no sudo
	} {
		if !strings.Contains(script, want) {
			t.Errorf("guest prep script missing %q", want)
		}
	}
}

func TestPrepareGuestRetriesUntilSSHReady(t *testing.T) {
	// A freshly booted guest resets the first few ssh connections; prepareGuest
	// must retry rather than abort setup.
	restoreRun, restoreInt := runGuestPrep, guestPrepInterval
	t.Cleanup(func() { runGuestPrep, guestPrepInterval = restoreRun, restoreInt })
	guestPrepInterval = time.Millisecond

	var calls int
	runGuestPrep = func(_ context.Context, out io.Writer, _ string) error {
		calls++
		if calls < 3 {
			return fmt.Errorf("vee ssh: exit status 255") // connection reset during boot
		}
		_, _ = io.WriteString(out, "prepared")
		return nil
	}

	var out strings.Builder
	if err := prepareGuest(context.Background(), &out); err != nil {
		t.Fatalf("prepareGuest: %v", err)
	}
	if calls != 3 {
		t.Errorf("want 3 ssh attempts, got %d", calls)
	}
	// Only the succeeding attempt's stdout should reach the user (progress dots
	// aside); the failed attempts' noise must not leak.
	if !strings.Contains(out.String(), "prepared") {
		t.Errorf("success output missing; got %q", out.String())
	}
}

func TestPrepareGuestGivesUpAfterTimeout(t *testing.T) {
	restoreRun, restoreInt, restoreTO := runGuestPrep, guestPrepInterval, guestPrepTimeout
	t.Cleanup(func() {
		runGuestPrep, guestPrepInterval, guestPrepTimeout = restoreRun, restoreInt, restoreTO
	})
	guestPrepInterval = time.Millisecond
	guestPrepTimeout = 20 * time.Millisecond

	runGuestPrep = func(context.Context, io.Writer, string) error {
		return fmt.Errorf("vee ssh: exit status 255")
	}

	err := prepareGuest(context.Background(), io.Discard)
	if err == nil {
		t.Fatal("want error when ssh never succeeds")
	}
	if !strings.Contains(err.Error(), "exit status 255") {
		t.Errorf("error should wrap the last ssh failure; got %v", err)
	}
}

func TestGuestMountPathUsedInBind(t *testing.T) {
	// The container bind source must be the guest virtiofs mount + srcSub.
	s := containerScript(TargetLinux, "build")
	// Sanity: the container-side path is fixed; the guest side is guestMount.
	if !strings.Contains(s, containerSrc) {
		t.Errorf("containerScript should reference %q", containerSrc)
	}
	if guestMount == "" || !strings.HasPrefix(guestMount, "/") {
		t.Errorf("guestMount must be an absolute path, got %q", guestMount)
	}
}
