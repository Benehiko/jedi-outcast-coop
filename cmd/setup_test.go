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
		hostGOOS                  string
		want                      buildMethod
		wantErr                   bool
	}{
		// Default (no flag) on a container-capable host: docker when vee is available.
		{name: "default with vee -> docker", hostGOOS: "linux", veeAvail: true, want: buildDocker},
		{name: "default with vee, toolchain missing -> docker", hostGOOS: "linux", veeAvail: true, toolMissing: true, want: buildDocker},
		{name: "default no vee, toolchain present -> host", hostGOOS: "linux", veeAvail: false, want: buildHost},
		{name: "default no vee, toolchain missing -> error", hostGOOS: "linux", veeAvail: false, toolMissing: true, wantErr: true},

		// Explicit flags win (container-capable host).
		{name: "--host forces host", hostGOOS: "linux", useHost: true, veeAvail: true, want: buildHost},
		{name: "--host with missing toolchain errors", hostGOOS: "linux", useHost: true, toolMissing: true, wantErr: true},
		{name: "--vm with vee -> vm", hostGOOS: "linux", useVM: true, veeAvail: true, want: buildVM},
		{name: "--vm without vee errors", hostGOOS: "linux", useVM: true, veeAvail: false, wantErr: true},
		{name: "--docker with vee -> docker", hostGOOS: "linux", useDocker: true, veeAvail: true, want: buildDocker},
		{name: "--docker without vee errors", hostGOOS: "linux", useDocker: true, veeAvail: false, wantErr: true},

		// macOS: the container/VM paths can't emit a Mach-O binary, so the
		// default is the host build and an explicit --docker/--vm is rejected —
		// regardless of vee availability.
		{name: "darwin default -> host", hostGOOS: "darwin", veeAvail: true, want: buildHost},
		{name: "darwin default, vee absent -> host", hostGOOS: "darwin", veeAvail: false, want: buildHost},
		{name: "darwin default, toolchain missing -> error", hostGOOS: "darwin", toolMissing: true, wantErr: true},
		{name: "darwin --host -> host", hostGOOS: "darwin", useHost: true, want: buildHost},
		{name: "darwin --host, toolchain missing -> error", hostGOOS: "darwin", useHost: true, toolMissing: true, wantErr: true},
		{name: "darwin --docker rejected even with vee", hostGOOS: "darwin", useDocker: true, veeAvail: true, wantErr: true},
		{name: "darwin --vm rejected even with vee", hostGOOS: "darwin", useVM: true, veeAvail: true, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decideBuildMethod(tc.useVM, tc.useHost, tc.useDocker, tc.veeAvail, tc.toolMissing, tc.hostGOOS)
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
