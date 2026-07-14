package patchengine_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Benehiko/jedi-outcast-coop/internal/patchengine"
)

// TestGoldenAgainstGitApply is the load-bearing correctness check: it applies
// the full co-op patch set, in order, to a pristine OpenJK tree TWO ways —
// once with `git apply` (the reference), once with the pure-Go engine — and
// asserts the two resulting trees are byte-identical.
//
// It is skipped unless the real submodule is present and is a git checkout
// (needed both as the pristine source and to run the reference `git apply`).
func TestGoldenAgainstGitApply(t *testing.T) {
	repo := repoRoot(t)
	sub := filepath.Join(repo, "openjk")
	if _, err := os.Stat(filepath.Join(sub, ".git")); err != nil {
		t.Skip("openjk submodule is not a git checkout; run: git submodule update --init")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH; needed for the reference apply")
	}
	patches := listPatches(t, filepath.Join(repo, "patches"))
	if len(patches) == 0 {
		t.Fatal("no patches found")
	}

	// Two pristine copies of the tracked source (no .git, no build, no untracked).
	goTree := exportPristine(t, sub)
	gitTree := exportPristine(t, sub)

	// Reference: git apply each patch in order into gitTree.
	initGit(t, gitTree)
	for _, p := range patches {
		cmd := exec.CommandContext(t.Context(), "git", "-C", gitTree, "apply", p)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("reference git apply %s failed: %v\n%s", filepath.Base(p), err, out)
		}
	}

	// Under test: pure-Go engine, same order, into goTree.
	for _, p := range patches {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := patchengine.Apply(goTree, b); err != nil {
			t.Fatalf("patchengine.Apply %s failed: %v", filepath.Base(p), err)
		}
	}

	assertTreesEqual(t, gitTree, goTree)
}

// exportPristine writes the submodule's tracked files (at HEAD) into a fresh
// temp dir, giving a clean pristine tree with no .git.
func exportPristine(t *testing.T, sub string) string {
	t.Helper()
	dst := t.TempDir()
	// `git -C sub archive HEAD | tar -x -C dst` reproduces exactly the tracked
	// tree, honouring .gitattributes, with no untracked/ignored files.
	archive := exec.CommandContext(t.Context(), "git", "-C", sub, "archive", "--format=tar", "HEAD")
	tar := exec.CommandContext(t.Context(), "tar", "-x", "-C", dst)
	pipe, err := archive.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	tar.Stdin = pipe
	if err := tar.Start(); err != nil {
		t.Fatal(err)
	}
	if err := archive.Run(); err != nil {
		t.Fatalf("git archive: %v", err)
	}
	if err := tar.Wait(); err != nil {
		t.Fatalf("tar extract: %v", err)
	}
	return dst
}

// initGit makes dir a git repo so `git apply` (which needs to run inside a work
// tree) operates on it; no commit is needed for `git apply` to a working tree.
func initGit(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "t@t"},
		{"config", "user.name", "t"},
	} {
		cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func listPatches(t *testing.T, dir string) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(dir, "*.patch"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(m)
	return m
}

// assertTreesEqual walks want and asserts got has the same files with identical
// bytes (ignoring the .git dir git-apply's tree carries).
func assertTreesEqual(t *testing.T, want, got string) {
	t.Helper()
	seen := map[string]bool{}
	err := filepath.Walk(want, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(want, p)
		if rel == "." || firstSegment(rel) == ".git" {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		seen[rel] = true
		wb, err := os.ReadFile(p) //nolint:gosec // G122: test-only walk of a trusted t.TempDir() tree
		if err != nil {
			return err
		}
		gb, err := os.ReadFile(filepath.Join(got, rel))
		if err != nil {
			t.Errorf("missing in go tree: %s (%v)", rel, err)
			return nil
		}
		if string(wb) != string(gb) {
			t.Errorf("content differs: %s", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Detect files the go tree has that the reference does not (e.g. a stray write).
	if err := filepath.Walk(got, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(got, p)
		if firstSegment(rel) == ".git" {
			return nil
		}
		if !seen[rel] {
			t.Errorf("go tree has extra file not in reference: %s", rel)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func firstSegment(rel string) string {
	if i := len(rel); i > 0 {
		for j := 0; j < len(rel); j++ {
			if rel[j] == filepath.Separator || rel[j] == '/' {
				return rel[:j]
			}
		}
	}
	return rel
}

// repoRoot finds the repo root by walking up for the patches+openjk markers.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		_, e1 := os.Stat(filepath.Join(dir, "patches"))
		_, e2 := os.Stat(filepath.Join(dir, "openjk"))
		if e1 == nil && e2 == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("repo root (patches/ + openjk/) not found; skipping golden test")
		}
		dir = parent
	}
}
