package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMissingReturnsDefaults(t *testing.T) {
	got, err := LoadFrom(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatalf("LoadFrom missing: %v", err)
	}
	if got != Defaults() {
		t.Fatalf("missing file should yield Defaults, got %+v", got)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	want := Defaults()
	want.Game.Sensitivity = 1.25
	want.Game.BlasterVelocity = 3400
	want.Game.AimAssist = true
	want.Graphics.Widescreen = false
	want.Graphics.MSAA = 4
	want.Graphics.TextureUpscale = true

	if err := want.SaveTo(path); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}
	got, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if got != want {
		t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", got, want)
	}
}

// A partial file (an older jk2coop that never wrote msaa) must keep the default
// for the absent key rather than zero-ing unrelated ones.
func TestLoadOverlaysOntoDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "partial.toml")
	if err := os.WriteFile(path, []byte("[graphics]\nwidescreen = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if got.Graphics.Widescreen {
		t.Fatal("widescreen should be the file's false")
	}
	if !got.Graphics.Lighting {
		t.Fatal("lighting absent from file must keep default true")
	}
	if got.Game.Sensitivity != Defaults().Game.Sensitivity {
		t.Fatalf("sensitivity absent from file must keep default, got %v", got.Game.Sensitivity)
	}
}

func TestAutoexecBytes(t *testing.T) {
	c := Defaults()
	c.Game.Sensitivity = 0.5
	c.Game.BlasterVelocity = 2300
	c.Graphics.MSAA = 4
	out := string(c.AutoexecBytes())

	for _, want := range []string{
		`seta sensitivity "0.5"`,
		`seta g_blasterVelocity "2300"`,
		`seta r_ext_multisample "4"`,
		`seta g_saberAutoAim "0"`,      // AimAssist default off
		`seta cg_dynamicCrosshair "0"`, // DynamicCrosshair default off
		`seta cg_fovSensitivityScale "0"`,
		`seta g_skipIntroCinematics "0"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("autoexec missing %q in:\n%s", want, out)
		}
	}
}

func TestAutoexecClassicFeel(t *testing.T) {
	c := Defaults()
	c.Game.AimAssist = true
	c.Game.DynamicCrosshair = true
	c.Game.SkipCutscenes = true
	out := string(c.AutoexecBytes())
	for _, want := range []string{
		`seta g_saberAutoAim "1"`,
		`seta cg_fovSensitivityScale "1"`,
		`seta cg_dynamicCrosshair "1"`,
		`seta g_skipIntroCinematics "1"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("autoexec missing %q in:\n%s", want, out)
		}
	}
}

func TestGfxSelection(t *testing.T) {
	c := Defaults()
	c.Graphics.Widescreen = true
	c.Graphics.Lighting = false
	sel := c.GfxSelection()
	if !sel[FeatureWidescreen] {
		t.Error("widescreen should be selected")
	}
	if sel[FeatureRenderFidelity] {
		t.Error("render-fidelity should be off")
	}
	if _, ok := sel["modern-combat"]; ok {
		t.Error("modern-combat must not be a selectable feature anymore")
	}
}

func TestGPUPaks(t *testing.T) {
	c := Defaults()
	c.Graphics.TextureUpscale = true
	c.Graphics.TextureGenerate = false
	up, gen := c.GPUPaks()
	if !up || gen {
		t.Fatalf("GPUPaks = (%v, %v), want (true, false)", up, gen)
	}
}

func TestFormatFloat(t *testing.T) {
	for _, tc := range []struct {
		in   float64
		want string
	}{
		{0.5, "0.5"},
		{2, "2"},
		{1.25, "1.25"},
	} {
		if got := formatFloat(tc.in); got != tc.want {
			t.Errorf("formatFloat(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
