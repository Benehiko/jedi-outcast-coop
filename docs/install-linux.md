# Installing on Linux

This guide gets a cooperative Jedi Outcast game running on Linux x86_64.

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

If you just want to play, run the one guided command and skip the manual build
steps below:

```sh
git clone --recurse-submodules <repo>
cd jedi-outcast-coop
make build            # produces ./jk2coop
./jk2coop setup       # submodule + patches + build + install, guided
```

`setup` initialises the OpenJK submodule, applies the co-op patches, builds the
engine, and installs. If the build tools (`cmake`, `ninja`, a C compiler) are
missing it prints the exact install command for your distro (apt / pacman / dnf)
and stops so you can install them and re-run. If you have
[`vee`](https://github.com/Benehiko/vee), `setup` can instead build the engine
inside a clean throwaway VM, so you never install a compiler on the host — it
prompts, or force the choice with `--vm` / `--host`. After a VM build it offers
to delete the VM (kept by default for fast rebuilds).

The manual steps 1–2 below are what `setup` automates; use them if you want to
drive each stage yourself.

## 1. Build the binaries

Requires: `cmake`, `ninja`, `gcc`, `SDL2`, `OpenAL`, `zlib`, `libpng`,
`libjpeg`. (`jk2coop setup` checks for these and prints the install command for
your distro if any are missing — you do not need to install them by hand first.)

```sh
# from the repository root
tools/apply-patches.sh          # apply the co-op patches to the pinned submodule

cmake -S openjk -B openjk/build -G Ninja \
  -DCMAKE_BUILD_TYPE=RelWithDebInfo \
  -DBuildJK2SPEngine=ON -DBuildJK2SPGame=ON -DBuildJK2SPRdVanilla=ON \
  -DBuildSPEngine=OFF -DBuildSPGame=OFF -DBuildSPRdVanilla=OFF \
  -DBuildMPEngine=OFF -DBuildMPRdVanilla=OFF -DBuildMPDed=OFF \
  -DBuildMPGame=OFF -DBuildMPCGame=OFF -DBuildMPUI=OFF -DBuildMPRend2=OFF
cmake --build openjk/build
```

This produces:

| Artifact | Role |
|---|---|
| `openjk/build/openjo_sp.x86_64` | Engine |
| `openjk/build/code/rd-vanilla/rdjosp-vanilla_x86_64.so` | OpenGL renderer module |
| `openjk/build/codeJK2/game/jospgamex86_64.so` | Singleplayer gamecode |

## 2. Install (recommended: one command)

The cross-platform `jk2coop` Go binary is the recommended installer (see
[tooling.md](tooling.md)):

```sh
jk2coop install                              # autodetect Steam GameData
jk2coop install --gamedata /path/to/"Jedi Outcast"/GameData
jk2coop install -y                           # assume yes to prompts (non-interactive)
```

`jk2coop install` builds the engine, symlinks your retail assets and the
co-op gamecode into place, applies your config (autoexec cvars, the
patch-backed graphics features — rebuilding the engine if they changed —
and any optional texture paks), and installs the launchers. To remove
everything it installed, run `jk2coop uninstall`.

The equivalent shell installer remains and works unchanged:

```sh
tools/install-coop.sh                        # autodetect Steam GameData
tools/install-coop.sh --gamedata /path/to/"Jedi Outcast"/GameData
```

The installer:

- finds your retail **GameData** under the standard Steam libraries (parsing
  `libraryfolders.vdf`); pass `--gamedata` if your install lives elsewhere
  (e.g. a NAS mount). It is validated by the presence of `base/assets0.pk3`;
- stages `~/.local/share/openjo/base/` with **symlinks** to your retail
  `assets*.pk3`, the built co-op gamecode, and the Co-op UI overlay;
- installs two launchers into `~/.local/bin/`;
- offers the **optional mods** below.

It is idempotent (safe to re-run), and `jk2coop uninstall` (or the shell
installer's `--uninstall`) removes exactly what it created (tracked in a
manifest). **Retail files are never touched.**

### Settings (config file)

Gameplay and graphics preferences live in a single config file at
`~/.config/jk2coop/config.toml`. Edit it with the two settings TUIs — or by
hand — and it is applied to the game on the next `install` or `launch`
(which rewrites `base/autoexec_sp.cfg`):

```sh
jk2coop game        # mouse sensitivity, blaster speed, aim assist, dynamic crosshair, skip cutscenes
jk2coop graphics    # widescreen, lighting, MSAA, texture upscale/generate (alias: gfx)
```

The `[game]` settings are all runtime cvars and take effect on the next
launch. Under `[graphics]`, `widescreen` and `lighting` are patch-backed:
changing them requires the engine to be rebuilt, which `jk2coop install`
does for you. `msaa` and the texture paks are not patch-backed.

```toml
[game]
sensitivity = 0.5        # base mouse sensitivity
blaster_velocity = 2300  # primary blaster bolt speed (retail 2300); g_blasterVelocity cvar
aim_assist = false       # legacy saber auto-aim + FOV-linked sensitivity
dynamic_crosshair = false
skip_cutscenes = false

[graphics]
widescreen = true        # patch-backed (needs rebuild to change)
lighting = true          # render-fidelity patch (needs rebuild)
msaa = 0                 # r_ext_multisample: 0/2/4/8 (runtime cvar)
texture_upscale = false  # GPU pak
texture_generate = false # GPU pak
```

Blaster speed is backed by patch `0025-blaster-velocity`, which turns the
compile-time `BLASTER_VELOCITY` into the archived `g_blasterVelocity` cvar.
That patch is part of the always-applied co-op base, so a normal
`jk2coop install` builds it in and blaster speed is adjustable from the
config with no extra steps.

### Optional mods

Beyond the core co-op install, three optional game-file mods each add a `zz…`
override pak to your `base/` (retail data is never modified, and uninstalling
removes them too). How you enable them depends on which installer you use.

| Mod | Config key (`[graphics]`) | What it does | Needs |
|---|---|---|---|
| Widescreen menu | `widescreen` | Adds QHD / ultrawide / 4K resolutions to **SETUP → VIDEO → Video Mode** (see [widescreen.md](widescreen.md)) | — |
| Generated textures | `texture_generate` | Original AI material textures via FLUX (see [asset-generation.md](asset-generation.md)) | GPU + container |
| Upscaled textures | `texture_upscale` | Real-ESRGAN hi-res override from your own retail art (see [hires-textures.md](hires-textures.md)) | GPU + container |

**With `jk2coop` (recommended)** the mods are config-driven — there are no
per-mod install flags. Set the key in `~/.config/jk2coop/config.toml` (or toggle
it in the `jk2coop graphics` TUI), then run `jk2coop install`, which builds any
newly-enabled pak and removes any the config no longer wants:

```sh
jk2coop graphics    # toggle "Widescreen" / "Texture upscale" / "Texture generate"
jk2coop install     # builds/removes the override paks to match the config
```

**With the shell installer** the same mods are per-mod flags. On an interactive
terminal it prompts **y/N** for each; run non-interactively (piped, CI) it
enables none unless you pass the matching flag:

| Mod | Flag |
|---|---|
| Widescreen menu | `--with-widescreen` |
| Generated textures | `--with-textures` |
| Upscaled textures | `--with-upscale` |

```sh
tools/install-coop.sh                       # prompts y/N for each optional mod
tools/install-coop.sh --all                 # enable every optional mod
tools/install-coop.sh --with-widescreen     # only the widescreen menu mod
tools/install-coop.sh --no-optional         # core install only, no prompts
```

The GPU-heavy mods (textures, upscale) are run only if a container runtime
(`nerdctl`/`podman`) and an AMD ROCm device (`/dev/kfd`) are present; otherwise
the installer prints the exact command to run later on suitable hardware.

### Combat and render presets

With `jk2coop`, combat feel and render fidelity are set through the config
file rather than install flags — modernized aim/crosshair/bolt feel via
`jk2coop game`, and lighting/widescreen via `jk2coop graphics` (see
[Settings](#settings-config-file) above, [modern-combat.md](modern-combat.md),
and [render-fidelity.md](render-fidelity.md)). Both default on.

The shell installer still writes two cvar-only presets to `base/` and takes
flags for them (they default on, and `--uninstall` removes them):

- `--combat modern|classic` (default `modern`) — modernized aim/crosshair/bolt
  feel; see [modern-combat.md](modern-combat.md).
- `--render high|classic` (default `high`) — sharper textures, anisotropic
  filtering, and the software-overbright lighting fix that keeps world/model
  lighting from going flat on Wayland/windowed; see
  [render-fidelity.md](render-fidelity.md).

```sh
tools/install-coop.sh --render classic      # retail render defaults
tools/install-coop.sh --combat classic      # retail combat feel
```

If `~/.local/bin` is not on your `PATH`, either add it or call the launchers
by their full path.

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

To run a second client **on the same machine** for testing, use `--second`:
it gives that client its own clean `fs_homepath` (`/tmp/jk2-client2`, wiped
first) with its own copy of the gamecode, since the game library is loaded
from the home path.

```sh
jk2coop-host                       # terminal 1
jk2coop-join 127.0.0.1 --second    # terminal 2 (same box)
```

You can also host and discover games from the in-game console — see
the co-op guide: [coop-guide.md](coop-guide.md).

## Manual install (without the script)

If you would rather stage things by hand:

```sh
mkdir -p ~/.local/share/openjo/base
ln -sfn "<steam>/Jedi Outcast/GameData/base/"assets*.pk3 ~/.local/share/openjo/base/
ln -sfn "$PWD/openjk/build/codeJK2/game/jospgamex86_64.so" ~/.local/share/openjo/base/
# the renderer module is loaded relative to the executable:
ln -sfn "$PWD/openjk/build/code/rd-vanilla/rdjosp-vanilla_x86_64.so" openjk/build/
cd openjk/build && ./openjo_sp.x86_64 +map kejim_post
```
