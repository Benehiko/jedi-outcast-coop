// Package pk3 reads and writes Quake-3 .pk3 archives (plain ZIP files) used by
// the Jedi Outcast engine, replacing the zip/unzip shell-outs in the original
// tools. Entries are written in a deterministic, sorted order so a rebuild from
// the same inputs produces a byte-identical pak.
package pk3

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Entry is a single file destined for a pak: its archive-relative path (always
// forward-slash separated, as the engine expects) and its on-disk source.
type Entry struct {
	// ArchivePath is the slash-separated path inside the pak (e.g. "ui/coop.menu").
	ArchivePath string
	// SourcePath is the file on disk to read the bytes from.
	SourcePath string
}

// Builder accumulates entries and writes them to a pak.
type Builder struct {
	entries map[string]string // archivePath -> sourcePath (last write wins)
}

// NewBuilder returns an empty pak builder.
func NewBuilder() *Builder {
	return &Builder{entries: make(map[string]string)}
}

// Add registers a single file. A later Add for the same archive path replaces
// the earlier one (matching how overlapping trees resolve).
func (b *Builder) Add(archivePath, sourcePath string) {
	b.entries[normalizeArchivePath(archivePath)] = sourcePath
}

// AddTree walks srcDir and adds every regular file under it, mapping each to an
// archive path of prefix + its path relative to srcDir. Dotfiles and any file
// whose name matches one of skipExts (case-insensitive, leading dot required,
// e.g. ".pk3") are skipped, mirroring the shell builders' `-x` excludes.
func (b *Builder) AddTree(srcDir, prefix string, skipExts ...string) error {
	skip := make(map[string]struct{}, len(skipExts))
	for _, e := range skipExts {
		skip[strings.ToLower(e)] = struct{}{}
	}
	return filepath.WalkDir(srcDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Skip dot-directories entirely.
			if path != srcDir && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		if _, bad := skip[strings.ToLower(filepath.Ext(d.Name()))]; bad {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		archivePath := normalizeArchivePath(rel)
		if prefix != "" {
			archivePath = normalizeArchivePath(prefix) + "/" + archivePath
		}
		b.entries[archivePath] = path
		return nil
	})
}

// Len reports how many entries will be written.
func (b *Builder) Len() int { return len(b.entries) }

// ArchivePaths returns the sorted archive paths that will be written.
func (b *Builder) ArchivePaths() []string {
	paths := make([]string, 0, len(b.entries))
	for p := range b.entries {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

// Write creates (truncating any existing file) the pak at outPath. Parent
// directories are created as needed. Entries are written sorted by archive path
// for deterministic output.
func (b *Builder) Write(outPath string) error {
	if len(b.entries) == 0 {
		return errors.New("pk3: nothing to write (no entries added)")
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	zw := zip.NewWriter(f)
	for _, archivePath := range b.ArchivePaths() {
		if err := writeEntry(zw, archivePath, b.entries[archivePath]); err != nil {
			_ = zw.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return f.Close()
}

func writeEntry(zw *zip.Writer, archivePath, sourcePath string) error {
	src, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	w, err := zw.CreateHeader(&zip.FileHeader{
		Name:   archivePath,
		Method: zip.Deflate,
	})
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, src); err != nil {
		return fmt.Errorf("pk3: copy %s: %w", sourcePath, err)
	}
	return nil
}

// normalizeArchivePath converts OS separators to forward slashes and trims any
// leading "./" or "/" so archive paths are clean and engine-resolvable.
func normalizeArchivePath(p string) string {
	p = filepath.ToSlash(p)
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, "/")
	return p
}
