//go:build windows

package install

import (
	"os"
	"path/filepath"
	"runtime"
)

// DetectPlatform returns the Windows install layout.
//
// The engine is openjo_sp.<arch>.exe; the gamecode/renderer are .dll. The data
// dir defaults under %LOCALAPPDATA%\OpenJO and launchers install as .cmd shims.
func DetectPlatform(buildDir string) Platform {
	a := arch()
	return Platform{
		OS:           runtime.GOOS,
		Arch:         a,
		DataDir:      envOr("JK2_DATA_DIR", filepath.Join(localAppData(), "OpenJO")),
		BinDir:       envOr("JK2_BIN_DIR", filepath.Join(localAppData(), "OpenJO", "bin")),
		EngineName:   "openjo_sp." + a + ".exe",
		GamecodeName: "jospgame" + a + ".dll",
		RendererName: "rdjosp-vanilla_" + a + ".dll",
		LauncherExt:  ".cmd",
	}
}

func localAppData() string {
	if v := os.Getenv("LOCALAPPDATA"); v != "" {
		return v
	}
	return filepath.Join(homeDir(), "AppData", "Local")
}

// steamRoots probes the common Windows Steam locations for GameData.
func steamRoots() []string {
	var roots []string
	if pf := os.Getenv("ProgramFiles(x86)"); pf != "" {
		roots = append(roots, filepath.Join(pf, "Steam"))
	}
	if pf := os.Getenv("ProgramFiles"); pf != "" {
		roots = append(roots, filepath.Join(pf, "Steam"))
	}
	roots = append(roots, `C:\Program Files (x86)\Steam`, `C:\Program Files\Steam`)
	return roots
}

// DefaultAssetsBase is the default retail base/ dir for the pak tools.
func DefaultAssetsBase() string {
	return filepath.Join(localAppData(), "OpenJO", "base")
}

// resolveEngine returns the engine executable and the dir holding the renderer.
func resolveEngine(buildDir string, p Platform) (bin, dir string) {
	return filepath.Join(buildDir, p.EngineName), buildDir
}
