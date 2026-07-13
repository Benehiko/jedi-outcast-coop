//go:build e2e

// Package e2e_test exercises the built jk2coop binary against the real repo.
//
// These tests need the OpenJK submodule checked out and git on PATH, so they
// are gated behind the `e2e` build tag and run via `make e2e` (or
// `go test -tags e2e ./e2e/...`), not the default unit-test run.
package e2e_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot walks up from the test's working directory to the repository root
// (the dir holding patches/, openjk/, and go.mod).
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if hasAll(dir, "patches", "openjk", "go.mod") {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate repository root from " + dir)
		}
		dir = parent
	}
}

func hasAll(dir string, names ...string) bool {
	for _, n := range names {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			return false
		}
	}
	return true
}

// buildBinary compiles jk2coop into a temp dir and returns its path. Honors
// JK2COOP_BIN if the caller pre-built one (e.g. `make e2e`).
func buildBinary(t *testing.T, root string) string {
	t.Helper()
	if bin := os.Getenv("JK2COOP_BIN"); bin != "" {
		return bin
	}
	bin := filepath.Join(t.TempDir(), "jk2coop")
	cmd := exec.CommandContext(t.Context(), "go", "build", "-mod=vendor", "-o", bin, ".")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build jk2coop: %v\n%s", err, out)
	}
	return bin
}

// resetSubmodule returns the OpenJK submodule to a pristine checkout. It is used
// both to set up a clean tree before applying and to clean up afterward, so the
// test never leaves the working copy dirty.
//
// It deliberately uses context.Background(), NOT t.Context(): the test context
// is canceled the moment the test function returns, but this runs from
// t.Cleanup() *after* that — a canceled context would abort the reset and leave
// the submodule dirty.
func resetSubmodule(t *testing.T, root string) {
	t.Helper()
	sub := filepath.Join(root, "openjk")
	if _, err := os.Stat(filepath.Join(sub, ".git")); err != nil {
		t.Skip("openjk submodule not initialized; run: git submodule update --init")
	}
	run(t, context.Background(), root, "git", "-C", sub, "checkout", "--", ".")
	run(t, context.Background(), root, "git", "-C", sub, "clean", "-fdq")
}

func run(t *testing.T, ctx context.Context, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}

// countPatches returns how many *.patch files live under patches/.
func countPatches(t *testing.T, root string) int {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(root, "patches", "*.patch"))
	if err != nil {
		t.Fatal(err)
	}
	return len(matches)
}

// TestPatchesApplyToPristineSubmodule is the core e2e: on a clean submodule,
// `jk2coop patches apply` must apply every patch in order and leave the tree
// changed. This mirrors the CI patches-apply job, but through the Go binary.
func TestPatchesApplyToPristineSubmodule(t *testing.T) {
	root := repoRoot(t)
	bin := buildBinary(t, root)

	resetSubmodule(t, root)
	t.Cleanup(func() { resetSubmodule(t, root) })

	out := run(t, t.Context(), root, bin, "patches", "apply")
	t.Logf("patches apply output:\n%s", out)

	// Every patch file must be reported as applied (none skipped) on a clean tree.
	nPatches := countPatches(t, root)
	nApplied := strings.Count(out, "applied ")
	if nApplied != nPatches {
		t.Fatalf("applied %d patches, want %d (all of patches/*.patch)", nApplied, nPatches)
	}
	if strings.Contains(out, "skip ") {
		t.Fatalf("a patch was skipped on a pristine submodule:\n%s", out)
	}

	// The submodule tree must now show changes (the patches did something).
	status := run(t, t.Context(), root, "git", "-C", filepath.Join(root, "openjk"), "status", "--short")
	if strings.TrimSpace(status) == "" {
		t.Fatal("no files changed in the submodule after applying patches")
	}
	t.Logf("submodule changed %d files", len(strings.Split(strings.TrimSpace(status), "\n")))
}

// TestPatchesApplyNotIdempotentOnDirtyTree pins the documented behavior: the
// patches overlap and are cumulative, so re-applying to an already-patched tree
// must fail (rather than silently double-apply). The user's remedy is to reset.
func TestPatchesApplyNotIdempotentOnDirtyTree(t *testing.T) {
	root := repoRoot(t)
	bin := buildBinary(t, root)

	resetSubmodule(t, root)
	t.Cleanup(func() { resetSubmodule(t, root) })

	// First apply: clean.
	run(t, t.Context(), root, bin, "patches", "apply")

	// Second apply on the now-dirty tree must fail with the reset guidance.
	cmd := exec.CommandContext(t.Context(), bin, "patches", "apply")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("re-applying to a dirty submodule unexpectedly succeeded:\n%s", out)
	}
	if !strings.Contains(string(out), "does not apply cleanly") {
		t.Fatalf("expected the reset guidance in the error, got:\n%s", out)
	}
}
