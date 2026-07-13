// Package launch runs the staged co-op engine directly, replacing the
// jk2coop-host / jk2coop-join launcher scripts with an in-binary command.
//
// It runs the same engine binary the installer stages (resolved via
// install.Platform.ResolveEngine) with fs_basepath pointed at the installed
// data dir, so a `launch` picks up the co-op gamecode, the linked retail
// assets, and any autoexec_*.cfg (combat + render presets) already in place.
//
// On Unix it execs the engine in place (via syscall.Exec) so the game becomes
// the caller's process — it keeps running under the user's shell rather than as
// a short-lived child. On Windows there is no exec(2); Run spawns the engine and
// waits.
package launch

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Benehiko/jedi-outcast-coop/internal/install"
)

// Mode selects what kind of session to launch.
type Mode int

const (
	// SinglePlayer loads a map locally (the default).
	SinglePlayer Mode = iota
	// Host starts a co-op game others can join over UDP.
	Host
	// Join connects to a co-op host.
	Join
)

// Options configures a launch.
type Options struct {
	// Mode is single-player, host, or join.
	Mode Mode
	// BuildDir is the OpenJK CMake build dir holding the engine + renderer.
	BuildDir string
	// Map is the map to load (SinglePlayer/Host). Empty uses install.DefaultMap.
	Map string
	// Connect is the "host[:port]" address to join (Join mode).
	Connect string
	// Port is the UDP port for Host mode (0 uses install.DefaultPort).
	Port int
	// Fullscreen runs the engine fullscreen; when false it runs windowed.
	Fullscreen bool
	// SkipCutscenes sets g_skipIntroCinematics 1 for this run.
	SkipCutscenes bool
	// Extra are additional raw engine arguments appended verbatim (after the
	// generated ones), e.g. []string{"+set", "r_mode", "-2"}.
	Extra []string
}

// Args builds the engine command line (without the executable) for p/opts.
// Exposed for testing and for callers that want to print the command.
func Args(p install.Platform, opts *Options) ([]string, error) {
	args := []string{
		"+set", "fs_basepath", p.DataDir,
	}

	if opts.Fullscreen {
		args = append(args, "+set", "r_fullscreen", "1")
	} else {
		args = append(args, "+set", "r_fullscreen", "0")
	}
	if opts.SkipCutscenes {
		args = append(args, "+set", "g_skipIntroCinematics", "1")
	}

	switch opts.Mode {
	case SinglePlayer:
		args = append(args, "+map", mapOrDefault(opts.Map))
	case Host:
		port := opts.Port
		if port == 0 {
			port = install.DefaultPort
		}
		args = append(args,
			"+set", "net_enabled", "1",
			"+set", "net_port", strconv.Itoa(port),
			"+map", mapOrDefault(opts.Map),
		)
	case Join:
		if opts.Connect == "" {
			return nil, fmt.Errorf("join needs a host address")
		}
		addr := opts.Connect
		if !strings.Contains(addr, ":") {
			addr = fmt.Sprintf("%s:%d", addr, install.DefaultPort)
		}
		args = append(args, "+set", "net_enabled", "1", "+connect", addr)
	default:
		return nil, fmt.Errorf("unknown launch mode %d", opts.Mode)
	}

	args = append(args, opts.Extra...)
	return args, nil
}

func mapOrDefault(m string) string {
	if m == "" {
		return install.DefaultMap
	}
	return m
}

// ErrEngineNotBuilt is returned by Resolve/Run when no engine binary is found
// under the build dir. Callers can match it (errors.Is) to print setup guidance.
var ErrEngineNotBuilt = errors.New("engine not built")

// Resolve returns the engine binary to run and the full argument list, or an
// error if the engine is not built where BuildDir expects it. A missing engine
// wraps ErrEngineNotBuilt.
func Resolve(p install.Platform, opts *Options) (bin string, args []string, err error) {
	bin, _ = p.ResolveEngine(opts.BuildDir)
	if bin == "" || !fileExists(bin) {
		return "", nil, fmt.Errorf("%w in %s", ErrEngineNotBuilt, opts.BuildDir)
	}
	a, err := Args(p, opts)
	if err != nil {
		return "", nil, err
	}
	return bin, a, nil
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}
