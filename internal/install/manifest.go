package install

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Manifest tracks the paths an install created so uninstall can remove exactly
// those. It is an append-only list of absolute paths, de-duplicated, persisted
// to a file under the data dir.
type Manifest struct {
	path    string
	entries []string
	seen    map[string]struct{}
}

// LoadManifest reads an existing manifest (or returns an empty one if absent).
func LoadManifest(path string) (*Manifest, error) {
	m := &Manifest{path: path, seen: map[string]struct{}{}}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return m, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		m.append(line)
	}
	return m, sc.Err()
}

func (m *Manifest) append(p string) {
	if _, ok := m.seen[p]; ok {
		return
	}
	m.seen[p] = struct{}{}
	m.entries = append(m.entries, p)
}

// Add records a created path (idempotent) and appends it to the manifest file.
func (m *Manifest) Add(p string) error {
	if _, ok := m.seen[p]; ok {
		return nil
	}
	m.append(p)
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(m.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(p + "\n"); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// Entries returns the tracked paths in insertion order.
func (m *Manifest) Entries() []string { return m.entries }

// Forget drops a path from the manifest and rewrites the file. Used when an
// install removes a previously-tracked artifact (e.g. an override pak the user
// has since disabled) so a later uninstall does not try to remove it again.
func (m *Manifest) Forget(p string) {
	if _, ok := m.seen[p]; !ok {
		return
	}
	delete(m.seen, p)
	kept := m.entries[:0]
	for _, e := range m.entries {
		if e != p {
			kept = append(kept, e)
		}
	}
	m.entries = kept
	m.rewrite()
}

// rewrite persists the current entries, replacing the file. Best-effort: a
// failure leaves the previous file (with the now-removed line) in place, which
// only means a stale entry, not data loss.
func (m *Manifest) rewrite() {
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return
	}
	var b strings.Builder
	for _, e := range m.entries {
		b.WriteString(e + "\n")
	}
	_ = os.WriteFile(m.path, []byte(b.String()), 0o644)
}

// Exists reports whether the manifest file is present on disk.
func (m *Manifest) Exists() bool { return fileExists(m.path) }

// removeReport is one line of uninstall output.
type removeReport struct {
	Path    string
	Kind    string // "file", "dir", "manifest"
	Removed bool
}

// Uninstall removes everything the manifest tracks: files/symlinks first, then
// the manifest itself, then any tracked directories that are now empty
// (deepest-first). Directories that still hold files the installer did not
// create are left in place. It returns a report per removal for the caller to
// print.
func (m *Manifest) Uninstall() []removeReport {
	var reports []removeReport
	var dirs []string

	for _, p := range m.entries {
		switch {
		case isSymlink(p) || fileExists(p):
			removed := os.Remove(p) == nil
			reports = append(reports, removeReport{Path: p, Kind: "file", Removed: removed})
		case dirExists(p):
			dirs = append(dirs, p)
		}
	}

	// Remove the manifest before rmdir-ing the data dir that holds it.
	manifestRemoved := os.Remove(m.path) == nil
	reports = append(reports, removeReport{Path: m.path, Kind: "manifest", Removed: manifestRemoved})

	// rmdir tracked dirs that are now empty, deepest first (by separator count).
	sort.Slice(dirs, func(i, j int) bool {
		return strings.Count(dirs[i], string(os.PathSeparator)) > strings.Count(dirs[j], string(os.PathSeparator))
	})
	for _, d := range dirs {
		// os.Remove on a directory only succeeds if it is empty — exactly the
		// desired "never force-remove" behavior.
		if os.Remove(d) == nil {
			reports = append(reports, removeReport{Path: d, Kind: "dir", Removed: true})
		}
	}
	return reports
}

func isSymlink(p string) bool {
	fi, err := os.Lstat(p)
	return err == nil && fi.Mode()&os.ModeSymlink != 0
}
