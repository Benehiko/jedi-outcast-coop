#!/usr/bin/env bash
#
# Headless M3 verification harness.
#
# Runs the co-op host + a dual-load remote client under a single Xvfb virtual
# framebuffer (no physical screen, and — crucially — no window manager, so the
# client window is always "focused" and the game loop runs full speed instead of
# stalling as it does on the real unfocused :1 display). The client renders with
# llvmpipe software GL and captures frames with the engine's `screenshot_png`
# command (glReadPixels -> PNG). We then analyse the PNGs with ImageMagick to
# verify the remote client actually rendered a 3D view (and, with an NPC spawned
# on the host, that characters appear).
#
# Usage: headless-verify.sh [map] [port]
set -u

MAP="${1:-kejim_post}"
PORT="${2:-29073}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BUILD="${JK2_BUILD:-$ROOT/openjk/build}"
BIN="$BUILD/openjo_sp.x86_64"
GAMESO="$BUILD/codeJK2/game/jospgamex86_64.so"
ASSETS="${JK2_ASSETS:-$HOME/.local/share/openjo}"
OUT="${JK2_HV_OUT:-/tmp/jk2-headless-verify}"
mkdir -p "$OUT"
HOST_HOME="/tmp/jk2-hv-host"
CLIENT_HOME="/tmp/jk2-hv-client"
HOST_LOG="$OUT/hv-host.log"
CLIENT_LOG="$OUT/hv-client.log"
XVFB_DISP=":99"

export SDL_VIDEODRIVER=x11

echo ">>> cleanup"
pkill -f 'openjo_sp.x86_64' 2>/dev/null
pkill -f "Xvfb $XVFB_DISP" 2>/dev/null
sleep 1
rm -rf "$HOST_HOME" "$CLIENT_HOME"
mkdir -p "$HOST_HOME/base" "$CLIENT_HOME/base"
# Both instances load their own gamecode copy from their homepath/base.
ln -sf "$GAMESO" "$HOST_HOME/base/jospgamex86_64.so"
ln -sf "$GAMESO" "$CLIENT_HOME/base/jospgamex86_64.so"

echo ">>> starting Xvfb on $XVFB_DISP"
Xvfb "$XVFB_DISP" -screen 0 1280x800x24 -nolisten tcp >/dev/null 2>&1 &
XVFB_PID=$!
sleep 2
export DISPLAY="$XVFB_DISP"

# ---- HOST -----------------------------------------------------------------
# A host-side cfg: once the level is up, spawn a stormtrooper near the player so
# it enters PVS and the remote client must render it (M3.3). bind a key is not
# needed; we queue commands with waits on the command line instead.
echo ">>> launching HOST (net_enabled, spawns an NPC after settle)"
# helpUsObi 1 = g_cheats, required for the CMD_CHEAT `npc spawn` command.
"$BIN" \
  +set fs_basepath "$ASSETS" +set fs_homepath "$HOST_HOME" \
  +set net_enabled 1 +set net_port "$PORT" +set developer 1 \
  +set helpUsObi 1 \
  +set r_fullscreen 0 +set r_mode -1 +set r_customwidth 640 +set r_customheight 400 \
  +set com_maxfps 30 +set com_maxfpsUnfocused 0 +set com_maxfpsMinimized 0 \
  +map "$MAP" \
  +wait 400 +"npc spawn stormtrooper" \
  +wait 50  +"npc spawn stormtrooper" \
  +wait 50  +"npc spawn stormtrooper" \
  +wait 100000 \
  > "$HOST_LOG" 2>&1 &
HOST_PID=$!

echo ">>> waiting for host to bind + load"
for i in $(seq 1 60); do
  grep -q 'Opening IP socket' "$HOST_LOG" 2>/dev/null && \
    grep -q 'loaded .* faces' "$HOST_LOG" 2>/dev/null && break
  kill -0 "$HOST_PID" 2>/dev/null || { echo "!!! host died"; tail -20 "$HOST_LOG"; kill $XVFB_PID; exit 1; }
  sleep 1
done
REAL_PORT=$(grep 'Opening IP socket' "$HOST_LOG" | grep -oE '[0-9]+$' | tail -1)
REAL_PORT="${REAL_PORT:-$PORT}"
echo ">>> host bound on $REAL_PORT; letting it settle + spawn NPCs"
sleep 8

# ---- CLIENT ---------------------------------------------------------------
# The client connects, settles, then takes several screenshots spaced out so we
# catch frames after the host player + NPCs are in its snapshot. com_maxfpsUnfocused 0
# keeps it full-speed even though (under Xvfb, without a WM) it is nominally focused.
echo ">>> launching CLIENT (dual-load remote), connecting to :$REAL_PORT"
"$BIN" \
  +set fs_basepath "$ASSETS" +set fs_homepath "$CLIENT_HOME" \
  +set net_enabled 1 +set developer 1 +set cl_timeout 120 \
  +set r_fullscreen 0 +set r_mode -1 +set r_customwidth 640 +set r_customheight 400 \
  +set com_maxfps 30 +set com_maxfpsUnfocused 0 +set com_maxfpsMinimized 0 \
  +connect "127.0.0.1:$REAL_PORT" \
  +wait 250 +screenshot_png \
  +wait 150 +screenshot_png \
  +wait 150 +screenshot_png \
  +wait 150 +screenshot_png \
  +wait 150 +screenshot_png \
  +wait 150 +screenshot_png \
  +wait 60 \
  > "$CLIENT_LOG" 2>&1 &
CLIENT_PID=$!

# NB: no `+quit` on either instance. The in-console `quit` runs a shutdown that
# makes a filesystem call after FS teardown ("Filesystem call made without
# initialization"), recurses, and pops a blocking zenity crash dialog that hangs
# a headless run for minutes. We wait for the expected screenshots, then end both
# with SIGTERM (below), which exits cleanly without that path.
CLIENT_SHOTDIR="$CLIENT_HOME/base/screenshots"
echo ">>> waiting for 6 client screenshot(s) (max ~90s)"
for i in $(seq 1 90); do
  kill -0 "$CLIENT_PID" 2>/dev/null || break
  [[ "$(ls -1 "$CLIENT_SHOTDIR"/*.png 2>/dev/null | wc -l)" -ge 6 ]] && break
  sleep 1
done
kill "$CLIENT_PID" 2>/dev/null   # SIGTERM: clean shutdown, no +quit crash dialog

echo ">>> teardown"
kill "$HOST_PID" 2>/dev/null
pkill -f 'openjo_sp.x86_64' 2>/dev/null
kill "$XVFB_PID" 2>/dev/null

echo ""
echo "========== HOST: connect + NPC spawn markers =========="
grep -niE 'Opening IP socket|ClientEnterWorld|Kyle|NPC_Spawn|stormtrooper|Spawning' "$HOST_LOG" | tail -12

echo ""
echo "========== CLIENT: dual-load + render markers =========="
grep -vE 'GL_[A-Z]' "$CLIENT_LOG" | grep -iE 'dual-load|loaded .*faces|Wrote|connectResponse|ClientEnter|timed out|error|couldn' | tail -20

echo ""
echo "========== SCREENSHOT ANALYSIS (client) =========="
shots=$(find "$CLIENT_HOME" -name 'shot*.png' 2>/dev/null | sort)
if [ -z "$shots" ]; then
  echo "  NO screenshots written — client likely never reached the 3D view."
else
  for s in $shots; do
    stats=$(magick "$s" -format "mean=%[fx:mean] stddev=%[fx:standard_deviation] colors=%k" info: 2>/dev/null)
    # A real rendered 3D view: mean well above black, high stddev, thousands of colors.
    verdict="BLACK/empty"
    mean=$(echo "$stats" | sed -E 's/.*mean=([0-9.]+).*/\1/')
    colors=$(echo "$stats" | sed -E 's/.*colors=([0-9]+).*/\1/')
    if awk "BEGIN{exit !($mean > 0.02 && $colors > 500)}"; then
      verdict="RENDERED 3D VIEW"
    fi
    echo "  $(basename "$s"): $stats  -> $verdict"
    cp "$s" "$OUT/$(basename "$s")" 2>/dev/null
  done
  echo ""
  echo "  copies + logs in: $OUT"
fi
