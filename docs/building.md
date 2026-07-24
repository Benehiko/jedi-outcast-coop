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

`jk2coop setup` does everything the manual sections below describe, in order.

**By default it builds in a container inside a throwaway VM** (via
[`vee`](https://github.com/Benehiko/vee)), so you install **neither a C/C++
toolchain nor Docker** on the host. If `vee` is not already on your `PATH`,
`setup` downloads a pinned, checksum-verified copy into the config dir
(`~/.config/jk2coop/bin`) and keeps it for later rebuilds — so the only true
prerequisite is a network connection the first time. See
[§ Building in a container (`--docker`)](#building-in-a-container---docker)
below, and [build-vm.md](build-vm.md) for the whole vee/VM story and how to
manage it with `jk2coop vee`. Override the default with:

- `--host` — build on this machine (needs the cmake/ninja/compiler toolchain;
  `setup` prints the exact install command if it is missing);
- `--vm` — build in a plain VM (no container) via vee.

When `vee` cannot be obtained (no network, unsupported platform) and is not
installed, `setup` falls back to a host build.

**On macOS the default is a native host build instead.** A Linux container/VM
cannot emit a macOS Mach-O binary (see the target table below), so on a Mac
`setup` skips `vee` entirely and builds with your local cmake/ninja/compiler
toolchain — `--docker` and `--vm` are rejected up front there. Install the
toolchain (`setup` prints the exact command when it is missing) or use the
`jk2coop-macos` CI artifact to skip building locally.

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

## Building in a container (`--docker`)

`jk2coop setup --docker` builds the engine inside a container without installing
**anything** on the host — not a C/C++ toolchain, and not even Docker. It works
by using [`vee`](https://github.com/Benehiko/vee) to run a small Linux VM from
its `docker` template (Alpine + the Docker daemon), then driving that daemon
over the Docker Engine API — vee forwards it to `tcp://127.0.0.1:2375` on the
host, and `jk2coop` talks to it with a tiny built-in HTTP client. The only host
prerequisite is `vee` itself (which brings QEMU/KVM).

```sh
jk2coop setup --docker      # build in a container in a vee VM; host stays clean
```

What happens under the hood:

1. `vee create jk2coop-docker --template docker --virtiofs-dir <source> …` boots
   the VM and shares your (already patched) engine source into it over
   **virtiofs**.
2. Inside the VM, the share is mounted and the Docker daemon is started.
3. `jk2coop` builds a small image (CMake, Ninja, the SDL2/OpenAL/zlib/png/jpeg
   dev libraries, and the mingw-w64 cross toolchain) via the Engine API.
4. A container compiles the engine with the bind-mounted source. Because the
   source is a virtiofs share, the build outputs appear back on the **host**
   automatically — there is no copy-out step.

The VM is kept after a successful build (a re-run reuses the warm VM and its
cached image); `setup` offers to delete it, or run `jk2coop vee vm delete` (or
`vee delete jk2coop-docker`). If `vee` is not already installed, `setup`
downloads a pinned, checksum-verified copy into `~/.config/jk2coop/bin` first.
See [build-vm.md](build-vm.md) for the full vee/VM lifecycle and `jk2coop vee`.

### Target matrix

The build always runs in a Linux container, and produces the binary your host
needs:

| Host OS | Output | How |
|---------|--------|-----|
| Linux   | Linux ELF (`.so`, `openjo_sp.<arch>`) | native compile in the container |
| Windows | Windows PE (`.exe`, `.dll`) | mingw-w64 cross-compile (the `.exe` runs on your Windows host) |
| macOS   | *not supported* | a Linux container cannot emit a macOS Mach-O binary, and Apple's SDK is not redistributable — build on the Mac with `--host` (Xcode), or use the `jk2coop-macos` CI artifact |

> **Note:** the Docker API inside the VM is plaintext and loopback-only (vee
> forwards it to `127.0.0.1` via user-mode NAT). That is fine for a throwaway
> local build VM; do not expose it beyond localhost.

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
