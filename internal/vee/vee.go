// Package vee resolves the `vee` VM manager that jk2coop shells out to for its
// container/VM engine builds (internal/dockerbuild and internal/vmbuild).
//
// Resolution order:
//
//  1. a `vee` already on PATH (a system install the user manages themselves), then
//  2. a copy jk2coop downloaded into its own config dir
//     (os.UserConfigDir()/jk2coop/bin/vee).
//
// Ensure downloads that managed copy from the vee GitHub releases when neither
// is present, verifying the published SHA-256 before trusting the binary. Keeping
// the download under the config dir (rather than a throwaway temp dir) means a
// user can rebuild the engine later — `jk2coop install`, a graphics change — and
// reuse the same vee without re-downloading it.
package vee

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// binName is the vee executable name (with the platform's extension).
func binName() string {
	if runtime.GOOS == "windows" {
		return "vee.exe"
	}
	return "vee"
}

// lookPath and userConfigDir are indirected for testing.
var (
	lookPath      = exec.LookPath
	userConfigDir = os.UserConfigDir
)

// ManagedDir is the directory jk2coop downloads its own vee into:
// os.UserConfigDir()/jk2coop/bin.
func ManagedDir() (string, error) {
	dir, err := userConfigDir()
	if err != nil {
		return "", fmt.Errorf("locating user config dir: %w", err)
	}
	return filepath.Join(dir, "jk2coop", "bin"), nil
}

// ManagedPath is the full path to the managed vee binary.
func ManagedPath() (string, error) {
	dir, err := ManagedDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, binName()), nil
}

// Resolve returns a usable vee path without touching the network: a `vee` on
// PATH first, else the managed copy under the config dir if it exists. The
// second return reports whether anything was found.
func Resolve() (string, bool) {
	if p, err := lookPath("vee"); err == nil {
		return p, true
	}
	managed, err := ManagedPath()
	if err != nil {
		return "", false
	}
	if fi, err := os.Stat(managed); err == nil && !fi.IsDir() {
		return managed, true
	}
	return "", false
}

// Available reports whether a usable vee is already present (PATH or managed),
// without downloading. It replaces the old per-package `Available()` probes.
func Available() bool {
	_, ok := Resolve()
	return ok
}

// Ensure returns a usable vee path, downloading the managed copy from GitHub
// releases if none is already present. Progress is written to out. A returned
// path is safe to pass to exec.Command. Ensure is a no-op (no network) when
// Resolve already finds one.
func Ensure(ctx context.Context, out interface{ Write([]byte) (int, error) }) (string, error) {
	if p, ok := Resolve(); ok {
		return p, nil
	}
	return download(ctx, out)
}
