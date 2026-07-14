import os, sys, hashlib
os.environ.setdefault("HF_HOME", "/models/hf")
os.environ.setdefault("HF_HUB_ENABLE_HF_TRANSFER", "0")
import torch
from diffusers import FluxPipeline

# On some ROCm builds (notably RDNA4/gfx1201) the fused flash / memory-efficient
# scaled-dot-product-attention kernels produce NaNs, which cast to a black image
# at VAE decode. GEN_ATTN=math forces the numerically-safe eager SDPA math
# kernel (slower but correct); "auto" leaves torch's default selection.
if os.environ.get("GEN_ATTN", "math") == "math":
    try:
        torch.backends.cuda.enable_flash_sdp(False)
        torch.backends.cuda.enable_mem_efficient_sdp(False)
        torch.backends.cuda.enable_math_sdp(True)
        print(">>> SDPA: forcing math kernel (GEN_ATTN=math)", flush=True)
    except Exception as e:
        print(f">>> SDPA backend select failed ({e}); using default", flush=True)

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

dtype_name = os.environ.get("GEN_DTYPE", "bf16")
dtype = {"bf16": torch.bfloat16, "fp16": torch.float16, "fp32": torch.float32}.get(dtype_name, torch.bfloat16)
print(f">>> dtype: {dtype_name} ({dtype})", flush=True)

# When HF_HUB_OFFLINE=1 (or GEN_OFFLINE=1), resolve purely from the local cache
# with NO token and NO network. `local_files_only=True` alone is not enough here:
# huggingface_hub's snapshot_download runs a strict completeness check against the
# repo's full file list, and FLUX.1-schnell ships extra top-level files the
# diffusers pipeline never needs (ae.safetensors, flux1-schnell.safetensors,
# README, .gitattributes). If those were skipped at download time the cached
# snapshot is "incomplete" and the load raises IncompleteSnapshotError. So in
# offline mode we point from_pretrained straight at the local snapshot DIRECTORY:
# a real dir path loads the diffusers components in place and skips the repo
# completeness check entirely. We locate it by finding model_index.json under the
# HF cache. Set GEN_MODEL_DIR to override.
OFFLINE = os.environ.get("HF_HUB_OFFLINE", "0") == "1" or os.environ.get("GEN_OFFLINE", "0") == "1"
MODEL_REF = "black-forest-labs/FLUX.1-schnell"
if OFFLINE:
    import glob
    snap = os.environ.get("GEN_MODEL_DIR", "")
    if not snap:
        hits = sorted(glob.glob(
            "/models/hf/models--black-forest-labs--FLUX.1-schnell/snapshots/*/model_index.json"))
        if not hits:
            print("!!! offline: no local FLUX snapshot found under /models/hf", file=sys.stderr, flush=True)
            sys.exit(5)
        snap = os.path.dirname(hits[-1])
    MODEL_REF = snap
    print(f">>> offline: loading from local snapshot dir {MODEL_REF}", flush=True)
pipe = FluxPipeline.from_pretrained(
    MODEL_REF, torch_dtype=dtype, cache_dir="/models/hf",
    local_files_only=OFFLINE,
)

# fp8 path (GEN_FP8=1): quantize the transformer (and text encoder) weights to
# fp8 with optimum-quanto. This roughly halves the transformer (~12 GB bf16 ->
# ~6 GB), so it fits a 16 GB GPU in `model`/`full` placement and avoids the
# fault-prone sequential offload. Quantization runs on CPU before the model
# touches the GPU.
if os.environ.get("GEN_FP8", "0") == "1":
    try:
        from optimum.quanto import freeze, quantize
        import optimum.quanto as _q
        # GEN_FP8_QTYPE selects the quantization dtype: qfloat8 (default),
        # qint8, qint4. qfloat8 preserves dynamic range better for diffusion;
        # if it produces confetti/noise on this GPU, qint8 is worth a try.
        qname = os.environ.get("GEN_FP8_QTYPE", "qfloat8")
        qtype = getattr(_q, qname, None)
        if qtype is None:
            print(f"!!! unknown GEN_FP8_QTYPE={qname}; falling back to qfloat8", file=sys.stderr, flush=True)
            qtype = _q.qfloat8
        # GEN_FP8_TE=1 (default) also quantizes text_encoder_2 (T5-XXL, ~9 GB) to
        # halve its footprint. GEN_FP8_TE=0 leaves it in the pipeline dtype: this
        # isolates whether quantizing the text encoder is corrupting the prompt
        # conditioning (which would scramble most prompts into noise) vs the
        # transformer being the culprit. Skipping it costs VRAM but model offload
        # still streams it.
        quant_te = os.environ.get("GEN_FP8_TE", "1") == "1"
        print(f">>> fp8: quantizing transformer{' + text_encoder_2' if quant_te else ' only (TE left in {})'.format(dtype_name)} ({qname})...", flush=True)
        quantize(pipe.transformer, weights=qtype)
        freeze(pipe.transformer)
        if quant_te and getattr(pipe, "text_encoder_2", None) is not None:
            quantize(pipe.text_encoder_2, weights=qtype)
            freeze(pipe.text_encoder_2)
        print(">>> fp8: quantization complete", flush=True)
    except Exception as e:
        print(f"!!! fp8 quantization failed: {e}", file=sys.stderr, flush=True)
        sys.exit(4)

# The FLUX transformer produces valid latents on RDNA4, but the VAE *decode* in
# bf16/fp16 on the GPU yields NaNs on gfx1201 (ROCm 7.2) -> a black image. When
# GEN_VAE_FP32=1 (default) we decode the latents on the CPU in fp32: the VAE is
# tiny, CPU fp32 decode is fast and numerically bulletproof, and it sidesteps
# both the bad GPU kernel and the offload dtype hooks. GEN_VAE_FP32=0 decodes
# on-device (the pipeline's normal path).
VAE_CPU_FP32 = os.environ.get("GEN_VAE_FP32", "1") == "1"
try:
    pipe.vae.enable_tiling()
except Exception:
    pass

# For CPU decode, load a SEPARATE, unhooked fp32 VAE pinned to the CPU. Reusing
# pipe.vae is unreliable: under model/sequential offload it carries accelerate
# hooks (and under fp8 its placement is hook-managed), so a manual .to("cpu")
# is silently ignored and the conv weights stay on the GPU while the latents are
# on the CPU -> "Input type (torch.FloatTensor) and weight type
# (torch.cuda.FloatTensor) should be the same". A fresh CPU VAE has no hooks.
_cpu_vae = None
if VAE_CPU_FP32:
    from diffusers import AutoencoderKL
    _cpu_vae = AutoencoderKL.from_pretrained(
        MODEL_REF, subfolder="vae",
        torch_dtype=torch.float32, cache_dir="/models/hf",
        local_files_only=OFFLINE,
    ).to("cpu").eval()

def decode_latents_cpu(latents):
    import numpy as np
    from PIL import Image
    vcfg = _cpu_vae.config
    lat = latents.detach().to("cpu", torch.float32)
    # FLUX packs latents; unpack to the VAE's spatial layout.
    vae_scale = 2 ** (len(vcfg.block_out_channels) - 1)
    lat = pipe._unpack_latents(lat, size, size, vae_scale)
    lat = (lat / vcfg.scaling_factor) + getattr(vcfg, "shift_factor", 0.0)
    with torch.no_grad():
        img = _cpu_vae.decode(lat, return_dict=False)[0]
    img = (img / 2 + 0.5).clamp(0, 1)
    arr = (img[0].permute(1, 2, 0).float().numpy() * 255).round().astype("uint8")
    return Image.fromarray(arr)

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
    if VAE_CPU_FP32:
        latents = pipe(
            prompt,
            height=size, width=size,
            num_inference_steps=steps,
            guidance_scale=0.0,      # schnell is guidance-distilled
            generator=g,
            output_type="latent",
        ).images
        img = decode_latents_cpu(latents)
    else:
        img = pipe(
            prompt,
            height=size, width=size,
            num_inference_steps=steps,
            guidance_scale=0.0,
            generator=g,
        ).images[0]
    out = os.path.join(outdir, f"{name}.png")
    img.save(out)
    print(f"    [{i+1}/{len(rows)}] {name} -> {out}", flush=True)
print(">>> generation done", flush=True)
