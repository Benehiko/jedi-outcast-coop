package textures

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// realesrganContainerfile is the on-demand build recipe for the local
// Real-ESRGAN image (see LocalUpscaleImage). It is built once, then cached by
// the runtime — we build our own rather than pull a third-party registry tag,
// all of which have rotted away.
//
//go:embed Containerfile.realesrgan
var realesrganContainerfile []byte

// LocalUpscaleImage is the tag of the image we build from
// realesrganContainerfile. It is local-only (localhost/), never pushed. The
// version suffix mirrors the pinned upstream release baked into the
// Containerfile — bump both together.
const LocalUpscaleImage = "localhost/jk2coop-realesrgan:v0.2.5.0"

// This file centralises the one piece of the texture pipelines that genuinely
// needs an external process: running a GPU model (Real-ESRGAN or FLUX) inside
// an ephemeral container. Everything else — extracting paks, normalising and
// snapping images, packing — is done natively in Go. The container step is
// isolated here so the rest of the package stays pure and testable.

// Runtime is a container runtime capable of `run --rm`.
type Runtime string

const (
	// RuntimeNerdctl is containerd's nerdctl CLI.
	RuntimeNerdctl Runtime = "nerdctl"
	// RuntimePodman is Podman.
	RuntimePodman Runtime = "podman"
)

// DetectRuntime picks a container runtime, preferring nerdctl then podman.
// It returns an error if neither is on PATH.
func DetectRuntime() (Runtime, error) {
	for _, rt := range []Runtime{RuntimeNerdctl, RuntimePodman} {
		if _, err := exec.LookPath(string(rt)); err == nil {
			return rt, nil
		}
	}
	return "", fmt.Errorf("no container runtime found (need %s or %s)", RuntimeNerdctl, RuntimePodman)
}

// ResolveRuntime validates an explicit runtime name, or autodetects when empty.
func ResolveRuntime(name string) (Runtime, error) {
	if name == "" {
		return DetectRuntime()
	}
	rt := Runtime(name)
	if rt != RuntimeNerdctl && rt != RuntimePodman {
		return "", fmt.Errorf("unsupported runtime %q (want %s or %s)", name, RuntimeNerdctl, RuntimePodman)
	}
	if _, err := exec.LookPath(name); err != nil {
		return "", fmt.Errorf("runtime %q not found on PATH", name)
	}
	return rt, nil
}

// hasRenderNode reports whether a DRI render node exists (i.e. a GPU is
// present) for Vulkan/ROCm passthrough.
func hasRenderNode() bool {
	_, err := os.Stat("/dev/dri/renderD128")
	return err == nil
}

// ContainerRun is a single `<runtime> run --rm …` invocation.
type ContainerRun struct {
	Runtime Runtime
	Image   string
	// Args are appended after the image (the container's own arguments).
	Args []string
	// Env holds KEY=VALUE strings passed with -e.
	Env []string
	// Mounts holds "hostpath:containerpath[:ro]" strings passed with -v.
	Mounts []string
	// ExtraFlags are runtime flags placed before the image (e.g. --device …).
	ExtraFlags []string
	// Entrypoint, when set, overrides the image entrypoint (["bash","-lc",script]).
	Entrypoint []string
	// Stdout/Stderr receive the container's output; nil means os.Stdout/os.Stderr.
	Stdout io.Writer
	Stderr io.Writer
}

// commandLine assembles the full argv for the run.
func (c ContainerRun) commandLine() []string {
	argv := []string{"run", "--rm"}
	argv = append(argv, c.ExtraFlags...)
	for _, e := range c.Env {
		argv = append(argv, "-e", e)
	}
	for _, m := range c.Mounts {
		argv = append(argv, "-v", m)
	}
	if len(c.Entrypoint) > 0 {
		// nerdctl/podman: --entrypoint takes the executable; the rest go as args.
		argv = append(argv, "--entrypoint", c.Entrypoint[0])
	}
	argv = append(argv, c.Image)
	if len(c.Entrypoint) > 1 {
		argv = append(argv, c.Entrypoint[1:]...)
	}
	argv = append(argv, c.Args...)
	return argv
}

// Run executes the container synchronously, streaming its output.
func (c ContainerRun) Run(ctx context.Context) error {
	stdout := c.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := c.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	argv := c.commandLine()
	cmd := exec.CommandContext(ctx, string(c.Runtime), argv...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s run failed: %w", c.Runtime, err)
	}
	return nil
}

// imageExists reports whether the runtime already has an image tagged ref, so
// we can skip the one-time build.
func (rt Runtime) imageExists(ctx context.Context, ref string) bool {
	cmd := exec.CommandContext(ctx, string(rt), "image", "inspect", ref)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	return cmd.Run() == nil
}

// buildImage builds ref from the given Containerfile bytes. The recipe is
// written to a temp build context (embedded assets have no on-disk home), then
// `<rt> build -t ref -f <file> <dir>` is run — syntax shared by nerdctl and
// podman. Build output is streamed so a slow first run is visible.
func (rt Runtime) buildImage(ctx context.Context, ref string, containerfile []byte, out io.Writer) error {
	if out == nil {
		out = os.Stderr
	}
	dir, err := os.MkdirTemp("", "jk2-realesrgan-build-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	cf := filepath.Join(dir, "Containerfile")
	if err := os.WriteFile(cf, containerfile, 0o644); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, string(rt), "build", "-t", ref, "-f", cf, dir)
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s build of %s failed: %w", rt, ref, err)
	}
	return nil
}

// EnsureUpscaleImage builds LocalUpscaleImage if the runtime does not already
// have it. Only our own local tag is ever built; a user-supplied --image is
// left untouched (the caller runs it as-is, letting the runtime pull/fail).
func (rt Runtime) EnsureUpscaleImage(ctx context.Context, ref string, progress func(string)) error {
	if ref != LocalUpscaleImage {
		return nil
	}
	if rt.imageExists(ctx, ref) {
		return nil
	}
	if progress != nil {
		progress(fmt.Sprintf("building Real-ESRGAN image %s (first run — downloads ~46 MB, ~1–2 min)…", ref))
	}
	return rt.buildImage(ctx, ref, realesrganContainerfile, nil)
}
