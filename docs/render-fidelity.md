# Render fidelity

Jedi Outcast's character and world models are surprisingly high-fidelity
— open one of the retail `.glm` models in Blender and it holds up well.
In-game the same asset can read as flat, dark, and soft. This document
explains why, what this project does about it, and how to control it.

The renderer is OpenJK's `rd-vanilla` — the fixed-function OpenGL 1.x
pipeline inherited from the Quake 3 / JK2 era. It is the renderer this
co-op build ships, and it is not being replaced (see
[Why not a modern renderer](#why-not-a-modern-renderer)). Everything here
works *within* that renderer: an engine fix (patch
[`0024-render-fidelity`](../patches/0024-render-fidelity.patch)) plus a
cvar preset the installers write.

## Why models look worse in-game than in Blender

Blender shows per-pixel PBR shading: real surface normals, roughness,
area lights, ambient occlusion. `rd-vanilla` cannot do any of that. The
gap comes from several places at once:

- **Per-vertex (Gouraud) lighting, not per-pixel.** GHOUL2 models
  (`.glm` / `.gla`) are lit once per vertex and the result is interpolated
  across each triangle. Fine detail between vertices — and per-pixel
  highlights — is simply not computed.
- **No normal, specular, or PBR maps.** `rd-vanilla` draws a diffuse
  texture modulated by a baked lightmap, and nothing else. The material
  response you see in Blender isn't in the pipeline at all. This is the
  single biggest structural gap, and no cvar closes it.
- **Lossy texture defaults.** Retail runs with DXT/S3TC texture
  compression (color banding and blur) and, on lower settings, `picmip`
  downsampling.
- **Low-resolution lightmaps.** World lighting is baked into small
  per-surface lightmaps — soft and blocky next to Blender's high-res GI.
- **Overbright silently disabled on modern setups.** This is the big,
  fixable one — see below.

### The overbright problem

Original JK2 rendered lightmaps with a hardware **overbright** step: the
lightmap contribution is effectively multiplied so lit surfaces can punch
above mid-grey. It's a large part of why the retail game looks "lit"
rather than evenly grey.

`rd-vanilla` implements overbright by scaling texture/lightmap colors
*down* by half at upload time (`identityLight`) and then doubling the
final image back up **through the hardware gamma ramp** at display. That
second half only works if the OS hands the engine a hardware gamma ramp
*and* the game is fullscreen. So the stock code does this
(`code/rd-vanilla/tr_image.cpp`, `R_SetColorMappings`):

```c
tr.overbrightBits = r_overBrightBits->integer;
if ( !glConfig.deviceSupportsGamma ) tr.overbrightBits = 0; // need hw gamma
if ( !glConfig.isFullscreen )        tr.overbrightBits = 0; // never in a window
```

On a modern Linux desktop the gamma ramp usually isn't available —
`sdl_window.cpp` sets `deviceSupportsGamma` from `SDL_SetWindowBrightness()`,
which **fails on Wayland and most compositors** — and many people run
borderless/windowed. In both cases overbright is forced to `0` and the
world/model lighting goes flat and dark. That's the biggest reason the
in-game image looks worse than the source assets on a current machine.

## The fix: software overbright (patch 0024, render fidelity)

The engine already has everything needed to apply overbright *without* the
hardware gamma ramp. When no hardware gamma is available,
`R_LightScaleTexture` bakes the gamma table — which already includes the
overbright `<< shift` — straight into textures and lightmaps at upload
time. The only thing stopping it was the unconditional "force off" above.

Patch
[`0024-render-fidelity.patch`](../patches/0024-render-fidelity.patch)
adds a cvar that gates that fallback:

| Cvar | Default | Meaning |
|---|---|---|
| `r_overBrightBitsSoftware` | `0` | `0` = classic behavior (overbright needs a hardware gamma ramp and a fullscreen surface, else it's off). `1` = keep overbright active regardless, delivered through the texture-upload gamma table. Latched (`vid_restart` / restart to apply). |

With it set, `r_overBrightBits 1` restores the retail lighting punch on
Wayland and in windowed mode, where it was previously dead. It changes
nothing on setups that already had a working hardware gamma ramp.

One subtlety on the software path: overbright scales textures *down* by
half at upload, and `R_ColorShiftLightingBytes` only boosts lightmaps by
`max(0, r_mapOverBrightBits - r_overBrightBits)`. With both at `1` that
boost is zero, so the scene just comes out dark. The preset therefore uses
`r_mapOverBrightBits 2` (one step above `r_overBrightBits 1`), which
restores the lightmap boost and gives lit surfaces their punch back —
verified headless (kejim_post, windowed): a flat mean-0.20 baseline
becomes a punchy mean-0.29 with visibly brighter, higher-contrast rock and
hull lighting.

Because textures are scaled at load, this is a latched cvar — it takes
effect on the next engine start, before the first map loads.

### Keeping colour on the software path (hue-preserving overbright)

There's a subtlety in *how* the overbright brightening is applied at upload.
The classic path bakes the overbright `<< shift` straight into the per-channel
gamma table (`s_gammatable`) and looks each channel up independently, clamping
at 255. That per-channel clamp is fine for the hardware-gamma ramp (it
brightens the whole framebuffer at display, so it can't change one texel's
colour) — but on the software path it distorts hue on any **saturated** texel:
the brightest channel hits the 255 ceiling first while the other two keep
climbing, so the colour ratio shifts. On bright, red-dominant **flesh** the red
clamps while green does not, and skin reads a hard **green** — most visible on
NPC faces (e.g. Jan in `kejim_post`) under a lit light-grid cell.

The fix splits the two operations (`code/rd-vanilla/tr_image.cpp`):

- `s_gammatableNoOB` holds **pure gamma** (no overbright shift), used by the
  software texture path. `s_gammatable` keeps the shift baked in for the
  hardware-gamma ramp, which can't distort per-texel hue.
- `R_OverbrightTexel` applies the overbright factor (`1 << overbrightBits`)
  across R/G/B together and, when that would push the brightest channel past
  255, scales the **whole texel** down by that channel so it caps at 255 with
  its exact hue. A saturated texel gives up a little brightness in the roll-off
  instead of picking up a colour cast.

This is purely a change to how `r_overBrightBitsSoftware 1` behaves — there is
no new cvar and the preset is unchanged. Verified headless (`npc spawn jan`,
`kejim_post`): the green face cast is gone while lit surfaces keep their punch.

### Models were still dark: entity lighting

Software overbright fixed the *world* but left character models dark and flat
against the newly-lit floor and walls. The two are lit by different code:

- **World surfaces** are lit by a baked **lightmap texture**. Both the diffuse
  texture and the lightmap are brightened by the overbright `<< shift` at upload,
  so lit surfaces punch above mid-grey.
- **GHOUL2 models** are lit **per vertex** from the light grid
  (`ambient + directed·(N·L)`, in `R_SetupEntityLighting` /
  `RB_CalcDiffuseColor`). That colour modulates the skin at draw time; it never
  passes through a texture upload, so it never received the overbright boost.

In the classic pipeline this was invisible: the hardware gamma ramp doubled the
*entire framebuffer* at display, lifting model vertex colours along with
everything else. The software path bakes that doubling into textures instead —
which does nothing for vertex-lit models. The effect is worst on the parts of a
rounded model that face away from the light (`N·L ≤ 0`), where the pixel falls
back to the ambient term alone; measured with `r_debugLight 1`, that ambient sat
around 40/255 while the lit floor was near full.

Patch
[`0024-render-fidelity.patch`](../patches/0024-render-fidelity.patch)
re-applies the missing factor to entity `ambient`/`directed` light — and lifts
the ambient ceiling from `identityLightByte` to full 255 to match — **only on the
software-overbright path** (`r_overBrightBitsSoftware` on and overbright active).
The classic hardware-gamma path is untouched, since it still gets its doubling at
display. There is no new cvar: models simply track the lit world whenever
software overbright is on. Verified headless (kejim_post, windowed, overbright
preset): frame mean 0.196 → 0.230 with the extra brightness concentrated on the
character models, not a flat wash (stddev 0.113 → 0.153).

### Watch out for saved gamma

The video menu's Brightness slider writes `r_gamma`, and it's easy to end
up with it cranked high (values around `2` are common). A high `r_gamma`
flattens the whole tone curve and **cancels the overbright contrast** — the
picture looks *brighter* but washed-out and milky, with crushed shadows and
lost texture detail. That's the opposite of what overbright is for. The
preset therefore also pins `r_gamma 1.0`; if your display genuinely needs
more brightness, nudge it back up a little (`r_gamma 1.1`–`1.3`) rather than
leaving it at a menu-set `~2`.

## The render-fidelity build + preset

Render fidelity is one of the two patch-backed graphics features (the other is
widescreen). It is controlled by `[graphics] lighting` in your config
(`jk2coop graphics`), which is **on by default**. Because it adds a latched
renderer cvar, toggling it rebuilds the engine on the next `jk2coop install`;
`jk2coop graphics` offers to rebuild immediately when you change it.

The core software-overbright behaviour comes from the patch itself
(`r_overBrightBitsSoftware`). On top of that, `jk2coop install` writes the
companion fidelity cvars into `base/autoexec_sp.cfg` (regenerated from your
config) whenever `lighting` is on; turning it off writes the same cvars back to
their retail defaults, so a machine previously built with lighting is fully
reverted rather than left with latched values. MSAA is a separate, user-controlled
setting in the same menu (`[graphics] msaa` → `r_ext_multisample`) and is written
independently of the preset, so it always reflects your choice.

The high-fidelity preset writes (latched cvars apply after a restart):

| Cvar | Value | Effect |
|---|---|---|
| `r_overBrightBitsSoftware` | `1` | Enable software overbright (render-fidelity patch). |
| `r_overBrightBits` | `1` | Restore lightmap overbright punch. |
| `r_mapOverBrightBits` | `2` | One step above `r_overBrightBits` so lightmaps keep their boost on the software path (see note above). |
| `r_gamma` | `1.0` | Neutral gamma. A high saved value (the video-menu Brightness slider stores it, often ~2) washes the picture out and cancels the overbright contrast. Not latched — raise it live if a display needs it. |
| `r_picmip` | `0` | Full-resolution textures (no mip downsampling). |
| `r_ext_compress_textures` | `0` | Uncompressed textures — no DXT banding/blur. |
| `r_texturebits` | `32` | 32-bit textures — no color banding. |
| `r_ext_texture_filter_anisotropic` | `16` | 16× anisotropic filtering — crisp at grazing angles. |
| `r_textureMode` | `GL_LINEAR_MIPMAP_LINEAR` | Trilinear filtering. |
| `r_swapInterval` | `1` | Vsync on — stops the frame tearing you get with uncapped swaps when moving the camera. Not latched; applies on the next frame. |
| `r_subdivisions` | `1` | Finer patch tessellation — smoother curved geometry. |
| `r_lodbias` | `-2` | Hold higher-detail model LODs at distance. |
| `r_lodscale` | `20` | Push LOD transitions further out. |

MSAA (`r_ext_multisample`) is **not** part of this preset — set it separately
via `jk2coop graphics` (`[graphics] msaa`: 0/2/4/8/16). It is latched.

**Not every GPU/driver can provide every sample count.** On some Mesa/Wayland
setups (radeonsi via SDL2's EGL path) a high sample count makes `eglChooseConfig`
fail to find a matching config; `SDL_CreateWindow` then fails for *every*
resolution and the renderer aborts with the misleading `...ERROR: no display
modes could be found` / `could not load OpenGL subsystem`. The engine does **not**
gracefully step down — it just fails to start.

To prevent that, jk2coop **probes the chosen MSAA level** against the installed
engine: it briefly launches the engine in a throwaway home path and watches for
the EGL/display failure. If the level is unsupported it warns and steps down to
the highest level that works instead (e.g. `16x` → `8x`). This guard runs at
three points, so an unsupported value never reaches the engine:

- **`jk2coop graphics`** — when you save an MSAA change.
- **Any launch** (`jk2coop launch` / `host` / `join`) — the autoexec is refreshed
  from `config.toml` before the game starts, and the clamp runs there. This is the
  catch-all: even a value edited into `config.toml` by hand, or written by an
  older install, is stepped down before the engine sees it. A change is saved back
  to `config.toml` so it sticks and the next launch needs no re-probe.

The probe is best-effort — if the engine isn't built yet or the machine can't be
probed (no display server), your choice is written unchanged rather than
second-guessed. If the game still fails to start after a hand-edit on a machine
that can't be probed, lower `[graphics] msaa` to `8` or below.

Turning `[graphics] lighting` off (via `jk2coop graphics`) rebuilds the engine
without the render-fidelity patch, reverting the overbright behaviour; pin the
companion cvars back to their retail defaults by hand if you set them.

### Display: resolution and fullscreen

The generated `autoexec_sp.cfg` always pins the **custom video mode**
(`r_mode -1`) with an explicit `r_customwidth`/`r_customheight`, rather than
leaving the engine on a saved indexed `r_mode`. This is deliberate: an indexed
mode saved by a previous run (e.g. `r_mode 17` = 2560×1440) can wedge startup on
a display server that can't enumerate that exact mode — the same
`no display modes could be found` failure. Pinning `-1` with a concrete size
sidesteps the engine's indexed-mode list entirely.

- `[graphics] res_width`/`res_height` set the size. `0`×`0` = **auto**, which
  falls back to a small, always-creatable `1280×720` window. Pick a specific
  resolution (or the detected native one) in `jk2coop graphics` for full size.
- `[graphics] fullscreen` controls `r_fullscreen`. It **defaults to `false`
  (windowed)**, which is the reliable choice on Wayland where fullscreen mode
  enumeration is flaky. Tick *Fullscreen* in `jk2coop graphics` to opt in.
- The launch `--windowed` flag still forces windowed for a single run,
  overriding the config.

The extra cost — uncompressed 32-bit full-res textures, anisotropic
filtering, finer tessellation — is trivial on modern GPUs. Everything is a
plain cvar; you can edit `autoexec_render.cfg`, delete it, or override any
value from the console at runtime (latched cvars apply after a restart).

## Assets: the other half of the gap

Cvars, the overbright fix, and better filtering close roughly half the
distance to the Blender view. The rest is asset resolution. Since
`rd-vanilla` can't do normal or specular maps, a **higher-resolution
diffuse texture is the substitute for surface detail**. This project ships
two optional, opt-in texture mods that build override paks from *your own*
retail data (retail files are never modified):

- **Upscale** (`[graphics] texture_upscale`, tier `texture_resolution`) —
  Real-ESRGAN hi-res override of your retail textures at 1K/2K/4K. See
  [hires-textures.md](hires-textures.md).
- **Textures** (`[graphics] texture_generate`) — AI-generated material textures.
  See [asset-generation.md](asset-generation.md).

With `r_picmip 0` and compression off, those higher-res textures are what
actually approach Blender-level surface detail in-game.

## Why not a modern renderer

`rd-rend2` is the modern GLSL renderer for the Jedi Knight engine (HDR,
tonemapping, real normal/specular/PBR maps, dynamic shadows, SSAO). It
would close the structural gap — but it does not fit this project:

- It contains **no JK2 multiplayer code**, and its **JK2 singleplayer path
  is explicitly unfinished/broken** and must be enabled at compile time;
  prebuilt binaries exclude it.
- Its focus and shipped binaries target **Jedi Academy multiplayer**
  (`codemp`).

This co-op build is **JK2 singleplayer (`codeJK2`)**, so rend2 is
effectively not available without a large, unsupported porting effort.
Treat `rd-vanilla` as the renderer and get the gains from the overbright
fix, cvars, better lightmaps, and higher-res textures.

## Reverting

- **Turn lighting off:** set `[graphics] lighting = false` via
  `jk2coop graphics` and rebuild, or remove the companion cvars you set in
  `autoexec_sp.cfg`.
- **Single cvar at runtime:** e.g. `r_overBrightBitsSoftware 0` then
  `vid_restart` (or restart the game) for the latched ones.
- **Uninstall** removes `autoexec_render.cfg` along with everything else it
  tracked.
