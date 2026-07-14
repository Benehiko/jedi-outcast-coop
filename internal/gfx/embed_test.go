package gfx_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Benehiko/jedi-outcast-coop/internal/gfx"
	"github.com/Benehiko/jedi-outcast-coop/internal/workdir"
)

// newWorkdir returns an EmbedManager rooted in a temp dir, isolated via
// JK2COOP_HOME so it never touches the real cache.
func newWorkdir(t *testing.T) (*gfx.EmbedManager, workdir.Dir) {
	t.Helper()
	t.Setenv(workdir.EnvVar, t.TempDir())
	wd, err := workdir.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	return &gfx.EmbedManager{Dir: wd}, wd
}

func TestEmbedManagerApplyBase(t *testing.T) {
	mgr, wd := newWorkdir(t)
	ctx := context.Background()

	// No features selected: base patches only.
	applied, err := mgr.Apply(ctx, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 0 {
		t.Errorf("expected no feature keys, got %v", applied)
	}
	// The base patch set creates net_ip.cpp (0005) — proves patches ran.
	if _, err := os.Stat(filepath.Join(wd.Src(), "code", "qcommon", "net_ip.cpp")); err != nil {
		t.Errorf("base patches did not run: %v", err)
	}
	// Manifest records pin + empty selection.
	man, err := wd.ReadManifest()
	if err != nil {
		t.Fatal(err)
	}
	if man.Pin == "" || len(man.Gfx) != 0 {
		t.Errorf("unexpected manifest: %+v", man)
	}
}

func TestEmbedManagerApplyFeatures(t *testing.T) {
	mgr, wd := newWorkdir(t)
	ctx := context.Background()

	sel := map[string]bool{"widescreen": true, "render-fidelity": true}
	applied, err := mgr.Apply(ctx, sel)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 2 {
		t.Errorf("expected 2 features applied, got %v", applied)
	}

	// Detect should now report both features present.
	got, err := mgr.Detect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"widescreen", "render-fidelity"} {
		if !got[k] {
			t.Errorf("Detect: expected %s present", k)
		}
	}

	man, err := wd.ReadManifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(man.Gfx) != 2 {
		t.Errorf("manifest gfx = %v, want 2 features", man.Gfx)
	}
}

func TestEmbedManagerEnsureAppliedIdempotent(t *testing.T) {
	mgr, _ := newWorkdir(t)
	ctx := context.Background()
	sel := map[string]bool{"widescreen": true}

	// First call: must rebuild.
	changed, err := mgr.EnsureApplied(ctx, sel)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("first EnsureApplied should have rebuilt")
	}
	// Second call with the same selection: no-op.
	changed, err = mgr.EnsureApplied(ctx, sel)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("second EnsureApplied with same selection should be a no-op")
	}
	// Changing the selection: rebuild again.
	changed, err = mgr.EnsureApplied(ctx, map[string]bool{"render-fidelity": true})
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("EnsureApplied with a changed selection should rebuild")
	}
}

func TestEmbedManagerReselectDropsFeature(t *testing.T) {
	mgr, wd := newWorkdir(t)
	ctx := context.Background()

	// Apply with widescreen, then re-apply with none: widescreen patch must be
	// gone (proves the pristine re-extract, not an additive apply).
	if _, err := mgr.Apply(ctx, map[string]bool{"widescreen": true}); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Apply(ctx, map[string]bool{}); err != nil {
		t.Fatal(err)
	}
	got, err := mgr.Detect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got["widescreen"] {
		t.Error("widescreen should be absent after re-applying with no features")
	}
	// A base-only file still exists (tree is patched, just without the feature).
	if _, err := os.Stat(filepath.Join(wd.Src(), "code", "qcommon", "net_ip.cpp")); err != nil {
		t.Errorf("base tree missing after reselect: %v", err)
	}
}
