# Task breakdown

The [implementation plan](implementation-plan.md) sliced into tasks sized
for one sitting each. Work top to bottom within a track; tracks A, C and D
are independent of each other and can be worked in parallel or interleaved.
Track E went last of the original plan; Track F (campaign UI for joiners,
[campaign-ui-plan.md](campaign-ui-plan.md)) is the current frontier.

Every task ends the same way: **run the loopback regression** (`cd
openjk/build && ./openjo_sp.x86_64 +map kejim_post` → exit 0, no errors,
no socket), regenerate the patch, and **commit immediately**. The patch
workflow and its traps are in plan § 0 — read that section before task T0
and believe it.

Patch numbers below start at `0009` and assume tasks land in the listed
order; renumber freely if they don't. One patch per task unless noted.

Legend: each task lists what it **needs** (dependencies), what to
**do**, and what **done** looks like (a check you can run, not a feeling).

## Status (2026-07-11)

### Completed

- **Track A** (remote client renders): A1–A6 done. M3 (host players +
  NPCs render on the joiner) = patch **0014**, character velocity/lean =
  **0015**; M4 render-stability confirmed by a 10-minute headless soak.
- **Track C** (distribution): C1 Linux installer, C2 CI
  (`.github/workflows/build.yml`), C3 winsock (patch **0016**), C5 macOS
  installer (`tools/install-coop-macos.sh`, logic validated off-Mac).
  Patch 0007 regenerated so `apply-patches.sh` runs clean.
- **Track D** (co-op UX): D1 `coop_host` (**0017**), D2 `localservers`
  LAN discovery (**0018**), D3 in-game Co-op menu (**0019** +
  `zz-coop-ui.pk3`). Verified headless via `uimenu coopMenu` + screenshot.
- **Track E** (four players): E1 `sv_maxclients` + E2 cap raise +
  protocol bump + E3 headless four-player verification (**0020**, incl.
  the loopback qport-collision root cause), and **E4 — the first
  human-verified LIVE session**: the developer hosted the kejim_post
  campaign in a real window with three bot joiners, four players in one
  game ("and it works"). Patch **0021** fixed the two bugs that session
  surfaced: disconnected joiner slots never left CS_ZOMBIE
  (`SV_CheckTimeouts` only examined slot 0), and rejected connects sat on
  a silent loading screen instead of showing the server's message.
- GPLv2 `LICENSE` at root, per-OS install guides
  (`docs/install-*.md`), and README license/trademark sections.

### Outstanding

- **Track F — campaign UI for joiners** (planned, not started): joiners
  get world + entities but no objectives, mission text, cutscene
  handling, or verified level transitions. Plan + task breakdown:
  [campaign-ui-plan.md](campaign-ui-plan.md); tasks F1–F5 below.
- **C4 — Windows installer**: the engine now links under MSVC — patch
  0005 links `wsock32` into the JK2SP engine, fixing the 13 winsock
  `LNK2019` unresolved externals (`WSAStartup`, `socket`, `bind`,
  `sendto`, …) that patch 0016's `net_ip.cpp` introduced but never
  linked, so the CI `jk2coop-windows` artifact now builds (the artifact
  glob was also corrected: the engine is `openjo_sp.x86_64.exe`, not
  `openjo_sp.exe`). The installer `tools/install-coop.ps1` is written —
  it autodetects `GameData` via the Steam registry key +
  `libraryfolders.vdf`, stages the co-op files additively (retail
  untouched), writes host/join `.cmd` launchers, and supports
  `-Uninstall`; its logic is verified end-to-end on a mock tree. What
  remains is running it against a real retail install on a live Windows
  machine and confirming the engine hosts/joins there.
- **C6 — macOS real-hardware verification**: `install-coop-macos.sh` is
  shellcheck-clean and validated against a mock build tree on Linux; it
  has not yet been run on a real Mac.
- **Extended live combat soak**: E3's "10-minute four-player firefight"
  in a live window — the E4 session verified live multi-player campaign
  play but was not a timed combat soak.

Patch numbers in task parentheticals below diverged from what landed
(C3 is 0016 not 0015; A6 needed no patch; D1/D2/D3 are 0017/0018/0019;
E1–E3 are 0020; E4 fixes are 0021). The only D3 piece not
machine-verified is mouse-clicking the buttons; the verb code paths and
the menu/feeder rendering are confirmed.

---

## T0 — Environment check (do this first)

- [x] **T0.1 Build and run what exists.**
  Needs: retail JK2 on disk, the packages in README § Building.
  Do: clone with submodule, `tools/apply-patches.sh`, configure + build
  per README, symlink assets and modules per README § Running, run the
  loopback regression, then run the two-client test from plan § 0
  (remember: wipe `/tmp/jk2-client2` first).
  Done: host window plays normally; second client connects, host logs
  `Kyle connected`, host can see the second Kyle move; second client's
  own window is black (that is the bug Track A fixes).

---

## Track A — remote client renders (dual-load)

Background: plan § Workstream A and
[cgame-split-investigation.md](cgame-split-investigation.md). Do not
reorder A1–A4; each is a prerequisite of the next. A5 is a loop.

- [x] **A1 — `GetCGameAPI` export.** (patch 0009)
  Needs: T0.
  Do: in `openjk/codeJK2/game/g_main.cpp`, beside `GetGameAPI` (line
  ~788), add the `GetCGameAPI` function exactly as sketched in plan
  § A1: assign `gi = *import`, build `gameinfo_import` the same way
  `GetGameAPI` does, call `GI_Init`. No `globals` wiring, returns void.
  Done: `nm -D openjk/build/codeJK2/game/jospgamex86_64.so | grep
  GetCGameAPI` shows the symbol; loopback regression passes (nothing
  calls it yet).

- [x] **A2 — client-safe import table.** (patch 0010, same patch as A3)
  Needs: A1.
  Do: in `openjk/code/client/cl_cgame.cpp`, add
  `static void CL_BuildCGameImport( game_import_t &import )`. Open
  `code/server/sv_game.cpp:898–1049` beside it and mirror all 127
  assignments in the same order, each into one of three buckets
  (plan § A2 has the full classification):
  1. pass-through (`Com_Printf`, `Com_Error`, `Sys_Milliseconds2`,
     `FS_*`, memory, cvars, command args) — assign the same function
     the server assigns;
  2. gamestate-backed — write small statics above the builder:
     `CL_CG_GetConfigstring` copying from `cl.gameState.stringData +
     cl.gameState.stringOffsets[index]` (pattern at `cl_cgame.cpp:297`),
     `CL_CG_SetConfigstring` doing **lookup-only, never allocating**
     (plan § A2 explains why this rule is load-bearing),
     `CL_CG_GetServerinfo` from `CS_SERVERINFO`;
  3. loud stubs for everything server-only — each prints its own name
     once (`static qboolean warned`) and returns 0/does nothing. Write
     a tiny macro to stamp these out; there are dozens.
  Done: compiles. Behaviour is untestable until A3 — land them together.

- [x] **A3 — load hook.** (patch 0010, with A2)
  Needs: A2.
  Do: in `CL_InitCGame` (`cl_cgame.cpp`), when `cgvm.entryPoint` is
  null, load the library and initialise it per the snippet in plan § A3
  (`Sys_LoadSPGameDll`, `CL_BuildCGameImport`, `GetCGameAPI`,
  `CL_InitCGameVM`); add the matching unload in `CL_ShutdownCGame`.
  Add two temporary probes: one in the branch ("dual-load: initialising
  cgame"), one in the host path proving the branch is skipped there.
  Done, in order:
  1. loopback regression passes and does **not** print the dual-load
     probe (host path untouched — verify by probe, not assumption);
  2. two-client test: the second client prints the probe and cgame code
     actually executes. A crash inside cgame code is **success** for
     this task — record the backtrace, it is A5's first work item;
  3. stub names printed by A2's table are captured in the commit
     message or a notes file — each is a discovered work item.

- [x] **A4 — defuse `CL_GetDefaultState`.** (patch 0011)
  Needs: A3 (only so its effect is observable).
  Do: `cl_cgame.cpp:240–258` reads `sv.svEntities[].baseline` — server
  memory — from client code. When no local server runs
  (`!com_sv_running->integer`), return a zeroed `entityState_t`
  instead.
  Done: two-client test behaves no worse than after A3; loopback
  regression passes.

- [x] **A5 — `gent` burn-down.** (one patch per batch: 0012, 0013, …)
  Needs: A3, A4. This is a loop, not a task; run it until milestone
  M4 (plan § Workstream A milestones).
  Each iteration:
  1. run the second client under gdb; take the first crash or the most
     obvious wrong visual;
  2. at that site, guard the server-state read and fall back to
     snapshot data — `cent->currentState.*`, `cg.snap->ps.*`, or a
     configstring. `codemp/cgame/` shows how MP gets the same value
     without a gentity. **Guard-and-fallback, never delete the `gent`
     path** — the host still has real gentities;
  3. rebuild gamecode only (`cmake --build openjk/build --target
     jospgamex86_64`), relaunch, repeat;
  4. commit a batch of related sites once both loopback regression and
     the two-client test pass.
  Milestone gates to record in the commit messages when crossed:
  M1 world renders (probe `re.LoadWorld` fires remotely) → M2 weapon +
  HUD → M3 host player, NPCs, doors render → M4 both players fight the
  same stormtrooper for 10 minutes, no crash.

- [x] **A6 — `entityStateFields` audit.** (patch 0014)
  Needs: nothing (independent of A1–A5, needs only T0); do it whenever
  the axis-gizmo bug (plan § Workstream A, "known adjacent defect")
  gets in the way of A5 testing.
  Do: in `code/qcommon/msg.cpp` (patch 0006 territory), compare every
  `entityStateFields` entry — order and bit width — against
  `codemp/qcommon/msg.cpp` and against `entityState_t` as compiled
  under `JK2_MODE`. The assert `numFields + 1 == sizeof(*from)/4`
  (62 + 1 == 63) must hold — check it in the **Debug** build, where
  asserts exist.
  Done: dying in a two-client session no longer draws the RGB axis
  gizmo; both regressions pass.

---

## Track C — installer / distribution

Background: plan § Workstream C. C1 and C2 need only T0. Keep the two
licensing rules in front of you: never redistribute retail files; never
modify the retail install (add files only).

- [x] **C1 — Linux installer.** (no patch — outer repo only)
  Needs: T0.
  Do: write `tools/install-coop.sh` to the spec in plan § C1: GameData
  autodetection (two standard Steam paths + `libraryfolders.vdf`
  parsing, `--gamedata` override, validate on `base/assets0.pk3`),
  stage `~/.local/share/openjo/base/` with symlinks, renderer link,
  `jk2coop-host`/`jk2coop-join` launchers in `~/.local/bin/` (join's
  `--second` wipes `/tmp/jk2-client2` first), idempotent re-run,
  `--uninstall` removing exactly what it created.
  Done: on this machine, from a clean `~/.local/share/openjo`, the
  script installs; `jk2coop-host` starts a hosting game;
  `jk2coop-join 127.0.0.1:29070 --second` connects; `--uninstall`
  leaves no trace; running it twice in a row changes nothing.

- [x] **C2 — CI builds.** (no patch)
  Needs: T0.
  Do: `.github/workflows/build.yml`, Linux only at first:
  checkout with submodule, apply patches, configure with the JK2SP
  flags from README, build, upload `openjo_sp.x86_64`,
  `jospgamex86_64.so`, `rdjosp-vanilla_x86_64.so` as artifacts.
  Add the windows-latest matrix leg **disabled or allowed-to-fail**
  with a comment pointing at C3 — it cannot link until winsock lands.
  Done: green Actions run on push with downloadable Linux artifacts.

- [x] **C3 — winsock port of the UDP transport.** (patch 0015)
  Needs: T0. Unblocks the Windows leg of C2.
  Do: make `code/qcommon/net_ip.cpp` compile on both platforms:
  `#ifdef _WIN32` halves for headers, `WSAStartup`/`WSACleanup`,
  `ioctlsocket FIONBIO` for non-blocking, `closesocket`,
  `WSAGetLastError` in `NET_ErrorString`. Every equivalent lives in
  `codemp/qcommon/net_ip.cpp` — copy its spellings, keep our file's
  structure and behaviour identical. Regenerate patch 0005 or stack a
  new patch on top, whichever applies cleanly (see plan § 0 for the
  new-file `add -N` trap).
  Done: Linux build unchanged (regression passes); the Windows CI leg
  from C2 compiles and links; two-client test on Linux still works.

- [ ] **C4 — Windows installer.** (no patch)
  Needs: C2 + C3 producing Windows artifacts. Ideally test on a real
  Windows machine; a wine smoke-test is better than nothing.
  Do: `tools/install-coop.ps1` per plan § C3: locate GameData via
  registry (`HKCU:\Software\Valve\Steam` → `SteamPath`) +
  `libraryfolders.vdf`, `-GameData` override; copy `openjo_sp.exe`,
  renderer DLL and gamecode DLL **into** `GameData\base`'s parent
  (additive only); host/join `.cmd` launchers; `-Uninstall` removes
  exactly the copied files.
  Done: on Windows with Steam JK2: install → host a game → second
  machine joins; uninstall leaves the retail dir byte-identical.

- [x] **C5 — macOS installer.** (no patch — outer repo only)
  Needs: T0. Not in the original plan; added alongside C1.
  Do: `tools/install-coop-macos.sh`, the macOS counterpart of C1 with
  the platform differences handled: data dir under
  `~/Library/Application Support/OpenJO`, launchers in `~/bin`, GameData
  autodetected under `~/Library/Application Support/Steam`
  (`libraryfolders.vdf` + `--gamedata` override), engine resolved as
  either an `openjo_sp.app` bundle or a plain `openjo_sp.<arch>` binary,
  gamecode/renderer `.dylib`s named per architecture (`x86_64`/`arm64`,
  `JK2_ARCH` override), same idempotent re-run and non-destructive
  `--uninstall`. Kept portable (no `tac`/`tail -r`).
  Done: shellcheck clean; logic validated on this Linux box against a
  mock macOS build tree (both `.app` and plain-binary forms, autodetect,
  idempotent re-run with no manifest dupes, uninstall that removes only
  what it created and preserves a pre-existing `~/bin` file, retail
  GameData left intact). Real-Mac run is C6.

- [ ] **C6 — macOS real-hardware verification.** (no patch)
  Needs: C5, a Mac (Intel or Apple Silicon) with Steam JK2.
  Do: build the JK2SP targets on the Mac (note whether
  `MakeApplicationBundles` produced an `.app` or a plain binary), run
  `tools/install-coop-macos.sh`, then `jk2coop-host` and
  `jk2coop-join 127.0.0.1 --second`. Exercise `--uninstall` and a re-run.
  File fixes for anything that only reproduces on real hardware
  (Gatekeeper/quarantine on the dylibs is the likely suspect — note
  whether `xattr -d com.apple.quarantine` or codesigning is needed, and
  fold the answer into docs/install-macos.md).
  Done: two-client co-op session on a real Mac; install → play →
  uninstall leaves no trace; install-macos.md updated with any
  hardware-only caveats.

---

## Track D — co-op UX: hosting, discovery, menu

Background: plan § Workstream D — read "What already exists" first; the
server side of discovery is already in the tree. D1–D2 are pure engine
work testable from the console; D3 adds the menu on top. All of D is
independent of Track A (a discovered, menu-joined client that renders
black is still a passing test).

- [x] **D1 — `coop_host` command.** (patch 0016)
  Needs: T0.
  Do, in two pieces (plan § D1):
  1. `code/qcommon/net_ip.cpp`: add `NET_Restart` — `NET_Shutdown`,
     re-read cvars, `NET_OpenIP`. Keep `NET_Init`'s semantics
     unchanged.
  2. Register `coop_host [maxplayers]` (server side —
     `sv_main.cpp`/`sv_init.cpp` is the natural home): set
     `net_enabled 1`, call `NET_Restart`, print the bound address and
     port. Port stays within the existing 29070–29079 scan of
     `NET_OpenIP` — do **not** use an ephemeral port (breaks D2;
     rationale in plan § D1). Store `maxplayers` for E5 to consume
     later; accepting and ignoring it is fine for now.
  Done: start a plain SP game (no flags), type `coop_host` in the
  console, second machine connects to the printed address. Loopback
  regression still opens no socket (the command was not run).

- [x] **D2 — LAN discovery.** (patch 0017)
  Needs: D1.
  Do (plan § D2):
  1. `code/server/sv_main.cpp` `SVC_Info` (line ~247): add `hostname`
     (new `sv_hostname` cvar, defaulting to the player name) and
     `game=jk2coop` to the infostring.
  2. `code/client/cl_main.cpp`: add `CL_LocalServers_f` (command
     `localservers`): clear the list, broadcast `getinfo <challenge>`
     to 255.255.255.255 ports 29070–29079, twice, staggered. Reference:
     `codemp/client/cl_main.cpp` `CL_LocalServers_f`.
  3. Same file, `CL_ConnectionlessPacket` (line ~616): add the
     `infoResponse` branch — verify challenge, `protocol`, and
     `game=jk2coop`; record `{address-from-packet-source, hostname,
     mapname, clients, sv_maxclients}` into a new
     `cls.localServers[16]`, deduped by address. Reference:
     `CL_ServerInfoPacket` in the same codemp file. Print each newly
     discovered server to the console (that print is D2's test surface
     and D3 renders the same array).
  Done: host on machine 1 via `coop_host`; on machine 2 `localservers`
  prints the host with map name and player count within two seconds;
  `connect` to the printed address works. A stock JA server on the LAN
  (if handy) does not appear.

- [x] **D3 — Co-op menu.** (patch 0018 + new committed asset + tool change)
  Needs: D1, D2. Split across three commits if convenient:
  1. **Overlay pk3 plumbing** (outer repo): create `assets/coop-ui/`
     holding original-authorship menu files (write from scratch — plan
     § D1 licensing note; these are ours and **are committed**, unlike
     anything retail). Add `tools/build-coop-ui-pk3.sh` zipping it to
     `zz-coop-ui.pk3`; wire installation into C1's installer if it has
     landed.
  2. **uiScript verbs** (`code/ui/ui_main.cpp`, `UI_RunMenuScript` at
     line ~895): `coopHost` → `Cbuf_AddText("coop_host …")`,
     `coopRefresh` → `localservers`, `coopJoin` → `connect` on the
     selected feeder row, `coopConnect` → `connect` on the
     `ui_coopAddress` cvar (register it archived). Add a feeder ID for
     the server list, backed directly by `cls.localServers` (the SP UI
     is in-engine; follow the `UI_FeederItemText` pattern in the same
     file).
  3. **The menu page**: a Co-op page reachable from the in-game menu:
     Host button (max-players selector 2–4 writing the `coop_host`
     argument), the server-list feeder with a Refresh button, and the
     direct-connect field + button (this is D3 of the plan, folded in
     here).
  Done: full mouse-only session — host loads a map, menu → Co-op →
  Host; joiner (other machine) menu → Co-op → sees host listed →
  clicks it → connected. Direct-connect field also works with a typed
  `ip:port`. Nothing on either command line.

---

## Track E — four players

Background: plan § Workstream E. **Gate: Track A at milestone M4** (two
players fully playable). E1+E2+E3 shipped together as patch **0020**
(the plan's "0019 for E1+E2" slot was taken by D3's co-op menu).

- [x] **E1 — `sv_maxclients` cvar.** (patch 0020)
  Registered `sv_maxclients` (latched, default 2, clamped 1–
  `MAX_CLIENTS`) in `sv_init.cpp`; `SV_DirectConnect`'s free-slot loop
  is now bounded by the cvar (`maxConnect`) while allocations stay
  `MAX_CLIENTS`-sized. `SVC_Info` reports the cvar. `SV_CoopHost_f`
  sets it from its arg (re-`Cvar_Get` to apply the latch). Verified
  headless: `sv_maxclients 1` refuses a joiner with "Server is full"
  (connectResponse 0); `2` accepts one (2 `ClientEnterWorld`,
  connectResponse 1); loopback boots clean.

- [x] **E2 — raise the cap.** (patch 0020)
  `#define MAX_CLIENTS 4` (`q_shared.h:618`) and `PROTOCOL_VERSION 41`
  (`qcommon.h:206`) in the same patch — the `CS_LIGHT_STYLES =
  CS_PLAYERS + MAX_CLIENTS` renumber shifts every later configstring,
  so a stale 2-client build is now rejected at connect on the protocol
  bump instead of silently desyncing. `svs.numSnapshotEntities`
  (`MAX_CLIENTS * 4 * 64`) auto-scales — no manual change. Builds
  clean; loopback + two-client tests unchanged.

- [x] **E3 — four-player verification.** (patch 0020)
  Host + 3 dual-load joiners on `kejim_post` under one Xvfb, each with
  its own wiped `fs_homepath` + gamecode symlink and a distinct name.
  **All four entered the world** (0 "Server is full", 0 snapshot-ring
  warnings, 0 crashes) and **all three clients dual-loaded and rendered
  real 3D frames** (ImageMagick mean ≈0.15, ~10k colours — far above
  the black-screen floor). Patch 0008's spawn ring keeps them clear.
  **Root cause found + fixed en route:** multiple same-IP loopback
  clients all seeded `net_qport` from `Com_Milliseconds()`, which is
  ≈0 this early in startup, so their qports collided and
  `SV_DirectConnect` reconnected joiner 2/3 into joiner 1's slot —
  only one ever entered. Fix (both in 0020): seed `net_qport` with the
  process id (new `Sys_GetProcessId()`, unix + win32) so same-host
  clients get distinct qports, and match the reuse loop on qport alone
  (drop the loopback-hostile `|| from.port == remoteAddress.port`
  clause, which `SV_PacketEvent` never needed either). This is the
  project's headline milestone.

- [x] **E4 — four-player LIVE session + slot-lifecycle fixes.** (patch 0021)
  Human-verified 2026-07-11: the developer hosted `kejim_post` in a real
  window on the desktop (campaign intro cinematic, objectives, and
  scripted NPCs all run for the host player) while three driven bot
  clients joined from a hidden Xvfb — four players in one live game,
  confirmed working by the player. Two bugs surfaced and fixed en route:
  1. **`SV_CheckTimeouts` only ever examined `svs.clients[0]`** (stock
     SP assumed one client), so a disconnected joiner's slot stayed
     `CS_ZOMBIE` forever — the server read "full" minutes after a
     leaver, and unresponsive joiners were never timed out. Fixed to
     loop every slot, matching codemp. Verified headless (fill 4/4 →
     one leaves → new joiner takes the freed slot) and live (the bot
     crew was swapped mid-session; all three replacement joiners
     entered the vacated slots).
  2. **A connect rejection left the joiner on a silent loading screen**:
     the client printed the server's OOB `print` ("Server is full.") to
     the console but kept resending connect requests forever. Now a
     `print` from the dialled server while still connecting is treated
     as a rejection — the client stops retrying and drops to the menu
     showing the server's message.
  Known limitation (next co-op tier, not a bug): campaign UI — cutscenes,
  objectives, mission text — renders only for the host player; joiners
  see world + entities. Playing the campaign co-op means the human hosts.
  That limitation is Track F.

---

## Track F — campaign UI for joiners

Background: [campaign-ui-plan.md](campaign-ui-plan.md) — read it first;
it holds the problem statement (SP gamecode calls the cgame in-process,
"jump the network" is a literal Raven comment), the design principles
(host player is canon; configstrings for state, server commands for
events; joiners spectate cutscenes), and the code pointers. One patch per
task. Every task keeps the solo loopback regression green.

- [ ] **F1 — objective sync.**
  Needs: nothing.
  Do: configstring range for `mission_objectives` (packed
  text/status/display per slot), written when the ICARUS objective verbs
  fire; joiner cgame mirrors it and the datapad reader
  (`cg_info.cpp:224`) uses the mirror when `cg_remoteClient`, the direct
  `gent` read otherwise. Bump `PROTOCOL_VERSION`.
  Done: joiner's datapad matches the host's, including one objective
  completed mid-session; late joiner gets the current set; solo datapad
  unchanged.

- [ ] **F2 — mission text + centerprint sync.**
  Needs: F1.
  Do: replace the gamecode's direct `CG_CenterPrint` calls
  (`g_target.cpp:1056` et al) with a helper that prints on the host and
  broadcasts `cp "<key>"` to all clients; handle `cp` in the joiner
  cgame's server-command dispatcher.
  Done: host hits kejim_post's checkpoint trigger → every joiner shows
  the same centerprint; solo unchanged.

- [ ] **F3 — cutscene handling (spectate/freeze MVP).**
  Needs: F2.
  Do: broadcast `cutscene 1|0` when the host enters/leaves an ICARUS
  camera (`g_camera.cpp`); joiners letterbox + suppress their input for
  the duration. Briefing videos stay host-only (joiners get a "briefing
  in progress" card).
  Done: kejim_post intro cutscene freezes + letterboxes joiners, releases
  cleanly; no joiner can roam mid-cutscene.

- [ ] **F4 — level transition follow.**
  Needs: F1 landed (investigate any time).
  Do: establish what happens to joiners when the host completes a level
  (`target_level_change` → `SV_SpawnServer`); make the standard
  new-gamestate → reload → re-enter flow work, and re-run the patch-0008
  spawn ring on the new map.
  Done: host finishes kejim_post; every joiner loads the next map and
  spawns clear without manual reconnect.

- [ ] **F5 — verification: harness + live session.**
  Needs: F1–F4.
  Do: extend the headless harness — scripted host walks kejim_post's
  opening to the checkpoint + first objective; assert the joiner log
  shows the `cp` broadcast, the objective configstring, and the F4
  map-change reload. Then a live session: human hosts a full level with
  3 bots.
  Done: harness green; a joiner window shows objectives + mission text
  through a full level, live.

---

## Suggested schedule

| Order | Task | Why then |
|---|---|---|
| 1 | T0.1 | Everything else assumes it |
| 2 | C1 | Immediately useful, zero engine risk, exercises the docs |
| 3 | A1 → A2+A3 → A4 | The critical path; start it as soon as T0 works |
| 4 | A5 loop (with A6 when the gizmo annoys) | Bulk of the work; every iteration ships visible progress |
| 5 | D1 → D2 in gaps | Console-testable without Track A; good context-switch work |
| 6 | C2, C3 in gaps | CI pays off earliest; winsock is self-contained |
| 7 | D3 | Best after A-M2, when a joiner can see what they joined |
| 8 | E1 → E2 → E3 | Gated on A-M4 by design |
| 9 | C4 | Last: needs C2+C3 artifacts and a Windows test machine |

Everything above is done except C4. What remains, in suggested order:

| Order | Task | Why then |
|---|---|---|
| 1 | F1 → F2 | Establishes the state/event routing pattern; F2 is small once F1 exists |
| 2 | F4 investigation | Biggest unknown in Track F — scout it before committing to F3's shape |
| 3 | F3 | Depends on F2's event channel |
| 4 | F4 fix → F5 | Transition fix lands on the routing pattern; F5 seals the track |
| 5 | C4, C6 | Hardware-gated (Windows box, real Mac) — do whenever the hardware appears |
