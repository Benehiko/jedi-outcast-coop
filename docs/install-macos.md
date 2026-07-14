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

## Fastest path: `jk2coop setup`

`./jk2coop setup` extracts the engine source **embedded in the binary**, applies
the co-op patches (in pure Go — no `git` needed), builds the engine, and installs
in one guided step. A pre-built `jk2coop` needs neither a clone nor the OpenJK
submodule; if you build `jk2coop` yourself, run `make build` first. If the build
tools are missing it prints the `xcode-select --install` + `brew install …`
commands to run, then re-run `setup`. (The VM build option is Linux-only; on
macOS `setup` builds on the host.)

The manual steps 1–2 below build from the OpenJK **submodule** instead — the
reference for patch development. A normal install does not need them; to make
`setup` use the submodule, pass `--repo .` from inside a checkout. See
[embedded-source.md](embedded-source.md).

## 1. Build the binaries

You need the Xcode command-line tools and the OpenJK build dependencies
(`cmake`, `SDL2`, `OpenAL`, `zlib`, `libpng`, `libjpeg` — for example via
Homebrew). `jk2coop setup` checks for these and prints the exact install
commands if any are missing. Build the JK2 singleplayer engine, renderer, and gamecode as on any
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

The cross-platform `jk2coop` Go binary is the recommended installer (see
[tooling.md](tooling.md)):

```sh
jk2coop install                              # autodetect Steam GameData
jk2coop install --gamedata "/path/to/Jedi Outcast/GameData"
jk2coop install -y                           # assume yes to prompts (non-interactive)
```

`jk2coop install` builds the engine, symlinks your retail assets and the
co-op gamecode into place, applies your config (autoexec cvars, the
patch-backed graphics features, and any optional texture paks), and
installs the launchers. Run `jk2coop uninstall` to remove everything it
installed.

The equivalent shell installer remains and works unchanged:

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
`jk2coop uninstall` (or the shell installer's `--uninstall`) removes exactly
what it created. **Retail files are never touched.**

If `~/bin` is not on your `PATH`, either add it or call the launchers by their
full path.

### Settings (config file)

Gameplay and graphics preferences live in a single config file at
`~/Library/Application Support/jk2coop/config.toml`. Edit it with the two
settings TUIs — or by hand — and it is applied on the next `install` or
`launch`:

```sh
jk2coop game        # mouse sensitivity, blaster speed, aim assist, dynamic crosshair, skip cutscenes
jk2coop graphics    # widescreen, lighting, MSAA, texture upscale/generate (alias: gfx)
```

`[game]` settings are runtime cvars. Under `[graphics]`, `widescreen` and
`lighting` are patch-backed and require an engine rebuild to change (which
`jk2coop install` handles); `msaa` and the texture paks are not. See the
[Linux guide](install-linux.md#settings-config-file) for the full TOML
schema.

### Optional mods

Three optional game-file mods each add a `zz…` override pak to `base/` (retail
data is never modified, and uninstalling removes them too). How you enable them
depends on which installer you use.

| Mod | Config key (`[graphics]`) | Availability |
|---|---|---|
| Widescreen menu | `widescreen` | Works on macOS (needs `python3` — Xcode CLT or `brew install python`) — see [widescreen.md](widescreen.md) |
| Generated textures | `texture_generate` | Linux GPU-only — the installer prints the command to run on a Linux machine |
| Upscaled textures | `texture_upscale` | Linux GPU-only — prints the command |

**With `jk2coop` (recommended)** the mods are config-driven — no per-mod install
flags. Toggle the key in the `jk2coop graphics` TUI (or edit
`~/Library/Application Support/jk2coop/config.toml`), then run `jk2coop install`,
which builds any newly-enabled pak and removes any the config no longer wants:

```sh
jk2coop graphics    # toggle "Widescreen" / "Texture upscale" / "Texture generate"
jk2coop install     # builds/removes the override paks to match the config
```

**With the shell installer** the same mods are per-mod flags. On an interactive
terminal it prompts **y/N** for each; run non-interactively it enables none
unless you pass the matching flag:

```sh
tools/install-coop-macos.sh                    # prompts y/N per optional mod
tools/install-coop-macos.sh --all              # enable everything offered
tools/install-coop-macos.sh --with-widescreen  # only the widescreen menu
tools/install-coop-macos.sh --no-optional      # core install only
```

The AI-texture mods need an AMD ROCm GPU container (Linux-only); on macOS they
are offered but resolve to a printed command rather than run.

### Combat and render presets

With `jk2coop`, combat feel and render fidelity are set through the config
file (`jk2coop game` and `jk2coop graphics`) rather than install flags — see
[Settings](#settings-config-file) above. Both default on.

The shell installer still writes two cvar-only presets to `base/` and takes
flags for them (they default on; `--uninstall` removes them):

- `--combat modern|classic` (default `modern`) — see
  [modern-combat.md](modern-combat.md).
- `--render high|classic` (default `high`) — sharper textures, anisotropic
  filtering, and the software-overbright lighting fix (matters in
  windowed/borderless); see [render-fidelity.md](render-fidelity.md).

> **Note:** the macOS installer's logic has been validated against a mock
> build tree, but has not yet been exercised end-to-end on a real Mac. If you
> hit a snag, please open an issue.

## 3. Play

With the `jk2coop` binary:

```sh
jk2coop launch                     # play; hosts a co-op game on UDP 29070 by default
jk2coop host                       # explicitly host a co-op game (machine 1)
jk2coop join <host-ip>             # join it (machine 2)
jk2coop launch --solo              # single-player
```

Or with the launcher scripts the shell installer writes:

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
the co-op guide: [coop-guide.md](coop-guide.md).
