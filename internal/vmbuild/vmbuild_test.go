package vmbuild

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/Benehiko/jedi-outcast-coop/internal/gfx"
	"github.com/Benehiko/jedi-outcast-coop/internal/prereq"
)

func TestBuildScriptContents(t *testing.T) {
	s := buildScript()
	for _, want := range []string{
		"set -eu",
		"mount -t virtiofs " + virtiofsTag,
		prereq.AptPackages,
		"cd " + guestMount,
		"cmake --build openjk/build",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("buildScript() missing %q in:\n%s", want, s)
		}
	}
	// The configure line must use the shared, CI-identical flags.
	for _, arg := range gfx.ConfigureArgs {
		if !strings.Contains(s, arg) {
			t.Fatalf("buildScript() missing configure arg %q", arg)
		}
	}
}

func TestRemoteScriptDecodes(t *testing.T) {
	args := remoteScript()
	// Shape: ssh <vm> -- <remote command>
	if len(args) != 4 || args[0] != "ssh" || args[1] != VMName || args[2] != "--" {
		t.Fatalf("unexpected argv: %v", args)
	}
	remote := args[3]
	// The remote command must have no raw newlines/quotes that ssh's flatten +
	// remote-shell re-parse would mangle.
	if strings.ContainsAny(remote, "\n'\"") {
		t.Fatalf("remote command has quoting-fragile chars: %q", remote)
	}
	// It must base64-decode back to exactly the build script.
	const prefix = "echo "
	rest := strings.TrimPrefix(remote, prefix)
	enc, _, ok := strings.Cut(rest, " |")
	if !ok {
		t.Fatalf("remote command not in expected form: %q", remote)
	}
	decoded, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		t.Fatalf("payload is not valid base64: %v", err)
	}
	if string(decoded) != buildScript() {
		t.Fatalf("decoded payload != buildScript()")
	}
}
