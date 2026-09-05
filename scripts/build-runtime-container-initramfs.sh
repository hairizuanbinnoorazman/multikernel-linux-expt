#!/usr/bin/env bash
set -euo pipefail

bundle=${1:?usage: build-runtime-container-initramfs.sh BUNDLE OUTPUT}
output=${2:?usage: build-runtime-container-initramfs.sh BUNDLE OUTPUT}
output_manifest=${output%.cpio.gz}.manifest.json
source_manifest=${output%.cpio.gz}.source-manifest.json
manifest=${MK_KERNEL_MANIFEST:-/etc/mkruntime/kernels/gce-mk2.json}
manifest_name=${MK_KERNEL_MANIFEST_NAME:-gce-mk2}
busybox=${BUSYBOX:-$(command -v busybox)}
script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
guest_init=${MK_GUEST_INIT:-"$script_dir/mk-agent-init"}
if [[ ! -f "$guest_init" ]]; then
	guest_init=$(cd "$script_dir/.." && pwd)/guest/mk-agent-init
fi
work=$(mktemp -d)
complete=false
cleanup() {
	rm -rf "$work"
	if [[ $complete != true ]]; then
		rm -f "$output" "$output_manifest" "$source_manifest" \
			"$source_manifest.before" "$source_manifest.after"
	fi
}
trap cleanup EXIT

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
source_root=$("$script_dir/validate-runtime-root.py" "$bundle" "$validated_config")
"$script_dir/build-runtime-rootfs.py" "$source_root" "$work/unused" "$source_manifest.before" --manifest-only \
	--max-bytes "${MK_ROOTFS_MAX_BYTES:-1073741824}" --max-inodes "${MK_ROOTFS_MAX_INODES:-131072}"
cp -a "$source_root/." "$work/bundle/rootfs/"
"$script_dir/build-runtime-rootfs.py" "$source_root" "$work/unused" "$source_manifest.after" --manifest-only \
	--max-bytes "${MK_ROOTFS_MAX_BYTES:-1073741824}" --max-inodes "${MK_ROOTFS_MAX_INODES:-131072}"
cmp -s "$source_manifest.before" "$source_manifest.after" || {
	echo 'OCI source root mutated during initramfs construction' >&2
	exit 1
}
mv "$source_manifest.after" "$source_manifest"
rm -f "$source_manifest.before"
jq '.root.path = "rootfs"' "$validated_config" >"$work/bundle/config.json"

"$script_dir/build-runtime-rootfs.py" "$work" "$output" "$output_manifest" \
	--max-bytes "${MK_INITRAMFS_MAX_BYTES:-1207959552}" --max-inodes "${MK_INITRAMFS_MAX_INODES:-131200}" \
	--min-free-bytes "${MK_INITRAMFS_MIN_FREE_BYTES:-1073741824}"
complete=true
