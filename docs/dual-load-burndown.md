# Dual-load `gent` burn-down log (Workstream A5)

Running record of crash sites hit by the remote client after the dual-load
cgame starts executing (A1–A4 landed). Each entry is a fault or wrong visual
discovered by running the second client under gdb against a host, per the A5
protocol in [tasks.md](tasks.md) and [implementation-plan.md](implementation-plan.md).

Test recipe (note the port-collision trap): a stale MP `openjkded` on the
default 29070 will shadow the host; always confirm the host actually bound
its port (`Opening IP socket: 0.0.0.0:<port>`) and connect to *that* port.
The second client needs assets reachable — it wipes its own `fs_homepath`
(`/tmp/jk2-client2`) for a clean config but points `fs_basepath` at
`~/.local/share/openjo` so `assets*.pk3` and the gamecode `.so` resolve.

## A3 milestone — dual-load fires

The remote client enters the dual-load branch, loads its own copy of
`jospgame`, runs `GetCGameAPI` with the client-safe import table, and
`CL_InitCGameVM` succeeds. The cgame then executes real code all the way
through `CG_Init → CG_GameStateReceived → CG_RegisterSounds`. No import
stub fired before the first crash — the renderer/collision/sound
pass-throughs all resolved. The architecture works; the burn-down begins.

## Discovered work items (crash-driven)

### #1 — `as_preCacheMap` is null in `CG_AS_Register`

- **Site:** `codeJK2/cgame/cg_main.cpp:637`, inside `CG_AS_Register`, at
  `STL_ITERATE( pi, (*as_preCacheMap) )`.
- **Backtrace:** `CG_AS_Register → CG_RegisterSounds → CG_GameStateReceived
  → CG_Init → vmMain → VM_Call → CL_InitCGame`.
- **Cause:** `as_preCacheMap` (the ambient-sound-set precache map) is
  populated by the server-side ambient-sound parse; on a serverless remote
  client it is null, and the dereference faults. This is the first true
  `gent`/server-state coupling the burn-down must guard.
- **Status:** fixed — guarded `if ( as_preCacheMap )` in `CG_AS_Register`
  (`cg_main.cpp`), still calling `cgi_AS_ParseSets()`.

### #2 — `com_buildScript` null in `CG_RegisterGraphics`

- **Site:** `codeJK2/cgame/cg_main.cpp:1479`, `if (com_buildScript->integer)`.
- **Cause:** `com_buildScript` is the game library's own cvar global, set in
  `InitGame` via `gi.cvar`; the dual-load client never runs `InitGame`.
- **Status:** fixed — `if (com_buildScript && com_buildScript->integer)`,
  matching the engine's own guard at `common.cpp:296`. Build-time flag,
  always 0 at runtime.

### #3 — item parms never loaded (weapon lookup fails)

- **Site:** `codeJK2/cgame/cg_weapons.cpp:82`, `CG_Error( "Couldn't find
  item for weapon ... Need to update Items.dat!" )` — `bg_itemlist` empty.
- **Cause:** `bg_itemlist` is filled by `IT_LoadItemParms()`, called from
  `InitGame`; the dual-load client skipped it. The follow-on crash was in
  `G_Alloc` (`g_mem.cpp:40`) reading the unregistered `g_debugalloc` cvar,
  because `G_InitMemory()` had not run either.
- **Status:** fixed — `GetCGameAPI` now calls `G_InitMemory()` then
  `IT_LoadItemParms()` after `GI_Init`, mirroring `InitGame`'s order
  (both client-safe: cvar registration + FS read/parse, no server state).

### #4 — view weapon needs the local player's gentity

- **Site:** `codeJK2/cgame/cg_weapons.cpp` `CG_AddViewWeapon`, first at
  line ~1057 (`cent->gent->client->clientInfo`), plus force-power and
  ghoul2/renderInfo reads throughout.
- **Cause:** the first-person viewmodel is rendered almost entirely from the
  local player's server `gentity` (`renderInfo`, ghoul2 model,
  `lowerLumbarBone`), which is null on a serverless client.
- **Status:** guarded — `CG_AddViewWeapon` early-returns when
  `!cent->gent || !cent->gent->client`. The viewmodel is skipped this frame
  (future work: a snapshot-backed viewmodel); the world still renders.

### #5 — HUD force-power / debug / goggles read the gentity

- **Site:** `codeJK2/cgame/cg_draw.cpp` `CG_DrawForcePower` (:289),
  `CG_DrawHUD` debug force draw (:796), LA-goggles `lightLevel` (:1157).
- **Cause:** HUD reads `cent->gent->client->ps.*` for the local player.
- **Status:** fixed — `CG_DrawForcePower` takes a `ps` pointer that falls
  back to `cg.snap->ps` (the same force-power fields arrive in the
  snapshot); the other two sites guarded inline.

### #6 — death-view force-speed / eFlags read the gentity

- **Site:** `codeJK2/cgame/cg_view.cpp:1865` `CG_DrawActiveFrame`,
  `cg_entities[0].gent->client->ps.forcePowersActive` and `gent->s.eFlags`.
- **Cause:** local player force-speed and lock/ATST eFlags read from the
  gentity in the per-frame draw setup.
- **Status:** fixed — locals `localForcePowers`/`localEFlags` fall back to
  `cg.snap->ps.forcePowersActive` / `cg.snap->ps.eFlags`.

## Milestone M1 reached — world renders remotely

With #1–#6 guarded, the remote client runs frames without crashing: it
loads the world (`...loaded 14235 faces...` on the client), the host logs
`Kyle connected`, and a 100-second session renders steadily with no
segfault. The black screen is gone. Two import stubs fire during rendering
without faulting — `inPVS` and `trace` (they return safe defaults) — logged
as future work items below.

### Open stub-backed work items (fire at runtime, no crash yet)

- `inPVS` — cgame calls it during rendering; currently returns qtrue. Back
  it with the client collision model (`CM_*`) when culling correctness
  matters.
- `trace` — returns a clear trace (fraction 1). Back with `CM_*` client
  collision when local traces (effects, marks) need real hits.
