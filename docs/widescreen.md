# Widescreen, QHD, and ultrawide support

Jedi Outcast shipped in a 4:3 world. On a modern 16:9, 21:9, or 32:9 display
the stock engine stretches everything horizontally, and its resolution list
tops out at 2048×1536. This project adds proper high-resolution and
wide-aspect support to the singleplayer (and co-op) engine.

There are two independent parts to "widescreen", and it helps to keep them
separate:

1. **The 3D world view** — the rendered scene. Getting this right is a matter
   of the *field of view*: on a wider screen you want to see *more* to the
   sides (Hor+), not a vertically-cropped, zoomed-in view.
2. **The 2D layer** — the HUD, menus, briefing screens, and full-screen
   overlays (binocular/scope masks, letterbox). These are authored in a fixed
   640×480 virtual canvas. Stretching that canvas to fill a wide framebuffer
   distorts every element — round reticles become ovals, text stretches, the
   HUD spreads to the far corners.

## What this project changes

### 2D aspect correction (`r_aspectCorrect2D`, default on)

The renderer used to map the 640×480 2D canvas across the *entire*
framebuffer with a single `glOrtho(0, 640, 480, 0, …)`. We now map that canvas
to a **centred 4:3 region** of the framebuffer and leave the extra width as
black pillarbox bars, so the HUD and menus keep their intended proportions on
any aspect ratio. On a taller-than-4:3 window it letterboxes instead.

| cvar | default | meaning |
|---|---|---|
| `r_aspectCorrect2D` | `1` | `1` = pillarbox/letterbox the 2D layer to 4:3 (correct proportions). `0` = legacy full-framebuffer stretch. |

`r_aspectCorrect2D` is a **latched, archived** renderer cvar: change it from
the console and run `vid_restart` (or restart the game) for it to take effect.

This only affects the 2D layer. The 3D world still fills the whole screen.

### HUD edge anchoring (`cg_hudEdgeAnchor`, default on)

Pillarboxing the 2D layer keeps menus and reticles correctly proportioned, but
it has one unwanted side effect: the corner HUD widgets (health bottom-left,
force/ammo bottom-right) ride inboard with the pillarbox band instead of sitting
at the true screen corners. On a 21:9 or 32:9 display that leaves them floating
well away from the edges.

With `cg_hudEdgeAnchor 1` the corner HUD clusters are anchored to the real screen
edges while everything else in the 2D layer — menus, briefings, the crosshair,
the centre "objective updated" text, scope/binocular masks — stays pillarboxed
and correctly proportioned. Only the two corner clusters move; their health,
armor, force, and ammo number readouts follow their frames automatically.

| cvar | default | meaning |
|---|---|---|
| `cg_hudEdgeAnchor` | `1` | `1` = anchor the corner HUD to the true screen edges on widescreen. `0` = keep it pillarboxed with the rest of the 2D layer. |

`cg_hudEdgeAnchor` is a **cgame** cvar (archived); it takes effect on the next
frame, no `vid_restart` needed. It only does anything when `r_aspectCorrect2D`
is on and the display is wider than 4:3 — on a 4:3 screen, or with aspect
correction off, there is no pillarbox to escape and the cvar is a no-op.

Under the hood, the corner HUD draws are tagged to replay under a full-width,
square-pixel 2D projection so the widgets land exactly at the edges without
stretching. The tag rides each draw command into the deferred render backend, so
the HUD paint's brief widescreen bracket is honoured even though the 2D command
list is replayed after the cgame frame returns.

### Widescreen field of view (`cg_fovAspectAdjust`)

The engine already contains a correct Hor+ FOV adjustment; it is simply off by
default (Raven's default). Turn it on for a proper wide view:

```
cg_fovAspectAdjust 1
cg_fov 80              // your base (4:3-equivalent) horizontal FOV; 80–100 is typical
```

With `cg_fovAspectAdjust 1`, the horizontal FOV is widened for your display's
aspect ratio using `atan(tan(fov/2) · aspect / (4/3)) · 2`, so a 21:9 screen
shows more of the scene at the sides rather than a stretched or zoomed image.
`cg_fov` is what you'd want on a 4:3 monitor; the engine scales up from there.

We deliberately **do not** force `cg_fovAspectAdjust` on, because Hor+ vs the
stock vertical FOV is a matter of taste and it is a persistent, user-archived
setting — forcing it would override an existing preference. Set it once and it
sticks.

### High-resolution and wide modes

The `r_mode` resolution list now includes modern 16:9, 21:9, 24:10, and 32:9
presets in addition to the originals:

| `r_mode` | resolution | aspect |
|---|---|---|
| 3 | 640×480 | 4:3 |
| 6 | 1024×768 | 4:3 |
| … | … | … |
| 13 | 1280×720 | 16:9 |
| 14 | 1600×900 | 16:9 |
| 15 | 1920×1080 | 16:9 |
| 16 | 2560×1080 | 21:9 |
| 17 | 2560×1440 | 16:9 (QHD) |
| 18 | 3440×1440 | 21:9 |
| 19 | 3840×1600 | 24:10 |
| 20 | 3840×2160 | 16:9 (4K) |
| 21 | 5120×1440 | 32:9 |

You do not have to use a preset. Two other options cover everything:

- **Desktop resolution**: `r_mode -2` uses your current desktop resolution and
  aspect ratio. This is the simplest choice for a fullscreen setup.
- **Custom resolution**: `r_mode -1` with `r_customwidth` / `r_customheight`
  for any size, e.g.

  ```
  r_mode -1
  r_customwidth 3440
  r_customheight 1440
  ```

`r_mode`, `r_customwidth`, and `r_customheight` are latched — run `vid_restart`
or restart after changing them.

### Choosing a mode from the in-game menu (optional)

The engine knows all the modes above, but the Single-Player **SETUP → VIDEO →
"Video Mode"** field is driven by two menu files inside your retail
`assets1.pk3`, and stock Raven builds only list resolutions up to 2048×1536.
So by default the new 16:9/21:9/24:10/32:9 presets are reachable from the
console (`r_mode 17`) but not from that menu dropdown.

`tools/build-widescreen-menu-pk3.sh` adds them to the menu. Like the texture
upscaler, it reads the menu files from **your own** copy of the game, appends the
extra resolution entries, and writes an override pak — it ships none of Raven's
files and never modifies your retail data:

```sh
tools/build-widescreen-menu-pk3.sh
# writes zz-widescreen-menu.pk3 into your base/; remove that one file to undo.
```

After it runs, the "Video Mode" field lists 1280×720 through 5120×1440 (32:9);
selecting one and choosing **APPLY CHANGES** sets `r_mode` and restarts video.
This is purely cosmetic menu plumbing — it does not change what the engine can
render, only what the dropdown offers.

## Recommended setup

For a fullscreen ultrawide (e.g. 3440×1440) experience:

```
r_fullscreen 1
r_mode -2                 // or -1 with r_customwidth/height, or a preset like 18
cg_fovAspectAdjust 1
cg_fov 90
// r_aspectCorrect2D is already 1 by default
vid_restart
```

You get a full-width, correctly-proportioned 3D view; menus stay pillarboxed to
4:3, and the corner HUD is anchored to the true screen edges (`cg_hudEdgeAnchor`
is on by default).

## Notes and limitations

- The 2D layer is genuinely 4:3 art, so pillarbox bars on the sides of menus are
  expected and correct — that is the non-distorting result. A few full-screen
  cinematic overlays (scope/binocular masks) are part of the same 2D layer and
  are therefore also pillarboxed. The corner HUD is the one exception: it is
  anchored to the true screen edges by default (`cg_hudEdgeAnchor`), since its
  widgets are meant to hug the corners rather than sit inside the 4:3 band.
- This is an engine/renderer change; it needs no new assets and does not touch
  your game data.
- Texture resolution is a separate topic — see
  [hires-textures.md](hires-textures.md) for upscaling the world textures
  themselves.
