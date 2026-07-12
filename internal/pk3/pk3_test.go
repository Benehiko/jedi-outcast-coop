package pk3

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildAndRead(t *testing.T) {
	dir := t.TempDir()
	// Two source files plus a dotfile and a .pk3 that must be excluded by AddTree.
	writeFile(t, filepath.Join(dir, "ui", "a.menu"), "alpha")
	writeFile(t, filepath.Join(dir, "ui", "sub", "b.menu"), "beta")
	writeFile(t, filepath.Join(dir, "ui", ".hidden"), "nope")
	writeFile(t, filepath.Join(dir, "ui", "old.pk3"), "nope")

	b := NewBuilder()
	if err := b.AddTree(filepath.Join(dir, "ui"), "ui", ".pk3"); err != nil {
		t.Fatalf("AddTree: %v", err)
	}
	got := b.ArchivePaths()
	want := []string{"ui/a.menu", "ui/sub/b.menu"}
	if len(got) != len(want) {
		t.Fatalf("archive paths = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("archive path %d = %q, want %q", i, got[i], want[i])
		}
	}

	out := filepath.Join(dir, "out.pk3")
	if err := b.Write(out); err != nil {
		t.Fatalf("Write: %v", err)
	}

	r, err := Open(out)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = r.Close() }()
	if data, _ := r.ReadFile("ui/a.menu"); string(data) != "alpha" {
		t.Fatalf("ui/a.menu = %q, want alpha", data)
	}
	if data, _ := r.ReadFile("ui/sub/b.menu"); string(data) != "beta" {
		t.Fatalf("ui/sub/b.menu = %q, want beta", data)
	}
}

func TestReadFileCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "ext_data", "npcs.cfg"), "data")
	out := filepath.Join(dir, "out.pk3")
	b := NewBuilder()
	if err := b.AddTree(filepath.Join(dir, "ext_data"), "ext_data"); err != nil {
		t.Fatal(err)
	}
	if err := b.Write(out); err != nil {
		t.Fatal(err)
	}
	r, err := Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	// Engine stores lowercased; caller asks canonical case.
	data, err := r.ReadFile("ext_data/NPCs.cfg")
	if err != nil {
		t.Fatalf("case-insensitive read: %v", err)
	}
	if string(data) != "data" {
		t.Fatalf("got %q", data)
	}
}

func TestDeterministicOutput(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "x", "1.txt"), "one")
	writeFile(t, filepath.Join(dir, "x", "2.txt"), "two")
	build := func() []byte {
		b := NewBuilder()
		if err := b.AddTree(filepath.Join(dir, "x"), "x"); err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(t.TempDir(), "o.pk3")
		if err := b.Write(out); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	// Same inputs → identical bytes (sorted entry order, fixed method).
	first, second := build(), build()
	if string(first) != string(second) {
		t.Fatal("pak output is not deterministic")
	}
}

func TestWriteEmptyFails(t *testing.T) {
	if err := NewBuilder().Write(filepath.Join(t.TempDir(), "e.pk3")); err == nil {
		t.Fatal("expected error writing empty pak")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
