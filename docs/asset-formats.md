# Jedi Outcast asset formats

Reference for the file types used by *Star Wars Jedi Knight II: Jedi Outcast*
(and the OpenJK engine this project builds), plus how to open each one — in
particular in Blender.

> **This project ships no game data.** These formats live inside the retail
> `assets*.pk3` files from your own legal copy of the game. Nothing here
> distributes or modifies that data; this is a guide to inspecting and editing
> your own files. Extracting and editing game assets is for your own use — check
> the game's EULA before redistributing anything derived from them.

## The container: `.pk3`

Everything ships inside `assetsN.pk3` files (`assets0.pk3`, `assets1.pk3`, …)
in the game's `base/` directory.

- A `.pk3` is **a plain ZIP archive**. Rename a copy to `.zip` and any archive
  tool will open it — no special software needed.
- The engine loads pk3s in alphabetical order; a later file overrides an earlier
  one. This is why override paks (this project's `zz-` / `zzz-` files) work
  without touching the originals.

```
base/
├── assets0.pk3        # models, code, effects
├── assets1.pk3        # textures, shaders, UI menus
├── assets2.pk3
└── assets3.pk3
```

Inside, assets are laid out by type: `models/`, `textures/`, `gfx/`, `sound/`,
`music/`, `ui/`, `scripts/`, `maps/`.

## Asset types

| Extension | What it is | Text or binary |
|---|---|---|
| `.md3` | Static / world model — weapons, items, map props (Quake III format) | Binary |
| `.glm` | Ghoul2 skeletal mesh — player and NPC character models | Binary |
| `.gla` | Ghoul2 skeleton + animation data (a `.glm` references its `.gla`) | Binary |
| `.bsp` | Compiled level geometry (a level's playable map) | Binary |
| `.map` | **Source** level file that compiles to `.bsp` (edited in Radiant) | Text |
| `.jpg`, `.tga` | Texture images | Binary |
| `.shader` | Material definitions — which textures, blending, scroll, glow, etc. | Text |
| `.efx` | Particle / effect scripts | Text |
| `.roq` | Pre-rendered video cutscenes | Binary |
| `.mp3`, `.wav` | Music and sound effects | Binary |
| `.menu` | Data-driven UI menu layouts (`ui/`) | Text |
| `.sab`, `.npc`, `.veh` | Sabers, NPC stats, vehicle definitions (`ext_data/`) | Text |

### 3D models — the Ghoul2 system

Characters use **Ghoul2**, id Tech 3's skeletal-animation format, split across
two files:

- **`.gla`** holds the skeleton and all animation frames. Multiple models can
  share one `.gla` (all humanoids share `_humanoid.gla`).
- **`.glm`** holds the mesh and its surfaces, and points at a `.gla` for its
  skeleton. A `.glm` is useless without its `.gla`.

World geometry decoration and hand-held items are **`.md3`** — simpler,
per-vertex animated, no skeleton.

## Opening in Blender

| Format | Blender support | How |
|---|---|---|
| `.md3` | Yes | The [blender-md3](https://github.com/neumond/blender-md3) importer, or the SoY/Nostalrius MD3 add-on. |
| `.glm` / `.gla` | Yes, with an add-on | **Mr. Wonko's "Jedi Academy" import/export add-on** (works for JK2's Ghoul2 too). Import the **`.gla` first** (skeleton), then the `.glm` (mesh) so it binds to the skeleton. |
| `.bsp` | Not directly | Blender has no native BSP importer for id Tech 3. Decompile to `.map` with `q3map2 -convert`, or edit the `.map` source in **GtkRadiant / NetRadiant**, not Blender. |
| `.jpg`, `.tga` | Yes | Any image tool. `.tga` (Targa) is natively supported by Blender and GIMP. |

> Note: JK2 does **not** use `.md5` (that is a Doom 3 format). If a tutorial
> mentions md5, it does not apply here — JK2 characters are `.glm` + `.gla`.

### Character workflow (step by step)

1. Copy `assets0.pk3` out of `base/`, rename the copy to `.zip`, extract it.
2. Find the character under `models/players/<name>/` — you'll see `model.glm`
   plus textures (`.jpg` / `.tga`) and a `.skin` file mapping surfaces to
   textures.
3. The animation skeleton is shared: `models/players/_humanoid/_humanoid.gla`.
4. In Blender, with Mr. Wonko's add-on installed:
   - **File → Import → JA `.gla`** — pick `_humanoid.gla`.
   - **File → Import → JA `.glm`** — pick `model.glm`; it binds to the imported
     skeleton.
5. Edit, then export back with the same add-on. Keep the original paths and
   file extensions so the engine (or an override pak) finds the result.

## See also

- [docs/hires-textures.md](hires-textures.md) — reading textures out of the
  retail paks and building a high-resolution override pak.
- [docs/asset-generation.md](asset-generation.md) — the local texture-generation
  tool and how override paks are assembled.
