package patchengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Benehiko/jedi-outcast-coop/internal/patchengine"
)

// write creates a file with content under dir and returns dir.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func read(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestApplyModify(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"foo.txt": "one\ntwo\nthree\n",
	})
	patch := `diff --git a/foo.txt b/foo.txt
--- a/foo.txt
+++ b/foo.txt
@@ -1,3 +1,3 @@
 one
-two
+TWO
 three
`
	if err := patchengine.Apply(dir, []byte(patch)); err != nil {
		t.Fatal(err)
	}
	if got, want := read(t, dir, "foo.txt"), "one\nTWO\nthree\n"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestApplyNewFile(t *testing.T) {
	dir := writeTree(t, map[string]string{})
	patch := `diff --git a/sub/new.txt b/sub/new.txt
new file mode 100644
--- /dev/null
+++ b/sub/new.txt
@@ -0,0 +1,2 @@
+hello
+world
`
	if err := patchengine.Apply(dir, []byte(patch)); err != nil {
		t.Fatal(err)
	}
	if got, want := read(t, dir, "sub/new.txt"), "hello\nworld\n"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

// TestApplyOffset verifies whole-hunk offset tolerance: the header says line 10
// but the context actually lives at line 2. git apply absorbs this; so must we.
func TestApplyOffset(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"f.txt": "a\nb\nc\nd\n",
	})
	patch := `diff --git a/f.txt b/f.txt
--- a/f.txt
+++ b/f.txt
@@ -10,2 +10,3 @@
 b
+INSERTED
 c
`
	if err := patchengine.Apply(dir, []byte(patch)); err != nil {
		t.Fatal(err)
	}
	if got, want := read(t, dir, "f.txt"), "a\nb\nINSERTED\nc\nd\n"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestApplyContextMismatch(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"f.txt": "a\nb\nc\n",
	})
	patch := `diff --git a/f.txt b/f.txt
--- a/f.txt
+++ b/f.txt
@@ -1,2 +1,2 @@
 a
-DIFFERENT
+x
`
	if err := patchengine.Apply(dir, []byte(patch)); err == nil {
		t.Fatal("expected context-mismatch error, got nil")
	}
}

func TestApplyMultiHunk(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"f.txt": "l1\nl2\nl3\nl4\nl5\nl6\n",
	})
	patch := `diff --git a/f.txt b/f.txt
--- a/f.txt
+++ b/f.txt
@@ -1,2 +1,2 @@
-l1
+L1
 l2
@@ -5,2 +5,2 @@
 l5
-l6
+L6
`
	if err := patchengine.Apply(dir, []byte(patch)); err != nil {
		t.Fatal(err)
	}
	if got, want := read(t, dir, "f.txt"), "L1\nl2\nl3\nl4\nl5\nL6\n"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestApplyRejectsDeletion(t *testing.T) {
	dir := writeTree(t, map[string]string{"f.txt": "x\n"})
	patch := `diff --git a/f.txt b/f.txt
deleted file mode 100644
--- a/f.txt
+++ /dev/null
@@ -1 +0,0 @@
-x
`
	if err := patchengine.Apply(dir, []byte(patch)); err == nil {
		t.Fatal("expected deletion to be rejected")
	}
}
