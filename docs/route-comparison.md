# Route Comparison, Re-priced

**Status:** Supersedes the recommendation in [coop-design.md](coop-design.md).
**Date:** After the first playable co-op test.

## Summary

The original study recommended hosting the Jedi Outcast campaign on the
Jedi Academy multiplayer tree, because that tree's netcode was already
multiplayer. That recommendation was made before either route had been
run.

Both routes have now been measured. **The recommendation is reversed.**
Widening the singleplayer engine is substantially cheaper than making
Jedi Outcast's assets work under Jedi Academy's engine.

The original comparison priced one axis — netcode — and assumed the rest
was free. The rest is not free.

## What changed

Two facts, neither visible from reading structure.

**Jedi Academy's animation enum diverges from Jedi Outcast's from index 1
onward.** Jedi Outcast defines 1,202 animation entries
(`codeJK2/game/anims.h`); Jedi Academy defines 1,534
(`codemp/game/anims.h`). Entries were both removed and inserted
throughout, not appended. The shipped `models/players/_humanoid/animation.cfg`
in `assets0.pk3` carries 980 entries and is indexed by those enum values.

Consequently every animation index above the first divergence resolves to
the wrong frame range. Player and NPC models render collapsed, and NPCs
play animations that never reach the frame events which fire their
weapons. This affects all 21 `.gla` skeletons and every `.glm` model bound
to them.

There is no configuration that fixes this. It requires either a
runtime remapping table between the two enums, derived and validated
animation by animation, or re-rigging Jedi Outcast's models against Jedi
Academy's skeleton — which discards the assets that motivate the project.

**The singleplayer engine's client cap is thirteen sites.** Raven did not
remove the multiplayer architecture from the singleplayer engine. They set
it to one and left the original value in a comment:

```c
#define MAX_CLIENTS  1 // 128      // code/qcommon/q_shared.h:618
```

The delta-compressed snapshot system, the netchan, usercmd handling, and
the client/server split are all present and compiled into `openjo_sp`.
The cap is a dial.

> **Corrected after implementation.** This section originally said "eight
> sites" and claimed the singleplayer engine "already speaks Quake 3's
> network protocol." The count was thirteen — five loops over
> `svs.clients` used an idiom the survey grep missed, including the
> snapshot send loop. And the protocol claim is only half right: the
> protocol layer is intact, but the **transport is absent**. `net_ip.cpp`
> does not exist in the singleplayer tree, `NET_Init` is an empty inline
> stub, and `NET_SendPacket` silently discards anything that is not
> loopback. Restoring the transport, not raising the cap, is the real cost
> of this route. See [widen-sp-progress.md](widen-sp-progress.md).
>
> This does not reverse the recommendation. Porting `net_ip.cpp` is a
> bounded, well-understood task against working code; the animation enum
> divergence on the multiplayer route is not.

## Measured costs

### Widen singleplayer

| Item | Sites | Notes |
|---|---:|---|
| `MAX_CLIENTS` | 1 | `q_shared.h:618`, original value `128` in comment |
| `svs.clients` allocation | 1 | `sv_init.cpp:179`, `sizeof(client_t) * 1` |
| Hardcoded `i < 1` client loops | 5 | `sv_client.cpp:100`, `sv_game.cpp:1091`, `sv_init.cpp:326`, `sv_main.cpp:195,229` |
| `sv_maxclients` info string | 1 | `sv_main.cpp:245`, literal `1` |
| **Engine subtotal** | **8** | |
| `level.maxclients = 1` | 1 | `g_main.cpp:654`, plus one-slot `G_Alloc` at `:655` |
| Save format `level.clients[0]` | 6 | `g_savegame.cpp:997,998,1044,1059,1062,1063` — two functions |
| Global `player` assignments | 6 | `g_main.cpp:217,722`; three are function-local shadows |
| PLAYERONLY trigger checks | ~3 | `g_trigger.cpp:194,103,1474` |
| **Gamecode subtotal** | **~16** | |

`ClientConnect`, `ClientBegin`, and `ClientSpawn` already take a
`clientNum` parameter (`g_client.cpp:534,590`). Client loops in the
gamecode already iterate `level.maxclients` generically.

The 474 reads of the global `player` are the real work, but they are not
474 edits. `player` has only **six assignment sites**, three of which are
function-local shadows. The reads need `player` to resolve to "the
relevant player" — a dispatch problem, not a rewrite. The honest estimate
for that work is unknown until attempted, but it is bounded by a single
symbol.

### Campaign on multiplayer

| Item | Cost | Status |
|---|---|---|
| Asset path names (menus, HUD, NPCs) | 2 cvars + 2 patches | **Done**, see coop-design.md |
| NPC team parse (`enemyTeam = -1`) | 1 patch | **Done**, upstream defect |
| `playerTeam` never set for clients | 1 patch | **Done**, upstream defect |
| Clients join as spectators | 1 cvar or patch | Diagnosed, not fixed |
| Missing spawn functions | ~40 classnames | Non-fatal; 14 on `kejim_post` |
| Animation enum divergence | **1,202 vs 1,534 entries** | **No known fix** |
| String tables (`MP_INGAME`) | unknown | `??` throughout the HUD |
| NPC AI port completeness | unknown | `if (0)` stubs, `rwwFIXMEFIXME` markers |

## Side by side

| | Widen singleplayer | Campaign on multiplayer |
|---|---|---|
| Netcode | Present, capped at 1 (8 sites) | Works |
| Animations | Native, correct | 1,202 vs 1,534 enum entries |
| Models | Native, correct | Render collapsed |
| NPC AI | Works — it runs the retail game | Partially ported; two defects found and fixed, more suspected |
| Strings and HUD | Native, correct | `??` throughout |
| Save format | 6 sites to generalise | Absent entirely |
| Principal risk | 474 reads of a global | An engine that cannot display the assets |

## Recommendation

Widen the singleplayer engine.

The singleplayer engine's assets, animations, AI, strings, and save system
are all natively correct, because it is the game those assets shipped
with. Its only defect for this purpose is a client cap that Raven
implemented as a `#define` and five loop bounds.

The multiplayer route requires making a different game's engine display
this game's data. Every layer touched so far — menus, HUD, NPC
definitions, team parsing, and now animations — has been a mismatch. Four
were cheap. The animation enum is not, and it is load-bearing for models,
combat, and saber hit detection.

## What is worth keeping from the multiplayer work

The multiplayer branch produced real value even though the route is being
abandoned:

- **Two upstream bugs, found and fixed.** The NPC team parse
  (`patches/0003`) and the unset client `playerTeam` (`patches/0002`) are
  genuine defects in OpenJK's multiplayer tree, worth upstreaming
  regardless of which route this project takes.
- **A working reference.** Two clients did connect, spawn, see each other,
  and fight. `Kill: 0 620 3: Kyle killed stormtrooper2 by MOD_SABER`. The
  netcode behaviour it demonstrates is the target to reproduce in the
  singleplayer engine.
- **A confirmed asset inventory.** Campaign maps load, entities spawn,
  Ghoul2 models resolve.

## Proposed next milestone

Raise `MAX_CLIENTS` in the singleplayer engine to 2, generalise the eight
engine sites and `level.maxclients`, and observe what breaks. The save
system will break — Raven left an assertion saying so
(`g_savegame.cpp:997`). Everything else is unknown, which is the point of
doing it.
