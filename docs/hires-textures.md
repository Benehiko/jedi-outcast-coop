# High-resolution textures (local AI upscale)

Jedi Outcast's world and character textures are early-2000s resolution. On a
QHD/4K/ultrawide display they look soft. `tools/upscale-textures.sh` generates
a **high-resolution override pak** from *your own* retail textures using a
locally-run neural upscaler (Real-ESRGAN), so the game renders sharper art with
no change to your original files.

This project ships **no game data**, and this tool does not change that: it
reads the proprietary `assets*.pk3` from your own legal copy, and writes a new
`zzz-hires-textures.pk3` you can delete at any time. Nothing is uploaded; the
upscale runs entirely on your machine.

> **This is optional and cosmetic.** It has no effect on co-op or gameplay.

The easiest way to build and install the pak is to let the installer run the
tool for you: set `texture_upscale = true` under `[graphics]` in your config (or
toggle **Texture upscale** in the `jk2coop graphics` TUI), then run
`jk2coop install`. It builds `zzz-hires-textures.pk3` into your `base/` when a
GPU container is available, and removes it if you later turn the setting off. The
rest of this document covers running `tools/upscale-textures.sh` directly, which
you need when tuning scale/model or building on a separate GPU machine.

## How it works

```
retail assets*.pk3
        │  (1) extract textures/ + models/ raster images
        ▼
   normalise to PNG
        │  (2) Real-ESRGAN upscale (2x or 4x), locally, GPU if available
        ▼
  upscaled PNGs
        │  (3) snap to power-of-two, restore original path + extension
        ▼
zzz-hires-textures.pk3   ──►  drop in base/, engine loads it over the originals
```

Two engine rules make steps (3) load-bearing — the tool handles both so the
output "just works":

- **Power-of-two dimensions are mandatory.** The renderer aborts with
  `dimensions ... not power of 2` on any texture whose width or height is not a
  power of two. A neural upscaler's integer scale does not preserve that
  (96×128 ×4 = 384×512, and 384 isn't a power of two), so every image is
  resized to the next power of two after upscaling.
- **The override must keep the original path *and* file extension.** The engine
  looks a texture up by trying the shader's exact extension first, and paks
  overlay by exact path — a `.png` placed where the game expects `foo.jpg`
  would *not* replace it. So each upscaled image is written back as the same
  path with the same extension (JPEG/TGA/PNG), and the pak name (`zzz-…`) sorts
  after `assets*.pk3` so it wins.

Only `textures/` and `models/` raster art is upscaled. The 2D HUD, menus,
fonts, and lightmaps are intentionally left alone — they are pixel-placed and
upscaling them would distort the interface.

## Requirements

- **Host tools** (already present on most dev machines, used for the
  extract/convert/repack plumbing): `unzip`, `zip`, and ImageMagick (`magick`
  or `convert`).
- **A container runtime** for the neural step: `nerdctl` or `podman`. The
  upscaler itself is **not** installed on your host — it runs in an ephemeral
  container.
- **A Real-ESRGAN ncnn-vulkan container image** (see below).
- **A GPU with Vulkan** is used automatically if `/dev/dri/renderD128` exists;
  otherwise it falls back to CPU (much slower).
- **Disk space.** A full run expands the whole texture set several times over
  (extracted + PNG + upscaled + packed). Budget well over the size of your
  assets — roughly **10–20 GB of scratch** for a 4× run. If your `/tmp` is a
  small tmpfs, point the tool's scratch at a roomy disk:

  ```sh
  TMPDIR=/path/to/big/disk/scratch tools/upscale-textures.sh …
  ```

  A full 2× run over the retail set produces an override pak on the order of
  ~0.6 GB; a 4× run is several times larger.

## Getting a Real-ESRGAN image

The tool expects an image whose entrypoint is the `realesrgan-ncnn-vulkan`
binary and that bundles the standard models (`realesrgan-x4plus`,
`realesr-animevideov3`, …), invoked as:

```
<image> -i /in -o /out -n <model> -s <scale> -f png
```

Point the tool at whichever image you use with `--image` (or the `UPSCALE_IMAGE`
environment variable). If you maintain your own small image, a Containerfile is
as short as fetching the upstream
[Real-ESRGAN ncnn-vulkan release](https://github.com/xinntao/Real-ESRGAN/releases)
zip (binary + `models/`) into a Vulkan-capable base and setting the binary as
the entrypoint. Mirror it into your own registry if you prefer not to pull at
run time.

> The default image name in the script is a convenience placeholder. Verify and
> pin an image you trust; there is no single canonical published image for this.

## Usage

```sh
# default: 4x, photographic model, reads ~/.local/share/openjo/base,
# writes zzz-hires-textures.pk3 next to your assets.
tools/upscale-textures.sh

# a quick trial on 40 textures first (cheap — only those are extracted):
tools/upscale-textures.sh --limit 40

# 2x (smaller, faster) with a specific assets dir and output path:
tools/upscale-textures.sh --scale 2 \
  --assets "/path/to/GameData/base" \
  --out    "/path/to/GameData/base/zzz-hires-textures.pk3"

# stylised/painted look instead of photographic:
tools/upscale-textures.sh --model realesr-animevideov3

# force CPU (no GPU) or pick a runtime:
tools/upscale-textures.sh --cpu --runtime podman
```

Key options (`--help` lists them all):

| Option | Meaning |
|---|---|
| `--assets DIR` | Where your retail `assets*.pk3` live (default `~/.local/share/openjo/base`). |
| `--out FILE` | Output pak (default `<assets>/zzz-hires-textures.pk3`). |
| `--scale 2\|4` | Upscale factor. `4` is sharper but far larger and slower. |
| `--model NAME` | `realesrgan-x4plus` (default) or `realesr-animevideov3`. |
| `--limit N` | Process only the first N textures — a fast trial. |
| `--jobs N` | Parallelism for the plumbing (default: CPU count). |
| `--image IMG` | Real-ESRGAN container image. |
| `--cpu` | Skip GPU passthrough. |

When it finishes you'll have `zzz-hires-textures.pk3` in your `base/`. Launch
the game — the engine loads it over the retail textures automatically.

## Removing it

Delete the one file. Your retail assets were never touched.

```sh
rm ~/.local/share/openjo/base/zzz-hires-textures.pk3
```

## Verifying / testing the pipeline

The script has a `--stub-upscale` mode that swaps the neural pass for a plain
Lanczos resize (no container, no model). It exercises the entire pipeline —
extract, power-of-two snap, extension restore, repack, and the engine override
— so you can confirm it produces a loadable pak on your machine before spending
time on a real GPU run:

```sh
tools/upscale-textures.sh --stub-upscale --limit 40 --out /tmp/test.pk3
```

The output of stub mode is only a bilinear-ish enlargement — not worth keeping —
but a clean run proves the plumbing. This is how the pipeline was validated: a
full stub run over all 2571 retail textures, packed, symlinked into `base/`,
and loaded into the engine on `kejim_post` with **no power-of-two error** and
correctly-mapped textures in the rendered scene.

## Notes

- Textures larger than your GPU's maximum are safely clamped by the engine at
  load time, so an over-large 4× pass won't crash — it just wastes disk. Prefer
  `--scale 2` unless you know you want (and can store) 4×.
- Upscaling is a one-time cost. Regenerate only if you change model or scale.
- For display-side widescreen/QHD/ultrawide setup (resolution and FOV, separate
  from texture resolution) see [widescreen.md](widescreen.md).
