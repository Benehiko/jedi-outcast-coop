package install

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/Benehiko/jedi-outcast-coop/internal/paks"
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
			opts.infof("widescreen build failed: %v", err)
		} else {
			_ = man.Add(wsPak)
			opts.infof("installed zz-widescreen-menu.pk3 (SETUP > VIDEO > Video Mode)")
		}
	}

	upscale, generate := cfg.GPUPaks()

	// --- Generated AI material textures (GPU + container; Linux-only) ------
	if generate {
		txPak := filepath.Join(baseDir, "zzz-generated-textures.pk3")
		tool := filepath.Join(opts.RepoRoot, "tools", "generate-textures.sh")
		if haveGPUContainer() {
			opts.sayf("Generating AI material textures (this can take a while)…")
			if err := runTool(ctx, tool, "--out", txPak); err != nil {
				opts.infof("texture generation failed; see docs/asset-generation.md")
			} else {
				_ = man.Add(txPak)
				opts.infof("installed zzz-generated-textures.pk3")
			}
		} else {
			opts.infof("no GPU container runtime detected (need nerdctl/podman + /dev/kfd).")
			opts.infof("run it later on a suitable Linux machine:")
			opts.infof("    %s --out '%s'", tool, txPak)
		}
	}

	// --- Real-ESRGAN hi-res texture upscale (GPU + container; Linux-only) --
	if upscale {
		upPak := filepath.Join(baseDir, "zzz-hires-textures.pk3")
		tool := filepath.Join(opts.RepoRoot, "tools", "upscale-textures.sh")
		if haveGPUContainer() {
			opts.sayf("Upscaling retail textures with Real-ESRGAN (this can take a while)…")
			if err := runTool(ctx, tool, "--assets", filepath.Join(gamedata, "base"), "--out", upPak); err != nil {
				opts.infof("upscale failed; see docs/hires-textures.md")
			} else {
				_ = man.Add(upPak)
				opts.infof("installed zzz-hires-textures.pk3")
			}
		} else {
			opts.infof("no GPU container runtime detected (need nerdctl/podman + /dev/kfd).")
			opts.infof("run it later on a suitable Linux machine:")
			opts.infof("    %s --assets '%s/base' --out '%s'", tool, gamedata, upPak)
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

// haveGPUContainer reports whether a container runtime + AMD ROCm device is
// available for the GPU texture mods (Linux only).
func haveGPUContainer() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if !hasCmd("nerdctl") && !hasCmd("podman") {
		return false
	}
	_, err := os.Stat("/dev/kfd")
	return err == nil
}

func hasCmd(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// runTool invokes a still-shell texture tool, streaming its output. These GPU
// pipelines remain shell scripts pending a phase-2 Go port.
func runTool(ctx context.Context, tool string, args ...string) error {
	cmd := exec.CommandContext(ctx, tool, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
