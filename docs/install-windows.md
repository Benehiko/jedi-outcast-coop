# Installing on Windows

> **Status: working, verified live.** The Windows engine builds under MSVC —
> the winsock port (patch 0016) makes the co-op UDP transport compile, and
> patch 0005 links `wsock32` into the JK2SP engine so it also *links* (without
> that library the build failed with winsock `LNK2019` unresolved externals).
> CI produces `openjo_sp.x86_64.exe`, the gamecode and renderer DLLs, and
> `SDL2.dll`. The PowerShell installer `tools/install-coop.ps1` (see § 2) has
> been run against a real retail install on Windows 10: the campaign plays,
> and co-op works both cross-platform (a Windows host with a Linux client, and
> the reverse) and between two Windows clients. This is still young — it is
> "experimental" in the sense of not yet widely exercised across Windows
> versions/hardware, so contributions and bug reports are welcome.

## Before you start: you need a legal copy of the game

**This project does not include Jedi Outcast's game data, and never will.**
The maps, models, textures, sounds, and music live in the retail
`assets*.pk3` files, which are proprietary and owned by their rights holders.
You must own a legal copy of *Star Wars Jedi Knight II: Jedi Outcast* — for
example the [Steam release](https://store.steampowered.com/app/6030/) — so
that those files already exist on your machine (typically under
`...\steamapps\common\Jedi Outcast\GameData\`).

**What this project ships**, and all it ships, is:

- the *source changes* to the [OpenJK](https://github.com/JACoders/OpenJK)
  engine that add cooperative play (the diffs in `patches/`), which build into
  the engine, the OpenGL renderer, and the singleplayer gamecode;
- a small original UI overlay for the in-game Co-op menu (`assets/coop-ui/`,
  packed into `zz-coop-ui.pk3` at build time); and
- the helper scripts in `tools/`.

Nothing from your retail game install is copied or redistributed here.

## 1. Get the binaries

The simplest path is `jk2coop setup`, which extracts the engine source
**embedded in the binary**, applies the co-op patches (in pure Go — no `git`
needed), builds the engine, and installs in one guided step — it prints what to
install if the MSVC/CMake toolchain is missing. A pre-built `jk2coop.exe` needs
neither a clone nor the OpenJK submodule; if you build `jk2coop` yourself, run
`make build` first.

If you would rather not build the engine at all, download a prebuilt one and
skip to install: fetch the `jk2coop-windows` artifact from a green
[CI run](../.github/workflows/build.yml).

By default `jk2coop setup` builds the Windows engine **without installing Visual
Studio, CMake, or Docker** on your machine: it cross-compiles with mingw-w64
inside a container running in a small VM managed by
[`vee`](https://github.com/Benehiko/vee), and the resulting `.exe`/`.dll` run on
your Windows host. If `vee` is not already on your `PATH`, `setup` downloads a
pinned, checksum-verified copy into `%AppData%\jk2coop\bin` and keeps it for
later rebuilds. See [building.md](building.md#building-in-a-container---docker)
for how the container path works and its target matrix, [build-vm.md](build-vm.md)
for managing the vee/VM setup (`jk2coop vee`), and the MSVC path above is still
the reference for patch development and what CI uses.

To build the engine from the OpenJK **submodule** by hand instead (patch
development), see [embedded-source.md](embedded-source.md) and:

- install Visual Studio (MSVC) and CMake, plus the OpenJK Windows build
  dependencies;
- apply the co-op patches — `tools/apply-patches.sh` runs under the Git-for-
  Windows bash that ships with Git;
- configure and build the JK2 singleplayer engine, renderer, and gamecode
  with the same `-DBuildJK2SP*` options shown in the
  [Linux guide](install-linux.md#1-build-the-binaries).

The CI `jk2coop-windows` artifact (and a local build) produces the engine
`openjo_sp.x86_64.exe`, the renderer `rdjosp-vanilla_x86_64.dll`, the gamecode
`jospgamex86_64.dll`, and `SDL2.dll` (the engine's windowing library, loaded
next to the exe).

## 2. Install

The cross-platform `jk2coop` Go binary is the recommended installer on
Windows too (see [tooling.md](tooling.md)):

```powershell
jk2coop install                 # autodetect Steam GameData
jk2coop install --gamedata "D:\Games\Jedi Outcast\GameData"
jk2coop install -y              # assume yes to prompts (non-interactive)
```

`jk2coop install` stages the co-op engine, symlinks your retail assets and
the co-op gamecode, applies your config (autoexec cvars, the patch-backed
graphics features, and any optional texture paks), and installs the
launchers. Run `jk2coop uninstall` to remove everything it installed.
(Creating symlinks on Windows needs Developer Mode enabled or an elevated
shell; if the OS refuses, the install fails with the underlying error.)

Gameplay and graphics preferences live in a single config file at
`%AppData%\jk2coop\config.toml`. Edit it with `jk2coop game` (mouse
sensitivity, blaster speed, aim assist, dynamic crosshair, skip cutscenes)
or `jk2coop graphics` (widescreen, lighting, MSAA, texture upscale/generate),
and it is applied on the next `install` or `launch`. `[game]` settings are
runtime cvars; under `[graphics]`, `widescreen` and `lighting` are
patch-backed and require a rebuild to change (which `install` handles). See
the [Linux guide](install-linux.md#settings-config-file) for the full TOML
schema.

### The PowerShell installer

`tools/install-coop.ps1` performs an additive install — it never copies,
overwrites, or modifies any retail file. It stages the co-op engine,
renderer, gamecode DLL, `SDL2.dll`, and the Co-op UI overlay into a separate
directory (by default `%LOCALAPPDATA%\jk2coop`) and writes two launcher
scripts beside it. At runtime the engine loads your retail assets read-only
from your `GameData` directory via `fs_cdpath`, while the co-op files load
from the staging directory via `fs_basepath`.

The installer also ensures the **Visual C++ 2015-2022 redistributable** is
present (the MSVC-built engine links the dynamic CRT and will not start
without it) — it is a shared system component, so `-Uninstall` leaves it in
place.

From a PowerShell prompt in the repository root:

```powershell
# Drop the three built binaries somewhere (e.g. the extracted jk2coop-windows
# artifact), then:
powershell -ExecutionPolicy Bypass -File tools\install-coop.ps1 `
  -Binaries <folder with the 3 files>
```

The installer locates your Jedi Outcast `GameData` automatically via the
Steam registry key (`HKCU:\Software\Valve\Steam`) and
`libraryfolders.vdf`. If your install lives somewhere non-standard (for
example a different drive or a network path), point at it explicitly:

```powershell
powershell -ExecutionPolicy Bypass -File tools\install-coop.ps1 `
  -Binaries .\artifact\RelWithDebInfo `
  -GameData "D:\Games\Jedi Outcast\GameData"
```

Re-running is idempotent. To remove exactly what the installer created
(leaving your retail install untouched):

```powershell
powershell -ExecutionPolicy Bypass -File tools\install-coop.ps1 -Uninstall
```

### Optional mods

Three optional game-file mods each add a `zz…` override pak to `base\` (retail
data is never modified, and uninstalling removes them too). How you enable them
depends on which installer you use.

| Mod | Config key (`[graphics]`) | What it does | Availability |
|---|---|---|---|
| Widescreen menu | `widescreen` | Adds QHD / ultrawide / 4K to **SETUP → VIDEO → Video Mode** (see [widescreen.md](widescreen.md)) | Works on Windows (built natively from your own retail menus) |
| Generated textures | `texture_generate` | Original AI material textures ([asset-generation.md](asset-generation.md)) | Linux GPU-only — the installer prints the command to run on a Linux machine |
| Upscaled textures | `texture_upscale` | Real-ESRGAN hi-res override ([hires-textures.md](hires-textures.md)) | Linux GPU-only — prints the command |

**With `jk2coop` (recommended)** the mods are config-driven — no per-mod install
switches. Toggle the key in the `jk2coop graphics` TUI (or edit
`%AppData%\jk2coop\config.toml`), then run `jk2coop install`, which builds any
newly-enabled pak and removes any the config no longer wants:

```powershell
jk2coop graphics    # toggle "Widescreen" / "Texture upscale" / "Texture generate"
jk2coop install     # builds/removes the override paks to match the config
```

**With the PowerShell installer** the same mods are per-mod switches. On an
interactive console it prompts **y/N** for each; run non-interactively it enables
none unless you pass the matching switch (`-WithWidescreen`, `-WithTextures`,
`-WithUpscale`):

```powershell
powershell -ExecutionPolicy Bypass -File tools\install-coop.ps1                    # prompts y/N
powershell -ExecutionPolicy Bypass -File tools\install-coop.ps1 -All               # enable everything offered
powershell -ExecutionPolicy Bypass -File tools\install-coop.ps1 -WithWidescreen    # only the widescreen menu
powershell -ExecutionPolicy Bypass -File tools\install-coop.ps1 -NoOptional        # core install only
```

The AI-texture mods need an AMD ROCm GPU container, which is a Linux-only setup;
on Windows they are offered but resolve to a printed command you can run on a
Linux machine, then copy the resulting `zzz-*.pk3` into your `base\`.

### Combat and render presets

With `jk2coop`, combat feel and render fidelity are set through the config
file (`jk2coop game` and `jk2coop graphics`) rather than install switches —
see the settings note under [Install](#2-install) above. Both default on.

The PowerShell installer still writes two cvar-only presets to `base\` and
takes switches for them (they default on; `-Uninstall` removes them):

- `-Combat modern|classic` (default `modern`) — see
  [modern-combat.md](modern-combat.md).
- `-Render high|classic` (default `high`) — sharper textures, anisotropic
  filtering, and the software-overbright lighting fix (matters in
  windowed/borderless); see [render-fidelity.md](render-fidelity.md).

## 3. Play

With the `jk2coop` binary:

```powershell
jk2coop launch                  # play; hosts a co-op game on UDP 29070 by default
jk2coop launch --map ns_streets # host a specific map
jk2coop join <host-ip>          # join a co-op game from another machine
jk2coop launch --solo           # single-player
```

The PowerShell installer also writes `jk2coop-host.cmd` and
`jk2coop-join.cmd` next to the staging directory. Host a game:

```bat
jk2coop-host.cmd            :: host kejim_post on port 29070
jk2coop-host.cmd ns_streets :: host a specific map
```

Join, from another machine:

```bat
jk2coop-join.cmd <host-ip>
```

Prefer to run the engine directly? The equivalent invocations are:

```bat
openjo_sp.x86_64.exe +set fs_basepath "<staging dir>" ^
  +set fs_cdpath "<...>\Jedi Outcast\GameData" ^
  +set net_enabled 1 +set net_port 29070 +map kejim_post

openjo_sp.x86_64.exe +set fs_basepath "<staging dir>" ^
  +set fs_cdpath "<...>\Jedi Outcast\GameData" ^
  +set net_enabled 1 +connect <host-ip>
```

You can also host and discover games from the in-game console — see
the co-op guide: [coop-guide.md](coop-guide.md).

## Status

The installer's logic (GameData autodetection, additive staging,
idempotent re-runs, and manifest-tracked `-Uninstall`) has been exercised
end-to-end on a mock tree. Running it against a real retail install on a
live Windows machine, and confirming the engine hosts/joins there, is the
last verification step for task C4 in [tasks.md](tasks.md).
