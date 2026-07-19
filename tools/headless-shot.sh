#!/usr/bin/env bash
#
# Single-instance headless screenshot harness.
#
# Boots ONE Single-Player engine instance under a window-manager-less Xvfb
# virtual framebuffer, drives it to a menu or a map, captures PNG frames with the
# engine's `screenshot_png` command (glReadPixels -> PNG), and analyses each PNG
# with ImageMagick to report whether it is a real rendered view or a black frame.
#
# This is the single-instance sibling of tools/headless-verify.sh (which drives a
# co-op host + remote client). Use this one to verify Single-Player UI and combat
# changes — menus, HUD, crosshair, weapon feel — without a physical display.
#
# WHY WM-LESS XVFB + screenshot_png
#   On the real desktop the game throttles/stalls when its window is unfocused, so
#   automated runs stall. Under Xvfb with no window manager the window is always
#   nominally focused, and com_maxfpsUnfocused 0 keeps the loop full speed. The
#   engine's screenshot_png reads the GL backbuffer directly, which is more
#   reliable headless than the jpeg `screenshot` path.
#
# GOTCHAS (learned the hard way)
#   - Some SP maps (e.g. kejim_post) open with a long scripted ICARUS cutscene
#     that eats +wait/+exec timing. Pass --skip-cutscenes (sets
#     g_skipIntroCinematics 1) to drop straight into player control.
#   - Menus are addressed by their INTERNAL name, not the file name: the controls
#     page is "controlsMenu", not "controls".
#   - Cheat commands (give, npc spawn, noclip) need --cheats (helpUsObi 1).
#
# Usage:
#   tools/headless-shot.sh --menu <menuName> [options]
#   tools/headless-shot.sh --map <mapName> [options]
#
# Options:
#   --menu NAME       Open UI menu NAME (internal name, e.g. controlsMenu) and shoot.
#   --map NAME        Load map NAME and shoot.
#   --cfg FILE        Exec this cfg (path relative to the instance base/, or absolute
#                     copied in) after load, before the shots. For give/npc/etc.
#   --cheats          Enable cheats (helpUsObi 1) for give/npc/noclip commands.
#   --skip-cutscenes  Set g_skipIntroCinematics 1 (auto-skip map-intro cutscenes).
#   --shots N         Number of screenshots to take (default 1).
#   --settle N        Engine-frames to wait after load before the first shot (default 200).
#   --out DIR         Where to copy the PNGs + log (default /tmp/jk2-headless-shot).
#   --width N --height N   Render size (default 1280x720).
#   -h, --help        Show this help.
#
# Env overrides: JK2_BUILD, JK2_ASSETS.
set -u

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BUILD="${JK2_BUILD:-$ROOT/openjk/build}"
BIN="$BUILD/openjo_sp.x86_64"
GAMESO="$BUILD/codeJK2/game/jospgamex86_64.so"
ASSETS="${JK2_ASSETS:-$HOME/.local/share/openjo}"

MENU="" MAP="" CFG="" CHEATS=0 SKIPCIN=0 SHOTS=1 SETTLE=200
OUT="/tmp/jk2-headless-shot" WIDTH=1280 HEIGHT=720
XVFB_DISP=":98"

die() { echo "error: $*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --menu) MENU="$2"; shift 2;;
    --map) MAP="$2"; shift 2;;
    --cfg) CFG="$2"; shift 2;;
    --cheats) CHEATS=1; shift;;
    --skip-cutscenes) SKIPCIN=1; shift;;
    --shots) SHOTS="$2"; shift 2;;
    --settle) SETTLE="$2"; shift 2;;
    --out) OUT="$2"; shift 2;;
    --width) WIDTH="$2"; shift 2;;
    --height) HEIGHT="$2"; shift 2;;
    -h|--help) sed -n '2,48p' "$0" | sed 's/^# \{0,1\}//'; exit 0;;
    *) die "unknown option: $1 (try --help)";;
  esac
done

[[ -n "$MENU" || -n "$MAP" ]] || die "need --menu NAME or --map NAME (see --help)"
[[ -x "$BIN" ]] || die "engine not found: $BIN (build it, or set JK2_BUILD)"
command -v Xvfb >/dev/null || die "Xvfb not installed"

HOME_DIR="/tmp/jk2-hs-home"
LOG="$OUT/hs.log"
mkdir -p "$OUT"

export SDL_VIDEODRIVER=x11

echo ">>> cleanup"
pkill -f 'openjo_sp.x86_64' 2>/dev/null
pkill -f "Xvfb $XVFB_DISP" 2>/dev/null
sleep 1
rm -rf "$HOME_DIR"; mkdir -p "$HOME_DIR/base"
# Instance loads its gamecode from homepath/base; symlink the freshly built .so.
ln -sf "$GAMESO" "$HOME_DIR/base/jospgamex86_64.so"
# Screenshots land under homepath; keep them isolated from the user's real install.

echo ">>> starting Xvfb on $XVFB_DISP (no window manager)"
Xvfb "$XVFB_DISP" -screen 0 "${WIDTH}x${HEIGHT}x24" -nolisten tcp >/dev/null 2>&1 &
XVFB_PID=$!
sleep 2
export DISPLAY="$XVFB_DISP"

# Build the +command chain: settle, (menu|nothing), shots, quit.
CMD=( "$BIN"
  +set fs_basepath "$ASSETS" +set fs_homepath "$HOME_DIR"
  +set r_fullscreen 0 +set r_mode -1 +set r_customwidth "$WIDTH" +set r_customheight "$HEIGHT"
  +set com_maxfps 30 +set com_maxfpsUnfocused 0 +set com_maxfpsMinimized 0
  +set developer 1 )
(( CHEATS ))  && CMD+=( +set helpUsObi 1 )
(( SKIPCIN )) && CMD+=( +set g_skipIntroCinematics 1 )

if [[ -n "$MAP" ]]; then
  CMD+=( +map "$MAP" )
else
  # A menu-only run still needs the UI up; the main menu loads on boot.
  :
fi

CMD+=( +wait "$SETTLE" )
[[ -n "$CFG" ]]  && CMD+=( +exec "$CFG" +wait 40 )
[[ -n "$MENU" ]] && CMD+=( +uimenu "$MENU" +wait 60 )
for _ in $(seq 1 "$SHOTS"); do
  CMD+=( +screenshot_png +wait 60 )
done
# NB: deliberately NO `+quit`. The engine's in-console `quit` runs a shutdown that
# makes a filesystem call after the FS subsystem is torn down ("Filesystem call
# made without initialization"), which recurses and pops a blocking zenity crash
# dialog — under headless Xvfb nobody clicks OK, so the run hangs for minutes. We
# instead let the engine idle after the last shot (the screenshots are already on
# disk) and end it with SIGTERM below, which exits cleanly without that path.
CMD+=( +wait 60 )

echo ">>> launching engine: ${MENU:+menu=$MENU }${MAP:+map=$MAP }shots=$SHOTS"
"${CMD[@]}" > "$LOG" 2>&1 &
PID=$!

# Wait until the expected number of screenshots have been written (bounded), then
# stop the engine with SIGTERM. Screenshots go to <homepath>/base/screenshots.
SHOTDIR="$HOME_DIR/base/screenshots"
echo ">>> waiting for $SHOTS screenshot(s) (max ~90s)"
for i in $(seq 1 90); do
  kill -0 "$PID" 2>/dev/null || break
  [[ "$(ls -1 "$SHOTDIR"/*.png 2>/dev/null | wc -l)" -ge "$SHOTS" ]] && break
  sleep 1
done
kill "$PID" 2>/dev/null   # SIGTERM: clean shutdown, no +quit crash dialog

echo ">>> teardown"
pkill -f 'openjo_sp.x86_64' 2>/dev/null
kill "$XVFB_PID" 2>/dev/null

echo ""
echo "========== engine markers =========="
grep -viE 'GL_[A-Z]' "$LOG" | grep -iE 'Parsing menu|Unable to find menu|loaded .*faces|Wrote screen|Com_Error|ERROR|couldn|expected token' | tail -15

echo ""
echo "========== SCREENSHOT ANALYSIS =========="
shots=$(find "$HOME_DIR" -name 'shot*.png' 2>/dev/null | sort)
if [[ -z "$shots" ]]; then
  echo "  NO screenshots written — the run never reached a rendered frame (see $LOG)."
  exit 1
fi
have_magick=1; command -v magick >/dev/null || have_magick=0
for s in $shots; do
  cp "$s" "$OUT/$(basename "$s")" 2>/dev/null
  if (( have_magick )); then
    stats=$(magick "$s" -format "mean=%[fx:mean] stddev=%[fx:standard_deviation] colors=%k" info: 2>/dev/null)
    mean=$(echo "$stats" | sed -E 's/.*mean=([0-9.]+).*/\1/')
    colors=$(echo "$stats" | sed -E 's/.*colors=([0-9]+).*/\1/')
    verdict="BLACK/empty"
    awk "BEGIN{exit !($mean > 0.02 && $colors > 500)}" && verdict="RENDERED"
    echo "  $(basename "$s"): $stats  -> $verdict"
  else
    echo "  $(basename "$s")  (install ImageMagick 'magick' for black/rendered analysis)"
  fi
done
echo ""
echo "  PNGs + log copied to: $OUT"
