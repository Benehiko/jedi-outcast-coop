package dockerbuild

import (
	"fmt"
	"runtime"
	"strings"
)

// Target is the OS the built engine artifacts must run on. The build always
// happens inside a Linux container (the vee "docker" template is Alpine); a
// non-Linux Target is produced by cross-compiling from that Linux container.
type Target int

const (
	// TargetLinux builds native ELF binaries in the Linux container.
	TargetLinux Target = iota
	// TargetWindows cross-compiles Windows PE binaries with mingw-w64.
	TargetWindows
)

// String returns the lowercase OS name, matching runtime.GOOS values.
func (t Target) String() string {
	switch t {
	case TargetWindows:
		return "windows"
	default:
		return "linux"
	}
}

// mingw is the toolchain triple prefix used for the Windows cross-build.
const mingwPrefix = "x86_64-w64-mingw32"

// TargetForHost maps the host GOOS to the engine Target the container should
// produce ("match the host, cross-compile where needed"):
//
//   - linux   → TargetLinux   (native compile in the container)
//   - windows → TargetWindows (mingw-w64 cross-compile; the .exe runs on the host)
//   - darwin  → error         (a Linux container cannot emit a macOS Mach-O
//     binary, and Apple's SDK cannot be redistributed to do so — build on the
//     Mac with Xcode, or fetch the CI artifact)
//
// goos is normally runtime.GOOS; it is a parameter so the mapping is testable.
func TargetForHost(goos string) (Target, error) {
	switch goos {
	case "linux":
		return TargetLinux, nil
	case "windows":
		return TargetWindows, nil
	case "darwin":
		return 0, fmt.Errorf("a macOS engine cannot be built in a Linux container " +
			"(no Mach-O cross-compile without Apple's non-redistributable SDK); " +
			"build on this Mac with Xcode (jk2coop setup --host) or use the " +
			"'jk2coop-macos' CI artifact")
	default:
		return 0, fmt.Errorf("unsupported host OS %q for the docker build path", goos)
	}
}

// HostTarget is TargetForHost(runtime.GOOS).
func HostTarget() (Target, error) {
	return TargetForHost(runtime.GOOS)
}

// mingwToolchain is the CMake toolchain file for the Windows cross-build. It is
// written into the shared source tree and referenced with
// -DCMAKE_TOOLCHAIN_FILE. Kept in Go (not embed) because it is tiny and the
// triple must stay in lockstep with the Dockerfile's mingw packages.
func mingwToolchain() string {
	return strings.Join([]string{
		"set(CMAKE_SYSTEM_NAME Windows)",
		"set(CMAKE_SYSTEM_PROCESSOR x86_64)",
		"set(TOOLCHAIN_PREFIX " + mingwPrefix + ")",
		"set(CMAKE_C_COMPILER ${TOOLCHAIN_PREFIX}-gcc)",
		"set(CMAKE_CXX_COMPILER ${TOOLCHAIN_PREFIX}-g++)",
		"set(CMAKE_RC_COMPILER ${TOOLCHAIN_PREFIX}-windres)",
		"set(CMAKE_FIND_ROOT_PATH /usr/${TOOLCHAIN_PREFIX})",
		"set(CMAKE_FIND_ROOT_PATH_MODE_PROGRAM NEVER)",
		"set(CMAKE_FIND_ROOT_PATH_MODE_LIBRARY ONLY)",
		"set(CMAKE_FIND_ROOT_PATH_MODE_INCLUDE ONLY)",
		"",
	}, "\n")
}
