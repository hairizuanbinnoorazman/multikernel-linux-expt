#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 4 ]]; then
    echo "usage: $0 INIT PROBE_BINARY MK_TRANSPORT_KO OUTPUT" >&2
    exit 2
fi

init_file=$(realpath "$1")
probe_binary=$(realpath "$2")
transport_module=$(realpath "$3")
output_file=$(realpath -m "$4")
stage_dir=$(mktemp -d)
trap 'rm -rf -- "$stage_dir"' EXIT

mkdir -p "$stage_dir/bin" "$stage_dir/lib" "$(dirname "$output_file")"
cp /usr/bin/busybox "$stage_dir/bin/busybox"
cp "$probe_binary" "$stage_dir/bin/mkvsock-probe"
cp "$transport_module" "$stage_dir/lib/mk_transport.ko"
cp "$init_file" "$stage_dir/init"
chmod 0755 "$stage_dir/init" "$stage_dir/bin/busybox" \
    "$stage_dir/bin/mkvsock-probe"

(
    cd "$stage_dir"
    find . -print0 | cpio --null --create --format=newc --quiet | gzip -9 >"$output_file"
)

echo "created $output_file"
sha256sum "$output_file" "$probe_binary" "$transport_module"
ls -lh "$output_file"
