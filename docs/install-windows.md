# Installing on Windows

> **Status: experimental — no installer yet.** The Windows *engine* builds
> (the winsock port, patch 0016, lets the co-op UDP transport compile under
> MSVC, and CI produces `openjo_sp.exe`), but the one-command installer for
> Windows is not written yet — it is tracked as task **C4** in
> [tasks.md](tasks.md). For now, Windows setup is manual. Contributions
> welcome.

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

This produces `openjo_sp.exe`, the renderer DLL, and the gamecode DLL
(`jospgamex86.dll` / `jospgamex86_64.dll` depending on the target).

## 2. Install (manual, for now)

Additive only — copy the built files alongside your retail install; never
modify or overwrite retail files.

1. Locate your retail `GameData` directory (the one containing
   `base\assets0.pk3`).
2. Copy the built **gamecode DLL** and the **Co-op UI overlay**
   (`zz-coop-ui.pk3`, built from `assets/coop-ui/`) into `GameData\base\`.
3. Keep `openjo_sp.exe` and the renderer DLL together in their own folder;
   point the engine at your retail assets with
   `+set fs_basepath "<path to>\Jedi Outcast\GameData"`.

## 3. Play

Host:

```bat
openjo_sp.exe +set fs_basepath "<...>\Jedi Outcast\GameData" ^
  +set net_enabled 1 +set net_port 29070 +map kejim_post
```

Join, from another machine:

```bat
openjo_sp.exe +set fs_basepath "<...>\Jedi Outcast\GameData" ^
  +set net_enabled 1 +connect <host-ip>
```

You can also host and discover games from the in-game console — see
[Hosting and finding games from the console](../README.md#hosting-and-finding-games-from-the-console)
in the README.

## Help wanted

A proper `tools/install-coop.ps1` (GameData autodetection via the Steam
registry key + `libraryfolders.vdf`, additive copy into `GameData\base`,
host/join `.cmd` launchers, `-Uninstall`) is the missing piece — see task C4
in [tasks.md](tasks.md).
