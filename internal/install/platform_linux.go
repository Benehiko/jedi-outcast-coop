//go:build linux

package install

import (
	"path/filepath"
	"runtime"
)

// DetectPlatform returns the Linux install layout.
//
// buildDir is the OpenJK CMake build directory (openjk/build). The renderer
// lives beside the engine binary there and is loaded relative to it, so the
// launchers run the engine from the build dir.
func DetectPlatform(buildDir string) Platform {
	home := homeDir()
	a := arch()
	return Platform{
		OS:           runtime.GOOS,
		Arch:         a,
		DataDir:      envOr("JK2_DATA_DIR", filepath.Join(home, ".local", "share", "openjo")),
		BinDir:       envOr("JK2_BIN_DIR", filepath.Join(home, ".local", "bin")),
		EngineName:   "openjo_sp." + a,
		GamecodeName: "jospgame" + a + ".so",
		RendererName: "rdjosp-vanilla_" + a + ".so",
	}
}

// steamRoots are the standard Steam library roots to probe for GameData.
func steamRoots() []string {
	home := homeDir()
	return []string{
		filepath.Join(home, ".steam", "steam"),
		filepath.Join(home, ".local", "share", "Steam"),
		filepath.Join(home, ".steam", "root"),
	}
}

// DefaultAssetsBase is the default retail base/ dir for the pak tools.
func DefaultAssetsBase() string {
	return filepath.Join(homeDir(), ".local", "share", "openjo", "base")
}

// resolveEngine returns the engine executable and the dir holding the renderer
// (the build dir). On Linux the engine is always a plain arch-suffixed binary.
func resolveEngine(buildDir string, p Platform) (bin, dir string) {
	return filepath.Join(buildDir, p.EngineName), buildDir
}
