# M3 plan — host players and NPCs render on the remote client

Workstream A, milestone M3 ("host player, NPCs, doors render"). Doors already
render (they are brush/bmodel movers driven entirely by the snapshot). What is
missing is **character models** — the host player and NPCs — because the SP
cgame renders them straight out of the server `gentity_t`, which a serverless
remote client does not have.

## Why nothing renders today

In JK2 single-player, cgame and game are the same DLL and share
`g_entities[]`. `CG_LinkCentsToGents` (`codeJK2/cgame/cg_main.cpp:1514`) points
every `cent->gent` at `&g_entities[i]`. On the remote client those gentities
are **zeroed** — never spawned by a server — so:

- `CG_Player` (`codeJK2/cgame/cg_players.cpp:4782`) early-returns at
  `!cent->gent->client` (4801) and `!ci->infoValid` (4821), and the ghoul2
  branch is gated on `cent->gent->ghoul2.size()` (4950) which is 0.
- The rendered model instance is `cent->gent->ghoul2`, wired into the
  refEntity by `CG_SetGhoul2Info` (`cg_ents.cpp:217`,
  `ent->ghoul2 = &cent->gent->ghoul2`). Empty gentity → empty ghoul2 → nothing
  drawn.
- Animation is read from `cent->gent->client->ps.legsAnim/torsoAnim` and
  `renderInfo` fps/timers (`cg_players.cpp:1039,1058,1073`), all gentity state.

So the model identity, the ghoul2 instance, the bolt/bone indices, the
renderInfo and the animation are **all off-network** in stock JK2.

## What the wire already carries

Per-entity `entityState_t` already networks (see `entityStateFields`,
`code/qcommon/msg.cpp:527`): position/angle trajectories, `origin`/`angles`,
`clientNum`, `modelindex`, `frame`, `legsAnim`/`torsoAnim` + timers, `weapon`,
`scale`, `modelScale`, `radius`, `saberActive`. That is enough to place and
animate a character — **except which model to load**.

## The one missing datum: model identity

- **Players:** the model is hardcoded `"kyle"` server-side
  (`g_client.cpp:1515,1828`); the `CS_PLAYERS` configstring writes
  `headModel\\ torsoModel\\ legsModel\\` — all empty (`g_client.cpp:602-611`).
- **NPCs:** the model comes from the `.npc` stats file into the gentity; the
  `NPC_type` string that selects it lives only in `g_entities[i].NPC_type`,
  networked nowhere.

## Chosen architecture — SP-native (rebuild into the gentity)

Rather than port MP's gentity-free render path (a large rewrite of `CG_Player`),
reuse the SP builder `G_SetG2PlayerModel` (`g_client.cpp:1384`). It is already
client-safe — it only calls renderer pass-throughs (`RE_RegisterSkin`,
`G2API_InitGhoul2Model`), configstring index lookups, and FS parses — and its
own comment (line 1388) says it exists "so the client can get it too." Calling
it on the remote client's own zeroed gentity builds a real `ghoul2`, resolves
every bolt/bone index via `G_SetG2PlayerModelInfo`, and sets
`clientInfo.infoValid = qtrue` (`g_client.cpp:1332`). The **entire existing
render path then works unchanged** — the only edits are (a) feed it the model
name from the network and (b) drive animation from the snapshot instead of the
absent server playerState.

MP is the reference for the *data plumbing*: MP carries the player model in the
`CS_PLAYERS` infostring `"model"` key and NPC models in `CS_MODELS + modelindex`
(`codemp/cgame/cg_players.c:1655,7048`). SP already broadcasts `CS_MODELS`, so
the NPC channel exists.

## Prerequisite discovered during implementation — M3.0

`CG_Player`, `CG_NewClientinfo`, and the render path all need
`g_entities[i].client` (a `gclient_t` with `clientInfo`/`renderInfo`) to be
populated. On the remote client that pointer is **null**: `GetCGameAPI` runs
`GI_Init`/`G_InitMemory`/`IT_LoadItemParms` but not `G_InitGame`, so the
`level.clients` allocation and the `g_entities[i].client = level.clients + i`
wiring (`g_main.cpp:654-663`) never happen.

Naively allocating those clients breaks the existing A5 guards, because
**several committed guards (#5 force power, #6 death-view, #8 crosshair) use
`cent->gent->client == NULL` as the "am I the remote client" sentinel** — e.g.
`cg_view.cpp:1871` `haveLocalClient = (localGent && localGent->client)`,
`cg_draw.cpp` crosshair, `cg_weapons.cpp`. If `client` becomes non-null those
branches flip to the gentity path and read a **zeroed** `client->ps` instead of
the correct `cg.snap->ps` — regressing the local player's force-speed FOV,
crosshair, etc.

**M3.0 (do first):** introduce an explicit cgame-scope flag
`qboolean cg_remoteClient` (set once when the dual-load branch initialises the
cgame), and migrate every committed sentinel from "`gent->client` is null" to
"`cg_remoteClient`". Only then allocate `level.clients` on the remote client
(mirroring `g_main.cpp:654-663`, client-safe: one `G_Alloc` + memset) so the
render path has somewhere to build the model. This makes the remote-client
condition explicit and independent of client-pointer nullness.

## Work breakdown (one patch per step unless noted)

### M3.1 — network the player model name  *(gamecode + cgame)*
- Server: in `ClientUserinfoChanged` (`g_client.cpp:602`) write the real model
  name into the `CS_PLAYERS` infostring (`model\kyle\`, or the client's chosen
  model) instead of the empty head/torso/legs keys.
- Client (remote only): when a `CS_PLAYERS` slot arrives or a player entity is
  first seen with a zeroed gentity, read the `model` key and call
  `G_SetG2PlayerModel(gent, model, …)` on that entity's gentity, then let the
  existing path render it.
- Guard everything on the remote-client condition
  (`cg_entities[cg.snap->ps.clientNum].gent->client == NULL`) so the host path
  is untouched.
- Done: host player's Kyle model renders on the remote client; loopback
  regression unaffected.

### M3.2 — drive animation from the snapshot on the remote client  *(cgame)*
- `CG_PlayerAnimation` (`cg_players.cpp:1039`) and the torso path (1073) read
  `gent->client->ps.legsAnim/torsoAnim` + `renderInfo` fps/timers. On the
  remote client fall back to `cent->currentState.legsAnim/torsoAnim` (already
  networked) with a default fps mod.
- Done: the host player's limbs animate (walk/run/aim), not just T-pose.

### M3.3 — network NPC model identity  *(gamecode + wire + cgame)*
- Server: broadcast each NPC's model via `CS_MODELS + modelindex` (the MP
  channel) — or add `NPC_type`/model to a new configstring range — so the
  client can identify the model. Confirm SP's `modelindex` is set for NPCs and
  reaches the client.
- Client (remote only): when an NPC entity (`EF_NPC`) is first seen, build its
  ghoul2 from that model name via `G_SetG2PlayerModel` on its gentity.
- Done: host NPCs (stormtroopers on kejim_post) render on the remote client.

### M3.4 — guard the empty-ghoul2 index / bolt sites  *(cgame)*
- Any residual `cent->gent->ghoul2[playerModel]` / bolt-matrix reads that fire
  before the rebuild completes must be guarded (they index an empty vector).
  Audit the sites in the agent map (`cg_players.cpp:5108-5187,5297-5331`,
  `cg_ents.cpp:365,492,539`).
- Done: no crash during the frame(s) between "entity seen" and "model built".

## Risks / open questions

- **Per-entity ghoul2 build cost.** Building a ghoul2 model per remote entity is
  expensive; do it once on first sight (cache on the gentity), never per frame.
- **Skeleton posing without the server's `CG_G2PlayerAngles`.** That function
  reads gentity bone indices; after `G_SetG2PlayerModelInfo` those indices exist
  on the client gentity, so it should work — verify.
- **Cleanup.** Free the client-built ghoul2 when the entity leaves / on
  disconnect, mirroring MP's `G2API_CleanGhoul2Models`.
- **Verification is interactive.** A headless client does not run its render
  loop (it idles and `cl_timeout`s), so M3 must be verified in a focused window
  with a human driving — the automated probe cannot observe it.

## Headless verification

`tools/headless-verify.sh` runs the host + a dual-load remote client under a
dedicated `Xvfb` virtual framebuffer (no physical screen, and no window manager
— so the client window is always effectively focused and the game loop runs at
full speed instead of stalling as it does unfocused on a real desktop). The
client renders with llvmpipe software GL and captures frames with the engine's
`screenshot_png` command; the script then classifies each PNG with ImageMagick
(a real 3D frame has mean ≳0.05, high stddev, and >500 unique colours; a
black/console screen is near-zero mean and ~13 colours). Env overrides:
`JK2_BUILD`, `JK2_ASSETS`, `JK2_HV_OUT`.

**Result (2026-07-11):** temporary in-cgame probes during a harness run
confirmed, on the remote client, all four characters building their ghoul2 from
the networked model name and entering the render path:
`built PLAYER 1 model='kyle' ghoul2valid=1 infoValid=1`,
`built NPC ent#354 model='jan' (from CS_MODELS+40) ghoul2valid=1`,
plus `RENDERING character ent#…` for players (ent#0/#1) and NPCs (ent#354/#356,
`eFlags_NPC=1`). Client frames were confirmed real 3D views, no crash; host
loopback regression exits 0. The probes have been removed; the harness remains
for future runs.

## Sequencing recommendation

M3.1 + M3.2 first (host player renders and animates — the visible headline),
then M3.3 + M3.4 (NPCs). Each is independently testable in a windowed session.

## Implementation record (what actually landed)

Code complete, builds clean, host loopback regression exits 0, and the remote
client connects + enters the world without crashing. Windowed two-player
verification (a human driving the client) still pending — the headless client
does not run its render loop, so it cannot exercise the render paths.

- **M3.0** — `cg_remoteClient` flag. Engine sets cvar `cg_remoteClient 1` in
  `CL_InitCGame`'s dual-load branch (`cl_cgame.cpp`); cgame registers it in
  `cvarTable`, latches it into `qboolean cg_remoteClient` after
  `CG_RegisterCvars` (`cg_main.cpp`). `GetCGameAPI` (`g_main.cpp`) allocates
  `level.clients` + wires `g_entities[i].client` when the cvar is set. Migrated
  the committed local-player sentinels from "gent->client is null" to
  `!cg_remoteClient`: `cg_view.cpp` force-speed FOV, `cg_draw.cpp` HUD force
  (#293, #796), crosshair accurate path (#1799), force-crosshair hint (#1708),
  saber-style (#374); `cg_weapons.cpp` view-weapon skip (#929).
- **M3.1** — player model name. `G_SetG2PlayerModel` persists the model name in
  the unused `renderInfo.modelName`; `ClientUserinfoChanged` emits it as the
  `model` key in the `CS_PLAYERS` configstring; `CG_NewClientinfo` rebuilds the
  ghoul2 via `G_SetG2PlayerModel` on the remote client (idempotent, skips if
  ghoul2 already valid).
- **M3.2** — animation from snapshot. `CG_PlayerAnimation` reads
  `cent->currentState.legsAnim` (networked) with a neutral `legsFpsMod` 1.0 on
  the remote client instead of the absent `gent->client->ps`/`renderInfo`.
- **M3.3** — NPC models. `G_SetG2PlayerModel` tags NPC entities by writing the
  glm's `G_ModelIndex` into `s.modelindex` (networked). New `CG_RemoteNPCModel`
  (called early in `CG_Player`) allocates a `gclient_t` for the NPC gentity on
  first sight, resolves the model name from `CS_MODELS + modelindex`, and
  rebuilds the ghoul2. **Tag condition is `s.number >= MAX_CLIENTS`, not
  `EF_NPC`** — the builder runs during the NPC stats parse (`NPC_stats.cpp:2208`)
  *before* the spawn code sets `EF_NPC` (`NPC_spawn.cpp:956`), so an `EF_NPC`
  test here is always false and the model never gets networked. Entity slots
  `[0,MAX_CLIENTS)` are reserved for real player clients (model in CS_PLAYERS);
  everything `G_Spawn`'d above that is an NPC. (This was a real bug the headless
  harness caught: NPCs arrived with `modelindex=0` until the condition changed.)
- **M3.4** — the ghoul2-index/bolt sites in `CG_Player` and its helpers are
  already protected by the original `cent->gent->ghoul2.size()` gates (e.g.
  `cg_players.cpp:5060`); once M3.1/M3.3 build the ghoul2 they behave exactly as
  on the host, and if a build fails the `.size()` gate skips them. No new guards
  were required beyond confirming that coverage.
