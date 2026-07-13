# Verifying changes headlessly

This project is a game engine, so most changes are only really verified by
running the game and looking at the result. Two committed harnesses do that
without a physical display, under a virtual framebuffer (Xvfb), so they work
over SSH and in CI-like environments.

Both capture PNG frames with the engine's `screenshot_png` command
(glReadPixels → PNG) and can be analysed with ImageMagick. Prefer these over
ad-hoc `xvfb-run` invocations — they encode the gotchas below that otherwise
cost a lot of trial and error.

## The two harnesses

| Tool | Use it for | Shape |
|---|---|---|
| [`tools/headless-shot.sh`](../tools/headless-shot.sh) | Single-player UI and combat: menus, HUD, crosshair, weapon feel | One engine instance → menu or map → screenshots |
| [`tools/headless-verify.sh`](../tools/headless-verify.sh) | Co-op / dual-load: does a remote client render the host's world and NPCs | Co-op host + remote client under one Xvfb, NPC spawn, frame analysis |

### Single-instance: `headless-shot.sh`

Boots one instance under a **window-manager-less** Xvfb, drives it to a menu or
a map, takes screenshots, and reports whether each PNG is a real rendered frame
or a black one.

```bash
# A menu (use the INTERNAL menu name, e.g. controlsMenu, not "controls"):
tools/headless-shot.sh --menu controlsMenu

# A map, skipping the scripted intro cutscene, with cheats for give/npc:
tools/headless-shot.sh --map kejim_post --skip-cutscenes --cheats \
  --cfg mytest.cfg --shots 3

# Output PNGs + log land in --out (default /tmp/jk2-headless-shot).
```

Key options: `--menu NAME`, `--map NAME`, `--cfg FILE` (exec a cfg after load,
for `give` / `npc spawn` / setting cvars), `--cheats` (`helpUsObi 1`),
`--skip-cutscenes` (`g_skipIntroCinematics 1`), `--shots N`, `--settle N`
(frames to wait before the first shot), `--out DIR`, `--width`/`--height`.

The run prints a per-screenshot verdict:

```
shot2026-...-.png: mean=0.25 stddev=0.29 colors=143651  -> RENDERED
```

`RENDERED` means the frame has real content (mean brightness above black, many
colours); `BLACK/empty` means the run never reached a drawn view — check the log.

### Co-op: `headless-verify.sh`

```bash
tools/headless-verify.sh [map] [port]      # default: kejim_post 29073
```

Starts a co-op host that spawns a few stormtroopers, connects one dual-load
remote client, captures several client frames, and reports whether the client
rendered a 3D view (and, by extension, the host player and NPCs across the
network). See [dual-load-burndown.md](dual-load-burndown.md) and
[m3-remote-model-plan.md](m3-remote-model-plan.md) for how it fits the co-op
work.

## Gotchas these harnesses encode

Learned the hard way; if you hand-roll a headless run, you will hit these:

- **No window manager under Xvfb.** On the real desktop the engine throttles or
  stalls when its window is unfocused, so automated runs stall mid-load. With no
  WM the window is always nominally focused; `com_maxfpsUnfocused 0` keeps the
  loop full speed regardless.
- **`screenshot_png`, not `screenshot`.** The PNG path reads the GL backbuffer
  directly and is reliable headless; the jpeg path is flakier.
- **Menus are addressed by internal name.** The controls page is `controlsMenu`,
  not `controls` — `uimenu controls` prints "Unable to find menu".
- **Some maps open with a long scripted cutscene** (e.g. `kejim_post`) that eats
  `+wait` / `+exec` timing. Use `--skip-cutscenes` to drop into player control.
- **Cheats use `helpUsObi 1`, not `sv_cheats`.** `give`, `npc spawn`, and
  `noclip` are cheat-gated commands.
- **The SP renderer is `rdjosp-vanilla` (REF_API 9).** Do not force
  `cl_renderer rd-vanilla` (that is the MP renderer, API 18) or the engine
  fails with "Couldn't initialize refresh".
- **Xvfb video init is occasionally flaky.** If a run logs "SDL_Init(VIDEO)
  FAILED (did not add any displays)", it is a framebuffer hiccup, not a game
  bug — retry. `headless-shot.sh` uses a fixed display and full teardown to
  minimise this.

## Requirements

- The engine and SP gamecode built (`openjk/build/openjo_sp.x86_64`,
  `jospgamex86_64.so`) — see [building.md](building.md). The harnesses symlink
  the freshly built `.so` into an isolated homepath, so they always test your
  latest build.
- Your retail assets staged (the installers do this) — the harnesses read them
  via `fs_basepath`.
- `Xvfb` for the virtual framebuffer, and ImageMagick (`magick`) for the
  rendered/black analysis (optional; without it you still get the PNGs).
