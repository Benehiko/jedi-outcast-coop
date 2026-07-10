# Roadmap

The plan from here. Read [route-comparison.md](route-comparison.md) for why
this route was chosen and [widen-sp-progress.md](widen-sp-progress.md) for
what has been done.

**Goal.** Two players cooperatively walking a Jedi Outcast campaign map,
fighting the same stormtroopers, on Linux. No cutscenes, no scripted
sequences, no saves.

**Route.** Widen the singleplayer engine. Its assets, animations, AI,
strings, and save system are all natively correct because it is the game
those assets shipped with.

## Where we are

| | Status |
|---|---|
| Native Linux build | Done |
| `MAX_CLIENTS = 2` | Done — 13 engine sites, 2 gamecode sites |
| UDP transport | Done — binds a socket, sends and receives |
| Server accepts a remote client | Done — `SV_DirectConnect` assigns slot 1 |
| Client connects and receives a gamestate | Done — host logs `Kyle connected` |
| Entity delta compression restored | Done — table + both delta functions enabled and correct |
| Client survives its first snapshot | Done — real field serialisation over the wire |
| Second player spawns clear of the first | Done — `G_DisplaceSpawnOrigin` |
| Second player visible in the host's world | **Done — confirmed on screen** |
| Second client moves, replicates to host | **Done — confirmed on screen** |
| Second client renders its own view | **Blocked** — no world is loaded into its renderer. See below |
| Two players fighting NPCs | Not started |

### The remote client is fully connected but has no world

The decisive observation was the user's, not a log line: **"I can move on
client 2 but I cannot see anything. Client 1 can see me moving."** Plus: no
gun.

So client 2 is a working client. It sends usercmds, the server simulates its
player, and the movement replicates back to client 1's view. The netcode
round-trip is complete in both directions. What it lacks is a world to draw.

Probes confirm the client-side chain runs to completion:

    gamestate: parsed ok, cmd loop done
    InitCGame: mapname='maps/kejim_post.bsp'
    InitCGame: calling CG_INIT
    InitCGame: CG_INIT returned
    gamestate: CL_StartHunkUsers returned, state=6 cgameStarted=1

`cls.state` reaches `CA_PRIMED`, `cgameStarted` is 1, the mapname resolves.
Nothing hangs and nothing errors.

The cause: `re.LoadWorld` is called from exactly one place, the cgame syscall
`CG_R_LOADWORLDMAP` (`cl_cgame.cpp:963`), and probes show **neither client
ever invokes it** -- not even the host, which renders correctly. The only
other world load is `CM_LoadMap` inside `SV_SpawnServer`
(`sv_init.cpp:295`).

In this engine the world reaches the renderer through the *server* side of
the process. A client sharing that process gets it for free. A remote client
runs no server, so `tr.world` is never populated: black screen, no weapon
model, no HUD, while movement and replication work perfectly.

### Why: the client's cgame VM is initialised by the server

Attempted and reverted. The chain is:

```c
// sv_game.cpp, inside SV_InitGameProgs
gameLibrary = Sys_LoadSPGameDll( "jospgame", &GetGameAPI );
ge = GetGameAPI( &import );          // populates the gamecode's `gi` table
//hook up the client while we're here
CL_InitCGameVM( gameLibrary );       // populates cgvm.entryPoint
```

```c
// vmachine.cpp
if ( cgvm.entryPoint ) { ... }       // null on a serverless client
```

A remote client never runs `SV_InitGameProgs`, so `cgvm.entryPoint` stays
null and **every `VM_Call` is a silent no-op**. `CG_INIT` appears to succeed
while the cgame never executes a line. The client is left connected, moving,
and replicating correctly, with no world, no weapon model and no HUD --
exactly the observed symptoms.

Loading the library from `CL_InitCGame` when `cgvm.entryPoint` is null does
make the cgame run. It then segfaults immediately:

```
CG_Init            cg_main.cpp:1772
CG_ParseMenu       "ui/hud.menu"
Com_Printf         g_main.cpp:871
0x0000000000000000                    <- gi.Printf is null
```

The cgame and the server game are **one shared library with one import
table**. `gi` is assigned only inside `GetGameAPI`, which the server calls.
`game_import_t` carries **127 function pointers**, most of them server
services -- `linkentity`, `trace`, `SetBrushModel`, the whole Ghoul2 API --
that a client cannot meaningfully provide. `cg_camera.cpp` alone calls `gi.`
eleven times.

So the singleplayer cgame is not separable from the singleplayer server as
the code stands. The options, none small:

- **Populate a client-side `gi`** with the subset the cgame actually touches,
  and abort on the rest. Requires auditing all 127 entries.
- **Split the cgame out of the game library**, as the multiplayer tree does
  (`cgamex86_64.so` is separate from `jampgamex86_64.so`).
- **Run a headless server on the joining client**, so the existing
  initialisation path runs unmodified, and let it slave to the remote one.
  Cheapest to try, ugliest to live with.

This is the deepest coupling found so far, and it is more fundamental than
the netcode was.

### Entity fields are not round-tripping correctly

Observed in play: a red/green/blue coordinate-axis gizmo is drawn where an
entity should be, meaning the renderer fell back to an origin marker because
`modelindex` arrived as zero or garbage. The second player renders correctly,
so most fields survive; at least one does not.

This is a defect in the restored `entityStateFields` table -- a wrong bit
width, or a field whose order does not match the `JK2_MODE` struct. Compare
each entry against `entityState_t` and against `codemp/qcommon/msg.cpp`.


Everything below Phase 1 is unverified. Estimates are ordinal, not
calendar: each phase is expected to be harder than the one above it.

---

## Phase 1 — Land the second client

> **The blocker named below was wrong.** Tracing `cls.state` through a live
> connection showed the handshake already completes:
>
> ```
> CL_Connect_f: entry state=1        (CA_DISCONNECTED)
> CL_Connect_f: set CA_CHALLENGING, addr=127.0.0.1:29090
> CL_Connect_f: after resend state=3 (CA_CHALLENGING — held)
> cl_parse: gamestate received -> CA_LOADING
> cl_cgame: -> CA_PRIMED
> ```
>
> and the host logs `Kyle connected`. The `CA_PRIMED` observed earlier was a
> *completed* connection misread as a stuck one. The real blocker is below.

### The actual blocker: the wire protocol does not serialise entities

The client connects, receives a gamestate, and segfaults on its first
snapshot:

```
MSG_ReadEntity          msg.cpp:836
CL_DeltaEntity          cl_parse.cpp:74
CL_ParsePacketEntities  cl_parse.cpp:165   (oldframe = 0x0, first snapshot)
CL_ParseSnapshot        cl_parse.cpp:271
CL_ParseServerMessage   cl_parse.cpp:535
Com_EventLoop           common.cpp:895
```

The cause is that Raven replaced entity serialisation with a shared-memory
shortcut. The server writes an *index into its own array*:

```c
void MSG_WriteEntity( msg_t *msg, entityState_t *to, int removeNum ) {
    ...
    MSG_WriteLong( msg, to - svs.snapshotEntities );   // pointer arithmetic
}

void MSG_ReadEntity( msg_t *msg, entityState_t *to ) {
    int index = MSG_ReadLong( msg );
    *to = svs.snapshotEntities[index];                 // the SERVER's array
}
```

`MSG_ReadEntity` is client-side code dereferencing `svs.snapshotEntities`.
That is coherent only while client and server share a process. A remote
client never allocates that array, so it is null, and the first snapshot
faults.

Neither function exists in the multiplayer tree. The singleplayer wire
protocol is, in this respect, not a wire protocol.

### What is present, and what is missing

| Piece | Status |
|---|---|
| `entityStateFields` | **Done** — enabled, made `JK2_MODE`-conditional |
| `MSG_WriteDeltaEntity` | **Done** — enabled; `numFields` assertion holds (62 vs 63) |
| `MSG_ReadDeltaEntity` | **Done** — enabled |
| `sv.svEntities[].baseline` | Present, populated at `sv_init.cpp:160` |
| `cl.entityBaselines[]` | **Absent** — removed from `clientActive_t` |
| Server emits `svc_baseline` | **Absent** — never sent |
| Client parses `svc_baseline` | **`assert(0)`** at `cl_parse.cpp:412` |

Both halves of baseline delta compression were removed, not merely
bypassed. `CL_ParsePacketEntities` still has the correct three-case
structure and still computes `newnum` and tracks `oldstate`; only
`CL_DeltaEntity` was collapsed to ignore them.

### Tasks

1. **Restore `cl.entityBaselines[MAX_GENTITIES]`** to `clientActive_t`
   (`code/client/client.h`). The multiplayer tree has it at `client.h:150`.
   Two commented-out references survive in `cl_cgame.cpp:245,251`.

2. **Emit `svc_baseline` from the server.** In the gamestate, as the
   multiplayer tree does in `SV_SendClientGameState`.

3. **Parse `svc_baseline` on the client**, replacing the `assert(0)`.

4. **Reconcile `entityStateFields` with `entityState_t`.** Do this first;
   it gates everything else. `MSG_WriteDeltaEntity` opens with

   ```c
   assert( numFields + 1 == sizeof(*from)/4 );
   ```

   `entityStateFields` (`msg.cpp:518`) has 68 entries, one commented out, so
   67 live. `sizeof(entityState_t)` is **252 bytes = 63 ints**, measured on
   the built binary. The assertion wants 63 and would get 68.

   The table is stale: it describes a struct that no longer exists. Because
   `MSG_WriteDeltaEntity` has no callers today, nothing has ever executed
   that assertion, and the rot went unnoticed. Every field in the table must
   be checked against the current `entityState_t` before the function can be
   trusted to serialise anything.

5. **Swap the four call sites.** Server `SV_EmitPacketEntities`
   (`sv_snapshot.cpp:96,104,112`) → `MSG_WriteDeltaEntity` with the three
   cases: delta from `oldent`; new entity from
   `&sv.svEntities[newnum].baseline` with `force`; removal with `to = NULL`.
   Client `CL_DeltaEntity` (`cl_parse.cpp:74`) → `MSG_ReadDeltaEntity`,
   taking `newnum` and `old` as parameters, matching
   `codemp/client/cl_parse.cpp`.

6. **Regression-test loopback singleplayer at every step.** The loopback
   path uses these same functions. Breaking it is the main risk of this
   change, and it is easy to miss because loopback would still work if the
   index shortcut were left in place for local clients.

### Blocker found while attempting tasks 1-3

Emitting a `svc_baseline` for every entity with a baseline **overflows the
gamestate message**. `kejim_post` has on the order of 900 live entities;
`MAX_MSGLEN` is 17408 bytes. The server logs

    WARNING: GameState overflowed for Kyle

and the loopback ring — deliberately `<= MAX_MSGLEN`, with an `#error` in
`net_chan.cpp` enforcing it — then smashes the stack in `NET_GetLoopPacket`.
The unconditional free-space check added in task 4 turns that silent
corruption into a clean `ERR_DROP`.

Tasks 1-3 therefore cannot land as written. Options, unevaluated:

- **Raise `MAX_MSGLEN`.** It is `1*17408`, with a commented-out `3*16384`
  directly beneath, suggesting it was tuned once before. The loopback ring
  must stay `<= MAX_MSGLEN`, and `Netchan_Transmit` already fragments
  anything over `FRAGMENT_SIZE`.
- **Send baselines across several messages**, as a reliable sequence before
  the first snapshot.
- **Send no baselines at all.** `SV_EmitPacketEntities` needs a `from` state
  for a newly-visible entity; the multiplayer tree uses the baseline, but a
  null state plus `force` would also serve, at the cost of a larger first
  snapshot per entity.

The third is the smallest change and should be measured first.

### Done when

A second `openjo_sp` process connects, receives snapshots without
crashing, and both processes stay up. The host already logs
`Kyle connected`; the goal is for it to keep doing so past the first
snapshot.

### Known hazards

- The `qport` mechanism (`net_chan.cpp`) exists but has never carried two
  clients. Two clients behind one address must not collide.
- `svs.numSnapshotEntities` was widened to `MAX_CLIENTS * 4 * 64`. Whether
  `4` is the right backup factor for this engine is untested.
- The `entityState_t` field table has already drifted (task 4). Any other
  delta table — `playerStateFields` for `MSG_WriteDeltaPlayerstate` — may
  have drifted too, though that one *is* called today and so is presumably
  sound.

---

## Phase 2 — Two players in one world

Once a second client is connected, it has an entity but no coherent place
in the game.

### Tasks

1. **Spawn points.** `kejim_post` has exactly one `info_player_deathmatch`
   (entity 345), aliased from the campaign's `info_player_start`. Two
   clients spawning at one point will telefrag or stack. Either pick a
   nearby free position, or suppress telefrag between co-op players.

2. **The `player` global.** `codeJK2/game/g_main.cpp:141` declares
   `gentity_t *player`, unconditionally aliased to `&g_entities[0]`. It has
   **474 reads and 6 assignment sites**, three of which are function-local
   shadows.

   This is a dispatch problem behind one symbol, not 474 edits. Classify the
   reads first:
   - *"any player"* — proximity, alerts, most AI. Should iterate clients.
   - *"the local player"* — HUD, camera, view state. Should stay client 0
     for now.
   - *"the player who did this"* — damage attribution, triggers. Should take
     an explicit argument.

   Do not attempt this until Phase 1 lands. It is the largest task in the
   project and the categories above are a hypothesis, not a finding.

3. **PLAYERONLY triggers.** `g_trigger.cpp:194` and `:646` test
   `other->s.number != 0`; `:134` tests `!activator->s.number`. A second
   player cannot trip them, so scripted doors and elevators will not open
   for player 2. Widen to `s.number < MAX_CLIENTS`.

### Done when

Two players spawn, see each other's models, and can both open a door.

---

## Phase 3 — Shared combat

### Tasks

1. **NPC perception.** In the multiplayer tree the AI's perception entry
   points are hardcoded to `g_entities[0]` behind OpenJK's own `OJKFIXME`
   markers. The singleplayer AI is the origin of that code, so expect the
   same pattern in `codeJK2/game/NPC_utils.cpp`. Audit
   `NPC_FindPlayer`, `NPC_CheckPlayerDistance`, and the alert handler.

2. **Damage attribution and death.** Verify a second player can be hit, can
   die, and that the game does not treat player 2's death as game-over.

3. **`cg_media.h:356`** sizes `clientinfo[MAX_CLIENTS]`, which now has two
   entries. The client-game has not been examined for other single-player
   assumptions. Five `MAX_CLIENTS` uses in `codeJK2/cgame/`.

### Done when

Both players can be attacked by the same stormtrooper, and both can kill it.

---

## Phase 4 — Durability

Deliberately last. None of it is needed to prove the concept, and all of it
is easier once the concept is proven.

1. **`sv_maxclients` as a real cvar.** The widened loops currently bound on
   the `MAX_CLIENTS` compile-time constant. Replace with a cvar so the
   client count is configurable without a rebuild.

2. **The save system.** `WriteLevel` writes one `GCLI` chunk holding
   `level.clients[0]`; `ReadLevel` reads one and copies it back. Four
   `level.clients[0]` uses plus two assertions, across two functions
   (`g_savegame.cpp`). Generalising means writing N
   chunks and bumping the save-format version so existing saves still load.

   Raven left assertions marking exactly this
   (`assert(level.maxclients == 1)`), and they sit inside `if (!qbAutosave)`
   — so autosaves pass and only a manual save trips them. **Test against a
   `Debug` build**: `RelWithDebInfo` defines `NDEBUG` and compiles `assert`
   out, so a passing test there proves nothing.

3. **A challenge handshake.** `SV_DirectConnect` currently accepts remote
   connections unchallenged, because Raven removed `svs.challenges` and
   `SV_GetChallenge` along with the transport. Acceptable on a trusted LAN;
   port `SV_GetChallenge` from `codemp/server/sv_main.cpp` before exposing
   the server anywhere else.

4. **Containerised build.** The project's own rules call for building in a
   container. Everything so far has been built on the host against system
   SDL2, OpenAL, zlib, libpng and libjpeg. Nothing was installed, but the
   boundary is unenforced.

---

## Explicitly out of scope

Recording these so they are decisions, not omissions.

- **ICARUS scripting and cutscenes.** The stated goal excludes them. NPCs
  spawn and fight without scripts; scripted sequences, spawn waves, and
  objectives will not fire.
- **The multiplayer route.** Abandoned. Jedi Academy's animation enum has
  1,534 entries against Jedi Outcast's 1,202, diverging from index 1, so
  that engine cannot display these assets. Two upstream bug fixes were
  salvaged from the attempt (`patches/0002`, `patches/0003`) and are worth
  submitting to OpenJK regardless.
- **More than two players.** `MAX_CLIENTS` is a define; raising it further
  is not expected to be interesting until two work.
- **A Vulkan renderer.** The renderer sits behind Quake 3's `refexport_t`
  module boundary, so it remains a clean follow-on. Mesa's Zink driver
  (`MESA_LOADER_DRIVER_OVERRIDE=zink`) runs the existing OpenGL renderer on
  Vulkan today with no code changes.

---

## Working rules

Earned the hard way in this project. See
[investigation-log.md](investigation-log.md) for what each cost.

1. **When the claim is about runtime behaviour, measure the runtime.** Three
   hypotheses about why NPCs ignored the players were each refuted by
   reading source. The players were spectators the whole time. A single
   probe on live state would have settled it in minutes.

2. **A structural claim is not a behavioural one.** "The multiplayer tree
   contains 23,685 lines of AI" was read off file sizes. That AI does not
   work. Line counts establish nothing.

3. **Verify claims against the data, not against the analysis.** A subagent
   reported `ui/jahud.txt` had no Jedi Outcast equivalent. `ui/jk2hud.txt`
   was sitting in the asset listing.

4. **Regression-test after every change.** `openjo_sp +map kejim_post` must
   exit 0 with zero errors and open no socket. Every commit in this project
   has been checked against that.

5. **Grep patterns miss things.** The client cap was surveyed as eight sites
   and was thirteen; five loops used a different idiom. When a count matters,
   sweep more than one way.
