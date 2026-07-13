// Package gfx models the optional graphics features as independently
// selectable units on top of the always-applied co-op base patches, and
// (re)applies a chosen selection to the pinned OpenJK submodule.
//
// The patch set splits into two tiers:
//
//   - The co-op BASE: networking, dual-cgame, four-player, menus (0001-0021),
//     plus the cvar-backed combat patches whose settings live in autoexec_sp.cfg
//     (modern-combat 0022, blaster-velocity 0025). Always applied; not
//     user-selectable at build time.
//   - Two GRAPHICS features, each one patch, each independently toggleable in
//     any combination (they add latched renderer cvars, so they must be built in):
//     widescreen       (0023) — 2D aspect correction, vidmodes, HUD anchor
//     render-fidelity  (0024) — software overbright + entity lighting
//
// Because the patches must apply to a PRISTINE submodule (they are cumulative
// and cannot be reverse-checked individually on a dirty tree), changing the
// selection means: reset the submodule to pristine, then apply the base plus
// the selected feature patches in order.
package gfx

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Feature is one optional graphics unit, backed by a single patch file.
type Feature struct {
	// Key is the stable identifier (e.g. "widescreen") used in flags/config.
	Key string
	// Title is the short human name shown in the TUI.
	Title string
	// Desc is a one-line description of what the feature does.
	Desc string
	// Patch is the patch filename (basename) under PatchesDir.
	Patch string
}

// Features is the ordered list of selectable graphics features. Order matches
// the patch numbering, which is also the apply order.
//
// modern-combat (0022) and blaster-velocity (0025) are NOT here: their cvars are
// CVAR_ARCHIVE, so the config drives them purely through autoexec_sp.cfg with no
// rebuild. Those two patches are part of the always-applied co-op base. Only the
// genuinely build-time (latched renderer cvar) features remain selectable.
var Features = []Feature{
	{
		Key:   "widescreen",
		Title: "Widescreen",
		Desc:  "16:9/21:9/32:9 aspect correction, extra video modes, edge-anchored HUD.",
		Patch: "0023-widescreen.patch",
	},
	{
		Key:   "render-fidelity",
		Title: "Render fidelity",
		Desc:  "Software overbright lighting plus a matching boost for character models.",
		Patch: "0024-render-fidelity.patch",
	},
}

// FeatureByKey returns the feature with the given key, or false.
func FeatureByKey(key string) (Feature, bool) {
	for _, f := range Features {
		if f.Key == key {
			return f, true
		}
	}
	return Feature{}, false
}

// Manager applies patch selections to the OpenJK submodule.
type Manager struct {
	// Submodule is the OpenJK checkout the patches apply to.
	Submodule string
	// PatchesDir holds the *.patch files.
	PatchesDir string
	// Git is the git executable (default "git").
	Git string
}

func (m *Manager) git() string {
	if m.Git != "" {
		return m.Git
	}
	return "git"
}

// basePatches returns the co-op base patch paths (everything that is not a
// graphics feature), sorted in apply order.
func (m *Manager) basePatches() ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(m.PatchesDir, "*.patch"))
	if err != nil {
		return nil, err
	}
	feat := map[string]bool{}
	for _, f := range Features {
		feat[f.Patch] = true
	}
	var base []string
	for _, p := range matches {
		if !feat[filepath.Base(p)] {
			base = append(base, p)
		}
	}
	sort.Strings(base)
	return base, nil
}

// Detect reports which features are currently applied to the submodule, by
// reverse-checking each feature patch against the working tree. A feature whose
// patch reverse-applies cleanly is considered present.
func (m *Manager) Detect(ctx context.Context) (map[string]bool, error) {
	if err := m.checkSubmodule(); err != nil {
		return nil, err
	}
	state := make(map[string]bool, len(Features))
	for _, f := range Features {
		p := filepath.Join(m.PatchesDir, f.Patch)
		state[f.Key] = m.gitApplyOK(ctx, "--reverse", "--check", p)
	}
	return state, nil
}

// Apply resets the submodule to pristine and applies the co-op base plus the
// selected feature patches, in order. selected maps feature key -> enabled.
// It returns the applied feature keys (in order) on success.
func (m *Manager) Apply(ctx context.Context, selected map[string]bool) ([]string, error) {
	if err := m.checkSubmodule(); err != nil {
		return nil, err
	}
	if err := m.reset(ctx); err != nil {
		return nil, fmt.Errorf("resetting submodule to pristine: %w", err)
	}

	base, err := m.basePatches()
	if err != nil {
		return nil, err
	}
	for _, p := range base {
		if err := m.apply(ctx, p); err != nil {
			return nil, fmt.Errorf("applying base patch %s: %w", filepath.Base(p), err)
		}
	}

	var applied []string
	for _, f := range Features {
		if !selected[f.Key] {
			continue
		}
		if err := m.apply(ctx, filepath.Join(m.PatchesDir, f.Patch)); err != nil {
			return applied, fmt.Errorf("applying %s: %w", f.Patch, err)
		}
		applied = append(applied, f.Key)
	}
	return applied, nil
}

// reset returns the submodule to the pinned commit: discard tracked changes and
// remove untracked files, matching the documented pristine-reset.
func (m *Manager) reset(ctx context.Context) error {
	if err := m.run(ctx, "checkout", "--", "."); err != nil {
		return err
	}
	return m.run(ctx, "clean", "-fd")
}

func (m *Manager) checkSubmodule() error {
	dot := filepath.Join(m.Submodule, ".git")
	if _, err := os.Stat(dot); err != nil {
		return fmt.Errorf("%s is not a git checkout; run: git submodule update --init", m.Submodule)
	}
	return nil
}

func (m *Manager) run(ctx context.Context, args ...string) error {
	full := append([]string{"-C", m.Submodule}, args...)
	cmd := exec.CommandContext(ctx, m.git(), full...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if s := strings.TrimSpace(string(out)); s != "" {
			return fmt.Errorf("%w: %s", err, s)
		}
		return err
	}
	return nil
}

func (m *Manager) gitApplyOK(ctx context.Context, args ...string) bool {
	full := append([]string{"-C", m.Submodule, "apply"}, args...)
	return exec.CommandContext(ctx, m.git(), full...).Run() == nil
}

func (m *Manager) apply(ctx context.Context, patch string) error {
	return m.run(ctx, "apply", patch)
}
