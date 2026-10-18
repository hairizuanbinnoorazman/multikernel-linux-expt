#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
build=$(mktemp -d)
trap 'rm -rf -- "$build"' EXIT
cc -D_GNU_SOURCE -O2 -Wall -Wextra -Werror "$root/scripts/test-native-server-sandbox.c" -o "$build/fixture"
timeout 10 "$build/fixture" "$build/image"
