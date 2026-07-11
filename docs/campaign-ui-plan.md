# Campaign UI for joiners — plan (Track F)

Status: **planned** (not started). Written 2026-07-11, after the first
human-verified four-player live session.

## The problem

The four-player network layer works: joiners connect, spawn, render the
world, the host player, and NPCs, and play alongside the host. What they
do **not** get is the campaign itself:

- no intro/briefing cinematics or in-game cutscenes,
- no mission objectives (the datapad is empty),
- no mission text, checkpoint messages, or scripted centerprints,
- untested behaviour when the host completes the level and the map
  changes.

This is architectural, not a defect in the wire protocol. Jedi Outcast
singleplayer was written as one process: the server-side gamecode and the
client-side cgame live in the same address space, so Raven's code simply
*calls across the boundary* instead of sending messages. Three findings
from the code (verified 2026-07-11):

1. **Centerprints jump the network — literally.**
   `codeJK2/game/g_target.cpp:1056`:
   `CG_CenterPrint( "@INGAME_CHECKPOINT", … ); //jump the network`
   The gamecode calls the cgame function directly. On the host that
   works; a joiner's dual-loaded cgame never hears about it. Same
   pattern in `g_active.cpp`, `g_misc.cpp`, `g_weapon.cpp`.

2. **The datapad reads server memory through a pointer.**
   `codeJK2/cgame/cg_info.cpp:224` reads
   `cent->gent->client->sess.mission_objectives[i]` — the cgame reaches
   through the entity's `gent` pointer into the server-side
   `clientSession_t` (`codeJK2/game/g_shared.h:619`,
   `objectives_t mission_objectives[MAX_MISSION_OBJ]`). On a joiner,
   `g_entities` is the zeroed local copy (no server sim), so there is
   nothing to read.

3. **Cutscenes are ICARUS camera commands executed in-process.**
   `codeJK2/game/g_camera.cpp` / `Q3_Interface.cpp` drive the cgame
   camera (`CGCam_*`) directly. The full-screen briefing videos
   (`CIN_PlayCinematic`) are also started host-side.

So Track F is a *message-routing* track: identify each host-side campaign
event, give it a wire representation (configstring or server command),
and teach the joiner's cgame to consume it.

## Design principles

- **The host player drives the story.** Triggers, ICARUS scripts, and
  level progress key off the host (entity 0). Joiners are companions.
  This matches how the live session is actually played.
- **Prefer configstrings for state, server commands for events.**
  Objectives are *state* (a late joiner must receive the current set →
  configstrings, which are part of the gamestate). Centerprints and
  "objective updated" pings are *events* → `SV_SendServerCommand(NULL, …)`
  broadcasts (the transport already delivers server commands to every
  primed client — verified in `sv_main.cpp`).
- **Joiners spectate cutscenes; they are not actors in them.** Replaying
  the ICARUS camera path on every client is the deluxe version; the MVP
  is: freeze joiner input + letterbox while the host is in a cutscene,
  release when it ends. Full-screen briefing videos stay host-only.
- **Never regress the solo game.** Every change must keep the loopback
  regression green; host-side behaviour with zero joiners must be
  byte-identical in effect.

## Tasks

### F1 — objective sync (state)

Needs: nothing (first Track F task).
Do: allocate a configstring range for objectives (one string per
`MAX_MISSION_OBJ` slot, packed `text\status\display`), written server-side
whenever `mission_objectives` changes (hook the ICARUS objective verbs
and `OBJ_*` setters). In the joiner's cgame, populate a local mirror from
the configstrings and switch `cg_info.cpp`'s datapad reader to use it when
`cg_remoteClient` (fall through to the direct `gent` read on the host, so
solo behaviour is untouched). Bump `PROTOCOL_VERSION`.
Done: joiner opens the datapad and sees the same objectives as the host,
including one completed mid-session; late joiner sees the current set on
connect; solo datapad unchanged.

### F2 — mission text + centerprint sync (events)

Needs: F1 (establishes the routing pattern).
Do: in the gamecode, replace the direct `CG_CenterPrint(…)` calls that
carry campaign text (`g_target.cpp`, `g_misc.cpp`, `g_active.cpp`,
`g_weapon.cpp`) with a helper that centerprints locally on the host AND
broadcasts `cp "<key>"` via `SV_SendServerCommand(NULL, …)`. Handle `cp`
in the joiner cgame's server-command dispatcher (the string keys like
`@INGAME_CHECKPOINT` resolve through the string packages each client
already loads). Objective-updated pings ride the same channel.
Done: when the host hits the first checkpoint trigger on `kejim_post`,
every joiner shows the same centerprint; solo unchanged.

### F3 — cutscene handling for joiners (MVP: spectate/freeze)

Needs: F2.
Do: server-side, when the host enters/leaves an ICARUS camera
(`g_camera.cpp` enable/disable), broadcast `cutscene 1|0`. Joiner cgame on
`cutscene 1`: letterbox bars, suppress its own input (drop usercmds or
zero them client-side), optionally display "cutscene in progress"; on
`cutscene 0`: restore. Joiners keep rendering their own view (frozen
input, world visible). Full-screen `CIN_PlayCinematic` briefings stay
host-only — joiners see a "briefing in progress" card instead.
Done: host triggers the kejim_post intro cutscene; joiners letterbox +
freeze for its duration and resume cleanly after; no joiner can wander
into trigger volumes mid-cutscene.

### F4 — level transition follow

Needs: F1 (worth verifying early, though).
Do: establish what actually happens today when the host completes
kejim_post (`target_level_change` → `SV_SpawnServer`): do joiners receive
a new gamestate and reload, or hang/desync? Fix whichever path fails —
the expected shape is the standard Q3 map-change flow (new gamestate,
clients reload the map and re-enter), plus re-running the patch-0008
spawn ring on the new map.
Done: host finishes kejim_post; all joiners load `artus_mine`
(or the next map) and spawn clear; nobody needs to manually reconnect.

### F5 — verification harness + live session

Needs: F1–F4.
Do: extend the headless harness: scripted host walks the opening of
kejim_post (console-driven movement is already proven) far enough to hit
the checkpoint trigger and complete objective 1; assert the joiner log
shows the `cp` broadcast, the objective configstring update, and (for F4)
the map-change reload. Then a live session: human hosts the full first
level with 3 bots.
Done: harness green in CI-able form; live session plays a level start to
finish with objectives + text visible on a joiner window.

## Suggested order

F1 → F2 → F3 → F4 → F5. F4 can be *investigated* any time (it is the
biggest unknown); its fix lands after F2's routing pattern exists.

## Out of scope (explicitly)

- Replaying ICARUS camera paths on joiners (deluxe cutscene mode).
- Joiner-visible briefing videos (`CIN_PlayCinematic` on joiners).
- Per-joiner objectives or divergent story state — the host is canon.
- Saving/loading co-op sessions (SP save code assumes one client; its
  asserts are the Debug-build tripwires noted in docs/building.md).
