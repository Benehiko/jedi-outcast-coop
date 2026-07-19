# Embedded engine source

`jk2coop` builds the co-op engine from OpenJK source. As of the embedded-source
change, a **released `jk2coop` binary carries that source inside itself** — a
pruned copy of the pinned OpenJK tree, the co-op patch set, and the co-op UI
assets are all baked into the executable with Go's `embed`. This means a
downloaded binary can go from nothing to a playable game with **no git, no
network, and no repository checkout**: it extracts the source into a work
directory, patches it in pure Go, builds it, and installs.

The git submodule and the `tools/*.sh` scripts are still the source of truth for
**developing** the patches; they are not needed to *use* a released binary.

## Two flows

| | Standalone (default) | Dev (`--repo`) |
|---|---|---|
| Source | embedded archive → work dir | `openjk/` git submodule |
| Patching | pure Go (`internal/patchengine`) | `git apply` |
| Pristine reset | re-extract the embedded archive | `git checkout` / `git clean` |
| Feature detection | work-dir manifest | `git apply --reverse --check` |
| Needs git / repo | no | yes |

`jk2coop setup`, `install`, and `launch` use the standalone flow by default. Pass
`--repo <path>` (or run from inside a checkout, for `install`/`launch`) to use
the dev flow against the submodule.

## Work directory

The standalone flow extracts and builds under a work directory, resolved in this
order:

1. `$JK2COOP_HOME`
2. `$XDG_CACHE_HOME/jk2coop`
3. `~/.cache/jk2coop`

Layout:

```
~/.cache/jk2coop/
  src/            extracted + patched OpenJK source
  src/build/      CMake build output (engine, renderer, gamecode)
  coop-ui/        extracted co-op UI assets (source for zz-coop-ui.pk3)
  manifest.json   { pin, gfx } describing the current src/ state
```

`manifest.json` records the embedded OpenJK commit the tree was extracted from
and which graphics features are patched in. On a re-run, `jk2coop` re-extracts
and re-patches only when the pin or the graphics selection changed — the
standalone analogue of the git submodule reset the dev flow performs.

## What is embedded (the prune)

Embedding the whole 179 MB submodule would bloat the binary, so only what the
JK2-SP build actually needs is embedded. The build compiles three targets —
the JK2-SP engine (`openjo_sp`), gamecode (`jospgame…`), and renderer
(`rdjosp-vanilla…`) — with every non-JK2-SP CMake target off (see
[building.md](building.md) for the exact flags).

**Kept** (≈46 MB uncompressed, ≈13 MB gzipped in the binary):

- `CMakeLists.txt`, `LICENSE.txt`, `README.md` — required at CMake configure
  time (the last two are handed to CPack, which errors if they are absent).
- `cmake/` — configure-time modules (`InstallConfig.cmake`).
- `code/`, `codeJK2/`, `shared/` — the compiled source.
- `lib/minizip/` (compiled) and `lib/gsl-lite/` (header-only, `#include`d).
- `codemp/` — **not** compiled by the JK2-SP build, but kept so the full patch
  set (patches 0001–0003 edit MP-tree files) applies to a real tree. Those
  patches are MP-tree defect fixes carried for upstreaming; they have no effect
  on the shipped SP engine.

**Pruned**: `tests/`, `tools/`, `scripts/`, `docs/`, the top-level `ui/`,
`build/`, `.git/`, and the bundled `lib/jpeg-9a`, `lib/zlib`, `lib/libpng`,
`lib/SDL2` (Linux links the system libraries; those bundled copies only compile
on Windows or with `-DUseInternalLibs=ON`).

The prune was verified by configuring and building the extracted+patched tree in
a clean container — all three targets compile with no missing-include errors.

## Regenerating the embed

The embedded assets live under `internal/embed/` and are generated from the
submodule:

```sh
make generate      # go generate ./internal/embed
```

This re-tars the pruned source at the submodule's current `HEAD` (via
`git archive`, so only tracked files at the pin, never working-tree dirt),
records the commit in `internal/embed/pin.txt`, and mirrors `patches/` and
`assets/coop-ui/` into the package.

**After adding a patch to `patches/` (or bumping the OpenJK submodule, or
editing `assets/coop-ui/`), run `make generate` and commit the result** —
including any *new* files it writes under `internal/embed/patches/`. The engine
binary embeds the mirror (`//go:embed patches/*.patch` in `embed.go`), not the
repo-root `patches/`, so a mirror file that is never committed is simply absent
from every binary built from the embedded source (the default `jk2coop setup`).

CI enforces this: the `embed-in-sync` job runs `make verify-embed`, which
regenerates the embed and fails if it produces any diff — so the baked-in source
can never silently drift from the pin. The gzip stream is produced by the Go
toolchain (pinned via `go-version-file` in CI), so the archive is
byte-reproducible across machines running the same Go version.

> **Guard scope (learned the hard way).** `verify-embed` originally checked only
> `git diff`, which does not report *untracked* files. A newly generated mirror
> patch is untracked, so a "forgot to commit the mirror" passed CI green while
> the binary shipped an incomplete patch set — patches 0027–0030 (the co-op
> sync/animation/weapon fixes) drifted out of the mirror across four merged PRs
> this way. `verify-embed` now also fails on any untracked file under
> `internal/embed/`, closing that hole.

## Patch application in pure Go

`internal/patchengine` applies the git-format patches without shelling out to
git. It parses each patch with `sourcegraph/go-diff` and applies the hunks by
locating each hunk's context in the source (matching `git apply`'s whole-hunk
offset tolerance — the patch headers' line numbers have drifted from the pinned
commit, so a strict positional apply would wrongly reject them). It supports
in-place modification and new-file creation; deletions, renames, mode changes,
and binary patches are rejected loudly (the co-op set contains none).

A golden test (`internal/patchengine`) applies the full patch set both with
`git apply` and with the pure-Go engine to a pristine tree and asserts the two
results are byte-identical, so the engine stays faithful to git's behaviour.
```
