// Package progress renders single-line, in-place progress bars for the slow
// texture pipelines (Real-ESRGAN upscale, FLUX generate). It is deliberately
// dependency-free so both the cmd layer and the installer can share one bar.
package progress

import (
	"fmt"
	"io"
	"strings"
)

// Bar renders a single-line, in-place progress bar to an io.Writer using a
// carriage return, so repeated updates overwrite the same line. It is safe to
// call from one goroutine at a time (the textures pipeline drives it serially).
//
// Phases are named; when the phase changes the previous bar's line is finished
// with a newline so each phase leaves one completed line behind.
type Bar struct {
	w       io.Writer
	width   int    // bar cell count
	phase   string // current phase label
	started bool   // a line is currently open (needs a trailing newline)
	indent  string // leading indent for each line
}

// phaseLabels gives the textures pipeline's internal phase keys human titles.
var phaseLabels = map[string]string{
	"normalise": "Normalising",
	"upscale":   "Upscaling",
	"snap":      "Packing",
}

// New returns a Bar writing to w. indent is prepended to every line so the bar
// aligns with surrounding indented output (pass "" for none).
func New(w io.Writer, indent string) *Bar {
	return &Bar{w: w, width: 32, indent: indent}
}

// Update draws the bar for phase at done/total. total <= 0 renders an
// indeterminate count. When the phase changes, the prior phase's line is closed
// with a newline first.
func (b *Bar) Update(phase string, done, total int) {
	if b.w == nil {
		return
	}
	if phase != b.phase {
		if b.started {
			_, _ = fmt.Fprint(b.w, "\n")
		}
		b.phase = phase
		b.started = true
	}
	label := phaseLabels[phase]
	if label == "" {
		label = phase
	}
	if total <= 0 {
		_, _ = fmt.Fprintf(b.w, "\r%s%s… %d", b.indent, label, done)
		return
	}
	if done > total {
		done = total
	}
	filled := done * b.width / total
	bar := strings.Repeat("█", filled) + strings.Repeat("░", b.width-filled)
	pct := done * 100 / total
	_, _ = fmt.Fprintf(b.w, "\r%s%s [%s] %3d%% (%d/%d)", b.indent, label, bar, pct, done, total)
}

// Done closes any open bar line with a newline so subsequent output starts
// fresh. Safe to call when no line is open.
func (b *Bar) Done() {
	if b.w != nil && b.started {
		_, _ = fmt.Fprint(b.w, "\n")
		b.started = false
	}
}
