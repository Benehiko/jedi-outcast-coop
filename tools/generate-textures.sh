#!/usr/bin/env bash
#
# generate-textures.sh — generate ORIGINAL, non-branded surface textures with a
# locally-run, openly-licensed generative model (FLUX.1-schnell, Apache-2.0),
# and pack them into a pk3.
#
# WHAT THIS IS (and is not)
#   This produces GENERIC surface material — metal, concrete, rock, rust,
#   fabric, panels — from text prompts, entirely on your machine. It is NOT a
#   Star Wars asset generator: the prompt manifest is deliberately limited to
#   non-branded, non-recognizable materials, and the output is packed under a
#   neutral textures/generated/ path rather than overwriting Raven's specific
#   textures. That keeps the result clean on both axes:
#
#     * Copyright: FLUX.1-schnell is Apache-2.0 and places no restrictions on
#       generated outputs, so the images are original works you may ship. (The
#       upscale tool, by contrast, derives from retail art and therefore only
#       ever runs on the user's own copy — see upscale-textures.sh.)
#     * Trademark: generic materials with no Star Wars motifs, insignia, ships,
#       or characters stay clear of Lucasfilm/Disney marks and trade dress.
#
#   Keep both properties if you extend PROMPTS below: materials only, nothing
#   recognizably from the game or the franchise.
#
# The model runs in an ephemeral container (nerdctl/podman). Two GPU backends
# are supported: AMD ROCm (default image) and NVIDIA CUDA (--cuda). The
# plumbing (power-of-two snap, packing) uses host ImageMagick + zip, like the
# other tools/ scripts.
#
# ENGINE CONSTRAINT: textures must be power-of-two (the renderer FATALs
# otherwise). Images are generated at a power-of-two size and re-snapped after,
# so this always holds.
#
# Usage:
#   tools/generate-textures.sh [options]
#
# Options:
#   --out FILE       Output pak (default: $HOME/.local/share/openjo/base/zzz-generated-textures.pk3)
#   --size N         Square texture size, power of two (default: 1024)
#   --steps N        Diffusion steps (FLUX.1-schnell is a few-step model; default 4)
#   --seed N         Base RNG seed for reproducibility (default 1)
#   --manifest FILE  A "name|prompt" manifest (one per line) to use instead of
#                    the built-in generic material set
#   --model-dir DIR  Where to cache the downloaded model weights
#                    (default: $HOME/.cache/flux-schnell)
#   --image IMG      Container image with PyTorch + the chosen GPU backend
#                    (default: ROCm image below; use --cuda for the NVIDIA one)
#   --cuda           Use the NVIDIA/CUDA backend + GPU flags instead of ROCm
#   --runtime RT     nerdctl or podman (default: autodetect)
#   --keep-work      Keep the scratch work directory
#   --dry-run        Print the manifest and settings, generate nothing
#   -h, --help       Show this help
#
# Environment:
#   HF_TOKEN         Hugging Face access token. REQUIRED for the default model:
#                    FLUX.1-schnell is Apache-2.0 but its HF repo is gated, so
#                    the weight download needs a token (and a one-time license
#                    acceptance on the model page) or it fails with HTTP 401.
#                    It is passed into the container only; never logged or baked
#                    into the image. Model weights cache under --model-dir and
#                    are reused across runs (not deleted).
#
# See docs/asset-generation.md for the licensing analysis, model notes, the
# token workflow, prompt guidance, and how to keep additions non-infringing.

set -euo pipefail

# ------------------------------------------------------------------ defaults
OUT="${JK2_GENERATED_OUT:-$HOME/.local/share/openjo/base/zzz-generated-textures.pk3}"
SIZE=1024
STEPS=4
SEED=1
MANIFEST=""
MODEL_DIR="${FLUX_MODEL_DIR:-$HOME/.cache/flux-schnell}"
ROCM_IMAGE="${GEN_ROCM_IMAGE:-rocm/pytorch:latest}"
CUDA_IMAGE="${GEN_CUDA_IMAGE:-pytorch/pytorch:2.4.0-cuda12.1-cudnn9-runtime}"
IMAGE=""
USE_CUDA=0
RUNTIME=""
KEEP_WORK=0
DRY_RUN=0

die() { echo "error: $*" >&2; exit 1; }
info() { echo ">>> $*"; }
need() { command -v "$1" >/dev/null 2>&1 || die "required host tool not found: $1"; }

# ------------------------------------------------------------------ built-in prompts
# Generic, non-branded, seamless material prompts. Each line: name|prompt.
# The "name" becomes textures/generated/<name>.jpg. KEEP THESE NON-RECOGNISABLE.
read -r -d '' DEFAULT_MANIFEST <<'EOF' || true
metal_panel_worn|seamless tileable texture of a worn brushed metal wall panel, subtle scratches and grime, industrial, flat even lighting, photographic, no logos, no text
metal_plate_riveted|seamless tileable texture of a riveted steel plate, weathered gunmetal, evenly lit, photographic, no markings
concrete_bare|seamless tileable texture of bare cast concrete, fine surface pitting, neutral grey, flat lighting, photographic
concrete_stained|seamless tileable texture of stained concrete floor, faint cracks and water marks, matte, photographic
rock_grey|seamless tileable texture of rough grey stone rock surface, natural, evenly lit, photographic
rust_heavy|seamless tileable texture of heavily rusted corroded iron, orange-brown patina, photographic, no text
metal_grating|seamless tileable texture of a dark metal floor grating pattern, industrial, evenly lit, photographic
sand_fine|seamless tileable texture of fine desert sand, subtle ripples, warm neutral tone, flat lighting, photographic
fabric_canvas|seamless tileable texture of coarse grey canvas fabric weave, matte, evenly lit, photographic
panel_scifi_plain|seamless tileable texture of a plain matte industrial wall panel with shallow seams, neutral grey, no symbols, no lettering, photographic
EOF

# ------------------------------------------------------------------ args
while [[ $# -gt 0 ]]; do
  case "$1" in
    --out)       OUT="$2"; shift 2;;
    --size)      SIZE="$2"; shift 2;;
    --steps)     STEPS="$2"; shift 2;;
    --seed)      SEED="$2"; shift 2;;
    --manifest)  MANIFEST="$2"; shift 2;;
    --model-dir) MODEL_DIR="$2"; shift 2;;
    --image)     IMAGE="$2"; shift 2;;
    --cuda)      USE_CUDA=1; shift;;
    --runtime)   RUNTIME="$2"; shift 2;;
    --keep-work) KEEP_WORK=1; shift;;
    --dry-run)   DRY_RUN=1; shift;;
    -h|--help)   sed -n '2,58p' "$0" | sed 's/^# \{0,1\}//'; exit 0;;
    *) die "unknown option: $1 (try --help)";;
  esac
done

# power-of-two check on --size
(( SIZE > 0 )) && (( (SIZE & (SIZE - 1)) == 0 )) || die "--size must be a power of two (got $SIZE)"

# ------------------------------------------------------------------ host tools
if command -v magick >/dev/null 2>&1; then IM=(magick); else need convert; IM=(convert); fi
need zip

# ------------------------------------------------------------------ manifest
MAN_CONTENT=""
if [[ -n "$MANIFEST" ]]; then
  [[ -f "$MANIFEST" ]] || die "manifest not found: $MANIFEST"
  MAN_CONTENT="$(cat "$MANIFEST")"
else
  MAN_CONTENT="$DEFAULT_MANIFEST"
fi
# Count valid lines.
MAN_N=$(printf '%s\n' "$MAN_CONTENT" | grep -cE '^[^#].*\|')
(( MAN_N > 0 )) || die "manifest has no 'name|prompt' lines"

if (( DRY_RUN == 1 )); then
  info "settings: size=$SIZE steps=$STEPS seed=$SEED backend=$([[ $USE_CUDA == 1 ]] && echo CUDA || echo ROCm)"
  info "would generate $MAN_N texture(s) into $OUT:"
  printf '%s\n' "$MAN_CONTENT" | grep -E '^[^#].*\|' | while IFS='|' read -r n p; do
    printf '    textures/generated/%s.jpg  <-  %s\n' "$n" "$p"
  done
  exit 0
fi

# ------------------------------------------------------------------ runtime + backend
if [[ -z "$RUNTIME" ]]; then
  if command -v nerdctl >/dev/null 2>&1; then RUNTIME=nerdctl
  elif command -v podman >/dev/null 2>&1; then RUNTIME=podman
  else die "no container runtime found (need nerdctl or podman)"; fi
fi
command -v "$RUNTIME" >/dev/null 2>&1 || die "runtime '$RUNTIME' not found"

GPU_ARGS=()
if (( USE_CUDA == 1 )); then
  IMAGE="${IMAGE:-$CUDA_IMAGE}"
  GPU_ARGS=(--gpus all)
  info "backend: NVIDIA/CUDA ($IMAGE)"
else
  IMAGE="${IMAGE:-$ROCM_IMAGE}"
  # ROCm needs the KFD + DRI devices.
  [[ -e /dev/kfd ]] || die "no /dev/kfd — ROCm not available; use --cuda or install ROCm"
  GPU_ARGS=(--device /dev/kfd --device /dev/dri --security-opt seccomp=unconfined)
  # `--group-add keep-groups` is a Podman-only convenience to inherit the host's
  # render/video group membership; nerdctl/containerd rejects it. Only pass it
  # under podman, and only when the render nodes are not world-accessible.
  if [[ "$RUNTIME" == "podman" ]]; then
    GPU_ARGS+=(--group-add keep-groups)
  fi
  info "backend: AMD/ROCm ($IMAGE)"
fi

mkdir -p "$MODEL_DIR"
# The model weights (tens of GB) are cached here and are NOT cleaned up — only
# the scratch work dir is. Re-runs reuse this cache instead of re-downloading.
info "model cache: $MODEL_DIR (persisted; reused across runs)"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/jk2-generate.XXXXXX")"
cleanup() { [[ "$KEEP_WORK" == "1" ]] || rm -rf "$WORK"; }
trap cleanup EXIT
info "work dir: $WORK (removed on exit unless --keep-work)"

OUTRAW="$WORK/out"   # model output PNGs
POT="$WORK/pot"      # power-of-two, JPEG, final tree to pack
mkdir -p "$OUTRAW" "$POT" "$WORK/manifest"
printf '%s\n' "$MAN_CONTENT" > "$WORK/manifest/prompts.txt"

# ------------------------------------------------------------------ generation script (runs in container)
# Written to the work dir and executed inside the container. Uses diffusers to
# load FLUX.1-schnell and render each prompt to a PNG. Model weights cache to
# /models (bind of $MODEL_DIR) so re-runs don't re-download.
cat > "$WORK/gen.py" <<'PY'
import os, sys, hashlib
os.environ.setdefault("HF_HOME", "/models/hf")
os.environ.setdefault("HF_HUB_ENABLE_HF_TRANSFER", "0")
import torch
from diffusers import FluxPipeline

size  = int(os.environ["GEN_SIZE"])
steps = int(os.environ["GEN_STEPS"])
seed  = int(os.environ["GEN_SEED"])
manifest = "/work/manifest/prompts.txt"
outdir   = "/work/out"
os.makedirs(outdir, exist_ok=True)

device = "cuda" if torch.cuda.is_available() else "cpu"
print(f">>> torch {torch.__version__}, device={device}, cuda_avail={torch.cuda.is_available()}", flush=True)
if device == "cpu":
    print("!!! no GPU visible to torch — generation would be extremely slow; aborting", file=sys.stderr, flush=True)
    sys.exit(3)

dtype = torch.bfloat16
pipe = FluxPipeline.from_pretrained("black-forest-labs/FLUX.1-schnell", torch_dtype=dtype, cache_dir="/models/hf")

# FLUX.1-schnell in bf16 needs ~24 GB to live fully on the GPU, which OOMs on
# consumer 16 GB cards even at 512x512. Choose a placement by available VRAM:
#   * >= ~26 GB  -> everything on GPU (fastest)
#   * >= ~12 GB  -> model CPU offload: submodules stream to the GPU one at a
#                   time; fits comfortably in 12-16 GB, modest speed cost
#   * otherwise  -> sequential CPU offload: lowest VRAM (~4 GB), slowest
# GEN_VRAM_MODE (auto|full|model|sequential) can override the auto choice.
vram_gb = 0.0
try:
    vram_gb = torch.cuda.get_device_properties(0).total_memory / (1024**3)
except Exception:
    pass
mode = os.environ.get("GEN_VRAM_MODE", "auto")
if mode == "auto":
    if vram_gb >= 26:   mode = "full"
    elif vram_gb >= 12: mode = "model"
    else:               mode = "sequential"
print(f">>> VRAM {vram_gb:.1f} GB -> placement mode: {mode}", flush=True)

try:
    pipe.enable_attention_slicing()
except Exception:
    pass
try:
    pipe.enable_vae_slicing()
except Exception:
    pass

if mode == "full":
    pipe = pipe.to(device)
elif mode == "model":
    pipe.enable_model_cpu_offload()
else:
    pipe.enable_sequential_cpu_offload()

rows = []
with open(manifest) as f:
    for line in f:
        line = line.strip()
        if not line or line.startswith("#") or "|" not in line:
            continue
        name, prompt = line.split("|", 1)
        rows.append((name.strip(), prompt.strip()))

print(f">>> generating {len(rows)} textures at {size}x{size}, {steps} steps", flush=True)
for i, (name, prompt) in enumerate(rows):
    # deterministic per-row seed. Use a CPU generator: it is valid in every
    # placement mode (including CPU-offload, where pipe components are not all
    # resident on the GPU), and keeps seeds reproducible across machines.
    s = (seed + int(hashlib.sha256(name.encode()).hexdigest(), 16)) % (2**31)
    g = torch.Generator(device="cpu").manual_seed(s)
    img = pipe(
        prompt,
        height=size, width=size,
        num_inference_steps=steps,
        guidance_scale=0.0,          # schnell is guidance-distilled
        generator=g,
    ).images[0]
    out = os.path.join(outdir, f"{name}.png")
    img.save(out)
    print(f"    [{i+1}/{len(rows)}] {name} -> {out}", flush=True)
print(">>> generation done", flush=True)
PY

# ------------------------------------------------------------------ 1. generate
info "starting model container (first run downloads FLUX.1-schnell weights ~ tens of GB)..."
# Pre-install diffusers stack inside the container, then run gen.py. The ROCm
# and CUDA base images ship torch already; we add diffusers + transformers.
# FLUX.1-schnell is Apache-2.0 but its Hugging Face repo is GATED: downloading
# the weights needs a one-time license acceptance on the model page and an HF
# access token. Pass it through as HF_TOKEN when set (never bake it into the
# image or the command line beyond this env var).
HF_ENV=()
if [[ -n "${HF_TOKEN:-}" ]]; then
  HF_ENV=(-e "HF_TOKEN=${HF_TOKEN}" -e "HUGGING_FACE_HUB_TOKEN=${HF_TOKEN}")
  info "passing HF_TOKEN to the container for the gated model download"
else
  info "no HF_TOKEN set — if the model is gated the download will 401 (see docs/asset-generation.md)"
fi

set +e
"$RUNTIME" run --rm "${GPU_ARGS[@]}" "${HF_ENV[@]}" \
  -e GEN_SIZE="$SIZE" -e GEN_STEPS="$STEPS" -e GEN_SEED="$SEED" \
  -e GEN_VRAM_MODE="${GEN_VRAM_MODE:-auto}" \
  -e PYTORCH_HIP_ALLOC_CONF="expandable_segments:True" \
  -e PYTORCH_CUDA_ALLOC_CONF="expandable_segments:True" \
  -e TORCH_BLAS_PREFER_HIPBLASLT="${TORCH_BLAS_PREFER_HIPBLASLT:-0}" \
  ${HSA_OVERRIDE_GFX_VERSION:+-e HSA_OVERRIDE_GFX_VERSION="$HSA_OVERRIDE_GFX_VERSION"} \
  -v "$WORK:/work" -v "$MODEL_DIR:/models" \
  "$IMAGE" \
  bash -lc '
    set -e
    python -m pip install --quiet --no-input "diffusers>=0.30" "transformers>=4.43" accelerate sentencepiece protobuf safetensors pillow
    python /work/gen.py
  '
RC=$?
set -e
if (( RC != 0 )); then
  {
    echo "generation container exited $RC."
    echo "  * exit 3 -> torch could not see the GPU inside the container."
    echo "    ROCm: confirm /dev/kfd + /dev/dri passthrough and that the image supports your GPU arch."
    echo "    CUDA: ensure the NVIDIA container toolkit is installed and pass --cuda."
    echo "  * a 401/403 -> the model is gated: accept its license on Hugging Face and"
    echo "    export HF_TOKEN=<your read token> before running (see docs/asset-generation.md)."
    echo "  * a download/OOM error -> see docs/asset-generation.md for VRAM and disk needs."
  } >&2
  die "generation failed"
fi

GENCOUNT=$(find "$OUTRAW" -type f -iname '*.png' | wc -l)
(( GENCOUNT > 0 )) || die "model produced no images"
info "generated $GENCOUNT textures"

# ------------------------------------------------------------------ 2. PoT snap -> JPEG
# Generated at a power-of-two already, but re-snap defensively and convert to
# JPEG (smaller; these are diffuse color maps). Packed under textures/generated/.
info "snapping to power-of-two and packing under textures/generated/ ..."
( cd "$OUTRAW" && find . -type f -iname '*.png' -print0 ) \
  | (cd "$OUTRAW" && xargs -0 -P "$(nproc 2>/dev/null || echo 4)" -I{} sh -c '
      rel="{}"; rel="${rel#./}"; stem="${rel%.*}"
      out="'"$POT"'/textures/generated/${stem}.jpg"
      mkdir -p "$(dirname "$out")"
      dim=$("'"${IM[0]}"'" identify -format "%w %h" "$rel" 2>/dev/null) || exit 0
      w=${dim% *}; h=${dim#* }
      p=1; while [ "$p" -lt "$w" ]; do p=$((p*2)); done; nw=$p
      p=1; while [ "$p" -lt "$h" ]; do p=$((p*2)); done; nh=$p
      if [ "$nw" = "$w" ] && [ "$nh" = "$h" ]; then
        "'"${IM[0]}"'" "$rel" -quality 95 "JPG:$out"
      else
        "'"${IM[0]}"'" "$rel" -resize "${nw}x${nh}!" -quality 95 "JPG:$out"
      fi
    ')

POTCOUNT=$(find "$POT" -type f | wc -l)
(( POTCOUNT > 0 )) || die "power-of-two snap produced no files"

# ------------------------------------------------------------------ 3. pack
info "packing -> $OUT"
rm -f "$WORK/_out.pk3"
( cd "$POT" && zip -q -r -X "$WORK/_out.pk3" . )
[[ -f "$WORK/_out.pk3" ]] || die "zip produced no pak"
mkdir -p "$(dirname "$OUT")"
mv "$WORK/_out.pk3" "$OUT"
SZ=$(du -h "$OUT" | cut -f1)

echo ""
info "done."
echo "    wrote: $OUT ($SZ, $POTCOUNT original textures under textures/generated/)"
echo "    These are generic, non-branded materials. Reference them from your own"
echo "    shaders/maps as textures/generated/<name>. To remove: rm '$OUT'"
