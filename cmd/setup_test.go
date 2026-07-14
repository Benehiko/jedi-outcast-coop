package cmd

import "testing"

func TestExclusiveBuildFlags(t *testing.T) {
	tests := []struct {
		name                   string
		useVM, useHost, useDkr bool
		wantErr                bool
	}{
		{name: "none", wantErr: false},
		{name: "vm only", useVM: true},
		{name: "host only", useHost: true},
		{name: "docker only", useDkr: true},
		{name: "vm+host", useVM: true, useHost: true, wantErr: true},
		{name: "vm+docker", useVM: true, useDkr: true, wantErr: true},
		{name: "host+docker", useHost: true, useDkr: true, wantErr: true},
		{name: "all three", useVM: true, useHost: true, useDkr: true, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := exclusiveBuildFlags(tc.useVM, tc.useHost, tc.useDkr)
			if (err != nil) != tc.wantErr {
				t.Fatalf("exclusiveBuildFlags(%v,%v,%v) err=%v, wantErr=%v",
					tc.useVM, tc.useHost, tc.useDkr, err, tc.wantErr)
			}
		})
	}
}

func TestDecideBuildMethod(t *testing.T) {
	tests := []struct {
		name                      string
		useVM, useHost, useDocker bool
		veeAvail, toolMissing     bool
		want                      buildMethod
		wantErr                   bool
	}{
		// Default (no flag): docker when vee is available.
		{name: "default with vee -> docker", veeAvail: true, want: buildDocker},
		{name: "default with vee, toolchain missing -> docker", veeAvail: true, toolMissing: true, want: buildDocker},
		{name: "default no vee, toolchain present -> host", veeAvail: false, want: buildHost},
		{name: "default no vee, toolchain missing -> error", veeAvail: false, toolMissing: true, wantErr: true},

		// Explicit flags win.
		{name: "--host forces host", useHost: true, veeAvail: true, want: buildHost},
		{name: "--host with missing toolchain errors", useHost: true, toolMissing: true, wantErr: true},
		{name: "--vm with vee -> vm", useVM: true, veeAvail: true, want: buildVM},
		{name: "--vm without vee errors", useVM: true, veeAvail: false, wantErr: true},
		{name: "--docker with vee -> docker", useDocker: true, veeAvail: true, want: buildDocker},
		{name: "--docker without vee errors", useDocker: true, veeAvail: false, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decideBuildMethod(tc.useVM, tc.useHost, tc.useDocker, tc.veeAvail, tc.toolMissing)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("decideBuildMethod() = %v, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("decideBuildMethod() unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("decideBuildMethod() = %v, want %v", got, tc.want)
			}
		})
	}
}
