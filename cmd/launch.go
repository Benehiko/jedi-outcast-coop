package cmd

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Benehiko/jedi-outcast-coop/internal/config"
	"github.com/Benehiko/jedi-outcast-coop/internal/install"
	"github.com/Benehiko/jedi-outcast-coop/internal/launch"
	"github.com/Benehiko/jedi-outcast-coop/internal/project"
)

// launchParams are the resolved options shared by the launch/host/join commands.
type launchParams struct {
	repo, buildDir string
	mapName        string
	port           int
	windowed       bool
	printOnly      bool
}

// runLaunch resolves the platform + config and runs (or prints) the engine for
// the given mode and connect address.
func runLaunch(cmd *cobra.Command, lp *launchParams, mode launch.Mode, connect string, extra []string) error {
	root, err := project.Root(lp.repo)
	if err != nil {
		return err
	}
	buildDir := lp.buildDir
	if buildDir == "" {
		buildDir = install.EnvOr("JK2_BUILD", filepath.Join(root, "openjk", "build"))
	}

	// Refresh the installed autoexec from the config so any settings change made
	// since the last install takes effect this launch.
	if cfg, cerr := config.Load(); cerr == nil {
		refreshAutoexec(cmd, cfg)
	}

	p := install.DetectPlatform(buildDir)
	opts := &launch.Options{
		Mode:          mode,
		BuildDir:      buildDir,
		Map:           lp.mapName,
		Connect:       connect,
		Port:          lp.port,
		ForceWindowed: lp.windowed,
		Extra:         extra,
	}

	if lp.printOnly {
		bin, a, err := launch.Resolve(p, opts)
		if err != nil {
			return notBuiltHint(err)
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), quoteCmd(bin, a))
		return nil
	}
	return notBuiltHint(launch.Run(p, opts))
}

// addLaunchFlags wires the shared flags onto a command.
func addLaunchFlags(cmd *cobra.Command, lp *launchParams) {
	f := cmd.Flags()
	f.StringVar(&lp.repo, "repo", "", "repository root (default: autodetect from cwd)")
	f.StringVar(&lp.buildDir, "build", "", "OpenJK CMake build dir (default: <repo>/openjk/build or $JK2_BUILD)")
	f.StringVar(&lp.mapName, "map", "", "map to load (default: "+install.DefaultMap+")")
	f.IntVar(&lp.port, "port", 0, "UDP port for hosting (default: "+fmt.Sprint(install.DefaultPort)+")")
	f.BoolVar(&lp.windowed, "windowed", false, "run windowed instead of fullscreen")
	f.BoolVar(&lp.printOnly, "print", false, "print the resolved engine command instead of running it")
}

// newLaunchCmd is the umbrella launch verb. Bare `launch` hosts a co-op game
// (the default); --join connects to one; --solo runs single-player.
func newLaunchCmd() *cobra.Command {
	var lp launchParams
	var join string
	var solo bool

	cmd := &cobra.Command{
		Use:   "launch [-- engine-args...]",
		Short: "Launch the game (hosts co-op by default)",
		Long: "Run the staged co-op engine. With no flags it hosts a co-op game others\n" +
			"can join. Use --join to connect to a host, or --solo for single-player.\n\n" +
			"  jk2coop launch                    host a co-op game (default)\n" +
			"  jk2coop launch --join HOST[:PORT] join a co-op game\n" +
			"  jk2coop launch --solo             single-player\n\n" +
			"Anything after `--` is passed to the engine verbatim.",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if solo && join != "" {
				return fmt.Errorf("--solo and --join are mutually exclusive")
			}
			if join != "" && lp.mapName != "" {
				return fmt.Errorf("--map does not apply when joining (the host picks the map)")
			}
			mode := launch.Host
			switch {
			case solo:
				mode = launch.SinglePlayer
			case join != "":
				mode = launch.Join
			}
			return runLaunch(cmd, &lp, mode, join, args)
		},
	}
	addLaunchFlags(cmd, &lp)
	cmd.Flags().StringVar(&join, "join", "", "join a co-op game at HOST[:PORT]")
	cmd.Flags().BoolVar(&solo, "solo", false, "run single-player instead of hosting")
	return cmd
}

// newHostCmd is the explicit host verb.
func newHostCmd() *cobra.Command {
	var lp launchParams
	cmd := &cobra.Command{
		Use:   "host [-- engine-args...]",
		Short: "Host a co-op game others can join",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLaunch(cmd, &lp, launch.Host, "", args)
		},
	}
	addLaunchFlags(cmd, &lp)
	return cmd
}

// newJoinCmd is the explicit join verb: `jk2coop join <HOST[:PORT]>`.
func newJoinCmd() *cobra.Command {
	var lp launchParams
	cmd := &cobra.Command{
		Use:   "join <HOST[:PORT]> [-- engine-args...]",
		Short: "Join a co-op game by IP",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if lp.mapName != "" {
				return fmt.Errorf("--map does not apply when joining (the host picks the map)")
			}
			return runLaunch(cmd, &lp, launch.Join, args[0], args[1:])
		},
	}
	addLaunchFlags(cmd, &lp)
	return cmd
}

// notBuiltHint augments a "not built" error with setup guidance, and passes any
// other error (or nil) through unchanged.
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
