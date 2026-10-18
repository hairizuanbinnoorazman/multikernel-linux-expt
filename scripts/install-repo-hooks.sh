#!/usr/bin/env bash
set -euo pipefail
root=$(git rev-parse --show-toplevel)
cd "$root"
current=$(git config --local --get core.hooksPath || true)
if [[ -n $current && $current != .githooks ]]; then
    printf 'Existing custom hooksPath (%s) needs an explicit merge with .githooks.\n' "$current" >&2
    exit 1
fi
# Install the required clean/smudge filters locally. Repository hooks include
# all four Git LFS hooks, so changing hooksPath keeps uploads/checkouts working.
if [[ $current != .githooks ]]; then
    git lfs install --local
else
    git lfs install --local --skip-repo
fi
git config --local core.hooksPath .githooks
printf 'Repository hooks enabled: evidence and staged files over 1 MiB use Git LFS.\n'
