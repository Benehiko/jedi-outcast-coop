#!/usr/bin/env bash
#
# JK2 co-op — Linux installer.
#
# Stages the engine data directory (~/.local/share/openjo/base) with symlinks to
# the retail assets and the built co-op gamecode, and installs two launcher
# commands (jk2coop-host, jk2coop-join) into ~/.local/bin.
#
# It never copies or modifies retail files — it only creates symlinks into your
# existing Steam install and small launcher scripts. `--uninstall` removes
# exactly what it created (tracked in a manifest), and re-running is idempotent.
#
# Usage:
#   tools/install-coop.sh [--gamedata PATH] [--uninstall] [--help]
#
#   --gamedata PATH   Point at your JK2 "GameData" directory explicitly (the one
#                     containing base/assets0.pk3). Needed if your install is not
#                     under a standard Steam library (e.g. a NAS mount).
#   --uninstall       Remove everything this installer created.
set -euo pipefail

# ---------------------------------------------------------------------------
# Paths
# ---------------------------------------------------------------------------
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BUILD="${JK2_BUILD:-$ROOT/openjk/build}"
ENGINE_BIN="$BUILD/openjo_sp.x86_64"
GAMECODE_SO="$BUILD/codeJK2/game/jospgamex86_64.so"
RENDERER_SO="$BUILD/rdjosp-vanilla_x86_64.so"

DATA_DIR="${JK2_DATA_DIR:-$HOME/.local/share/openjo}"
BASE_DIR="$DATA_DIR/base"
BIN_DIR="${JK2_BIN_DIR:-$HOME/.local/bin}"
MANIFEST="$DATA_DIR/.coop-install-manifest"

HOST_LAUNCHER="$BIN_DIR/jk2coop-host"
JOIN_LAUNCHER="$BIN_DIR/jk2coop-join"

DEFAULT_PORT=29070
DEFAULT_MAP=kejim_post
SECOND_CLIENT_HOME=/tmp/jk2-client2

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
say()  { printf '%s\n' "$*"; }
info() { printf '  %s\n' "$*"; }
die()  { printf 'error: %s\n' "$*" >&2; exit 1; }

# Record a path we created so --uninstall can remove exactly it.
manifest_add() {
    # Avoid duplicate lines on idempotent re-runs.
    if [[ ! -f "$MANIFEST" ]] || ! grep -qxF "$1" "$MANIFEST" 2>/dev/null; then
        printf '%s\n' "$1" >> "$MANIFEST"
    fi
}

# Create (or refresh) a symlink and record it. Idempotent.
link() {
    local target="$1" linkpath="$2"
    ln -sfn "$target" "$linkpath"
    manifest_add "$linkpath"
}

# ---------------------------------------------------------------------------
# GameData autodetection
# ---------------------------------------------------------------------------
# Print the first GameData dir that contains base/assets0.pk3, or nothing.
detect_gamedata() {
    local roots=(
        "$HOME/.steam/steam"
        "$HOME/.local/share/Steam"
        "$HOME/.steam/root"
    )
    # Candidate library roots: the standard roots plus any extra libraries
    # listed in libraryfolders.vdf under them.
    local libs=()
    local r
    for r in "${roots[@]}"; do
        [[ -d "$r/steamapps" ]] && libs+=("$r")
        local vdf="$r/steamapps/libraryfolders.vdf"
        if [[ -f "$vdf" ]]; then
            # Extract the "path"  "…"  values. Handles both the modern nested
            # and the legacy flat libraryfolders.vdf formats.
            while IFS= read -r p; do
                [[ -n "$p" && -d "$p/steamapps" ]] && libs+=("$p")
            done < <(grep -oE '"path"[[:space:]]*"[^"]+"' "$vdf" 2>/dev/null \
                     | sed -E 's/.*"path"[[:space:]]*"([^"]+)".*/\1/')
        fi
    done

    local lib gd
    for lib in "${libs[@]}"; do
        gd="$lib/steamapps/common/Jedi Outcast/GameData"
        if [[ -f "$gd/base/assets0.pk3" ]]; then
            printf '%s\n' "$gd"
            return 0
        fi
    done
    return 1
}

# ---------------------------------------------------------------------------
# Uninstall
# ---------------------------------------------------------------------------
do_uninstall() {
    say "Uninstalling JK2 co-op…"
    if [[ ! -f "$MANIFEST" ]]; then
        info "no install manifest at $MANIFEST — nothing tracked to remove."
        return 0
    fi
    # Collect the tracked directories to try last, and remove files/symlinks now.
    # Remove in reverse order (files before the dirs that may hold them).
    local line
    local dirs=()
    while IFS= read -r line; do
        [[ -z "$line" ]] && continue
        if [[ -L "$line" || -f "$line" ]]; then
            rm -f "$line"
            info "removed $line"
        elif [[ -d "$line" ]]; then
            dirs+=("$line")
        fi
    done < <(tac "$MANIFEST")

    # The manifest itself lives under the data dir, so remove it before trying to
    # rmdir that directory, else the dir is never empty.
    rm -f "$MANIFEST"
    info "removed manifest"

    # Now rmdir tracked directories that we created and that are empty, deepest
    # first (a child must go before its parent, or the parent is never empty).
    # Never force-remove: if a dir still holds files we did not create (e.g. the
    # user's own homepath contents), leave it in place.
    local d
    while IFS= read -r d; do
        [[ -n "$d" ]] || continue
        rmdir "$d" 2>/dev/null && info "removed dir $d" || true
    done < <(printf '%s\n' "${dirs[@]}" | awk '{print gsub(/\//,"/"), $0}' | sort -rn | cut -d' ' -f2-)

    say "Done. Retail files and your Steam install were never touched."
}

# ---------------------------------------------------------------------------
# Install
# ---------------------------------------------------------------------------
do_install() {
    local gamedata="$1"

    say "Installing JK2 co-op…"

    # Preconditions: the build must exist.
    [[ -x "$ENGINE_BIN"   ]] || die "engine not built: $ENGINE_BIN (build it per README first)"
    [[ -f "$GAMECODE_SO"  ]] || die "gamecode not built: $GAMECODE_SO"
    [[ -f "$RENDERER_SO"  ]] || die "renderer not built: $RENDERER_SO"

    # Resolve GameData.
    if [[ -z "$gamedata" ]]; then
        say "Locating your Jedi Outcast GameData…"
        gamedata="$(detect_gamedata || true)"
        [[ -n "$gamedata" ]] || die "could not find GameData under any Steam library.
       Pass it explicitly:  tools/install-coop.sh --gamedata /path/to/Jedi Outcast/GameData"
    fi
    [[ -f "$gamedata/base/assets0.pk3" ]] || \
        die "invalid --gamedata: no base/assets0.pk3 under: $gamedata"
    info "GameData: $gamedata"

    # Stage the engine data dir.
    mkdir -p "$BASE_DIR"; manifest_add "$BASE_DIR"; manifest_add "$DATA_DIR"
    say "Staging $BASE_DIR"
    local pk3 count=0
    shopt -s nullglob
    for pk3 in "$gamedata"/base/assets*.pk3; do
        link "$pk3" "$BASE_DIR/$(basename "$pk3")"
        count=$((count + 1))
    done
    shopt -u nullglob
    (( count > 0 )) || die "no assets*.pk3 found in $gamedata/base"
    info "linked $count asset pak(s)"

    # The co-op gamecode the host + a dual-load client both load.
    link "$GAMECODE_SO" "$BASE_DIR/$(basename "$GAMECODE_SO")"
    info "linked gamecode $(basename "$GAMECODE_SO")"

    # The renderer is loaded relative to the engine binary; it already lives in
    # the build dir beside openjo_sp.x86_64, so the launchers run from there.
    [[ -e "$BUILD/$(basename "$RENDERER_SO")" ]] || \
        die "renderer not beside engine binary in $BUILD (expected by the loader)"

    # Launchers.
    mkdir -p "$BIN_DIR"; manifest_add "$BIN_DIR"
    say "Installing launchers in $BIN_DIR"

    cat > "$HOST_LAUNCHER" <<EOF
#!/usr/bin/env bash
# jk2coop-host [map] — start a co-op game others can join. Generated by install-coop.sh.
exec "$ENGINE_BIN" \\
    +set fs_basepath "$DATA_DIR" \\
    +set net_enabled 1 +set net_port $DEFAULT_PORT \\
    +map "\${1:-$DEFAULT_MAP}"
EOF
    chmod +x "$HOST_LAUNCHER"; manifest_add "$HOST_LAUNCHER"
    info "jk2coop-host"

    cat > "$JOIN_LAUNCHER" <<EOF
#!/usr/bin/env bash
# jk2coop-join <host[:port]> [--second] — join a co-op game. Generated by install-coop.sh.
set -euo pipefail
if [[ \$# -lt 1 || "\${1:-}" == "-h" || "\${1:-}" == "--help" ]]; then
    echo "usage: jk2coop-join <host[:port]> [--second]" >&2
    exit 1
fi
host="\$1"; shift || true
case "\$host" in *:*) : ;; *) host="\$host:$DEFAULT_PORT" ;; esac

args=( +set fs_basepath "$DATA_DIR" +set net_enabled 1 )
if [[ "\${1:-}" == "--second" ]]; then
    # A second client ON THE SAME MACHINE needs its own clean fs_homepath, and
    # its own copy of the gamecode there (Sys_LoadSPGameDll searches homepath
    # first and does not fall back to basepath for the game .so).
    rm -rf "$SECOND_CLIENT_HOME"
    mkdir -p "$SECOND_CLIENT_HOME/base"
    ln -sfn "$GAMECODE_SO" "$SECOND_CLIENT_HOME/base/$(basename "$GAMECODE_SO")"
    args+=( +set fs_homepath "$SECOND_CLIENT_HOME" )
fi
exec "$ENGINE_BIN" "\${args[@]}" +connect "\$host"
EOF
    chmod +x "$JOIN_LAUNCHER"; manifest_add "$JOIN_LAUNCHER"
    info "jk2coop-join"

    say ""
    say "Installed. Try:"
    say "    jk2coop-host                      # host on port $DEFAULT_PORT"
    say "    jk2coop-join 127.0.0.1 --second   # join from a second local client"
    case ":$PATH:" in
        *":$BIN_DIR:"*) : ;;
        *) say ""; say "note: $BIN_DIR is not on your PATH; add it or call the launchers by full path." ;;
    esac
}

# ---------------------------------------------------------------------------
# Argument parsing
# ---------------------------------------------------------------------------
GAMEDATA=""
ACTION=install
while [[ $# -gt 0 ]]; do
    case "$1" in
        --gamedata) GAMEDATA="${2:?--gamedata needs a PATH}"; shift 2 ;;
        --gamedata=*) GAMEDATA="${1#*=}"; shift ;;
        --uninstall) ACTION=uninstall; shift ;;
        -h|--help)
            sed -n '2,20p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
            exit 0 ;;
        *) die "unknown argument: $1 (see --help)" ;;
    esac
done

case "$ACTION" in
    install)   do_install "$GAMEDATA" ;;
    uninstall) do_uninstall ;;
esac
