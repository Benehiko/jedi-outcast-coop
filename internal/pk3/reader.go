package pk3

import (
	"archive/zip"
	"fmt"
	"io"
	"strings"
)

// Reader opens a pak for reading its members.
type Reader struct {
	zr *zip.ReadCloser
}

// Open opens the pak at path for reading. Close it when done.
func Open(path string) (*Reader, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	return &Reader{zr: zr}, nil
}

// Close releases the underlying archive.
func (r *Reader) Close() error { return r.zr.Close() }

// Names returns every member path in the pak, in archive order.
func (r *Reader) Names() []string {
	names := make([]string, 0, len(r.zr.File))
	for _, f := range r.zr.File {
		names = append(names, f.Name)
	}
	return names
}

// ReadFile returns the bytes of the named member. The lookup is
// case-insensitive on the full path: the engine stores names lowercased
// (e.g. ext_data/npcs.cfg) while callers often ask by canonical case.
func (r *Reader) ReadFile(name string) ([]byte, error) {
	f := r.find(name)
	if f == nil {
		return nil, fmt.Errorf("pk3: entry not found: %s", name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	return io.ReadAll(rc)
}

// Has reports whether a member exists (case-insensitive on the full path).
func (r *Reader) Has(name string) bool { return r.find(name) != nil }

func (r *Reader) find(name string) *zip.File {
	// Exact match first, then case-insensitive.
	for _, f := range r.zr.File {
		if f.Name == name {
			return f
		}
	}
	lname := strings.ToLower(name)
	for _, f := range r.zr.File {
		if strings.ToLower(f.Name) == lname {
			return f
		}
	}
	return nil
}
