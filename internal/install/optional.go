package install

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/Benehiko/jedi-outcast-coop/internal/paks"
)

// installOptionalMods resolves and applies the optional game-file mods
// (widescreen, textures, upscale). Each just adds a zz… override pak to base/;
// none touch retail data.
func installOptionalMods(ctx context.Context, man *Manifest, opts *Options, gamedata, baseDir string) error {
	any := false

	// --- Modern combat feel + optional cutscene skip ----------------------
	// Always writes autoexec_sp.cfg (the engine execs it at startup) so the
	// combat cvars win over any stale openjo_sp.cfg. --combat classic restores
	// the legacy feel; cutscene auto-skip is a separate opt-in.
	if err := writeCombatConfig(man, opts, baseDir); err != nil {
		return err
	}
	any = true

	// --- Widescreen / QHD / ultrawide video-menu modes --------------------
	yes, err := opts.resolveOpt(opts.Widescreen,
		"Add widescreen / QHD / ultrawide / 4K resolutions to the video menu?")
	if err != nil {
		return err
	}
	if yes {
		any = true
		wsPak := filepath.Join(baseDir, "zz-widescreen-menu.pk3")
		opts.sayf("Enabling widescreen video-menu modes…")
		if _, err := paks.BuildWidescreen(baseDir, wsPak); err != nil {
			opts.infof("widescreen build failed: %v", err)
		} else {
			_ = man.Add(wsPak)
			opts.infof("installed zz-widescreen-menu.pk3 (SETUP > VIDEO > Video Mode)")
		}
	}

	// --- Generated AI material textures (GPU + container; Linux-only) ------
	yes, err = opts.resolveOpt(opts.Textures,
		"Generate original AI material textures? (needs a Linux GPU + container)")
	if err != nil {
		return err
	}
	if yes {
		any = true
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
	yes, err = opts.resolveOpt(opts.Upscale,
		"Build a Real-ESRGAN hi-res texture override from your own retail textures? (needs a Linux GPU + container)")
	if err != nil {
		return err
	}
	if yes {
		any = true
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

	if !any {
		opts.infof("no optional mods selected.")
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
