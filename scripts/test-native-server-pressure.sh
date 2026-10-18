#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
build=$(mktemp -d)
trap 'rm -rf -- "$build"' EXIT
cc -O2 -Wall -Wextra -Werror "$root/scripts/test-native-server-pressure.c" -o "$build/fixture"
timeout 20 "$build/fixture" "$build/image" "$build/server.log"
