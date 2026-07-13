package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Benehiko/jedi-outcast-coop/internal/install"
	"github.com/Benehiko/jedi-outcast-coop/internal/project"
)

func newInstallCmd() *cobra.Command {
	var (
		repo, buildDir, gamedata          string
		uninstall, all, noOptional, yes   bool
		withWide, withTextures, withUpscl bool
		combat, sensitivity               string
		skipCutscenes, noSkipCutscenes    bool
	)

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Stage the co-op engine data dir and install the launchers",
		Long: "Stage the engine data directory (symlinks to the retail assets and the\n" +
			"built co-op gamecode) and install the jk2coop-host / jk2coop-join launchers.\n\n" +
			"It never copies or modifies retail files. --uninstall removes exactly what\n" +
			"was installed, and re-running is idempotent.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := project.Root(repo)
			if err != nil {
				return err
			}
			if buildDir == "" {
				buildDir = install.EnvOr("JK2_BUILD", filepath.Join(root, "openjk", "build"))
			}

			if combat != install.CombatModern && combat != install.CombatClassic {
				return fmt.Errorf("--combat must be %q or %q (got: %q)", install.CombatModern, install.CombatClassic, combat)
			}
			if !numberRe.MatchString(sensitivity) {
				return fmt.Errorf("--sensitivity must be a non-negative number (got: %q)", sensitivity)
			}

			p := install.DetectPlatform(buildDir)
			opts := &install.Options{
				RepoRoot:      root,
				BuildDir:      buildDir,
				GameData:      gamedata,
				Widescreen:    resolveState(all, noOptional, withWide),
				Textures:      resolveState(all, noOptional, withTextures),
				Upscale:       resolveState(all, noOptional, withUpscl),
				Combat:        combat,
				Sensitivity:   sensitivity,
				SkipCutscenes: resolveCutscenes(noOptional, skipCutscenes, noSkipCutscenes),
				AssumeYes:     yes,
				Out:           cmd.OutOrStdout(),
				Prompt:        stdinPrompt(cmd),
			}

			if uninstall {
				return install.Uninstall(p, opts)
			}
			return install.Install(cmd.Context(), p, opts)
		},
	}

	f := cmd.Flags()
	f.StringVar(&repo, "repo", "", "repository root (default: autodetect from cwd)")
	f.StringVar(&buildDir, "build", "", "OpenJK CMake build dir (default: <repo>/openjk/build or $JK2_BUILD)")
	f.StringVar(&gamedata, "gamedata", "", "path to your Jedi Outcast GameData dir (default: Steam autodetect)")
	f.BoolVar(&uninstall, "uninstall", false, "remove everything this installer created")
	f.BoolVar(&withWide, "with-widescreen", false, "enable the widescreen/QHD/ultrawide video-menu mod")
	f.BoolVar(&withTextures, "with-textures", false, "generate the AI material-texture pak (GPU + container)")
	f.BoolVar(&withUpscl, "with-upscale", false, "build the Real-ESRGAN hi-res texture pak (GPU + container)")
	f.BoolVar(&all, "all", false, "enable every optional mod")
	f.BoolVar(&noOptional, "no-optional", false, "skip all optional-mod prompts (core install only)")
	f.BoolVarP(&yes, "yes", "y", false, "assume \"yes\" to prompts (non-interactive)")
	f.StringVar(&combat, "combat", install.CombatModern, "combat feel: modern | classic")
	f.StringVar(&sensitivity, "sensitivity", install.DefaultSensitivity, "base mouse sensitivity for modern combat")
	f.BoolVar(&skipCutscenes, "skip-cutscenes", false, "auto-skip scripted map-intro cutscenes")
	f.BoolVar(&noSkipCutscenes, "no-skip-cutscenes", false, "never auto-skip cutscenes (suppress the prompt)")
	return cmd
}

// numberRe matches a non-negative decimal (used to validate --sensitivity).
var numberRe = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)

// resolveCutscenes maps the cutscene flags to an install.OptState. --no-optional
// and --no-skip-cutscenes force off; --skip-cutscenes forces on; else "ask".
func resolveCutscenes(noOptional, skip, noSkip bool) install.OptState {
	switch {
	case noOptional, noSkip:
		return install.OptNo
	case skip:
		return install.OptYes
	default:
		return install.OptAsk
	}
}

// resolveState maps the mutually-influencing flags to an install.OptState.
// --all/--no-optional win over an individual --with-*; otherwise --with-* forces
// yes and the default is "ask".
func resolveState(all, noOptional, with bool) install.OptState {
	switch {
	case noOptional:
		return install.OptNo
	case all || with:
		return install.OptYes
	default:
		return install.OptAsk
	}
}

// stdinPrompt returns a y/N prompt bound to stdin, or nil when stdin is not a
// terminal (so "ask" resolves to "no" non-interactively).
func stdinPrompt(cmd *cobra.Command) func(string) (bool, error) {
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return nil
	}
	reader := bufio.NewReader(os.Stdin)
	return func(question string) (bool, error) {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s [y/N] ", question)
		line, err := reader.ReadString('\n')
		if err != nil {
			return false, nil //nolint:nilerr // EOF/interrupt means "no"
		}
		ans := strings.ToLower(strings.TrimSpace(line))
		return ans == "y" || ans == "yes", nil
	}
}
