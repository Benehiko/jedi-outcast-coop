package install

import (
	"os"
	"path/filepath"

	"github.com/Benehiko/jedi-outcast-coop/internal/paks"
)

// writeAutoexec writes base/autoexec_sp.cfg from the user config. The engine
// execs autoexec_sp.cfg on startup (after openjo_sp.cfg), so these values take
// effect even if an older config on disk persisted different ones. The cfg is
// manifest-tracked so uninstall removes it.
//
// It also builds zz-sensitivity-menu.pk3 so the CONTROLS slider can reach and
// fine-tune the lower modern sensitivity values (retail min is 2).
func writeAutoexec(man *Manifest, opts *Options, baseDir string) error {
	cfg := filepath.Join(baseDir, "autoexec_sp.cfg")
	if err := os.WriteFile(cfg, opts.Config.AutoexecBytes(), 0o644); err != nil {
		return err
	}
	if err := man.Add(cfg); err != nil {
		return err
	}
	opts.infof("wrote autoexec_sp.cfg from config")

	// Rescale the CONTROLS mouse-sensitivity slider so it can reach the low
	// modern range. Retail assets are never modified; this is an override pak.
	smPak := filepath.Join(baseDir, "zz-sensitivity-menu.pk3")
	if _, err := paks.BuildSensitivity(baseDir, smPak, paks.DefaultSensitivityRange); err != nil {
		opts.infof("sensitivity-menu build failed: %v", err)
	} else {
		_ = man.Add(smPak)
		opts.infof("installed zz-sensitivity-menu.pk3 (CONTROLS slider: 0.1–2)")
	}
	return nil
}
