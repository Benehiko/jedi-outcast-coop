package install

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseLibraryFolders(t *testing.T) {
	dir := t.TempDir()
	vdf := filepath.Join(dir, "libraryfolders.vdf")
	content := `"libraryfolders"
{
	"0"
	{
		"path"		"/home/u/.local/share/Steam"
	}
	"1"
	{
		"path"		"/mnt/games/SteamLibrary"
	}
}`
	if err := os.WriteFile(vdf, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got := parseLibraryFolders(vdf)
	want := []string{"/home/u/.local/share/Steam", "/mnt/games/SteamLibrary"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("path %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseLibraryFoldersWindowsEscaping(t *testing.T) {
	dir := t.TempDir()
	vdf := filepath.Join(dir, "libraryfolders.vdf")
	// Windows VDF escapes backslashes as \\.
	content := `"libraryfolders" { "0" { "path" "C:\\Program Files (x86)\\Steam" } }`
	if err := os.WriteFile(vdf, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got := parseLibraryFolders(vdf)
	if len(got) != 1 || got[0] != `C:\Program Files (x86)\Steam` {
		t.Fatalf("got %v", got)
	}
}

func TestSteamLibrariesDedup(t *testing.T) {
	root := t.TempDir()
	// A root is only a library if it has steamapps/.
	if err := os.MkdirAll(filepath.Join(root, "steamapps"), 0o755); err != nil {
		t.Fatal(err)
	}
	// libraryfolders.vdf points back at the same root — must dedup.
	vdf := filepath.Join(root, "steamapps", "libraryfolders.vdf")
	if err := os.WriteFile(vdf, []byte(`"x" { "path" "`+root+`" }`), 0o644); err != nil {
		t.Fatal(err)
	}
	libs := steamLibraries([]string{root})
	if len(libs) != 1 {
		t.Fatalf("expected 1 deduped library, got %v", libs)
	}
}
