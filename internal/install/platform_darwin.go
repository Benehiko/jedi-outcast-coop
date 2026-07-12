//go:build darwin

package install

import (
	"os"
	"path/filepath"
	"runtime"
)

// DetectPlatform returns the macOS install layout.
//
// The engine may be an .app bundle (openjo_sp.app) or a plain arch-suffixed
// binary; the gamecode/renderer are .dylib carrying the build architecture. The
// data dir lives under ~/Library/Application Support/OpenJO; the retail GameData
// under ~/Library/Application Support/Steam.
func DetectPlatform(buildDir string) Platform {
	home := homeDir()
	a := arch()
	return Platform{
		OS:           runtime.GOOS,
		Arch:         a,
		DataDir:      envOr("JK2_DATA_DIR", filepath.Join(home, "Library", "Application Support", "OpenJO")),
		BinDir:       envOr("JK2_BIN_DIR", filepath.Join(home, "bin")),
		EngineName:   "openjo_sp." + a,
		GamecodeName: "jospgame" + a + ".dylib",
		RendererName: "rdjosp-vanilla_" + a + ".dylib",
	}
}

func steamRoots() []string {
	return []string{
		filepath.Join(homeDir(), "Library", "Application Support", "Steam"),
	}
}

// DefaultAssetsBase is the default retail base/ dir for the pak tools.
func DefaultAssetsBase() string {
	return filepath.Join(homeDir(), "Library", "Application Support", "OpenJO", "base")
}

// resolveEngine finds the actual engine executable and the directory the loader
// treats as fs_apppath (which holds the renderer). On macOS the engine is either
// an .app bundle or a plain binary; both are autodetected.
func resolveEngine(buildDir string, p Platform) (bin, dir string) {
	appBundle := filepath.Join(buildDir, "openjo_sp.app", "Contents", "MacOS", "openjo_sp")
	if isExecutable(appBundle) {
		return appBundle, filepath.Dir(appBundle)
	}
	plain := filepath.Join(buildDir, p.EngineName)
	if isExecutable(plain) {
		return plain, buildDir
	}
	return "", ""
}

func isExecutable(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0
}
