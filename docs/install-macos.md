# Installing on macOS

This guide gets a cooperative Jedi Outcast game running on macOS
(Intel `x86_64` or Apple Silicon `arm64`).

## Before you start: you need a legal copy of the game

**This project does not include Jedi Outcast's game data, and never will.**
The maps, models, textures, sounds, and music live in the retail
`assets*.pk3` files, which are proprietary and owned by their rights holders.
You must own a legal copy of *Star Wars Jedi Knight II: Jedi Outcast* — for
example the [Steam release](https://store.steampowered.com/app/6030/) — so
that those files already exist on your machine.

**What this project ships**, and all it ships, is:

- the *source changes* to the [OpenJK](https://github.com/JACoders/OpenJK)
  engine that add cooperative play (the diffs in `patches/`), which build into
  three binaries — the engine, the OpenGL renderer, and the singleplayer
  gamecode;
- a small original UI overlay for the in-game Co-op menu (`assets/coop-ui/`,
  packed into `zz-coop-ui.pk3` at build time); and
- the installer and helper scripts in `tools/`.

The installer only ever **symlinks** your existing retail files into the
place the engine looks for them. It copies nothing from your game install and
modifies nothing there.

## 1. Build the binaries

You need the Xcode command-line tools and the OpenJK build dependencies
(`cmake`, `SDL2`, `OpenAL`, `zlib`, `libpng`, `libjpeg` — for example via
Homebrew). Build the JK2 singleplayer engine, renderer, and gamecode as on any
platform; see the [OpenJK macOS build
notes](https://github.com/JACoders/OpenJK) for toolchain specifics.

Apply the co-op patches first:

```sh
tools/apply-patches.sh
```

then configure and build with the same `-DBuildJK2SP*` options shown in the
[Linux guide](install-linux.md#1-build-the-binaries). Depending on the CMake
`MakeApplicationBundles` option, the engine builds either as an
`openjo_sp.app` bundle or a plain `openjo_sp.<arch>` binary — the installer
handles both. The renderer and gamecode are `.dylib`s whose names carry the
architecture, e.g. `jospgamearm64.dylib` and `rdjosp-vanilla_arm64.dylib`.

## 2. Install (recommended: one command)

```sh
tools/install-coop-macos.sh                  # autodetect Steam GameData
tools/install-coop-macos.sh --gamedata "/path/to/Jedi Outcast/GameData"
```

The macOS installer works like the Linux one, with the platform differences
handled for you:

- the engine data directory is `~/Library/Application Support/OpenJO`, and the
  launchers go in `~/bin`;
- your retail **GameData** is found under
  `~/Library/Application Support/Steam` (parsing `libraryfolders.vdf`); pass
  `--gamedata` if it lives elsewhere;
- the engine is used whether it built as an `openjo_sp.app` bundle or a plain
  `openjo_sp.<arch>` binary — both are autodetected;
- the build architecture defaults to your machine (Apple Silicon → `arm64`);
  override it with the `JK2_ARCH` environment variable if you cross-built.

It stages the data directory with **symlinks** to your retail `assets*.pk3`,
the built co-op gamecode, and the Co-op UI overlay. It is idempotent, and
`--uninstall` removes exactly what it created. **Retail files are never
touched.**

If `~/bin` is not on your `PATH`, either add it or call the launchers by their
full path.

> **Note:** the macOS installer's logic has been validated against a mock
> build tree, but has not yet been exercised end-to-end on a real Mac. If you
> hit a snag, please open an issue.

## 3. Play

```sh
jk2coop-host                       # host a game on UDP 29070 (machine 1)
jk2coop-join <host-ip>             # join it (machine 2)
```

To run a second client on the same machine for testing, use `--second`
(same behaviour as on Linux):

```sh
jk2coop-host                       # terminal 1
jk2coop-join 127.0.0.1 --second    # terminal 2 (same box)
```

You can also host and discover games from the in-game console — see
[Hosting and finding games from the console](../README.md#hosting-and-finding-games-from-the-console)
in the README.
