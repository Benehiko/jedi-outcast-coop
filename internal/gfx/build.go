package gfx

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/Benehiko/jedi-outcast-coop/internal/prereq"
)

// ConfigureArgs are the `cmake -S <src> -B <build>` configure flags for the
// JK2-SP-only engine build: RelWithDebInfo, the three JK2 SP targets on, every
// other (SP-vanilla, MP) target off. Exported so both the host build here and
// the VM build (internal/vmbuild) issue the identical configure line, keeping
// them byte-compatible with docs/building.md and .github/workflows/build.yml.
var ConfigureArgs = []string{
	"-G", "Ninja",
	"-DCMAKE_BUILD_TYPE=RelWithDebInfo",
	"-DBuildJK2SPEngine=ON",
	"-DBuildJK2SPGame=ON",
	"-DBuildJK2SPRdVanilla=ON",
	"-DBuildSPEngine=OFF", "-DBuildSPGame=OFF", "-DBuildSPRdVanilla=OFF",
	"-DBuildMPEngine=OFF", "-DBuildMPRdVanilla=OFF", "-DBuildMPDed=OFF",
	"-DBuildMPGame=OFF", "-DBuildMPCGame=OFF", "-DBuildMPUI=OFF", "-DBuildMPRend2=OFF",
}

// Build runs the CMake configure (if needed) and build for the JK2 SP engine,
// renderer, and gamecode, streaming output to the given writer. buildDir is the
// CMake build directory (e.g. <repo>/openjk/build); srcDir is the OpenJK source
// (the submodule root). It uses the same configure flags (RelWithDebInfo, the
// JK2-SP targets on, every other target off) as docs/building.md and the CI
// build (.github/workflows/build.yml), so "built via gfx", "built by hand", and
// "built in CI" stay identical.
func Build(ctx context.Context, srcDir, buildDir string, out interface {
	Write([]byte) (int, error)
},
) error {
	// Preflight: fail early with actionable, copy-paste install guidance when the
	// build toolchain is missing, instead of a raw `exec: "cmake": not found`.
	if missing := prereq.Missing(); len(missing) > 0 {
		return fmt.Errorf("cannot build the engine:\n%s", prereq.Guidance(missing))
	}

	// Configure only when the build tree does not yet exist; an existing tree
	// re-uses its cached generator + options.
	if _, err := os.Stat(buildDir + "/CMakeCache.txt"); err != nil {
		args := append([]string{"-S", srcDir, "-B", buildDir}, ConfigureArgs...)
		cfg := exec.CommandContext(ctx, "cmake", args...)
		cfg.Stdout, cfg.Stderr = out, out
		if err := cfg.Run(); err != nil {
			return fmt.Errorf("cmake configure: %w", err)
		}
	}

	b := exec.CommandContext(ctx, "cmake", "--build", buildDir)
	b.Stdout, b.Stderr = out, out
	if err := b.Run(); err != nil {
		return fmt.Errorf("cmake build: %w", err)
	}
	return nil
}
