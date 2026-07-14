package gfxprobe

import (
	"context"
	"testing"

	"github.com/Benehiko/jedi-outcast-coop/internal/install"
)

// noEnginePlatform points at a build dir with no engine binary, so the probe
// cannot run. These tests assert the "unprobeable" contract without needing a
// display server or a built engine (so they are safe in CI).
func noEnginePlatform() (install.Platform, string) {
	return install.Platform{OS: "linux", Arch: "x86_64", DataDir: "/nonexistent"},
		"/nonexistent/build"
}

func TestMSAASupportedOffAlwaysTrue(t *testing.T) {
	p, dir := noEnginePlatform()
	ok, probed := MSAASupported(context.Background(), p, dir, 0)
	if !ok || !probed {
		t.Errorf("MSAA off must be supported without probing; got ok=%v probed=%v", ok, probed)
	}
}

func TestMSAASupportedNoEngineIsUnprobed(t *testing.T) {
	p, dir := noEnginePlatform()
	ok, probed := MSAASupported(context.Background(), p, dir, 8)
	if probed {
		t.Errorf("with no engine the probe must report probed=false; got ok=%v probed=%v", ok, probed)
	}
}

func TestHighestSupportedKeepsChoiceWhenUnprobeable(t *testing.T) {
	p, dir := noEnginePlatform()
	// want=16 cannot be probed (no engine) → the user's choice is kept unchanged,
	// never silently lowered.
	got, probed := HighestSupportedMSAA(context.Background(), p, dir, 16, []int{2, 4, 8, 16})
	if probed {
		t.Errorf("unprobeable machine must report probed=false; got probed=%v", probed)
	}
	if got != 16 {
		t.Errorf("unprobeable machine must keep the requested level; want 16, got %d", got)
	}
}

func TestHighestSupportedOffIsAlwaysOff(t *testing.T) {
	p, dir := noEnginePlatform()
	got, probed := HighestSupportedMSAA(context.Background(), p, dir, 0, []int{2, 4, 8, 16})
	if got != 0 || !probed {
		t.Errorf("MSAA off needs no probe; want 0/true, got %d/%v", got, probed)
	}
}

func TestContainsAny(t *testing.T) {
	subs := [][]byte{[]byte("EGL config"), []byte("no display modes")}
	if !containsAny([]byte("...Couldn't find matching EGL config..."), subs) {
		t.Error("should match a failure signature substring")
	}
	if containsAny([]byte("Using 24 color bits, 24 depth"), subs) {
		t.Error("a clean success line must not match a failure signature")
	}
}
