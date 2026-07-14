package cmd

import "testing"

func TestDecideBuild(t *testing.T) {
	yes := func(string) (bool, error) { return true, nil }
	no := func(string) (bool, error) { return false, nil }

	tests := []struct {
		name        string
		useVM       bool
		useHost     bool
		toolMissing bool
		veeAvail    bool
		interactive bool
		prompt      func(string) (bool, error)
		wantVM      bool
		wantErr     bool
	}{
		{name: "--vm forces VM", useVM: true, wantVM: true},
		{name: "--vm forces VM even without vee", useVM: true, veeAvail: false, wantVM: true},
		{name: "--host forces host", useHost: true, wantVM: false},
		{name: "--host with missing toolchain errors", useHost: true, toolMissing: true, wantErr: true},

		{name: "no vee, toolchain present -> host", veeAvail: false, wantVM: false},
		{name: "no vee, toolchain missing -> error", veeAvail: false, toolMissing: true, wantErr: true},

		{name: "non-interactive, vee, toolchain present -> host", veeAvail: true, interactive: false, wantVM: false},
		{name: "non-interactive, vee, toolchain missing -> VM", veeAvail: true, interactive: false, toolMissing: true, wantVM: true},

		{name: "interactive, user picks VM", veeAvail: true, interactive: true, prompt: yes, wantVM: true},
		{name: "interactive, user picks host", veeAvail: true, interactive: true, prompt: no, wantVM: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decideBuild(tc.useVM, tc.useHost, tc.toolMissing, tc.veeAvail, tc.interactive, tc.prompt)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("decideBuild() = %v, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("decideBuild() unexpected error: %v", err)
			}
			if got != tc.wantVM {
				t.Fatalf("decideBuild() = %v, want %v", got, tc.wantVM)
			}
		})
	}
}
