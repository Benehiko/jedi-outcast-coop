package launch

import (
	"strings"
	"testing"

	"github.com/Benehiko/jedi-outcast-coop/internal/install"
)

func testPlatform() install.Platform {
	return install.Platform{OS: "linux", Arch: "x86_64", DataDir: "/data"}
}

func joined(args []string) string { return strings.Join(args, " ") }

func TestArgsSinglePlayerDefaults(t *testing.T) {
	got, err := Args(testPlatform(), &Options{Mode: SinglePlayer, Fullscreen: true})
	if err != nil {
		t.Fatal(err)
	}
	s := joined(got)
	if !strings.Contains(s, "+set fs_basepath /data") {
		t.Errorf("missing fs_basepath: %s", s)
	}
	if !strings.Contains(s, "+set r_fullscreen 1") {
		t.Errorf("missing fullscreen: %s", s)
	}
	if !strings.HasSuffix(s, "+map "+install.DefaultMap) {
		t.Errorf("want default map at end, got: %s", s)
	}
	if strings.Contains(s, "g_skipIntroCinematics") {
		t.Errorf("skip-cutscenes should be off by default: %s", s)
	}
}

func TestArgsWindowedAndSkipAndExtra(t *testing.T) {
	got, err := Args(testPlatform(), &Options{
		Mode:          SinglePlayer,
		Map:           "t2_trip",
		Fullscreen:    false,
		SkipCutscenes: true,
		Extra:         []string{"+set", "r_mode", "-2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := joined(got)
	if !strings.Contains(s, "+set r_fullscreen 0") {
		t.Errorf("want windowed: %s", s)
	}
	if !strings.Contains(s, "+set g_skipIntroCinematics 1") {
		t.Errorf("want skip-cutscenes: %s", s)
	}
	if !strings.Contains(s, "+map t2_trip") {
		t.Errorf("want explicit map: %s", s)
	}
	if !strings.HasSuffix(s, "+set r_mode -2") {
		t.Errorf("extra args must come last verbatim: %s", s)
	}
}

func TestArgsHostUsesDefaultPort(t *testing.T) {
	got, err := Args(testPlatform(), &Options{Mode: Host, Fullscreen: true})
	if err != nil {
		t.Fatal(err)
	}
	s := joined(got)
	if !strings.Contains(s, "+set net_enabled 1") {
		t.Errorf("host must enable net: %s", s)
	}
	if !strings.Contains(s, "+set net_port "+itoa(install.DefaultPort)) {
		t.Errorf("host must use default port: %s", s)
	}
	if !strings.Contains(s, "+map "+install.DefaultMap) {
		t.Errorf("host must load a map: %s", s)
	}
}

func TestArgsHostCustomPort(t *testing.T) {
	got, _ := Args(testPlatform(), &Options{Mode: Host, Port: 30000})
	if !strings.Contains(joined(got), "+set net_port 30000") {
		t.Errorf("host must honor custom port: %s", joined(got))
	}
}

func TestArgsJoinAddsDefaultPort(t *testing.T) {
	got, err := Args(testPlatform(), &Options{Mode: Join, Connect: "10.0.0.2"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(joined(got), "+connect 10.0.0.2:"+itoa(install.DefaultPort)) {
		t.Errorf("join must append default port: %s", joined(got))
	}
}

func TestArgsJoinKeepsExplicitPort(t *testing.T) {
	got, _ := Args(testPlatform(), &Options{Mode: Join, Connect: "10.0.0.2:40000"})
	if !strings.Contains(joined(got), "+connect 10.0.0.2:40000") {
		t.Errorf("join must keep explicit port: %s", joined(got))
	}
}

func TestArgsJoinRequiresAddress(t *testing.T) {
	if _, err := Args(testPlatform(), &Options{Mode: Join}); err == nil {
		t.Fatal("join without an address must error")
	}
}

// itoa avoids importing strconv just for the tests.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
