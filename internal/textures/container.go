package textures

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

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
