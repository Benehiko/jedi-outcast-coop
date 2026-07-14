# jk2coop tooling binary

`jk2coop` is a single, cross-platform, testable Go binary that reimplements the
deterministic parts of the `tools/*.sh` scripts: applying the OpenJK patch set,
building the override paks, and installing the co-op engine data directory and
launchers. It is the recommended way to run these steps on Linux, macOS, and
Windows — the shell scripts remain in `tools/` and continue to work unchanged.

The GPU/container pipelines (texture generation, Real-ESRGAN upscale) and the
headless render harnesses (`headless-verify.sh`, `soak-m4.sh`) are still shell
scripts; the installer shells out to them for the optional texture mods.

## Building

Requires Go 1.26+.

```bash
make build          # produces ./jk2coop with version metadata baked in
# or
go build -mod=vendor -o jk2coop .
```

Pre-built binaries for each platform are attached to every tagged
[GitHub Release](https://github.com/Benehiko/jedi-outcast-coop/releases)
(`jk2coop_<version>_<os>_<arch>.tar.gz`, `.zip` for Windows):

```bash
# replace v0.1.0 with the latest release tag
curl -LO https://github.com/Benehiko/jedi-outcast-coop/releases/download/v0.1.0/jk2coop_v0.1.0_linux_amd64.tar.gz
tar -xzf jk2coop_v0.1.0_linux_amd64.tar.gz
```

## Shell autocompletion

The binary generates completion scripts for bash, zsh, fish, and PowerShell:

```bash
source <(jk2coop completion bash)                       # bash, current shell
jk2coop completion zsh > "${fpath[1]}/_jk2coop"         # zsh
jk2coop completion fish > ~/.config/fish/completions/jk2coop.fish
```

```powershell
jk2coop completion powershell | Out-String | Invoke-Expression
```

Run `jk2coop completion <shell> --help` for how to install each one
persistently.

## Commands

There are exactly seven user-facing commands, plus `version` and a hidden
`dev` group for the low-level build steps:

| Command | What it does |
| --- | --- |
| `jk2coop install` | Installs the engine and applies your config: builds/stages the data dir (symlinks + gamecode), writes the autoexec cvars, rebuilds the engine if a patch-backed graphics feature changed, and places any optional texture paks. OS-detected. Flags: `--repo`, `--build`, `--gamedata`, `-y`/`--yes`. |
| `jk2coop launch` | Runs the staged engine. **Hosts a co-op game by default**; `--join HOST[:PORT]` to join, `--solo` for single-player. Also `--map`, `--windowed`, `--port`, `--print`. |
| `jk2coop host` | Explicitly hosts a co-op game. |
| `jk2coop join <IP[:PORT]>` | Joins a co-op game by IP (a positional argument). |
| `jk2coop game` | Game Settings TUI — mouse sensitivity, blaster speed, aim assist, dynamic crosshair, skip cutscenes. All runtime cvars, no rebuild. |
| `jk2coop graphics` (alias `gfx`) | Graphics Settings TUI — widescreen, lighting, resolution, MSAA, texture upscale, texture generate. Widescreen and lighting are patch-backed and offer a rebuild on change; resolution, MSAA and the texture paks are not. Resolution auto-suggests the monitor's current mode. |
| `jk2coop uninstall` | Removes exactly what the install created (manifest-tracked). |
| `jk2coop version` | Prints version, commit, and build date. |

### The hidden `dev` group

The low-level build steps that used to be top-level commands now live under a
hidden `jk2coop dev …` group. Regular users never need them; they are for
working on the engine and the paks:

| Command | Replaces | What it does |
| --- | --- | --- |
| `jk2coop dev patches apply` | `apply-patches.sh` | Applies the patch set to the pinned OpenJK submodule, in order. |
| `jk2coop dev pk3 coop-ui` | `build-coop-ui-pk3.sh` | Packs `assets/coop-ui/ui` into `zz-coop-ui.pk3`. |
| `jk2coop dev pk3 coop-npcs <GameData/base>` | `build-coop-npcs-pk3.sh` | Extracts the retail NPC config and repackages it as `zzz-coop-npcs.pk3`. |
| `jk2coop dev pk3 widescreen` | `build-widescreen-menu-pk3.sh` | Patches the SP video-menu resolution list into `zz-widescreen-menu.pk3`. |
| `jk2coop dev pk3 sensitivity` | `build-sensitivity-menu-pk3.sh` | Rescales the SP CONTROLS mouse-sensitivity slider into `zz-sensitivity-menu.pk3`. |
| `jk2coop dev gfx …` | — | Low-level graphics-feature toggle (reapply patches → rebuild → reinstall). `jk2coop graphics` is the user-facing wrapper. |

Run any command with `--help` for its flags.

### Repository root detection

`install`, and the `dev patches`/`dev pk3` build steps, locate the repo root
by walking up from the working directory until they find the `patches/`,
`openjk/`, and `go.mod` markers. Run them from anywhere inside the checkout,
or pass `--repo <path>`.

### Install flags

`install` no longer takes combat/render/mod flags — those settings now live
in the config file (see [Settings](#settings-the-config-file) below). The
remaining flags are:

```bash
jk2coop install                       # autodetect Steam GameData
jk2coop install --gamedata /path/to/"Jedi Outcast"/GameData
jk2coop install --repo /path/to/checkout   # repo root (else autodetected)
jk2coop install --build /path/to/build     # engine build dir
jk2coop install -y                    # assume "yes" to prompts (non-interactive)
```

`install` builds/stages the engine, symlinks your retail assets and the co-op
gamecode, applies your config (writing the autoexec cvars, rebuilding the
engine if a patch-backed graphics feature changed, and placing any optional
texture paks), and installs the launchers. To remove everything it created,
run `jk2coop uninstall` (manifest-tracked).

The install always writes `base/autoexec_sp.cfg` (the engine execs it at
startup, so it wins over a stale `openjo_sp.cfg`) from your config's `[game]`
cvars. See [Settings](#settings-the-config-file) and
[modern-combat.md](modern-combat.md).

### Settings: the config file

`jk2coop`'s single source of truth for gameplay and graphics preferences is a
TOML file resolved via `os.UserConfigDir`:

| OS | Path |
| --- | --- |
| Linux | `~/.config/jk2coop/config.toml` |
| macOS | `~/Library/Application Support/jk2coop/config.toml` |
| Windows | `%AppData%\jk2coop\config.toml` |

Edit it with the `jk2coop game` and `jk2coop graphics` TUIs, or by hand. It is
applied to the game on the next `install` or `launch` (which rewrites
`base/autoexec_sp.cfg`).

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
msaa = 0                 # r_ext_multisample: 0/2/4/8/16 (runtime cvar)
texture_upscale = false  # GPU pak
texture_generate = false # GPU pak
```

The `[game]` fields are all runtime cvars, so `jk2coop game` never rebuilds.
Under `[graphics]`, `widescreen` and `lighting` are patch-backed features —
changing them means the engine must be rebuilt, which `jk2coop graphics`
offers to do (and `jk2coop install` does on demand). `msaa` and the two
texture paks are not patch-backed.

`blaster_velocity` is backed by patch `0025-blaster-velocity`, which turns the
compile-time `BLASTER_VELOCITY` into the archived `g_blasterVelocity` cvar.
That patch is part of the always-applied co-op base (patches `0001`–`0021`),
so a normal `jk2coop install` builds it in and blaster speed is adjustable
from the config with no extra steps.

### Launch flags

`jk2coop launch` runs the same engine `install` staged, with `fs_basepath`
pointed at the data dir so it picks up the co-op gamecode, the linked retail
assets, and your config-derived `autoexec_sp.cfg`. It **hosts a co-op game by
default**, and subsumes the generated `jk2coop-host` / `jk2coop-join` launcher
scripts. `jk2coop host` and `jk2coop join <IP>` are explicit shortcuts for the
host/join modes.

```bash
jk2coop launch                        # host a co-op game on UDP 29070 (default), fullscreen
jk2coop launch --solo                 # single-player, default map (kejim_post)
jk2coop launch --map t2_trip          # a specific map
jk2coop launch --windowed             # run windowed instead of fullscreen
jk2coop launch --port 30000           # host on a specific port
jk2coop launch --join 192.168.1.5     # join (defaults to :29070)
jk2coop launch --join 192.168.1.5:30000
jk2coop launch --print                # print the resolved engine command, don't run it
jk2coop launch -- +set r_mode -2      # pass raw engine args after `--`
```

Cutscene skip and the other combat cvars are no longer launch flags — set
them once in the config with `jk2coop game`.

On Unix `launch` **replaces** the `jk2coop` process with the engine (via
`exec`), so the game keeps running under your shell after `jk2coop` would have
exited. On Windows there is no `exec`; the engine runs as a child and `jk2coop`
waits for it. If the engine isn't built where it's expected, `launch` errors
with setup guidance instead of running — build first (see
[building.md](building.md)) and `jk2coop install`, or point `--build <dir>` at
your build.

Platform layout (overridable via the same `JK2_*` env vars the scripts use):

| | Linux | macOS | Windows |
| --- | --- | --- | --- |
| Data dir (`JK2_DATA_DIR`) | `~/.local/share/openjo` | `~/Library/Application Support/OpenJO` | `%LOCALAPPDATA%\OpenJO` |
| Bin dir (`JK2_BIN_DIR`) | `~/.local/bin` | `~/bin` | `%LOCALAPPDATA%\OpenJO\bin` |
| Gamecode | `jospgame<arch>.so` | `jospgame<arch>.dylib` | `jospgame<arch>.dll` |
| Launchers | `jk2coop-host`, `jk2coop-join` | same | `jk2coop-host.cmd`, `jk2coop-join.cmd` |

> **Windows note:** staging uses symlinks (`ln -sfn` equivalent). Creating a
> symlink on Windows needs Developer Mode enabled or an elevated shell; if the
> OS refuses, the install fails with the underlying error.

### Graphics settings (`jk2coop graphics`)

`jk2coop graphics` (alias `gfx`) is the Graphics Settings TUI. It edits the
`[graphics]` block of the config file — widescreen, lighting, resolution, MSAA,
texture upscale, and texture generate — and applies the change:

- **`widescreen`** and **`lighting`** are patch-backed features (16:9/21:9/32:9
  2D aspect correction with edge-anchored HUD, and software-overbright lighting
  with a matching model-brightness boost). Because the patches must apply to a
  pristine submodule, changing either means resetting OpenJK to the pinned
  commit and reapplying the co-op base (patches `0001`–`0021`, always on) plus
  the chosen features, then rebuilding. `jk2coop graphics` offers that rebuild
  when you change one.
- **Resolution** (`res_width`/`res_height` → `r_mode -1` + `r_customwidth`/
  `r_customheight`) is a runtime cvar set — no rebuild. `auto` (`0x0`) leaves the
  engine on its own `r_mode` default; any other value forces the custom video
  mode. The TUI auto-suggests your monitor's current resolution (detected via
  `xrandr` on X11 or `wlr-randr` on Wayland) and flags the matching entry as
  `native`.
- **`msaa`** (`r_ext_multisample`: 0/2/4/8/16) is a runtime cvar — no rebuild.
- **`texture_upscale`** and **`texture_generate`** place optional GPU-built
  override paks — no engine rebuild.

Most of the patch-backed features are also live cvars (`r_aspectCorrect2D`,
`cg_hudEdgeAnchor`, `r_overBrightBits*`), so for day-to-day tweaking you can
toggle them from the console without a rebuild — see
[widescreen.md](widescreen.md) and [render-fidelity.md](render-fidelity.md).
Use `jk2coop graphics` when you want a feature's code compiled in or out
entirely.

The low-level feature toggle (reset → reapply patches → `cmake --build` →
`jk2coop install`) is still available under the hidden `jk2coop dev gfx …`
group for engine work; `jk2coop graphics` is the user-facing wrapper over it.

## Development

```bash
make hooks   # enable the pre-commit hook (format check + lint + build) for this clone
make fmt     # apply gofumpt + goimports
make lint    # format check + golangci-lint (mirrors CI)
make test    # go test -race ./... (unit tests)
make e2e     # end-to-end tests (needs the OpenJK submodule + git)
```

### End-to-end tests

`e2e/` (gated behind the `e2e` build tag) drives the built binary against the
real repository rather than mocks. `make e2e` builds `jk2coop` and runs:

- **`TestPatchesApplyToPristineSubmodule`** — resets the OpenJK submodule to
  pristine, runs `jk2coop dev patches apply`, and asserts every `patches/*.patch`
  is reported `applied` (none skipped) and the submodule tree actually changed.
- **`TestPatchesApplyNotIdempotentOnDirtyTree`** — applies once, then asserts a
  second apply on the now-patched tree fails with the reset guidance (the
  patches overlap and are cumulative, so double-applying must error).

Both reset the submodule on cleanup, so the working copy is never left dirty.

CI runs the unit checks in the `go` job of `.github/workflows/go.yml` (lint,
format, host build, darwin/windows cross-compile, race tests) and the e2e tests
in a separate `e2e` job that checks out the submodule. Tagged `v*` pushes
trigger `.github/workflows/release.yml`, which builds every platform and
attaches the archives to a GitHub Release.

## Design notes

- **Paks are built with Go's `archive/zip`** — no `zip`/`unzip` shell-out.
  Entries are written in sorted order with a fixed compression method, so a
  rebuild from the same inputs is byte-identical.
- **The widescreen menu patch operates on raw bytes**, preserving the retail
  menu files' CRLF line endings and latin-1 encoding, and refuses to touch a
  file whose resolution list is not in the exact stock form (already patched or
  a different edition).
- **`dev patches apply` is not idempotent on a dirty tree** — the patches overlap
  and are cumulative (e.g. 0004 sets the `sv_maxclients` infostring to
  `MAX_CLIENTS` and 0020 later rewrites that same line to honour the runtime
  cvar), so re-running against a fully-patched submodule aborts. Reset first:
  `git -C openjk checkout -- . && git -C openjk clean -fd`.
- **Uninstall never force-removes.** It deletes the files/symlinks it created,
  then only `rmdir`s tracked directories that are now empty (deepest-first), so
  any directory still holding files it did not create is left in place.
