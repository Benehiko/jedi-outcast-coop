# Jedi Outcast Rebuild

Native Linux build of Jedi Outcast singleplayer, targeting cooperative
campaign play (campaign maps, no cutscenes).

## Status

Engine, renderer, and singleplayer gamecode build and run natively on
Linux x86_64 against the retail Steam assets.

Toward cooperative play: the singleplayer engine accepts two clients over
a restored UDP transport with real entity serialisation. A second client
connects (LAN included), spawns clear of the host, moves, and replicates
to the host's screen. It does not yet render its own view — its cgame
never initialises without a local server. The fix is planned in
[docs/implementation-plan.md](docs/implementation-plan.md); the roadmap is
[docs/roadmap.md](docs/roadmap.md).

## Documentation

| Document | What it is |
|---|---|
| [roadmap.md](docs/roadmap.md) | **Start here.** What happens next, in phases, with tasks |
| [tasks.md](docs/tasks.md) | **Implementing? Start here.** The plan as ordered, sitting-sized tasks with done-checks |
| [implementation-plan.md](docs/implementation-plan.md) | Handoff-ready plan: remote-client rendering (dual-load), co-op UX, four players, Linux/Windows installer |
| [cgame-split-investigation.md](docs/cgame-split-investigation.md) | Why the cgame library split was rejected — measurements and the replacement design |
| [widen-sp-progress.md](docs/widen-sp-progress.md) | What has been done on the current route |
| [route-comparison.md](docs/route-comparison.md) | Why this route was chosen over hosting on the multiplayer tree |
| [investigation-log.md](docs/investigation-log.md) | Everything tried, measured, and concluded — including the wrong turns |
| [coop-design.md](docs/coop-design.md) | Superseded. The original study; its findings hold, its conclusion does not |

## Approach

The game engine is not reverse engineered. Raven Software released the
Jedi Outcast source under the GPL in 2013; [OpenJK](https://github.com/JACoders/OpenJK)
maintains it. Only the retail assets (`assets*.pk3`) are proprietary,
and they are used in place, unmodified.

Changes to OpenJK live in `patches/` and are applied to a pinned submodule
by `tools/apply-patches.sh`, rather than carrying a fork.

## Layout

- `openjk/` — OpenJK source checkout (upstream)
- `openjk/build/` — build output (gitignored)

## Building

Requires: cmake, ninja, gcc, SDL2, OpenAL, zlib, libpng, libjpeg.

    cmake -S openjk -B openjk/build -G Ninja \
      -DCMAKE_BUILD_TYPE=RelWithDebInfo \
      -DBuildJK2SPEngine=ON -DBuildJK2SPGame=ON -DBuildJK2SPRdVanilla=ON \
      -DBuildSPEngine=OFF -DBuildSPGame=OFF -DBuildSPRdVanilla=OFF \
      -DBuildMPEngine=OFF -DBuildMPRdVanilla=OFF -DBuildMPDed=OFF \
      -DBuildMPGame=OFF -DBuildMPCGame=OFF -DBuildMPUI=OFF -DBuildMPRend2=OFF
    cmake --build openjk/build

Produces three artifacts:

| Artifact | Role |
|---|---|
| `openjo_sp.x86_64` | Engine |
| `code/rd-vanilla/rdjosp-vanilla_x86_64.so` | OpenGL renderer module |
| `codeJK2/game/jospgamex86_64.so` | Singleplayer gamecode |

## Running

The engine reads assets and modules from `~/.local/share/openjo/base/`
(note: `openjo`, not `openjk` — this is the Jedi Outcast target).
Symlink the retail assets and the freshly built gamecode into place:

    mkdir -p ~/.local/share/openjo/base
    ln -sfn "<steam>/Jedi Outcast/GameData/base/"assets*.pk3 ~/.local/share/openjo/base/
    ln -sfn "$PWD/openjk/build/codeJK2/game/jospgamex86_64.so" ~/.local/share/openjo/base/

The renderer module is loaded relative to the executable:

    ln -sfn "$PWD/openjk/build/code/rd-vanilla/rdjosp-vanilla_x86_64.so" openjk/build/

Then:

    cd openjk/build && ./openjo_sp.x86_64 +map kejim_post

### One-command co-op install (Linux)

`tools/install-coop.sh` does the staging above and adds two launcher
commands. It only creates symlinks into your existing Steam install and
small launcher scripts — it never copies or modifies retail files.

    tools/install-coop.sh                        # autodetect Steam GameData
    tools/install-coop.sh --gamedata /path/to/"Jedi Outcast"/GameData

GameData is found under the standard Steam libraries (parsing
`libraryfolders.vdf`); pass `--gamedata` if your install lives elsewhere
(e.g. a NAS mount). The installer is idempotent, and `--uninstall` removes
exactly what it created (tracked in a manifest) — retail files are never
touched.

It installs two launchers into `~/.local/bin`:

    jk2coop-host [map]                 # host a co-op game on UDP 29070
    jk2coop-join <host[:port]> [--second]

`jk2coop-join`'s `--second` flag is for running a second client on the
**same machine**: it gives that client its own clean `fs_homepath`
(`/tmp/jk2-client2`, wiped first) with its own copy of the gamecode, since
the game library is loaded from the home path.

    jk2coop-host                       # machine/terminal 1
    jk2coop-join 127.0.0.1 --second    # machine/terminal 2 (same box)

## Cooperative campaign

**Route reversed.** See [docs/route-comparison.md](docs/route-comparison.md).
Hosting the campaign on the Jedi Academy multiplayer tree works well
enough to play — two clients connect, spawn, see each other, and fight —
but that engine's animation enum has 1,534 entries against Jedi Outcast's
1,202, diverging from index 1. Jedi Outcast's models render collapsed and
its NPCs never complete their attack animations. The current plan is to
widen the singleplayer engine instead, whose client cap is eight sites.

[docs/investigation-log.md](docs/investigation-log.md) records everything
tried, measured, and concluded, including the wrong turns.

The multiplayer branch below still runs, and produced two upstream bug
fixes worth keeping.

Build the multiplayer targets by inverting the `BuildMP*` / `BuildJK2SP*`
flags above. Note the two trees use *different* data directories:

| Tree | Data directory | Gamecode module |
|---|---|---|
| Jedi Outcast singleplayer | `~/.local/share/openjo/base/` | `jospgamex86_64.so` |
| Multiplayer | `~/.local/share/openjk/base/` | `jampgamex86_64.so`, `cgamex86_64.so`, `uix86_64.so` |

Jedi Academy's multiplayer code hardcodes Jedi Academy's asset paths.
Jedi Outcast ships the same data under different names, so three of the
four fixes are configuration and one is a patch.

Apply the source patches to the pinned submodule, then rebuild:

    tools/apply-patches.sh
    cmake --build openjk/build

Generate the NPC compatibility archive from your own retail installation:

    tools/build-coop-npcs-pk3.sh "<steam>/Jedi Outcast/GameData/base"
    cp zzz-coop-npcs.pk3 ~/.local/share/openjk/base/

Start a dedicated server on a campaign map:

    cd openjk/build
    ./openjkded.x86_64 +set dedicated 1 +set sv_pure 0 \
        +set net_port 29070 +set sv_maxclients 8 +map kejim_post

Connect a client. The two cvars point the menu and HUD loaders at Jedi
Outcast's files rather than Jedi Academy's:

    ./openjk.x86_64 +set sv_pure 0 \
        +set ui_menuFilesMP "ui/jk2mpmenus.txt" \
        +set cg_hudFiles "ui/jk2hud.txt" \
        +set name Kyle +connect 127.0.0.1:29070

Run a second client with a different `fs_homepath` to play locally:

    ./openjk.x86_64 +set fs_homepath /tmp/jk2-client2 ... +set name Jan ...

Clients connect as **spectators** — floating, no clipping, unable to
shoot. `ClientBegin` fires for spectators, so a connected client in the
server log has not necessarily joined the game. Put them on the free team
from the server console:

    forceteam 0 free
    forceteam 1 free

Campaign NPCs skip spectators (`NPC_ValidEnemy`), so until a client joins
a team the stormtroopers will correctly ignore it.

### Asset path differences

| Jedi Academy expects | Jedi Outcast ships | Resolved by |
|---|---|---|
| `ext_data/Siege/Classes/*.scl` | nothing (Siege is Academy-only) | patch: non-fatal |
| `ui/jampmenus.txt` | `ui/jk2mpmenus.txt` | cvar `ui_menuFilesMP` |
| `ui/jahud.txt` | `ui/jk2hud.txt` | cvar `cg_hudFiles` |
| `ui/jamp/menudef.h` | `ui/jk2mp/menudef.h` | patch: probe and fall back |
| `ext_data/NPCs/*.npc` | `ext_data/npcs.cfg` | `build-coop-npcs-pk3.sh` |

The `menudef.h` case is not cosmetic. Without those defines every
symbolic constant in Jedi Outcast's `.menu` files fails to parse, and the
client is dropped rather than merely rendering an ugly menu.

## Debug builds

`RelWithDebInfo` and `Release` both define `NDEBUG`, which compiles out
`assert()`. The singleplayer save code carries assertions that Raven left
as deliberate tripwires for exactly the change this project is making, so
test anything touching saves against a `Debug` tree:

    cmake -S openjk -B openjk/build-debug -G Ninja -DCMAKE_BUILD_TYPE=Debug \
      -DBuildJK2SPEngine=ON -DBuildJK2SPGame=ON -DBuildJK2SPRdVanilla=ON
    cmake --build openjk/build-debug

Verify the assertions are live before trusting a passing test:

    nm -u openjk/build-debug/codeJK2/game/jospgamex86_64.so | grep assert

## Development loop

Gameplay code lives in `openjk/codeJK2/game/` and builds as a standalone
shared library. Because the gamecode is symlinked into the engine's
search path, rebuilding that one target is sufficient — no reinstall:

    cmake --build openjk/build --target jospgamex86_64

Relaunch to pick up the change.
