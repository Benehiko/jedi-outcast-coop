package paks

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Benehiko/jedi-outcast-coop/internal/pk3"
)

// The SP menus that carry the mouse-sensitivity slider. The MP menus under
// ui/jk2mp/ are a separate slider and left alone.
var sensitivityMenus = []string{"controls.menu", "ingamecontrols.menu"}

// The retail slider line uses three tabs between the keyword and the cvar name:
//
//	cvarfloat			"sensitivity" 5 2 30
//
// We match the whole stock token so a menu that was already rescaled (or a
// different edition) is skipped, not corrupted.
const (
	sensitivityTabs   = "\t\t\t"
	sensitivityStock  = "cvarfloat" + sensitivityTabs + `"sensitivity" 5 2 30`
	sensitivityPrefix = "cvarfloat" + sensitivityTabs + `"sensitivity"`
)

// SensitivityRange is the slider default/min/max to write.
type SensitivityRange struct {
	Default, Min, Max string
}

// DefaultSensitivityRange rescales the slider to 0.5 default, 0.1 min, 2 max —
// small enough that dragging gives ~0.1 granularity across the bar, with 2.0 at
// the top. (The engine has no explicit slider step, so this is smooth, not
// hard-snapped; exact values can still be typed with `sensitivity <n>`.)
var DefaultSensitivityRange = SensitivityRange{Default: "0.5", Min: "0.1", Max: "2"}

var numberRe = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)

// ParseSensitivityRange parses a "DEFAULT MIN MAX" string into a range,
// validating each field is a non-negative number.
func ParseSensitivityRange(s string) (SensitivityRange, error) {
	fields := strings.Fields(s)
	if len(fields) != 3 {
		return SensitivityRange{}, fmt.Errorf("--range must be three numbers 'DEFAULT MIN MAX' (got: %q)", s)
	}
	for _, v := range fields {
		if !numberRe.MatchString(v) {
			return SensitivityRange{}, fmt.Errorf("--range must be three numbers 'DEFAULT MIN MAX' (got: %q)", s)
		}
	}
	return SensitivityRange{Default: fields[0], Min: fields[1], Max: fields[2]}, nil
}

// SensitivityResult reports what BuildSensitivity produced.
type SensitivityResult struct {
	OutPath string
	Patched []string // menu files patched (e.g. "ui/controls.menu")
	Skipped []string // menu files skipped, with a reason
	Range   SensitivityRange
}

// BuildSensitivity rescales the SP CONTROLS "Mouse Sensitivity" slider so it
// covers a modern, fine-grained low range.
//
// The menu files belong to Raven and live inside the retail assets*.pk3, so this
// repo does not ship them. It reads them from the user's own copy (the
// assets*.pk3 in assetsDir), rewrites only the one sensitivity cvarfloat line,
// and writes an override pak. Retail assets are never modified; removing the
// feature is a single rm of the output pak. The replace is byte-exact to
// preserve the files' CRLF line endings and latin-1 encoding.
func BuildSensitivity(assetsDir, outPath string, r SensitivityRange) (*SensitivityResult, error) {
	paksList, err := filepath.Glob(filepath.Join(assetsDir, "assets*.pk3"))
	if err != nil {
		return nil, err
	}
	if len(paksList) == 0 {
		return nil, fmt.Errorf("no assets*.pk3 in %q — point --assets at your retail base/", assetsDir)
	}

	stage, err := os.MkdirTemp("", "jk2-sensitivity-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	uiDir := filepath.Join(stage, "ui")
	if err := os.MkdirAll(uiDir, 0o755); err != nil {
		return nil, err
	}

	newLine := fmt.Appendf(nil, "cvarfloat%s%q %s %s %s", sensitivityTabs, "sensitivity", r.Default, r.Min, r.Max)
	stock := []byte(sensitivityStock)

	res := &SensitivityResult{OutPath: outPath, Range: r}
	for _, name := range sensitivityMenus {
		data, ok := readMenuWithSensitivity(paksList, name)
		if !ok {
			res.Skipped = append(res.Skipped, fmt.Sprintf("ui/%s (no retail pak carries it with a sensitivity slider)", name))
			continue
		}
		if !bytes.Contains(data, stock) {
			res.Skipped = append(res.Skipped, fmt.Sprintf("ui/%s (sensitivity slider not in the expected stock form — already rescaled or different edition)", name))
			continue
		}
		patched := bytes.ReplaceAll(data, stock, newLine)
		if err := os.WriteFile(filepath.Join(uiDir, name), patched, 0o644); err != nil {
			return nil, err
		}
		res.Patched = append(res.Patched, "ui/"+name)
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

// readMenuWithSensitivity finds the retail pak that carries ui/<name> with a
// sensitivity slider and returns its bytes. The lookup is case-insensitive on
// the entry (retail ships ui/controls.menu, some editions ui/Controls.menu).
func readMenuWithSensitivity(paksList []string, name string) ([]byte, bool) {
	want := "ui/" + name
	for _, p := range paksList {
		r, err := pk3.Open(p)
		if err != nil {
			continue
		}
		data, err := r.ReadFile(want) // ReadFile is already case-insensitive.
		_ = r.Close()
		if err == nil && bytes.Contains(data, []byte(sensitivityPrefix)) {
			return data, true
		}
	}
	return nil, false
}
