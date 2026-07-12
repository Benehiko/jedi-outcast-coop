# Jedi Outcast Rebuild

A build of Jedi Outcast singleplayer with **cooperative campaign play**
— up to four players in one campaign game — for **Linux and Windows**.

## Status

Working today, verified in a live session: one player **hosts the
campaign** (briefing, objectives, scripted NPCs) and up to three others
join over UDP/LAN, spawn beside the host, and play with their own fully
rendered view. Hosting, LAN discovery, and an in-game Co-op menu are all
in.

**Linux and Windows both run the co-op engine**, and they interoperate:
a Windows host and a Linux client (or the reverse) can play the same
game over the network — verified live. Linux has a one-command
installer; Windows has a PowerShell installer (`tools/install-coop.ps1`).
A macOS installer is written but not yet verified on real hardware.

The engine also runs at modern resolutions — QHD, 4K, and ultrawide
(21:9 / 32:9) — with the HUD and menus kept in correct proportion instead
of stretched, and an opt-in widescreen field of view
([docs/widescreen.md](docs/widescreen.md)).

Combat is modernized for today's hardware: mouse aim is decoupled from
FOV (no more sluggish, hyper-sensitive feel on high-DPI mice), saber
auto-aim no longer snaps onto nearby enemies by default, and blaster
bolts fly roughly twice as fast — all opt-in via cvars
([docs/modern-combat.md](docs/modern-combat.md)).

In progress: syncing the campaign UI — objectives, mission text,
cutscene handling — to joiners ([Track F](docs/campaign-ui-plan.md)).
Current task status: [docs/tasks.md](docs/tasks.md).

## Installing

**This project does not include Jedi Outcast's game data, and never
will.** You must own a legal copy of *Star Wars Jedi Knight II: Jedi
Outcast* — for example the
[Steam release](https://store.steampowered.com/app/6030/) — so the
retail `assets*.pk3` files already exist on your machine.

What this project ships is: the *source changes* to the OpenJK engine
that add co-op (the diffs in `patches/`, which build into the engine,
renderer, and gamecode binaries), an **original** Co-op menu overlay
(`assets/coop-ui/`), and the installer scripts in `tools/`. The
installers only ever symlink (Linux/macOS) or additively place (Windows)
files — your game install is never copied from or modified.

| OS | Guide | Status |
|---|---|---|
| Linux | [docs/install-linux.md](docs/install-linux.md) | one-command installer |
| Windows | [docs/install-windows.md](docs/install-windows.md) | PowerShell installer; co-op verified live |
| macOS | [docs/install-macos.md](docs/install-macos.md) | one-command installer (not yet run on real hardware) |

The short version on Linux, once built ([docs/building.md](docs/building.md)):

    tools/install-coop.sh              # symlinks your Steam assets + co-op gamecode into place
    jk2coop-host                       # host a game on UDP 29070
    jk2coop-join <host-ip>             # join it from another machine

On Windows (from the `jk2coop-windows` CI artifact or a local build):

    powershell -ExecutionPolicy Bypass -File tools\install-coop.ps1
    jk2coop-host.cmd                   # host a game on UDP 29070
    jk2coop-join.cmd <host-ip>         # join it from another machine

Hosting from the in-game console/menu and LAN discovery:
[docs/coop-guide.md](docs/coop-guide.md).

## Documentation

| Document | What it is |
|---|---|
| [install-linux.md](docs/install-linux.md) / [install-macos.md](docs/install-macos.md) / [install-windows.md](docs/install-windows.md) | **Playing? Start here.** Per-OS install guides |
| [coop-guide.md](docs/coop-guide.md) | Hosting, finding, and joining co-op games |
| [widescreen.md](docs/widescreen.md) | Running at QHD / 4K / ultrawide with correct HUD proportions and FOV |
| [modern-combat.md](docs/modern-combat.md) | Modernized combat feel: FOV-independent aim, saber auto-aim off by default, faster blaster bolts (all cvar/opt-in) |
| [hires-textures.md](docs/hires-textures.md) | Optional: locally AI-upscale your own textures into a high-res override pak |
| [asset-generation.md](docs/asset-generation.md) | Optional: locally generate original, non-branded material textures (Apache-licensed model); the licensing/trademark analysis |
| [building.md](docs/building.md) | Building from source, debug builds, development loop |
| [ci.md](docs/ci.md) | What the GitHub Actions CI checks, and how to run those checks locally |
| [tasks.md](docs/tasks.md) | **Implementing? Start here.** Status: what's done, what's outstanding, as sitting-sized tasks |
| [campaign-ui-plan.md](docs/campaign-ui-plan.md) | Track F plan: syncing objectives, mission text, and cutscenes to joiners |
| [roadmap.md](docs/roadmap.md) | The original plan in phases |
| [implementation-plan.md](docs/implementation-plan.md) | Handoff-ready plan: dual-load rendering, co-op UX, four players, installers |
| [cgame-split-investigation.md](docs/cgame-split-investigation.md) | Why the cgame library split was rejected |
| [route-comparison.md](docs/route-comparison.md) | Why the SP engine was widened instead of hosting on the MP tree |
| [mp-route.md](docs/mp-route.md) | The superseded MP-tree route, preserved with instructions |
| [investigation-log.md](docs/investigation-log.md) | Everything tried, measured, and concluded — including the wrong turns |
| [widen-sp-progress.md](docs/widen-sp-progress.md) / [coop-design.md](docs/coop-design.md) | Historical: early progress log and the superseded original study |

## Approach

The engine is not reverse engineered. Raven Software released the Jedi
Outcast source under the GPLv2 in 2013;
[OpenJK](https://github.com/JACoders/OpenJK) maintains it. Only the
retail assets are proprietary, and they are used in place, unmodified.
Changes to OpenJK live in `patches/` and are applied to a pinned
submodule by `tools/apply-patches.sh`, rather than carrying a fork.

## License

The engine is [OpenJK](https://github.com/JACoders/OpenJK), GPLv2. This
project's changes to it (`patches/`) are derivative works and are
therefore also **GPLv2**, as are the built binaries; the full license
text is in [LICENSE](LICENSE). The original authorship in this
repository — patches, tools, docs, and the Co-op UI overlay — is offered
under the same terms.

This project ships **no game data**. The retail `assets*.pk3` files are
proprietary and are used in place from your own legal copy of the game;
see [Installing](#installing).

### Trademarks

*Star Wars*, *Jedi Knight*, *Jedi Outcast*, and related names and marks
are trademarks of their respective owners (Lucasfilm / Disney, and the
game's publishers). This is an unofficial, non-commercial, fan-made
project, not affiliated with, endorsed by, or sponsored by any of those
rights holders. The GPL covers the source code only — it grants no
rights in these trademarks or in the proprietary game assets.
