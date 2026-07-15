# The build VM (vee) — what it is and how to manage it

`jk2coop` compiles the Jedi Outcast co-op engine for you. By default it does
this **inside a throwaway virtual machine**, so you never install a C/C++
compiler, the SDL2/OpenAL/zlib/png/jpeg development libraries, or Docker on your
own machine. This document explains what that machinery is, why it exists, and
how to inspect and manage it.

If you would rather build directly on your machine with your own toolchain, skip
all of this and run `jk2coop setup --host` (see
[docs/building.md](building.md)).

## Why a VM at all

The engine is a native C/C++ program. Building it normally means installing
cmake, ninja, a compiler, and a handful of `-dev` libraries — a different set of
packages on every OS and distro, and a real chance of version mismatches. Rather
than ask you to do that, `jk2coop` runs the whole build inside a small,
disposable Linux VM that already has (or installs, once) exactly the right
toolchain. Your machine stays clean; the VM does the dirty work.

The VM is managed by [`vee`](https://github.com/Benehiko/vee), a small tool that
drives QEMU/KVM. `vee` is the **only** thing the default build needs on your
host.

## What `jk2coop setup` does with vee

When you run `jk2coop setup` (with no `--host`), it:

1. **Finds or downloads vee.** If `vee` is already on your `PATH`, that is used.
   Otherwise `jk2coop` downloads a pinned release of vee from
   [GitHub](https://github.com/Benehiko/vee/releases), **verifies its published
   SHA-256 checksum**, and installs it into the jk2coop config directory
   (`~/.config/jk2coop/bin/vee` on Linux;
   `os.UserConfigDir()/jk2coop/bin/vee` in general). The copy is kept there so
   later rebuilds reuse it — nothing is re-downloaded, and the download never
   touches system directories or needs root.

2. **Creates a build VM** (named `jk2coop-docker` for the default container
   build, or `jk2coop-build` for `--vm`) and **shares your patched engine
   source into it over virtiofs**, so the compiled binaries appear back on your
   host automatically — there is no copy-out step.

3. **Builds the engine inside the VM** and installs the result on your host
   (symlinking your retail assets and placing the built gamecode — your game
   data is never copied or modified).

The default path runs the build in a Docker container *inside* the VM (so not
even Docker is needed on the host); `--vm` runs a plain in-VM compile instead.
The mechanics of the container path are documented in
[docs/building.md](building.md#building-in-a-container---docker).

### If vee cannot be obtained

If the automatic download fails (no network, or no prebuilt vee for your
platform) and you have not installed vee yourself, `jk2coop` falls back to a
**host build** when your toolchain is present, or tells you exactly what to
install otherwise. You are never stuck: install vee manually, install the
toolchain and use `--host`, or fix the network and retry.

## Managing the VM: `jk2coop vee`

The build VM is **kept between runs** on purpose — a rebuild (after a graphics
change, or a re-`install`) reuses the warm VM and its cached toolchain instead
of recreating it from scratch. It costs some disk while it sits idle. The
`jk2coop vee` command group lets you see and manage all of this:

```sh
jk2coop vee status        # where vee lives + whether a build VM exists
jk2coop vee download      # download the managed vee copy now (ahead of setup)
jk2coop vee vm delete     # delete the build VM (frees disk; next build recreates it)
```

- **`jk2coop vee status`** prints the resolved vee path (PATH install or the
  managed copy) and whether each build VM is present.
- **`jk2coop vee download`** fetches the managed vee copy without running a
  build. It is a no-op if vee is already available. Use it to pre-stage vee, or
  to re-fetch after deleting it by hand.
- **`jk2coop vee vm delete`** removes the build VM and its disks. This is always
  safe: the next build recreates the VM (only slower, since it re-provisions the
  toolchain). Use it to reclaim disk when you are done building.

`setup` also offers to delete the VM interactively right after a successful
build; answering "no" (the default) keeps it for faster rebuilds.

## Under the hood, at a glance

- **vee binary:** on `PATH`, else `~/.config/jk2coop/bin/vee` (downloaded,
  checksum-verified, pinned).
- **VM names:** `jk2coop-docker` (default container build), `jk2coop-build`
  (`--vm` plain build).
- **Source sharing:** your work dir (`~/.cache/jk2coop`) is shared into the VM
  over virtiofs; build outputs land back on the host with no copy step.
- **Networking:** for the container path, the in-VM Docker API is forwarded to
  `tcp://127.0.0.1:2375` (loopback only) — see the note in
  [docs/building.md](building.md#building-in-a-container---docker).

To remove everything jk2coop installed on the host, use `jk2coop uninstall`. To
remove the downloaded vee and the work dir, delete `~/.config/jk2coop/bin` and
`~/.cache/jk2coop` (and `jk2coop vee vm delete` first to drop the VM).
