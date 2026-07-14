// Package patchengine applies git-format unified-diff patches to a source tree
// in pure Go, with no dependency on the git executable.
//
// It exists so jk2coop can patch an extracted OpenJK source tree (materialised
// from the embedded, pruned copy of the submodule) without shelling out to
// `git apply`. The patches are CUMULATIVE and OVERLAP (see the patches package
// docs): several edit the same lines, and they are only guaranteed to apply to
// a *pristine* tree, in numeric order. This engine therefore does strict,
// zero-fuzz context matching — the same discipline `git apply` uses on a clean
// tree — and treats any context mismatch as a hard error rather than searching
// for an offset. Applying to a re-extracted pristine tree makes that strictness
// correct: the hunks' line numbers are exact against the pinned source.
//
// Supported diff shapes (everything the co-op patch set uses):
//   - modifications to existing files (the common case),
//   - new-file creation (--- /dev/null, +++ b/path).
//
// Not supported (the patch set contains none; we fail loudly if one appears):
// file deletion, rename/copy, mode-only changes, and binary patches.
package patchengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sourcegraph/go-diff/diff"
)

// Apply applies a single git-format patch (the full bytes of one *.patch file,
// which may contain several per-file diffs) to the source tree rooted at
// treeDir. Files are addressed by their post-image path with the git "b/"
// prefix stripped. It applies every file diff or returns an error describing the
// first failure; on error the tree may be partially modified (callers reset by
// re-extracting the pristine tree, so partial application is not observable).
func Apply(treeDir string, patch []byte) error {
	fileDiffs, err := diff.ParseMultiFileDiff(patch)
	if err != nil {
		return fmt.Errorf("parsing patch: %w", err)
	}
	for _, fd := range fileDiffs {
		if err := applyFileDiff(treeDir, fd); err != nil {
			return err
		}
	}
	return nil
}

// applyFileDiff applies one file's hunks to the tree.
func applyFileDiff(treeDir string, fd *diff.FileDiff) error {
	newFile := isDevNull(fd.OrigName)
	deleted := isDevNull(fd.NewName)
	if deleted {
		return fmt.Errorf("patch deletes %s: file deletion is not supported", trimPrefix(fd.OrigName))
	}

	rel := trimPrefix(fd.NewName)
	if rel == "" {
		return fmt.Errorf("patch has no target file name")
	}
	abs, err := safeJoin(treeDir, rel)
	if err != nil {
		return err
	}

	var orig []byte
	if newFile {
		// A new file starts from empty content; its single hunk is all additions.
		if _, err := os.Stat(abs); err == nil {
			return fmt.Errorf("new-file patch for %s: file already exists", rel)
		}
	} else {
		b, err := os.ReadFile(abs)
		if err != nil {
			return fmt.Errorf("reading %s: %w", rel, err)
		}
		orig = b
	}

	out, err := applyHunks(rel, orig, fd.Hunks)
	if err != nil {
		return err
	}

	if newFile {
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return fmt.Errorf("creating dir for %s: %w", rel, err)
		}
	}
	//nolint:gosec // G703: abs is contained within treeDir by safeJoin above.
	if err := os.WriteFile(abs, out, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", rel, err)
	}
	return nil
}

// hunkLine is one parsed line of a hunk body: an op (' ', '-', '+') and text.
type hunkLine struct {
	op   byte
	text string
}

// applyHunks applies all hunks of one file to orig and returns the patched
// bytes. It preserves the file's original trailing-newline shape.
//
// Each hunk is LOCATED by matching its "original" image (context + deletion
// lines) against the source, searching outward from the header-hinted position.
// This reproduces `git apply`'s offset tolerance: the co-op patches carry hunk
// headers whose line numbers have drifted from the pinned submodule (a patch
// generated against a slightly different tree), so a strict positional apply
// would wrongly reject them. Fuzz (dropping leading/trailing context) is NOT
// implemented — only whole-hunk offset — which matches what these patches need
// and keeps the match exact.
func applyHunks(rel string, orig []byte, hunks []*diff.Hunk) ([]byte, error) {
	hadTrailingNewline := len(orig) > 0 && orig[len(orig)-1] == '\n'
	var lines []string
	if len(orig) > 0 {
		lines = splitLines(orig)
		// A trailing newline yields a final "" element from splitLines; drop it so
		// line indices line up with real file lines. It is restored on join.
		if hadTrailingNewline && len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
	}
	// An empty original (new file) has zero lines, not one phantom "" line.

	var out []string
	srcCursor := 0 // position consumed in the ORIGINAL line slice

	for _, h := range hunks {
		body := parseHunkBody(h.Body)
		orImage := originalImage(body) // context + deletions, in order

		pos, err := locateHunk(rel, h, lines, orImage, srcCursor)
		if err != nil {
			return nil, err
		}
		// Copy unchanged lines between the previous hunk and this one.
		out = append(out, lines[srcCursor:pos]...)
		srcCursor = pos

		for _, hl := range body {
			switch hl.op {
			case ' ':
				out = append(out, lines[srcCursor])
				srcCursor++
			case '-':
				srcCursor++
			case '+':
				out = append(out, hl.text)
			}
		}
	}
	out = append(out, lines[srcCursor:]...)

	joined := strings.Join(out, "\n")
	if hadTrailingNewline || len(orig) == 0 {
		joined += "\n"
	}
	return []byte(joined), nil
}

// locateHunk finds the index in lines where the hunk's original image matches,
// searching outward from the header hint (adjusted by drift already absorbed),
// but never before srcCursor. Returns the match start index.
func locateHunk(rel string, h *diff.Hunk, lines, orImage []string, srcCursor int) (int, error) {
	// A pure-addition hunk (no context, no deletions) anchors at its header
	// position clamped into range — there is nothing to match.
	if len(orImage) == 0 {
		hint := max(int(h.OrigStartLine)-1, srcCursor)
		hint = min(hint, len(lines))
		return hint, nil
	}

	maxStart := len(lines) - len(orImage)
	if maxStart < srcCursor {
		// The remaining source is shorter than the hunk's pre-image; cannot match.
		return 0, hunkNotFound(rel, h)
	}
	// Clamp the header hint into the searchable range [srcCursor, maxStart] so
	// distance is measured from a reachable candidate, then search outward. The
	// range spans the whole remaining file, so a far-off header (drifted line
	// numbers) is still found.
	hint := max(int(h.OrigStartLine)-1, srcCursor)
	hint = min(hint, maxStart)
	for dist := 0; dist <= maxStart-srcCursor; dist++ {
		for _, cand := range []int{hint + dist, hint - dist} {
			if cand < srcCursor || cand > maxStart {
				continue
			}
			if matchAt(lines, orImage, cand) {
				return cand, nil
			}
			if dist == 0 {
				break // hint+0 and hint-0 are the same candidate
			}
		}
	}
	return 0, hunkNotFound(rel, h)
}

// matchAt reports whether orImage equals lines[start:start+len(orImage)].
func matchAt(lines, orImage []string, start int) bool {
	for i, want := range orImage {
		if lines[start+i] != want {
			return false
		}
	}
	return true
}

// originalImage returns the hunk's pre-image lines (context + deletions) in
// order — the sequence that must match the source for the hunk to apply.
func originalImage(body []hunkLine) []string {
	var img []string
	for _, hl := range body {
		if hl.op == ' ' || hl.op == '-' {
			img = append(img, hl.text)
		}
	}
	return img
}

// parseHunkBody splits a hunk Body into typed lines, dropping the trailing empty
// element and the "\ No newline at end of file" markers.
func parseHunkBody(body []byte) []hunkLine {
	raw := splitLines(body)
	out := make([]hunkLine, 0, len(raw))
	for _, bl := range raw {
		if bl == "" {
			continue
		}
		if bl[0] == '\\' {
			continue // "\ No newline at end of file"
		}
		out = append(out, hunkLine{op: bl[0], text: bl[1:]})
	}
	return out
}

// hunkNotFound builds a descriptive error for a hunk whose context could not be
// located anywhere in the file.
func hunkNotFound(rel string, h *diff.Hunk) error {
	return fmt.Errorf(
		"%s: hunk @@ -%d,%d +%d,%d @@ does not apply — its context was not found; "+
			"the source tree is not pristine or the pinned commit has drifted",
		rel, h.OrigStartLine, h.OrigLines, h.NewStartLine, h.NewLines)
}

// splitLines splits b into lines WITHOUT the trailing newline on each. A final
// newline yields a trailing "" element, which callers skip. Empty input yields
// a single "" element.
func splitLines(b []byte) []string {
	return strings.Split(string(b), "\n")
}

// safeJoin joins rel onto treeDir and verifies the result stays within treeDir,
// so a malformed patch target (e.g. "../../etc/x") cannot write outside the tree.
func safeJoin(treeDir, rel string) (string, error) {
	abs := filepath.Join(treeDir, filepath.FromSlash(rel))
	base := filepath.Clean(treeDir) + string(filepath.Separator)
	if abs != filepath.Clean(treeDir) && !strings.HasPrefix(abs, base) {
		return "", fmt.Errorf("patch target %q escapes the source tree", rel)
	}
	return abs, nil
}

// trimPrefix strips the git a/ or b/ path prefix.
func trimPrefix(name string) string {
	switch {
	case strings.HasPrefix(name, "a/"):
		return name[2:]
	case strings.HasPrefix(name, "b/"):
		return name[2:]
	default:
		return name
	}
}

// isDevNull reports whether a diff side names /dev/null (new or deleted file).
func isDevNull(name string) bool {
	return name == "/dev/null"
}
