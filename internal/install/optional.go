package install

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Benehiko/jedi-outcast-coop/internal/paks"
	"github.com/Benehiko/jedi-outcast-coop/internal/textures"
)

// zz/zzz override paks the optional mods produce, relative to base/. Listed so
// installOptionalMods can remove a pak that the config no longer wants.
var optionalPaks = []string{
	"zz-widescreen-menu.pk3",
	"zzz-generated-textures.pk3",
	"zzz-hires-textures.pk3",
}

// installOptionalMods applies the optional game-file mods the config asks for
// (widescreen menu, generated/upscaled textures) and removes any it no longer
// wants. Each just adds a zz… override pak to base/; none touch retail data.
func installOptionalMods(ctx context.Context, man *Manifest, opts *Options, gamedata, baseDir string) error {
	cfg := opts.Config

	// --- Autoexec cvars (combat feel, sensitivity, blaster, MSAA, cutscenes) --
	// Always regenerated from the config; the engine execs it at startup so it
	// wins over any stale openjo_sp.cfg.
	if err := writeAutoexec(man, opts, baseDir); err != nil {
		return err
	}

	// --- Widescreen video-menu pak (companion to the 0023 engine patch) ------
	wsPak := filepath.Join(baseDir, "zz-widescreen-menu.pk3")
	if cfg.Graphics.Widescreen {
		opts.sayf("Enabling widescreen video-menu modes…")
		if _, err := paks.BuildWidescreen(baseDir, wsPak); err != nil {
			opts.warnf("widescreen video-menu modes not installed",
				fmt.Sprintf("%v", err),
				"the game still runs; retail video modes remain available.")
		} else {
			_ = man.Add(wsPak)
			opts.infof("installed zz-widescreen-menu.pk3 (SETUP > VIDEO > Video Mode)")
		}
	}

	upscale, generate := cfg.GPUPaks()

	// --- Generated AI material textures (GPU + container; Linux-only) ------
	if generate {
		txPak := filepath.Join(baseDir, "zzz-generated-textures.pk3")
		if textures.GPUAvailable() {
			opts.sayf("Generating AI material textures (this can take a while)…")
			_, err := textures.BuildGeneratedPak(ctx, textures.GenerateOptions{
				OutPath:  txPak,
				HFToken:  os.Getenv("HF_TOKEN"),
				Progress: func(s string) { opts.infof("%s", s) },
			})
			if err != nil {
				opts.warnf("AI texture generation failed — zzz-generated-textures.pk3 not installed",
					err.Error(),
					"scroll up for the tool's output; see docs/asset-generation.md",
					fmt.Sprintf("re-run by hand: jk2coop dev textures generate --out '%s'", txPak))
			} else {
				_ = man.Add(txPak)
				opts.infof("installed zzz-generated-textures.pk3")
			}
		} else {
			opts.infof("no GPU container runtime detected (need nerdctl/podman + /dev/kfd).")
			opts.infof("run it later on a suitable Linux machine:")
			opts.infof("    jk2coop dev textures generate --out '%s'", txPak)
		}
	}

	// --- Real-ESRGAN hi-res texture upscale (GPU + container; Linux-only) --
	if upscale {
		upPak := filepath.Join(baseDir, "zzz-hires-textures.pk3")
		assetsBase := filepath.Join(gamedata, "base")
		if textures.GPUAvailable() {
			opts.sayf("Upscaling retail textures with Real-ESRGAN (this can take a while)…")
			_, err := textures.BuildUpscaledPak(ctx, textures.UpscaleOptions{
				AssetsDir: assetsBase,
				OutPath:   upPak,
				Progress:  func(s string) { opts.infof("%s", s) },
			})
			if err != nil {
				opts.warnf("texture upscale failed — zzz-hires-textures.pk3 not installed",
					err.Error(),
					"scroll up for the tool's output; see docs/hires-textures.md",
					fmt.Sprintf("re-run by hand: jk2coop dev textures upscale --assets '%s' --out '%s'", assetsBase, upPak))
			} else {
				_ = man.Add(upPak)
				opts.infof("installed zzz-hires-textures.pk3")
			}
		} else {
			opts.infof("no GPU container runtime detected (need nerdctl/podman + /dev/kfd).")
			opts.infof("run it later on a suitable Linux machine:")
			opts.infof("    jk2coop dev textures upscale --assets '%s' --out '%s'", assetsBase, upPak)
		}
	}

	// --- Remove paks the config no longer wants ---------------------------
	wanted := map[string]bool{
		"zz-widescreen-menu.pk3":     cfg.Graphics.Widescreen,
		"zzz-generated-textures.pk3": generate,
		"zzz-hires-textures.pk3":     upscale,
	}
	for _, name := range optionalPaks {
		if wanted[name] {
			continue
		}
		p := filepath.Join(baseDir, name)
		if fileExists(p) {
			if err := os.Remove(p); err == nil {
				man.Forget(p)
				opts.infof("removed %s (disabled in config)", name)
			}
		}
	}
	return nil
}
