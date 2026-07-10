# Implementation plan — remote client rendering and installer

This is a handoff document. It assumes no prior context beyond being a C++
developer; everything project-specific you need is either in here or linked.
Read [cgame-split-investigation.md](cgame-split-investigation.md) first —
it is the measurement basis for Workstream A and explains why the obvious
approach (splitting the cgame into its own library) was rejected.

**Goal recap.** Two players cooperatively playing Jedi Outcast campaign maps
on Linux (Windows later), no cutscenes. Current state: a second client
connects over UDP, spawns displaced from the host, moves, and replicates to
the host's screen — but renders a black screen itself, because its
client-game (cgame) module never runs. Making it run and render is
Workstream A. Shipping the result legally on Linux and Windows is
Workstream C.

---

## 0. Project mechanics (read before touching anything)

### Layout

- `openjk/` — git submodule pinned to upstream JACoders/OpenJK commit
  `2ba5021`. **Never commit to it.**
- `patches/0001…0008` — all our engine changes, applied on top of the pin
  by `tools/apply-patches.sh` (idempotent; safe to re-run).
- `docs/` — this plan, the roadmap, and the investigation history.
- Relevant source trees inside the submodule:
  - `code/` — JK2 singleplayer engine (client, server, qcommon, renderer
    glue). This is the engine we ship.
  - `codeJK2/game/` + `codeJK2/cgame/` — JK2 singleplayer gamecode +
    client-game. Compiled together into **one** shared library,
    `jospgamex86_64.so`.
  - `codemp/` — Jedi Academy multiplayer tree. We do not ship it, but it
    is the reference implementation for everything Raven deleted from SP
    (real netcode, winsock transport, snapshot-only cgame).

### Change workflow (non-negotiable)

1. Edit files inside `openjk/`.
2. Build, regression-test (below), two-client test if relevant.
3. Regenerate or add a patch:
   `git -C openjk diff > patches/000N-short-name.patch`
   - **If you created a new file**, run
     `git -C openjk add -N path/to/new-file` first, or the diff silently
     omits it. This has burned us once.
   - After any `git apply`, verify the file contents actually changed;
     `git apply --3way` has reported "Applied cleanly" while doing nothing.
4. Commit the patch (and docs) to the outer repo **immediately** after the
   regression passes. Work left uncommitted in the submodule has been lost
   twice.

### Build

```sh
tools/apply-patches.sh
cmake -S openjk -B openjk/build -G Ninja \
  -DCMAKE_BUILD_TYPE=RelWithDebInfo \
  -DBuildJK2SPEngine=ON -DBuildJK2SPGame=ON -DBuildJK2SPRdVanilla=ON \
  -DBuildSPEngine=OFF -DBuildSPGame=OFF -DBuildSPRdVanilla=OFF \
  -DBuildMPEngine=OFF -DBuildMPRdVanilla=OFF -DBuildMPDed=OFF \
  -DBuildMPGame=OFF -DBuildMPCGame=OFF -DBuildMPUI=OFF -DBuildMPRend2=OFF
cmake --build openjk/build
```

Iterating on gamecode only: `cmake --build openjk/build --target
jospgamex86_64` (it is symlinked into the engine's search path, so no
reinstall). `RelWithDebInfo` defines `NDEBUG` — **asserts are compiled
out**. A separate `openjk/build-debug` (`-DCMAKE_BUILD_TYPE=Debug`) exists
for anything guarded by assertions, notably the save code.

### Test protocol

Every change, no exceptions:

1. **Loopback regression.** `cd openjk/build && ./openjo_sp.x86_64 +map
   kejim_post` must exit 0, log zero errors, and open no socket
   (networking is off unless `net_enabled 1`). This is the working
   singleplayer build; breaking it is the primary risk of all work below.
2. **Two-client test** (for anything touching the client/server/cgame
   path):
   ```sh
   # host
   ./openjo_sp.x86_64 +set net_enabled 1 +set net_port 29070 +map kejim_post
   # second client — ALWAYS wipe its homepath first; a stale
   # openjo_sp.cfg there has silently broken connections before
   rm -rf /tmp/jk2-client2
   ./openjo_sp.x86_64 +set net_enabled 1 +set fs_homepath /tmp/jk2-client2 \
       +connect 127.0.0.1:29070
   ```
3. When a claim is about runtime behaviour, measure the runtime (gdb,
   probes, `Com_Printf`), not the source. Grep counts and static reasoning
   have been wrong repeatedly in this project — see "Working rules" in
   [roadmap.md](roadmap.md).

Retail assets come from the user's own install
(`.../steamapps/common/Jedi Outcast/GameData/base/assets*.pk3`), symlinked
into `~/.local/share/openjo/base/`. **Never commit any retail file or
anything extracted from one.**

---

## Workstream A — make the remote client render (dual-load)

**Design in one paragraph.** The cgame is compiled into the same shared
library as the server game and gets its import table (`gi`, 127 function
pointers) only when the *server* calls `GetGameAPI`. A remote client runs
no server, so its cgame never initialises and every `VM_Call` is a silent
no-op — black screen. Instead of splitting the library (rejected; see the
investigation doc), the client process loads its own copy of the library
through a new entry point that populates `gi` from a **client-safe** import
table. Because `g_entities` and `level` are static arrays inside the
library, the client's copy has valid zeroed state, and the cgame's 849
direct reads of server entities degrade to reading zeros rather than
faulting — which turns an impossible up-front audit into an incremental,
crash-driven burn-down.

### A1. New export `GetCGameAPI` (patch to `codeJK2/game/g_main.cpp`)

`GetGameAPI` is at `g_main.cpp:788`. Add beside it:

```c
extern "C" Q_EXPORT void QDECL GetCGameAPI( game_import_t *import ) {
    gameinfo_import_t gameinfo_import;

    gi = *import;

    gameinfo_import.FS_FOpenFile = gi.FS_FOpenFile;
    gameinfo_import.FS_Read = gi.FS_Read;
    gameinfo_import.FS_FCloseFile = gi.FS_FCloseFile;
    gameinfo_import.Cvar_Set = gi.cvar_set;
    gameinfo_import.Cvar_VariableStringBuffer = gi.Cvar_VariableStringBuffer;
    gameinfo_import.Cvar_Create = G_Cvar_Create;

    GI_Init( &gameinfo_import );
}
```

i.e. `GetGameAPI` minus the `globals` export wiring. `GI_Init` runs
`WP_LoadWeaponParms` and friends — that is *wanted* (the cgame uses weapon
and item parms); it crashed in the earlier prototype only because the
import table routed `SetConfigstring` to the real server. With A2's table
it is safe.

### A2. Client-safe import table (`code/client/cl_cgame.cpp`)

Write `static void CL_BuildCGameImport( game_import_t &import )`. The
server's version of this table is the 127 assignments in
`SV_InitGameProgs`, `code/server/sv_game.cpp:898–1049` — mirror its shape
exactly and classify every entry into one of three buckets:

1. **Pass-through** — pure `qcommon` services with no server state. Assign
   the same functions the server does: `Printf = Com_Printf`,
   `Error = Com_Error`, `Milliseconds = Sys_Milliseconds2`, all `FS_*`,
   `Malloc/Free`, `cvar*`, `argc/argv/SendConsoleCommand`.
2. **Gamestate-backed** — reimplement against the client's received data:
   - `GetConfigstring` → copy from
     `cl.gameState.stringData + cl.gameState.stringOffsets[index]` (the
     existing pattern at `cl_cgame.cpp:297`).
   - `SetConfigstring` → **lookup-only**. `G_EffectIndex`/`G_SoundIndex`
     et al. call it to *register* strings; on the client the string is
     already in the gamestate at the index the **server** chose, so a
     lookup returns agreement by construction. On a miss: warn once,
     return/no-op. **Never allocate an index client-side** — that would
     silently desync every subsequent index. This is the workstream's one
     truly dangerous bug class.
   - `GetServerinfo` → from `CS_SERVERINFO` in `cl.gameState`.
3. **Loud stubs** — server-only services a client cannot provide:
   `linkentity`, `unlinkentity`, `EntitiesInBox`, `EntityContact`,
   `trace`, `pointcontents`, `SetBrushModel`, `inPVS`, `DropClient`,
   `SendServerCommand`, the server Ghoul2 half (`G2API_*` entries in the
   table), `SetUserinfo`, save-related entries. Each stub prints its own
   name once (`static qboolean warned`) and returns zero/does nothing.
   **Every stub that fires at runtime is a discovered work item** — that
   is the intended discovery mechanism, do not pre-solve all 127.

If `gi.trace`/`pointcontents` turn out to fire on real code paths, back
them with the client collision model (`CM_*` — the client loads the map
via `CM_LoadMap` on `CL_ParseGamestate`/cgame init path); do this only
when a stub actually fires.

### A3. Load hook in `CL_InitCGame` (`code/client/cl_cgame.cpp`)

At the point where `CL_InitCGame` runs with `cgvm.entryPoint == NULL`
(remote client), do what the reverted prototype did, minus its mistake:

```c
if ( !cgvm.entryPoint ) {
    cl_gameLibrary = Sys_LoadSPGameDll( "jospgame", &GetCGameAPI_t );
    game_import_t import;
    CL_BuildCGameImport( import );
    GetCGameAPI( &import );
    CL_InitCGameVM( cl_gameLibrary );
}
```

plus the matching unload in `CL_ShutdownCGame`. `CL_InitCGameVM`
(`cl_cgame.cpp`) already pulls `dllEntry` and `vmMain` from a library
handle — unchanged. **The host path must not change at all**: on the host
`cgvm.entryPoint` is already set by `SV_InitGameProgs` before
`CL_InitCGame` runs, so the branch is naturally skipped. Verify that with
a probe, not by assumption.

The prototype's exact shape (including `Sys_LoadSPGameDll` usage and the
shutdown half) can be recovered from the reflog discussion in
[roadmap.md](roadmap.md) § "Prototype: populating client gi".

### A4. Defuse `CL_GetDefaultState` (`code/client/cl_cgame.cpp:240–258`)

It reads `sv.svEntities[index].baseline` — server memory — from client
code. On a remote client `sv` is zeroed; today it "works" by returning
zeros from valid static memory, but it is wrong by construction. Make it
return a zeroed `entityState_t` when no local server is running
(`com_sv_running->integer == 0`).

### A5. Crash-driven `gent` burn-down

With A1–A4 in place the cgame will initialise, load the map
(`CG_GameStateReceived → cgi_CM_LoadMap` / `cgi_R_LoadWorldMap`,
`codeJK2/cgame/cg_main.cpp:1619/:1219`) and start drawing frames. Then it
will start hitting the real coupling: 849 `->gent` dereferences and 278
direct `g_entities`/`level.` reads against zeroed memory. Zeros render
wrong, `gent->client->…` and other second-level pointers crash.

Protocol per fault/wrong visual:

1. Run the second client under gdb; crash gives file:line.
2. Guard the dereference (`if ( cent->gent && cent->gent->client )` —
   327 such guards already exist as the house idiom) and take the value
   from snapshot data instead: `cent->currentState.*`,
   `cg.snap->ps.*`, or a configstring. The multiplayer cgame
   (`codemp/cgame/`) is the reference for "how to get X without a
   gentity" — it renders everything from snapshots.
3. Rebuild `jospgamex86_64` only, relaunch, repeat.
4. Batch related sites per patch/commit; loopback regression each time
   (host has real `gent`s, so guards must not change host behaviour —
   guard-and-fallback, never remove the `gent` path).

Expect the runtime set to be far smaller than 849: scoreboard mission
stats, cutscene camera, and in-ATST HUD paths never run in co-op.

### Milestones and done-when

| Milestone | Done when |
|---|---|
| M1: cgame runs remotely | Second client reaches `CA_ACTIVE`; probe shows `re.LoadWorld` invoked on the remote client; world visible instead of black screen |
| M2: own view complete | Weapon model + HUD render on the remote client |
| M3: world population | Host player, NPCs, doors render and animate on the remote client |
| M4: playable | Both players fight the same stormtrooper to death; neither process crashes in a 10-minute session |

### Known adjacent defect (fix during A5)

Death screen shows an RGB axis gizmo — an entity whose `modelindex`
arrives 0/garbage, i.e. at least one entry of the restored
`entityStateFields` table (`code/qcommon/msg.cpp`, patch 0006) has a wrong
bit width or order. Audit the table entry-by-entry against
`codemp/qcommon/msg.cpp` and against `entityState_t` **as compiled under
`JK2_MODE`** (the struct is `#ifdef`-conditional; the table already is —
keep them in lockstep; the assert `numFields + 1 == sizeof/4` must hold:
62 + 1 == 63).

---

## Workstream B — save/durability items (deferred, unchanged)

Phase 3 and 4 of [roadmap.md](roadmap.md) stand as written: NPC perception
hardcodes on `g_entities[0]` (`codeJK2/game/NPC_utils.cpp`), the `player`
global (474 reads — classify, do not mass-edit), PLAYERONLY triggers,
`sv_maxclients` as a cvar, N-client `GCLI` save chunks (test against the
Debug build — the asserts are the spec), and porting the challenge
handshake from `codemp/server/sv_main.cpp` before any non-LAN exposure.

---

## Workstream C — installer / distribution (Linux + Windows)

### Licensing model (fixed constraints)

- Engine + gamecode are **GPL** (Raven's 2013 release, via OpenJK). We may
  ship binaries; source obligation is met by this public repo (pin +
  patches).
- Retail `assets*.pk3` are **proprietary**. Never redistributed, never
  committed, never embedded in an artifact — the installer *locates* the
  user's own legal copy (Steam/GOG) and wires it up. `tools/
  build-coop-npcs-pk3.sh` is the existing pattern: extract from the
  user's install at install time.
- Retail installation stays pristine: we add files (or symlinks) only,
  never modify or move retail files. Uninstall = delete what we created.

### C1. Linux installer — `tools/install-coop.sh`

Bash, no dependencies beyond coreutils + the already-required build tools.

1. **Locate GameData** (`--gamedata PATH` overrides autodetection):
   - `~/.steam/steam/steamapps/common/Jedi Outcast/GameData`
   - `~/.local/share/Steam/steamapps/common/Jedi Outcast/GameData`
   - Additional Steam libraries: parse `"path"` entries from
     `steamapps/libraryfolders.vdf` in both roots.
   - Validate by the presence of `base/assets0.pk3`; fail with a clear
     message naming the flag if not found.
2. **Stage the engine data dir** `~/.local/share/openjo/base/`:
   symlink `assets*.pk3` from GameData, symlink the built
   `jospgamex86_64.so` from `openjk/build/codeJK2/game/`.
3. **Renderer link**: `rdjosp-vanilla_x86_64.so` next to the engine binary
   (it is loaded relative to the executable).
4. **Launchers**: install `jk2coop-host [map]` and
   `jk2coop-join <host[:port]> [--second]` into `~/.local/bin/`.
   - host: `openjo_sp.x86_64 +set net_enabled 1 +set net_port 29070 +map ${1:-kejim_post}`
   - join: `+set net_enabled 1 +connect $1`; `--second` adds
     `+set fs_homepath /tmp/jk2-client2` **and wipes that directory
     first** (stale configs there have broken connections).
5. Idempotent; `--uninstall` removes exactly what it created.

### C2. Windows support — prerequisites first

Windows has two hard prerequisites before an installer is meaningful:

1. **Port the UDP transport to winsock.** Patch 0005's
   `code/qcommon/net_ip.cpp` is POSIX-only (`arpa/inet.h`, `fcntl`,
   `close`). `codemp/qcommon/net_ip.cpp` in the same submodule contains
   the winsock variant of every call (`WSAStartup`, `ioctlsocket`
   `FIONBIO`, `closesocket`, `WSAGetLastError`) — port the `#ifdef _WIN32`
   halves into our file. Keep the file's behaviour identical; only the
   syscall spellings change.
2. **Windows builds.** OpenJK builds on MSVC already; add a GitHub Actions
   workflow with a matrix (ubuntu-latest + windows-latest) that runs
   `tools/apply-patches.sh`, configures with the JK2SP flags above, builds,
   and uploads `openjo_sp.exe` / `jospgamex86_64.dll` /
   `rdjosp-vanilla_x86_64.dll` (and the Linux equivalents) as release
   artifacts. This is also the containerised/CI build the project rules
   ask for.

### C3. Windows installer — `tools/install-coop.ps1`

PowerShell 5+ (stock Windows 10/11), same contract as C1:

1. **Locate GameData**: registry `HKCU:\Software\Valve\Steam` →
   `SteamPath`, then `steamapps/libraryfolders.vdf` for extra libraries;
   GOG via `HKLM:\SOFTWARE\WOW6432Node\GOG.com\Games` enumeration;
   `-GameData PATH` override. Validate on `base\assets0.pk3`.
2. **Install binaries into GameData** (the standard OpenJK convention on
   Windows: the engine treats the exe's directory as `fs_basepath`, so no
   symlinks or homepath guessing are needed): copy `openjo_sp.exe`, the
   renderer DLL beside it, and `jospgamex86_64.dll` into `GameData\base\`.
   Additive only — no retail file is touched.
3. **Launchers**: Start-menu/desktop shortcuts or `.cmd` wrappers for host
   and join, mirroring C1's cvars.
4. `-Uninstall` removes exactly the copied files.

A signed NSIS/Inno GUI installer is a possible later polish step; the
scripts are the deliverable.

### Suggested order

C1 is independent of Workstream A and can land immediately (it packages
what already works for the host, and the join launcher is ready for when
A lands). C2.2 (CI) next, then C2.1 (winsock), then C3.

---

## Pitfalls index (each of these has already cost a day or an evening)

- `git apply --3way` claiming success while changing nothing; diffs
  omitting untracked files without `add -N`. Verify file contents.
- `RelWithDebInfo` compiles out `assert()`. The save code's asserts are
  load-bearing tripwires — test saves on the Debug build.
- Stale `openjo_sp.cfg` in a reused `fs_homepath` silently breaks
  connections. Wipe `/tmp/jk2-client2` before every two-client test.
- Grep matches lie: "faces" matched "(all interfaces)", "overflow" matched
  a GL extension name. Read the raw line before believing a surprising hit.
- Loopback buffer: `MAX_LOOPDATA` must stay ≤ `MAX_MSGLEN` (17408); a
  `#error` in `net_chan.cpp` enforces it. Gamestate-sized bursts (~900
  entities) overflow it — that is why baselines are not transmitted and
  newly visible entities are delta'd from a null state instead.
- Heredocs in bash break on lines starting with the delimiter-ish text
  (e.g. `MSG_`); use `git commit -F <file>`.
