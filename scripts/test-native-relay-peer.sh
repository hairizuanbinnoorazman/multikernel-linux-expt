#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
build=$(mktemp -d)
trap 'rm -rf -- "$build"' EXIT
cc -O2 -Wall -Wextra -Werror "$root/scripts/test-native-relay-peer.c" -o "$build/control"
timeout 10 "$build/control"
