# Widening the Singleplayer Engine

Progress log for the route chosen in [route-comparison.md](route-comparison.md).

## Milestone 1: `MAX_CLIENTS = 2` — complete

The singleplayer engine now allocates two client slots, reserves two
entity slots for them, and loads the campaign map `kejim_post` with no
errors and a clean shutdown.

### What changed

`patches/0004-widen-sp-max-clients.patch`, seven files.

The route comparison counted eight engine sites. **It was thirteen.** Five
were missed because they iterate `svs.clients` with a different loop
idiom (`for (i=0,cl=svs.clients ; i < 1 ; i++,cl++)`) that the original
grep pattern did not match. One of them, the snapshot send loop, is the
function that transmits world state to clients — without it a second
client would connect and receive nothing.

| File | Change |
|---|---|
| `code/qcommon/q_shared.h:618` | `MAX_CLIENTS` 1 → 2 |
| `code/server/sv_init.cpp:179` | `Z_Malloc(sizeof(client_t) * 1)` → `* MAX_CLIENTS` |
| `code/server/sv_init.cpp:180` | `numSnapshotEntities = 2 * 4 * 64` → `MAX_CLIENTS * 4 * 64` |
| `code/server/sv_init.cpp:326` | `for (i=0; i<1; i++)` → `i<MAX_CLIENTS` |
| `code/server/sv_client.cpp:77` | reconnect slot reuse — **missed initially** |
| `code/server/sv_client.cpp:100` | free-slot search |
| `code/server/sv_game.cpp:1091` | clear gentity pointers |
| `code/server/sv_main.cpp:149` | send data to all relevant clients — **missed initially** |
| `code/server/sv_main.cpp:195` | client loop |
| `code/server/sv_main.cpp:229` | connected-client count |
| `code/server/sv_main.cpp:245` | `sv_maxclients` info string literal `1` → `MAX_CLIENTS` |
| `code/server/sv_main.cpp:318` | identify packet sender — **missed initially** |
| `code/server/sv_snapshot.cpp:709` | snapshot send loop — **missed initially** |
| `codeJK2/game/g_main.cpp:654` | `level.maxclients = 1` → `MAX_CLIENTS` |
| `codeJK2/game/g_main.cpp:659` | `g_entities[0].client = level.clients` → loop over all clients |

The snapshot ring buffer at `sv_init.cpp:180` is sized per client. The
multiplayer tree makes this explicit:

```c
// multiplayer
svs.numSnapshotEntities = sv_maxclients->integer * 4 * MAX_SNAPSHOT_ENTITIES;
// singleplayer, before
svs.numSnapshotEntities = 2 * 4 * 64;
```

The leading `2` is not a client count — the multiplayer equivalent uses
`PACKET_BACKUP` there — but the factor must nonetheless scale with clients
or one client's snapshots overwrite another's.

The loop bounds were replaced with `MAX_CLIENTS` rather than a cvar, so
that raising the define is the single point of control. Introducing a
real `sv_maxclients` cvar is part of milestone 2.

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

## Correction: the singleplayer engine has no network transport

The route comparison stated that the singleplayer engine "already speaks
Quake 3's network protocol." That is true of the **protocol** and false of
the **transport**, and the distinction was not checked before the claim
was made.

Present and compiled in: delta-compressed entity snapshots
(`sv_snapshot.cpp`), usercmd handling (`sv_client.cpp`), netchan
sequencing and fragmentation (`net_chan.cpp`), and message
serialisation (`msg.cpp`).

Absent: the wire.

- **`net_ip.cpp` does not exist** in `code/qcommon/`. The multiplayer tree
  has it, at 1,085 lines. `net_chan.cpp` is the only network file the
  singleplayer engine compiles (`code/CMakeLists.txt:169`).
- **`NET_Init` is an empty inline stub**, annotated by OpenJK's own
  maintainers:

  ```c
  // code/qcommon/qcommon.h
  // For compatibility with shared code
  static inline void NET_Init( void ) {}
  static inline void NET_Shutdown( void ) {}
  ```

- **`NET_SendPacket` silently discards anything that is not loopback.**
  The function body handles `NA_LOOPBACK` and then returns; there is no
  `else` branch and no socket call.

  ```c
  void NET_SendPacket( netsrc_t sock, int length, const void *data, netadr_t to ) {
      if ( to.type == NA_LOOPBACK ) {
          NET_SendLoopPacket (sock, length, data, to);
          return;
      }
  }
  ```

- **`NET_StringToAdr` resolves exactly one address.** Only the literal
  string `"localhost"` succeeds, mapping to `NA_LOOPBACK`. Everything else
  becomes `NA_BAD`.

- The only receive path is `NET_GetLoopPacket`, reading a two-entry
  in-memory ring buffer.

`nm` on the built binary confirms this: `openjo_sp.x86_64` contains eight
`NET_*` symbols, all from `net_chan.cpp`, and none of `NET_Init`,
`NET_GetPacket`, or `NET_OpenIP`.

Raven did not merely cap the client count. They removed the UDP transport
and left the protocol layer running over an in-memory buffer. The single
player is a network client with no network.

## Milestone 2: the transport — done; the connection — not yet

`patches/0005-sp-udp-transport.patch` adds `code/qcommon/net_ip.cpp`, a
reduced port of the multiplayer socket layer, and wires it in.

### Verified

- `openjo_sp.x86_64` now links `socket`, `bind`, `recvfrom`, `sendto` and
  `gethostbyname`. Before, it contained eight `NET_*` symbols and no
  syscalls.
- With `net_enabled 1 net_port 29090`, `ss` reports the process holding
  `UDP 0.0.0.0:29090`.
- With `net_enabled` unset the game opens no socket, loads `kejim_post`,
  exits 0 and logs zero errors — singleplayer is untouched.
- A second `openjo_sp` process resolves the host address and reaches
  `SV_DirectConnect`, which assigns it client slot 1.

### Three more functions Raven had gutted

Found by running the connection, not by reading:

```c
const char *NET_AdrToString (netadr_t a) {
    if (a.type == NA_LOOPBACK) { ... }
    return s;                      // NA_IP: returns the stale static buffer
}

qboolean NET_CompareAdr (netadr_t a, netadr_t b) {
    if (a.type == NA_LOOPBACK) return qtrue;
    Com_Printf("bad address type"); return qfalse;   // every IP compares unequal
}
```

`NET_CompareBaseAdr` had the same omission. A server that cannot compare
two IP addresses cannot match an incoming packet to a client, so this was
fatal rather than cosmetic.

Four client-index bounds checks were also still hardcoded, and one of them
produced a visible `SV_GetUserinfo: bad index 1` error dialog the first
time a second client was accepted — which was itself the proof that
`SV_DirectConnect` had assigned slot 1 over the wire:

| Site | Function |
|---|---|
| `sv_init.cpp:104` | `SV_SetUserinfo` |
| `sv_init.cpp:127` | `SV_GetUserinfo` |
| `sv_game.cpp:107` | `SV_GameSendServerCommand` |
| `sv_game.cpp:123` | `SV_GameDropClient` |

### Where it stops

The client-side `connect <address[:port]>` command did not exist; the
singleplayer client only ever attached to its own server via
`CL_MapLoading`. `CL_Connect_f` has been added and registered.

It resolves the address and sets `cls.state = CA_CHALLENGING`, but the
engine's own startup subsequently drives the local client to `CA_PRIMED`
(state 6). `CL_CheckForResend` only transmits while state is between
`CA_CONNECTING` and `CA_CHALLENGING`, so the `connect` packet is never
sent and the host never sees a connection attempt.

This is a client-state-machine problem, not a transport problem. The
singleplayer client assumes it is always attached to a local server;
connecting to a remote one requires suppressing that local attach, or
deferring `CL_Connect_f` until after client initialisation completes.

That is the next piece of work.

## Reference: restoring the network transport

This is a larger task than "connect a second client", and it is the real
cost of the widen-singleplayer route.

The work is to port `codemp/qcommon/net_ip.cpp` into the singleplayer
engine. It is not a copy: the two trees' function signatures differ, most
visibly

```c
// singleplayer: by value
void NET_SendPacket (netsrc_t sock, int length, const void *data, netadr_t to);
// multiplayer:  by pointer
void NET_SendPacket (netsrc_t sock, int length, const void *data, const netadr_t *to);
```

Much of the multiplayer file is not needed — SOCKS proxying, IPX, master
server heartbeats. The essential surface is `NET_Init`, `NET_Shutdown`,
`NET_Config`, `NET_OpenIP`, `NET_GetPacket`, and a real `NET_SendPacket`
and `NET_StringToAdr`.

Once a packet can cross a socket, `SV_DirectConnect` already searches for
a free slot among `MAX_CLIENTS` and rejects with "Server is full" — that
path is now correct and untested.

`sv_maxclients` should become a real cvar at the same time, replacing the
`MAX_CLIENTS` compile-time bound in the loops widened above.

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
