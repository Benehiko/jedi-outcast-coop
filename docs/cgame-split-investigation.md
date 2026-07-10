# Is the cgame library split required? No.

**Verdict: do not split the cgame into its own library.** The split is
neither sufficient nor necessary for the actual goal — making the remote
client render its world. This document records the measurements behind
that verdict and the plan that replaces the split.

## The question

The remote client is fully connected — it moves, and its movement
replicates to the host — but renders nothing, because its cgame never
runs: `cgvm.entryPoint` is populated only by `SV_InitGameProgs`, which a
serverless client never calls. Two prototypes failed:

1. Loading the library from the client and calling nothing: cgame ran and
   crashed on the null `gi` import table.
2. Populating `gi` with the *server's* import table (`SV_BuildGameImport`):
   crashed deeper, in `GetGameAPI → GI_Init → WP_LoadWeaponParms →
   G_EffectIndex → SV_SetConfigstring`, dereferencing the null
   `sv.configstrings` of a server that was never spawned.

The conclusion drawn at the time — "the cgame must be split into its own
library, as the multiplayer tree does" — was premature. This
investigation re-examined it before committing to the largest refactor in
the project.

## Measurement 1: the split does not fix remote rendering

The earlier scoping counted **55 direct cross-calls** (19 cgame→game, 36
game→cgame) and treated them as the coupling to be resolved by a link
boundary. That count missed the dominant coupling entirely: the SP cgame
reads server-side game **state**, not just game **functions**.

| Coupling | Count | Where |
|---|---|---|
| `->gent` dereferences (server `gentity_t` via `centity_t::gent`) | **849** | 18 of the cgame's source files |
| Direct `g_entities[...]` / `level.` reads | **278** | `cg_event.cpp`, `cg_scoreboard.cpp`, `cg_view.cpp`, `cg_draw.cpp`, … |
| Existing null-style guards on `gent` paths | 327 | throughout — the guard idiom already exists |

The wiring is one line, set at init
(`codeJK2/cgame/cg_main.cpp:1502`):

```c
cg_entities[i].gent = &g_entities[i];
```

A split cgame library on a remote machine still executes all 849 of those
dereferences — against a `g_entities` array that no server has populated.
Moving the code into `jospcgamex86_64.so` changes where the symbols live,
not where the data comes from. The multiplayer cgame never has this
problem because it renders purely from snapshot data (`entityState_t`,
`playerState_t`, configstrings); the SP cgame was written assuming the
server's memory is one pointer away.

**The real work — making the cgame render from received data instead of
server memory — is identical with or without the split.** The split adds
build surgery and 55 forced boundary decisions on top, up front, before
anything renders.

## Measurement 2: the split is not needed to make the cgame run

Three facts, all verified against the source:

1. **`GetGameAPI` is almost a passive table swap.**
   (`codeJK2/game/g_main.cpp:788`) It assigns `gi = *import`, fills the
   `globals` export table, and makes exactly one call: `GI_Init`, which
   loads gameinfo/weapon/item parms. The earlier crash was not caused by
   init being run — it was caused by init running against the *server's*
   import table, whose `SetConfigstring` writes `sv.configstrings`. With
   a client-safe import table the same init path is harmless (and the
   parms it loads are ones the cgame needs anyway).

2. **`g_entities` and `level` are static arrays inside the library**
   (`g_main.cpp:51`, `g_main.cpp:48`) — not pointers handed over by the
   engine. A second copy of the library loaded into the client process
   owns valid, zeroed instances of both. Every `gent->field` read yields
   zero instead of faulting. Only second-level pointer dereferences
   (`gent->client->…`) can crash, and those are exactly the sites the
   existing 327-guard idiom already handles elsewhere.

3. **The cgame↔engine boundary already exists and already works.** The
   cgame talks to the engine exclusively through `cgi_*` syscalls
   (`CL_CgameSystemCalls`), and `CL_InitCGameVM` needs only the library
   handle. That half needs no change at all.

So the existing single library can simply be **loaded a second time by
the client**, given a client-safe `gi`, and run. No build split, no
55-call boundary negotiation.

## The replacement plan: dual-load

1. **New export in `g_main.cpp`** — `GetCGameAPI(game_import_t *import)`:
   identical to `GetGameAPI` (assign `gi`, run `GI_Init`) but returns
   nothing the server would use. Trivial patch; we control the source.

2. **Client-safe import table in `cl_cgame.cpp`** —
   `CL_BuildCGameImport()`:
   - *Neutral services pass through*: `Printf`, `Error`, `Malloc/Free`,
     `FS_*`, cvar access — these are `qcommon` services with no server
     state.
   - *Configstrings come from the received gamestate*: `getConfigstring`
     reads `cl.gameState`; `SetConfigstring` becomes lookup-only.
     Registration helpers like `G_EffectIndex` then resolve to the
     **server-allocated** indices, so client and server agree by
     construction. A lookup miss must warn loudly and return 0, never
     allocate — a client-side allocation would silently desync every
     index after it.
   - *Server-only services get loud stubs*: `linkentity`, `trace`,
     `SetBrushModel`, the server Ghoul2 half. Each stub logs its name
     once. Every stub that fires at runtime is a discovered work item,
     found by measurement instead of by auditing 127 entries up front.

3. **Hook in `CL_InitCGame`** — when `cgvm.entryPoint` is null, load
   `jospgame`, call `GetCGameAPI(client_import)`, then
   `CL_InitCGameVM(library)`. This is the reverted prototype minus its
   one mistake (using the server table).

4. **Fix `CL_GetDefaultState`** (`cl_cgame.cpp:240`) — it reads
   `sv.svEntities[].baseline` from client code; on a remote client that
   is another null-server landmine. Return a zeroed state.

5. **Crash-driven `gent` burn-down.** Run it. Each fault or wrong visual
   is one `gent->client`-style dereference on a path the remote client
   actually hits (view, player rendering, weapons, events, effects), to
   be guarded and backed by snapshot data. The 849 static count includes
   large paths a co-op client never executes (scoreboard mission stats,
   cutscene camera, in-ATST HUD); the runtime set is what matters, per
   working rule 1: measure the runtime.

### Risks carried forward

- Any client-side path that *registers* media by configstring index must
  be lookup-only (see above); this is the one way dual-load can silently
  desync.
- `cg_pmove` prediction and local traces use client collision
  (`cgi_CM_*`), which works once the client loads the map — but any
  `gi.trace` reached through shared game code will hit a stub and needs
  a client-side equivalent.
- Two copies of the library in one process on the host must never
  happen: the host's `cgvm.entryPoint` is already set by
  `SV_InitGameProgs`, so the `CL_InitCGame` hook naturally skips it.
  Keep it that way — the host path must not change at all.

## Why this beats the split

| | Library split | Dual-load |
|---|---|---|
| Build changes | New target, source-set surgery, per-library duplication of shared code | None |
| 55 cross-calls | All become boundary decisions before anything links | Stay ordinary calls |
| 849 `gent` reads | Still all present, still all broken remotely | Same set, burned down by runtime discovery against zeroed (valid) memory |
| Host regression risk | High — the working singleplayer build is restructured | Near zero — host path untouched |
| First rendered frame on remote client | After the whole refactor | After steps 1–4 |

The split remains available later as a cleanliness refactor if the
boundary ever stabilises. It is not the path to a rendering remote
client.
