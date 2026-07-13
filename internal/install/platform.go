// Package install stages the co-op engine data directory (symlinks to the
// retail assets + the built gamecode) and installs the launcher commands,
// replacing the per-OS install-coop.sh / install-coop-macos.sh / install-coop.ps1
// scripts with one cross-platform implementation.
//
// It never copies or modifies retail files — it only creates symlinks into the
// existing game install and small launcher scripts. Uninstall removes exactly
// what it created (tracked in a manifest), and re-running is idempotent.
package install

import (
	"os"
	"path/filepath"
	"runtime"
)

// Platform holds the OS-specific names and locations the installer needs.
type Platform struct {
	// OS is the target GOOS ("linux", "darwin", "windows").
	OS string
	// Arch is the build architecture used in the gamecode/renderer names
	// (e.g. "x86_64", "arm64").
	Arch string
	// DataDir is the engine's fs_basepath (…/base lives under it).
	DataDir string
	// BinDir is where the launcher commands are installed.
	BinDir string
	// EngineName is the engine executable's basename within the build dir.
	EngineName string
	// GamecodeName is the gamecode module basename (.so/.dylib/.dll).
	GamecodeName string
	// RendererName is the renderer module basename beside the engine.
	RendererName string
	// LauncherExt is appended to launcher command names (".cmd" on Windows).
	LauncherExt string
}

// BaseDir is DataDir/base — the engine's search path root.
func (p Platform) BaseDir() string { return filepath.Join(p.DataDir, "base") }

// ManifestPath is the install manifest under DataDir.
func (p Platform) ManifestPath() string { return filepath.Join(p.DataDir, ".coop-install-manifest") }

// HostLauncher / JoinLauncher are the two generated launcher command paths.
func (p Platform) HostLauncher() string {
	return filepath.Join(p.BinDir, "jk2coop-host"+p.LauncherExt)
}

func (p Platform) JoinLauncher() string {
	return filepath.Join(p.BinDir, "jk2coop-join"+p.LauncherExt)
}

const (
	defaultPort = 29070
	defaultMap  = "kejim_post"
)

// DefaultPort is the co-op UDP port hosts listen on and joiners default to.
const DefaultPort = defaultPort

// DefaultMap is the campaign map launched when none is given.
const DefaultMap = defaultMap

// ResolveEngine returns the engine executable to run and the directory holding
// the renderer module (the loader's fs_apppath), given the CMake build dir. It
// is the same resolution the installer and launchers use, so `launch` runs
// exactly the binary `install` staged. Returns ("", "") if the engine is not
// built.
func (p Platform) ResolveEngine(buildDir string) (bin, dir string) {
	return resolveEngine(buildDir, p)
}

// arch resolves the build architecture from an override env var or the host.
func arch() string {
	if a := os.Getenv("JK2_ARCH"); a != "" {
		return a
	}
	switch runtime.GOARCH {
	case "arm64":
		return "arm64"
	default:
		return "x86_64"
	}
}

// homeDir returns the user home, falling back to os.UserHomeDir.
func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return os.Getenv("HOME")
}

// EnvOr returns the value of env key or fallback when unset/empty.
func EnvOr(key, fallback string) string {
	return envOr(key, fallback)
}

// envOr returns the value of env key or fallback when unset/empty.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
