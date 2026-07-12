#!/usr/bin/env bash
#
# build-sensitivity-menu-pk3.sh — rescale the Single-Player CONTROLS >
# "Mouse Sensitivity" slider so it covers a modern, fine-grained low range.
#
# WHY THIS EXISTS
#   The retail sensitivity slider is defined as `cvarfloat "sensitivity" 5 2 30`
#   (default 5, min 2, max 30). On a modern high-DPI mouse that whole range is
#   fast, and — because the JK2 menu slider is a continuous drag with no discrete
#   step — the useful low end is a tiny sliver you can't land on precisely. The
#   modern-combat work lowered the shipped default to 0.5 (see
#   docs/modern-combat.md), but the menu slider still bottomed out at 2, so you
#   could not reach or fine-tune the new low values from the UI.
#
#   This rescales the slider to `0.5 0.1 2` (default 0.5, min 0.1, max 2). The
#   range is now small enough that dragging it gives ~0.1 granularity across the
#   bar, and 2.0 becomes the top of the slider. (The engine has no explicit slider
#   step, so this is smooth, not hard-snapped to 0.1 — you can still type an exact
#   value with `sensitivity <n>` in the console.)
#
# WHAT IT DOES (and the copyright shape)
#   The menu files belong to Raven and live inside your retail assets*.pk3, so this
#   repo does NOT ship them. Exactly like tools/build-widescreen-menu-pk3.sh, this
#   reads the menu files from YOUR OWN copy of the game, rewrites only the one
#   sensitivity `cvarfloat` line, and writes an override pak
#   (zz-sensitivity-menu.pk3) into your base/. The "zz-" prefix sorts it after the
#   retail paks so it shadows the stock menus. Your retail assets are never
#   modified. Remove the feature by deleting the one pak.
#
# Usage:
#   tools/build-sensitivity-menu-pk3.sh [options]
#
# Options:
#   --assets DIR   Directory containing your retail assets*.pk3
#                  (default: $HOME/.local/share/openjo/base)
#   --out FILE     Output pak path (default: <assets>/zz-sensitivity-menu.pk3)
#   --range "D MIN MAX"
#                  Slider default/min/max to write (default: "0.5 0.1 2").
#   -h, --help     Show this help
set -euo pipefail

ASSETS="${JK2_ASSETS_DIR:-$HOME/.local/share/openjo/base}"
OUT=""
RANGE="0.5 0.1 2"

die() { echo "error: $*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "required tool not found: $1"; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --assets) ASSETS="$2"; shift 2;;
    --out)    OUT="$2"; shift 2;;
    --range)  RANGE="$2"; shift 2;;
    -h|--help) sed -n '2,39p' "$0" | sed 's/^# \{0,1\}//'; exit 0;;
    *) die "unknown option: $1 (try --help)";;
  esac
done

need zip
need unzip
need python3
OUT="${OUT:-$ASSETS/zz-sensitivity-menu.pk3}"

# Validate --range is three numbers.
read -r R_DEF R_MIN R_MAX _ <<<"$RANGE"
for v in "$R_DEF" "$R_MIN" "$R_MAX"; do
  [[ "$v" =~ ^[0-9]+([.][0-9]+)?$ ]] || die "--range must be three numbers 'DEFAULT MIN MAX' (got: '$RANGE')"
done

# The two SP menus that carry the mouse-sensitivity slider. The MP menus under
# ui/jk2mp/ are a separate slider and left alone.
MENUS=(controls.menu ingamecontrols.menu)

# The stock slider line, and its replacement. We match the whole cvarfloat token
# so a menu that was already rescaled (or a different edition) is skipped, not
# corrupted.
STOCK='cvarfloat			"sensitivity" 5 2 30'
NEW="cvarfloat			\"sensitivity\" $R_DEF $R_MIN $R_MAX"

WORK="$(mktemp -d "${TMPDIR:-/tmp}/jk2-sensitivity-menu.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT
mkdir -p "$WORK/ui"

PATCHED=0
for f in "${MENUS[@]}"; do
  # Find the retail pak that actually carries this menu (case-insensitive: retail
  # ships ui/controls.menu, some editions ui/Controls.menu).
  SRC=""
  for p in "$ASSETS"/assets*.pk3; do
    [[ -e "$p" ]] || continue
    real="$(unzip -Z1 "$p" 2>/dev/null | grep -ixE "ui/$f" | head -1 || true)"
    if [[ -n "$real" ]] && unzip -p "$p" "$real" 2>/dev/null | grep -qF 'cvarfloat			"sensitivity"'; then
      SRC="$p"; SRC_ENTRY="$real"; break
    fi
  done
  if [[ -z "$SRC" ]]; then
    echo ">>> skip $f (no retail pak carries it with a sensitivity slider)"
    continue
  fi

  # Emit at the lowercase path the menu loader references (ui/controls.menu,
  # ui/ingamecontrols.menu), regardless of the retail entry's case.
  unzip -p "$SRC" "$SRC_ENTRY" > "$WORK/ui/$f" 2>/dev/null || { echo ">>> skip $f (extract failed)"; continue; }

  if ! grep -qF "$STOCK" "$WORK/ui/$f"; then
    echo ">>> skip $f (sensitivity slider not in the expected stock form — already rescaled or different edition)"
    rm -f "$WORK/ui/$f"; continue
  fi

  # Byte-exact replace to preserve CRLF + latin-1 encoding.
  python3 - "$WORK/ui/$f" "$STOCK" "$NEW" <<'PY'
import sys
path, stock, new = sys.argv[1], sys.argv[2], sys.argv[3]
with open(path, 'rb') as fh:
    data = fh.read()
sb, nb = stock.encode('latin-1'), new.encode('latin-1')
n = data.count(sb)
if n < 1:
    sys.stderr.write(f"error: sensitivity slider not found in {path}\n"); sys.exit(2)
open(path, 'wb').write(data.replace(sb, nb))
PY
  echo ">>> patched ui/$f  (from '$SRC_ENTRY' in $(basename "$SRC"))"
  PATCHED=$((PATCHED+1))
done

(( PATCHED > 0 )) || die "no menu files could be patched"

mkdir -p "$(dirname "$OUT")"
rm -f "$OUT"
( cd "$WORK" && zip -q -r -X "$OUT" ui )
[[ -f "$OUT" ]] || die "zip produced no pak"

echo ""
echo ">>> built $OUT ($PATCHED menu file(s))"
echo "    CONTROLS > \"Mouse Sensitivity\" slider is now $R_DEF default, $R_MIN min, $R_MAX max."
echo "    The slider is continuous (no hard 0.1 step); type exact values with 'sensitivity <n>'."
echo "    To remove: rm '$OUT'"
