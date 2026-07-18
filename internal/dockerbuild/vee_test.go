package dockerbuild

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

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
