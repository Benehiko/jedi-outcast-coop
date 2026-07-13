package install

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManifestUninstallOrder(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	baseDir := filepath.Join(dataDir, "base")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A file inside base, plus the two dirs, tracked in "created" order.
	pak := filepath.Join(baseDir, "assets0.pk3")
	if err := os.WriteFile(pak, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	manPath := filepath.Join(dataDir, ".manifest")
	man, err := LoadManifest(manPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{baseDir, dataDir, pak} {
		if err := man.Add(p); err != nil {
			t.Fatal(err)
		}
	}

	// Reload from disk to prove persistence, then uninstall.
	man2, err := LoadManifest(manPath)
	if err != nil {
		t.Fatal(err)
	}
	man2.Uninstall()

	// Everything the manifest created must be gone: the file, both dirs, the
	// manifest itself. base/ must be removed before data/ (deepest-first).
	for _, p := range []string{pak, baseDir, dataDir, manPath} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("%s still exists after uninstall", p)
		}
	}
}

func TestManifestLeavesForeignFiles(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A file we did NOT create sits in the tracked dir.
	foreign := filepath.Join(dataDir, "user-save.dat")
	if err := os.WriteFile(foreign, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	man, err := LoadManifest(filepath.Join(dataDir, ".manifest"))
	if err != nil {
		t.Fatal(err)
	}
	if err := man.Add(dataDir); err != nil {
		t.Fatal(err)
	}
	man.Uninstall()

	// The non-empty dir (and the foreign file) must survive.
	if _, err := os.Stat(foreign); err != nil {
		t.Fatalf("foreign file was removed: %v", err)
	}
}

func TestManifestAddIdempotent(t *testing.T) {
	dir := t.TempDir()
	man, err := LoadManifest(filepath.Join(dir, ".manifest"))
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err := man.Add("/some/path"); err != nil {
			t.Fatal(err)
		}
	}
	if len(man.Entries()) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(man.Entries()))
	}
}
