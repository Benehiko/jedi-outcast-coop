package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/Benehiko/jedi-outcast-coop/internal/config"
	"github.com/Benehiko/jedi-outcast-coop/internal/gfx"
	"github.com/Benehiko/jedi-outcast-coop/internal/install"
	"github.com/Benehiko/jedi-outcast-coop/internal/project"
)

func newGfxCmd() *cobra.Command {
	var (
		repo, buildDir string
		set            []string
		printOnly      bool
		noBuild        bool
		noInstall      bool
	)

	cmd := &cobra.Command{
		Use:   "gfx",
		Short: "Choose which graphics features are built into the engine",
		Long: "Toggle the optional graphics features — modern combat, widescreen,\n" +
			"and render fidelity — as independent units, then reset the OpenJK\n" +
			"submodule, reapply the co-op base plus the selected features, rebuild\n" +
			"the engine, and reinstall.\n\n" +
			"With no flags it opens an interactive selector. For scripts, use\n" +
			"--set to choose non-interactively:\n" +
			"  jk2coop gfx --set widescreen=on --set render-fidelity=off\n\n" +
			"--print shows the current state and exits.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := project.Root(repo)
			if err != nil {
				return err
			}
			if buildDir == "" {
				buildDir = install.EnvOr("JK2_BUILD", filepath.Join(root, "openjk", "build"))
			}
			mgr := &gfx.Manager{
				Submodule:  filepath.Join(root, "openjk"),
				PatchesDir: filepath.Join(root, "patches"),
			}

			ctx := cmd.Context()
			current, err := mgr.Detect(ctx)
			if err != nil {
				return err
			}

			if printOnly {
				printState(cmd, current)
				return nil
			}

			// Decide the target selection: --set flags (non-interactive) or the TUI.
			var target map[string]bool
			var changed bool
			if len(set) > 0 {
				target, err = applySetFlags(current, set)
				if err != nil {
					return err
				}
				changed = differs(current, target)
			} else {
				res, rerr := runSelector(current)
				if rerr != nil {
					return rerr
				}
				if !res.Confirmed {
					cmd.Println("cancelled; no changes made.")
					return nil
				}
				target, changed = res.Selected, res.Changed
			}

			if !changed {
				cmd.Printf("no change — %s\n", gfx.SummaryLine(target))
				return nil
			}

			cmd.Printf("Applying: %s\n", gfx.SummaryLine(target))
			applied, err := mgr.Apply(ctx, target)
			if err != nil {
				return err
			}
			cmd.Printf("patched: co-op base + [%s]\n", strings.Join(applied, ", "))

			if noBuild {
				cmd.Println("skipped build (--no-build); run `cmake --build " + buildDir + "` then `jk2coop install`")
				return nil
			}
			cmd.Println("building engine…")
			if err := gfx.Build(ctx, filepath.Join(root, "openjk"), buildDir, cmd.OutOrStdout()); err != nil {
				return err
			}

			if noInstall {
				cmd.Println("skipped install (--no-install); run `jk2coop install` to stage it")
				return nil
			}
			cmd.Println("reinstalling…")
			return reinstall(ctx, root, buildDir, cmd)
		},
	}

	f := cmd.Flags()
	f.StringVar(&repo, "repo", "", "repository root (default: autodetect from cwd)")
	f.StringVar(&buildDir, "build", "", "OpenJK CMake build dir (default: <repo>/openjk/build or $JK2_BUILD)")
	f.StringArrayVar(&set, "set", nil, "set a feature non-interactively, e.g. --set widescreen=on (repeatable)")
	f.BoolVar(&printOnly, "print", false, "print the current feature state and exit")
	f.BoolVar(&noBuild, "no-build", false, "apply patches but do not rebuild")
	f.BoolVar(&noInstall, "no-install", false, "build but do not reinstall")
	return cmd
}

func printState(cmd *cobra.Command, state map[string]bool) {
	for _, ft := range gfx.Features {
		mark := "off"
		if state[ft.Key] {
			mark = "on"
		}
		cmd.Printf("  %-16s %-4s  %s\n", ft.Key, mark, ft.Desc)
	}
}

// applySetFlags applies "key=on|off" assignments onto a copy of the current
// state, validating keys and values.
func applySetFlags(current map[string]bool, set []string) (map[string]bool, error) {
	target := make(map[string]bool, len(current))
	for _, ft := range gfx.Features {
		target[ft.Key] = current[ft.Key]
	}
	for _, s := range set {
		key, val, ok := strings.Cut(s, "=")
		if !ok {
			return nil, fmt.Errorf("--set expects key=on|off (got: %q)", s)
		}
		key = strings.TrimSpace(key)
		if _, known := gfx.FeatureByKey(key); !known {
			return nil, fmt.Errorf("unknown feature %q (known: %s)", key, strings.Join(featureKeys(), ", "))
		}
		switch strings.ToLower(strings.TrimSpace(val)) {
		case "on", "1", "true", "yes":
			target[key] = true
		case "off", "0", "false", "no":
			target[key] = false
		default:
			return nil, fmt.Errorf("--set %s: value must be on or off (got: %q)", key, val)
		}
	}
	return target, nil
}

func featureKeys() []string {
	keys := make([]string, 0, len(gfx.Features))
	for _, ft := range gfx.Features {
		keys = append(keys, ft.Key)
	}
	sort.Strings(keys)
	return keys
}

func differs(a, b map[string]bool) bool {
	for _, ft := range gfx.Features {
		if a[ft.Key] != b[ft.Key] {
			return true
		}
	}
	return false
}

// runSelector runs the bubbletea TUI and returns the user's decision.
func runSelector(current map[string]bool) (gfx.Result, error) {
	m := gfx.NewModel(current)
	final, err := tea.NewProgram(m).Run()
	if err != nil {
		return gfx.Result{}, fmt.Errorf("running selector: %w", err)
	}
	rr, ok := final.(interface{ RunResult() gfx.Result })
	if !ok {
		return gfx.Result{}, fmt.Errorf("selector returned unexpected model")
	}
	return rr.RunResult(), nil
}

// reinstall re-stages the freshly built engine from the current config. This is
// the low-level `dev gfx` path; normal users go through `jk2coop graphics` +
// `jk2coop install`. Install is idempotent and config-driven.
func reinstall(ctx context.Context, root, buildDir string, cmd *cobra.Command) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	p := install.DetectPlatform(buildDir)
	opts := &install.Options{
		RepoRoot:  root,
		BuildDir:  buildDir,
		Config:    &cfg,
		AssumeYes: true,
		Out:       cmd.OutOrStdout(),
		Prompt:    stdinPrompt(cmd),
	}
	return install.Install(ctx, p, opts)
}
