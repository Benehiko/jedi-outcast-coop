package dockerbuild

import (
	"strings"
	"testing"

	"github.com/Benehiko/jedi-outcast-coop/internal/gfx"
)

func TestTargetForHost(t *testing.T) {
	tests := []struct {
		goos    string
		want    Target
		wantErr bool
	}{
		{goos: "linux", want: TargetLinux},
		{goos: "windows", want: TargetWindows},
		{goos: "darwin", wantErr: true},
		{goos: "plan9", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.goos, func(t *testing.T) {
			got, err := TargetForHost(tc.goos)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("TargetForHost(%q) = %v, want error", tc.goos, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("TargetForHost(%q) unexpected error: %v", tc.goos, err)
			}
			if got != tc.want {
				t.Fatalf("TargetForHost(%q) = %v, want %v", tc.goos, got, tc.want)
			}
		})
	}
}

func TestDarwinErrorGuidesToHostOrCI(t *testing.T) {
	_, err := TargetForHost("darwin")
	if err == nil {
		t.Fatal("expected an error for darwin")
	}
	// The message must point at the two real escape hatches.
	for _, want := range []string{"--host", "CI artifact"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("darwin error missing %q: %v", want, err)
		}
	}
}

func TestMingwToolchainContents(t *testing.T) {
	tc := mingwToolchain()
	for _, want := range []string{
		"set(CMAKE_SYSTEM_NAME Windows)",
		"set(TOOLCHAIN_PREFIX " + mingwPrefix + ")",
		"${TOOLCHAIN_PREFIX}-gcc",
		"${TOOLCHAIN_PREFIX}-g++",
		"${TOOLCHAIN_PREFIX}-windres",
		"CMAKE_FIND_ROOT_PATH_MODE_PROGRAM NEVER",
	} {
		if !strings.Contains(tc, want) {
			t.Errorf("mingwToolchain() missing %q", want)
		}
	}
}

func TestContainerScriptLinux(t *testing.T) {
	s := containerScript(TargetLinux, "build")
	// Configure line must use the shared, CI-identical flags.
	for _, arg := range gfx.ConfigureArgs {
		if !strings.Contains(s, arg) {
			t.Errorf("containerScript missing configure arg %q", arg)
		}
	}
	if !strings.Contains(s, "cmake --build "+containerSrc+"/build") {
		t.Errorf("containerScript missing build command:\n%s", s)
	}
	// Linux must NOT reference the mingw toolchain file.
	if strings.Contains(s, toolchainFile) {
		t.Errorf("Linux containerScript should not use the mingw toolchain:\n%s", s)
	}
}

func TestContainerScriptWindows(t *testing.T) {
	s := containerScript(TargetWindows, "build-windows")
	if !strings.Contains(s, "-DCMAKE_TOOLCHAIN_FILE="+containerSrc+"/"+toolchainFile) {
		t.Errorf("Windows containerScript missing toolchain flag:\n%s", s)
	}
	if !strings.Contains(s, "cmake --build "+containerSrc+"/build-windows") {
		t.Errorf("Windows containerScript wrong build dir:\n%s", s)
	}
}

func TestBuildSubdirDistinct(t *testing.T) {
	if buildSubdir(TargetLinux) == buildSubdir(TargetWindows) {
		t.Fatal("Linux and Windows build subdirs must differ to avoid clobbering")
	}
}

func TestBuildContextIsTarWithDockerfile(t *testing.T) {
	b, err := buildContext()
	if err != nil {
		t.Fatalf("buildContext() error: %v", err)
	}
	if len(b) == 0 {
		t.Fatal("buildContext() returned empty tar")
	}
	// The embedded Dockerfile must carry both toolchains' package hints.
	for _, want := range []string{"mingw-w64", "cmake", "ninja-build"} {
		if !strings.Contains(string(dockerfile), want) {
			t.Errorf("embedded Dockerfile missing %q", want)
		}
	}
}
