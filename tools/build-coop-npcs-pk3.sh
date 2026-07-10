#!/usr/bin/env bash
#
# Build the co-op NPC compatibility pk3.
#
# Jedi Outcast stores its NPC definitions in a single `ext_data/NPCs.cfg`
# inside assets0.pk3. Jedi Academy's multiplayer gamecode instead reads
# every `ext_data/NPCs/*.npc` and concatenates them into one buffer,
# skipping any base .cfg entirely (see codemp/game/NPC_stats.c:3576).
#
# The grammar and keys are identical -- both trees hand the concatenated
# buffer to the same NPC_ParseParms(). So no format translation is
# required: the file only needs to be relocated and renamed.
#
# This script extracts NPCs.cfg from the user's own retail installation
# and repackages it. No proprietary asset is stored in this repository.
#
# Usage:
#   tools/build-coop-npcs-pk3.sh <path-to-GameData/base> [output-dir]

set -euo pipefail

BASE_DIR="${1:-}"
OUT_DIR="${2:-$(pwd)}"

if [[ -z "$BASE_DIR" ]]; then
    echo "usage: $0 <path-to-GameData/base> [output-dir]" >&2
    echo "  e.g. $0 \"\$HOME/.steam/steam/steamapps/common/Jedi Outcast/GameData/base\"" >&2
    exit 2
fi

ASSETS="$BASE_DIR/assets0.pk3"
if [[ ! -r "$ASSETS" ]]; then
    echo "error: cannot read $ASSETS" >&2
    exit 1
fi

for tool in unzip zip; do
    command -v "$tool" >/dev/null || { echo "error: $tool not found" >&2; exit 1; }
done

mkdir -p "$OUT_DIR"
OUT_DIR="$(cd "$OUT_DIR" && pwd)"

STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

mkdir -p "$STAGE/ext_data/NPCs"

# assets0.pk3 stores it lowercase as ext_data/npcs.cfg
if ! unzip -p "$ASSETS" 'ext_data/npcs.cfg' > "$STAGE/ext_data/NPCs/jk2npcs.npc" 2>/dev/null; then
    echo "error: ext_data/npcs.cfg not found inside $ASSETS" >&2
    exit 1
fi

if [[ ! -s "$STAGE/ext_data/NPCs/jk2npcs.npc" ]]; then
    echo "error: extracted npcs.cfg is empty" >&2
    exit 1
fi

# The name is prefixed so the archive sorts after assets5.pk3 and
# therefore shadows it in the engine's search path.
OUT="$OUT_DIR/zzz-coop-npcs.pk3"
rm -f "$OUT"
( cd "$STAGE" && zip -q -r "$OUT" ext_data )

blocks=$(grep -cE '^[A-Za-z_][A-Za-z0-9_]*[[:space:]]*$' "$STAGE/ext_data/NPCs/jk2npcs.npc" || true)
echo "wrote $OUT ($(stat -c%s "$OUT") bytes, ~$blocks NPC definitions)"
echo
echo "Install with:"
echo "  cp \"$OUT\" ~/.local/share/openjk/base/"
