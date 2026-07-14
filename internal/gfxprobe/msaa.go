// Package gfxprobe verifies that graphics settings the TUI offers can actually
// be realised by the installed engine on the current machine, so an unsupported
// choice degrades gracefully instead of crashing the game at launch.
//
// The motivating case: on some Mesa/Wayland setups (radeonsi via SDL2's EGL
// path) requesting a high MSAA sample count makes eglChooseConfig fail to find a
// matching config. SDL_CreateWindow then fails for every resolution and the
// renderer reports the misleading "no display modes could be found", aborting
// startup. There is no cvar that reports the max supported sample count before a
// window exists, so we probe by briefly launching the engine and watching for
// the failure signature.
package gfxprobe

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/Benehiko/jedi-outcast-coop/internal/install"
)

// probeFailure holds the substrings that mark a failed GL/EGL init in the
// engine's output. Any of these means the probed sample count is unusable.
var probeFailure = [][]byte{
	[]byte("no display modes could be found"),
	[]byte("could not load OpenGL subsystem"),
	[]byte("Couldn't find matching EGL config"),
}

// MSAASupported reports whether the installed engine can create a GL context at
// the given multisample sample count on this machine. samples of 0 (MSAA off) is
// always considered supported without probing.
//
// ok is the result; probed is false when the check could not run at all (engine
// not built, no display server, probe timed out or the binary could not be
// launched) — callers should treat probed=false as "don't second-guess the
// user" and keep the requested value rather than assuming failure.
func MSAASupported(ctx context.Context, p install.Platform, buildDir string, samples int) (ok, probed bool) {
	if samples <= 0 {
		return true, true
	}
	bin, dir := p.ResolveEngine(buildDir)
	if bin == "" {
		return false, false // engine not built — nothing to probe
	}
	if fi, err := os.Stat(bin); err != nil || fi.IsDir() {
		return false, false
	}

	// Probe in a throwaway fs_homepath so the real config, saved cvars and crash
	// logs are never touched. A tiny 640x480 windowed run is enough to force the
	// EGL config selection that fails.
	tmp, err := os.MkdirTemp("", "jk2coop-msaaprobe-")
	if err != nil {
		return false, false
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	// A working GL init reaches "+quit" and exits in well under a second; a failed
	// one prints the EGL/display error and then can hang indefinitely instead of
	// quitting. The engine's stdout is block-buffered when piped, so we can't scan
	// it live — instead redirect combined output to a file (flushed to the OS as
	// it's written, so it survives a kill) and poll the file for a decisive line.
	// As soon as a failure signature or the success marker appears we kill the
	// process; the overall deadline is a backstop for output that shows neither.
	logf, err := os.CreateTemp(tmp, "probe-*.log")
	if err != nil {
		return false, false
	}
	defer func() { _ = logf.Close() }()

	runCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, bin,
		// fs_basepath points at the real data dir so assets/default.cfg load (a
		// missing default.cfg aborts before GL init); fs_homepath is the throwaway
		// tmp so the user's config and saved cvars are never touched.
		"+set", "fs_basepath", p.DataDir,
		"+set", "fs_homepath", tmp,
		"+set", "com_homepath", tmp,
		"+set", "r_fullscreen", "0",
		"+set", "r_mode", "3", // 640x480, smallest indexed mode
		"+set", "developer", "1", // unmask the Com_DPrintf EGL error
		"+set", "r_ext_multisample", strconv.Itoa(samples),
		"+quit",
	)
	// The renderer module sits beside the engine binary; run from there so the
	// loader finds it, matching how the launcher runs the game.
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdout = logf
	cmd.Stderr = logf
	if err := cmd.Start(); err != nil {
		return false, false
	}

	// Poll the log for a verdict while the process runs. done closes when the
	// engine exits on its own (clean +quit).
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()

	verdict := 0 // 0 = undecided, 1 = supported, -1 = unsupported
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
poll:
	for {
		if data, rerr := os.ReadFile(logf.Name()); rerr == nil {
			switch {
			case containsAny(data, probeFailure):
				verdict = -1
				break poll
			case bytes.Contains(data, successMarker):
				verdict = 1
				break poll
			}
		}
		select {
		case <-done:
			// Process exited; do a final read to catch a verdict flushed at exit.
			if data, rerr := os.ReadFile(logf.Name()); rerr == nil {
				if containsAny(data, probeFailure) {
					verdict = -1
				} else if bytes.Contains(data, successMarker) {
					verdict = 1
				}
			}
			break poll
		case <-runCtx.Done():
			break poll // deadline/cancel — decide from whatever the log holds
		case <-ticker.C:
		}
	}

	// Stop the engine (it may be hung after a failure) and reap it.
	_ = cmd.Process.Kill()
	<-done

	switch verdict {
	case 1:
		return true, true
	case -1:
		return false, true
	default:
		// No decisive line. A process that exited cleanly with no error is
		// treated as supported; a killed/hung one with no verdict is inconclusive.
		if runCtx.Err() == context.DeadlineExceeded {
			return false, false
		}
		return true, true
	}
}

// successMarker is printed once the GL context is created — the point past which
// the requested sample count is proven usable ("Using N color bits, ...").
var successMarker = []byte("color bits")

func containsAny(b []byte, subs [][]byte) bool {
	for _, s := range subs {
		if bytes.Contains(b, s) {
			return true
		}
	}
	return false
}

// HighestSupportedMSAA returns the largest value in candidates (which must be
// sorted ascending, e.g. {2,4,8,16}) that the engine can realise, at or below
// want. It returns want unchanged if probing is unavailable, so a machine that
// cannot be probed keeps the user's choice. It never returns a value above want.
func HighestSupportedMSAA(ctx context.Context, p install.Platform, buildDir string, want int, candidates []int) (result int, probed bool) {
	if want <= 0 {
		return want, true
	}
	// Walk down from want through the candidate levels, returning the first that
	// probes clean. Probing stops as soon as one succeeds.
	for i := len(candidates) - 1; i >= 0; i-- {
		c := candidates[i]
		if c > want {
			continue
		}
		ok, ran := MSAASupported(ctx, p, buildDir, c)
		if !ran {
			return want, false // cannot probe — respect the user's choice
		}
		if ok {
			return c, true
		}
	}
	// Nothing in the list probed clean; MSAA off is always safe.
	return 0, true
}
