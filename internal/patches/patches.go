// Package patches applies this repo's OpenJK patch set to the pinned submodule.
//
// The patches are CUMULATIVE and OVERLAP: several touch the same lines (for
// example 0004 sets MAX_CLIENTS to 2 and 0020 later changes that 2 to 4). They
// apply cleanly in order to a *pristine* submodule, but a single patch cannot be
// reliably reverse-checked against an already-fully-patched tree — its region
// may have been superseded by a later patch. So applying is NOT idempotent on a
// dirty tree: reset the submodule to pristine and re-run against a clean tree.
package patches

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Status is the outcome of applying (or checking) a single patch.
type Status int

const (
	// Applied means the patch was applied to the submodule.
	Applied Status = iota
	// Skipped means the patch was already applied (reverse-check passed).
	Skipped
)

func (s Status) String() string {
	switch s {
	case Applied:
		return "applied"
	case Skipped:
		return "skip"
	default:
		return "unknown"
	}
}

// Result reports what happened to one patch.
type Result struct {
	Name   string
	Status Status
}

// Applier applies patches from patchesDir to the git checkout at submodule.
type Applier struct {
	// Submodule is the OpenJK checkout the patches apply to.
	Submodule string
	// PatchesDir holds the *.patch files, applied in lexical order.
	PatchesDir string
	// Git is the git executable (default "git").
	Git string
}

// List returns the patch files in PatchesDir, sorted lexically (their numeric
// prefix makes lexical order the intended apply order).
func (a *Applier) List() ([]string, error) {
	glob := filepath.Join(a.PatchesDir, "*.patch")
	matches, err := filepath.Glob(glob)
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	return matches, nil
}

// Apply applies every patch in order. It returns the per-patch results in apply
// order. A patch that neither applies nor reverse-applies cleanly aborts with a
// descriptive error (the usual cause is a dirty/partially-patched submodule).
func (a *Applier) Apply(ctx context.Context) ([]Result, error) {
	git := a.Git
	if git == "" {
		git = "git"
	}
	if err := a.checkSubmodule(); err != nil {
		return nil, err
	}

	patches, err := a.List()
	if err != nil {
		return nil, err
	}
	if len(patches) == 0 {
		return nil, nil
	}

	var results []Result
	for _, p := range patches {
		name := filepath.Base(p)
		switch {
		case a.gitApplyOK(ctx, git, "--reverse", "--check", p):
			results = append(results, Result{Name: name, Status: Skipped})
		case a.gitApplyOK(ctx, git, "--check", p):
			if err := a.gitApply(ctx, git, p); err != nil {
				return results, fmt.Errorf("applying %s: %w", name, err)
			}
			results = append(results, Result{Name: name, Status: Applied})
		default:
			return results, a.applyError(name)
		}
	}
	return results, nil
}

func (a *Applier) checkSubmodule() error {
	dot := filepath.Join(a.Submodule, ".git")
	if _, err := os.Stat(dot); err != nil {
		return fmt.Errorf("%s is not a git checkout; run: git submodule update --init", a.Submodule)
	}
	return nil
}

// gitApplyOK runs `git -C <sub> apply <args...>` and reports success. Used for
// the --check probes, whose non-zero exit is an expected, non-error signal.
func (a *Applier) gitApplyOK(ctx context.Context, git string, args ...string) bool {
	full := append([]string{"-C", a.Submodule, "apply"}, args...)
	cmd := exec.CommandContext(ctx, git, full...)
	return cmd.Run() == nil
}

func (a *Applier) gitApply(ctx context.Context, git, patch string) error {
	cmd := exec.CommandContext(ctx, git, "-C", a.Submodule, "apply", patch)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if s := strings.TrimSpace(string(out)); s != "" {
			return fmt.Errorf("%w: %s", err, s)
		}
		return err
	}
	return nil
}

func (a *Applier) applyError(name string) error {
	//nolint:staticcheck // ST1005: this is a multi-line, user-facing help message, not a wrapped error.
	return fmt.Errorf(`%s does not apply cleanly to the pinned submodule.
  The most common cause is a partially/fully patched submodule (these
  patches overlap, so a re-run on a dirty tree can trip here). Reset the
  submodule to pristine and run this again:
      git -C %q checkout -- . && git -C %q clean -fd
      jk2coop patches apply
  If it still fails on a clean submodule, the pinned commit or the patch
  has drifted and the patch needs regenerating.`, name, a.Submodule, a.Submodule)
}
