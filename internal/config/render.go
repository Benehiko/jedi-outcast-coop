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

	// Graphics cvar (patch-backed graphics go through the engine build, not here).
	b.WriteString(seta("r_ext_multisample", strconv.Itoa(c.Graphics.MSAA)))

	return b.Bytes()
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
