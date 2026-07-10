# Investigation Log

A record of what was attempted, what was measured, what was concluded, and
where the conclusions were wrong. Kept because several of the wrong turns
cost significant effort and are easy to repeat.

## Starting point

A retail Steam installation of Jedi Outcast: two 32-bit Windows
executables, a few middleware DLLs, and four `.pk3` archives.

Three questions were asked: could the graphics API be mapped and replaced
with Vulkan, could PunkBuster be removed, and could the game be rebuilt
for Linux.

## Findings that closed those questions

**PunkBuster is not present.** Zero string matches in either executable,
no `pb/` directory, no `pbcl.dll` or `pbsv.dll`. It shipped with Jedi
Academy and some later patches, not this build. Nothing to remove.

**The renderer is OpenGL 1.x, loaded dynamically.** 649 `gl*` symbols in
`jk2sp.exe`, with `opengl32.dll` absent from the import table — the engine
resolves entry points by name at runtime, which is stock Quake 3
`QGL_Init` behaviour. `dinput.dll` and `dsound.dll` provide input and
audio. No Direct3D anywhere.

**Decompilation was never necessary.** Raven released the Jedi Outcast
source under the GPL in 2013. The executables are compiled artifacts of
code that can be cloned. The renderer sits behind Quake 3's `refexport_t`
module boundary, so a Vulkan backend is a sibling module rather than an
engine rewrite — Quake3e already has one. Mesa's Zink driver
(`MESA_LOADER_DRIVER_OVERRIDE=zink`) runs the existing renderer on Vulkan
today with no code changes.

The assets are the only irreplaceable part of the installation, and they
are standard formats: JPEG, TGA, MP3, WAV, ZIP.

## Asset inventory

14,978 entries across four archives, roughly 597 MB.

| Type | Count | Notes |
|---|---:|---|
| `.mp3` | 8,126 | Voice and music |
| `.jpg` / `.tga` | 3,200 | Textures |
| `.ibi` | 1,358 | Compiled ICARUS scripts |
| `.wav` | 309 | Sound effects |
| `.md3` | 276 | Static models |
| `.efx` | 255 | Particle effects |
| `.skin` | 105 | Texture-set swaps |
| `.glm` / `.gla` | 71 / 21 | Ghoul2 meshes and skeletons |
| `.bsp` | 45 | The campaign |
| `.shader` | 32 | Materials (text) |
| `.qvm` | 9 | Multiplayer bytecode |

The archives divide by role, not content. `assets0` is the game; `assets1`
is almost entirely audio; `assets2` is a patch that shadows `assets0`;
`assets5` is the multiplayer patch. Load order is lexical, later archives
shadowing earlier ones — which is also the mod mechanism.

## Native Linux build

OpenJK's Jedi Outcast singleplayer targets build cleanly against system
SDL2, OpenAL, zlib, libpng, and libjpeg. Three artifacts:

| Artifact | Role |
|---|---|
| `openjo_sp.x86_64` | Engine |
| `rdjosp-vanilla_x86_64.so` | OpenGL renderer module |
| `jospgamex86_64.so` | Singleplayer gamecode |

The original binaries were 32-bit Windows; these are 64-bit Linux. The
gamecode builds as a standalone shared library and rebuilds in about eight
seconds, which is the feature-development loop.

Two path conventions are easy to get wrong and cost time here: the data
directory is `~/.local/share/openjo/` (Jedi **O**utcast, not `openjk`),
and the gamecode module loads from that directory while the renderer
module loads relative to the executable.

## The co-op investigation

The goal was cooperative campaign play — campaign maps, no cutscenes.

### First conclusion, later overturned

Two routes were compared: widen the singleplayer engine to accept multiple
clients, or host the campaign on the multiplayer tree.

The multiplayer tree turned out to be Jedi Academy's, and it retains
ICARUS, the singleplayer AI roster, Ghoul2, and the saber system. Its
netcode already sits on top of the systems the campaign needs. On that
basis, campaign-on-multiplayer was recommended.

**This recommendation was made from file sizes and structure, without
running either route.** See [route-comparison.md](route-comparison.md) for
why it was reversed.

### What was verified empirically

Campaign maps load on the multiplayer engine. `kejim_post` initialised
under `openjkded`, the server came up with eight client slots, and it ran
indefinitely. Fourteen distinct classnames had no spawn function, dropping
57 entity instances — props, pickups, cameras, `target_autosave`,
`target_secret`. Unknown classnames are non-fatal (`g_spawn.c:700-731`).

Two clients connected, spawned, saw each other, and fought. The server
logged `Kill: 0 620 3: Kyle killed stormtrooper2 (st_guard2) by MOD_SABER`.

### The recurring problem

Jedi Academy's code hardcodes Jedi Academy's asset paths. Jedi Outcast
ships equivalent data under different names. This pattern appeared five
times:

| Jedi Academy expects | Jedi Outcast ships | Fatal | Fix |
|---|---|---|---|
| `ext_data/NPCs/*.npc` | `ext_data/npcs.cfg` | no, silent | repackage as `.pk3` |
| `ext_data/Siege/Classes/*.scl` | nothing | yes | patch: non-fatal |
| `ui/jampmenus.txt` | `ui/jk2mpmenus.txt` | yes | cvar `ui_menuFilesMP` |
| `ui/jahud.txt` | `ui/jk2hud.txt` | yes | cvar `cg_hudFiles` |
| `ui/jamp/menudef.h` | `ui/jk2mp/menudef.h` | no, but drops the client | patch: probe and fall back |

The `menudef.h` case is worth recording. Its symptom is a wall of parse
errors that look cosmetic — `expected integer but found MENU_TRUE`. In
fact, without the global defines every symbolic constant in Jedi Outcast's
`.menu` files becomes an unparseable token, and the failure cascades into
`DROPPED` during the connection handshake.

### Two genuine upstream bugs

**NPC team names were parsed from the key, not the value.**
`codemp/game/NPC_stats.c` formatted the parsed token — the literal string
`"enemyTeam"` — rather than its value:

```c
Com_sprintf(tk, sizeof(tk), "NPC%s", token);
NPC->client->enemyTeam = GetIDForString(TeamTable, tk);
```

`"NPCenemyTeam"` is not in `TeamTable`, so `GetIDForString` returned the
table's `{"", -1}` terminator. Every campaign NPC had `playerTeam` and
`enemyTeam` set to `-1`. The correct function, `TranslateTeamName(value)`,
was present in the same file but commented out, alongside the original
call. Fixed in `patches/0003`.

**Multiplayer never assigns `playerTeam` to human clients.** Singleplayer
does, at `codeJK2/game/g_client.cpp:1625`. The singleplayer AI compares
that field directly against its `enemyTeam`
(`NPC_AI_Stormtrooper.c:782`). Fixed in `patches/0002`.

Both are real defects in OpenJK's multiplayer tree, worth upstreaming.

### The blocker

Jedi Academy's animation enum has 1,534 entries; Jedi Outcast's has 1,202.
They diverge from the first index. Every animation lookup above that point
resolves to the wrong frame range in Jedi Outcast's `.gla` skeletons.

Models render collapsed. NPCs play animations that never reach the frame
events that fire their weapons. This is a data format incompatibility, not
a bug, and it underlies models, combat timing, and saber hit detection.

## Wrong turns, and what they cost

These are recorded because each was expensive and each is easy to repeat.

**Claiming the multiplayer tree had no ICARUS or campaign support.**
Asserted from memory of Quake 3's architecture. `codemp/icarus/` exists and
is wired into the engine at `sv_gameapi.cpp:2866`. Corrected by reading.

**Claiming Jedi Outcast's NPC stats were compiled into the gamecode.**
They are not; they are read from `ext_data/npcs.cfg`, which was in the
archives all along. The claim was made before extracting the file.

**Claiming `ui/jahud.txt` had no Jedi Outcast equivalent.** A subagent
reported this; `ui/jk2hud.txt` was present in the asset listing. Verify
claims against the data, not against the analysis.

**Reading the recommendation off file sizes.** The design document stated
that the multiplayer tree "contains the full singleplayer AI roster,"
citing 23,685 lines across `NPC_AI_*.c`. The files are there. The code is
half-ported, with whole flag behaviours stubbed to `if (0)` behind
`rwwFIXMEFIXME` markers and perception entry points hardcoded to
`g_entities[0]` behind an `OJKFIXME`. Line counts do not establish
behaviour.

**Debugging the AI against an empty world.** Three hypotheses were raised
for why stormtroopers ignored the players — wrong team, ICARUS freeze,
missing script flags — and the source refuted each. The actual state was
that **the clients were spectators**. `sessionTeam` was `3`
(`TEAM_SPECTATOR`); they floated, could not clip, and could not shoot.
`NPC_ValidEnemy` explicitly skips spectators, so the AI was behaving
correctly the entire time.

The tell was in the user's description — floating, no-clip, cannot shoot —
and not in any log. `ClientBegin` fires for spectators too, so the log line
repeatedly cited as proof that "both players are in the game" never
established that. Confirmed by probing `ClientSpawn` for `sessionTeam`, and
resolved with `forceteam 0 free`.

The general lesson: several hours were spent reasoning about code when a
single probe on live state would have settled it. Where a claim is about
runtime behaviour, measure the runtime.

## Current state

Four patches, applied to a pinned OpenJK submodule by
`tools/apply-patches.sh`:

1. Jedi Outcast asset paths in the multiplayer UI
2. Set `playerTeam` for multiplayer clients
3. Parse NPC team names from the value

One tool, `tools/build-coop-npcs-pk3.sh`, which extracts `npcs.cfg` from
the user's own retail installation and repackages it. No proprietary asset
is stored in this repository.

The route has been re-priced and reversed. See
[route-comparison.md](route-comparison.md).
