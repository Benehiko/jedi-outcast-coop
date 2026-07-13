# Widescreen HUD — command-stream plan

## Problem recap

On a display wider than 4:3, `r_aspectCorrect2D` (patch 0022) pillarboxes the
entire 2D layer to a centred 4:3 region so menus keep their proportions. The
side effect is that the corner HUD widgets (health bottom-left, ammo
bottom-right) ride inboard with the pillarbox band instead of sitting at the
true screen corners.

Goal: the HUD frames anchor to the real screen edges, while everything else in
the 2D layer (menus, briefings, crosshair, zoom masks, full-screen shaders)
stays pillarboxed and correctly proportioned.

## What the prototype proved

A throwaway prototype (cvar-gated wide ortho + cgame x-shift, since reverted)
established:

1. **The engine math is correct.** A full-width, square-scaled 2D ortho with
   horizontal virtual extent `640 * (aspect / (4/3))`, centred so virtual X runs
   `[-halfExtra, 640 + halfExtra]`, renders the 2D layer edge-to-edge with square
   pixels (no stretch). Verified headless at 2560×1080.
2. **The cgame shift is correct.** `cgs.glconfig.vidWidth/Height` gives the real
   aspect; shifting the left HUD X by `-halfExtra` and the right HUD X by
   `+halfExtra` moves the widgets into the former pillarbox bars.
3. **A cvar cannot carry per-draw state.** The 2D layer is recorded as a command
   list and replayed in the backend *after* cgame returns. A cvar toggled on
   then off within `CG_DrawHUD` reads its final value (off) for every replayed
   draw, so the HUD drew with the wrong ortho and disappeared off-screen.

Conclusion: the "wide" flag must live **in the draw command stream**, not in a
cvar or a shared `backEnd` field mutated outside the replay.

## Design

Carry a per-command `wide` flag on the stretch-pic (and rotate-pic) command, set
by a small cgame→engine hook that brackets the HUD paint. `RB_SetGL2D` chooses
the ortho from the flag on the command currently being replayed.

### Engine (rd-vanilla)

1. **`tr_local.h`** — add `int wide;` to `stretchPicCommand_t` (and
   `rotatePicCommand_t` if the HUD uses rotated pics; the radar dial does, so
   include it). Add a matching field to `backEndState_t`:
   `qboolean current2Dwide;` — the ortho currently programmed.

2. **`tr_backend.cpp` — `RB_SetGL2D`** — take the desired mode from a parameter
   or a `backEnd`-scoped variable set immediately before the call (not a cvar).
   Refactor to `RB_SetGL2D( qboolean wide )`:
   - `wide == qfalse`: the existing pillarboxed 4:3 path (unchanged).
   - `wide == qtrue`: full-width viewport, `qglOrtho(-halfExtra, 640+halfExtra,
     480, 0, 0, 1)` with `halfExtra = (640*(aspect/target) - 640) * 0.5` for
     `aspect > target`; for `aspect <= target` fall through to the normal path
     (nothing to widen). Record `backEnd.current2Dwide = wide`.

3. **`tr_backend.cpp` — `RB_StretchPic` / `RB_RotatePic`** — replace the
   `if ( !backEnd.projection2D ) RB_SetGL2D();` guard with one that also
   re-runs setup when the mode changes mid-batch:
   ```
   if ( !backEnd.projection2D || backEnd.current2Dwide != cmd->wide ) {
       if ( tess.numIndexes ) RB_EndSurface();   // flush the batch at the old ortho
       RB_SetGL2D( (qboolean)cmd->wide );
   }
   ```
   Flushing before the ortho switch is required — verts already batched belong to
   the previous projection.

4. **`tr_cmds.cpp` — `RE_StretchPic` / `RE_RotatePic`** — set `cmd->wide` from a
   renderer-global `int r_2Dwide` (a plain backend-writable int, default 0) that
   the new export toggles. Keep the existing signature so no other caller
   changes.

5. **New export** — add `void (*Set2DWide)( qboolean wide );` to the refexport
   table in `tr_public.h`, implemented in `tr_cmds.cpp` (or `tr_init.cpp`) as
   `void RE_Set2DWide( qboolean w ){ r_2Dwide = w ? 1 : 0; }`. Wire it into the
   export struct in `RE_GetRefAPI` (`tr_init.cpp`).

### Engine↔cgame bridge

6. **cgame syscall** — JK2 SP cgame reaches the renderer through the `cgi_*`
   import table, not numeric syscalls for render ops (`cgi_R_DrawStretchPic` maps
   to `re.DrawStretchPic`). Add `cgi_R_Set2DWide` to `cg_syscalls.cpp` calling
   the new export, and declare it in `cg_local.h` / the cgame import glue. Follow
   exactly how `cgi_R_SetColor` is plumbed (same file, same pattern).

### cgame (codeJK2)

7. **`cg_draw.cpp` — `CG_DrawHUD`** — compute `hudShift` from
   `cgs.glconfig` (the prototype formula). When `hudShift > 0.5`:
   - `cgi_R_Set2DWide( qtrue );`
   - draw left HUD with `x -= hudShift`, right HUD with `x += hudShift`;
   - `cgi_R_Set2DWide( qfalse );` before returning.
   Because the flag now rides each command, the on/off brackets are recorded in
   order and honoured at replay — the failure mode the prototype hit.

### cvar

8. `cg_hudEdgeAnchor` (cgame, `CVAR_ARCHIVE`, default `1`). `CG_DrawHUD` only
   applies the shift + wide flag when it's set, so users can restore the
   pillarboxed HUD. (Engine `r_2Dwide` stays an internal mechanism, not user
   facing.)

## What must NOT change

- The pillarboxed path for menus, briefings, crosshair, weapon-select, zoom
  masks, and full-screen shaders (`CG_DrawPic(0,0,640,480,…)`). Those never call
  `Set2DWide(qtrue)`, so they replay under the normal 4:3 ortho.
- `r_aspectCorrect2D` semantics and default. The wide HUD is layered on top; with
  `r_aspectCorrect2D 0` (legacy stretch) the shift is a no-op worth guarding
  (skip when aspect correction is off, since there's no pillarbox to escape).

## Verification

- Headless at several aspects — `+set r_customwidth/​height` for 1920×1080 (16:9),
  2560×1080 (21:9), 3840×1080 (32:9), and a 4:3 control (1600×1200, shift must be
  zero → HUD unchanged). Confirm: HUD frames at the true corners; the "Datapad
  updated" centre text and the crosshair stay centred/pillarboxed; open a menu
  (`+uimenu ingame`) and confirm it is still pillarboxed, not stretched.
- Batch-flush correctness: watch for a one-frame smear or a missing HUD element
  at the ortho switch (means a batch wasn't flushed before `RB_SetGL2D`).
- Build the JK2SP engine + gamecode + renderer; run the co-op smoke path.

## Scope / files

New patch `0026-widescreen-hud-edge-anchor.patch`:

- `code/rd-vanilla/tr_local.h` — command + backend fields
- `code/rd-vanilla/tr_backend.cpp` — `RB_SetGL2D(wide)`, batch-aware guard
- `code/rd-vanilla/tr_cmds.cpp` — `cmd->wide`, `RE_Set2DWide`, `r_2Dwide`
- `code/rd-vanilla/tr_init.cpp` — export wiring
- `code/rd-common/tr_public.h` — `Set2DWide` export
- `codeJK2/cgame/cg_syscalls.cpp` + import glue — `cgi_R_Set2DWide`
- `codeJK2/cgame/cg_draw.cpp` — `CG_DrawHUD` shift + brackets, `cg_hudEdgeAnchor`
- `codeJK2/cgame/cg_main.cpp` — register `cg_hudEdgeAnchor`

Docs: fold into `docs/widescreen.md` (a "HUD edge anchoring" section) once
landed, and delete this plan file.

## Open questions

- Does any HUD element use `RE_RotatePic` (the circular radar/force dials look
  rotated)? If so it needs the same `wide` field + guard, or those dials will
  snap back to the pillarboxed ortho. Check `CG_DrawRadar` / the dial draws
  before implementing.
- The force/ammo *number* fields (`CG_DrawNumField`) draw at `x + offset` — they
  inherit the shifted `x`, so they follow the frame automatically. Confirm in the
  first screenshot.
