# Dual-load `gent` burn-down log (Workstream A5)

Running record of crash sites hit by the remote client after the dual-load
cgame starts executing (A1–A4 landed). Each entry is a fault or wrong visual
discovered by running the second client under gdb against a host, per the A5
protocol in [tasks.md](tasks.md) and [implementation-plan.md](implementation-plan.md).

Test recipe (note the port-collision trap): a stale MP `openjkded` on the
default 29070 will shadow the host; always confirm the host actually bound
its port (`Opening IP socket: 0.0.0.0:<port>`) and connect to *that* port.
The second client needs assets reachable — it wipes its own `fs_homepath`
(`/tmp/jk2-client2`) for a clean config but points `fs_basepath` at
`~/.local/share/openjo` so `assets*.pk3` and the gamecode `.so` resolve.

## A3 milestone — dual-load fires

The remote client enters the dual-load branch, loads its own copy of
`jospgame`, runs `GetCGameAPI` with the client-safe import table, and
`CL_InitCGameVM` succeeds. The cgame then executes real code all the way
through `CG_Init → CG_GameStateReceived → CG_RegisterSounds`. No import
stub fired before the first crash — the renderer/collision/sound
pass-throughs all resolved. The architecture works; the burn-down begins.

## Discovered work items (crash-driven)

### #1 — `as_preCacheMap` is null in `CG_AS_Register`

- **Site:** `codeJK2/cgame/cg_main.cpp:637`, inside `CG_AS_Register`, at
  `STL_ITERATE( pi, (*as_preCacheMap) )`.
- **Backtrace:** `CG_AS_Register → CG_RegisterSounds → CG_GameStateReceived
  → CG_Init → vmMain → VM_Call → CL_InitCGame`.
- **Cause:** `as_preCacheMap` (the ambient-sound-set precache map) is
  populated by the server-side ambient-sound parse; on a serverless remote
  client it is null, and the dereference faults. This is the first true
  `gent`/server-state coupling the burn-down must guard.
- **Status:** open — first A5 item.
