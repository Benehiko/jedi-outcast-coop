// Package config owns the user-editable jk2coop config file, the single source
// of truth for the game's tunable settings. The two settings TUIs edit this
// file; install and launch read it and apply it to the game (autoexec cvars,
// which patch-backed graphics features the engine is built with, and which
// optional override paks to build).
//
// The file lives at os.UserConfigDir()/jk2coop/config.toml (e.g.
// ~/.config/jk2coop/config.toml on Linux). It is created on first Save; until
// then Load returns Defaults so a fresh machine behaves like the previous
// flag-driven install with its default choices.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config is the whole user config, split into the two settings menus.
type Config struct {
	Game     Game     `toml:"game"`
	Graphics Graphics `toml:"graphics"`
}

// Game holds the "Game Settings" menu values. All of these are runtime engine
// cvars written to autoexec_sp.cfg, so changing them never needs a rebuild.
type Game struct {
	// Sensitivity is the base mouse sensitivity (seta sensitivity). The stock
	// engine default of 5 is fast on a modern high-DPI mouse; 0.5 (the default
	// here) is calmer.
	Sensitivity float64 `toml:"sensitivity"`
	// BlasterVelocity is the primary blaster bolt speed (seta g_blasterVelocity),
	// exposed by the 0025 patch. Stock is 2300.
	BlasterVelocity int `toml:"blaster_velocity"`
	// AimAssist enables the legacy JK2 feel: saber auto-aim and FOV-linked mouse
	// sensitivity (g_saberAutoAim 1, cg_fovSensitivityScale 1). Off = modern.
	AimAssist bool `toml:"aim_assist"`
	// DynamicCrosshair enables the legacy crosshair that drifts with the weapon
	// muzzle (cg_dynamicCrosshair 1). Off = fixed screen-center crosshair.
	DynamicCrosshair bool `toml:"dynamic_crosshair"`
	// SkipCutscenes auto-skips scripted map-intro cutscenes
	// (g_skipIntroCinematics 1).
	SkipCutscenes bool `toml:"skip_cutscenes"`
}

// Graphics holds the "Graphics Settings" menu values. Widescreen and Lighting
// are patch-backed (they change the engine build and need a rebuild); MSAA is a
// plain cvar; the two texture options build optional GPU override paks.
type Graphics struct {
	// Widescreen enables 16:9/21:9/32:9 aspect correction, extra video modes and
	// an edge-anchored HUD (patch 0023 + zz-widescreen-menu.pk3). Needs rebuild.
	Widescreen bool `toml:"widescreen"`
	// Lighting enables the software-overbright render-fidelity path plus the
	// matching character-model lighting boost (patch 0024). Needs rebuild.
	Lighting bool `toml:"lighting"`
	// MSAA is the multisample sample count written as r_ext_multisample
	// (0 = off, else 2/4/8). Applied at next launch; no rebuild.
	MSAA int `toml:"msaa"`
	// ResWidth/ResHeight are the game resolution written as r_customwidth /
	// r_customheight with r_mode -1. Both 0 means "auto": leave the engine on its
	// own r_mode default and force no custom size. Applied at next launch; no
	// rebuild.
	ResWidth  int `toml:"res_width"`
	ResHeight int `toml:"res_height"`
	// TextureUpscale builds a Real-ESRGAN hi-res override pak from the retail
	// textures (zzz-hires-textures.pk3). GPU + container gated; no rebuild.
	TextureUpscale bool `toml:"texture_upscale"`
	// TextureGenerate builds an AI material-texture pak
	// (zzz-generated-textures.pk3). GPU + container gated; no rebuild.
	TextureGenerate bool `toml:"texture_generate"`
}

// StockBlasterVelocity is the retail primary blaster bolt speed (weapons.h).
const StockBlasterVelocity = 2300

// Defaults returns the out-of-the-box config. These reproduce the previous
// flag-driven install's default choices: modern combat feel, the mod's intended
// widescreen + render fidelity on, textures off.
func Defaults() Config {
	return Config{
		Game: Game{
			Sensitivity:      0.5,
			BlasterVelocity:  StockBlasterVelocity,
			AimAssist:        false,
			DynamicCrosshair: false,
			SkipCutscenes:    false,
		},
		Graphics: Graphics{
			Widescreen:      true,
			Lighting:        true,
			MSAA:            0,
			ResWidth:        0, // 0x0 = auto (engine picks; no forced custom mode)
			ResHeight:       0,
			TextureUpscale:  false,
			TextureGenerate: false,
		},
	}
}

// Path is the config file location: os.UserConfigDir()/jk2coop/config.toml. The
// JK2COOP_CONFIG env var overrides it (used by tests and power users).
func Path() (string, error) {
	if p := os.Getenv("JK2COOP_CONFIG"); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locating user config dir: %w", err)
	}
	return filepath.Join(dir, "jk2coop", "config.toml"), nil
}

// Load reads the config from Path, overlaying the file's values onto Defaults so
// keys absent from an older file keep sane defaults (forward-compatible
// migration). A missing file is not an error: it returns Defaults unchanged.
func Load() (Config, error) {
	p, err := Path()
	if err != nil {
		return Config{}, err
	}
	return LoadFrom(p)
}

// LoadFrom is Load against an explicit path.
func LoadFrom(path string) (Config, error) {
	cfg := Defaults()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return Config{}, fmt.Errorf("reading %s: %w", path, err)
	}
	if err := toml.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	return cfg, nil
}

// Save writes the config to Path, creating the parent directory if needed.
func (c Config) Save() (string, error) {
	p, err := Path()
	if err != nil {
		return "", err
	}
	return p, c.SaveTo(p)
}

// SaveTo is Save against an explicit path.
func (c Config) SaveTo(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(configHeader); err != nil {
		return err
	}
	if err := toml.NewEncoder(f).Encode(c); err != nil {
		return err
	}
	return f.Close()
}

// configHeader is a comment prepended to the written file so a user opening it
// by hand knows what it is and that the CLI manages it.
const configHeader = "# jk2coop config — edit via `jk2coop game` / `jk2coop graphics`,\n" +
	"# or by hand. Applied to the game on the next install/launch.\n\n"
