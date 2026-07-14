package textures

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Benehiko/jedi-outcast-coop/internal/pk3"
)

// genPy is the in-container generation script (diffusers + FLUX.1-schnell),
// embedded so the Go binary is self-contained — no sibling script file to ship.
//
//go:embed gen.py
var genPy string

// Default container images for the two GPU backends.
const (
	// DefaultROCmImage is the AMD/ROCm PyTorch image (default backend).
	DefaultROCmImage = "rocm/pytorch:latest"
	// DefaultCUDAImage is the NVIDIA/CUDA PyTorch image (--cuda backend).
	DefaultCUDAImage = "pytorch/pytorch:2.4.0-cuda12.1-cudnn9-runtime"
)

// pipInstall is the in-container dependency install run before gen.py, matching
// the shell tool. The base images ship torch; we add the diffusers stack.
const pipInstall = `python -m pip install --quiet --no-input "diffusers>=0.30" "transformers>=4.43" ` +
	`accelerate sentencepiece protobuf safetensors pillow optimum-quanto`

// DefaultManifest is the built-in generic, non-branded material set. Each entry
// becomes textures/generated/<name>.jpg. KEEP THESE NON-RECOGNISABLE: generic
// materials only, nothing recognisably from the game or the franchise, so the
// output stays clear of Lucasfilm/Disney marks and trade dress.
var DefaultManifest = []ManifestEntry{
	{"metal_panel_worn", "seamless tileable texture of a worn brushed metal wall panel, subtle scratches and grime, industrial, flat even lighting, photographic, no logos, no text"},
	{"metal_plate_riveted", "seamless tileable texture of a riveted steel plate, weathered gunmetal, evenly lit, photographic, no markings"},
	{"concrete_bare", "seamless tileable texture of bare cast concrete, fine surface pitting, neutral grey, flat lighting, photographic"},
	{"concrete_stained", "seamless tileable texture of stained concrete floor, faint cracks and water marks, matte, photographic"},
	{"rock_grey", "seamless tileable texture of rough grey stone rock surface, natural, evenly lit, photographic"},
	{"rust_heavy", "seamless tileable texture of heavily rusted corroded iron, orange-brown patina, photographic, no text"},
	{"metal_grating", "seamless tileable texture of a dark metal floor grating pattern, industrial, evenly lit, photographic"},
	{"sand_fine", "seamless tileable texture of fine desert sand, subtle ripples, warm neutral tone, flat lighting, photographic"},
	{"fabric_canvas", "seamless tileable texture of coarse grey canvas fabric weave, matte, evenly lit, photographic"},
	{"panel_scifi_plain", "seamless tileable texture of a plain matte industrial wall panel with shallow seams, neutral grey, no symbols, no lettering, photographic"},
}

// ManifestEntry is one texture to generate: a filename stem and a prompt.
type ManifestEntry struct {
	Name   string
	Prompt string
}

// ParseManifest reads a "name|prompt" manifest (one per line; # comments and
// blank lines ignored).
func ParseManifest(content string) ([]ManifestEntry, error) {
	var out []ManifestEntry
	for line := range strings.SplitSeq(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "|") {
			continue
		}
		name, prompt, _ := strings.Cut(line, "|")
		name, prompt = strings.TrimSpace(name), strings.TrimSpace(prompt)
		if name == "" || prompt == "" {
			continue
		}
		out = append(out, ManifestEntry{Name: name, Prompt: prompt})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("manifest has no 'name|prompt' lines")
	}
	return out, nil
}

func manifestText(entries []ManifestEntry) string {
	var b strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&b, "%s|%s\n", e.Name, e.Prompt)
	}
	return b.String()
}

// GenerateOptions configures BuildGeneratedPak.
type GenerateOptions struct {
	// OutPath is the pak to write (e.g. …/zzz-generated-textures.pk3).
	OutPath string
	// Manifest is the texture set; empty uses DefaultManifest.
	Manifest []ManifestEntry
	// Size is the square texture size (power of two; default 1024).
	Size int
	// Steps is the diffusion step count (default 4; schnell is a few-step model).
	Steps int
	// Seed is the base RNG seed (default 1).
	Seed int
	// ModelDir caches the downloaded model weights (persisted across runs).
	ModelDir string
	// Image overrides the container image; empty picks ROCm/CUDA by backend.
	Image string
	// CUDA selects the NVIDIA/CUDA backend instead of AMD/ROCm.
	CUDA bool
	// Runtime is "nerdctl"/"podman"; empty autodetects.
	Runtime string
	// HFToken is the Hugging Face token for the first (gated) weight download.
	HFToken string
	// Env holds extra KEY=VALUE tuning vars passed to the container (GEN_FP8, …).
	Env []string
	// WorkDir, when set, is used as the scratch dir; empty uses a temp dir.
	WorkDir string
	// Progress, when non-nil, receives status lines.
	Progress func(string)
}

// GenerateResult reports what BuildGeneratedPak produced.
type GenerateResult struct {
	OutPath  string
	Textures int
	Bytes    int64
}

func (o *GenerateOptions) progress(format string, a ...any) {
	if o.Progress != nil {
		o.Progress(fmt.Sprintf(format, a...))
	}
}

// BuildGeneratedPak generates the manifest's textures with FLUX.1-schnell in a
// container, snaps each to power-of-two, and packs them under
// textures/generated/ as JPEGs.
func BuildGeneratedPak(ctx context.Context, opts GenerateOptions) (*GenerateResult, error) {
	if opts.OutPath == "" {
		return nil, fmt.Errorf("output path is required")
	}
	if opts.Size == 0 {
		opts.Size = 1024
	}
	if opts.Size <= 0 || opts.Size&(opts.Size-1) != 0 {
		return nil, fmt.Errorf("size must be a power of two (got %d)", opts.Size)
	}
	if opts.Steps == 0 {
		opts.Steps = 4
	}
	if opts.Seed == 0 {
		opts.Seed = 1
	}
	manifest := opts.Manifest
	if len(manifest) == 0 {
		manifest = DefaultManifest
	}

	rt, err := ResolveRuntime(opts.Runtime)
	if err != nil {
		return nil, err
	}

	// Backend + GPU flags.
	image := opts.Image
	var gpuFlags []string
	if opts.CUDA {
		if image == "" {
			image = DefaultCUDAImage
		}
		gpuFlags = []string{"--gpus", "all"}
		opts.progress("backend: NVIDIA/CUDA (%s)", image)
	} else {
		if image == "" {
			image = DefaultROCmImage
		}
		if _, err := os.Stat("/dev/kfd"); err != nil {
			return nil, fmt.Errorf("no /dev/kfd — ROCm not available; use CUDA backend or install ROCm")
		}
		gpuFlags = []string{
			"--device", "/dev/kfd",
			"--device", "/dev/dri",
			"--security-opt", "seccomp=unconfined",
		}
		// --group-add keep-groups is a Podman-only convenience to inherit the
		// host's render/video groups; containerd/nerdctl rejects it.
		if rt == RuntimePodman {
			gpuFlags = append(gpuFlags, "--group-add", "keep-groups")
		}
		opts.progress("backend: AMD/ROCm (%s)", image)
	}

	modelDir := opts.ModelDir
	if modelDir == "" {
		home, _ := os.UserHomeDir()
		modelDir = filepath.Join(home, ".cache", "flux-schnell")
	}
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		return nil, err
	}
	opts.progress("model cache: %s (persisted; reused across runs)", modelDir)

	work := opts.WorkDir
	cleanup := func() {}
	if work == "" {
		tmp, err := os.MkdirTemp("", "jk2-generate-*")
		if err != nil {
			return nil, err
		}
		work = tmp
		cleanup = func() { _ = os.RemoveAll(tmp) }
	}
	defer cleanup()

	outRaw := filepath.Join(work, "out")
	manDir := filepath.Join(work, "manifest")
	for _, d := range []string{outRaw, manDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	if err := os.WriteFile(filepath.Join(manDir, "prompts.txt"), []byte(manifestText(manifest)), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(work, "gen.py"), []byte(genPy), 0o644); err != nil {
		return nil, err
	}

	// 1. Generate inside the container.
	opts.progress("starting model container (first run downloads FLUX.1-schnell weights ~ tens of GB)…")
	if err := runGenerateContainer(ctx, rt, image, gpuFlags, work, modelDir, opts); err != nil {
		return nil, err
	}

	// 2. PoT-snap each generated PNG to a JPEG under textures/generated/, pack.
	opts.progress("snapping to power-of-two and packing under textures/generated/…")
	builder := pk3.NewBuilder()
	potDir := filepath.Join(work, "pot")
	count, err := snapGenerated(outRaw, potDir, builder)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, fmt.Errorf("model produced no images")
	}

	opts.progress("packing → %s", opts.OutPath)
	if err := builder.Write(opts.OutPath); err != nil {
		return nil, err
	}
	fi, err := os.Stat(opts.OutPath)
	if err != nil {
		return nil, err
	}
	return &GenerateResult{OutPath: opts.OutPath, Textures: count, Bytes: fi.Size()}, nil
}

// snapGenerated walks the generated PNGs, snaps each to power-of-two, and writes
// it as a JPEG under textures/generated/<stem>.jpg, registering it with builder.
func snapGenerated(outRaw, potDir string, builder *pk3.Builder) (int, error) {
	count := 0
	err := filepath.WalkDir(outRaw, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || strings.ToLower(filepath.Ext(path)) != ".png" {
			return nil
		}
		rel, err := filepath.Rel(outRaw, path)
		if err != nil {
			return err
		}
		stem := filepath.ToSlash(strings.TrimSuffix(rel, filepath.Ext(rel)))
		img, err := DecodeFile(path)
		if err != nil {
			return err
		}
		snapped := SnapToPowerOfTwo(img)
		archivePath := "textures/generated/" + stem + ".jpg"
		out := filepath.Join(potDir, filepath.FromSlash(archivePath))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		if err := EncodeFile(out, snapped, FormatJPEG); err != nil {
			return err
		}
		builder.Add(archivePath, out)
		count++
		return nil
	})
	return count, err
}

// runGenerateContainer assembles the env and runs pip-install + gen.py inside
// the model container.
func runGenerateContainer(ctx context.Context, rt Runtime, image string, gpuFlags []string, work, modelDir string, opts GenerateOptions) error {
	env := []string{
		fmt.Sprintf("GEN_SIZE=%d", opts.Size),
		fmt.Sprintf("GEN_STEPS=%d", opts.Steps),
		fmt.Sprintf("GEN_SEED=%d", opts.Seed),
		"PYTORCH_HIP_ALLOC_CONF=expandable_segments:True",
		"PYTORCH_CUDA_ALLOC_CONF=expandable_segments:True",
	}
	// Pass through GPU-tuning vars from the host environment, matching the shell
	// tool's defaults where the container script does not already default them.
	env = append(env, hostEnvDefaults()...)
	env = append(env, opts.Env...)
	if opts.HFToken != "" {
		env = append(env,
			"HF_TOKEN="+opts.HFToken,
			"HUGGING_FACE_HUB_TOKEN="+opts.HFToken)
		opts.progress("passing HF_TOKEN to the container for the gated model download")
	} else {
		opts.progress("no HF_TOKEN set — if the model is gated the download will 401 (see docs/asset-generation.md)")
	}

	run := ContainerRun{
		Runtime:    rt,
		Image:      image,
		ExtraFlags: gpuFlags,
		Env:        env,
		Mounts: []string{
			work + ":/work",
			modelDir + ":/models",
		},
		Entrypoint: []string{"bash", "-lc", "set -e\n" + pipInstall + "\npython /work/gen.py"},
	}
	return run.Run(ctx)
}

// hostEnvDefaults forwards the GPU-tuning env vars the shell tool honoured,
// carrying the same defaults, so behaviour matches for existing users.
func hostEnvDefaults() []string {
	defaults := []struct {
		key, def string
	}{
		{"GEN_VRAM_MODE", "auto"},
		{"GEN_DTYPE", "bf16"},
		{"GEN_ATTN", "math"},
		{"GEN_VAE_FP32", "1"},
		{"GEN_FP8", "0"},
		{"GEN_FP8_TE", "1"},
		{"GEN_FP8_QTYPE", "qfloat8"},
		{"TORCH_BLAS_PREFER_HIPBLASLT", "0"},
	}
	out := make([]string, 0, len(defaults)+2)
	for _, d := range defaults {
		v := os.Getenv(d.key)
		if v == "" {
			v = d.def
		}
		out = append(out, d.key+"="+v)
	}
	// Optional pass-throughs with no default (only forwarded when set).
	for _, k := range []string{"HF_HUB_OFFLINE", "HSA_OVERRIDE_GFX_VERSION"} {
		if v := os.Getenv(k); v != "" {
			out = append(out, k+"="+v)
		}
	}
	return out
}

// GPUAvailable reports whether a container runtime and a GPU device are present
// for the generate/upscale pipelines (Linux only). Mirrors the installer's
// haveGPUContainer check so callers can gate cleanly.
func GPUAvailable() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if _, err := DetectRuntime(); err != nil {
		return false
	}
	_, err := os.Stat("/dev/kfd")
	return err == nil
}
