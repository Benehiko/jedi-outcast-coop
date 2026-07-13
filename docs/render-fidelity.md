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

## The installer preset

The installers write a `base/autoexec_render.cfg`, exec'd from
`autoexec_sp.cfg` on startup (the engine only auto-execs the latter). The
preset is chosen with `--render high|classic` (Linux/macOS) or
`-Render high|classic` (Windows); **`high` is the default**.

`--render high` writes:

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
| `r_ext_multisample` | `8` | 8× MSAA — smooths the stair-stepped polygon edges (ship hulls against sky/rock, crate edges). Latched; the driver falls back to a lower sample count, or none, if it can't provide 8×. |
| `r_swapInterval` | `1` | Vsync on — stops the frame tearing you get with uncapped swaps when moving the camera. Not latched; applies on the next frame. |
| `r_subdivisions` | `1` | Finer patch tessellation — smoother curved geometry. |
| `r_lodbias` | `-2` | Hold higher-detail model LODs at distance. |
| `r_lodscale` | `20` | Push LOD transitions further out. |

`--render classic` pins the same cvars back to their retail engine
defaults, so a machine previously installed with `high` is fully reverted
(rather than left with latched values).

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

- **Upscale** (`--with-upscale`) — Real-ESRGAN hi-res override of your
  retail textures. See [hires-textures.md](hires-textures.md).
- **Textures** (`--with-textures`) — AI-generated material textures. See
  [asset-generation.md](asset-generation.md).

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

- **Preset only:** re-run the installer with `--render classic`
  (`-Render classic` on Windows), or delete
  `base/autoexec_render.cfg`.
- **Single cvar at runtime:** e.g. `r_overBrightBitsSoftware 0` then
  `vid_restart` (or restart the game) for the latched ones.
- **Uninstall** removes `autoexec_render.cfg` along with everything else it
  tracked.
