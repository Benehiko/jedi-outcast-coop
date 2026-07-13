package gfx

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// markerLines is the pristine test file: unique markers separated by filler so
// each feature's one-line hunk has non-overlapping context.
var markerLines = []string{
	"a", "b", "c",
	"BASE", // co-op base patch target
	"g", "h", "i",
	"WIDE_HERE",
	"j", "k", "l",
	"FID_HERE",
	"m", "n", "o",
}

// initRepo makes a throwaway git repo standing in for the OpenJK submodule, with
// a single tracked file, and returns its path plus a patches dir. The patches
// add distinct marker lines so each "feature" is independently detectable.
func initRepo(t *testing.T) (sub, patches string) {
	t.Helper()
	sub = t.TempDir()
	patches = t.TempDir()

	git := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", sub}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	// Pristine file with markers spaced far apart (3+ filler lines between each)
	// so no two feature hunks share context lines — mirrors the real patches,
	// which touch disjoint regions and thus reverse-check independently.
	body := strings.Join(markerLines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(sub, "f.txt"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-qm", "pristine")

	// A base (co-op) patch replacing BASE, plus one patch per feature.
	write := func(name, old, new string) {
		p := makeLinePatch(old, new)
		if err := os.WriteFile(filepath.Join(patches, name), []byte(p), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("0001-base.patch", "BASE", "BASE_APPLIED")
	write(Features[0].Patch, "WIDE_HERE", "WIDE_APPLIED")
	write(Features[1].Patch, "FID_HERE", "FID_APPLIED")
	return sub, patches
}

// makeLinePatch builds a git-apply-able unified diff that replaces the unique
// marker line old with new, with one line of context on each side.
func makeLinePatch(old, new string) string {
	lines := markerLines
	idx := -1
	for i, l := range lines {
		if l == old {
			idx = i
			break
		}
	}
	if idx < 0 {
		panic("marker not found: " + old)
	}
	start := idx
	if start > 0 {
		start--
	}
	end := idx + 1
	if end < len(lines)-1 {
		end++
	}
	var hunk strings.Builder
	for i := start; i <= end; i++ {
		if i == idx {
			fmt.Fprintf(&hunk, "-%s\n+%s\n", lines[i], new)
		} else {
			fmt.Fprintf(&hunk, " %s\n", lines[i])
		}
	}
	count := end - start + 1
	return fmt.Sprintf("--- a/f.txt\n+++ b/f.txt\n@@ -%d,%d +%d,%d @@\n%s",
		start+1, count, start+1, count, hunk.String())
}

func TestApplyAndDetectSubsets(t *testing.T) {
	sub, patches := initRepo(t)
	m := &Manager{Submodule: sub, PatchesDir: patches}
	ctx := context.Background()

	cases := []map[string]bool{
		{},
		{"widescreen": true},
		{"render-fidelity": true},
		{"widescreen": true, "render-fidelity": true},
	}
	for _, want := range cases {
		if _, err := m.Apply(ctx, want); err != nil {
			t.Fatalf("Apply(%v): %v", want, err)
		}
		got, err := m.Detect(ctx)
		if err != nil {
			t.Fatalf("Detect: %v", err)
		}
		for _, f := range Features {
			if got[f.Key] != want[f.Key] {
				t.Errorf("selection %v: feature %s detected=%v want=%v", want, f.Key, got[f.Key], want[f.Key])
			}
		}
		// base must always be applied
		body, _ := os.ReadFile(filepath.Join(sub, "f.txt"))
		if !strings.Contains(string(body), "BASE_APPLIED") {
			t.Errorf("selection %v: base patch not applied", want)
		}
	}
}

func TestSummaryLine(t *testing.T) {
	tests := []struct {
		sel  map[string]bool
		want string
	}{
		{map[string]bool{}, "all graphics features off"},
		{map[string]bool{"widescreen": true, "render-fidelity": true}, "render-fidelity, widescreen (all on)"},
		{map[string]bool{"widescreen": true}, "widescreen (render-fidelity off)"},
	}
	for _, tc := range tests {
		if got := SummaryLine(tc.sel); got != tc.want {
			t.Errorf("SummaryLine(%v) = %q, want %q", tc.sel, got, tc.want)
		}
	}
}
