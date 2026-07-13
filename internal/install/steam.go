package install

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
)

// libraryPathRe extracts the "path" "…" values from a libraryfolders.vdf. It
// handles both the modern nested and the legacy flat formats.
var libraryPathRe = regexp.MustCompile(`"path"\s*"([^"]+)"`)

// ErrGameDataNotFound is returned when no Steam library yields a GameData dir
// with base/assets0.pk3.
var ErrGameDataNotFound = errors.New("could not find GameData under any Steam library")

// DetectGameData returns the first GameData dir that contains base/assets0.pk3
// across the platform's Steam roots (and any extra libraries their
// libraryfolders.vdf lists), or ErrGameDataNotFound.
func DetectGameData() (string, error) {
	libs := steamLibraries(steamRoots())
	for _, lib := range libs {
		gd := filepath.Join(lib, "steamapps", "common", "Jedi Outcast", "GameData")
		if fileExists(filepath.Join(gd, "base", "assets0.pk3")) {
			return gd, nil
		}
	}
	return "", ErrGameDataNotFound
}

// steamLibraries expands the given roots into the full set of Steam library
// directories: the roots that have a steamapps/ plus any extra libraries listed
// in their libraryfolders.vdf.
func steamLibraries(roots []string) []string {
	var libs []string
	seen := map[string]struct{}{}
	add := func(p string) {
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		if dirExists(filepath.Join(p, "steamapps")) {
			seen[p] = struct{}{}
			libs = append(libs, p)
		}
	}
	for _, r := range roots {
		add(r)
		vdf := filepath.Join(r, "steamapps", "libraryfolders.vdf")
		for _, p := range parseLibraryFolders(vdf) {
			add(p)
		}
	}
	return libs
}

// parseLibraryFolders returns the library paths referenced in a
// libraryfolders.vdf, or nil if it cannot be read.
func parseLibraryFolders(vdf string) []string {
	data, err := os.ReadFile(vdf)
	if err != nil {
		return nil
	}
	var out []string
	for _, m := range libraryPathRe.FindAllStringSubmatch(string(data), -1) {
		// VDF escapes backslashes; unescape the common \\ case for Windows paths.
		out = append(out, unescapeVDF(m[1]))
	}
	return out
}

func unescapeVDF(s string) string {
	// VDF strings escape backslash and quote. Paths in the wild use \\ for a
	// single separator; collapse it. Quotes never appear inside a path value.
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			out = append(out, s[i])
			continue
		}
		out = append(out, s[i])
	}
	return string(out)
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
