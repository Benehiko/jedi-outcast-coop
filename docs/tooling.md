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

Every subcommand maps 1:1 to one of the original scripts:

| Command | Replaces | What it does |
| --- | --- | --- |
| `jk2coop patches apply` | `apply-patches.sh` | Applies the patch set to the pinned OpenJK submodule, in order. |
| `jk2coop pk3 coop-ui` | `build-coop-ui-pk3.sh` | Packs `assets/coop-ui/ui` into `zz-coop-ui.pk3`. |
| `jk2coop pk3 coop-npcs <GameData/base>` | `build-coop-npcs-pk3.sh` | Extracts the retail NPC config and repackages it as `zzz-coop-npcs.pk3`. |
| `jk2coop pk3 widescreen` | `build-widescreen-menu-pk3.sh` | Patches the SP video-menu resolution list into `zz-widescreen-menu.pk3`. |
| `jk2coop pk3 sensitivity` | `build-sensitivity-menu-pk3.sh` | Rescales the SP CONTROLS mouse-sensitivity slider into `zz-sensitivity-menu.pk3`. |
| `jk2coop install` | `install-coop.sh` / `install-coop-macos.sh` / `install-coop.ps1` | Stages the data dir (symlinks + gamecode) and installs the launchers. OS-detected. |
| `jk2coop install --uninstall` | `… --uninstall` | Removes exactly what the install created (manifest-tracked). |
| `jk2coop launch` | `jk2coop-host` / `jk2coop-join` | Runs the staged engine: single-player (default), `--host`, or `--join <addr>`. |
| `jk2coop gfx` | — | Toggle the graphics features (combat, widescreen, render fidelity), then reapply patches, rebuild, and reinstall. |
| `jk2coop version` | — | Prints version, commit, and build date. |

Run any command with `--help` for its flags.

### Repository root detection

`patches`, `pk3 coop-ui`, and `install` locate the repo root by walking up from
the working directory until they find the `patches/`, `openjk/`, and `go.mod`
markers. Run them from anywhere inside the checkout, or pass `--repo <path>`.

### Install flags

```bash
jk2coop install                       # autodetect Steam GameData, prompt for optional mods
jk2coop install --gamedata /path/to/"Jedi Outcast"/GameData
jk2coop install --all                 # enable every optional mod
jk2coop install --with-widescreen     # only the widescreen menu mod
jk2coop install --no-optional         # core install only, no prompts
jk2coop install --yes                 # assume "yes" to prompts (non-interactive)
jk2coop install --uninstall           # remove everything it created
```

**Modern combat feel.** The install always writes `base/autoexec_sp.cfg` (the
engine execs it at startup, so it wins over a stale `openjo_sp.cfg`) with the
combat cvars, matching `install-coop.sh`:

```bash
jk2coop install --combat modern       # default: free aim, fixed crosshair, FOV-independent sensitivity, fast bolts
jk2coop install --combat classic      # legacy feel (auto-aim, dynamic crosshair, FOV-linked sensitivity)
jk2coop install --sensitivity 0.7     # base mouse sensitivity for modern mode (default 0.5)
jk2coop install --skip-cutscenes      # auto-skip scripted map-intro cutscenes
jk2coop install --no-skip-cutscenes   # never auto-skip (suppress the prompt)
```

In `modern` mode the install also builds `zz-sensitivity-menu.pk3` so the
CONTROLS slider can reach the lower modern range (retail min is 2). See
[modern-combat.md](modern-combat.md).

### Launch flags

`jk2coop launch` runs the same engine `install` staged, with `fs_basepath`
pointed at the data dir so it picks up the co-op gamecode, the linked retail
assets, and your `autoexec_*.cfg` presets (combat + render). It subsumes the
generated `jk2coop-host` / `jk2coop-join` launcher scripts.

```bash
jk2coop launch                        # single-player, default map (kejim_post), fullscreen
jk2coop launch --map t2_trip          # single-player, a specific map
jk2coop launch --windowed             # run windowed instead of fullscreen
jk2coop launch --skip-cutscenes       # auto-skip scripted map-intro cutscenes this run
jk2coop launch --host                 # host a co-op game on UDP 29070
jk2coop launch --host --port 30000    # host on a specific port
jk2coop launch --join 192.168.1.5     # join (defaults to :29070)
jk2coop launch --join 192.168.1.5:30000
jk2coop launch --print                # print the resolved engine command, don't run it
jk2coop launch -- +set r_mode -2      # pass raw engine args after `--`
```

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

### Graphics features (`jk2coop gfx`)

The graphics work is split into three patch-level features you can turn on or
off independently:

| Feature | Patch | What it does |
| --- | --- | --- |
| `modern-combat` | `0022` | Free aim, faster blaster bolts, fixed screen-center crosshair. |
| `widescreen` | `0023` | 16:9/21:9/32:9 2D aspect correction, extra video modes, edge-anchored HUD. |
| `render-fidelity` | `0024` | Software overbright lighting plus a matching brightness boost for character models. |

Because the patches must apply to a pristine submodule, changing the selection
means resetting OpenJK to the pinned commit and reapplying the co-op base
(patches `0001`–`0021`, always on) plus the chosen features. `jk2coop gfx` does
that for you, then rebuilds the engine and reinstalls it:

```bash
jk2coop gfx                                   # interactive selector (arrow keys, space, enter)
jk2coop gfx --print                           # show which features are currently built in
jk2coop gfx --set widescreen=off              # turn one off non-interactively
jk2coop gfx --set modern-combat=on --set render-fidelity=on
jk2coop gfx --set widescreen=on --no-build    # apply patches only (rebuild yourself later)
jk2coop gfx --set widescreen=on --no-install  # apply + build, but don't restage
```

The interactive selector shows each feature's current state and flags pending
changes; press <kbd>enter</kbd> to apply, <kbd>q</kbd> to cancel. Applying runs
the full pipeline — reset → reapply → `cmake --build` → `jk2coop install` — so
the engine is ready to launch when it finishes. It only restages the engine and
gamecode; the optional asset mods (AI textures, upscale) are left to
`jk2coop install` and are never triggered here.

Most of these features are also live cvars (`r_aspectCorrect2D`,
`cg_hudEdgeAnchor`, `r_overBrightBits*`), so for day-to-day tweaking you can
toggle them from the console without a rebuild — see
[widescreen.md](widescreen.md) and [render-fidelity.md](render-fidelity.md).
Use `jk2coop gfx` when you want a feature's code compiled out entirely (for
example to A/B against stock behavior).

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
  pristine, runs `jk2coop patches apply`, and asserts every `patches/*.patch`
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
- **`patches apply` is not idempotent on a dirty tree** — the patches overlap
  and are cumulative (e.g. 0004 sets the `sv_maxclients` infostring to
  `MAX_CLIENTS` and 0020 later rewrites that same line to honour the runtime
  cvar), so re-running against a fully-patched submodule aborts. Reset first:
  `git -C openjk checkout -- . && git -C openjk clean -fd`.
- **Uninstall never force-removes.** It deletes the files/symlinks it created,
  then only `rmdir`s tracked directories that are now empty (deepest-first), so
  any directory still holding files it did not create is left in place.
