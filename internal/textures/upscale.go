package textures

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Benehiko/jedi-outcast-coop/internal/pk3"
)

// DefaultUpscaleImage is the container image providing the
// realesrgan-ncnn-vulkan binary. Override with --image / UpscaleOptions.Image.
const DefaultUpscaleImage = "docker.io/utkuozbulak/realesrgan-ncnn-vulkan:latest"

// DefaultUpscaleModel is the photographic Real-ESRGAN model.
const DefaultUpscaleModel = "realesrgan-x4plus"

// UpscaleOptions configures BuildUpscaledPak. Only AssetsDir and OutPath are
// required; the rest default to the values the shell tool used.
type UpscaleOptions struct {
	// AssetsDir holds the retail assets*.pk3 (the user's own game files).
	AssetsDir string
	// OutPath is the override pak to write (e.g. …/zzz-hires-textures.pk3).
	OutPath string
	// Scale is the upscale factor, 2 or 4 (default 4).
	Scale int
	// Model is the Real-ESRGAN model name (default DefaultUpscaleModel).
	Model string
	// Image is the container image (default DefaultUpscaleImage).
	Image string
	// Runtime is "nerdctl"/"podman"; empty autodetects.
	Runtime string
	// Limit, when >0, processes only the first N texture entries (quick trial).
	Limit int
	// ForceCPU disables GPU passthrough for the neural pass.
	ForceCPU bool
	// Stub replaces the neural pass with a plain Catmull-Rom resize (no
	// container). Exercises the whole extract→PoT→pack pipeline for CI/dev.
	Stub bool
	// WorkDir, when set, is used as the scratch dir (kept by the caller);
	// empty creates and removes a temp dir.
	WorkDir string
	// Progress, when non-nil, receives human-readable status lines.
	Progress func(string)
}

// UpscaleResult reports what BuildUpscaledPak produced.
type UpscaleResult struct {
	OutPath  string
	Textures int   // hi-res textures packed
	Bytes    int64 // size of the output pak
}

// texEntry is one texture to process: its pak-relative path and the pak it
// resolves from (last pak wins, matching engine load order).
type texEntry struct {
	name string
	pak  string
}

func (o *UpscaleOptions) progress(format string, a ...any) {
	if o.Progress != nil {
		o.Progress(fmt.Sprintf(format, a...))
	}
}

// isRasterTexture reports whether a pak entry is a surface/model raster texture
// we upscale: under textures/ or models/, with a jpg/jpeg/tga/png extension.
// The 2D HUD, menus, fonts and lightmaps are deliberately left alone.
func isRasterTexture(name string) bool {
	lower := strings.ToLower(name)
	if !strings.HasPrefix(lower, "textures/") && !strings.HasPrefix(lower, "models/") {
		return false
	}
	switch filepath.Ext(lower) {
	case ".jpg", ".jpeg", ".tga", ".png":
		return true
	}
	return false
}

// BuildUpscaledPak runs the full upscale pipeline and writes the override pak.
func BuildUpscaledPak(ctx context.Context, opts UpscaleOptions) (*UpscaleResult, error) {
	if opts.Scale == 0 {
		opts.Scale = 4
	}
	if opts.Scale != 2 && opts.Scale != 4 {
		return nil, fmt.Errorf("scale must be 2 or 4 (got %d)", opts.Scale)
	}
	if opts.Model == "" {
		opts.Model = DefaultUpscaleModel
	}
	if opts.Image == "" {
		opts.Image = DefaultUpscaleImage
	}
	if opts.AssetsDir == "" {
		return nil, fmt.Errorf("assets dir is required")
	}
	if opts.OutPath == "" {
		return nil, fmt.Errorf("output path is required")
	}
	if fi, err := os.Stat(opts.AssetsDir); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("assets dir not found: %s", opts.AssetsDir)
	}

	// Resolve the container runtime up front (unless stubbing) so we fail fast
	// before doing the expensive extraction.
	var rt Runtime
	if !opts.Stub {
		r, err := ResolveRuntime(opts.Runtime)
		if err != nil {
			return nil, err
		}
		rt = r
	}

	paks, err := filepath.Glob(filepath.Join(opts.AssetsDir, "assets*.pk3"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paks)
	if len(paks) == 0 {
		return nil, fmt.Errorf("no assets*.pk3 in %s — point --assets at your retail base/", opts.AssetsDir)
	}
	opts.progress("found %d retail pak(s) in %s", len(paks), opts.AssetsDir)

	// Scratch layout.
	work := opts.WorkDir
	cleanup := func() {}
	if work == "" {
		tmp, err := os.MkdirTemp("", "jk2-upscale-*")
		if err != nil {
			return nil, err
		}
		work = tmp
		cleanup = func() { _ = os.RemoveAll(tmp) }
	}
	defer cleanup()
	inDir := filepath.Join(work, "in") // PNG-normalised model inputs
	upDir := filepath.Join(work, "up") // upscaler output PNGs
	for _, d := range []string{inDir, upDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}

	// 1. Index the raster texture entries across every pak (last pak wins).
	entries, err := indexTextures(paks)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no textures found in the paks — are these valid JK2 assets?")
	}
	if opts.Limit > 0 && opts.Limit < len(entries) {
		opts.progress("found %d texture entries; limiting to the first %d", len(entries), opts.Limit)
		entries = entries[:opts.Limit]
	} else {
		opts.progress("found %d texture entries to upscale", len(entries))
	}

	// 2. Extract + normalise each entry to a PNG under inDir, mirroring its
	//    relative path (extensionless). Also remember the original extension so
	//    step 4 can restore it.
	opts.progress("normalising %d textures to PNG…", len(entries))
	origExt := make(map[string]string, len(entries)) // relStem -> original ext
	if err := extractAndNormalise(paks, entries, inDir, origExt); err != nil {
		return nil, err
	}
	if len(origExt) == 0 {
		return nil, fmt.Errorf("PNG normalisation produced no files")
	}

	// 3. Upscale: neural container, or a plain resize when stubbing.
	if opts.Stub {
		opts.progress("STUB upscale: plain %dx Catmull-Rom resize (not Real-ESRGAN)", opts.Scale)
		if err := stubUpscale(inDir, upDir, opts.Scale); err != nil {
			return nil, err
		}
	} else {
		opts.progress("upscaling with Real-ESRGAN model %q at %dx (the slow part)…", opts.Model, opts.Scale)
		if err := runUpscaleContainer(ctx, rt, opts, inDir, upDir); err != nil {
			return nil, err
		}
	}

	// 4. PoT-snap each upscaled PNG and write it back at the original path and
	//    extension, then pack.
	opts.progress("snapping to power-of-two and restoring original formats…")
	builder := pk3.NewBuilder()
	potDir := filepath.Join(work, "pot")
	count, err := snapAndCollect(upDir, potDir, origExt, builder)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, fmt.Errorf("power-of-two snap produced no files")
	}
	opts.progress("prepared %d hi-res textures", count)

	opts.progress("packing override pak → %s", opts.OutPath)
	if err := builder.Write(opts.OutPath); err != nil {
		return nil, err
	}
	fi, err := os.Stat(opts.OutPath)
	if err != nil {
		return nil, err
	}
	return &UpscaleResult{OutPath: opts.OutPath, Textures: count, Bytes: fi.Size()}, nil
}

// indexTextures builds the ordered, de-duplicated list of raster texture
// entries across paks. Later paks win (keep the last occurrence's source pak),
// matching engine load order. First-seen order is preserved for stable output.
func indexTextures(paks []string) ([]texEntry, error) {
	seen := make(map[string]int) // name -> index in order
	var order []texEntry
	for _, pak := range paks {
		r, err := pk3.Open(pak)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", filepath.Base(pak), err)
		}
		for _, name := range r.Names() {
			if !isRasterTexture(name) {
				continue
			}
			if i, ok := seen[name]; ok {
				order[i].pak = pak // later pak wins
				continue
			}
			seen[name] = len(order)
			order = append(order, texEntry{name: name, pak: pak})
		}
		_ = r.Close()
	}
	return order, nil
}

// extractAndNormalise reads each entry from its resolved pak, decodes it, and
// writes a PNG under inDir at the entry's relative path with a .png extension.
// origExt records relStem -> original extension for the format-restore step.
func extractAndNormalise(paks []string, entries []texEntry, inDir string, origExt map[string]string) error {
	// Group entries by source pak to open each pak once.
	byPak := make(map[string][]string)
	for _, e := range entries {
		byPak[e.pak] = append(byPak[e.pak], e.name)
	}
	for _, pak := range paks {
		names := byPak[pak]
		if len(names) == 0 {
			continue
		}
		r, err := pk3.Open(pak)
		if err != nil {
			return err
		}
		for _, name := range names {
			data, err := r.ReadFile(name)
			if err != nil {
				continue // skip unreadable member
			}
			img, err := DecodeImage(name, data)
			if err != nil {
				continue // skip undecodable texture rather than abort the run
			}
			stem := strings.TrimSuffix(name, filepath.Ext(name))
			origExt[stem] = strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
			out := filepath.Join(inDir, filepath.FromSlash(stem)+".png")
			if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
				_ = r.Close()
				return err
			}
			if err := EncodeFile(out, img, FormatPNG); err != nil {
				_ = r.Close()
				return err
			}
		}
		_ = r.Close()
	}
	return nil
}

// stubUpscale replaces the neural pass with a plain scale-factor resize of
// every PNG under inDir into upDir, mirroring paths.
func stubUpscale(inDir, upDir string, scale int) error {
	return filepath.WalkDir(inDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || strings.ToLower(filepath.Ext(path)) != ".png" {
			return nil
		}
		img, err := DecodeFile(path)
		if err != nil {
			return err
		}
		b := img.Bounds()
		up := Resize(img, b.Dx()*scale, b.Dy()*scale)
		rel, err := filepath.Rel(inDir, path)
		if err != nil {
			return err
		}
		out := filepath.Join(upDir, rel)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		return EncodeFile(out, up, FormatPNG)
	})
}

// snapAndCollect walks the upscaled PNGs, snaps each to power-of-two, restores
// the original extension/format (from origExt), writes it under potDir, and
// registers it with the pak builder. Returns the count packed.
func snapAndCollect(upDir, potDir string, origExt map[string]string, builder *pk3.Builder) (int, error) {
	count := 0
	err := filepath.WalkDir(upDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || strings.ToLower(filepath.Ext(path)) != ".png" {
			return nil
		}
		rel, err := filepath.Rel(upDir, path)
		if err != nil {
			return err
		}
		stem := filepath.ToSlash(strings.TrimSuffix(rel, filepath.Ext(rel)))
		ext, ok := origExt[stem]
		if !ok {
			return nil // no known original for this output; skip
		}
		img, err := DecodeFile(path)
		if err != nil {
			return err
		}
		snapped := SnapToPowerOfTwo(img)
		archivePath := stem + "." + ext
		out := filepath.Join(potDir, filepath.FromSlash(archivePath))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		if err := EncodeFile(out, snapped, FormatForExt(ext)); err != nil {
			return err
		}
		builder.Add(archivePath, out)
		count++
		return nil
	})
	return count, err
}

// runUpscaleContainer runs realesrgan-ncnn-vulkan in batch mode over inDir,
// writing PNGs to upDir. It assembles GPU passthrough (a DRI render node,
// unless ForceCPU) the same way the shell tool did.
func runUpscaleContainer(ctx context.Context, rt Runtime, opts UpscaleOptions, inDir, upDir string) error {
	run := ContainerRun{
		Runtime: rt,
		Image:   opts.Image,
		Mounts: []string{
			inDir + ":/in:ro",
			upDir + ":/out",
		},
		// realesrgan-ncnn-vulkan: -i dir -o dir -n model -s scale -f png.
		Args: []string{
			"-i", "/in",
			"-o", "/out",
			"-n", opts.Model,
			"-s", fmt.Sprintf("%d", opts.Scale),
			"-f", "png",
		},
	}
	if !opts.ForceCPU && hasRenderNode() {
		run.ExtraFlags = []string{"--device", "/dev/dri:/dev/dri"}
		opts.progress("GPU render node found — enabling Vulkan acceleration")
	} else {
		opts.progress("running Real-ESRGAN on CPU (no GPU passthrough) — this is slow")
	}
	return run.Run(ctx)
}
