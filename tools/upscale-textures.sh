#!/usr/bin/env bash
#
# upscale-textures.sh — generate a high-resolution texture override pak for
# Jedi Outcast from YOUR OWN retail assets, using a local Real-ESRGAN model.
#
# This project ships NO game data. This tool reads the proprietary assets*.pk3
# files from your own legal copy of the game, upscales the world/model textures
# with a locally-run neural upscaler, and writes a new override pak
# (zzz-hires-textures.pk3) into your base/ directory. Your original assets are
# never modified — the engine simply loads the override pak on top of them
# (paks are searched by name in descending order, so a "zzz-" pak wins).
#
# The neural upscale runs inside an ephemeral container (nerdctl/podman) so
# nothing is installed on the host for it; GPU (Vulkan) acceleration is used
# automatically when a render node is present, else it falls back to CPU. The
# plumbing steps (unzip / image convert / zip) use the host's standard tools
# (unzip, ImageMagick, zip), the same way the rest of tools/ uses git and cmake.
#
# ---------------------------------------------------------------------------
# ENGINE CONSTRAINTS THIS TOOL RESPECTS
#
#  * Textures MUST have power-of-two dimensions. The renderer FATALs on any
#    non-power-of-two image (tr_image.cpp: "dimensions ... not power of 2!").
#    Real-ESRGAN's integer scale does not preserve that (e.g. 96x128 *4 = 384x512,
#    and 384 is not a power of two), so every upscaled image is snapped to the
#    next power-of-two before packing.
#  * The override must keep the ORIGINAL path AND extension. The engine resolves
#    a texture by trying the shader's exact extension first, and paks overlay by
#    path — a .png placed at a .jpg's path does NOT shadow it. So each upscaled
#    image is written back out at the source file's exact relative path and
#    extension (jpg/tga/png), TGA uncompressed (which the JK2 loader reads).
#  * Only real 3D surface/model art is upscaled: textures/ and models/. The 2D
#    HUD, menus, fonts and lightmaps are pixel-placed or size-sensitive and are
#    deliberately skipped — upscaling them distorts the UI.
#  * Oversized textures are safely clamped by the engine to maxTextureSize at
#    load, so a 4x pass that overshoots your GPU limit will not crash; it just
#    wastes disk. Keep --scale sensible for your hardware.
# ---------------------------------------------------------------------------
#
# Usage:
#   tools/upscale-textures.sh [options]
#
# Options:
#   --assets DIR     Directory containing your retail assets*.pk3
#                    (default: $HOME/.local/share/openjo/base)
#   --out FILE       Output pak path
#                    (default: <assets>/zzz-hires-textures.pk3)
#   --scale N        Upscale factor: 2 or 4 (default: 4)
#   --model NAME     Real-ESRGAN model: realesrgan-x4plus (default, photographic)
#                    or realesr-animevideov3 (cleaner for stylised/painted art)
#   --limit N        Only process the first N textures (for a quick trial run)
#   --jobs N         Parallel image workers for the plumbing (default: nproc)
#   --image IMG      Container image providing the realesrgan-ncnn-vulkan binary
#                    (default: $UPSCALE_IMAGE or the value below; see
#                    docs/hires-textures.md for how to obtain/mirror one)
#   --runtime RT     Container runtime: nerdctl or podman (default: autodetect)
#   --cpu            Force CPU mode (skip GPU/Vulkan passthrough)
#   --stub-upscale   TEST MODE: replace the neural pass with a plain Lanczos
#                    resize (no container). Exercises the whole extract → PoT →
#                    extension-restore → repack → override pipeline without the
#                    model. The output is a bilinear-ish enlargement, not for
#                    real use — it is how the pipeline is verified in CI/dev.
#   --keep-work      Keep the scratch work directory instead of deleting it
#   -h, --help       Show this help
#
# See docs/hires-textures.md for the full guide, model notes, and how to make
# the override optional/removable.

set -euo pipefail

# ------------------------------------------------------------------ defaults
ASSETS="${JK2_ASSETS_BASE:-$HOME/.local/share/openjo/base}"
OUT=""
SCALE=4
MODEL="realesrgan-x4plus"
LIMIT=0
JOBS="$(nproc 2>/dev/null || echo 4)"
IMAGE="${UPSCALE_IMAGE:-docker.io/utkuozbulak/realesrgan-ncnn-vulkan:latest}"
RUNTIME=""
FORCE_CPU=0
STUB=0
KEEP_WORK=0

die() { echo "error: $*" >&2; exit 1; }
info() { echo ">>> $*"; }
need() { command -v "$1" >/dev/null 2>&1 || die "required host tool not found: $1"; }

# ------------------------------------------------------------------ args
while [[ $# -gt 0 ]]; do
  case "$1" in
    --assets)  ASSETS="$2"; shift 2;;
    --out)     OUT="$2"; shift 2;;
    --scale)   SCALE="$2"; shift 2;;
    --model)   MODEL="$2"; shift 2;;
    --limit)   LIMIT="$2"; shift 2;;
    --jobs)    JOBS="$2"; shift 2;;
    --image)   IMAGE="$2"; shift 2;;
    --runtime) RUNTIME="$2"; shift 2;;
    --cpu)     FORCE_CPU=1; shift;;
    --stub-upscale) STUB=1; shift;;
    --keep-work) KEEP_WORK=1; shift;;
    -h|--help) sed -n '2,72p' "$0" | sed 's/^# \{0,1\}//'; exit 0;;
    *) die "unknown option: $1 (try --help)";;
  esac
done

[[ "$SCALE" == "2" || "$SCALE" == "4" ]] || die "--scale must be 2 or 4"
OUT="${OUT:-$ASSETS/zzz-hires-textures.pk3}"

# ------------------------------------------------------------------ host tools
# The plumbing uses standard host tools (installed like git/cmake). Only the
# neural upscale is containerised.
need unzip
need zip
if command -v magick >/dev/null 2>&1; then IM=(magick); else need convert; IM=(convert); fi

# ------------------------------------------------------------------ runtime (upscale only)
if (( STUB == 0 )); then
  if [[ -z "$RUNTIME" ]]; then
    if command -v nerdctl >/dev/null 2>&1; then RUNTIME=nerdctl
    elif command -v podman >/dev/null 2>&1; then RUNTIME=podman
    else die "no container runtime found (need nerdctl or podman for the upscale step; or use --stub-upscale)"; fi
  fi
  command -v "$RUNTIME" >/dev/null 2>&1 || die "runtime '$RUNTIME' not found"
fi

# GPU passthrough for Vulkan, when a render node exists and CPU wasn't forced.
GPU_ARGS=()
if (( STUB == 0 && FORCE_CPU == 0 )) && [[ -e /dev/dri/renderD128 ]]; then
  GPU_ARGS=(--device /dev/dri:/dev/dri)
  info "GPU render node found — enabling Vulkan acceleration"
elif (( STUB == 0 )); then
  info "running Real-ESRGAN on CPU (no GPU passthrough) — this is slow"
fi

# ------------------------------------------------------------------ inputs
[[ -d "$ASSETS" ]] || die "assets dir not found: $ASSETS"
shopt -s nullglob
PK3S=("$ASSETS"/assets*.pk3)
(( ${#PK3S[@]} > 0 )) || die "no assets*.pk3 in $ASSETS — point --assets at your retail base/"
info "found ${#PK3S[@]} retail pak(s) in $ASSETS"

WORK="$(mktemp -d "${TMPDIR:-/tmp}/jk2-upscale.XXXXXX")"
cleanup() { [[ "$KEEP_WORK" == "1" ]] || rm -rf "$WORK"; }
trap cleanup EXIT
info "work dir: $WORK"

RAW="$WORK/raw"          # extracted source textures (only the ones we upscale)
IN="$WORK/in"            # PNG-normalised inputs for the model
UP="$WORK/upscaled"      # upscaler output PNGs (non-power-of-two)
POT="$WORK/pot"          # final tree to pack: PoT + original extension
mkdir -p "$RAW" "$IN" "$UP" "$POT"

# ------------------------------------------------------------------ 1. extract
# Extract only the raster textures under textures/ and models/, from every pak,
# letting later paks overwrite earlier ones (the same order the engine resolves
# them). The retail assets are large (assets0.pk3 alone is ~500 MB / thousands
# of entries), so when --limit is set we resolve the file list *first* and
# extract only the first N entries — a --limit run must not unpack gigabytes.
info "indexing texture entries in ${#PK3S[@]} pak(s)..."

# Build the ordered, de-duplicated list of raster entries across all paks.
# Later paks win, matching load order: keep the LAST occurrence of each path.
declare -A SEEN=()
ENTRIES=()   # relative paths, in first-seen order (then overwritten by later paks)
for pk in "${PK3S[@]}"; do
  while IFS= read -r name; do
    case "$name" in
      textures/*|models/*)
        case "${name,,}" in
          *.jpg|*.jpeg|*.tga|*.png)
            if [[ -z "${SEEN[$name]:-}" ]]; then ENTRIES+=("$name"); fi
            SEEN[$name]="$pk"   # remember which pak to pull it from (last wins)
            ;;
        esac
        ;;
    esac
  done < <(unzip -Z1 "$pk" 2>/dev/null)
done

TOTAL=${#ENTRIES[@]}
(( TOTAL > 0 )) || die "no textures found in the paks — are these valid JK2 assets?"

if (( LIMIT > 0 && LIMIT < TOTAL )); then
  info "found $TOTAL texture entries; limiting to the first $LIMIT (--limit)"
  ENTRIES=("${ENTRIES[@]:0:LIMIT}")
else
  info "found $TOTAL texture entries to upscale"
fi
COUNT=${#ENTRIES[@]}

info "extracting $COUNT texture(s) from paks..."
# Extract each selected entry from the pak that should provide it (last wins).
# Group by source pak to keep unzip invocations down.
for pk in "${PK3S[@]}"; do
  sel=()
  for name in "${ENTRIES[@]}"; do
    [[ "${SEEN[$name]}" == "$pk" ]] && sel+=("$name")
  done
  (( ${#sel[@]} > 0 )) || continue
  # unzip a specific list of members; exit 11 (nothing matched) is not an error.
  unzip -o -qq "$pk" "${sel[@]}" -d "$RAW" || [[ $? -eq 11 ]] || \
    die "unzip failed on $(basename "$pk")"
done

# Sanity: the extracted raster set should match what we asked for.
mapfile -d '' TEXFILES < <(find "$RAW" -type f \( -iname '*.jpg' -o -iname '*.jpeg' -o -iname '*.tga' -o -iname '*.png' \) -print0)
(( ${#TEXFILES[@]} > 0 )) || die "extraction produced no texture files"

# ------------------------------------------------------------------ 2. normalise -> PNG
# Real-ESRGAN reads/writes PNG most reliably, so convert every source texture
# (TGA/JPG/PNG) to a PNG under $IN, mirroring its relative path (extensionless).
info "normalising $COUNT textures to PNG (jobs=$JOBS)..."
export IM_BIN="${IM[0]}"
( cd "$RAW" && find . -type f \( -iname '*.jpg' -o -iname '*.jpeg' -o -iname '*.tga' -o -iname '*.png' \) -print0 ) \
  | (cd "$RAW" && xargs -0 -P "$JOBS" -I{} sh -c '
      rel="{}"; rel="${rel#./}"
      out="'"$IN"'/${rel%.*}.png"
      mkdir -p "$(dirname "$out")"
      "$IM_BIN" "$rel" "PNG24:$out" 2>/dev/null || true
    ')
INCOUNT=$(find "$IN" -type f -iname '*.png' | wc -l)
(( INCOUNT > 0 )) || die "PNG normalisation produced no files"

# ------------------------------------------------------------------ 3. upscale
if (( STUB == 1 )); then
  info "STUB upscale: plain ${SCALE}x Lanczos resize (not Real-ESRGAN)"
  ( cd "$IN" && find . -type f -iname '*.png' -print0 ) \
    | (cd "$IN" && xargs -0 -P "$JOBS" -I{} sh -c '
        rel="{}"; rel="${rel#./}"
        out="'"$UP"'/$rel"
        mkdir -p "$(dirname "$out")"
        "'"${IM[0]}"'" "$rel" -filter Lanczos -resize '"$((SCALE*100))"'% "PNG24:$out"
      ')
else
  info "upscaling with Real-ESRGAN model '$MODEL' at ${SCALE}x (the slow part)..."
  # realesrgan-ncnn-vulkan batch mode: -i dir -o dir -n model -s scale -f png.
  set +e
  "$RUNTIME" run --rm "${GPU_ARGS[@]}" \
    -v "$IN:/in:ro" -v "$UP:/out" \
    "$IMAGE" \
    -i /in -o /out -n "$MODEL" -s "$SCALE" -f png
  RC=$?
  set -e
  if (( RC != 0 )); then
    {
      echo "Real-ESRGAN container exited $RC."
      echo "  * image not found -> pull or mirror it and pass --image (see docs/hires-textures.md)"
      echo "  * Vulkan/GPU error -> retry with --cpu"
      echo "  * to test the rest of the pipeline without the model, use --stub-upscale"
    } >&2
    die "upscale step failed"
  fi
fi

UPCOUNT=$(find "$UP" -type f -iname '*.png' | wc -l)
(( UPCOUNT > 0 )) || die "upscale produced no output"
info "upscaled $UPCOUNT textures"

# ------------------------------------------------------------------ 4. PoT snap + restore extension
# For each ORIGINAL extracted file (the source of truth for path+extension),
# take its upscaled PNG, snap to the next power of two in each axis, and write
# it back at the original path with the original extension. This is what makes
# the override both loadable (power-of-two) and effective (shadows by path+ext).
info "snapping to power-of-two and restoring original formats (jobs=$JOBS)..."
( cd "$RAW" && find . -type f \( -iname '*.jpg' -o -iname '*.jpeg' -o -iname '*.tga' -o -iname '*.png' \) -print0 ) \
  | (cd "$RAW" && xargs -0 -P "$JOBS" -I{} sh -c '
      orig="{}"; rel="${orig#./}"
      stem="${rel%.*}"; ext="${rel##*.}"
      up="'"$UP"'/${stem}.png"
      [ -f "$up" ] || exit 0
      out="'"$POT"'/$rel"
      mkdir -p "$(dirname "$out")"
      dim=$("'"${IM[0]}"'" identify -format "%w %h" "$up" 2>/dev/null) || exit 0
      w=${dim% *}; h=${dim#* }
      p=1; while [ "$p" -lt "$w" ]; do p=$((p*2)); done; nw=$p
      p=1; while [ "$p" -lt "$h" ]; do p=$((p*2)); done; nh=$p
      lc=$(printf "%s" "$ext" | tr "[:upper:]" "[:lower:]")
      case "$lc" in
        tga)       fmt="-define tga:compression=none TGA:$out" ;;
        jpg|jpeg)  fmt="-quality 95 JPG:$out" ;;
        *)         fmt="PNG24:$out" ;;
      esac
      if [ "$nw" = "$w" ] && [ "$nh" = "$h" ]; then
        "'"${IM[0]}"'" "$up" $fmt
      else
        "'"${IM[0]}"'" "$up" -resize "${nw}x${nh}!" $fmt
      fi
    ')

POTCOUNT=$(find "$POT" -type f | wc -l)
(( POTCOUNT > 0 )) || die "power-of-two snap produced no files"
info "prepared $POTCOUNT hi-res textures"

# ------------------------------------------------------------------ 5. repack
info "packing override pak -> $OUT"
rm -f "$WORK/_out.pk3"
( cd "$POT" && zip -q -r -X "$WORK/_out.pk3" . )
[[ -f "$WORK/_out.pk3" ]] || die "zip step produced no pak"

mkdir -p "$(dirname "$OUT")"
mv "$WORK/_out.pk3" "$OUT"
SZ=$(du -h "$OUT" | cut -f1)

echo ""
info "done."
echo "    wrote: $OUT ($SZ, $POTCOUNT textures)"
echo ""
echo "    The engine loads this pak on top of your retail assets automatically"
echo "    (it sorts after assets*.pk3). Launch the game to see the hi-res"
echo "    textures. To remove them, delete that one file:"
echo "        rm '$OUT'"
