# Modern combat feel

Jedi Outcast (2002) was tuned for the mouse-and-monitor hardware of its
day. Three of its combat defaults read as dated on modern high-DPI, high
-refresh-rate mice:

1. **Aim felt sluggish and hyper-sensitive at the same time.** Mouse
   sensitivity was hard-multiplied by `fov_y / 75`. At the game's default
   sub-75 field of view this quietly divided your turn speed, and it also
   made the sensitivity lurch every time the FOV changed (zooming, force
   speed, widescreen FOV). On a modern mouse the net effect is an aim that
   fights you.
2. **Aim "attached" to nearby enemies.** With a saber equipped the game
   auto-aimed — snapping your facing and swing direction onto the closest
   enemy when you stood still or ran straight at them. Modern players
   expect free aim.
3. **The crosshair drifted and lagged behind the view.** The default
   "dynamic" crosshair was traced from the moving weapon muzzle along
   smoothed interpolated angles, so it sat off-center and visibly trailed
   the view when you turned — worse once aim was made snappier. Modern
   shooters use a fixed screen-center crosshair.
4. **Blaster bolts were slow.** Primary-fire projectile velocities were
   low enough that bolts read as lobbed "slugs" you could stroll around,
   rather than the snappy near-hitscan of a modern shooter.

This change modernizes all three. It is engine-side (gamecode +
client-game), shipped as patch
[`patches/0024-modern-combat.patch`](../patches/0024-modern-combat.patch);
no asset or retail-file changes are involved.

## What changed

### 1. Mouse sensitivity decoupled from FOV

`codeJK2/cgame/cg_view.cpp` no longer scales the per-frame sensitivity by
`fov_y / 75` by default. Mouse input is now 1:1 and FOV-independent, the
way modern shooters behave.

A new archived cvar restores the old behavior for anyone who wants it:

| Cvar | Default | Meaning |
|---|---|---|
| `cg_fovSensitivityScale` | `0` | `0` = sensitivity is FOV-independent (modern). `1` = legacy JK2 behavior (sensitivity scales with `fov_y / 75`). |

The force-speed and timescale handling that already modulated turn speed
is unchanged; only the FOV term was made opt-in.

### 2. Saber auto-aim off by default

`g_saberAutoAim` now defaults to `0` (was `1`), so aim is free by
default. The cvar is retained — set `g_saberAutoAim 1` to bring back the
legacy "snap onto the nearest enemy" behavior.

The cvar also lost its `CVAR_CHEAT` flag so it can be toggled at runtime
without enabling cheats. It keeps `CVAR_ARCHIVE`, so your choice
persists.

| Cvar | Default | Meaning |
|---|---|---|
| `g_saberAutoAim` | `0` | `0` = free aim (modern). `1` = auto-aim snaps facing/swings onto the closest enemy when standing still or running forward. |

Guns never had auto-aim in the first place — player blaster/pistol fire
has always used your raw view angles — so this only affects the saber.

### 3. Fixed screen-center crosshair

`cg_dynamicCrosshair` now defaults to `0` (was `1`).

The legacy dynamic crosshair (`1`) was traced from the moving weapon
muzzle along the *interpolated* view angles (`lerpAngles`) rather than
from screen center. That put the crosshair off-center at the barrel and
made it visibly drift and lag behind the view as you turned — the effect
gets more noticeable once aim is snappier (see change 1). The fixed
crosshair (`0`) sits at screen center and tracks your view exactly, the
way modern shooters do.

| Cvar | Default | Meaning |
|---|---|---|
| `cg_dynamicCrosshair` | `0` | `0` = fixed screen-center crosshair (modern, no drift). `1` = legacy dynamic crosshair traced from the moving weapon muzzle, which lags/drifts behind the view. |

Note: the legacy dynamic crosshair was "100% accurate" in the sense that
it showed the muzzle's exact line including barrel parallax. With the
fixed crosshair, at very close range a bolt fired from the offset muzzle
can land slightly off dead-center; at normal engagement distances the two
converge.

### 4. Faster projectiles

Primary blaster-type projectile velocities were roughly doubled for a
snappy, near-hitscan feel. Values are `#define`s in
`codeJK2/game/weapons.h`:

| Weapon | Old velocity | New velocity |
|---|---|---|
| E-11 Blaster | 2300 | 4600 |
| Bryar pistol | 1800 | 3600 |
| Wookiee bowcaster | 1300 | 2600 |
| Heavy repeater (primary) | 1600 | 3200 |
| Heavy repeater (alt) | 1100 | 2200 |
| DEMP2 | 1800 | 3200 |

Deliberately left alone:

- **Rocket launcher** (900) — intentionally slow so its shots can be
  seen, dodged, and pushed.
- **Flechette** (3500) — already fast.
- **Thermal detonators, trip mines, det packs** — thrown/placed
  ordnance, not bolts.
- **AT-ST cannons** and NPC-only weapon defs (e.g. the Mark1 droid's
  local bowcaster define) — vehicle / enemy balance.

Enemy shots stay evadable: NPC blaster velocity is still cut by
`BLASTER_NPC_VEL_CUT` / `BLASTER_NPC_HARD_VEL_CUT`, so with the player at
4600 an ordinary trooper bolt lands around the old player speed.

## Reverting to classic feel

Everything is a cvar or a build-time constant:

```
seta g_saberAutoAim 1          // classic saber auto-aim
seta cg_fovSensitivityScale 1  // classic FOV-linked sensitivity
seta cg_dynamicCrosshair 1     // classic muzzle-traced dynamic crosshair
```

Projectile speeds are compile-time; to restore them, edit the velocities
in `codeJK2/game/weapons.h` (or drop patch `0024` from the stack) and
rebuild.

## Verifying

The change builds into the single-player module `jospgamex86_64.so`
(SP gamecode and client-game are one module in this tree). After
`tools/apply-patches.sh`, rebuild and confirm the cvars are present:

```
ninja -C openjk/build jospgamex86_64
strings openjk/build/codeJK2/game/jospgamex86_64.so | grep -E 'g_saberAutoAim|cg_fovSensitivityScale|cg_dynamicCrosshair'
```
