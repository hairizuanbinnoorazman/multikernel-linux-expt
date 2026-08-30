#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 6 ]]; then
    echo "usage: $0 INIT NBD_BINARY MK_TRANSPORT_KO NBD_KO BUSYBOX OUTPUT" >&2
    exit 2
fi

init_file=$(realpath "$1")
nbd_binary=$(realpath "$2")
transport_module=$(realpath "$3")
nbd_module=$(realpath "$4")
busybox=$(realpath "$5")
output_file=$(realpath -m "$6")
stage_dir=$(mktemp -d)
trap 'rm -rf -- "$stage_dir"' EXIT

mkdir -p "$stage_dir/bin" "$stage_dir/lib" "$(dirname "$output_file")"
cp "$busybox" "$stage_dir/bin/busybox"
cp "$nbd_binary" "$stage_dir/bin/mkvsock-nbd"
cp "$transport_module" "$stage_dir/lib/mk_transport.ko"
cp "$nbd_module" "$stage_dir/lib/nbd.ko"
cp "$init_file" "$stage_dir/init"
chmod 0755 "$stage_dir/init" "$stage_dir/bin/busybox" "$stage_dir/bin/mkvsock-nbd"

(
    cd "$stage_dir"
    find . -print0 | cpio --null --create --format=newc --quiet | gzip -9 >"$output_file"
)
sha256sum "$output_file" "$nbd_binary" "$transport_module" "$nbd_module"
ls -lh "$output_file"
