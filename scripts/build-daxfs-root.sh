#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
    echo "usage: $0 OUTPUT_DIRECTORY" >&2
    exit 2
fi

output_dir=$(realpath -m "$1")
script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo_dir=$(dirname "$script_dir")
daxfs_commit=${DAXFS_COMMIT:-11ab401585b79b4a7c9164019852e0219e197d13}
build_time=$(date -u +%Y-%m-%dT%H:%M:%SZ)
kerf_init=${KERF_INIT:-"$HOME/src/kerf/src/kerf/data/kerf-init"}

if [[ ! -x "$kerf_init" ]]; then
    echo "Kerf init binary not found or not executable: $kerf_init" >&2
    exit 1
fi

mkdir -p "$output_dir/bin" "$output_dir/dev" "$output_dir/proc" \
    "$output_dir/sys" "$output_dir/run" "$output_dir/tmp" \
    "$output_dir/mnt" "$output_dir/opt/proof/nested"
cp /usr/bin/busybox "$output_dir/bin/busybox"
cp "$repo_dir/guest/daxfs-proof.sh" "$output_dir/bin/daxfs-proof"
cp "$kerf_init" "$output_dir/init"
chmod 0755 "$output_dir/init" "$output_dir/bin/busybox" \
    "$output_dir/bin/daxfs-proof"
ln -sfn busybox "$output_dir/bin/sh"

cat >"$output_dir/DAXFS-MARKER" <<EOF
DAXFS_COMMIT=$daxfs_commit
BUILD_TIME=$build_time
FIXED_TEST_STRING=multikernel-daxfs-root-proof-v1
EOF
printf 'nested deterministic payload\n' >"$output_dir/opt/proof/nested/payload.txt"
: >"$output_dir/opt/proof/empty"
printf 'mode proof\n' >"$output_dir/opt/proof/mode-0640"
chmod 0640 "$output_dir/opt/proof/mode-0640"

# Generated manifests must not describe a stale prior manifest or hash the
# checksum file while it is still being written.
rm -f "$output_dir/MANIFEST.metadata" "$output_dir/MANIFEST.sha256"

(
    cd "$output_dir"
    find . -mindepth 1 -printf '%P\t%y\t%m\t%s\n' | LC_ALL=C sort >MANIFEST.metadata
    find . -type f ! -name MANIFEST.sha256 -print0 | \
        LC_ALL=C sort -z | xargs -0 sha256sum >MANIFEST.sha256
    sha256sum -c MANIFEST.sha256
)

echo "created deterministic DAXFS root at $output_dir"
sha256sum "$output_dir/MANIFEST.metadata" "$output_dir/MANIFEST.sha256"
