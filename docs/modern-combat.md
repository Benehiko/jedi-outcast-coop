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
5. **The blaster sprayed and hit soft.** Every player primary bolt was
   nudged inside a random ±0.5° cone, so shots wandered off the fixed
   crosshair; and bolts only shoved a target when they *killed* it, so
   connecting shots on a standing enemy applied no knockback and felt
   weak — made worse by the low 20-per-bolt damage.

This change modernizes those, and adds an opt-in to auto-skip the
scripted map-intro cutscenes. It is engine-side (gamecode + client-game),
shipped as patches
[`0022-modern-combat.patch`](../patches/0022-modern-combat.patch) (changes
1–4 and the cutscene skip) and
[`0026-blaster-combat-feel.patch`](../patches/0026-blaster-combat-feel.patch)
(change 5); no asset or retail-file changes are involved.

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

This is separate from the engine's base `sensitivity` cvar, which is the
raw multiplier on mouse movement. Its stock default (`5`) is fast on a
modern high-DPI mouse, so the install sets a calmer `0.5` by default
(tunable — see [Settings](#settings)). You can change it any time with
`sensitivity <n>` in the console, or set `[game] sensitivity` in the
config file.

The CONTROLS menu slider is also rescaled. Retail defines it as
`cvarfloat "sensitivity" 5 2 30` — default 5, **min 2**, max 30 — so the
UI could not even reach the new low values. In modern mode the installers
build a `zz-sensitivity-menu.pk3` override (via
`tools/build-sensitivity-menu-pk3.sh`, from your own retail menus) that
rescales it to `0.5 0.1 2` (default 0.5, min 0.1, max 2) and adds a small
value readout next to the slider — an editfield bound to `sensitivity`, so
it live-updates as you drag and you can click it to type an exact value.
The JK2 menu slider is a continuous drag with no discrete step, so the
small range gives roughly 0.1 granularity across the bar rather than a hard
snap; `sensitivity <n>` in the console still sets any exact value. Like the
widescreen menu mod, retail assets are never modified — remove it by
deleting the one pak.

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

### 5. Sharper, weightier blaster (accuracy, knockback, damage)

Two things made the E-11 blaster feel bad to use: shots wandered off the
crosshair, and the ones that connected did almost nothing to a standing
enemy. Both were baked into the stock behavior; patch
[`0026-blaster-combat-feel`](../patches/0026-blaster-combat-feel.patch) makes
each one a cvar and ships modern defaults.

**Accuracy.** Stock JK2 sprayed *every* player primary bolt inside a random
±0.5° cone (`BLASTER_MAIN_SPREAD`), so shots drifted off the fixed
screen-center crosshair and the gun read as inaccurate. `g_blasterSpread` is
that spread half-angle in degrees; the default `0` makes the player's primary
pinpoint — the bolt goes exactly where you aim. This affects **player primary
fire only**: NPC troopers keep their own aim-skill spread, and alt-fire keeps
its wider `BLASTER_ALT_SPREAD`.

**Knockback (the "weak hit" fix).** Stock blaster bolts carried the
`DAMAGE_DEATH_KNOCKBACK` flag, which means a hit only shoves the target when
it *kills* — a living enemy took zero push, so connecting shots felt like they
did nothing. `g_blasterKnockback 1` (default) drops that death-only flag for
the player's primary fire, so every hit runs the normal live-target knockback
path: a small, mass-scaled shove that reads as impact. Set it to `0` to
restore the stock feel (push on death only). NPCs are unaffected.

**Damage.** The stock 20/bolt takes many hits to drop an enemy, which adds to
the weak feel. `g_blasterDamage` overrides the player primary damage per bolt;
the default `40` doubles it so hits land with weight without one-shotting.
Set it to `0` to defer to the game's own value (retail 20, or whatever a
`.wpn` file loads). Player primary only — NPC and alt-fire damage are
untouched.

| Cvar | Default | Meaning |
|---|---|---|
| `g_blasterSpread` | `0` | Player primary spread half-angle in degrees. `0` = pinpoint (modern); `0.5` = retail spray. |
| `g_blasterKnockback` | `1` | `1` = player primary hits shove living targets (modern). `0` = knockback on death only (retail). |
| `g_blasterDamage` | `0`/`40` | Player primary damage per bolt. `0` = use the loaded `.wpn` value (retail 20). The install writes `40` by default. |

Like `g_blasterVelocity` (change 4), these are runtime archived cvars backed
by an always-applied co-op patch, so a normal `jk2coop install` builds them in
and they are tunable from the config with no rebuild.

### 6. Optional cutscene auto-skip

`g_skipIntroCinematics` (new cvar, default `0`).

JK2 plays scripted in-engine cutscenes (ICARUS camera sequences) at the
start of many campaign maps. They can always be skipped by pressing *use*,
which fast-forwards the sequence (the game bumps `timescale` to 100 until
the camera ends). With `g_skipIntroCinematics 1`, that same skip fires
automatically the moment a cutscene starts — so maps drop you straight
into player control without a keypress.

Default is `0` (cutscenes play), so the campaign's storytelling is intact
out of the box. Set to `1` if you want to blow past every scripted intro
(handy for replays and for automated testing).

| Cvar | Default | Meaning |
|---|---|---|
| `g_skipIntroCinematics` | `0` | `0` = scripted map cutscenes play. `1` = auto-fast-forward them into player control. |

Implementation: `codeJK2/game/g_active.cpp`, in the in-camera branch of
`ClientThink_real` — when the cvar is on and we are not already skipping,
it calls the same `G_StartCinematicSkip()` the *use* button triggers.

## Settings

Combat feel is configured in the config file at
`~/.config/jk2coop/config.toml` (macOS `~/Library/Application
Support/jk2coop/config.toml`, Windows `%AppData%\jk2coop\config.toml`),
under the `[game]` block. Edit it with the Game Settings TUI:

```sh
jk2coop game        # mouse sensitivity, blaster speed, aim assist, dynamic crosshair, skip cutscenes
```

`jk2coop install` and `jk2coop launch` write these choices into
`base/autoexec_sp.cfg` (the engine execs it at startup, after
`openjo_sp.cfg`, so it overrides a stale config that may have persisted the
old values). The `[game]` fields map to the cvars above:

| Config field | Cvar(s) | Effect |
|---|---|---|
| `sensitivity` | `sensitivity` | Base mouse sensitivity (default `0.5`; JK2 stock is `5`, fast on a high-DPI mouse) |
| `blaster_velocity` | `g_blasterVelocity` | Primary blaster bolt speed (retail `2300`) |
| `blaster_spread` | `g_blasterSpread` | Player primary spread half-angle in degrees (default `0` = pinpoint; retail `0.5`) |
| `blaster_knockback` | `g_blasterKnockback` | `true` = player primary hits shove living targets (default); `false` = retail (push on kill only) |
| `blaster_damage` | `g_blasterDamage` | Player primary damage per bolt (default `40`; `0` = use the game's own value, retail `20`) |
| `aim_assist` | `g_saberAutoAim`, `cg_fovSensitivityScale` | `true` restores legacy saber auto-aim and FOV-linked sensitivity |
| `dynamic_crosshair` | `cg_dynamicCrosshair` | `true` restores the legacy muzzle-traced dynamic crosshair |
| `skip_cutscenes` | `g_skipIntroCinematics` | `true` auto-skips scripted map-intro cutscenes |

The modern defaults (`aim_assist = false`, `dynamic_crosshair = false`,
`skip_cutscenes = false`, `sensitivity = 0.5`) give free aim, a fixed
crosshair, FOV-independent sensitivity, and fast bolts. Set `aim_assist`
and `dynamic_crosshair` to `true` for the legacy feel.

The install also builds `zz-sensitivity-menu.pk3` so the CONTROLS slider
can reach the lower modern range (retail min is 2).

> **Blaster speed cvar.** `blaster_velocity` is backed by patch
> `0025-blaster-velocity`, which turns the compile-time `BLASTER_VELOCITY`
> into the archived `g_blasterVelocity` cvar. That patch is part of the
> always-applied co-op base, so a normal `jk2coop install` builds it in and
> the value is tunable from the config with no rebuild.

## Reverting to classic feel

Everything is a cvar or a build-time constant:

```
seta g_saberAutoAim 1          // classic saber auto-aim
seta cg_fovSensitivityScale 1  // classic FOV-linked sensitivity
seta cg_dynamicCrosshair 1     // classic muzzle-traced dynamic crosshair
seta g_skipIntroCinematics 0   // let map-intro cutscenes play (default)
seta g_blasterSpread 0.5       // classic player blaster spray
seta g_blasterKnockback 0      // classic knockback (on kill only)
seta g_blasterDamage 20        // classic blaster damage per bolt
```

Projectile speeds are compile-time; to restore them, edit the velocities
in `codeJK2/game/weapons.h` (or drop patch `0022` from the stack) and
rebuild.

## Verifying

The change builds into the single-player module `jospgamex86_64.so`
(SP gamecode and client-game are one module in this tree). After
`tools/apply-patches.sh`, rebuild and confirm the cvars are present:

```
ninja -C openjk/build jospgamex86_64
strings openjk/build/codeJK2/game/jospgamex86_64.so | grep -E 'g_saberAutoAim|cg_fovSensitivityScale|cg_dynamicCrosshair|g_skipIntroCinematics|g_blasterSpread|g_blasterKnockback|g_blasterDamage'
```

To confirm the blaster cvars register at runtime with their defaults, drive
one headless instance into a map (they are game-init cvars, so they appear only
after a map loads, not in the main menu) and dump them:

```
tools/headless-shot.sh --map kejim_post --skip-cutscenes --cheats \
  --cfg your-dump.cfg   # a cfg containing: cvarlist g_blaster
```
