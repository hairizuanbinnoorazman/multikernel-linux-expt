#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
build=$(mktemp -d)
trap 'rm -rf -- "$build"' EXIT

cc -O2 -Wall -Wextra -Werror \
	"$root/scripts/test-mkvsock-nbd-timeouts.c" \
	-o "$build/test-mkvsock-nbd-timeouts"
"$build/test-mkvsock-nbd-timeouts"
