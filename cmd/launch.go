package cmd

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Benehiko/jedi-outcast-coop/internal/install"
	"github.com/Benehiko/jedi-outcast-coop/internal/launch"
	"github.com/Benehiko/jedi-outcast-coop/internal/project"
)

func newLaunchCmd() *cobra.Command {
	var (
		repo, buildDir string
		host           bool
		join           string
		mapName        string
		port           int
		windowed       bool
		skipCutscenes  bool
		printOnly      bool
	)

	cmd := &cobra.Command{
		Use:   "launch [-- engine-args...]",
		Short: "Run the staged co-op engine (single-player, host, or join)",
		Long: "Run the engine the installer staged, with fs_basepath pointed at the\n" +
			"installed data dir so it picks up the co-op gamecode, linked retail\n" +
			"assets, and your autoexec presets (combat + render).\n\n" +
			"Modes:\n" +
			"  jk2coop launch                       single-player (default map)\n" +
			"  jk2coop launch --map kejim_post      single-player, a specific map\n" +
			"  jk2coop launch --host                host a co-op game\n" +
			"  jk2coop launch --join HOST[:PORT]    join a co-op game\n\n" +
			"Anything after `--` is passed to the engine verbatim, e.g.\n" +
			"  jk2coop launch -- +set r_mode -2\n\n" +
			"On Unix the engine replaces this process (it keeps running under your\n" +
			"shell); on Windows it runs as a child and jk2coop waits for it.",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := project.Root(repo)
			if err != nil {
				return err
			}
			if buildDir == "" {
				buildDir = install.EnvOr("JK2_BUILD", filepath.Join(root, "openjk", "build"))
			}

			if host && join != "" {
				return fmt.Errorf("--host and --join are mutually exclusive")
			}

			mode := launch.SinglePlayer
			switch {
			case host:
				mode = launch.Host
			case join != "":
				mode = launch.Join
			}
			if join != "" && mapName != "" {
				return fmt.Errorf("--map does not apply when joining (the host picks the map)")
			}

			p := install.DetectPlatform(buildDir)
			opts := &launch.Options{
				Mode:          mode,
				BuildDir:      buildDir,
				Map:           mapName,
				Connect:       join,
				Port:          port,
				Fullscreen:    !windowed,
				SkipCutscenes: skipCutscenes,
				Extra:         args,
			}

			if printOnly {
				bin, a, err := launch.Resolve(p, opts)
				if err != nil {
					return notBuiltHint(err)
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), quoteCmd(bin, a))
				return nil
			}

			return notBuiltHint(launch.Run(p, opts))
		},
	}

	f := cmd.Flags()
	f.StringVar(&repo, "repo", "", "repository root (default: autodetect from cwd)")
	f.StringVar(&buildDir, "build", "", "OpenJK CMake build dir (default: <repo>/openjk/build or $JK2_BUILD)")
	f.BoolVar(&host, "host", false, "host a co-op game others can join")
	f.StringVar(&join, "join", "", "join a co-op game at HOST[:PORT]")
	f.StringVar(&mapName, "map", "", "map to load (default: "+install.DefaultMap+")")
	f.IntVar(&port, "port", 0, "UDP port for --host (default: "+fmt.Sprint(install.DefaultPort)+")")
	f.BoolVar(&windowed, "windowed", false, "run windowed instead of fullscreen")
	f.BoolVar(&skipCutscenes, "skip-cutscenes", false, "auto-skip scripted map-intro cutscenes this run")
	f.BoolVar(&printOnly, "print", false, "print the resolved engine command instead of running it")
	return cmd
}

// notBuiltHint augments a "not built" error with setup guidance, and passes any
// other error (or nil) through unchanged. Run only returns on failure, so this
// never fires on a successful exec.
func notBuiltHint(err error) error {
	if errors.Is(err, launch.ErrEngineNotBuilt) {
		return fmt.Errorf("%w\n"+
			"       build the engine first (see docs/building.md), then `jk2coop install`;\n"+
			"       if it is built elsewhere, pass --build <dir>", err)
	}
	return err
}

// quoteCmd renders a command + args with minimal shell quoting for display.
func quoteCmd(bin string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, shellQuote(bin))
	for _, a := range args {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if strings.ContainsAny(s, " \t\"'$`") {
		return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	return s
}
