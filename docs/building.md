# Building

## Quick start (one command)

Most people should not run the manual steps below. A pre-built `jk2coop`
binary embeds the engine source, so setup needs no clone and no submodule:

```sh
./jk2coop setup       # extract embedded source + patches + engine build + install
```

Or build `jk2coop` from a clone first:

```sh
git clone --recurse-submodules <repo>
cd jedi-outcast-coop
make build            # produces ./jk2coop
./jk2coop setup       # same guided setup, from the embedded source
```

`jk2coop setup` does everything the manual sections below describe, in order,
and stops with a clear, copy-paste install command if a build tool is missing.
If you have [`vee`](https://github.com/Benehiko/vee) it can also build the
engine inside a disposable VM, so you never install a C/C++ toolchain on the
host — `setup` prompts for this (or force it with `--vm` / `--host`).

By default `setup` builds from the **source embedded in the binary**, extracted
to `~/.cache/jk2coop` — see [embedded-source.md](embedded-source.md). The manual
sections below operate on the `openjk/` submodule directly and are the reference
for **patch development**; to make `setup` use the submodule instead of the
embedded source, pass `--repo .` from inside a checkout.

## Repository layout

- `openjk/` — pinned OpenJK submodule (upstream source; co-op changes are
  applied to it as patches, never committed to it)
- `openjk/build/` — build output (gitignored)
- `patches/` — this project's source changes, one cumulative diff per file
  set, applied by `tools/apply-patches.sh`
- `assets/coop-ui/` — the original Co-op menu overlay, packed into
  `zz-coop-ui.pk3` by `tools/build-coop-ui-pk3.sh`
- `tools/` — installers and helper scripts
- `docs/` — documentation

## Linux

Requires: cmake, ninja, gcc, SDL2, OpenAL, zlib, libpng, libjpeg.

```sh
git clone --recurse-submodules <repo>
cd jedi-outcast-rebuild
tools/apply-patches.sh              # apply the co-op patches to the submodule
```

Or, using the cross-platform `jk2coop` Go binary (equivalent; see
[tooling.md](tooling.md)):

```sh
go build -mod=vendor -o jk2coop .
./jk2coop dev patches apply
```

The patches are cumulative and overlap (several touch the same lines — e.g.
one patch sets the `sv_maxclients` infostring to `MAX_CLIENTS` and a later one
rewrites that same line to honour the runtime cvar). They apply
cleanly in order to a **pristine** submodule, but `apply-patches.sh` is not
idempotent on a dirty tree: re-running it against an already-patched submodule
can fail on an overlapping patch. To re-apply, reset the submodule first:

```sh
git -C openjk checkout -- . && git -C openjk clean -fd
tools/apply-patches.sh
```

Continuing the build:

```sh

cmake -S openjk -B openjk/build -G Ninja \
  -DCMAKE_BUILD_TYPE=RelWithDebInfo \
  -DBuildJK2SPEngine=ON -DBuildJK2SPGame=ON -DBuildJK2SPRdVanilla=ON \
  -DBuildSPEngine=OFF -DBuildSPGame=OFF -DBuildSPRdVanilla=OFF \
  -DBuildMPEngine=OFF -DBuildMPRdVanilla=OFF -DBuildMPDed=OFF \
  -DBuildMPGame=OFF -DBuildMPCGame=OFF -DBuildMPUI=OFF -DBuildMPRend2=OFF
cmake --build openjk/build
```

Produces three artifacts:

| Artifact | Role |
|---|---|
| `openjo_sp.x86_64` | Engine |
| `code/rd-vanilla/rdjosp-vanilla_x86_64.so` | OpenGL renderer module |
| `codeJK2/game/jospgamex86_64.so` | Singleplayer gamecode |

macOS and Windows build with the same `-DBuildJK2SP*` options; see the
[macOS](install-macos.md) and [Windows](install-windows.md) install guides
for toolchain specifics and artifact names.

## Running without the installer

The engine reads assets and modules from `~/.local/share/openjo/base/`
(note: `openjo`, not `openjk` — this is the Jedi Outcast target).
Symlink the retail assets and the freshly built gamecode into place:

```sh
mkdir -p ~/.local/share/openjo/base
ln -sfn "<steam>/Jedi Outcast/GameData/base/"assets*.pk3 ~/.local/share/openjo/base/
ln -sfn "$PWD/openjk/build/codeJK2/game/jospgamex86_64.so" ~/.local/share/openjo/base/
# the renderer module is loaded relative to the executable:
ln -sfn "$PWD/openjk/build/code/rd-vanilla/rdjosp-vanilla_x86_64.so" openjk/build/
cd openjk/build && ./openjo_sp.x86_64 +map kejim_post
```

`tools/install-coop.sh` automates all of this — see
[install-linux.md](install-linux.md).

## Debug builds

`RelWithDebInfo` and `Release` both define `NDEBUG`, which compiles out
`assert()`. The singleplayer save code carries assertions that Raven left
as deliberate tripwires for exactly the change this project is making, so
test anything touching saves against a `Debug` tree:

```sh
cmake -S openjk -B openjk/build-debug -G Ninja -DCMAKE_BUILD_TYPE=Debug \
  -DBuildJK2SPEngine=ON -DBuildJK2SPGame=ON -DBuildJK2SPRdVanilla=ON
cmake --build openjk/build-debug
```

Verify the assertions are live before trusting a passing test:

```sh
nm -u openjk/build-debug/codeJK2/game/jospgamex86_64.so | grep assert
```

## Development loop

Gameplay code lives in `openjk/codeJK2/game/` and builds as a standalone
shared library. Because the gamecode is symlinked into the engine's
search path, rebuilding that one target is sufficient — no reinstall:

```sh
cmake --build openjk/build --target jospgamex86_64
```

Relaunch to pick up the change.

Every change should end with the loopback regression:

```sh
cd openjk/build && ./openjo_sp.x86_64 +map kejim_post   # exit 0, no errors
```
