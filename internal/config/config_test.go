package config

import (
	"os"
	"path/filepath"
	"strconv"
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
	want.Graphics.ResWidth = 2560
	want.Graphics.ResHeight = 1440
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

func TestAutoexecRenderPreset(t *testing.T) {
	// Lighting on → high-fidelity preset.
	on := Defaults()
	on.Graphics.Lighting = true
	on.Graphics.MSAA = 4
	out := string(on.AutoexecBytes())
	for _, want := range []string{
		`seta r_overBrightBitsSoftware "1"`,
		`seta r_picmip "0"`,
		`seta r_lodbias "-2"`,
		`seta r_ext_texture_filter_anisotropic "16"`,
		`seta r_ext_multisample "4"`, // user MSAA, not the preset's 8
	} {
		if !strings.Contains(out, want) {
			t.Errorf("lighting-on autoexec missing %q in:\n%s", want, out)
		}
	}
	// The preset must not hardcode MSAA to 8 (user controls it).
	if strings.Contains(out, `seta r_ext_multisample "8"`) {
		t.Error("preset should not hardcode MSAA 8; MSAA is user-controlled")
	}

	// Lighting off → retail defaults revert the latched cvars.
	off := Defaults()
	off.Graphics.Lighting = false
	outOff := string(off.AutoexecBytes())
	for _, want := range []string{
		`seta r_overBrightBitsSoftware "0"`,
		`seta r_ext_compress_textures "1"`,
		`seta r_lodbias "0"`,
	} {
		if !strings.Contains(outOff, want) {
			t.Errorf("lighting-off autoexec missing %q in:\n%s", want, outOff)
		}
	}
}

func TestAutoexecResolution(t *testing.T) {
	// A set resolution writes the custom video mode at that size.
	c := Defaults()
	c.Graphics.ResWidth = 2560
	c.Graphics.ResHeight = 1440
	out := string(c.AutoexecBytes())
	for _, want := range []string{
		`seta r_mode "-1"`,
		`seta r_customwidth "2560"`,
		`seta r_customheight "1440"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("resolution autoexec missing %q in:\n%s", want, out)
		}
	}

	// Auto (0x0) still pins r_mode -1 with a safe fallback size, so a stale
	// indexed r_mode from a prior run can never wedge startup.
	auto := Defaults() // ResWidth/ResHeight default to 0
	outAuto := string(auto.AutoexecBytes())
	for _, want := range []string{
		`seta r_mode "-1"`,
		`seta r_customwidth "` + strconv.Itoa(AutoResFallback.W) + `"`,
		`seta r_customheight "` + strconv.Itoa(AutoResFallback.H) + `"`,
	} {
		if !strings.Contains(outAuto, want) {
			t.Errorf("auto resolution missing %q in:\n%s", want, outAuto)
		}
	}
	// Auto must never leave a stale indexed r_mode unqualified.
	if strings.Contains(outAuto, `seta r_mode "17"`) {
		t.Errorf("auto resolution must not carry a stale indexed r_mode:\n%s", outAuto)
	}
}

func TestAutoexecFullscreen(t *testing.T) {
	off := Defaults() // windowed by default
	if !strings.Contains(string(off.AutoexecBytes()), `seta r_fullscreen "0"`) {
		t.Errorf("default autoexec must be windowed (r_fullscreen 0):\n%s", off.AutoexecBytes())
	}
	on := Defaults()
	on.Graphics.Fullscreen = true
	if !strings.Contains(string(on.AutoexecBytes()), `seta r_fullscreen "1"`) {
		t.Errorf("fullscreen config must write r_fullscreen 1:\n%s", on.AutoexecBytes())
	}
}

func TestResolutionRow(t *testing.T) {
	// Auto with a detected monitor shows the native hint.
	r := Resolution{}
	row := NewResolutionRow("Resolution", "", &r, Resolution{W: 1920, H: 1080})
	if got := row.Display(); got != "auto (1920x1080)" {
		t.Errorf("auto display = %q, want %q", got, "auto (1920x1080)")
	}
	// Cycling right lands on the suggested native mode first (it is added first).
	row.adjust(+1)
	if r != (Resolution{W: 1920, H: 1080}) {
		t.Fatalf("after one step, got %v, want 1920x1080", r)
	}
	if got := row.Display(); got != "1920x1080 (native)" {
		t.Errorf("native display = %q, want %q", got, "1920x1080 (native)")
	}
	if !row.changed() {
		t.Error("row should report changed after adjusting off the initial auto value")
	}

	// A custom current value that is not in the common list stays selectable.
	odd := Resolution{W: 1234, H: 567}
	oddRow := NewResolutionRow("Resolution", "", &odd, Resolution{})
	oddRow.adjust(-1) // step back to it from wherever the cursor starts
	found := false
	for i := 0; i < len(oddRow.choices); i++ {
		if oddRow.choices[i] == (Resolution{W: 1234, H: 567}) {
			found = true
		}
	}
	if !found {
		t.Error("custom current resolution must be present in the cycle")
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
