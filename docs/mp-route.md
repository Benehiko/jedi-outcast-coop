# The multiplayer-tree route (superseded)

**Route reversed.** See [route-comparison.md](route-comparison.md).
Hosting the campaign on the Jedi Academy multiplayer tree works well
enough to play — two clients connect, spawn, see each other, and fight —
but that engine's animation enum has 1,534 entries against Jedi Outcast's
1,202, diverging from index 1. Jedi Outcast's models render collapsed and
its NPCs never complete their attack animations. The project widened the
singleplayer engine instead.

[investigation-log.md](investigation-log.md) records everything tried,
measured, and concluded, including the wrong turns.

The multiplayer branch below still runs, and produced two upstream bug
fixes worth keeping. This page preserves how to use it.

## Building and running the MP tree

Build the multiplayer targets by inverting the `BuildMP*` / `BuildJK2SP*`
flags from [building.md](building.md). Note the two trees use *different*
data directories:

| Tree | Data directory | Gamecode module |
|---|---|---|
| Jedi Outcast singleplayer | `~/.local/share/openjo/base/` | `jospgamex86_64.so` |
| Multiplayer | `~/.local/share/openjk/base/` | `jampgamex86_64.so`, `cgamex86_64.so`, `uix86_64.so` |

Jedi Academy's multiplayer code hardcodes Jedi Academy's asset paths.
Jedi Outcast ships the same data under different names, so three of the
four fixes are configuration and one is a patch.

Apply the source patches to the pinned submodule, then rebuild:

```sh
tools/apply-patches.sh
cmake --build openjk/build
```

Generate the NPC compatibility archive from your own retail installation:

```sh
tools/build-coop-npcs-pk3.sh "<steam>/Jedi Outcast/GameData/base"
cp zzz-coop-npcs.pk3 ~/.local/share/openjk/base/
```

Start a dedicated server on a campaign map:

```sh
cd openjk/build
./openjkded.x86_64 +set dedicated 1 +set sv_pure 0 \
    +set net_port 29070 +set sv_maxclients 8 +map kejim_post
```

Connect a client. The two cvars point the menu and HUD loaders at Jedi
Outcast's files rather than Jedi Academy's:

```sh
./openjk.x86_64 +set sv_pure 0 \
    +set ui_menuFilesMP "ui/jk2mpmenus.txt" \
    +set cg_hudFiles "ui/jk2hud.txt" \
    +set name Kyle +connect 127.0.0.1:29070
```

Run a second client with a different `fs_homepath` to play locally:

```sh
./openjk.x86_64 +set fs_homepath /tmp/jk2-client2 ... +set name Jan ...
```

Clients connect as **spectators** — floating, no clipping, unable to
shoot. `ClientBegin` fires for spectators, so a connected client in the
server log has not necessarily joined the game. Put them on the free team
from the server console:

```
forceteam 0 free
forceteam 1 free
```

Campaign NPCs skip spectators (`NPC_ValidEnemy`), so until a client joins
a team the stormtroopers will correctly ignore it.

## Asset path differences

| Jedi Academy expects | Jedi Outcast ships | Resolved by |
|---|---|---|
| `ext_data/Siege/Classes/*.scl` | nothing (Siege is Academy-only) | patch: non-fatal |
| `ui/jampmenus.txt` | `ui/jk2mpmenus.txt` | cvar `ui_menuFilesMP` |
| `ui/jahud.txt` | `ui/jk2hud.txt` | cvar `cg_hudFiles` |
| `ui/jamp/menudef.h` | `ui/jk2mp/menudef.h` | patch: probe and fall back |
| `ext_data/NPCs/*.npc` | `ext_data/npcs.cfg` | `build-coop-npcs-pk3.sh` |

The `menudef.h` case is not cosmetic. Without those defines every
symbolic constant in Jedi Outcast's `.menu` files fails to parse, and the
client is dropped rather than merely rendering an ugly menu.
