package paks

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Benehiko/jedi-outcast-coop/internal/pk3"
)

// The retail resolution list ends with mode 10 (2048x1536). We match that exact
// tail and append the Track-G modes (13-21, added by patches 0022/0023). If a
// menu's line does not end in the stock way (already patched, or a different
// edition), we skip it rather than corrupt it.
const (
	widescreenTail = `@MENUS1_2048_X_1536 10 }`
	widescreenAdd  = `@MENUS1_2048_X_1536 10  "1280 X 720 (16:9)" 13  "1600 X 900 (16:9)" 14  "1920 X 1080 (16:9)" 15  "2560 X 1080 (21:9)" 16  "2560 X 1440 QHD" 17  "3440 X 1440 (21:9)" 18  "3840 X 1600 (24:10)" 19  "3840 X 2160 4K" 20  "5120 X 1440 (32:9)" 21 }`
)

// widescreenMenus are the two menu files that carry the Video Mode field.
var widescreenMenus = []string{"ingamesetup.menu", "setup.menu"}

// WidescreenResult reports what BuildWidescreen produced.
type WidescreenResult struct {
	OutPath   string
	SourcePak string
	Patched   []string // menu files patched (e.g. "ui/ingamesetup.menu")
	Skipped   []string // menu files skipped, with a reason
}

// BuildWidescreen adds the widescreen/QHD/ultrawide/4K video modes to the SP
// SETUP > VIDEO "Video Mode" menu field.
//
// The two menu files belong to Raven and live inside the retail assets1.pk3, so
// this repo does not ship them. Instead it reads them from the user's own copy
// (the assets*.pk3 in assetsDir), appends the extra resolution entries to the
// single cvarFloatList line that defines the Video Mode field, and writes an
// override pak. Retail assets are never modified; removing the feature is a
// single rm of the output pak.
//
// It operates on raw bytes to preserve the files' CRLF line endings and
// latin-1 encoding exactly.
func BuildWidescreen(assetsDir, outPath string) (*WidescreenResult, error) {
	srcPak, err := findVideoMenuPak(assetsDir)
	if err != nil {
		return nil, err
	}

	r, err := pk3.Open(srcPak)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()

	stage, err := os.MkdirTemp("", "jk2-widescreen-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	uiDir := filepath.Join(stage, "ui")
	if err := os.MkdirAll(uiDir, 0o755); err != nil {
		return nil, err
	}

	res := &WidescreenResult{OutPath: outPath, SourcePak: srcPak}
	tail := []byte(widescreenTail)
	add := []byte(widescreenAdd)

	for _, name := range widescreenMenus {
		arc := "ui/" + name
		data, err := r.ReadFile(arc)
		if err != nil {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s (not in source pak)", arc))
			continue
		}
		n := bytes.Count(data, tail)
		if n == 0 {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s (resolution list not in the expected stock form — already patched or different edition)", arc))
			continue
		}
		if n != 1 {
			return nil, fmt.Errorf("expected exactly one resolution list in %s, found %d", arc, n)
		}
		patched := bytes.Replace(data, tail, add, 1)
		if err := os.WriteFile(filepath.Join(uiDir, name), patched, 0o644); err != nil {
			return nil, err
		}
		res.Patched = append(res.Patched, arc)
	}

	if len(res.Patched) == 0 {
		return nil, fmt.Errorf("no menu files could be patched")
	}

	if err := os.Remove(outPath); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	b := pk3.NewBuilder()
	if err := b.AddTree(uiDir, "ui"); err != nil {
		return nil, err
	}
	if err := b.Write(outPath); err != nil {
		return nil, err
	}
	return res, nil
}

// findVideoMenuPak returns the assets*.pk3 in assetsDir that actually carries
// the SP video menu (ui/ingamesetup.menu containing ui_r_mode).
func findVideoMenuPak(assetsDir string) (string, error) {
	paks, err := filepath.Glob(filepath.Join(assetsDir, "assets*.pk3"))
	if err != nil {
		return "", err
	}
	for _, p := range paks {
		r, err := pk3.Open(p)
		if err != nil {
			continue
		}
		data, err := r.ReadFile("ui/ingamesetup.menu")
		_ = r.Close()
		if err == nil && bytes.Contains(data, []byte("ui_r_mode")) {
			return p, nil
		}
	}
	return "", fmt.Errorf("no assets*.pk3 in %q contains the SP video menu (ui/ingamesetup.menu with ui_r_mode). Point --assets at your retail base/", assetsDir)
}
