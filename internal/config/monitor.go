package config

import (
	"bufio"
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Resolution is a width x height pair in pixels.
type Resolution struct {
	W, H int
}

// AutoResFallback is the safe window size written for "auto" resolution when no
// explicit size is set. 1280x720 is small enough that every driver can create a
// window at it, avoiding the stale-indexed-mode / EGL-config failures that abort
// the renderer at larger or exotic modes.
var AutoResFallback = Resolution{W: 1280, H: 720}

// CommonResolutions is the fixed list of selectable resolutions offered by the
// graphics TUI, smallest to largest. The auto/native option (0x0) is offered
// separately and not part of this list.
var CommonResolutions = []Resolution{
	{1280, 720},
	{1366, 768},
	{1600, 900},
	{1920, 1080},
	{2560, 1080}, // 21:9 ultrawide
	{2560, 1440},
	{3440, 1440}, // 21:9 ultrawide
	{3840, 1080}, // 32:9 super-ultrawide
	{3840, 2160},
	{5120, 1440}, // 32:9 super-ultrawide
}

// DetectMonitor returns the primary monitor's current resolution, or ok=false
// if it cannot be determined (no display server reachable, tool missing, or the
// output could not be parsed). It never runs anything that installs software;
// it only shells out to display tools that are already present, and returns
// quietly on any failure so the TUI can fall back to a sensible default.
func DetectMonitor() (Resolution, bool) {
	// A short deadline keeps a wedged display tool from stalling the TUI.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if r, ok := detectXrandr(ctx); ok {
		return r, true
	}
	if r, ok := detectWlrRandr(ctx); ok {
		return r, true
	}
	return Resolution{}, false
}

// xrandrCurrent matches the resolution flagged with a trailing "*" (the mode
// currently in use) in an `xrandr` line, e.g. "   1920x1080     60.00*+".
var xrandrCurrent = regexp.MustCompile(`^\s*(\d+)x(\d+)\s+.*\*`)

func detectXrandr(ctx context.Context) (Resolution, bool) {
	out, err := exec.CommandContext(ctx, "xrandr", "--current").Output()
	if err != nil {
		return Resolution{}, false
	}
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		m := xrandrCurrent.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		w, _ := strconv.Atoi(m[1])
		h, _ := strconv.Atoi(m[2])
		if w > 0 && h > 0 {
			return Resolution{W: w, H: h}, true
		}
	}
	return Resolution{}, false
}

// wlrCurrent matches the current mode line of `wlr-randr`, e.g.
// "    1920x1080 px, 60.000000 Hz (preferred, current)".
var wlrCurrent = regexp.MustCompile(`^\s*(\d+)x(\d+)\s*px.*current`)

func detectWlrRandr(ctx context.Context) (Resolution, bool) {
	out, err := exec.CommandContext(ctx, "wlr-randr").Output()
	if err != nil {
		return Resolution{}, false
	}
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		m := wlrCurrent.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		w, _ := strconv.Atoi(m[1])
		h, _ := strconv.Atoi(m[2])
		if w > 0 && h > 0 {
			return Resolution{W: w, H: h}, true
		}
	}
	return Resolution{}, false
}
