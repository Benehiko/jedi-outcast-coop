#!/usr/bin/env bash
#
# Package the co-op UI overlay (assets/coop-ui/) into zz-coop-ui.pk3.
#
# The pk3 is just a zip. The "zz-" prefix makes it sort AFTER the retail
# assets*.pk3 so its ui/menus.txt shadows the stock one (adding our Co-op page).
# Everything in it is original authorship — no retail files are included.
#
# Usage: tools/build-coop-ui-pk3.sh [output-dir]
#   Default output: assets/coop-ui/zz-coop-ui.pk3
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC="$ROOT/assets/coop-ui"
OUT_DIR="${1:-$SRC}"
OUT="$OUT_DIR/zz-coop-ui.pk3"

[[ -d "$SRC/ui" ]] || { echo "error: $SRC/ui not found" >&2; exit 1; }

mkdir -p "$OUT_DIR"
rm -f "$OUT"

# Zip the ui/ tree (deterministic order; exclude the pk3 itself and any dotfiles).
( cd "$SRC" && zip -q -r -X "$OUT" ui -x '*.pk3' '*/.*' )

echo "built $OUT"
zip -sf "$OUT" | sed 's/^/  /'
