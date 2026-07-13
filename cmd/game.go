package cmd

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/Benehiko/jedi-outcast-coop/internal/config"
)

func newGameCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "game",
		Short: "Game Settings — mouse, aim, blaster, cutscenes",
		Long: "Edit the Game Settings in your config (mouse sensitivity, blaster bolt\n" +
			"speed, aim assist, crosshair, cutscene skip). These are all runtime\n" +
			"cvars: they take effect on the next launch, no rebuild needed. Saving\n" +
			"writes ~/.config/jk2coop/config.toml and refreshes the game's autoexec.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			rows := []config.Row{
				config.NewFloatRow("Mouse sensitivity",
					"Base look speed. Lower is calmer on a high-DPI mouse.",
					false, &cfg.Game.Sensitivity, 0.1, 5.0, 0.1),
				config.NewIntRow("Blaster speed",
					"Primary blaster bolt velocity (retail 2300; higher = faster, harder to dodge).",
					false, &cfg.Game.BlasterVelocity, 800, 6000, 100, ""),
				config.NewBoolRow("Aim assist",
					"Legacy JK2 saber auto-aim and FOV-linked mouse feel. Off = modern free aim.",
					false, &cfg.Game.AimAssist),
				config.NewBoolRow("Dynamic crosshair",
					"Legacy crosshair that drifts with the weapon. Off = fixed screen-center dot.",
					false, &cfg.Game.DynamicCrosshair),
				config.NewBoolRow("Skip cutscenes",
					"Auto-skip scripted map-intro cutscenes.",
					false, &cfg.Game.SkipCutscenes),
			}

			res, err := runForm("jedi outcast co-op · game", "Game Settings", rows)
			if err != nil {
				return err
			}
			if !res.Confirmed {
				cmd.Println("cancelled; no changes made.")
				return nil
			}
			if !res.Changed {
				cmd.Println("no changes.")
				return nil
			}
			path, err := cfg.Save()
			if err != nil {
				return err
			}
			cmd.Printf("saved %s\n", path)
			refreshAutoexec(cmd, cfg)
			return nil
		},
	}
	return cmd
}

// runForm runs a settings form and returns the user's decision.
func runForm(eyebrow, title string, rows []config.Row) (config.FormResult, error) {
	m := config.NewForm(eyebrow, title, rows)
	final, err := tea.NewProgram(m).Run()
	if err != nil {
		return config.FormResult{}, fmt.Errorf("running settings form: %w", err)
	}
	rr, ok := final.(interface{ RunResult() config.FormResult })
	if !ok {
		return config.FormResult{}, fmt.Errorf("settings form returned unexpected model")
	}
	return rr.RunResult(), nil
}
