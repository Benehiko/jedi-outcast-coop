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
| Client sends the connect packet | **Blocked** — see Phase 1 |
| Two players in one world | Not started |
| Two players fighting NPCs | Not started |

Everything below Phase 1 is unverified. Estimates are ordinal, not
calendar: each phase is expected to be harder than the one above it.

---

## Phase 1 — Land the second client

**The one blocker.** `CL_Connect_f` sets `cls.state = CA_CHALLENGING`, but
the engine primes its own local client to `CA_PRIMED` during cgame
initialisation (`code/client/cl_cgame.cpp:1419`). `CL_CheckForResend` only
transmits while the state is between `CA_CONNECTING` and `CA_CHALLENGING`
(`cl_main.cpp:536`), so the connect packet is never sent.

The singleplayer client assumes it is always attached to its own local
server. A remote connection has to suppress or unwind that assumption.

### Tasks

1. **Decide the mechanism.** Two candidates, and they differ in blast radius:
   - Defer `CL_Connect_f` until after client initialisation completes, so it
     runs against a settled state machine rather than racing it.
   - Introduce a "client only" mode that skips the local-server attach
     entirely, closer to how the multiplayer client starts.

   The second is cleaner and larger. Prototype the first to learn what the
   state machine actually does.

2. **Instrument before changing.** Probe `cls.state` transitions through
   startup. The last two blockers in this project were found by reading a
   live value, not by reading source. Do that first.

3. **Make the client send `connect`.** Success is a single line in the host
   log: `SVC_DirectConnect`.

4. **Complete the handshake.** `SV_DirectConnect` → `SV_SendClientGameState`
   → `CL_ParseGamestate` → `CA_PRIMED` on the *remote* client. The netchan,
   snapshot and usercmd machinery are all present and compiled; none of it
   has ever run over a real socket in this engine.

### Done when

The host logs `ClientConnect: 1` and `ClientBegin: 1` for a second
`openjo_sp` process, and both processes stay up.

### Known hazards

- The `qport` mechanism (`net_chan.cpp`) exists but has never carried two
  clients. Two clients behind one address must not collide.
- `svs.numSnapshotEntities` was widened to `MAX_CLIENTS * 4 * 64`. Whether
  `4` is the right backup factor for this engine is untested.

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
