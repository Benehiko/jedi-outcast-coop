package gfx

import (
	"context"
	"fmt"

	"github.com/Benehiko/jedi-outcast-coop/internal/embed"
	"github.com/Benehiko/jedi-outcast-coop/internal/patchengine"
	"github.com/Benehiko/jedi-outcast-coop/internal/workdir"
)

// EmbedManager applies a graphics selection to the work-dir source tree using
// the binary's embedded OpenJK source and patch set, with no git and no repo.
// It is the standalone-binary counterpart to Manager (which drives the git
// submodule for the dev flow). Both apply the SAME patch files in the SAME order
// and share Features/basePatches semantics; they differ only in where the source
// comes from and how "reset to pristine" is achieved:
//
//   - Manager:      git checkout/clean on the submodule; git apply.
//   - EmbedManager: re-extract the embedded tarball;      pure-Go patch apply.
//
// Detection is manifest-based (EmbedManager owns the tree state end to end)
// rather than the reverse-check probing Manager must do on a shared checkout.
type EmbedManager struct {
	// Dir is the resolved work directory.
	Dir workdir.Dir
}

// Detect reports which features the extracted tree currently reflects, read from
// the work-dir manifest. An absent/mismatched tree reports no features.
func (m *EmbedManager) Detect(context.Context) (map[string]bool, error) {
	man, err := m.Dir.ReadManifest()
	if err != nil {
		return nil, err
	}
	state := make(map[string]bool, len(Features))
	set := map[string]bool{}
	for _, k := range man.Gfx {
		set[k] = true
	}
	// Only report a feature as present when the tree is actually at the embedded
	// pin; a stale/absent tree means nothing is reliably built.
	pinOK := man.Pin == embed.Pin()
	for _, f := range Features {
		state[f.Key] = pinOK && set[f.Key]
	}
	return state, nil
}

// Apply re-extracts the embedded source to a pristine tree and applies the co-op
// base plus the selected feature patches, in order, in pure Go. On success it
// records the selection (and the embedded pin) in the work-dir manifest and
// returns the applied feature keys in order.
func (m *EmbedManager) Apply(_ context.Context, selected map[string]bool) ([]string, error) {
	if err := m.Dir.ExtractPristine(); err != nil {
		return nil, fmt.Errorf("extracting pristine source: %w", err)
	}

	patches, err := embed.Patches()
	if err != nil {
		return nil, err
	}
	feat := map[string]string{} // patch filename -> feature key
	for _, f := range Features {
		feat[f.Patch] = f.Key
	}

	var applied []string
	src := m.Dir.Src()
	for _, p := range patches {
		key, isFeature := feat[p.Name]
		if isFeature && !selected[key] {
			continue // a graphics feature that is not selected: skip its patch
		}
		if err := patchengine.Apply(src, p.Data); err != nil {
			return applied, fmt.Errorf("applying %s: %w", p.Name, err)
		}
		if isFeature {
			applied = append(applied, key)
		}
	}

	// Record the new state so Detect/NeedsRebuild can short-circuit next time.
	if err := m.Dir.WriteManifest(workdir.Manifest{Pin: embed.Pin(), Gfx: applied}); err != nil {
		return applied, fmt.Errorf("writing manifest: %w", err)
	}
	return applied, nil
}

// EnsureApplied re-extracts and re-patches only when the tree does not already
// reflect the wanted selection at the embedded pin; otherwise it is a no-op. It
// returns whether it rebuilt the tree.
func (m *EmbedManager) EnsureApplied(ctx context.Context, selected map[string]bool) (bool, error) {
	want := selectedKeys(selected)
	need, err := m.Dir.NeedsRebuild(want)
	if err != nil {
		return false, err
	}
	if !need {
		return false, nil
	}
	if _, err := m.Apply(ctx, selected); err != nil {
		return false, err
	}
	return true, nil
}

// selectedKeys returns the enabled feature keys (in Features order) for selected.
func selectedKeys(selected map[string]bool) []string {
	var out []string
	for _, f := range Features {
		if selected[f.Key] {
			out = append(out, f.Key)
		}
	}
	return out
}
