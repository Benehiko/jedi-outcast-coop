// Package workdir manages jk2coop's runtime work directory: the place the
// embedded OpenJK source is extracted, patched, and built when jk2coop runs as
// a standalone binary (no repo checkout).
//
// Layout under the work dir (default $XDG_CACHE_HOME/jk2coop, i.e.
// ~/.cache/jk2coop):
//
//	src/            extracted + patched OpenJK source tree
//	src/build/      CMake build output (engine, renderer, gamecode)
//	coop-ui/        extracted co-op UI assets (source for zz-coop-ui.pk3)
//	manifest.json   {pin, gfx selection} describing the current src/ state
//
// The manifest lets jk2coop tell whether src/ already reflects the embedded
// source pin and the desired graphics selection, so it re-extracts and re-patches
// only when something changed — the standalone analogue of the git submodule
// reset the repo flow performs.
package workdir

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Benehiko/jedi-outcast-coop/internal/embed"
)

// Dir is a resolved work directory.
type Dir struct {
	// Root is the work-dir root (e.g. ~/.cache/jk2coop).
	Root string
}

// EnvVar overrides the work-dir root when set.
const EnvVar = "JK2COOP_HOME"

// Resolve returns the work directory, honouring $JK2COOP_HOME, then
// $XDG_CACHE_HOME/jk2coop, then ~/.cache/jk2coop. It does not create anything.
func Resolve() (Dir, error) {
	if v := strings.TrimSpace(os.Getenv(EnvVar)); v != "" {
		abs, err := filepath.Abs(v)
		return Dir{Root: abs}, err
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME")); xdg != "" {
		return Dir{Root: filepath.Join(xdg, "jk2coop")}, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Dir{}, err
	}
	return Dir{Root: filepath.Join(home, ".cache", "jk2coop")}, nil
}

// Src is the extracted+patched OpenJK source tree.
func (d Dir) Src() string { return filepath.Join(d.Root, "src") }

// Build is the CMake build directory.
func (d Dir) Build() string { return filepath.Join(d.Src(), "build") }

// CoopUI is the extracted co-op UI asset dir.
func (d Dir) CoopUI() string { return filepath.Join(d.Root, "coop-ui") }

// BlasterFX is the extracted blaster impact-FX asset dir (source for
// zz-blaster-fx.pk3).
func (d Dir) BlasterFX() string { return filepath.Join(d.Root, "blaster-fx") }

// manifestPath is where the state manifest lives.
func (d Dir) manifestPath() string { return filepath.Join(d.Root, "manifest.json") }

// Manifest records what the extracted src/ tree currently reflects.
type Manifest struct {
	// Pin is the embedded OpenJK commit the src/ tree was extracted from.
	Pin string `json:"pin"`
	// Gfx is the sorted list of graphics feature keys currently patched in.
	Gfx []string `json:"gfx"`
}

// ReadManifest loads the manifest, returning a zero Manifest (not an error) when
// none exists yet.
func (d Dir) ReadManifest() (Manifest, error) {
	b, err := os.ReadFile(d.manifestPath())
	if os.IsNotExist(err) {
		return Manifest{}, nil
	}
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// WriteManifest persists the manifest, creating the work dir if needed.
func (d Dir) WriteManifest(m Manifest) error {
	if err := os.MkdirAll(d.Root, 0o755); err != nil {
		return err
	}
	sort.Strings(m.Gfx)
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(d.manifestPath(), append(b, '\n'), 0o644)
}

// ExtractPristine removes any existing src/ tree and re-extracts the embedded
// OpenJK source into it, giving a clean pristine tree at the embedded pin. It
// does NOT write the manifest — the caller does that after patching, so a crash
// mid-patch leaves a manifest that does not falsely claim the tree is ready.
func (d Dir) ExtractPristine() error {
	src := d.Src()
	if err := os.RemoveAll(src); err != nil {
		return err
	}
	if err := os.MkdirAll(src, 0o755); err != nil {
		return err
	}
	return embed.ExtractSource(src)
}

// NeedsRebuild reports whether the src/ tree must be re-extracted and re-patched
// to match the embedded pin and the wanted graphics selection. want is the
// sorted list of enabled feature keys.
func (d Dir) NeedsRebuild(want []string) (bool, error) {
	m, err := d.ReadManifest()
	if err != nil {
		return true, err
	}
	if m.Pin != embed.Pin() {
		return true, nil
	}
	if _, err := os.Stat(d.Src()); err != nil {
		return true, nil //nolint:nilerr // a missing src tree means "must rebuild", not an error to surface
	}
	return !equalStrings(m.Gfx, want), nil
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	as := append([]string(nil), a...)
	bs := append([]string(nil), b...)
	sort.Strings(as)
	sort.Strings(bs)
	for i := range as {
		if as[i] != bs[i] {
			return false
		}
	}
	return true
}
