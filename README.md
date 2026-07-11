# Jedi Outcast Rebuild

Native Linux build of Jedi Outcast singleplayer, targeting cooperative
campaign play (campaign maps, no cutscenes).

## Status

Engine, renderer, and singleplayer gamecode build and run natively on
Linux x86_64 against the retail Steam assets.

Toward cooperative play: the singleplayer engine accepts **up to four
clients** (`sv_maxclients`, default 2) over a restored UDP transport with
real entity serialisation. Joiners connect (LAN included), spawn clear of the
host, and **render their own view** — the dual-loaded cgame draws the world,
the HUD, and the host player and NPCs, verified crash-free over a 10-minute
soak. A four-player session (host + three joiners) has been verified headless:
all four enter the world and every client renders. Hosting, LAN discovery, and
an in-game Co-op menu are in; a one-command Linux installer stages it all.
Still open: verifying active multi-player combat in a live windowed session and
the Windows build/installer. The plan is in
[docs/implementation-plan.md](docs/implementation-plan.md); the roadmap is
[docs/roadmap.md](docs/roadmap.md); current task status is in
[docs/tasks.md](docs/tasks.md).

## Documentation

| Document | What it is |
|---|---|
| [install-linux.md](docs/install-linux.md) / [install-macos.md](docs/install-macos.md) / [install-windows.md](docs/install-windows.md) | **Playing? Start here.** Per-OS install guides |
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
Jedi Outcast source under the GPLv2 in 2013; [OpenJK](https://github.com/JACoders/OpenJK)
maintains it. Only the retail assets (`assets*.pk3`) are proprietary,
and they are used in place, unmodified — this project never ships them (see
[License](#license)).

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

## Installing

### You need a legal copy of the game

**This project does not include Jedi Outcast's game data, and never will.**
The maps, models, textures, sounds, and music are in the retail
`assets*.pk3` files, which are proprietary and owned by their rights holders.
You must own a legal copy of *Star Wars Jedi Knight II: Jedi Outcast* — for
example the [Steam release](https://store.steampowered.com/app/6030/) — so
that those files already exist on your machine.

**What this project ships**, and all it ships, is:

- the *source changes* to the OpenJK engine that add cooperative play (the
  diffs in `patches/`), which build into three binaries — the engine, the
  OpenGL renderer, and the singleplayer gamecode;
- a small **original** UI overlay for the in-game Co-op menu
  (`assets/coop-ui/`, packed into `zz-coop-ui.pk3` at build time); and
- the installer and helper scripts in `tools/`.

The Linux/macOS installers only ever **symlink** your existing retail files
into a separate engine data directory — they copy nothing out of your game
install and touch nothing in it. The (manual, for now) Windows path is
**additive only**: it places the built binaries and the co-op overlay
alongside the retail files without overwriting or modifying any of them.

### Per-OS install guides

| OS | Guide | Status |
|---|---|---|
| Linux | [docs/install-linux.md](docs/install-linux.md) | one-command installer |
| macOS | [docs/install-macos.md](docs/install-macos.md) | one-command installer (validated off-Mac; not yet run on real hardware) |
| Windows | [docs/install-windows.md](docs/install-windows.md) | experimental — engine builds, installer not written yet |

The short version on Linux, once you have [built the binaries](#building):

    tools/install-coop.sh              # symlinks your Steam assets + co-op gamecode into place
    jk2coop-host                       # host a game on UDP 29070
    jk2coop-join <host-ip>             # join it from another machine

`jk2coop-join`'s `--second` flag runs a second client on the **same machine**
for testing (its own clean `fs_homepath` + gamecode copy). See the per-OS
guides for details, `--gamedata`/`--uninstall`, and the macOS/Windows paths.

### Hosting and finding games from the console

You can also host from a game that is already running, with no launch flags:

    coop_host [maxplayers]      # open the network socket for the current game
                               # maxplayers 1-4 (default 2); sets sv_maxclients
                               # prints the port other machines should join

And a joiner can discover co-op hosts on the local network instead of typing an
IP:

    localservers                # broadcasts on the LAN; prints each co-op host
                               # found, with its name, map, and player count

The in-game Co-op menu (`uimenu coopMenu`, shipped in `zz-coop-ui.pk3`) drives both from buttons: Host, a LAN server list with Refresh/Join, and a direct-connect field. `sv_hostname` sets the name shown in the list.

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

## License

The engine is [OpenJK](https://github.com/JACoders/OpenJK), which Raven
Software released and OpenJK maintains under the **GNU General Public License,
version 2**. This project's changes to it (`patches/`) are derivative works of
that code and are therefore also licensed under **GPLv2**; so are the built
binaries. The full license text is in [LICENSE](LICENSE). The original
authorship in this repository — patches, tools, docs, and the Co-op UI overlay
— is offered under the same terms.

This project ships **no game data**. The retail `assets*.pk3` files are
proprietary and are used in place from your own legal copy of the game; see
[Installing](#installing).

### Trademarks

*Star Wars*, *Jedi Knight*, *Jedi Outcast*, and related names and marks are
trademarks of their respective owners (Lucasfilm / Disney, and the game's
publishers). This is an unofficial, non-commercial, fan-made project. It is
not affiliated with, endorsed by, or sponsored by any of those rights holders.
The GPL covers the source code only — it grants no rights in these trademarks
or in the proprietary game assets.
