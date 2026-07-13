package patches

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListSorted(t *testing.T) {
	dir := t.TempDir()
	// Create out of order; List must return numeric-prefix (lexical) order.
	for _, n := range []string{"0010-c.patch", "0001-a.patch", "0002-b.patch", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a := &Applier{PatchesDir: dir}
	got, err := a.List()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"0001-a.patch", "0002-b.patch", "0010-c.patch"}
	if len(got) != len(want) {
		t.Fatalf("got %d patches, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if filepath.Base(got[i]) != want[i] {
			t.Fatalf("patch %d = %q, want %q", i, filepath.Base(got[i]), want[i])
		}
	}
}

func TestStatusString(t *testing.T) {
	if Applied.String() != "applied" || Skipped.String() != "skip" {
		t.Fatalf("unexpected status strings: %q %q", Applied, Skipped)
	}
}
