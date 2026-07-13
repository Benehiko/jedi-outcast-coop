package install

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Benehiko/jedi-outcast-coop/internal/paks"
)

// Combat modes.
const (
	CombatModern  = "modern"
	CombatClassic = "classic"

	// DefaultSensitivity is written for modern combat. JK2's engine default is
	// 5, which is fast on a modern high-DPI mouse; 0.5 is a calmer start.
	DefaultSensitivity = "0.5"
)

// writeCombatConfig writes base/autoexec_sp.cfg with the modern-combat cvars (or
// the classic feel) plus the optional cutscene auto-skip. The engine execs
// autoexec_sp.cfg on startup after openjo_sp.cfg, so these values take effect
// even if an older config on disk persisted the legacy ones. The cfg is
// manifest-tracked so uninstall removes it.
//
// In modern mode it also builds zz-sensitivity-menu.pk3 so the CONTROLS slider
// can reach and fine-tune the lower modern values (retail min is 2).
func writeCombatConfig(man *Manifest, opts *Options, baseDir string) error {
	mode := opts.Combat
	if mode == "" {
		mode = CombatModern
	}
	sens := opts.Sensitivity
	if sens == "" {
		sens = DefaultSensitivity
	}

	var aim, xhair, sensScale int
	var desc string
	if mode == CombatClassic {
		aim, xhair, sensScale = 1, 1, 1
		desc = "classic (legacy auto-aim, dynamic crosshair, FOV-linked sensitivity)"
	} else {
		aim, xhair, sensScale = 0, 0, 0
		desc = "modern (free aim, fixed crosshair, FOV-independent sensitivity)"
	}

	skipYes, err := opts.resolveOpt(opts.SkipCutscenes, "Auto-skip scripted map-intro cutscenes?")
	if err != nil {
		return err
	}
	skip := 0
	if skipYes {
		skip = 1
	}

	var b []byte
	b = fmt.Appendf(b, "// Written by jk2coop install — modern combat feel.\n")
	b = fmt.Appendf(b, "// Delete this file (or run the installer with --combat classic) to change it.\n")
	b = fmt.Appendf(b, "seta g_saberAutoAim %q\n", fmt.Sprint(aim))
	b = fmt.Appendf(b, "seta cg_dynamicCrosshair %q\n", fmt.Sprint(xhair))
	b = fmt.Appendf(b, "seta cg_fovSensitivityScale %q\n", fmt.Sprint(sensScale))
	b = fmt.Appendf(b, "seta g_skipIntroCinematics %q\n", fmt.Sprint(skip))
	if mode == CombatModern {
		b = fmt.Appendf(b, "seta sensitivity %q\n", sens)
	}

	cfg := filepath.Join(baseDir, "autoexec_sp.cfg")
	if err := os.WriteFile(cfg, b, 0o644); err != nil {
		return err
	}
	if err := man.Add(cfg); err != nil {
		return err
	}
	opts.infof("wrote autoexec_sp.cfg: combat=%s, cutscene-skip=%d", desc, skip)

	// Rescale the CONTROLS mouse-sensitivity slider in modern mode only.
	if mode == CombatModern {
		smPak := filepath.Join(baseDir, "zz-sensitivity-menu.pk3")
		if _, err := paks.BuildSensitivity(baseDir, smPak, paks.DefaultSensitivityRange); err != nil {
			opts.infof("sensitivity-menu build failed: %v", err)
		} else {
			_ = man.Add(smPak)
			opts.infof("installed zz-sensitivity-menu.pk3 (CONTROLS > MOUSE/JOYSTICK slider: 0.1–2)")
		}
	}
	return nil
}
