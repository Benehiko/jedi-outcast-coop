# Cooperative Campaign: Design and Feasibility

**Status:** Investigation complete. Load test passed with one new finding.
**Goal:** Two or more players cooperatively playing the Jedi Outcast
campaign maps. No cutscenes, no scripted sequences.

## Summary

Two routes were evaluated. The recommendation is to host the campaign on
the multiplayer tree (`codemp/`) rather than to widen the singleplayer
tree (`codeJK2/`) to support multiple clients.

The decisive fact is that `codemp/` is not a conventional Quake 3
multiplayer codebase. It is Jedi Academy's multiplayer gamecode, and
Raven built it on the campaign engine: it retains ICARUS, the complete
singleplayer enemy AI roster, Ghoul2, and the saber system. The netcode
we need therefore already sits on top of the AI and scripting we need.

## Findings

All claims below are cited to source. Line numbers refer to OpenJK at
commit `2ba5021`.

### The singleplayer engine already speaks the network protocol

The singleplayer engine was not stripped of Quake 3's client/server
architecture. It was capped to one client.

- Delta-compressed entity snapshots: `code/server/sv_snapshot.cpp:506`
  (`SV_BuildClientSnapshot`), `:666` (`SV_SendClientSnapshot`)
- Usercmd handling: `code/server/sv_client.cpp:414` (`SV_UserMove`)
- Netchan: `client_t` holds a real `netchan_t` at `code/server/server.h:134`
- `code/qcommon/msg.cpp` and `net_chan.cpp` are compiled into the
  singleplayer target (`code/CMakeLists.txt:168-169`)

The single player is already a network client. It connects to
`localhost`, which `NET_StringToAdr` traps as `NA_LOOPBACK`
(`code/qcommon/net_chan.cpp:583-585`), and packets travel through an
in-memory ring buffer (`:542`) rather than a socket.

The cap is compile-time:

```c
#define MAX_CLIENTS 1        // code/qcommon/q_shared.h:618
```

The comment on that line shows the value was formerly `128`. Client
loops are literal `i < 1` (for example `sv_snapshot.cpp:709`).
`sv_maxclients` is not a cvar in the singleplayer engine at all; it
appears only as the constant `1` written into a server info string
(`code/server/sv_main.cpp:245`).

### The singleplayer gamecode assumes one player pervasively

This, not the engine, is what makes widening the singleplayer tree
expensive.

A global `gentity_t *player` is unconditionally aliased to
`&g_entities[0]` (`codeJK2/game/g_main.cpp:141`, assigned at `:722`) and
is referenced approximately 160 times across the game tree. A further
~44 raw `&g_entities[0]` literals appear in the AI files alone.

The client array is sized for exactly one client in the *gamecode*,
independent of any engine value:

```c
level.maxclients = 1;                    // codeJK2/game/g_main.cpp:654
level.clients = G_Alloc(...);            // :655-656, one slot
```

`ClientConnect` and `ClientBegin` are written generically over
`clientNum` (`codeJK2/game/g_client.cpp:534-597`), but a second client
would index `level.clients + 1` out of bounds on that allocation.

### ICARUS is cleanly per-entity and can be disabled

This is the most favorable finding for the stated goal.

Each scriptable entity owns its own sequencer and task manager
(`codeJK2/game/g_ICARUS.cpp:604-606`). The four core ICARUS files —
`Sequencer.cpp`, `TaskManager.cpp`, `Interpreter.cpp`, `Sequence.cpp` —
contain zero references to `player` or `g_entities[0]`. ICARUS itself
has no single-player coupling.

A global kill switch already exists. `stop_icarus`
(`codeJK2/game/g_main.cpp:173`) guards all three per-frame
`taskManager->Update()` call sites (`:965`, `:997`, `:1407`). It is
currently set on player death. Setting it at initialization no-ops all
script task execution without disturbing allocation.

Basic level function does not depend on scripts. Entities spawn via
`G_SpawnEntitiesFromString` (`g_main.cpp:693`), independent of ICARUS.
Doors, movers, and triggers are C code (`g_mover.cpp`, `g_trigger.cpp`).
`ICARUS_RunScript` returns harmlessly when an entity has no sequencer
(`g_ICARUS.cpp:96-100`). Scripted *gameplay* events — spawn waves,
scripted doors, objectives — will not fire, but geometry, movers,
triggers, items, and NPC AI continue to work.

"No cutscenes" is therefore a supported configuration rather than a
compromise.

### NPC targeting is team-based, with hardcoded aggro shortcuts

Enemy selection uses `gi.EntitiesInBox` plus a team check
(`NPC_FindNearestEnemy`, `codeJK2/game/NPC_utils.cpp:1041`); validity is
`TEAM_PLAYER` membership, not entity index. A second player on
`TEAM_PLAYER` is naturally a valid target.

However, the surrounding aggro heuristics hardcode client 0:

- `NPC_FindPlayer()` is `NPC_TargetVisible(&g_entities[0])` (`NPC_utils.cpp:1141`)
- `NPC_CheckPlayerDistance` forces enemy to `&g_entities[0]` (`:1170-1172`)
- Alert handling tests `event->owner == &g_entities[0]` (`:1120-1121`)

A second player would be attacked when nearest, but ignored by
proximity aggro and "find player" behaviors.

### Triggers and saves are hard blockers for the widen-SP route

Triggers carrying the PLAYERONLY spawnflag test entity index directly:

```c
if ( other->s.number != 0 ) return;      // codeJK2/game/g_trigger.cpp:194-197
```

A second player cannot trip them. `trigger_visible` similarly hardcodes
`&g_entities[0]` (`:1474`).

The save format serializes exactly one `gclient_t`
(`codeJK2/game/g_savegame.cpp:991-997` in `WriteLevel`, `:1027-1059` in
`ReadLevel`), each guarded by a deliberate tripwire the original authors
left:

```c
assert(level.maxclients == 1);  // I'll need to know if this changes,
                                // otherwise I'll need to change the way ReadGame works
```

### The multiplayer tree retains the campaign engine

`codemp/` contains, in full:

- **ICARUS** — `codemp/icarus/`, wired into the engine at
  `codemp/server/sv_gameapi.cpp:2866-2874`, invoked at spawn
  (`codemp/game/g_spawn.c:951-960`). `behaviorSet` and
  `script_targetname` keys are parsed (`g_spawn.c:127-208`).
- **The full singleplayer AI roster** — `NPC_AI_Stormtrooper.c`,
  `NPC_AI_Jedi.c`, Rancor, Wampa, ATST and others; ~23,685 lines across
  the AI files. This is distinct from the player-bot system
  (`ai_main.c`, `g_bot.c`), which coexists.
- **Ghoul2** — `codemp/ghoul2/`
- **The saber system** — `codemp/game/w_saber.c` (9,490 lines),
  `bg_saber.c`, `bg_saberLoad.c`; force powers in `w_force.c`

### Map format is identical

Both trees use `RBSP` version 1: `codemp/qcommon/qfiles.h:307-309` and
`code/qcommon/qfiles.h:204-207`. The multiplayer version check at
`codemp/qcommon/cm_load.cpp:722` therefore passes for campaign maps, and
the lump set loaded (`:733-744`) is the set campaign maps contain. The
RMG/SubBSP machinery in the multiplayer tree is additive and does not
reject a conventional BSP.

### Unknown entities degrade gracefully

An unrecognized classname is not fatal. `G_CallSpawn` prints
`"%s doesn't have a spawn function"` and returns false
(`codemp/game/g_spawn.c:700-731`); the caller frees the entity and
continues (`:946-948`). Classname matching is case-insensitive
(`Q_stricmp`, `:697`).

A campaign map will load on the multiplayer engine, missing only the
entities whose spawn functions are absent.

### The gap is roughly forty spawn functions

Spawn table sizes are comparable: 273 entries in multiplayer
(`codemp/game/g_spawn.c:496`), 268 in singleplayer
(`codeJK2/game/g_spawn.cpp`). After normalizing for case and excluding
field-name tokens, approximately 40 singleplayer classnames have no
multiplayer spawn function. Notable ones:

| Category | Missing classnames |
|---|---|
| Campaign flow | `target_autosave`, `target_secret`, `target_change_parm`, `trigger_visible`, `trigger_entdist` |
| Cameras | `misc_camera`, `misc_camera_focus`, `misc_camera_track` |
| Turrets, vehicles | `misc_atst_drivable`, `misc_sentry_turret`, `misc_ion_cannon`, `misc_laser_arm` |
| Breakables, props | `misc_exploding_crate`, `misc_gas_tank`, `misc_crystal_crate`, `object_cargo_barrel1` |
| Shooters, effects | `shooter_grenade`, `shooter_rocket`, `fx_target_beam`, `fx_cloudlayer` |

Several of these — autosave, secrets, cameras — are the cutscene and
progression machinery this project explicitly excludes.

## Comparison

| | Widen singleplayer | Campaign on multiplayer |
|---|---|---|
| Netcode | Present, capped at one client | Already multiplayer |
| `g_entities[0]` assumptions | ~200 sites to correct | Already multi-client |
| NPC and AI | Present | Present |
| ICARUS | Present | Present |
| Ghoul2, saber | Present | Present |
| Map format | Native | Identical (RBSP v1) |
| Principal work | Client array, save format, PLAYERONLY triggers, AI aggro | ~40 spawn functions |

## Recommendation

Host the campaign on the multiplayer tree.

Widening the singleplayer tree requires correcting roughly two hundred
hardcoded references to entity zero across AI and gameplay code, then
rewriting the save format. Hosting on multiplayer requires porting
approximately forty spawn functions, most of which transfer directly
from `codeJK2/game/`, and several of which are cutscene machinery that
is out of scope.

## Load test results

Both open questions were resolved empirically by running the campaign
map `kejim_post` on the multiplayer dedicated server (`openjkded`),
built from the same tree, against the retail assets.

    ./openjkded.x86_64 +set dedicated 1 +set sv_pure 0 +map kejim_post

### The map loads

`InitGame` completed and the server remained up serving the level, with
`sv_maxclients` reporting `8`. No `Com_Error`, no assertion, no crash.
The BSP loaded, the entity string was parsed, and the game module
initialized. The first open question is answered: campaign maps load on
the multiplayer engine.

### The entity gap is smaller than estimated, for this map

Fourteen distinct classnames had no spawn function, dropping 57 entity
instances. The map loaded regardless, as `G_CallSpawn`'s
non-fatal path predicted.

| Instances | Classname |
|---:|---|
| 12 | `item_bacta` |
| 10 | `misc_model_ammo_rack` |
| 9 | `misc_exploding_crate` |
| 5 | `misc_model_gun_rack` |
| 4 | `item_battery` |
| 3 | `misc_gas_tank` |
| 3 | `misc_camera` |
| 2 | `misc_trip_mine` |
| 2 | `misc_model_cargo_small` |
| 2 | `item_la_goggles` |
| 2 | `fx_explosion_trail` |
| 1 | `trigger_location` |
| 1 | `target_secret` |
| 1 | `target_autosave` |

These are props, pickups, and cutscene machinery. `target_autosave`,
`target_secret`, and `misc_camera` are out of scope by design. The
`ref_tag ... has invalid target` errors in the log are a direct
consequence of `misc_camera` being dropped and are therefore expected.

Note this is one map of forty-five. The ~40-classname figure derived
from the spawn tables remains the estimate for the campaign as a whole.

### NPCs do not spawn — this is the real blocker

The load test surfaced a problem that source reading did not:

    ERROR: Couldn't spawn NPC jan
    ERROR: Couldn't spawn NPC stormtrooper
    ERROR: Couldn't spawn NPC stormtrooper2

The multiplayer tree contains the stormtrooper AI in full
(`codemp/game/NPC_AI_Stormtrooper.c`, 2,779 lines). The code is present;
instantiation fails. This is the second open question landing exactly
where it was flagged: Jedi Outcast's NPC definition assets do not match
what Jedi Academy's NPC stats code expects.

The consequence is that a campaign map will load and be walkable, but
will be empty of enemies until the NPC asset layer is reconciled.

## Revised assessment

The recommendation is unchanged — campaign on multiplayer remains
substantially cheaper than widening singleplayer — but the risk has
moved. The entity spawn table is a smaller problem than estimated. The
NPC asset layer is a larger one, and it was invisible to code reading.

## Proposed first milestone

Two players spawn on a campaign map and can move around it together.
ICARUS disabled, no saves, no enemies. This is now known to be
achievable: the map loads, the server accepts eight clients, and the
missing entities are non-fatal.

Enemies are a separate, subsequent milestone gated on diagnosing why
`NPC_Spawn` rejects Jedi Outcast's NPC definitions. That investigation
should start at `codemp/game/NPC_spawn.c` and the `.npc` files inside
`assets0.pk3`.
