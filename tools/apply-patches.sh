#!/usr/bin/env bash
#
# Apply this project's patches to the pinned OpenJK submodule.
#
# The submodule tracks upstream JACoders/OpenJK. Rather than carry a fork,
# local changes live in patches/ and are applied on top of the pinned
# commit.
#
# The patches are CUMULATIVE and OVERLAP: several touch the same lines (for
# example 0004 sets the sv_maxclients infostring to MAX_CLIENTS and 0020 later
# rewrites that same line to honour the runtime sv_maxclients cvar). They
# apply cleanly in order to a *pristine* submodule, but a single patch cannot be
# reliably reverse-checked against an already-fully-patched tree — its region may
# have been superseded by a later patch. So this script is NOT idempotent on a
# dirty tree: re-run it against a CLEAN submodule
#
#     git -C openjk checkout -- . && git -C openjk clean -fd
#     tools/apply-patches.sh
#
# rather than on top of a partially/fully patched one.
#
# Usage: tools/apply-patches.sh

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SUB="$ROOT/openjk"

if [[ ! -d "$SUB/.git" && ! -f "$SUB/.git" ]]; then
    echo "error: $SUB is not a git checkout; run: git submodule update --init" >&2
    exit 1
fi

shopt -s nullglob
patches=("$ROOT"/patches/*.patch)
if (( ${#patches[@]} == 0 )); then
    echo "no patches to apply"
    exit 0
fi

for p in "${patches[@]}"; do
    name="$(basename "$p")"
    if git -C "$SUB" apply --reverse --check "$p" 2>/dev/null; then
        echo "skip    $name (already applied)"
    elif git -C "$SUB" apply --check "$p" 2>/dev/null; then
        git -C "$SUB" apply "$p"
        echo "applied $name"
    else
        {
            echo "error: $name does not apply cleanly to the pinned submodule."
            echo "  The most common cause is a partially/fully patched submodule (these"
            echo "  patches overlap, so a re-run on a dirty tree can trip here). Reset the"
            echo "  submodule to pristine and run this script again:"
            echo "      git -C \"$SUB\" checkout -- . && git -C \"$SUB\" clean -fd"
            echo "      tools/apply-patches.sh"
            echo "  If it still fails on a clean submodule, the pinned commit or the patch"
            echo "  has drifted and the patch needs regenerating."
        } >&2
        exit 1
    fi
done
