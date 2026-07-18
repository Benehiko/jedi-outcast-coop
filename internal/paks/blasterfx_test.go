package paks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Benehiko/jedi-outcast-coop/internal/pk3"
)

// writeFile writes content to path, creating parent dirs.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildBlasterFX(t *testing.T) {
	src := t.TempDir()
	// Author two effect scripts under effects/blaster, the layout the installer
	// mirrors from assets/blaster-fx.
	writeFile(t, filepath.Join(src, "effects", "blaster", "wall_impact.efx"), "Decal\n{\n\tshaders\n\t[\n\t\tgfx/damage/burnmark1\n\t]\n}\n")
	writeFile(t, filepath.Join(src, "effects", "blaster", "flesh_impact.efx"), "Particle\n{\n\tcount 1\n}\n")

	out := filepath.Join(src, "zz-blaster-fx.pk3")
	arcs, err := BuildBlasterFX(src, out)
	if err != nil {
		t.Fatalf("BuildBlasterFX: %v", err)
	}
	// Both effects must be packed under the effects/blaster/ prefix.
	joined := strings.Join(arcs, "\n")
	for _, want := range []string{"effects/blaster/wall_impact.efx", "effects/blaster/flesh_impact.efx"} {
		if !strings.Contains(joined, want) {
			t.Errorf("pak missing %s (got %v)", want, arcs)
		}
	}

	r, err := pk3.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	data, err := r.ReadFile("effects/blaster/wall_impact.efx")
	if err != nil {
		t.Fatalf("wall_impact.efx missing from pak: %v", err)
	}
	if !strings.Contains(string(data), "burnmark1") {
		t.Fatalf("wall_impact.efx content lost: %q", data)
	}
}

func TestBuildBlasterFXMissing(t *testing.T) {
	// No effects/ subtree → error rather than an empty pak.
	if _, err := BuildBlasterFX(t.TempDir(), filepath.Join(t.TempDir(), "o.pk3")); err == nil {
		t.Fatal("expected error when effects/ absent")
	}
}
