#!/usr/bin/env bash
#
# Apply this project's patches to the pinned OpenJK submodule.
#
# The submodule tracks upstream JACoders/OpenJK. Rather than carry a fork,
# local changes live in patches/ and are applied on top of the pinned
# commit. Re-running is safe: already-applied patches are skipped.
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
        echo "error: $name does not apply cleanly to the pinned submodule" >&2
        exit 1
    fi
done
