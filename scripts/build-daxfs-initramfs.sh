#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 3 ]]; then
    echo "usage: $0 INIT_FILE DAXFS_MODULE OUTPUT_FILE" >&2
    exit 2
fi

init_file=$(realpath "$1")
daxfs_module=$(realpath "$2")
output_file=$(realpath -m "$3")
stage_dir=$(mktemp -d)
trap 'rm -rf -- "$stage_dir"' EXIT

mkdir -p "$stage_dir/bin" "$(dirname "$output_file")"
cp /usr/bin/busybox "$stage_dir/bin/busybox"
cp "$init_file" "$stage_dir/init"
cp "$daxfs_module" "$stage_dir/daxfs.ko"
touch "$stage_dir/INITRAMFS_ONLY"
chmod 0755 "$stage_dir/init" "$stage_dir/bin/busybox"

(
    cd "$stage_dir"
    find . -print0 | cpio --null --create --format=newc --quiet | gzip -9 >"$output_file"
)

echo "created $output_file"
sha256sum "$output_file"
ls -lh "$output_file"
