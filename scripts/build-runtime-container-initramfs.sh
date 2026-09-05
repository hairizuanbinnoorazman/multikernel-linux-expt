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
"$script_dir/validate-runtime-image.py" "$source_root" "$validated_config" "$bootstrap" \
	>"$work/image-validation.json"
"$script_dir/build-runtime-rootfs.py" "$source_root" "$work/unused" "$source_manifest.before" --manifest-only \
	--max-bytes "${MK_ROOTFS_MAX_BYTES:-1073741824}" --max-inodes "${MK_ROOTFS_MAX_INODES:-131072}" \
	>"$work/source-before-result.json"
cp -a "$source_root/." "$work/bundle/rootfs/"
"$script_dir/build-runtime-rootfs.py" "$source_root" "$work/unused" "$source_manifest.after" --manifest-only \
	--max-bytes "${MK_ROOTFS_MAX_BYTES:-1073741824}" --max-inodes "${MK_ROOTFS_MAX_INODES:-131072}" \
	>"$work/source-after-result.json"
cmp -s "$source_manifest.before" "$source_manifest.after" || {
	echo 'OCI source root mutated during initramfs construction' >&2
	exit 1
}
mv "$source_manifest.after" "$source_manifest"
rm -f "$source_manifest.before"
jq '.root.path = "rootfs"' "$validated_config" >"$work/bundle/config.json"

"$script_dir/build-runtime-rootfs.py" "$work" "$output" "$output_manifest" \
	--max-bytes "${MK_INITRAMFS_MAX_BYTES:-1207959552}" --max-inodes "${MK_INITRAMFS_MAX_INODES:-131200}" \
	--min-free-bytes "${MK_INITRAMFS_MIN_FREE_BYTES:-1073741824}" >"$work/archive-result.json"
"$script_dir/verify-runtime-rootfs.py" "$output" "$output_manifest" >"$work/verification-result.json"
jq -e --slurpfile built "$work/archive-result.json" --slurpfile verified "$work/verification-result.json" \
	'$built[0].initramfs_sha256 == $verified[0].archive_sha256 and $built[0].manifest_sha256 == $verified[0].manifest_sha256' \
	>/dev/null
jq -n \
	--arg source_root "$source_root" --arg requested_root "$root_path" \
	--slurpfile source_before "$work/source-before-result.json" \
	--slurpfile source_after "$work/source-after-result.json" \
	--slurpfile image "$work/image-validation.json" \
	--slurpfile archive "$work/archive-result.json" \
	--slurpfile verification "$work/verification-result.json" \
	--slurpfile kernel "$bootstrap" \
	'{schema_version: 1, requested_root: $requested_root, source_root: $source_root,
	  source_scan_before: $source_before[0], source_scan_after: $source_after[0], image: $image[0],
	  generated_archive: $archive[0], verified_archive: $verification[0],
	  selected_kernel: $kernel[0]}'
complete=true
