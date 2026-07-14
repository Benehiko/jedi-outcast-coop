package prereq

import (
	"errors"
	"strings"
	"testing"
)

func TestMissing(t *testing.T) {
	tests := []struct {
		name    string
		present map[string]bool
		want    []string
	}{
		{
			name:    "all present",
			present: map[string]bool{"cmake": true, "ninja": true, "cc": true},
			want:    nil,
		},
		{
			name:    "cmake and ninja missing",
			present: map[string]bool{"cc": true},
			want:    []string{"cmake", "ninja"},
		},
		{
			name:    "none present",
			present: map[string]bool{},
			want:    []string{"cmake", "ninja", "cc"},
		},
	}

	orig := lookPath
	t.Cleanup(func() { lookPath = orig })

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lookPath = func(cmd string) (string, error) {
				if tc.present[cmd] {
					return "/usr/bin/" + cmd, nil
				}
				return "", errors.New("not found")
			}
			var got []string
			for _, m := range Missing() {
				got = append(got, m.Cmd)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("Missing() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGuidanceEmpty(t *testing.T) {
	if got := Guidance(nil); got != "" {
		t.Fatalf("Guidance(nil) = %q, want empty", got)
	}
}

func TestInstallLines(t *testing.T) {
	tests := []struct {
		name      string
		goos      string
		osRelease string
		wantSub   string
	}{
		{"debian by ID", "linux", `ID=ubuntu` + "\n", "apt-get install"},
		{"arch by ID", "linux", `ID=arch` + "\n", "pacman -S"},
		{"fedora by ID", "linux", `ID=fedora` + "\n", "dnf install"},
		{"derivative via ID_LIKE", "linux", "ID=endeavouros\nID_LIKE=arch\n", "pacman -S"},
		{"mint via ID_LIKE", "linux", "ID=linuxmint\nID_LIKE=\"ubuntu debian\"\n", "apt-get install"},
		{"unknown linux lists all", "linux", "ID=weird\n", "Debian/Ubuntu"},
		{"empty os-release lists all", "linux", "", "pacman -S"},
		{"darwin", "darwin", "", "brew install"},
		{"darwin xcode", "darwin", "", "xcode-select"},
		{"windows", "windows", "", "Visual Studio"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := installLines(tc.goos, tc.osRelease)
			if !strings.Contains(got, tc.wantSub) {
				t.Fatalf("installLines(%q, %q) = %q, want substring %q", tc.goos, tc.osRelease, got, tc.wantSub)
			}
		})
	}
}

func TestGuidanceNamesMissingTools(t *testing.T) {
	got := Guidance([]Tool{{Cmd: "cmake", Why: "x"}, {Cmd: "ninja", Why: "y"}})
	for _, want := range []string{"cmake", "ninja", "Install the engine build toolchain"} {
		if !strings.Contains(got, want) {
			t.Fatalf("Guidance missing %q in:\n%s", want, got)
		}
	}
}
