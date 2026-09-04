#!/usr/bin/env bash
set -euo pipefail

bundle=${1:?usage: build-runtime-container-initramfs.sh BUNDLE OUTPUT}
output=${2:?usage: build-runtime-container-initramfs.sh BUNDLE OUTPUT}
manifest=${MK_KERNEL_MANIFEST:-/etc/mkruntime/kernels/gce-mk2.json}
manifest_name=${MK_KERNEL_MANIFEST_NAME:-gce-mk2}
busybox=${BUSYBOX:-$(command -v busybox)}
script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
guest_init=${MK_GUEST_INIT:-"$script_dir/mk-agent-init"}
if [[ ! -f "$guest_init" ]]; then
	guest_init=$(cd "$script_dir/.." && pwd)/guest/mk-agent-init
fi
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

test -f "$bundle/config.json"
validated_config=$work/config.json
"$script_dir/validate-runtime-oci.py" "$bundle/config.json" "$validated_config"
bootstrap=$work/bootstrap.json
"$script_dir/validate-runtime-bootstrap.py" "$manifest" "$manifest_name" >"$bootstrap"
agent=$(jq -er '.agent' "$bootstrap")
relay=$(jq -er '.relay' "$bootstrap")
module=$(jq -er '.module' "$bootstrap")

mkdir -p "$work"/{bin,dev,proc,sys,tmp,bundle/rootfs}
install -m 0755 "$busybox" "$work/bin/busybox"
install -m 0755 "$agent" "$work/mk-agent"
install -m 0755 "$relay" "$work/mkvsock-relay"
install -m 0644 "$module" "$work/mk_transport.ko"
install -m 0755 "$guest_init" "$work/init"

# containerd remains the image/snapshot owner. ctr passes snapshot mounts for
# bundle/rootfs; Docker's containerd image store can instead pass an absolute
# OCI root.path. Copy either prepared root into a private per-sandbox artifact
# without modifying the snapshot.
root_path=$(jq -er '.root.path' "$validated_config")
if [[ "$root_path" = /* ]]; then
	source_root=$root_path
else
	source_root=$bundle/$root_path
fi
test -d "$source_root"
cp -a "$source_root/." "$work/bundle/rootfs/"
jq '.root.path = "rootfs"' "$validated_config" >"$work/bundle/config.json"

(cd "$work" && find . -xdev -print0 | sort -z | cpio --null -o --format=newc --owner=0:0 2>/dev/null) | gzip -n -9 >"$output"
chmod 0600 "$output"
sha256sum "$output"
