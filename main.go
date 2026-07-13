// Command jk2coop is the tooling CLI for the Jedi Outcast co-op rebuild.
//
// It replaces the collection of shell scripts under tools/ with a single,
// cross-platform, testable binary. Subcommands mirror the original scripts:
//
//	jk2coop patches apply     — apply this repo's patches to the OpenJK submodule
//	jk2coop pk3 coop-ui       — build the co-op UI overlay pak
//	jk2coop pk3 coop-npcs     — build the co-op NPC compatibility pak
//	jk2coop pk3 widescreen    — build the widescreen video-menu pak
//	jk2coop install           — stage the engine data dir + launchers
//	jk2coop install --uninstall
package main

import (
	"github.com/Benehiko/jedi-outcast-coop/cmd"
)

func main() {
	cmd.Execute()
}
