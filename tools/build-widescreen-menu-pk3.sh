#!/usr/bin/env bash
#
# build-widescreen-menu-pk3.sh — add the widescreen / QHD / ultrawide / 4K video
# modes to the Single-Player SETUP > VIDEO > "Video Mode" menu field.
#
# WHY THIS EXISTS
#   The widescreen work (Track G, patches 0022/0023) added modes 13-21 to the
#   engine's video-mode table (r_vidModes in shared/sdl/sdl_window.cpp: 1280x720,
#   1080p, 2560x1080, 2560x1440 QHD, 3440x1440, 3840x1600, 4K, 5120x1440). Those
#   modes WORK from the console (`r_mode 17`) and config, but the SP setup menu's
#   "Video Mode" cycle field is data-driven from ui/ingamesetup.menu +
#   ui/setup.menu, whose resolution list stops at 2048x1536 (mode 10). So the new
#   modes were unreachable from the in-game menu. This tool closes that gap.
#
# WHAT IT DOES (and the copyright shape)
#   The two menu files belong to Raven and live inside your retail assets1.pk3, so
#   this repo does NOT ship them. Instead — exactly like tools/upscale-textures.sh
#   — this reads the menu files from YOUR OWN copy of the game, appends the extra
#   resolution entries to the single `cvarFloatList` line that defines the Video
#   Mode field, and writes an override pak (zz-widescreen-menu.pk3) into your
#   base/. The "zz-" prefix sorts it after the retail paks so it shadows the stock
#   menus. Your retail assets are never modified. Remove the feature by deleting
#   the one pak.
#
#   The appended entries use literal quoted strings (e.g. "2560 X 1440 QHD") mapped
#   to the engine mode numbers, so no localized string-table token is required.
#
# Usage:
#   tools/build-widescreen-menu-pk3.sh [options]
#
# Options:
#   --assets DIR   Directory containing your retail assets*.pk3
#                  (default: $HOME/.local/share/openjo/base)
#   --out FILE     Output pak path (default: <assets>/zz-widescreen-menu.pk3)
#   -h, --help     Show this help
set -euo pipefail

ASSETS="${JK2_ASSETS_DIR:-$HOME/.local/share/openjo/base}"
OUT=""

die() { echo "error: $*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "required tool not found: $1"; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --assets) ASSETS="$2"; shift 2;;
    --out)    OUT="$2"; shift 2;;
    -h|--help) sed -n '2,44p' "$0" | sed 's/^# \{0,1\}//'; exit 0;;
    *) die "unknown option: $1 (try --help)";;
  esac
done

need zip
need unzip
OUT="${OUT:-$ASSETS/zz-widescreen-menu.pk3}"

# The retail resolution list ends with mode 10 (2048x1536). We match that exact
# tail and append the Track-G modes. If a menu's line does not end in the stock
# way (already patched, or a different edition), we skip it rather than corrupt it.
TAIL='@MENUS1_2048_X_1536 10 }'
ADD='@MENUS1_2048_X_1536 10  "1280 X 720 (16:9)" 13  "1600 X 900 (16:9)" 14  "1920 X 1080 (16:9)" 15  "2560 X 1080 (21:9)" 16  "2560 X 1440 QHD" 17  "3440 X 1440 (21:9)" 18  "3840 X 1600 (24:10)" 19  "3840 X 2160 4K" 20  "5120 X 1440 (32:9)" 21 }'

# Find the retail pak that actually carries the SP menu with the Video Mode field.
SRC_PAK=""
for p in "$ASSETS"/assets*.pk3; do
  [[ -e "$p" ]] || continue
  if unzip -p "$p" 'ui/ingamesetup.menu' 2>/dev/null | grep -q 'ui_r_mode'; then
    SRC_PAK="$p"; break
  fi
done
[[ -n "$SRC_PAK" ]] || die "no assets*.pk3 in '$ASSETS' contains the SP video menu (ui/ingamesetup.menu with ui_r_mode). Point --assets at your retail base/."
echo ">>> source menus: $SRC_PAK"

WORK="$(mktemp -d "${TMPDIR:-/tmp}/jk2-widescreen-menu.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT
mkdir -p "$WORK/ui"

PATCHED=0
for f in ingamesetup.menu setup.menu; do
  if ! unzip -p "$SRC_PAK" "ui/$f" > "$WORK/ui/$f" 2>/dev/null; then
    echo ">>> skip $f (not in source pak)"; rm -f "$WORK/ui/$f"; continue
  fi
  if ! grep -qF "$TAIL" "$WORK/ui/$f"; then
    echo ">>> skip $f (resolution list not in the expected stock form — already patched or different edition)"
    rm -f "$WORK/ui/$f"; continue
  fi
  # Replace only the stock tail on the cvarFloatList line; F-string safe (no regex metachars in play).
  python3 - "$WORK/ui/$f" "$TAIL" "$ADD" <<'PY'
import sys
path, tail, add = sys.argv[1], sys.argv[2], sys.argv[3]
with open(path, 'rb') as fh:
    data = fh.read()
# Operate on bytes to preserve CRLF and 8859 encoding exactly.
tb, ab = tail.encode('latin-1'), add.encode('latin-1')
n = data.count(tb)
if n != 1:
    sys.stderr.write(f"error: expected exactly one resolution list in {path}, found {n}\n")
    sys.exit(2)
open(path, 'wb').write(data.replace(tb, ab))
PY
  echo ">>> patched ui/$f"
  PATCHED=$((PATCHED+1))
done

(( PATCHED > 0 )) || die "no menu files could be patched"

mkdir -p "$(dirname "$OUT")"
rm -f "$OUT"
( cd "$WORK" && zip -q -r -X "$OUT" ui )
[[ -f "$OUT" ]] || die "zip produced no pak"

echo ""
echo ">>> built $OUT ($PATCHED menu file(s))"
echo "    SETUP > VIDEO > \"Video Mode\" now lists 720p through 5120x1440 (32:9)."
echo "    These map to engine modes 13-21 (added by the widescreen patches 0022/0023)."
echo "    To remove: rm '$OUT'"
