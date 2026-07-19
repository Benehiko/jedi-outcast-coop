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
- **Status:** RESOLVED (MVP), patch 0031, issue #45. On the remote client
  `CG_AddViewWeapon` now delegates to `CG_RemoteAddViewWeapon`, which builds the
  first-person gun from the networked `ps->weapon` using only `cg.*` view state
  (`CG_CalculateWeaponPosition` + `cg_gun_*`) — no gentity, ghoul2, renderInfo,
  saber, or muzzle-flash state. The gun is drawn at a static animation frame
  (frame 0): shown but not fire-animated, because the fire→frame mapping needs
  the local player's ghoul2 skeleton the remote client never builds. The host
  path is untouched (guarded on `cg_remoteClient`). Verified live headless:
  joiner's own weapon renders in first person (previously empty hands).
  Remaining future work: fire animation and muzzle flash on the remote viewmodel.

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

## Past M1 — crashes under active play (patch 0012)

M1 was reached with an idle/short session. Playing the remote client
actively — moving, aiming, shooting, with entities falling under gravity —
surfaced two more server-state reads. Both fixed in patch 0012; a live
two-client session then ran for minutes of active play with the remote
client rendering its HUD, viewmodel, and the host player's model (M2 —
weapon + HUD — confirmed on screen).

### #7 — `g_gravity` null in `EvaluateTrajectory`

- **Site:** `codeJK2/game/bg_misc.cpp:464` (`EvaluateTrajectory`) and
  `:517` (`EvaluateTrajectoryDelta`), both `TR_GRAVITY`:
  `result[2] -= ... g_gravity->value ...`.
- **Backtrace:** `EvaluateTrajectory → CG_CalcEntityLerpPositions
  (cg_ents.cpp:1616) → CG_AddCEntity → CG_AddPacketEntities →
  CG_DrawActiveFrame`.
- **Cause:** `g_gravity` is registered by `G_InitGame` (`g_main.cpp:569`),
  server-only; it is null on the serverless remote client, whose cgame
  still lerps `TR_GRAVITY` trajectories for packet entities (falling items,
  bolts, corpses). The first such entity faults the client.
- **Status:** fixed — `g_gravity ? g_gravity->value : DEFAULT_GRAVITY`.
  The fallback equals the cvar's own default (`"800"`), so the host path is
  unchanged. Mirrors MP's `bg_misc.c`, which uses `DEFAULT_GRAVITY` here.

### #8 — `g_entities[0].client` null in the dynamic crosshair scan

- **Site:** `codeJK2/cgame/cg_draw.cpp:1815` in `CG_ScanForCrosshairEntity`,
  `VectorCopy( g_entities[0].client->renderInfo.eyePoint, start )`.
- **Backtrace:** `CG_ScanForCrosshairEntity → ... → CG_DrawActiveFrame`.
  Fires when the dynamic crosshair (`cg_dynamicCrosshair`) traces from the
  local player's eye each frame.
- **Cause:** the "100% accurate" crosshair path traces from the local
  player's gentity — `g_entities[0].client->renderInfo` and
  `CalcMuzzlePoint( &g_entities[0], ... )` — hardcoded to client 0 and
  reading server state. On the remote client the local player has no
  gentity, so `g_entities[0].client` is null.
- **Status:** fixed — gate the accurate path on `g_entities[0].client`;
  when null the code falls through to the existing view-origin path
  (`cg.refdef.vieworg`/`viewaxis`), which is snapshot-derived and valid on
  both host and remote client.

## Resolved: host / partner did not always see the other player (patch 0013)

Reported as "the host can't see the second player or the NPCs, but shooting
them works." Diagnosed with two temporary probes — one in
`CG_AddPacketEntities` dumping each viewer's `cg.snap` entity list, one in
`SV_AddEntitiesVisibleFromPoint` naming the gate that dropped entity 1.

Finding: **this was correct PVS culling, not a render bug.** The probes
showed both players' snapshots are built identically and the render path is
shared; the only always-sent entity was entity 0, because the SP "always
send" test `( ent->svFlags & SVF_BROADCAST || !e )` force-adds *only* the
viewer's own entity 0. Every other entity — the other player and the NPCs —
had to pass the PVS/area cull. When the two players stood in the same map
area they saw each other and all shared NPCs (snapshot of ~105 entities
including `1c`); when they were in different areas the other player was
correctly culled (snapshot of ~13, no `1c`). NPCs "worked" only because
they happened to share the players' PVS leaves.

For co-op this culling is wrong-feeling: partners should not vanish when
they step into the next room. Fix (patch 0013, `sv_snapshot.cpp`): widen
the always-send test from `!e` to `e < MAX_CLIENTS`, so **every connected
player is force-sent to every viewer** regardless of PVS. Unused client
slots are already filtered by the earlier `!ent->inuse` check. This mirrors
MP, which force-sends clients per viewer via `SVF_BROADCASTCLIENTS`
(`codemp/server/sv_snapshot.cpp:443`). NPCs still follow normal PVS.
Verified: a temporary probe confirmed `player 1 force-sent to viewer 0`
every frame across a two-client session, and the host renders the partner
in an adjacent room; loopback regression still exits 0.

## M3 — host players + NPCs render on the remote client (patch 0014)

The remote client now rebuilds and renders other players' and NPCs' ghoul2
models (previously invisible because the SP cgame reads them from the server
gentity a serverless client lacks). Full design + implementation in
[m3-remote-model-plan.md](m3-remote-model-plan.md). Summary: a `cg_remoteClient`
flag; `level.clients` allocated on the remote client; the player model name
networked via a new `model` key in the `CS_PLAYERS` configstring and NPC models
via `s.modelindex`→`CS_MODELS`; the ghoul2 rebuilt client-side through the
existing `G_SetG2PlayerModel`; animation driven from the networked
`entityState.legsAnim`/`torsoAnim`.

**Verified headlessly (2026-07-11)** with a new Xvfb + `screenshot_png` +
ImageMagick harness (`tools/headless-verify.sh`) — this replaced the "headless
can't render" belief; the real blocker had been window focus on the `:1` session
throttling the loop, which a windowless Xvfb avoids. Temporary in-cgame probes
confirmed on the remote client all four characters building their ghoul2 from the
networked model name and entering the render path: players ent#0/#1 model
`kyle`, NPCs ent#354/#356 models `jan`/`kyle` (from `CS_MODELS`), each followed
by `RENDERING character ent#…`. Client frames were confirmed real 3D views
(mean ~0.09, ~13k colours). The probes were removed before the patch was
finalised.

## M4 — sustained two-client soak, no crash

Milestone gate: "both players fight the same stormtrooper for 10 minutes, no
crash." Split into what can and cannot be driven headlessly:

- **Render-stability (headless, PASSED).** `tools/soak-m4.sh` runs the host +
  a dual-load remote client under Xvfb, the client under gdb, with a wall-clock
  duration control and a handful of persistent NPCs. In a 10-minute run
  (2026-07-11) the remote client rendered the host player + NPCs continuously,
  stayed connected the whole time (the host still logged the client's packets at
  the 10-minute teardown), and **crashed zero times** — gdb captured no signal.
  The `inPVS`/`trace` import stubs fired throughout without incident. This
  exercises the whole M3 render path (build + lerp + draw players and NPCs) under
  sustained load, which is the crash surface M4 is about.
- **Active-combat (needs a human).** The literal spec — both players *fighting* —
  requires player input the headless harness cannot inject (no keyboard/mouse
  into the client). A windowed session with a human driving the client is still
  wanted to certify combat-input paths (firing, taking damage, death/respawn as
  the *local* player on the remote client). Render-side death/effects for OTHER
  entities are covered by the soak.

Harness caveats found and worked around: spawning NPCs on a tight loop while the
host player is AFK exhausts the entity pool (`G_Spawn: no free entities`) and
takes the *host* down — a harness artifact, not a remote-client bug; the soak
now spawns a small fixed set. Also, the client goes quiet in its log once its
screenshot chain ends (it just idle-renders) — "survived N s" must be read from
the wall-clock process-alive check, not the log-timestamp span.
