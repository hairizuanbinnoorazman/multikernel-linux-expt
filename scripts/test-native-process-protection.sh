#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
build=$(mktemp -d)
trap 'rm -rf -- "$build"' EXIT
for kind in relay nbd; do
    flags=()
    [[ $kind != relay ]] || flags+=(-DTEST_RELAY)
    if [[ $# == 2 && $1 == --baseline ]]; then
        source_file=$build/mkvsock-$kind.c
        git -C "$root" show "$2:tools/mkvsock-$kind.c" > "$source_file"
        flags+=(-DBASELINE_ALLOWED "-DSOURCE_FILE=\"$source_file\"")
    elif [[ $# != 0 ]]; then
        echo 'usage: test-native-process-protection.sh [--baseline COMMIT]' >&2
        exit 2
    fi
    cc -O2 -Wall -Wextra -Werror -I "$root/tools" "${flags[@]}" \
        "$root/scripts/test-native-process-protection.c" -o "$build/$kind"
    timeout 10 "$build/$kind"
done
