package gfx

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// Build runs the CMake configure (if needed) and build for the JK2 SP engine,
// renderer, and gamecode, streaming output to the given writer. buildDir is the
// CMake build directory (e.g. <repo>/openjk/build); srcDir is the OpenJK source
// (the submodule root). It is a thin wrapper over the same cmake invocation the
// docs describe, so "built via gfx" and "built by hand" stay identical.
func Build(ctx context.Context, srcDir, buildDir string, out interface {
	Write([]byte) (int, error)
},
) error {
	// Configure only when the build tree does not yet exist; an existing tree
	// re-uses its cached generator + options.
	if _, err := os.Stat(buildDir + "/CMakeCache.txt"); err != nil {
		cfg := exec.CommandContext(ctx, "cmake",
			"-S", srcDir, "-B", buildDir, "-G", "Ninja",
			"-DCMAKE_BUILD_TYPE=Release",
			"-DBuildJK2SPEngine=ON",
			"-DBuildJK2SPGame=ON",
			"-DBuildJK2SPRdVanilla=ON",
		)
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
