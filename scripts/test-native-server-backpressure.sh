#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
build=$(mktemp -d)
trap 'rm -rf -- "$build"' EXIT
for link_mode in dynamic static; do
 flags=()
 [[ $link_mode != static ]] || flags=(-static)
 cc "${flags[@]}" -O2 -Wall -Wextra -Werror "$root/scripts/test-native-server-backpressure.c" -o "$build/controls"
 timeout 10 "$build/controls"
 cc "${flags[@]}" -O2 -Wall -Wextra -Werror -DMK_SERVER_QUEUE_BACKPRESSURE=1 "$root/scripts/test-native-server-pressure.c" -o "$build/server"
 timeout 20 "$build/server" "$build/image" "$build/server.log"
 printf 'NATIVE_SERVER_LINK_MODE_PASS mode=%s\n' "$link_mode"
done
