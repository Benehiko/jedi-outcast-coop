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
# OPTIONAL MODS
#   Beyond the core co-op install, the script can also enable optional game-file
#   mods, each of which just adds a `zz…` override pak to your base/ (never
#   touching retail data). On an interactive terminal it prompts y/N for each;
#   when run non-interactively (piped, CI) it enables none unless you pass flags.
#     * widescreen — add QHD / ultrawide / 4K resolutions to the video menu
#     * textures   — generate original AI material textures (needs a GPU+container)
#     * upscale    — Real-ESRGAN hi-res override from your own retail textures
#   See docs/widescreen.md, docs/asset-generation.md, docs/hires-textures.md.
#
# Usage:
#   tools/install-coop.sh [--gamedata PATH] [options] [--uninstall] [--help]
#
#   --gamedata PATH   Point at your JK2 "GameData" directory explicitly (the one
#                     containing base/assets0.pk3). Needed if your install is not
#                     under a standard Steam library (e.g. a NAS mount).
#   --with-widescreen Enable the widescreen/QHD/ultrawide video-menu mod.
#   --with-textures   Generate the AI material-texture pak (GPU + container).
#   --with-upscale    Build the Real-ESRGAN hi-res texture pak (GPU + container).
#   --combat MODE     Combat feel: 'modern' (default; free aim, fixed crosshair,
#                     FOV-independent sensitivity, faster bolts) or 'classic'
#                     (legacy auto-aim, dynamic crosshair, FOV-linked sensitivity).
#                     Written to base/autoexec_sp.cfg so it overrides stale configs.
#   --skip-cutscenes  Auto-skip scripted map-intro cutscenes (off by default).
#   --no-skip-cutscenes  Never auto-skip cutscenes (suppress the prompt).
#   --sensitivity N   Base mouse sensitivity for modern mode (default 0.5; the
#                     JK2 engine default is 5). Ignored with --combat classic.
#   --all             Enable every optional mod above.
#   --no-optional     Skip all optional-mod prompts (core install only).
#   --yes, -y         Assume "yes" to prompts (non-interactive; pairs with --all).
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

# Copy a file into base/ and record it (used for generated paks that live in the
# work/output dir rather than a stable repo path). Idempotent.
install_pak() {
    local src="$1" name="$2"
    cp -f "$src" "$BASE_DIR/$name"
    manifest_add "$BASE_DIR/$name"
}

# ---------------------------------------------------------------------------
# Optional mods
# ---------------------------------------------------------------------------
# Each optional mod is opt-in. Selection is resolved from flags first; on a TTY,
# anything still "ask" is prompted y/N; off a TTY, "ask" resolves to "no".
# Values: ask | yes | no.
OPT_WIDESCREEN=ask
OPT_TEXTURES=ask
OPT_UPSCALE=ask
ASSUME_YES=0

# Combat feel. The gamecode already ships modern defaults (free aim, fixed
# crosshair, faster bolts); this writes them explicitly into autoexec_sp.cfg so
# a stale, pre-existing openjo_sp.cfg can't override them, and lets the user pick
# the classic feel instead. COMBAT_MODE: modern | classic. Cutscene auto-skip is
# a separate opt-in (off by default, matching the gamecode default).
COMBAT_MODE=modern
OPT_SKIPCUTSCENES=ask
# Base mouse sensitivity written for modern combat. JK2's engine default is 5,
# which is fast on a modern high-DPI mouse; 0.5 is a calmer starting point.
# Only written in modern mode (classic leaves the engine/user value alone).
MOUSE_SENSITIVITY=0.5

# True if we can prompt the user (stdin is a real terminal).
is_interactive() { [[ -t 0 ]]; }

# Ask a yes/no question (default No). Returns 0 for yes.
# --yes auto-confirms a prompt that would otherwise be shown interactively; it
# does NOT turn an un-prompted (non-interactive) "ask" into a yes — use explicit
# --with-* / --all flags to enable mods in a non-interactive install.
confirm() {
    local prompt="$1" reply
    if ! is_interactive; then return 1; fi
    if (( ASSUME_YES == 1 )); then say "  $prompt [y/N] y (--yes)"; return 0; fi
    read -r -p "  $prompt [y/N] " reply || true
    [[ "$reply" =~ ^[Yy]([Ee][Ss])?$ ]]
}

# Resolve a tri-state (ask|yes|no) flag into a decision, prompting if "ask".
# $1 = current value, $2 = prompt text. Prints "yes" or "no".
resolve_opt() {
    local state="$1" prompt="$2"
    case "$state" in
        yes) echo yes ;;
        no)  echo no ;;
        *)   if confirm "$prompt"; then echo yes; else echo no; fi ;;
    esac
}

# Is a container runtime + AMD ROCm device available for the GPU mods?
have_gpu_container() {
    { command -v nerdctl >/dev/null 2>&1 || command -v podman >/dev/null 2>&1; } \
        && [[ -e /dev/kfd ]]
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
# Optional-mod installation
# ---------------------------------------------------------------------------
# Runs after the core install. For each mod, resolves the opt-in decision, and
# if yes either builds+links the pak or (for GPU mods without a runtime) prints
# the exact command to run later. All installed paks are manifest-tracked.
# Write autoexec_sp.cfg with the modern-combat cvars (or the classic feel), plus
# the optional cutscene auto-skip. The engine execs autoexec_sp.cfg on startup,
# after openjo_sp.cfg, so these values take effect even if an older config on
# disk persisted the legacy ones. Manifest-tracked so --uninstall removes it.
write_combat_config() {
    local cfg="$BASE_DIR/autoexec_sp.cfg"
    local skip aim xhair sens desc

    if [[ "$COMBAT_MODE" == classic ]]; then
        aim=1; xhair=1; sens=1
        desc="classic (legacy auto-aim, dynamic crosshair, FOV-linked sensitivity)"
    else
        aim=0; xhair=0; sens=0
        desc="modern (free aim, fixed crosshair, FOV-independent sensitivity)"
    fi

    if [[ "$(resolve_opt "$OPT_SKIPCUTSCENES" \
        "Auto-skip scripted map-intro cutscenes?")" == yes ]]; then
        skip=1
    else
        skip=0
    fi

    {
        echo "// Written by install-coop.sh — modern combat feel."
        echo "// Delete this file (or run the installer with --combat classic) to change it."
        echo "seta g_saberAutoAim \"$aim\""
        echo "seta cg_dynamicCrosshair \"$xhair\""
        echo "seta cg_fovSensitivityScale \"$sens\""
        echo "seta g_skipIntroCinematics \"$skip\""
        [[ "$COMBAT_MODE" == modern ]] && echo "seta sensitivity \"$MOUSE_SENSITIVITY\""
    } > "$cfg"
    manifest_add "$cfg"
    info "wrote autoexec_sp.cfg: combat=$desc, cutscene-skip=$skip"
}

do_optional_mods() {
    local gamedata="$1"
    local any=0

    # --- Widescreen / QHD / ultrawide video-menu modes ---------------------
    if [[ "$(resolve_opt "$OPT_WIDESCREEN" \
        "Add widescreen / QHD / ultrawide / 4K resolutions to the video menu?")" == yes ]]; then
        any=1
        local ws_tool="$ROOT/tools/build-widescreen-menu-pk3.sh"
        local ws_pak="$BASE_DIR/zz-widescreen-menu.pk3"
        if [[ -x "$ws_tool" ]]; then
            say "Enabling widescreen video-menu modes…"
            # The builder derives from the user's own retail menus in base/; point
            # it at the staged base (which links the retail assets*.pk3).
            if "$ws_tool" --assets "$BASE_DIR" --out "$ws_pak" >/dev/null 2>&1; then
                manifest_add "$ws_pak"
                info "installed zz-widescreen-menu.pk3 (SETUP > VIDEO > Video Mode)"
            else
                info "widescreen builder failed; run it manually: $ws_tool"
            fi
        else
            info "widescreen builder not found: $ws_tool"
        fi
    fi

    # --- Generated AI material textures (GPU + container) -------------------
    if [[ "$(resolve_opt "$OPT_TEXTURES" \
        "Generate original AI material textures? (needs a GPU + container, large download)")" == yes ]]; then
        any=1
        local tx_tool="$ROOT/tools/generate-textures.sh"
        local tx_pak="$BASE_DIR/zzz-generated-textures.pk3"
        if [[ ! -x "$tx_tool" ]]; then
            info "texture generator not found: $tx_tool"
        elif have_gpu_container; then
            say "Generating AI material textures (this can take a while)…"
            if "$tx_tool" --out "$tx_pak"; then
                manifest_add "$tx_pak"
                info "installed zzz-generated-textures.pk3"
            else
                info "texture generation failed; see docs/asset-generation.md"
            fi
        else
            info "no GPU container runtime detected (need nerdctl/podman + /dev/kfd)."
            info "run it later on a suitable machine:"
            info "    $tx_tool --out '$tx_pak'"
        fi
    fi

    # --- Real-ESRGAN hi-res texture upscale (GPU + container) ---------------
    if [[ "$(resolve_opt "$OPT_UPSCALE" \
        "Build a Real-ESRGAN hi-res texture override from your own retail textures? (needs a GPU + container)")" == yes ]]; then
        any=1
        local up_tool="$ROOT/tools/upscale-textures.sh"
        local up_pak="$BASE_DIR/zzz-hires-textures.pk3"
        if [[ ! -x "$up_tool" ]]; then
            info "upscaler not found: $up_tool"
        elif have_gpu_container; then
            say "Upscaling retail textures with Real-ESRGAN (this can take a while)…"
            info "note: the default Real-ESRGAN container image may be unavailable;"
            info "if the pull fails, pass a working image via --image (see docs/hires-textures.md)."
            if "$up_tool" --assets "$gamedata/base" --out "$up_pak"; then
                manifest_add "$up_pak"
                info "installed zzz-hires-textures.pk3"
            else
                info "upscale failed; see docs/hires-textures.md"
            fi
        else
            info "no GPU container runtime detected (need nerdctl/podman + /dev/kfd)."
            info "run it later on a suitable machine:"
            info "    $up_tool --assets '$gamedata/base' --out '$up_pak'"
        fi
    fi

    # --- Modern combat feel + optional cutscene skip -----------------------
    # Always writes autoexec_sp.cfg (the engine execs it at startup) so the
    # combat cvars win over any stale openjo_sp.cfg. `--combat classic` restores
    # the legacy feel; cutscene auto-skip is a separate opt-in.
    write_combat_config
    any=1

    (( any == 0 )) && info "no optional mods selected."
    return 0
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

    # Optional game-file mods (widescreen, textures, upscale).
    say ""
    say "Optional mods:"
    do_optional_mods "$gamedata"

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
        --with-widescreen) OPT_WIDESCREEN=yes; shift ;;
        --with-textures)   OPT_TEXTURES=yes; shift ;;
        --with-upscale)    OPT_UPSCALE=yes; shift ;;
        --combat) COMBAT_MODE="${2:?--combat needs modern|classic}"; shift 2 ;;
        --combat=*) COMBAT_MODE="${1#*=}"; shift ;;
        --skip-cutscenes)    OPT_SKIPCUTSCENES=yes; shift ;;
        --no-skip-cutscenes) OPT_SKIPCUTSCENES=no; shift ;;
        --sensitivity) MOUSE_SENSITIVITY="${2:?--sensitivity needs a number}"; shift 2 ;;
        --sensitivity=*) MOUSE_SENSITIVITY="${1#*=}"; shift ;;
        --all)             OPT_WIDESCREEN=yes; OPT_TEXTURES=yes; OPT_UPSCALE=yes; shift ;;
        --no-optional)     OPT_WIDESCREEN=no; OPT_TEXTURES=no; OPT_UPSCALE=no; OPT_SKIPCUTSCENES=no; shift ;;
        --yes|-y)          ASSUME_YES=1; shift ;;
        --uninstall) ACTION=uninstall; shift ;;
        -h|--help)
            sed -n '2,43p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
            exit 0 ;;
        *) die "unknown argument: $1 (see --help)" ;;
    esac
done

case "$COMBAT_MODE" in
    modern|classic) ;;
    *) die "--combat must be 'modern' or 'classic' (got: $COMBAT_MODE)" ;;
esac

[[ "$MOUSE_SENSITIVITY" =~ ^[0-9]+([.][0-9]+)?$ ]] || \
    die "--sensitivity must be a non-negative number (got: $MOUSE_SENSITIVITY)"

case "$ACTION" in
    install)   do_install "$GAMEDATA" ;;
    uninstall) do_uninstall ;;
esac
