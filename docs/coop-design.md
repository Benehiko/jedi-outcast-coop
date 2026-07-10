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

### NPCs did not spawn — diagnosed and resolved

The load test surfaced a problem that source reading did not:

    ERROR: Couldn't spawn NPC jan
    ERROR: Couldn't spawn NPC stormtrooper
    ERROR: Couldn't spawn NPC stormtrooper2

The multiplayer tree contains the stormtrooper AI in full
(`codemp/game/NPC_AI_Stormtrooper.c`, 2,779 lines), so the code was
present but instantiation failed.

The cause is a file location, not a format. Compare the two
`NPC_LoadParms` implementations:

| | Jedi Outcast (`codeJK2/game/NPC_stats.cpp:2223`) | Jedi Academy (`codemp/game/NPC_stats.c:3561`) |
|---|---|---|
| Base file | reads `ext_data/NPCs.cfg` | none — `NPC2.cfg` is commented out at `:3564` |
| Extensions | appends `ext_data/*.npc` | appends `ext_data/NPCs/*.npc` |
| Buffer | `NPCParms`, `MAX_NPC_DATA_SIZE` = `0x40000` | identical |
| Parser | `NPC_ParseParms()` scans the buffer for a named block | identical |

Both concatenate every source into one buffer and hand it to the same
parser. The grammar (`Name { key value ... }`) and the key set are the
same. Jedi Outcast ships its 78 NPC definitions as a single
`ext_data/npcs.cfg` (38,637 bytes) inside `assets0.pk3`; Jedi Academy's
multiplayer code never looks there.

No translation is required. The file only needs to be relocated to
`ext_data/NPCs/` and given a `.npc` extension. At 38 KB it is far
within the 256 KB buffer, and Jedi Academy additionally runs
`COM_Compress()` over each file before concatenating.

`tools/build-coop-npcs-pk3.sh` extracts the file from the user's own
retail installation and repackages it as `zzz-coop-npcs.pk3`. The `zzz-`
prefix makes the archive sort after `assets5.pk3`, so it shadows the
retail archives in the engine's search path. No proprietary asset is
stored in this repository.

### Result

Re-running the same load test with the pk3 installed:

| | Before | After |
|---|---|---|
| `Couldn't spawn NPC` errors | 8 | **0** |
| Total error lines | 13 | 5 |

No new errors were introduced. The five remaining are all
`ref_tag ... has invalid target`, the cutscene camera focus entities
orphaned by `misc_camera` having no spawn function — out of scope by
design.

`entitylist` on the running server confirms live enemies rather than
silent absence: `NPC_Stormtrooper` at entity indices 111, 118, 176, 194
and 195, with the renderer disk-loading Jedi Outcast's Ghoul2 skeletal
model `models/players/stormtrooper/model.glm` under the Jedi Academy
engine.

## Revised assessment

The recommendation is unchanged, and both risks identified before the
load test have now been retired. The entity spawn table is a smaller
problem than estimated, and the NPC asset layer — which looked like the
larger one — turned out to be a file copy.

## First milestone: reached

Two clients connect to a dedicated server running the campaign map
`kejim_post` and both enter the game:

    ClientConnect: 0 [127.0.0.1:29072] "Kyle^7"
    ClientBegin: 0
    ClientConnect: 1 [127.0.0.1:29073] "Jan^7"
    ClientBegin: 1

No disconnects, both client processes rendering.

Reaching it required resolving three further instances of the same
problem the NPC investigation uncovered: Jedi Academy's client code
hardcodes Jedi Academy's asset paths, and Jedi Outcast ships equivalent
data under different names.

| Jedi Academy expects | Jedi Outcast ships | Fatal at init | Resolved by |
|---|---|---|---|
| `ext_data/Siege/Classes/*.scl` | nothing | yes | patch: non-fatal, Siege stays unavailable |
| `ui/jampmenus.txt` | `ui/jk2mpmenus.txt` | yes | cvar `ui_menuFilesMP` |
| `ui/jahud.txt` | `ui/jk2hud.txt` | yes | cvar `cg_hudFiles` |
| `ui/jamp/menudef.h` | `ui/jk2mp/menudef.h` | no, but drops the client | patch: probe, fall back |

Only the Siege and `menudef.h` cases needed source changes; both are in
`codemp/ui/ui_main.c` and carried as `patches/0001-jk2-asset-paths-in-mp-ui.patch`.

The `menudef.h` case is worth recording because it was nearly dismissed.
Its symptom is a wall of parse errors that look cosmetic —
`expected integer but found MENU_TRUE`. In fact, without the global
defines every symbolic constant in Jedi Outcast's `.menu` files becomes
an unparseable token, and the failure cascades into `DROPPED` during the
connection handshake. Loading the correct header took the client from a
dropped connection to `ClientBegin`.

Campaign spawn points needed no work. `SP_info_player_start` aliases
itself to `info_player_deathmatch` at `codemp/game/g_client.c:127-129`,
so the multiplayer spawn selector handles campaign maps natively. The
concern raised before the test was unfounded.

## Known remaining work

- Approximately 40 singleplayer spawn functions to port for full prop
  and pickup coverage across all 45 maps (14 classnames on
  `kejim_post`). Non-fatal in the meantime.
- Player spawn points: campaign maps carry `info_player_start`, not the
  deathmatch spawn points multiplayer expects. Unverified.
- ICARUS is present in the multiplayer tree and will attempt to run
  `.ibi` scripts for entities carrying `script_targetname` or
  `behaviorSet` keys. For the no-cutscenes goal these should be
  suppressed.
- Weapon and item pickups (`item_bacta`, `item_battery`,
  `item_la_goggles`) have no multiplayer spawn function.
- The server logs `WARNING: Entity used itself` roughly every thirteen
  seconds during play. Not investigated. Likely an entity whose `target`
  resolves to itself because its intended target was one of the dropped
  classnames.
- Broadcast messages print raw localization keys (`@@@PLCONNECT`,
  `@@@DISCONNECTED`). Jedi Academy's `strip/` string tables differ from
  Jedi Outcast's. Cosmetic.
- The multiplayer gametype is FFA (`g_gametype 0`), so players can damage
  each other and NPCs treat both as `TEAM_PLAYER` targets only
  incidentally. A cooperative gametype, or friendly-fire and team
  assignment, remains to be configured.
- Whether the two clients render each other's player models in-world was
  confirmed visually by the user, not by automated check.
