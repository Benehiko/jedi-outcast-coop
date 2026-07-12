# Generating original textures with a local AI model

`tools/generate-textures.sh` creates **original, non-branded surface textures**
with a locally-run, openly-licensed generative model (FLUX.1-schnell), and
packs them into a pk3 you can ship or use. This is distinct from
[hires-textures.md](hires-textures.md), which *upscales your own retail art* and
therefore only ever runs on your own copy.

## Why these assets are safe to ship — and where the limits are

Two separate legal questions apply to AI-generated game assets. This tool is
designed to stay on the safe side of both; understand them before you extend it.

### Copyright — handled by the model license

The output's copyright status flows from the model you generate with.
**FLUX.1-schnell is released under Apache-2.0**, which "can be used for personal,
scientific, and commercial purposes" and places **no restrictions on generated
outputs**. So images produced with it are original works with no upstream
copyleft or use restriction, and may be redistributed — including bundled in a
GPLv2 project.

Note two things:

- Some other popular image models are **not** safe to ship from. Stable
  Diffusion's CreativeML OpenRAIL-M carries use restrictions that are
  incompatible with GPL redistribution, and FLUX.1-**dev** is a *non-commercial*
  license. Only use a model whose license clearly permits redistribution of
  outputs — schnell (Apache-2.0) qualifies; those do not. If you change
  `--image`/the model, re-check its license.
- In several jurisdictions (notably the US) a purely AI-generated image with no
  meaningful human authorship may **not be copyrightable at all**. That is fine
  for *shipping* generic material — nobody needs to own a concrete texture — but
  it means you cannot claim exclusive rights over the output the way the rest of
  this repo's original work (patches, the Co-op UI) is licensed. Treat generated
  material as unencumbered content, not as your copyrighted work.

### Trademark — handled by *what you generate*

Copyright is not the real exposure for a *Star Wars* game; **trademark and trade
dress are.** Depicting a stormtrooper, a lightsaber, Imperial insignia, a named
character, or the recognizable look of Jedi Outcast implicates Lucasfilm/Disney
marks **regardless of who or what authored the pixels.** This project's whole
posture — engine patches, no game data, unofficial fan project — depends on
staying clear of that line (see the README's trademark section).

So the built-in prompt manifest is deliberately limited to **generic materials**
— metal, concrete, rock, rust, fabric, panels — with explicit "no logos, no
text, no symbols" phrasing, and the output is packed under a neutral
`textures/generated/` path rather than overwriting Raven's specific textures.
The result is non-branded material that is not passing itself off as, and does
not depict, anything from the franchise.

**If you edit the prompts, keep both properties:** materials only, nothing
recognizably from the game or Star Wars, no insignia, no characters, no ships,
no lettering. The moment an asset is recognizably Star Wars, the model license
no longer saves you — trademark governs.

## Requirements

- A **container runtime** (`nerdctl` or `podman`). The model runs in an
  ephemeral container; nothing is installed on the host for it.
- A **GPU**:
  - **AMD (ROCm)** — the default. Needs a ROCm-capable card and the ROCm
    userspace in the image (the script passes `/dev/kfd` + `/dev/dri`). RDNA4
    (e.g. RX 9070) is supported from ROCm 7.2 onward.
  - **NVIDIA (CUDA)** — pass `--cuda` (needs the NVIDIA container toolkit).
- **VRAM**: FLUX.1-schnell in bf16 wants a lot (~ high-teens GB) for 1024²; if
  you are tight, generate at `--size 512` or use an fp8 build of the weights.
- **Disk**: the weights are tens of GB and cache under `--model-dir`
  (default `~/.cache/flux-schnell`) so re-runs don't re-download. Point
  `TMPDIR` at a roomy disk if `/tmp` is a small tmpfs.

## The model and its access token (reference)

**Model:** [`black-forest-labs/FLUX.1-schnell`](https://huggingface.co/black-forest-labs/FLUX.1-schnell)
— a few-step, guidance-distilled text-to-image model, **Apache-2.0**. It is the
one used here because that license permits shipping the output (see above).

**It is a *gated* Hugging Face repo.** Apache-2.0 governs the *use* of the model
and its output, but Black Forest Labs still puts the weights behind a gate: you
must accept the terms on the model page **and** download with a Hugging Face
access token. Without a token the download fails with **HTTP 401 Unauthorized**.
One-time setup:

1. Sign in to Hugging Face and open the model page above; accept the license so
   your account is granted access.
2. Create a **read**-scoped token at
   <https://huggingface.co/settings/tokens>.
3. Provide it to the tool as the `HF_TOKEN` environment variable — the script
   passes it into the container (as `HF_TOKEN` + `HUGGING_FACE_HUB_TOKEN`) only
   for the download; it is never written to the image, the pak, or the logs:

   ```sh
   export HF_TOKEN=hf_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
   tools/generate-textures.sh
   ```

   Treat the token as a secret. Prefer exporting it in your shell over putting it
   on the command line. Revoke it from the tokens page if it is ever exposed.

**Weights are cached and reused.** The download lands in `--model-dir`
(default `~/.cache/flux-schnell`) and is **not** deleted between runs — only the
scratch work dir is cleaned up. The first run downloads ~24 GB (bf16); every run
after reuses the cache and starts generating immediately. Delete that directory
by hand only if you want to reclaim the space.

### VRAM placement (`GEN_VRAM_MODE`)

FLUX.1-schnell in bf16 needs roughly **24 GB** to run entirely on the GPU. The
tool picks a placement automatically from the detected VRAM, and you can force it
with `GEN_VRAM_MODE`:

| Mode | Peak VRAM | Trade-off |
|---|---|---|
| `full` | ~24 GB | Fastest; needs a ≥24 GB card. |
| `model` | ~16 GB, but the transformer stays resident so it can still spike over 16 GB at denoise | `enable_model_cpu_offload` — submodules stream to the GPU. |
| `sequential` | ~4–8 GB VRAM, but moves the **whole model into host RAM** (~24 GB) | `enable_sequential_cpu_offload` — lowest VRAM, slowest, and RAM-hungry on the host. |

```sh
GEN_VRAM_MODE=sequential tools/generate-textures.sh   # lowest VRAM
```

### Known limitation on 16 GB RDNA4 (measured)

On a Radeon **RX 9070** (gfx1201, **16 GB**) with the `rocm/pytorch:latest`
image (torch `2.10.0+rocm7.2`), `/dev/kfd` + `/dev/dri` passed through, the model
downloads and the pipeline loads and *starts* generating, but 16 GB is not enough
to finish a bf16 run:

- **bf16 `full` / `model`** OOM the GPU at denoise (the transformer alone is
  ~12 GB bf16, plus activations).
- **bf16 `sequential`** fits the GPU but needs ~24 GB host RAM for the offloaded
  model, and — critically — repeatedly **hard-faults the GPU** mid-denoise on
  gfx1201 ("Memory access fault by GPU node-1 … Page not present", GPU
  coredump). This can also corrupt the running desktop; avoid it on RDNA4.
- The **VAE decode** in bf16/fp16 on gfx1201 produces **NaNs → a black image**
  (the transformer's latents are fine; only the decode is bad). The tool works
  around this by default (`GEN_VAE_FP32=1`): it decodes the latents on the CPU
  in fp32 with a separate, unhooked VAE — numerically bulletproof.
- The tool sets `TORCH_BLAS_PREFER_HIPBLASLT=0` by default because RDNA4's fused
  `hipBLASLt` path throws `HIPBLAS_STATUS_*` errors mid-generation.

**The most reliable path on 16 GB is `GEN_FP8=1`** (built in): it quantizes the
transformer + T5 text encoder to fp8 with `optimum-quanto`, roughly halving the
transformer so it fits in `model` offload **without** the fault-prone sequential
path. In testing on the RX 9070 this cleared the transformer denoise with no GPU
fault. Combine with the default CPU fp32 VAE decode:

```sh
GEN_FP8=1 GEN_VRAM_MODE=model tools/generate-textures.sh --size 512
```

Tuning knobs (all environment variables):

| Var | Default | Purpose |
|---|---|---|
| `GEN_FP8` | `0` | `1` = fp8-quantize the transformer/T5 (fits 16 GB, avoids offload faults). |
| `GEN_VRAM_MODE` | `auto` | `full` / `model` / `sequential` placement (see table above). |
| `GEN_VAE_FP32` | `1` | `1` = CPU fp32 VAE decode (fixes the gfx1201 black-image NaN). |
| `GEN_DTYPE` | `bf16` | `bf16` / `fp16` / `fp32` compute dtype. |
| `GEN_ATTN` | `math` | `math` = eager SDPA (safe); `auto` = torch default. |
| `TORCH_BLAS_PREFER_HIPBLASLT` | `0` | `0` avoids RDNA4's flaky fused BLAS. |

Other routes to a finished pack: a **≥24 GB** GPU (`full`, no offload), or
`GEN_VRAM_MODE=sequential` on **non-RDNA4** hardware with ≥ ~28 GB free host RAM.

RDNA4 needs ROCm ≥ 7.2 either way. On NVIDIA, use `--cuda` with the NVIDIA
container toolkit; a 16 GB+ NVIDIA card runs `model` offload comfortably and
24 GB runs `full`.

> **Status on 16 GB RDNA4:** as of this writing the fp8 path clears the
> transformer without faulting but a full end-to-end pack had not yet been
> produced on the 16 GB RX 9070 test card (generation was stopped to avoid
> further GPU faults destabilising the desktop). fp8 is the recommended path;
> a ≥24 GB or NVIDIA card is the sure one.

## Usage

```sh
# preview the manifest + settings without generating anything:
tools/generate-textures.sh --dry-run

# generate the built-in generic material set (AMD/ROCm, 1024², 4 steps):
tools/generate-textures.sh

# NVIDIA instead:
tools/generate-textures.sh --cuda

# smaller/faster (lower VRAM):
tools/generate-textures.sh --size 512

# your own material prompts (name|prompt per line), still non-branded:
tools/generate-textures.sh --manifest my-materials.txt
```

Key options (`--help` lists all):

| Option | Meaning |
|---|---|
| `--size N` | Square power-of-two size (default 1024). |
| `--steps N` | Diffusion steps (schnell is a few-step model; default 4). |
| `--seed N` | Base seed; per-texture seeds derive from it, so runs are reproducible. |
| `--manifest FILE` | Use your own `name|prompt` list instead of the built-in set. |
| `--cuda` | NVIDIA backend instead of ROCm. |
| `--model-dir DIR` | Where to cache weights (default `~/.cache/flux-schnell`, reused across runs, not deleted). |
| `--dry-run` | Show what would be generated, generate nothing. |

The gated-model token is supplied via the `HF_TOKEN` environment variable (see
[the model + token reference](#the-model-and-its-access-token-reference)
above), not a flag.

The output pak (`zzz-generated-textures.pk3`) contains
`textures/generated/<name>.jpg`. Reference those paths from your own shaders or
maps; the pak sorts after `assets*.pk3` so its paths resolve normally. Remove it
by deleting the one file.

## Engine constraint

Textures must be power-of-two (the renderer aborts otherwise). The tool
generates at a power-of-two size and re-snaps after, so this always holds — the
same rule the upscaler handles.

## Provenance / attribution

If you ship a pack generated this way, record how it was made — model
(FLUX.1-schnell, Apache-2.0), that the prompts are generic materials, and the
seed — so the pack's origin is auditable. A short `README` inside the pack or a
note in your release is enough.
