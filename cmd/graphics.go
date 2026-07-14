package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Benehiko/jedi-outcast-coop/internal/config"
	"github.com/Benehiko/jedi-outcast-coop/internal/gfxprobe"
	"github.com/Benehiko/jedi-outcast-coop/internal/install"
	"github.com/Benehiko/jedi-outcast-coop/internal/project"
)

// msaaLevels mirrors the sample counts offered by the MSAA row, ascending. It is
// the canonical candidate list owned by gfxprobe, aliased here for the row.
var msaaLevels = gfxprobe.MSAALevels

// msaaLabel renders a sample count the way the MSAA row does ("off", "8x").
func msaaLabel(n int) string {
	if n <= 0 {
		return "off"
	}
	return fmt.Sprintf("%dx", n)
}

// resolveBuildDir mirrors the launch/install default: an explicit --build wins,
// else $JK2_BUILD, else <repo>/openjk/build.
func resolveBuildDir(root, buildDir string) string {
	if buildDir != "" {
		return buildDir
	}
	return install.EnvOr("JK2_BUILD", filepath.Join(root, "openjk", "build"))
}

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

			// Bind resolution to a local pair the row mutates; copied back into cfg
			// before save. suggested is the detected native mode (zero if none).
			resSel := config.Resolution{W: cfg.Graphics.ResWidth, H: cfg.Graphics.ResHeight}
			suggested, _ := config.DetectMonitor()

			rows := []config.Row{
				config.NewBoolRow("Widescreen",
					"16:9/21:9/32:9 aspect correction, extra video modes, edge-anchored HUD.",
					true, &cfg.Graphics.Widescreen),
				config.NewBoolRow("Lighting",
					"Software-overbright render fidelity plus a character-model lighting boost.",
					true, &cfg.Graphics.Lighting),
				config.NewResolutionRow("Resolution",
					"Game resolution (r_customwidth/height). auto lets the engine pick; native matches your monitor.",
					&resSel, suggested),
				config.NewEnumRow("MSAA",
					"Multisample anti-aliasing. Higher is smoother edges, more GPU cost.",
					false, &cfg.Graphics.MSAA,
					append([]int{0}, msaaLevels...),
					map[int]string{0: "off", 2: "2x", 4: "4x", 8: "8x", 16: "16x"}),
				config.NewBoolRow("Fullscreen",
					"Run fullscreen. Off = windowed, the reliable choice on Wayland "+
						"where fullscreen mode enumeration is flaky.",
					false, &cfg.Graphics.Fullscreen),
				config.NewBoolRow("Texture upscale",
					"Real-ESRGAN hi-res override built from your retail textures (GPU + container).",
					false, &cfg.Graphics.TextureUpscale),
				config.NewEnumRow("Upscale resolution",
					"Output tier for texture upscale. Higher is sharper and larger; 1K is fastest.",
					false, &cfg.Graphics.TextureResolution,
					[]int{config.TextureResolution1K, config.TextureResolution2K, config.TextureResolution4K},
					map[int]string{
						config.TextureResolution1K: "1K",
						config.TextureResolution2K: "2K",
						config.TextureResolution4K: "4K",
					}),
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
			// Copy the resolution the row edited back into the config before saving.
			cfg.Graphics.ResWidth, cfg.Graphics.ResHeight = resSel.W, resSel.H

			// Guard against an MSAA level the GPU/driver can't realise: some
			// Mesa/Wayland setups fail eglChooseConfig at high sample counts,
			// which crashes the renderer at launch ("no display modes could be
			// found"). Probe the chosen level against the installed engine and
			// step down to the highest that works. Best-effort: if the engine
			// isn't built or can't be probed here, the user's choice is kept.
			buildDir = resolveBuildDir(root, buildDir)
			p := install.DetectPlatform(buildDir)
			if usable, changed := gfxprobe.ClampMSAA(
				cmd.Context(), p, buildDir, cfg.Graphics.MSAA,
			); changed {
				cmd.Printf("note: %s MSAA is unsupported on this GPU/driver; using %s instead.\n",
					msaaLabel(cfg.Graphics.MSAA), msaaLabel(usable))
				cfg.Graphics.MSAA = usable
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
