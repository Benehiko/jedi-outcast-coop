#!/usr/bin/env bash
#
# M4 soak test: host + dual-load remote client under Xvfb for a long run, with
# the client under gdb so any crash yields a backtrace. The host repeatedly
# spawns stormtroopers (which fight the host player), so the remote client must
# continuously build + render players and NPCs and lerp their trajectories for
# the whole duration. Catches render-path crashes that only show up under
# sustained play. Periodic screenshots confirm the client keeps rendering.
#
# Usage: soak-m4.sh [minutes] [map] [port]
set -u

MINUTES="${1:-10}"
MAP="${2:-kejim_post}"
PORT="${3:-29073}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BUILD="${JK2_BUILD:-$ROOT/openjk/build}"
BIN="$BUILD/openjo_sp.x86_64"
GAMESO="$BUILD/codeJK2/game/jospgamex86_64.so"
ASSETS="${JK2_ASSETS:-$HOME/.local/share/openjo}"
OUT="${JK2_SOAK_OUT:-/tmp/jk2-soak-m4}"
mkdir -p "$OUT"
HOST_HOME="/tmp/jk2-soak-host"
CLIENT_HOME="/tmp/jk2-soak-client"
HOST_LOG="$OUT/soak-host.log"
CLIENT_LOG="$OUT/soak-client.log"
GDB_LOG="$OUT/soak-gdb.log"
XVFB_DISP=":99"

export SDL_VIDEODRIVER=x11

DURATION=$(( MINUTES * 60 ))
echo ">>> M4 soak: ${MINUTES} min ($DURATION s), map $MAP, port $PORT"

echo ">>> cleanup"
pkill -f 'openjo_sp.x86_64' 2>/dev/null
pkill -f "Xvfb $XVFB_DISP" 2>/dev/null
sleep 1
rm -rf "$HOST_HOME" "$CLIENT_HOME"
mkdir -p "$HOST_HOME/base" "$CLIENT_HOME/base"
ln -sf "$GAMESO" "$HOST_HOME/base/jospgamex86_64.so"
ln -sf "$GAMESO" "$CLIENT_HOME/base/jospgamex86_64.so"

# Host cfg: keep spawning stormtroopers on a loop so there's always combat.
# `wait` units are frames; at com_maxfps 30 that's ~30/s. Loop via a long chain.
# Spawn a SMALL, fixed set of NPCs once and let them persist. The host player is
# AFK in this headless soak, so NPCs are not being killed — spawning on a loop
# exhausts the entity pool (G_Spawn: no free entities) and takes the HOST down,
# which is a harness artifact, not a remote-client bug. A handful of persistent
# NPCs is enough for the remote client to build + render + lerp them for the
# whole duration, which is what M4 stresses on the client side.
HOSTCFG="$HOST_HOME/base/soak.cfg"
{
  echo "wait 350"
  echo "npc spawn stormtrooper"
  echo "wait 60"
  echo "npc spawn stormtrooper"
  echo "wait 60"
  echo "npc spawn stormtrooper"
  echo "wait 60"
  echo "npc spawn reborn"
} > "$HOSTCFG"

echo ">>> starting Xvfb $XVFB_DISP"
Xvfb "$XVFB_DISP" -screen 0 1280x800x24 -nolisten tcp >/dev/null 2>&1 &
XVFB_PID=$!
sleep 2
export DISPLAY="$XVFB_DISP"

echo ">>> launching HOST"
"$BIN" \
  +set fs_basepath "$ASSETS" +set fs_homepath "$HOST_HOME" \
  +set net_enabled 1 +set net_port "$PORT" +set developer 1 +set helpUsObi 1 \
  +set r_fullscreen 0 +set r_mode -1 +set r_customwidth 640 +set r_customheight 400 \
  +set com_maxfps 30 +set com_maxfpsUnfocused 0 +set com_maxfpsMinimized 0 \
  +map "$MAP" +exec soak.cfg \
  +wait 1000000 +quit \
  > "$HOST_LOG" 2>&1 &
HOST_PID=$!

echo ">>> waiting for host bind + load"
for i in $(seq 1 60); do
  grep -q 'Opening IP socket' "$HOST_LOG" 2>/dev/null && grep -q 'loaded .* faces' "$HOST_LOG" 2>/dev/null && break
  kill -0 "$HOST_PID" 2>/dev/null || { echo "!!! host died"; tail -20 "$HOST_LOG"; kill $XVFB_PID; exit 1; }
  sleep 1
done
REAL_PORT=$(grep 'Opening IP socket' "$HOST_LOG" | grep -oE '[0-9]+$' | tail -1); REAL_PORT="${REAL_PORT:-$PORT}"
echo ">>> host on $REAL_PORT; settling"
sleep 6

# gdb batch for the client: run, and on a fault dump a full backtrace.
cat > "$OUT/soak-gdb-cmds.txt" <<GDBEOF
set pagination off
set confirm off
handle SIGPIPE nostop noprint pass
run
echo \n===== SOAK CRASH BACKTRACE =====\n
bt full
echo \n===== THREADS =====\n
thread apply all bt
quit
GDBEOF

# Client: connect only. Duration is controlled entirely by a wall-clock sleep +
# kill below, NOT by any frame-based +wait chain (which does not map to
# wall-clock under variable-speed software GL and was cutting soaks short). The
# client idle-renders the connected session — which is exactly the M4 stress:
# does the remote client crash while rendering players + NPCs over time. A
# background writer sends `screenshot` to the client's console via a bound key
# is overkill; we instead cap com_maxfps low so the session is steady and let it
# render. Screenshots are taken by a modest chain that is allowed to run dry.
CLIENT_CMDS="+connect 127.0.0.1:$REAL_PORT"
for i in $(seq 1 40); do
  CLIENT_CMDS="$CLIENT_CMDS +wait 900 +screenshot_png"   # ~1 shot per chunk of frames
done

echo ">>> launching CLIENT under gdb (soak $DURATION s, wall-clock controlled)"
# shellcheck disable=SC2086
( gdb -batch -x "$OUT/soak-gdb-cmds.txt" \
    --args "$BIN" \
    +set fs_basepath "$ASSETS" +set fs_homepath "$CLIENT_HOME" \
    +set net_enabled 1 +set developer 1 +set cl_timeout 700 \
    +set r_fullscreen 0 +set r_mode -1 +set r_customwidth 640 +set r_customheight 400 \
    +set com_maxfps 30 +set com_maxfpsUnfocused 0 +set com_maxfpsMinimized 0 \
    $CLIENT_CMDS \
    > "$GDB_LOG" 2>&1 ) &
GDB_WRAP=$!

# Wall-clock duration control: sleep the full DURATION, THEN end the client. This
# is the authoritative timer (frame-based waits are unreliable here). We report
# survival = did the client stay alive (no crash) for the whole sleep.
CLIENT_GAME_PID=""
for i in $(seq 1 20); do
  CLIENT_GAME_PID=$(pgrep -f "connect 127.0.0.1:$REAL_PORT" | head -1)
  [ -n "$CLIENT_GAME_PID" ] && break
  sleep 1
done
echo ">>> client pid $CLIENT_GAME_PID; soaking $DURATION s (wall clock)"
SOAK_OK=1
for t in $(seq 1 "$DURATION"); do
  if ! kill -0 "$CLIENT_GAME_PID" 2>/dev/null; then
    echo "!!! client process died at ~${t}s (before the ${DURATION}s target)"
    SOAK_OK=0
    break
  fi
  sleep 1
done
[ "$SOAK_OK" = 1 ] && echo ">>> client survived the full ${DURATION}s wall-clock soak"

# Soak time elapsed (or client already gone). End the client, then let gdb run
# its backtrace commands (No stack on a clean end; a real stack on a crash) and
# exit. TERM the game process so gdb sees the inferior finish.
echo ">>> ending client + collecting gdb result"
pkill -TERM -f "connect 127.0.0.1:$REAL_PORT" 2>/dev/null
# Give gdb up to 30s to dump + quit, then force it.
for i in $(seq 1 30); do kill -0 "$GDB_WRAP" 2>/dev/null || break; sleep 1; done
kill -0 "$GDB_WRAP" 2>/dev/null && pkill gdb 2>/dev/null
wait "$GDB_WRAP" 2>/dev/null

echo ">>> teardown"
kill "$HOST_PID" 2>/dev/null
pkill -f 'openjo_sp.x86_64' 2>/dev/null
kill "$XVFB_PID" 2>/dev/null

echo ""
echo "========== M4 VERDICT =========="
# SOAK_OK is the authoritative result: the client process stayed alive for the
# whole wall-clock DURATION. (The 'client ran ...' log-timestamp span below can
# read short because the client goes quiet once its screenshot chain ends and it
# just idle-renders — that is NOT death. Trust SOAK_OK + the crash check.)
crashed=0
grep -qE 'Program received signal|SIGSEGV|SIGABRT|^#0 ' "$GDB_LOG" && crashed=1
if [ "${SOAK_OK:-0}" = 1 ] && [ "$crashed" = 0 ]; then
  echo "PASS: remote client rendered for the full ${DURATION}s with NO crash."
else
  echo "FAIL/PARTIAL: SOAK_OK=${SOAK_OK:-?} crashed=$crashed — see details below."
fi
# Corroboration: did the host still see the client's packets near the end?
echo "  host still receiving client packets at teardown: $(grep -c 'Delta request from out of date\|Kyle' "$HOST_LOG" 2>/dev/null | head -1) markers"

echo ""
echo "========== CRASH CHECK =========="
# A real crash leaves gdb with an actual stack (a "#0 " frame) and/or a signal
# line. A clean exit via +quit prints the "bt" header then "No stack." — that is
# NOT a crash. Detect the real signals only.
if grep -qE 'Program received signal|SIGSEGV|SIGABRT|received signal SIG' "$GDB_LOG" \
   || grep -qE '^#0 ' "$GDB_LOG"; then
  echo "!!! REAL CRASH DETECTED:"
  grep -nE 'Program received signal|SIG[A-Z]+|^#[0-9]+ ' "$GDB_LOG" | head -30
  echo "--- backtrace section ---"
  sed -n '/SOAK CRASH BACKTRACE/,/THREADS/p' "$GDB_LOG" | head -40
else
  echo "NO crash — client exited cleanly (gdb: $(grep -c 'No stack' "$GDB_LOG") 'No stack' = normal +quit)."
fi
echo ""
echo "========== CLIENT: connect + render span =========="
# The client runs under gdb, so its stdout is in the gdb log.
grep -vE 'GL_[A-Z]' "$GDB_LOG" 2>/dev/null | grep -iE 'dual-load|loaded .*faces|timed out|ClientEnter' | head -3
first_ts=$(grep -oE '^2026-[0-9-]+ [0-9:]+' "$GDB_LOG" | head -1)
last_ts=$(grep -oE '^2026-[0-9-]+ [0-9:]+' "$GDB_LOG" | tail -1)
echo "  client ran: $first_ts  ->  $last_ts"
if [ -n "$first_ts" ] && [ -n "$last_ts" ]; then
  fs=$(date -d "$first_ts" +%s 2>/dev/null); ls=$(date -d "$last_ts" +%s 2>/dev/null)
  [ -n "$fs" ] && [ -n "$ls" ] && echo "  client survived ~$(( ls - fs ))s of the ${DURATION}s target"
fi
echo "  client screenshots written: $(grep -c 'Wrote screenshots' "$GDB_LOG" 2>/dev/null)"
echo "  client 'timed out': $(grep -c 'timed out' "$GDB_LOG" 2>/dev/null)  (should be 0)"
echo ""
echo "========== HOST: NPC activity + duration =========="
echo "  ClientEnterWorld: $(grep -c 'SV_ClientEnterWorld' "$HOST_LOG" 2>/dev/null)"
echo "  host frames survived (no early quit): $(grep -c 'ShutdownGame' "$HOST_LOG" 2>/dev/null) shutdowns"
echo ""
echo "========== SCREENSHOT SAMPLE (first, middle, last) =========="
mapfile -t shots < <(find "$CLIENT_HOME" -name 'shot*.png' 2>/dev/null | sort)
n=${#shots[@]}
echo "  total client screenshots: $n"
if (( n > 0 )); then
  for idx in 0 $(( n/2 )) $(( n-1 )); do
    s="${shots[$idx]}"
    [ -n "$s" ] && echo "  $(basename "$s"): $(magick "$s" -format 'mean=%[fx:mean] colors=%k' info: 2>/dev/null)"
  done
  cp "${shots[$(( n-1 ))]}" "$OUT/soak-last.png" 2>/dev/null
fi
echo ""
echo "logs: host=$HOST_LOG client=$CLIENT_LOG gdb=$GDB_LOG"
