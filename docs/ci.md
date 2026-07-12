# Continuous integration

GitHub Actions gate every push to `main` and every pull request. There are two
workflows; all jobs must pass before a change merges.

## `build.yml` — compile the engine

Builds the JK2 SP/co-op engine, gamecode, and renderer from the pinned OpenJK
submodule with this repo's patches applied.

| Job leg | Runner | Required | What it does |
|---|---|---|---|
| `linux` | `ubuntu-latest` | yes | apt deps → `apply-patches.sh` → CMake/Ninja build → **artifact sanity check** → upload `jk2coop-linux` |
| `windows` | `windows-latest` | no (experimental) | MSVC build → upload `jk2coop-windows` (with `SDL2.dll`). Allowed to fail while the Windows leg is stabilised. |

**Artifact sanity check** (Linux): CI has no proprietary game assets, so it
cannot run a map. Instead it proves the three built artifacts are well-formed
and export the entry points the engine `dlopen()`s — the renderer must export
`GetRefAPI`, the gamecode must export `GetGameAPI` and `vmMain`, and the engine
must be an ELF64 binary. This catches link-level breakage that a bare compile
would not.

## `lint.yml` — fast checks, no compiler

| Job | What it does |
|---|---|
| `shellcheck` | Runs ShellCheck (`-S warning`) over `tools/*.sh`. Repo `.shellcheckrc` disables `SC2034` for the intentional unused-counter loop pattern. |
| `patches-apply` | Resets the OpenJK submodule to pristine and runs `tools/apply-patches.sh`, asserting the whole patch set applies cleanly and in order. |

The `patches-apply` job guards the project's core invariant. This repo carries
its engine changes as a stack of cumulative, overlapping patches rather than a
fork, so a drifted or broken patch is the single most likely way to break the
build. Catching it here — independently of the slower compile — keeps "the
patches apply" and "CI is green" in lockstep.

## Running the same checks locally

```sh
# shellcheck (containerised — no host install)
docker run --rm -v "$PWD:/mnt" -w /mnt koalaman/shellcheck:stable -S warning tools/*.sh

# patch idempotency (the exact CI step)
git -C openjk checkout -- . && git -C openjk clean -fdq
tools/apply-patches.sh

# artifact sanity, after a Linux build
nm -D openjk/build/code/rd-vanilla/rdjosp-vanilla_x86_64.so | grep -qE ' T GetRefAPI$'  && echo ok
nm -D openjk/build/codeJK2/game/jospgamex86_64.so           | grep -qE ' T GetGameAPI$' && echo ok
```
