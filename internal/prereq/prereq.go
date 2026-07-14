// Package prereq detects the native build toolchain the OpenJK engine build
// needs (cmake, ninja, a C/C++ compiler) and produces copy-paste guidance for
// installing what is missing, tailored to the host OS and Linux distro family.
//
// It is deliberately dependency-free (no other internal package) so it can be
// imported anywhere — notably by internal/gfx, which builds the engine — without
// risking an import cycle.
package prereq

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Tool is one build prerequisite exposed as a PATH command, with the package
// name that provides it on each supported package manager.
type Tool struct {
	// Cmd is the executable probed on PATH (e.g. "cmake").
	Cmd string
	// Why is a short human reason the tool is needed, shown in guidance.
	Why string
}

// coreTools are the executables the build shells out to. The C compiler is
// probed as "cc" (the POSIX name gcc/clang both provide) so either satisfies it.
var coreTools = []Tool{
	{Cmd: "cmake", Why: "configures and drives the engine build"},
	{Cmd: "ninja", Why: "the build backend cmake generates for"},
	{Cmd: "cc", Why: "the C/C++ compiler (gcc or clang)"},
}

// The full package sets per manager, kept in one place so the guidance and the
// VM build (internal/vmbuild) install the same libraries. The library packages
// (SDL2, OpenAL, zlib, libpng, libjpeg) are not PATH commands, so they cannot be
// probed directly — they are always listed in the install line. The apt set
// mirrors .github/workflows/build.yml.
const (
	AptPackages    = "cmake ninja-build build-essential libsdl2-dev libopenal-dev zlib1g-dev libpng-dev libjpeg-dev"
	PacmanPackages = "cmake ninja gcc sdl2 openal zlib libpng libjpeg-turbo"
	DnfPackages    = "cmake ninja-build gcc-c++ SDL2-devel openal-soft-devel zlib-devel libpng-devel libjpeg-turbo-devel"
	BrewPackages   = "cmake ninja sdl2 openal-soft libpng jpeg-turbo"
)

// lookPath is indirected for testing.
var lookPath = exec.LookPath

// Missing returns the core tools not found on PATH, in declared order. An empty
// slice means the toolchain commands are all present (library headers are not
// probed — a failed cmake configure is the real gate for those).
func Missing() []Tool {
	var missing []Tool
	for _, t := range coreTools {
		if _, err := lookPath(t.Cmd); err != nil {
			missing = append(missing, t)
		}
	}
	return missing
}

// Guidance returns a human-readable block naming the missing tools and the
// exact command to install the full build toolchain for the host OS (and, on
// Linux, the detected distro family). It is safe to call with an empty slice —
// it then returns "" — but callers normally only call it when Missing is
// non-empty.
func Guidance(missing []Tool) string {
	if len(missing) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("Missing build tools:\n")
	for _, t := range missing {
		fmt.Fprintf(&b, "  - %s (%s)\n", t.Cmd, t.Why)
	}
	b.WriteString("\nInstall the engine build toolchain:\n")
	b.WriteString(installLines(runtime.GOOS, readOSRelease()))
	return strings.TrimRight(b.String(), "\n")
}

// installLines returns the indented install command(s) for the given OS. osID is
// the /etc/os-release content used only on Linux to pick the package manager.
func installLines(goos, osRelease string) string {
	switch goos {
	case "darwin":
		return "  xcode-select --install   # C toolchain\n" +
			"  brew install " + BrewPackages + "\n"
	case "windows":
		return "  Install Visual Studio (with the C++ workload) and CMake:\n" +
			"    https://visualstudio.microsoft.com/  and  https://cmake.org/download/\n" +
			"  Or skip building: download the prebuilt 'jk2coop-windows' engine\n" +
			"  artifact from a green CI run (see docs/install-windows.md).\n"
	default: // linux and unknown unix
		switch linuxFamily(osRelease) {
		case "debian":
			return "  sudo apt-get install -y " + AptPackages + "\n"
		case "arch":
			return "  sudo pacman -S --needed " + PacmanPackages + "\n"
		case "fedora":
			return "  sudo dnf install -y " + DnfPackages + "\n"
		default:
			return "  Debian/Ubuntu: sudo apt-get install -y " + AptPackages + "\n" +
				"  Arch:          sudo pacman -S --needed " + PacmanPackages + "\n" +
				"  Fedora:        sudo dnf install -y " + DnfPackages + "\n"
		}
	}
}

// linuxFamily maps /etc/os-release ID / ID_LIKE to a package-manager family:
// "debian", "arch", "fedora", or "" when it cannot be determined.
func linuxFamily(osRelease string) string {
	ids := osReleaseIDs(osRelease)
	for _, id := range ids {
		switch id {
		case "debian", "ubuntu", "linuxmint", "pop", "raspbian":
			return "debian"
		case "arch", "manjaro", "endeavouros", "cachyos":
			return "arch"
		case "fedora", "rhel", "centos", "rocky", "almalinux":
			return "fedora"
		}
	}
	return ""
}

// osReleaseIDs extracts the ID then ID_LIKE tokens from os-release content, in
// that priority order (ID first so a derivative's own name wins, then its
// upstream family from ID_LIKE).
func osReleaseIDs(content string) []string {
	var id string
	var likes []string
	for line := range strings.SplitSeq(content, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "ID="):
			id = unquote(strings.TrimPrefix(line, "ID="))
		case strings.HasPrefix(line, "ID_LIKE="):
			likes = strings.Fields(unquote(strings.TrimPrefix(line, "ID_LIKE=")))
		}
	}
	var out []string
	if id != "" {
		out = append(out, id)
	}
	return append(out, likes...)
}

func unquote(s string) string {
	return strings.Trim(strings.TrimSpace(s), `"'`)
}

// readOSRelease returns the contents of /etc/os-release (or the systemd
// fallback), or "" when neither is readable (non-Linux, or unreadable).
func readOSRelease() string {
	for _, p := range []string{"/etc/os-release", "/usr/lib/os-release"} {
		if b, err := os.ReadFile(p); err == nil {
			return string(b)
		}
	}
	return ""
}
