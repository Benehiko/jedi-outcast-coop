package paks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Benehiko/jedi-outcast-coop/internal/pk3"
)

// makePak writes a pak at path containing the given archivePath->content members.
func makePak(t *testing.T, path string, members map[string]string) {
	t.Helper()
	stage := t.TempDir()
	b := pk3.NewBuilder()
	for arc, content := range members {
		src := filepath.Join(stage, filepath.FromSlash(arc))
		if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		b.Add(arc, src)
	}
	if err := b.Write(path); err != nil {
		t.Fatal(err)
	}
}

func TestBuildCoopNPCs(t *testing.T) {
	base := t.TempDir()
	// Retail assets0.pk3 stores the NPC config lowercased.
	makePak(t, filepath.Join(base, "assets0.pk3"), map[string]string{
		"ext_data/npcs.cfg": "Kyle\n{\n\tplayerModel kyle\n}\n\nStormtrooper\n{\n}\n",
	})

	outDir := t.TempDir()
	res, err := BuildCoopNPCs(base, outDir)
	if err != nil {
		t.Fatalf("BuildCoopNPCs: %v", err)
	}
	if res.NumDefs != 2 {
		t.Fatalf("NumDefs = %d, want 2", res.NumDefs)
	}

	r, err := pk3.Open(res.OutPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	// The file must be relocated to ext_data/NPCs/jk2npcs.npc.
	data, err := r.ReadFile("ext_data/NPCs/jk2npcs.npc")
	if err != nil {
		t.Fatalf("relocated file missing: %v", err)
	}
	if !strings.Contains(string(data), "Kyle") {
		t.Fatalf("relocated file lost content: %q", data)
	}
}

func TestBuildCoopNPCsMissing(t *testing.T) {
	base := t.TempDir()
	makePak(t, filepath.Join(base, "assets0.pk3"), map[string]string{
		"other/file.txt": "x",
	})
	if _, err := BuildCoopNPCs(base, t.TempDir()); err == nil {
		t.Fatal("expected error when npcs.cfg absent")
	}
}

const stockLine = `cvarFloatList { "640 X 480" 3  @MENUS1_2048_X_1536 10 }`

func TestBuildWidescreen(t *testing.T) {
	assets := t.TempDir()
	// The video menu must contain ui_r_mode for the pak to be selected, plus the
	// exact stock resolution tail on a CRLF line to prove byte-fidelity.
	menu := "// header\r\n" + "ui_r_mode\r\n" + stockLine + "\r\n"
	makePak(t, filepath.Join(assets, "assets1.pk3"), map[string]string{
		"ui/ingamesetup.menu": menu,
		"ui/setup.menu":       menu,
	})

	out := filepath.Join(assets, "zz-widescreen-menu.pk3")
	res, err := BuildWidescreen(assets, out)
	if err != nil {
		t.Fatalf("BuildWidescreen: %v", err)
	}
	if len(res.Patched) != 2 {
		t.Fatalf("patched %d files, want 2 (%v)", len(res.Patched), res.Skipped)
	}

	r, err := pk3.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	data, err := r.ReadFile("ui/ingamesetup.menu")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "2560 X 1440 QHD") {
		t.Fatalf("patched menu missing QHD entry: %q", s)
	}
	// CRLF and the leading content must be preserved exactly.
	if !strings.HasPrefix(s, "// header\r\nui_r_mode\r\n") {
		t.Fatalf("byte fidelity broken: %q", s[:min(40, len(s))])
	}
}

func TestBuildWidescreenSkipsAlreadyPatched(t *testing.T) {
	assets := t.TempDir()
	// A menu with ui_r_mode but NOT the stock tail (already patched / different
	// edition) must be skipped, not corrupted — and with no patchable file the
	// build fails rather than shipping an empty pak.
	menu := "ui_r_mode\r\ncvarFloatList { something else }\r\n"
	makePak(t, filepath.Join(assets, "assets1.pk3"), map[string]string{
		"ui/ingamesetup.menu": menu,
	})
	if _, err := BuildWidescreen(assets, filepath.Join(assets, "o.pk3")); err == nil {
		t.Fatal("expected error when no menu has the stock resolution list")
	}
}
