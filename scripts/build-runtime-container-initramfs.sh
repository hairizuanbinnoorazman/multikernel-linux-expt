#!/usr/bin/env bash
set -euo pipefail

bundle=${1:?usage: build-runtime-container-initramfs.sh BUNDLE OUTPUT}
output=${2:?usage: build-runtime-container-initramfs.sh BUNDLE OUTPUT}
output_manifest=${output%.cpio.gz}.manifest.json
source_manifest=${output%.cpio.gz}.source-manifest.json
storage_output=${MK_STORAGE_OUTPUT:-$(dirname "$output")/root.ext4}
storage_metadata=$(dirname "$output")/storage.json
manifest=${MK_KERNEL_MANIFEST:-/etc/mkruntime/kernels/gce-mk2.json}
manifest_name=${MK_KERNEL_MANIFEST_NAME:-gce-mk2}
task_identity=${MK_TASK_IDENTITY:-}
storage_port=${MK_STORAGE_PORT:-}
script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo_dir=$(cd "$script_dir/.." && pwd)
guest_init=${MK_GUEST_INIT:-$repo_dir/guest/mk-agent-init}
bootstrap_init=${MK_STORAGE_BOOTSTRAP_INIT:-$repo_dir/guest/runtime-mediated-init}
scratch=$(mktemp -d)
root=$scratch/root
boot=$scratch/boot
metadata=$scratch/metadata
complete=false
cleanup() {
	rm -rf "$scratch"
	if [[ $complete != true ]]; then
		rm -f "$output" "$output_manifest" "$source_manifest" "$storage_output" "$storage_metadata" \
			"$source_manifest.before" "$source_manifest.after"
	fi
}
trap cleanup EXIT

test -f "$bundle/config.json"
mkdir -p "$root" "$boot" "$metadata"
validated_config=$metadata/config.json
"$script_dir/validate-runtime-oci.py" "$bundle/config.json" "$validated_config"
: "${task_identity:?MK_TASK_IDENTITY is required}"
: "${storage_port:?MK_STORAGE_PORT is required}"
busybox=${BUSYBOX:-$(command -v busybox)}
nbd_helper=${MK_NBD_HELPER:-/usr/local/libexec/multikernel/mkvsock-nbd}
nbd_module=${MK_NBD_MODULE:-$(modinfo -n nbd)}
bootstrap=$metadata/bootstrap.json
"$script_dir/validate-runtime-bootstrap.py" "$manifest" "$manifest_name" >"$bootstrap"
agent=$(jq -er '.agent' "$bootstrap")
relay=$(jq -er '.relay' "$bootstrap")
module=$(jq -er '.module' "$bootstrap")
kernel_release=$(jq -er '.kernel_release' "$bootstrap")

for artifact in "$busybox" "$nbd_helper" "$nbd_module" "$guest_init" "$bootstrap_init"; do
	test -f "$artifact" && test ! -L "$artifact"
	mode=$(stat -c %a "$artifact")
	(( (8#$mode & 022) == 0 )) || { echo "unsafe writable bootstrap artifact: $artifact" >&2; exit 1; }
done
test "$(stat -c %u "$nbd_helper")" = 0
test "$(stat -c %u "$nbd_module")" = 0
test "$(modinfo -F name "$nbd_module")" = nbd
case "$(modinfo -F vermagic "$nbd_module")" in
	"$kernel_release "*) ;;
	*) echo "NBD module vermagic differs from selected child kernel" >&2; exit 1 ;;
esac

mkdir -p "$root"/{bin,dev,proc,sys,tmp,bundle/rootfs,.multikernel}
install -m 0755 "$busybox" "$root/bin/busybox"
install -m 0755 "$agent" "$root/mk-agent"
install -m 0755 "$relay" "$root/mkvsock-relay"
install -m 0755 "$guest_init" "$root/init"

# Containerd remains the registry, content, layer, and snapshot owner. The
# source is mounted read-only by the adapter and scanned before and after copy.
root_path=$(jq -er '.root.path' "$validated_config")
source_root=$("$script_dir/validate-runtime-root.py" "$bundle" "$validated_config")
"$script_dir/validate-runtime-image.py" "$source_root" "$validated_config" "$bootstrap" \
	>"$metadata/image-validation.json"
"$script_dir/build-runtime-rootfs.py" "$source_root" "$metadata/unused" "$source_manifest.before" --manifest-only \
	--max-bytes "${MK_ROOTFS_MAX_BYTES:-1073741824}" --max-inodes "${MK_ROOTFS_MAX_INODES:-131072}" \
	>"$metadata/source-before-result.json"
"$script_dir/runtime-storage-identity.py" "$task_identity" "$source_manifest.before" >"$metadata/storage-identity.json"
image_id=$(jq -er '.image_id' "$metadata/storage-identity.json")
filesystem_uuid=$(jq -er '.filesystem_uuid' "$metadata/storage-identity.json")
printf '%s\n' "$image_id" >"$root/.multikernel/image-id"
chmod 0600 "$root/.multikernel/image-id"
cp -a "$source_root/." "$root/bundle/rootfs/"
"$script_dir/build-runtime-rootfs.py" "$source_root" "$metadata/unused" "$source_manifest.after" --manifest-only \
	--max-bytes "${MK_ROOTFS_MAX_BYTES:-1073741824}" --max-inodes "${MK_ROOTFS_MAX_INODES:-131072}" \
	>"$metadata/source-after-result.json"
cmp -s "$source_manifest.before" "$source_manifest.after" || {
	echo 'OCI source root mutated during storage construction' >&2
	exit 1
}
mv "$source_manifest.after" "$source_manifest"
rm -f "$source_manifest.before"
jq '.root.path = "rootfs"' "$validated_config" >"$root/bundle/config.json"

"$script_dir/build-runtime-storage.py" "$root" "$storage_output" "$storage_metadata" \
	--image-id "$image_id" --uuid "$filesystem_uuid" --port "$storage_port" \
	--size "${MK_STORAGE_SIZE_BYTES:-2147483648}" --inodes "${MK_STORAGE_INODES:-262144}" \
	--min-free-bytes "${MK_STORAGE_MIN_FREE_BYTES:-1073741824}" >"$metadata/storage-result.json"

mkdir -p "$boot/bin" "$boot/lib"
install -m 0755 "$busybox" "$boot/bin/busybox"
install -m 0755 "$nbd_helper" "$boot/bin/mkvsock-nbd"
install -m 0644 "$module" "$boot/lib/mk_transport.ko"
install -m 0644 "$nbd_module" "$boot/lib/nbd.ko"
install -m 0755 "$bootstrap_init" "$boot/init"
"$script_dir/build-runtime-rootfs.py" "$boot" "$output" "$output_manifest" \
	--max-bytes "${MK_BOOTSTRAP_MAX_BYTES:-134217728}" --max-inodes 128 \
	--min-free-bytes "${MK_INITRAMFS_MIN_FREE_BYTES:-1073741824}" >"$metadata/archive-result.json"
"$script_dir/verify-runtime-rootfs.py" "$output" "$output_manifest" >"$metadata/verification-result.json"
jq -e --slurpfile built "$metadata/archive-result.json" --slurpfile verified "$metadata/verification-result.json" \
	'$built[0].initramfs_sha256 == $verified[0].archive_sha256 and $built[0].manifest_sha256 == $verified[0].manifest_sha256' \
	>/dev/null

jq -n \
	--arg source_root "$source_root" --arg requested_root "$root_path" \
	--arg nbd_helper "$nbd_helper" --arg nbd_module "$nbd_module" \
	--arg nbd_helper_sha256 "$(sha256sum "$nbd_helper" | awk '{print $1}')" \
	--arg nbd_module_sha256 "$(sha256sum "$nbd_module" | awk '{print $1}')" \
	--slurpfile source_before "$metadata/source-before-result.json" \
	--slurpfile source_after "$metadata/source-after-result.json" \
	--slurpfile identity "$metadata/storage-identity.json" \
	--slurpfile image "$metadata/image-validation.json" \
	--slurpfile storage "$metadata/storage-result.json" \
	--slurpfile archive "$metadata/archive-result.json" \
	--slurpfile verification "$metadata/verification-result.json" \
	--slurpfile kernel "$bootstrap" \
	'{schema_version: 1, requested_root: $requested_root, source_root: $source_root,
	  source_scan_before: $source_before[0], source_scan_after: $source_after[0], source_identity: $identity[0],
	  image: $image[0], storage: $storage[0], generated_bootstrap: $archive[0],
	  verified_bootstrap: $verification[0], selected_kernel: $kernel[0],
	  nbd: {helper: $nbd_helper, helper_sha256: $nbd_helper_sha256, module: $nbd_module, module_sha256: $nbd_module_sha256}}'
complete=true
