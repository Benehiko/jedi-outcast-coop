# Widening the Singleplayer Engine

Progress log for the route chosen in [route-comparison.md](route-comparison.md).

## Milestone 1: `MAX_CLIENTS = 2` — complete

The singleplayer engine now allocates two client slots, reserves two
entity slots for them, and loads the campaign map `kejim_post` with no
errors and a clean shutdown.

### What changed

`patches/0004-widen-sp-max-clients.patch`, six files:

| File | Change |
|---|---|
| `code/qcommon/q_shared.h:618` | `MAX_CLIENTS` 1 → 2 |
| `code/server/sv_init.cpp:179` | `Z_Malloc(sizeof(client_t) * 1)` → `* MAX_CLIENTS` |
| `code/server/sv_init.cpp:326` | `for (i=0; i<1; i++)` → `i<MAX_CLIENTS` |
| `code/server/sv_client.cpp:100` | same |
| `code/server/sv_game.cpp:1091` | same |
| `code/server/sv_main.cpp:195` | same |
| `code/server/sv_main.cpp:229` | same |
| `code/server/sv_main.cpp:245` | `sv_maxclients` info string literal `1` → `MAX_CLIENTS` |
| `codeJK2/game/g_main.cpp:654` | `level.maxclients = 1` → `MAX_CLIENTS` |
| `codeJK2/game/g_main.cpp:659` | `g_entities[0].client = level.clients` → loop over all clients |

The loop bounds were replaced with `MAX_CLIENTS` rather than a cvar, so
that raising the define is the single point of control. Introducing a
real `sv_maxclients` cvar is a later step.

### What was verified

- All three singleplayer targets compile with no new warnings.
- `openjo_sp.x86_64 +map kejim_post` exits 0.
- ICARUS initialises, entities spawn, `ShutdownGame` runs cleanly.
- Zero `ERROR` lines, unchanged from the one-client baseline.

Nothing in the singleplayer codebase statically depends on
`MAX_CLIENTS == 1`. That was not obvious in advance and is the main
result of this milestone.

### What was *not* verified

**A second client has not connected.** The engine has two slots; nothing
has occupied the second one. The singleplayer client connects over a
loopback netchan and there is currently no way to attach a second one.
That is milestone 2.

## The save system

The original design document predicted the save system would be the first
thing to break, citing two assertions Raven left as tripwires:

```c
assert(level.maxclients == 1);  // I'll need to know if this changes,
                                // otherwise I'll need to change the way ReadGame works
```

**It did not break, and the assertions did not fire.** Two facts explain
why, and both matter for the work ahead.

First, `RelWithDebInfo` defines `NDEBUG`, so `assert()` compiles to
nothing. A test in that configuration would have silently written a
corrupt save rather than tripping the tripwire. A separate `Debug` tree
was built (`openjk/build-debug`) specifically so the assertions are live —
confirmed by `nm -u` showing two undefined `assert` references in the debug
gamecode and none in the release one.

Second, and more usefully: **both assertions sit inside `if (!qbAutosave)`.**

```c
void WriteLevel(qboolean qbAutosave)
{
    if (!qbAutosave) //-always save the client
    {
        assert(level.maxclients == 1);
        gclient_t client = level.clients[0];
        EnumerateFields(savefields_gClient, &client, INT_ID('G','C','L','I'));
```

Autosaves skip client serialisation entirely. The map's `target_autosave`
fired during testing and wrote two 40 KB saves with `maxclients == 2`
without touching the guarded path. Only a *manual* save reaches it.

### The actual shape of the save problem

`WriteLevel` writes exactly one `GCLI` chunk containing `level.clients[0]`.
`ReadLevel` reads exactly one and copies it back to `level.clients[0]`.
Six sites, two functions (`g_savegame.cpp:997,998,1044,1059,1062,1063`).

Generalising it means writing N chunks and reading N, plus a save-format
version bump so existing saves still load. This is bounded and mechanical.
It is not the blocker the original document implied — but it is real, and
it will fail loudly the first time a manual save happens with two clients,
which is the correct behaviour.

## Milestone 2: a second client

The engine has two client slots and no way to fill the second. What is
needed:

- `SV_DirectConnect` currently accepts one loopback client. The netchan,
  snapshot, and usercmd machinery are all present and compiled in
  (`msg.cpp`, `net_chan.cpp`, `sv_snapshot.cpp`, `sv_client.cpp`).
- The singleplayer client (`code/client/`) connects to `localhost`, which
  `NET_StringToAdr` traps as `NA_LOOPBACK`. A second, non-loopback client
  would need the UDP path, which exists but is unexercised.
- `sv_maxclients` is not a cvar in the singleplayer engine. It should
  become one before the second client is useful.

## Open questions

- The global `player` symbol (`g_main.cpp:141`) is unconditionally aliased
  to `&g_entities[0]` and read 474 times. It has only six assignment sites.
  Nothing has been done about it yet, and nothing needed to be for this
  milestone.
- PLAYERONLY triggers test `other->s.number != 0` (`g_trigger.cpp:194`).
  A second player will not trip them.
- `cg_media.h:356` sizes `clientinfo[MAX_CLIENTS]`, which now has two
  entries. The client-game has not been examined for other single-player
  assumptions.
