package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Benehiko/jedi-outcast-coop/internal/config"
	"github.com/Benehiko/jedi-outcast-coop/internal/install"
	"github.com/Benehiko/jedi-outcast-coop/internal/progress"
	"github.com/Benehiko/jedi-outcast-coop/internal/textures"
)

// upscaleTier maps a resolution-tier pixel value (1024/2048/4096) to the
// Real-ESRGAN neural scale factor and the largest-side output cap, matching
// config.Config.UpscaleTier. An unset (0) value uses the default tier.
func upscaleTier(resolution int) (scale, maxSize int, err error) {
	switch resolution {
	case 0:
		resolution = config.DefaultTextureResolution
	case config.TextureResolution1K, config.TextureResolution2K, config.TextureResolution4K:
		// valid
	default:
		return 0, 0, fmt.Errorf("resolution must be 1024, 2048 or 4096 (got %d)", resolution)
	}
	scale, maxSize = config.Config{Graphics: config.Graphics{TextureResolution: resolution}}.UpscaleTier()
	return scale, maxSize, nil
}

func newTexturesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "textures",
		Short: "Build the optional GPU texture override paks (upscale, generate)",
	}
	cmd.AddCommand(newTexturesUpscaleCmd(), newTexturesGenerateCmd())
	return cmd
}

func newTexturesUpscaleCmd() *cobra.Command {
	var (
		assets, out, model, image, runtimeName string
		scale, limit, resolution               int
		forceCPU, stub, force                  bool
	)
	cmd := &cobra.Command{
		Use:   "upscale",
		Short: "Build a Real-ESRGAN hi-res override pak from your own retail assets",
		Long: "Read the retail assets*.pk3 from your own copy of the game, upscale the\n" +
			"world/model textures with a locally-run Real-ESRGAN model (in a container),\n" +
			"snap each to power-of-two, and write an override pak (zzz-hires-textures.pk3)\n" +
			"that the engine loads on top of your assets. Retail assets are never modified.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if assets == "" {
				assets = install.DefaultAssetsBase()
			}
			if out == "" {
				out = filepath.Join(assets, "zzz-hires-textures.pk3")
			}
			// The resolution tier drives the neural scale and the output cap; an
			// explicit --scale still overrides the scale the tier implies.
			tierScale, maxSize, err := upscaleTier(resolution)
			if err != nil {
				return err
			}
			if !cmd.Flags().Changed("scale") {
				scale = tierScale
			}
			pb := progress.New(cmd.OutOrStdout(), "  ")
			res, err := textures.BuildUpscaledPak(cmd.Context(), textures.UpscaleOptions{
				AssetsDir: assets,
				OutPath:   out,
				Scale:     scale,
				MaxSize:   maxSize,
				Model:     model,
				Image:     image,
				Runtime:   runtimeName,
				Limit:     limit,
				ForceCPU:  forceCPU,
				Stub:      stub,
				Force:     force,
				Progress: func(s string) {
					pb.Done() // close any open bar line before a status line
					cmd.Printf(">>> %s\n", s)
				},
				ProgressBar: pb.Update,
			})
			pb.Done()
			if err != nil {
				return err
			}
			cmd.Println()
			if res.Skipped {
				cmd.Printf(">>> up to date. %s (%s) already matches the current inputs — nothing to rebuild.\n", res.OutPath, humanBytes(res.Bytes))
				cmd.Println("    Pass --force to rebuild anyway.")
				return nil
			}
			cmd.Printf(">>> done. wrote %s (%s, %d textures)\n", res.OutPath, humanBytes(res.Bytes), res.Textures)
			cmd.Println("    The engine loads this pak on top of your retail assets automatically.")
			cmd.Printf("    To remove: rm %q\n", res.OutPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&assets, "assets", "", "directory with your retail assets*.pk3 (default: platform base dir)")
	cmd.Flags().StringVar(&out, "out", "", "output pak (default: <assets>/zzz-hires-textures.pk3)")
	cmd.Flags().IntVar(&resolution, "resolution", config.DefaultTextureResolution, "output resolution tier in pixels: 1024 (1K), 2048 (2K) or 4096 (4K)")
	cmd.Flags().IntVar(&scale, "scale", 4, "advanced: Real-ESRGAN neural factor (2 or 4); overrides the tier's default")
	cmd.Flags().BoolVar(&force, "force", false, "rebuild even if the existing pak already matches the current inputs")
	cmd.Flags().StringVar(&model, "model", textures.DefaultUpscaleModel, "Real-ESRGAN model (realesrgan-x4plus or realesr-animevideov3)")
	cmd.Flags().StringVar(&image, "image", "", "container image providing realesrgan-ncnn-vulkan (default: built-in)")
	cmd.Flags().StringVar(&runtimeName, "runtime", "", "container runtime: nerdctl or podman (default: autodetect)")
	cmd.Flags().IntVar(&limit, "limit", 0, "process only the first N textures (quick trial run)")
	cmd.Flags().BoolVar(&forceCPU, "cpu", false, "force CPU mode (skip GPU/Vulkan passthrough)")
	cmd.Flags().BoolVar(&stub, "stub-upscale", false, "TEST MODE: plain Catmull-Rom resize instead of Real-ESRGAN (no container)")
	return cmd
}

func newTexturesGenerateCmd() *cobra.Command {
	var (
		out, manifestPath, modelDir, image, runtimeName string
		size, steps, seed                               int
		useCUDA, dryRun                                 bool
	)
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate original, non-branded material textures with FLUX.1-schnell",
		Long: "Generate generic, non-branded surface materials (metal, concrete, rock, …)\n" +
			"with a locally-run FLUX.1-schnell model (in a container) and pack them under\n" +
			"textures/generated/ into zzz-generated-textures.pk3. FLUX.1-schnell is\n" +
			"Apache-2.0; the outputs are original works you may ship. This is NOT a Star\n" +
			"Wars asset generator — keep any added prompts to generic materials only.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if out == "" {
				out = filepath.Join(install.DefaultAssetsBase(), "zzz-generated-textures.pk3")
			}
			manifest := textures.DefaultManifest
			if manifestPath != "" {
				content, err := os.ReadFile(manifestPath)
				if err != nil {
					return err
				}
				parsed, err := textures.ParseManifest(string(content))
				if err != nil {
					return err
				}
				manifest = parsed
			}

			if dryRun {
				backend := "ROCm"
				if useCUDA {
					backend = "CUDA"
				}
				cmd.Printf(">>> settings: size=%d steps=%d seed=%d backend=%s\n", nonZero(size, 1024), nonZero(steps, 4), nonZero(seed, 1), backend)
				cmd.Printf(">>> would generate %d texture(s) into %s:\n", len(manifest), out)
				for _, e := range manifest {
					cmd.Printf("    textures/generated/%s.jpg  <-  %s\n", e.Name, e.Prompt)
				}
				return nil
			}

			res, err := textures.BuildGeneratedPak(cmd.Context(), textures.GenerateOptions{
				OutPath:  out,
				Manifest: manifest,
				Size:     size,
				Steps:    steps,
				Seed:     seed,
				ModelDir: modelDir,
				Image:    image,
				CUDA:     useCUDA,
				Runtime:  runtimeName,
				HFToken:  os.Getenv("HF_TOKEN"),
				Progress: func(s string) { cmd.Printf(">>> %s\n", s) },
			})
			if err != nil {
				return err
			}
			cmd.Println()
			cmd.Printf(">>> done. wrote %s (%s, %d textures under textures/generated/)\n", res.OutPath, humanBytes(res.Bytes), res.Textures)
			cmd.Printf("    Reference them from your own shaders/maps as textures/generated/<name>.\n")
			cmd.Printf("    To remove: rm %q\n", res.OutPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "output pak (default: <base>/zzz-generated-textures.pk3)")
	cmd.Flags().IntVar(&size, "size", 1024, "square texture size, power of two")
	cmd.Flags().IntVar(&steps, "steps", 4, "diffusion steps (FLUX.1-schnell is a few-step model)")
	cmd.Flags().IntVar(&seed, "seed", 1, "base RNG seed for reproducibility")
	cmd.Flags().StringVar(&manifestPath, "manifest", "", "a 'name|prompt' manifest to use instead of the built-in material set")
	cmd.Flags().StringVar(&modelDir, "model-dir", "", "where to cache the model weights (default: ~/.cache/flux-schnell)")
	cmd.Flags().StringVar(&image, "image", "", "container image with PyTorch + the chosen GPU backend (default: built-in)")
	cmd.Flags().BoolVar(&useCUDA, "cuda", false, "use the NVIDIA/CUDA backend instead of AMD/ROCm")
	cmd.Flags().StringVar(&runtimeName, "runtime", "", "container runtime: nerdctl or podman (default: autodetect)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the manifest and settings, generate nothing")
	return cmd
}

// humanBytes renders a byte count as a short human-readable size (e.g. "4.2M").
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%c", float64(n)/float64(div), "KMGTPE"[exp])
}

// nonZero returns v if non-zero, else def (for dry-run display defaults).
func nonZero(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}
