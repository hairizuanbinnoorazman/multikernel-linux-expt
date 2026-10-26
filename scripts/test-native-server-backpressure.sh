#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
build=$(mktemp -d)
trap 'rm -rf -- "$build"' EXIT
cc -O2 -Wall -Wextra -Werror "$root/scripts/test-native-server-backpressure.c" -o "$build/controls"
timeout 10 "$build/controls"
cc -O2 -Wall -Wextra -Werror -DMK_SERVER_QUEUE_BACKPRESSURE=1 "$root/scripts/test-native-server-pressure.c" -o "$build/server"
timeout 20 "$build/server" "$build/image" "$build/server.log"
