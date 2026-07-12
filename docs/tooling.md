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
(`jk2coop_<version>_<os>_<arch>.tar.gz`, `.zip` for Windows).

## Commands

Every subcommand maps 1:1 to one of the original scripts:

| Command | Replaces | What it does |
| --- | --- | --- |
| `jk2coop patches apply` | `apply-patches.sh` | Applies the patch set to the pinned OpenJK submodule, in order. |
| `jk2coop pk3 coop-ui` | `build-coop-ui-pk3.sh` | Packs `assets/coop-ui/ui` into `zz-coop-ui.pk3`. |
| `jk2coop pk3 coop-npcs <GameData/base>` | `build-coop-npcs-pk3.sh` | Extracts the retail NPC config and repackages it as `zzz-coop-npcs.pk3`. |
| `jk2coop pk3 widescreen` | `build-widescreen-menu-pk3.sh` | Patches the SP video-menu resolution list into `zz-widescreen-menu.pk3`. |
| `jk2coop install` | `install-coop.sh` / `install-coop-macos.sh` / `install-coop.ps1` | Stages the data dir (symlinks + gamecode) and installs the launchers. OS-detected. |
| `jk2coop install --uninstall` | `… --uninstall` | Removes exactly what the install created (manifest-tracked). |
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

## Development

```bash
make hooks   # enable the pre-commit hook (format check + lint + build) for this clone
make fmt     # apply gofumpt + goimports
make lint    # format check + golangci-lint (mirrors CI)
make test    # go test -race ./...
```

CI runs the same checks in `.github/workflows/go.yml`, plus a cross-compile of
the darwin/windows targets. Tagged `v*` pushes trigger
`.github/workflows/release.yml`, which builds every platform and attaches the
archives to a GitHub Release.

## Design notes

- **Paks are built with Go's `archive/zip`** — no `zip`/`unzip` shell-out.
  Entries are written in sorted order with a fixed compression method, so a
  rebuild from the same inputs is byte-identical.
- **The widescreen menu patch operates on raw bytes**, preserving the retail
  menu files' CRLF line endings and latin-1 encoding, and refuses to touch a
  file whose resolution list is not in the exact stock form (already patched or
  a different edition).
- **`patches apply` is not idempotent on a dirty tree** — the patches overlap
  and are cumulative (e.g. 0004 sets `MAX_CLIENTS` to 2 and 0020 changes it to
  4), so re-running against a fully-patched submodule aborts. Reset first:
  `git -C openjk checkout -- . && git -C openjk clean -fd`.
- **Uninstall never force-removes.** It deletes the files/symlinks it created,
  then only `rmdir`s tracked directories that are now empty (deepest-first), so
  any directory still holding files it did not create is left in place.
