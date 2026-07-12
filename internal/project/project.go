// Package project locates the repository root so subcommands can resolve the
// submodule, patches/, assets/, and tools/ regardless of the working directory.
package project

import (
	"errors"
	"os"
	"path/filepath"
)

// markers are files/dirs that only exist at the repo root.
var markers = []string{"patches", "openjk", "go.mod"}

// Root walks up from start (or the working directory when start is empty) until
// it finds a directory containing the repo markers, and returns that directory.
func Root(start string) (string, error) {
	if start == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		start = wd
	}
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if isRoot(dir) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("could not locate repository root (no patches/, openjk/, go.mod found in any parent) — run from inside the repo or pass --repo")
		}
		dir = parent
	}
}

func isRoot(dir string) bool {
	found := 0
	for _, m := range markers {
		if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
			found++
		}
	}
	// Require at least two markers so a stray go.mod elsewhere does not match.
	return found >= 2
}
