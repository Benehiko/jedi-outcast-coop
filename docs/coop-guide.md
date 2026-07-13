# Playing co-op

How to host, find, and join cooperative games once the game is
[installed](install-linux.md). Up to **four players** per game
(`sv_maxclients`, default 2).

## From the `jk2coop` binary

```sh
jk2coop launch                       # play; hosts a co-op game on UDP 29070 by default
jk2coop host                         # explicitly host a co-op game
jk2coop join <host[:port]>           # join a co-op game by IP
jk2coop launch --join <host[:port]>  # same as `join`
jk2coop launch --map <map>           # host a specific map
```

## From the launcher scripts

The shell/PowerShell installers also write these launcher scripts:

```sh
jk2coop-host [map]                   # host a game on UDP 29070
jk2coop-join <host[:port]> [--second]
```

`--second` runs a second client on the **same machine** for testing: it
gives that client its own clean `fs_homepath` (`/tmp/jk2-client2`, wiped
first) with its own copy of the gamecode, since the game library is
loaded from the home path.

```sh
jk2coop-host                       # machine/terminal 1
jk2coop-join 127.0.0.1 --second    # machine/terminal 2 (same box)
```

## From the in-game console

Host from a game that is already running, with no launch flags:

```
coop_host [maxplayers]      # open the network socket for the current game
                            # maxplayers 1-4 (default 2); sets sv_maxclients
                            # prints the port other machines should join
```

Discover co-op hosts on the local network instead of typing an IP:

```
localservers                # broadcasts on the LAN; prints each co-op host
                            # found, with its name, map, and player count
```

## From the Co-op menu

The in-game Co-op menu (`uimenu coopMenu`, shipped in `zz-coop-ui.pk3`)
drives both from buttons: Host, a LAN server list with Refresh/Join, and
a direct-connect field. `sv_hostname` sets the name shown in the list.

## Who should host

**The player who wants the story hosts.** Campaign scripting — the intro
briefing, cutscenes, mission objectives, checkpoint text, level triggers
— runs for the host player. Joiners currently get the world, the other
players, and the NPCs, but not the campaign UI; syncing it to joiners is
[Track F](campaign-ui-plan.md). If a joiner is rejected, the reason
("Server is full.", protocol mismatch) is shown on the menu.
