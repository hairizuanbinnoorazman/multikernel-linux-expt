#!/usr/bin/env bash
set -euo pipefail
set -x

source_root=${1:?usage: test-runtime-g4-input-boundaries-live.sh SOURCE_ROOT}
work_root=${MK_G4_INPUT_WORK_ROOT:-/var/tmp}
work=$(mktemp -d "$work_root"/mk-g4-input-boundaries.XXXXXX)
cleanup() {
	rm -rf -- "$work"
}
trap cleanup EXIT
export TMPDIR=$work

tests=(
	scripts/test-runtime-root-validation.py
	scripts/test-runtime-rootfs-build.py
	scripts/test-runtime-storage-build.py
	scripts/test-runtime-bind-materialization.py
)
for relative in "${tests[@]}"; do
	path=$source_root/$relative
	test -f "$path" && test ! -L "$path"
	python3 "$path" -v
done

echo G4_INPUT_BOUNDARIES_LIVE_PASS
