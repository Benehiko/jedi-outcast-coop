package cmd

import (
	"github.com/spf13/cobra"

	"github.com/Benehiko/jedi-outcast-coop/internal/config"
	"github.com/Benehiko/jedi-outcast-coop/internal/project"
)

func newGraphicsCmd() *cobra.Command {
	var repo, buildDir string

	cmd := &cobra.Command{
		Use:     "graphics",
		Aliases: []string{"gfx"},
		Short:   "Graphics Settings — widescreen, lighting, MSAA, textures",
		Long: "Edit the Graphics Settings in your config. MSAA is a runtime cvar (no\n" +
			"rebuild). Widescreen and lighting are built into the engine, so changing\n" +
			"them offers to rebuild + reinstall. Texture upscale/generate build\n" +
			"optional GPU paks on the next install.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := project.Root(repo)
			if err != nil {
				return err
			}

			cfg, err := config.Load()
			if err != nil {
				return err
			}

			rows := []config.Row{
				config.NewBoolRow("Widescreen",
					"16:9/21:9/32:9 aspect correction, extra video modes, edge-anchored HUD.",
					true, &cfg.Graphics.Widescreen),
				config.NewBoolRow("Lighting",
					"Software-overbright render fidelity plus a character-model lighting boost.",
					true, &cfg.Graphics.Lighting),
				config.NewEnumRow("MSAA",
					"Multisample anti-aliasing. Higher is smoother edges, more GPU cost.",
					false, &cfg.Graphics.MSAA,
					[]int{0, 2, 4, 8},
					map[int]string{0: "off", 2: "2x", 4: "4x", 8: "8x"}),
				config.NewBoolRow("Texture upscale",
					"Real-ESRGAN hi-res override built from your retail textures (GPU + container).",
					false, &cfg.Graphics.TextureUpscale),
				config.NewBoolRow("Texture generate",
					"Generated AI material textures (GPU + container).",
					false, &cfg.Graphics.TextureGenerate),
			}

			res, err := runForm("jedi outcast co-op · graphics", "Graphics Settings", rows)
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

			// MSAA takes effect via autoexec; refresh it either way.
			refreshAutoexec(cmd, cfg)

			// Widescreen / lighting are patch-backed: offer a rebuild now.
			if res.RebuildNeeded {
				return offerRebuild(cmd, root, buildDir, cfg)
			}
			cmd.Println("texture paks (if enabled) build on the next `jk2coop install`.")
			return nil
		},
	}

	f := cmd.Flags()
	f.StringVar(&repo, "repo", "", "repository root (default: autodetect from cwd)")
	f.StringVar(&buildDir, "build", "", "OpenJK CMake build dir (default: <repo>/openjk/build or $JK2_BUILD)")
	return cmd
}
