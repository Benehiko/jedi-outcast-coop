#!/usr/bin/env bash
#
# JK2 co-op — macOS installer.
#
# Stages the engine data directory (~/Library/Application Support/OpenJO/base)
# with symlinks to the retail assets and the built co-op gamecode, and installs
# two launcher commands (jk2coop-host, jk2coop-join) into ~/bin.
#
# It never copies or modifies retail files — it only creates symlinks into your
# existing Steam install and small launcher scripts. `--uninstall` removes
# exactly what it created (tracked in a manifest), and re-running is idempotent.
#
# macOS differs from the Linux installer in a few ways this script handles:
#   - the engine may be an .app bundle (openjo_sp.app) or a plain binary
#     (openjo_sp.<arch>); both are autodetected;
#   - the co-op gamecode / renderer are .dylib, not .so, and carry the build
#     architecture (x86_64 or arm64) in their name;
#   - the data dir lives under ~/Library/Application Support/OpenJO, and the
#     retail GameData lives under ~/Library/Application Support/Steam.
#
# Usage:
#   tools/install-coop-macos.sh [--gamedata PATH] [--uninstall] [--help]
#
#   --gamedata PATH   Point at your JK2 "GameData" directory explicitly (the one
#                     containing base/assets0.pk3). Needed if your install is not
#                     under the standard Steam library.
#   --uninstall       Remove everything this installer created.
set -euo pipefail

# ---------------------------------------------------------------------------
# Paths
# ---------------------------------------------------------------------------
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BUILD="${JK2_BUILD:-$ROOT/openjk/build}"

# The build architecture drives the dylib names and the plain-binary suffix.
# Respect an explicit override, else match the machine (Apple Silicon -> arm64).
ARCH="${JK2_ARCH:-}"
if [[ -z "$ARCH" ]]; then
    case "$(uname -m)" in
        arm64|aarch64) ARCH=arm64 ;;
        *)             ARCH=x86_64 ;;
    esac
fi

GAMECODE_DYLIB="$BUILD/codeJK2/game/jospgame${ARCH}.dylib"
RENDERER_NAME="rdjosp-vanilla_${ARCH}.dylib"

# The engine is either an .app bundle or a plain arch-suffixed executable,
# depending on the CMake MakeApplicationBundles option. Resolve whichever exists.
APP_BUNDLE="$BUILD/openjo_sp.app"
PLAIN_BIN="$BUILD/openjo_sp.${ARCH}"
ENGINE_BIN=""          # the actual executable we exec
ENGINE_DIR=""          # the dir the loader treats as fs_apppath (holds dylibs)
if [[ -x "$APP_BUNDLE/Contents/MacOS/openjo_sp" ]]; then
    ENGINE_BIN="$APP_BUNDLE/Contents/MacOS/openjo_sp"
    ENGINE_DIR="$APP_BUNDLE/Contents/MacOS"
elif [[ -x "$PLAIN_BIN" ]]; then
    ENGINE_BIN="$PLAIN_BIN"
    ENGINE_DIR="$BUILD"
fi

DATA_DIR="${JK2_DATA_DIR:-$HOME/Library/Application Support/OpenJO}"
BASE_DIR="$DATA_DIR/base"
BIN_DIR="${JK2_BIN_DIR:-$HOME/bin}"
MANIFEST="$DATA_DIR/.coop-install-manifest"

HOST_LAUNCHER="$BIN_DIR/jk2coop-host"
JOIN_LAUNCHER="$BIN_DIR/jk2coop-join"

DEFAULT_PORT=29070
DEFAULT_MAP=kejim_post
SECOND_CLIENT_HOME="${TMPDIR:-/tmp}/jk2-client2"

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
# BSD ln (macOS) supports -sfn just like GNU ln.
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
        "$HOME/Library/Application Support/Steam"
    )
    # Candidate library roots: the standard root plus any extra libraries listed
    # in libraryfolders.vdf under it.
    local libs=()
    local r
    for r in "${roots[@]}"; do
        [[ -d "$r/steamapps" ]] && libs+=("$r")
        local vdf="$r/steamapps/libraryfolders.vdf"
        if [[ -f "$vdf" ]]; then
            # Extract the "path" "…" values. Handles the modern nested and the
            # legacy flat libraryfolders.vdf formats.
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
    # Remove files/symlinks now; collect tracked directories to rmdir last.
    # Order among files does not matter (they are independent); the directories
    # are removed deepest-first below, so no manifest reversal is needed here
    # (avoids depending on a reverse tool — macOS has no tac, Linux no tail -r).
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
    done < "$MANIFEST"

    # The manifest itself lives under the data dir, so remove it before trying to
    # rmdir that directory, else the dir is never empty.
    rm -f "$MANIFEST"
    info "removed manifest"

    # rmdir tracked directories we created that are now empty, deepest first.
    # Never force-remove: leave any dir that still holds files we did not create.
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

    say "Installing JK2 co-op (macOS, ${ARCH})…"

    # Preconditions: the build must exist. The renderer sits beside the engine
    # binary (ENGINE_DIR): in a plain build that is the build dir; in an .app
    # build CMake's fixup_bundle has already copied it inside the bundle. Either
    # way the loader finds it there, so that is the one place we check.
    [[ -n "$ENGINE_BIN" ]] || die "engine not built: expected $APP_BUNDLE or $PLAIN_BIN (build it per README first; set JK2_ARCH if your build is a different arch)"
    [[ -f "$GAMECODE_DYLIB" ]] || die "gamecode not built: $GAMECODE_DYLIB"
    [[ -f "$ENGINE_DIR/$RENDERER_NAME" ]] || die "renderer not built beside engine: $ENGINE_DIR/$RENDERER_NAME"
    info "engine: $ENGINE_BIN"

    # Resolve GameData.
    if [[ -z "$gamedata" ]]; then
        say "Locating your Jedi Outcast GameData…"
        gamedata="$(detect_gamedata || true)"
        [[ -n "$gamedata" ]] || die "could not find GameData under your Steam library.
       Pass it explicitly:  tools/install-coop-macos.sh --gamedata '/path/to/Jedi Outcast/GameData'"
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

    # The co-op gamecode the host + a dual-load client both load. On macOS the SP
    # loader searches fs_homepath, fs_apppath (the binary's dir), and fs_basepath;
    # staging it into fs_basepath/base is enough for the host and for a joiner
    # that shares this basepath.
    link "$GAMECODE_DYLIB" "$BASE_DIR/$(basename "$GAMECODE_DYLIB")"
    info "linked gamecode $(basename "$GAMECODE_DYLIB")"

    # The co-op UI overlay (Co-op menu). Build it if it isn't built yet, then
    # stage it. It sorts after the retail assets so its ui/menus.txt wins.
    local coop_pk3="$ROOT/assets/coop-ui/zz-coop-ui.pk3"
    if [[ ! -f "$coop_pk3" && -x "$ROOT/tools/build-coop-ui-pk3.sh" ]]; then
        "$ROOT/tools/build-coop-ui-pk3.sh" >/dev/null 2>&1 || true
    fi
    if [[ -f "$coop_pk3" ]]; then
        link "$coop_pk3" "$BASE_DIR/zz-coop-ui.pk3"
        info "linked co-op UI overlay zz-coop-ui.pk3"
    fi

    # Launchers.
    mkdir -p "$BIN_DIR"; manifest_add "$BIN_DIR"
    say "Installing launchers in $BIN_DIR"

    cat > "$HOST_LAUNCHER" <<EOF
#!/usr/bin/env bash
# jk2coop-host [map] — start a co-op game others can join. Generated by install-coop-macos.sh.
exec "$ENGINE_BIN" \\
    +set fs_basepath "$DATA_DIR" \\
    +set net_enabled 1 +set net_port $DEFAULT_PORT \\
    +map "\${1:-$DEFAULT_MAP}"
EOF
    chmod +x "$HOST_LAUNCHER"; manifest_add "$HOST_LAUNCHER"
    info "jk2coop-host"

    cat > "$JOIN_LAUNCHER" <<EOF
#!/usr/bin/env bash
# jk2coop-join <host[:port]> [--second] — join a co-op game. Generated by install-coop-macos.sh.
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
    # its own copy of the gamecode there (the SP loader searches homepath first).
    rm -rf "$SECOND_CLIENT_HOME"
    mkdir -p "$SECOND_CLIENT_HOME/base"
    ln -sfn "$GAMECODE_DYLIB" "$SECOND_CLIENT_HOME/base/$(basename "$GAMECODE_DYLIB")"
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
            sed -n '2,29p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
            exit 0 ;;
        *) die "unknown argument: $1 (see --help)" ;;
    esac
done

case "$ACTION" in
    install)   do_install "$GAMEDATA" ;;
    uninstall) do_uninstall ;;
esac
