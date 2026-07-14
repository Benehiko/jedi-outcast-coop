// Package dockerbuild builds the OpenJK engine inside a container running in a
// disposable Docker VM managed by the user's `vee` tool. The host needs neither
// a C/C++ build toolchain nor Docker itself: the Docker daemon lives inside the
// vee "docker" template VM (Alpine + dockerd), and this package drives it over
// the Docker Engine API (tcp://127.0.0.1:2375, forwarded by vee) with a minimal
// HTTP client — no Docker SDK, no testcontainers.
//
// The host source tree is shared into the VM over virtiofs and bind-mounted
// into the build container, so the artifacts the build writes under the source
// tree appear back on the host automatically (the same writeback the native and
// vmbuild paths rely on).
//
// Targets follow "match the host, cross-compile where needed": a Linux host
// builds Linux ELF binaries natively in the container; a Windows host builds
// Windows PE binaries via the mingw-w64 cross toolchain baked into the image
// (the .exe runs on the host). A macOS host cannot be served here — see
// TargetForHost.
package dockerbuild

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Benehiko/jedi-outcast-coop/internal/gfx"
)

// imageTag is the tag of the build image produced inside the VM.
const imageTag = "jk2coop-build:latest"

// containerSrc is where the shared source tree is bind-mounted in the container.
const containerSrc = "/src"

// readyTimeout bounds how long we wait for dockerd to answer after (re)start.
const readyTimeout = 90 * time.Second

// Build compiles the engine for the host's Target inside the vee docker-build
// VM and leaves the artifacts under shareDir/<srcSub>/<buildSub> on the host.
//
//   - shareDir is the host directory shared into the VM over virtiofs (the repo
//     root for the dev flow, the work-dir root for the standalone flow).
//   - srcSub is the OpenJK source dir RELATIVE to shareDir ("openjk" or "src").
//
// The co-op patches must already be applied to the shared tree on the host; the
// container only compiles. Progress streams to out.
func Build(ctx context.Context, shareDir, srcSub string, out io.Writer) error {
	if !Available() {
		return fmt.Errorf("vee not found on PATH; install it or build on this machine instead")
	}
	target, err := HostTarget()
	if err != nil {
		return err
	}

	// For a Windows build, drop the CMake toolchain file into the shared tree so
	// the container can reference it by a path under the bind mount.
	buildSub := buildSubdir(target)
	if target == TargetWindows {
		if err := writeToolchain(filepath.Join(shareDir, srcSub)); err != nil {
			return err
		}
	}

	_, _ = fmt.Fprintf(out, "Creating docker-build VM %q (sharing %s over virtiofs)…\n", VMName, shareDir)
	if err := veeCreate(ctx, shareDir, out); err != nil {
		return err
	}

	_, _ = fmt.Fprintln(out, "Preparing guest (mount share, start dockerd)…")
	if err := prepareGuest(ctx, out); err != nil {
		return err
	}

	client := newClient(defaultBase())
	_, _ = fmt.Fprint(out, "Waiting for the Docker daemon")
	waitCtx, cancel := context.WithTimeout(ctx, readyTimeout)
	defer cancel()
	if err := client.waitReady(waitCtx, out); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, " ready.")

	if err := ensureImage(ctx, client, out); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(out, "Building the engine in a container (target: %s)…\n", target)
	return runBuild(ctx, client, target, srcSub, buildSub, out)
}

// ensureImage builds the deps image unless a warm VM already has it.
func ensureImage(ctx context.Context, client *dockerClient, out io.Writer) error {
	exists, err := client.imageExists(ctx, imageTag)
	if err != nil {
		return err
	}
	if exists {
		_, _ = fmt.Fprintln(out, "Build image already present; skipping image build.")
		return nil
	}
	tarCtx, err := buildContext()
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, "Building the deps image (cmake, ninja, mingw-w64, libraries)…")
	return client.buildImage(ctx, tarCtx, imageTag, out)
}

// runBuild creates, runs, and tails a container that configures + builds the
// engine, then reports its exit status. The source is bind-mounted from the
// guest's virtiofs mount; buildSub is written under the source tree and appears
// on the host through the share.
func runBuild(ctx context.Context, client *dockerClient, target Target, srcSub, buildSub string, out io.Writer) error {
	// The guest path of the shared source is <guestMount>/<srcSub>.
	guestSrc := guestMount + "/" + srcSub

	id, err := client.createContainer(ctx, createOptions{
		Image:      imageTag,
		Cmd:        []string{"sh", "-c", containerScript(target, buildSub)},
		Binds:      []string{guestSrc + ":" + containerSrc},
		WorkingDir: containerSrc,
	})
	if err != nil {
		return err
	}
	// Always clean up the container; the image (the expensive part) is kept.
	defer func() {
		// Use a fresh context so cleanup runs even if ctx was cancelled.
		rmCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		_ = client.removeContainer(rmCtx, id)
	}()

	if err := client.startContainer(ctx, id); err != nil {
		return err
	}
	if err := client.streamLogs(ctx, id, out); err != nil {
		return err
	}
	code, err := client.waitContainer(ctx, id)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("engine build failed in container (exit %d)", code)
	}
	return nil
}

// containerScript is the shell run inside the container: configure (if needed)
// then build, using the shared CI-identical configure flags. For Windows it
// also points CMake at the mingw toolchain file.
func containerScript(target Target, buildSub string) string {
	args := append([]string{"-S", containerSrc, "-B", containerSrc + "/" + buildSub}, gfx.ConfigureArgs...)
	if target == TargetWindows {
		args = append(args, "-DCMAKE_TOOLCHAIN_FILE="+containerSrc+"/"+toolchainFile)
	}
	configure := "cmake " + strings.Join(args, " ")
	build := "cmake --build " + containerSrc + "/" + buildSub
	return strings.Join([]string{
		"set -eu",
		// Configure only when the build tree is absent (an existing tree reuses
		// its cache), matching gfx.Build's behaviour.
		"[ -f " + containerSrc + "/" + buildSub + "/CMakeCache.txt ] || " + configure,
		build,
	}, "\n")
}

// toolchainFile is the mingw toolchain file's name, written into the source tree
// root (referenced under the container bind mount as <containerSrc>/<name>).
const toolchainFile = "jk2coop-mingw-w64.cmake"

// writeToolchain writes the mingw CMake toolchain file into srcDir on the host
// (visible to the container through the virtiofs share).
func writeToolchain(srcDir string) error {
	p := filepath.Join(srcDir, toolchainFile)
	if err := os.WriteFile(p, []byte(mingwToolchain()), 0o644); err != nil {
		return fmt.Errorf("writing mingw toolchain file: %w", err)
	}
	return nil
}

// buildSubdir is the build output directory (relative to the source tree) for a
// target. Distinct dirs keep the Linux and Windows trees from clobbering each
// other in a shared source checkout.
func buildSubdir(target Target) string {
	if target == TargetWindows {
		return "build-windows"
	}
	return "build"
}
