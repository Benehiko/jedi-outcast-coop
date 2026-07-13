package paks

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Benehiko/jedi-outcast-coop/internal/pk3"
)

const stockSensitivityLine = "cvarfloat\t\t\t\"sensitivity\" 5 2 30"

func TestBuildSensitivity(t *testing.T) {
	assets := t.TempDir()
	// controls.menu lives in one pak, ingamecontrols.menu in another — the
	// builder resolves each independently. CRLF is used to prove byte-fidelity.
	menuA := "// controls\r\n" + stockSensitivityLine + "\r\nother\r\n"
	menuB := "// ingame\r\n" + stockSensitivityLine + "\r\n"
	makePak(t, filepath.Join(assets, "assets0.pk3"), map[string]string{
		"ui/controls.menu": menuA,
	})
	makePak(t, filepath.Join(assets, "assets1.pk3"), map[string]string{
		"ui/ingamecontrols.menu": menuB,
	})

	out := filepath.Join(assets, "zz-sensitivity-menu.pk3")
	res, err := BuildSensitivity(assets, out, DefaultSensitivityRange)
	if err != nil {
		t.Fatalf("BuildSensitivity: %v", err)
	}
	if len(res.Patched) != 2 {
		t.Fatalf("patched %d, want 2 (skipped: %v)", len(res.Patched), res.Skipped)
	}

	r, err := pk3.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	data, err := r.ReadFile("ui/controls.menu")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	want := "cvarfloat\t\t\t\"sensitivity\" 0.5 0.1 2"
	if !strings.Contains(s, want) {
		t.Fatalf("rescaled line missing; got %q", s)
	}
	if strings.Contains(s, "5 2 30") {
		t.Fatalf("stock range still present: %q", s)
	}
	// CRLF + leading comment preserved exactly.
	if !strings.HasPrefix(s, "// controls\r\n") {
		t.Fatalf("byte fidelity broken: %q", s)
	}
}

func TestBuildSensitivityCaseInsensitivePak(t *testing.T) {
	assets := t.TempDir()
	// Some editions ship the entry capitalized; the lookup is case-insensitive.
	makePak(t, filepath.Join(assets, "assets0.pk3"), map[string]string{
		"ui/Controls.menu": stockSensitivityLine + "\r\n",
	})
	out := filepath.Join(assets, "o.pk3")
	res, err := BuildSensitivity(assets, out, DefaultSensitivityRange)
	if err != nil {
		t.Fatalf("BuildSensitivity: %v", err)
	}
	if len(res.Patched) != 1 {
		t.Fatalf("patched %d, want 1", len(res.Patched))
	}
}

func TestBuildSensitivitySkipsRescaled(t *testing.T) {
	assets := t.TempDir()
	// A menu whose slider is already rescaled (not the stock 5 2 30) must be
	// skipped, and with nothing patchable the build fails.
	makePak(t, filepath.Join(assets, "assets0.pk3"), map[string]string{
		"ui/controls.menu": "cvarfloat\t\t\t\"sensitivity\" 0.5 0.1 2\r\n",
	})
	if _, err := BuildSensitivity(assets, filepath.Join(assets, "o.pk3"), DefaultSensitivityRange); err == nil {
		t.Fatal("expected error when no menu has the stock slider")
	}
}

func TestParseSensitivityRange(t *testing.T) {
	got, err := ParseSensitivityRange("0.5 0.1 2")
	if err != nil {
		t.Fatal(err)
	}
	if got != (SensitivityRange{Default: "0.5", Min: "0.1", Max: "2"}) {
		t.Fatalf("got %+v", got)
	}
	for _, bad := range []string{"", "1 2", "1 2 3 4", "a b c", "1 2 x"} {
		if _, err := ParseSensitivityRange(bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}
