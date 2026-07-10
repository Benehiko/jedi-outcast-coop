# Jedi Outcast Rebuild

Native Linux build of Jedi Outcast singleplayer, targeting cooperative
campaign play (campaign maps, no cutscenes).

## Status

Engine, renderer, and singleplayer gamecode build and run natively on
Linux x86_64 against the retail Steam assets.

## Approach

The game engine is not reverse engineered. Raven Software released the
Jedi Outcast source under the GPL in 2013; [OpenJK](https://github.com/JACoders/OpenJK)
maintains it. Only the retail assets (`assets*.pk3`) are proprietary,
and they are used in place, unmodified.

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

## Cooperative campaign

See [docs/coop-design.md](docs/coop-design.md) for the feasibility study.
The campaign is hosted on the multiplayer tree, which retains ICARUS,
the singleplayer AI roster, Ghoul2, and the saber system.

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

## Development loop

Gameplay code lives in `openjk/codeJK2/game/` and builds as a standalone
shared library. Because the gamecode is symlinked into the engine's
search path, rebuilding that one target is sufficient — no reinstall:

    cmake --build openjk/build --target jospgamex86_64

Relaunch to pick up the change.
