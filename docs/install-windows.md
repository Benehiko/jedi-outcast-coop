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

Either download the `jk2coop-windows` artifact from a green
[CI run](../.github/workflows/build.yml), or build locally:

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

## 2. Install with the PowerShell installer

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

After the core install the script offers optional game-file mods, each of which
just adds a `zz…` override pak to `base\` (retail data is never modified, and
`-Uninstall` removes them too). On an interactive console it prompts **y/N** for
each; run non-interactively it enables none unless you pass the matching switch.

| Mod | Switch | What it does | Availability |
|---|---|---|---|
| Widescreen menu | `-WithWidescreen` | Adds QHD / ultrawide / 4K to **SETUP → VIDEO → Video Mode** (see [widescreen.md](widescreen.md)) | Works on Windows (built natively in PowerShell from your own retail menus) |
| Generated textures | `-WithTextures` | Original AI material textures ([asset-generation.md](asset-generation.md)) | Linux GPU-only — the installer prints the command to run on a Linux machine |
| Upscaled textures | `-WithUpscale` | Real-ESRGAN hi-res override ([hires-textures.md](hires-textures.md)) | Linux GPU-only — prints the command |

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

Two cvar-only presets are written to `base\` and default on (revertible;
`-Uninstall` removes them):

- `-Combat modern|classic` (default `modern`) — see
  [modern-combat.md](modern-combat.md).
- `-Render high|classic` (default `high`) — sharper textures, anisotropic
  filtering, and the software-overbright lighting fix (matters in
  windowed/borderless); see [render-fidelity.md](render-fidelity.md).

## 3. Play

The installer writes `jk2coop-host.cmd` and `jk2coop-join.cmd` next to the
staging directory. Host a game:

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
