package paks

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Benehiko/jedi-outcast-coop/internal/pk3"
)

// BuildBlasterFX packages the enhanced blaster impact effects
// (assets/blaster-fx/effects) into zz-blaster-fx.pk3. The "zz-" prefix sorts it
// after the retail assets*.pk3, so its effects/blaster/*.efx shadow the stock
// impact effects (adding the scorch decal, hotter flash, and lingering smoke).
// Everything packed is original authorship — the .efx scripts are written by
// this project and only reference shaders that already ship in retail assets. No
// retail files are included.
//
// srcDir is the assets/blaster-fx directory (must contain an effects/ subtree).
// outPath is the pak to write. It returns the archive paths written.
func BuildBlasterFX(srcDir, outPath string) ([]string, error) {
	fxDir := filepath.Join(srcDir, "effects")
	if fi, err := os.Stat(fxDir); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("%s not found", fxDir)
	}

	b := pk3.NewBuilder()
	// Pack the effects/ tree under the "effects/" prefix, excluding any stray
	// .pk3 and dotfiles (AddTree already skips dotfiles).
	if err := b.AddTree(fxDir, "effects", ".pk3"); err != nil {
		return nil, err
	}
	if b.Len() == 0 {
		return nil, fmt.Errorf("no files under %s", fxDir)
	}
	if err := os.Remove(outPath); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err := b.Write(outPath); err != nil {
		return nil, err
	}
	return b.ArchivePaths(), nil
}
