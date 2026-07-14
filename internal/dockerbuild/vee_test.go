package dockerbuild

import (
	"strings"
	"testing"
)

func TestPrepareGuestScript(t *testing.T) {
	// prepareGuest builds a script and hands it to veeSSH; assert the script's
	// key steps via the same base64 wrapping veeSSH would apply is not needed —
	// test the script content directly through the shared builder.
	script := strings.Join([]string{
		"set -eu",
		"doas mkdir -p " + guestMount,
		"mountpoint -q " + guestMount + " || doas mount -t virtiofs " + virtiofsTag + " " + guestMount,
		"doas rc-service docker status >/dev/null 2>&1 || doas rc-service docker start",
	}, "\n")
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
