package install

import (
	"bytes"
	"strings"
	"testing"
)

func TestWarnfRecordsAndPrints(t *testing.T) {
	var buf bytes.Buffer
	o := &Options{Out: &buf}
	o.warnf("texture upscale failed — zzz-hires-textures.pk3 not installed",
		"upscale-textures.sh exited: exit status 1",
		"see docs/hires-textures.md")

	if len(o.warnings) != 1 {
		t.Fatalf("want 1 recorded warning, got %d", len(o.warnings))
	}
	out := buf.String()
	// A distinct WARNING marker sets it apart from the indented success lines.
	if !strings.Contains(out, "WARNING:") {
		t.Errorf("warning must be visually distinct (WARNING:):\n%s", out)
	}
	// The underlying error must be surfaced, not swallowed.
	if !strings.Contains(out, "exit status 1") {
		t.Errorf("warning must include the real error detail:\n%s", out)
	}
	if !strings.Contains(out, "docs/hires-textures.md") {
		t.Errorf("warning should point at the docs:\n%s", out)
	}
}

func TestSummarizeCleanInstall(t *testing.T) {
	var buf bytes.Buffer
	o := &Options{Out: &buf}
	o.summarize()
	out := buf.String()
	if !strings.Contains(out, "Installed. Try:") {
		t.Errorf("clean install summary should be plain:\n%s", out)
	}
	if strings.Contains(out, "warning") {
		t.Errorf("clean install must not mention warnings:\n%s", out)
	}
}

func TestSummarizeRelistsWarnings(t *testing.T) {
	var buf bytes.Buffer
	o := &Options{Out: &buf}
	o.warnf("texture upscale failed — zzz-hires-textures.pk3 not installed", "detail")
	o.warnf("AI texture generation failed — zzz-generated-textures.pk3 not installed", "detail")
	buf.Reset() // isolate the summary from the at-failure prints

	o.summarize()
	out := buf.String()
	// Count + both summaries reappear, so a failure buried in the log is caught.
	if !strings.Contains(out, "2 warnings") {
		t.Errorf("summary should count the warnings:\n%s", out)
	}
	if !strings.Contains(out, "texture upscale failed") ||
		!strings.Contains(out, "AI texture generation failed") {
		t.Errorf("summary should re-list every failed extra:\n%s", out)
	}
	// And it must make clear the game is still usable.
	if !strings.Contains(out, "installed and playable") {
		t.Errorf("summary should reassure the install is usable:\n%s", out)
	}
}

func TestSummarizeSingularWarning(t *testing.T) {
	var buf bytes.Buffer
	o := &Options{Out: &buf}
	o.warnf("widescreen video-menu modes not installed", "detail")
	buf.Reset()
	o.summarize()
	if out := buf.String(); !strings.Contains(out, "1 warning ") {
		t.Errorf("single failure should read '1 warning' (singular):\n%s", out)
	}
}
