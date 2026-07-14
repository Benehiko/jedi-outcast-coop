package textures

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Benehiko/jedi-outcast-coop/internal/pk3"
)

// DefaultUpscaleImage is the container image providing the
// realesrgan-ncnn-vulkan binary. It defaults to a local tag we build ourselves
// on first use (see LocalUpscaleImage / Containerfile.realesrgan) rather than a
// third-party registry image. Override with --image / UpscaleOptions.Image.
const DefaultUpscaleImage = LocalUpscaleImage

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
	// MaxSize, when >0, caps the largest side of each output texture to this many
	// pixels (before the power-of-two snap): a texture larger than MaxSize on its
	// long axis is proportionally downscaled first. 0 = no cap (keep the full
	// Scale× result). Lets callers offer 1K/2K/4K output tiers.
	MaxSize int
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
	// Force rebuilds even when an existing output pak's fingerprint already
	// matches the current inputs (which would otherwise be skipped).
	Force bool
	// WorkDir, when set, is used as the scratch dir (kept by the caller);
	// empty creates and removes a temp dir.
	WorkDir string
	// Progress, when non-nil, receives human-readable status lines.
	Progress func(string)
	// ProgressBar, when non-nil, receives per-phase progress counts (phase name,
	// items done, total). Callers render an in-place bar; total may be 0 for an
	// indeterminate phase. Called repeatedly for the same phase as work advances.
	ProgressBar func(phase string, done, total int)
}

// UpscaleResult reports what BuildUpscaledPak produced.
type UpscaleResult struct {
	OutPath  string
	Textures int   // hi-res textures packed
	Bytes    int64 // size of the output pak
	// Skipped is true when an up-to-date output pak already existed and the slow
	// pipeline was not re-run. Textures is 0 in that case (nothing repacked).
	Skipped bool
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

func (o *UpscaleOptions) bar(phase string, done, total int) {
	if o.ProgressBar != nil {
		o.ProgressBar(phase, done, total)
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

	// Checksum short-circuit: if an output pak already exists and its recorded
	// fingerprint matches the current inputs (paks + pipeline knobs), the slow
	// neural upscale would reproduce byte-for-byte the same result — skip it.
	want, err := upscaleFingerprint(paks, opts)
	if err != nil {
		return nil, err
	}
	if !opts.Force {
		if got := readUpscaleStamp(opts.OutPath); got != "" && got == want {
			fi, err := os.Stat(opts.OutPath)
			if err != nil {
				return nil, err
			}
			opts.progress("output pak is already up to date — skipping upscale (use --force to rebuild)")
			return &UpscaleResult{OutPath: opts.OutPath, Bytes: fi.Size(), Skipped: true}, nil
		}
	}

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
	if err := extractAndNormalise(paks, entries, inDir, origExt, func(done int) {
		opts.bar("normalise", done, len(entries))
	}); err != nil {
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
	count, err := snapAndCollect(upDir, potDir, origExt, opts.MaxSize, builder, func(done int) {
		opts.bar("snap", done, len(origExt))
	})
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, fmt.Errorf("power-of-two snap produced no files")
	}
	opts.progress("prepared %d hi-res textures", count)

	// Embed the fingerprint so a later run can skip an unchanged rebuild.
	stampPath := filepath.Join(work, "upscale-stamp")
	if err := os.WriteFile(stampPath, []byte(want), 0o644); err != nil {
		return nil, err
	}
	builder.Add(upscaleStampName, stampPath)

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
func extractAndNormalise(paks []string, entries []texEntry, inDir string, origExt map[string]string, onProgress func(done int)) error {
	// Group entries by source pak to open each pak once.
	byPak := make(map[string][]string)
	for _, e := range entries {
		byPak[e.pak] = append(byPak[e.pak], e.name)
	}
	done := 0
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
			done++
			if onProgress != nil {
				onProgress(done)
			}
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
func snapAndCollect(upDir, potDir string, origExt map[string]string, maxSize int, builder *pk3.Builder, onProgress func(done int)) (int, error) {
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
		if onProgress != nil {
			onProgress(count + 1)
		}
		img, err := DecodeFile(path)
		if err != nil {
			return err
		}
		snapped := SnapToPowerOfTwo(CapLongestSide(img, maxSize))
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
	// Build our own image on first use (no-op for a user-supplied --image).
	if err := rt.EnsureUpscaleImage(ctx, opts.Image, opts.Progress); err != nil {
		return err
	}

	// realesrgan-ncnn-vulkan's directory mode is NOT recursive — it only reads
	// the top level of -i. Our inputs mirror the pak's nested tree
	// (textures/…, models/…), so we flatten every PNG into a single scratch
	// dir for the container, run it once, then restore the outputs to their
	// nested paths under upDir.
	flatIn, err := os.MkdirTemp(filepath.Dir(inDir), "flat-in-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(flatIn) }()
	flatOut, err := os.MkdirTemp(filepath.Dir(upDir), "flat-out-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(flatOut) }()

	flatToRel, err := flattenTree(inDir, flatIn)
	if err != nil {
		return err
	}
	if len(flatToRel) == 0 {
		return fmt.Errorf("no PNG inputs to upscale")
	}

	run := ContainerRun{
		Runtime: rt,
		Image:   opts.Image,
		Mounts: []string{
			flatIn + ":/in:ro",
			flatOut + ":/out",
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
	// The container is one opaque batch call; realesrgan-ncnn-vulkan prints no
	// machine-readable progress. It does, however, write each output PNG as it
	// finishes, so poll flatOut for the produced count to drive a live bar.
	total := len(flatToRel)
	opts.bar("upscale", 0, total)
	stopPoll := make(chan struct{})
	var pollWG sync.WaitGroup
	if opts.ProgressBar != nil {
		pollWG.Go(func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-stopPoll:
					return
				case <-ticker.C:
					opts.bar("upscale", countPNGs(flatOut), total)
				}
			}
		})
	}
	runErr := run.Run(ctx)
	close(stopPoll)
	pollWG.Wait()
	if runErr != nil {
		return runErr
	}
	opts.bar("upscale", total, total)

	return unflattenTree(flatOut, upDir, flatToRel)
}

// countPNGs returns the number of .png files directly in dir (non-recursive);
// used to poll the flat upscaler output for live progress. Errors yield 0.
func countPNGs(dir string) int {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range ents {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".png") {
			n++
		}
	}
	return n
}

// flattenTree copies every .png under srcDir (at any depth) into a single flat
// dstDir, giving each a unique numeric name. It returns a map from the flat
// filename (e.g. "000123.png") back to the original srcDir-relative path, so
// the upscaled outputs can be restored to their nested locations.
func flattenTree(srcDir, dstDir string) (map[string]string, error) {
	flatToRel := make(map[string]string)
	i := 0
	err := filepath.WalkDir(srcDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || strings.ToLower(filepath.Ext(path)) != ".png" {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		flat := fmt.Sprintf("%08d.png", i)
		i++
		flatToRel[flat] = filepath.ToSlash(rel)
		if err := copyFile(path, filepath.Join(dstDir, flat)); err != nil {
			return err
		}
		return nil
	})
	return flatToRel, err
}

// unflattenTree moves each upscaled flat output back to its original nested
// path under dstDir, using the mapping from flattenTree. Outputs with no known
// mapping are skipped.
func unflattenTree(flatOut, dstDir string, flatToRel map[string]string) error {
	for flat, rel := range flatToRel {
		src := filepath.Join(flatOut, flat)
		if _, err := os.Stat(src); err != nil {
			continue // upscaler produced no output for this input; skip
		}
		dst := filepath.Join(dstDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := copyFile(src, dst); err != nil {
			return err
		}
	}
	return nil
}

// copyFile copies src to dst, creating or truncating dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
