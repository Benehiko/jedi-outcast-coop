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

## Development loop

Gameplay code lives in `openjk/codeJK2/game/` and builds as a standalone
shared library. Because the gamecode is symlinked into the engine's
search path, rebuilding that one target is sufficient — no reinstall:

    cmake --build openjk/build --target jospgamex86_64

Relaunch to pick up the change.
