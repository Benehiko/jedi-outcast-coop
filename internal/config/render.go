package config

import (
	"bytes"
	"fmt"
	"strconv"
)

// gfx feature keys, mirrored from internal/gfx.Features. Kept as local
// constants so this package does not import internal/gfx (which would be a
// cycle: install imports both). The install layer maps these onto gfx.Apply.
const (
	FeatureWidescreen     = "widescreen"
	FeatureRenderFidelity = "render-fidelity"
)

// AutoexecBytes renders the autoexec_sp.cfg contents from the config. The engine
// execs this file at startup (after openjo_sp.cfg), so these values win over any
// stale persisted cvars. Every setting here is a plain archive cvar — writing
// the file is enough, no rebuild required.
func (c Config) AutoexecBytes() []byte {
	var b bytes.Buffer
	b.WriteString("// Written by jk2coop — generated from your config.toml.\n")
	b.WriteString("// Edit via `jk2coop game` / `jk2coop graphics`, not here (this is regenerated).\n")

	// Combat feel.
	b.WriteString(seta("g_saberAutoAim", boolCvar(c.Game.AimAssist)))
	b.WriteString(seta("cg_dynamicCrosshair", boolCvar(c.Game.DynamicCrosshair)))
	b.WriteString(seta("cg_fovSensitivityScale", boolCvar(c.Game.AimAssist)))
	b.WriteString(seta("g_skipIntroCinematics", boolCvar(c.Game.SkipCutscenes)))
	b.WriteString(seta("sensitivity", formatFloat(c.Game.Sensitivity)))
	b.WriteString(seta("g_blasterVelocity", strconv.Itoa(c.Game.BlasterVelocity)))

	// Render-fidelity preset. With lighting on, the software-overbright path (from
	// the 0024 patch) is paired with the companion fidelity cvars. With it off, the
	// same cvars are pinned back to retail defaults so a machine previously built
	// with lighting is fully reverted rather than left with latched values.
	b.WriteString("\n// render fidelity preset\n")
	for _, kv := range renderPreset(c.Graphics.Lighting) {
		b.WriteString(seta(kv.cvar, kv.val))
	}

	// MSAA is user-controlled (jk2coop graphics), independent of the preset, so it
	// is written last and wins over any preset value.
	b.WriteString(seta("r_ext_multisample", strconv.Itoa(c.Graphics.MSAA)))

	// Resolution. A non-zero size selects the custom video mode (r_mode -1) at the
	// requested width/height; 0x0 leaves the engine on its own r_mode so the game
	// picks a mode as before.
	if c.Graphics.ResWidth > 0 && c.Graphics.ResHeight > 0 {
		b.WriteString("\n// resolution\n")
		b.WriteString(seta("r_mode", "-1"))
		b.WriteString(seta("r_customwidth", strconv.Itoa(c.Graphics.ResWidth)))
		b.WriteString(seta("r_customheight", strconv.Itoa(c.Graphics.ResHeight)))
	}

	return b.Bytes()
}

type cvarKV struct{ cvar, val string }

// renderPreset returns the render-fidelity cvars for lighting on (high) or off
// (retail defaults). MSAA is deliberately excluded: it is a separate user
// setting. Mirrors the shell installer's autoexec_render.cfg preset.
func renderPreset(lighting bool) []cvarKV {
	if lighting {
		return []cvarKV{
			{"r_overBrightBitsSoftware", "1"},
			{"r_overBrightBits", "1"},
			{"r_mapOverBrightBits", "2"},
			{"r_gamma", "1.0"},
			{"r_picmip", "0"},
			{"r_ext_compress_textures", "0"},
			{"r_texturebits", "32"},
			{"r_ext_texture_filter_anisotropic", "16"},
			{"r_textureMode", "GL_LINEAR_MIPMAP_LINEAR"},
			{"r_swapInterval", "1"},
			{"r_subdivisions", "1"},
			{"r_lodbias", "-2"},
			{"r_lodscale", "20"},
		}
	}
	return []cvarKV{
		{"r_overBrightBitsSoftware", "0"},
		{"r_overBrightBits", "0"},
		{"r_mapOverBrightBits", "0"},
		{"r_gamma", "1.0"},
		{"r_picmip", "0"},
		{"r_ext_compress_textures", "1"},
		{"r_texturebits", "0"},
		{"r_ext_texture_filter_anisotropic", "16"},
		{"r_textureMode", "GL_LINEAR_MIPMAP_LINEAR"},
		{"r_swapInterval", "0"},
		{"r_subdivisions", "4"},
		{"r_lodbias", "0"},
		{"r_lodscale", "10"},
	}
}

// GfxSelection maps the config onto the gfx feature keys — which patch-backed
// features the engine must be built with. Blaster velocity is now a runtime
// cvar (0025 is always applied), so it is not here; only widescreen and lighting
// are build-time toggles.
func (c Config) GfxSelection() map[string]bool {
	return map[string]bool{
		FeatureWidescreen:     c.Graphics.Widescreen,
		FeatureRenderFidelity: c.Graphics.Lighting,
	}
}

// GPUPaks reports which slow GPU override paks the config wants built.
func (c Config) GPUPaks() (upscale, generate bool) {
	return c.Graphics.TextureUpscale, c.Graphics.TextureGenerate
}

func seta(cvar, val string) string {
	return fmt.Sprintf("seta %s %q\n", cvar, val)
}

func boolCvar(on bool) string {
	if on {
		return "1"
	}
	return "0"
}

// formatFloat renders a sensitivity value without a trailing ".0" for whole
// numbers (so 2.0 -> "2", 0.5 -> "0.5"), matching how the engine expects it.
func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
