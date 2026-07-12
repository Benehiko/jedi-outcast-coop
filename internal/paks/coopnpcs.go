package paks

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/Benehiko/jedi-outcast-coop/internal/pk3"
)

// npcBlockRe matches an NPC definition header line (a bare identifier on its own
// line), used only to report an approximate definition count.
var npcBlockRe = regexp.MustCompile(`(?m)^[A-Za-z_][A-Za-z0-9_]*[[:space:]]*$`)

// CoopNPCsResult reports what BuildCoopNPCs produced.
type CoopNPCsResult struct {
	OutPath  string
	Bytes    int64
	NumDefs  int
	NumBytes int
}

// BuildCoopNPCs extracts the retail NPC definitions from the user's own
// assets0.pk3 and repackages them so Jedi Academy's multiplayer gamecode picks
// them up.
//
// Jedi Outcast stores its NPC definitions in a single ext_data/NPCs.cfg inside
// assets0.pk3. Jedi Academy's MP gamecode instead reads every
// ext_data/NPCs/*.npc and concatenates them, skipping any base .cfg. The grammar
// and keys are identical, so no format translation is needed: the file is only
// relocated (ext_data/NPCs/jk2npcs.npc) and the pak named to sort last.
//
// No proprietary asset is stored in this repo — the data comes from the user's
// own game copy at baseDir. The pak is written to outDir as zzz-coop-npcs.pk3.
func BuildCoopNPCs(baseDir, outDir string) (*CoopNPCsResult, error) {
	assets := filepath.Join(baseDir, "assets0.pk3")
	r, err := pk3.Open(assets)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", assets, err)
	}
	defer func() { _ = r.Close() }()

	// assets0.pk3 stores it lowercased as ext_data/npcs.cfg.
	data, err := r.ReadFile("ext_data/npcs.cfg")
	if err != nil {
		return nil, fmt.Errorf("ext_data/npcs.cfg not found inside %s", assets)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("extracted npcs.cfg is empty")
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	absOut, err := filepath.Abs(outDir)
	if err != nil {
		return nil, err
	}

	// Stage the relocated/renamed file, then pack ext_data/ into the pak.
	stage, err := os.MkdirTemp("", "jk2-coop-npcs-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(stage) }()

	npcDir := filepath.Join(stage, "ext_data", "NPCs")
	if err := os.MkdirAll(npcDir, 0o755); err != nil {
		return nil, err
	}
	npcFile := filepath.Join(npcDir, "jk2npcs.npc")
	if err := os.WriteFile(npcFile, data, 0o644); err != nil {
		return nil, err
	}

	// The name is prefixed so the archive sorts after assets5.pk3 and therefore
	// shadows it in the engine's search path.
	outPath := filepath.Join(absOut, "zzz-coop-npcs.pk3")
	if err := os.Remove(outPath); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	b := pk3.NewBuilder()
	if err := b.AddTree(filepath.Join(stage, "ext_data"), "ext_data"); err != nil {
		return nil, err
	}
	if err := b.Write(outPath); err != nil {
		return nil, err
	}

	fi, err := os.Stat(outPath)
	if err != nil {
		return nil, err
	}
	return &CoopNPCsResult{
		OutPath: outPath,
		Bytes:   fi.Size(),
		NumDefs: len(npcBlockRe.FindAll(data, -1)),
	}, nil
}
