#!/usr/bin/env bash
set -euo pipefail

if [[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]]; then
	PS4='+${BASH_SOURCE}:${LINENO}: '
	set -x
fi

source_root=${1:?usage: test-runtime-capacity-enospc-live.sh SOURCE_ROOT}
scratch=$(mktemp -d -p /var/tmp mk-capacity-enospc.XXXXXX)
mountpoint=$scratch/constrained
small_root=$scratch/small-root
large_root=$scratch/large-root
inode_root=$scratch/inode-root

cleanup() (
	set +e
	mountpoint -q "$mountpoint" && sudo umount "$mountpoint"
	sudo rm -rf -- "$scratch"
)
trap cleanup EXIT

observe() {
	local key=$1
	shift
	printf 'OBSERVATION_BEGIN key=%s\n%s\nOBSERVATION_END key=%s\n' "$key" "$*" "$key"
}

mount_constrained() {
	local options=$1
	mountpoint -q "$mountpoint" && sudo umount "$mountpoint"
	mkdir -p "$mountpoint"
	sudo mount -t tmpfs -o "$options" tmpfs "$mountpoint"
	sudo chown "$(id -u):$(id -g)" "$mountpoint"
	chmod 0700 "$mountpoint"
	df -B1 "$mountpoint"
	df -i "$mountpoint"
}

assert_empty_mount() {
	[[ -z $(find "$mountpoint" -mindepth 1 -maxdepth 1 -print -quit) ]]
}

[[ $(id -u) -ne 0 ]] || { echo 'run as an ordinary sudo-capable user' >&2; exit 1; }
test -x "$source_root/scripts/build-runtime-storage.py"
test -x "$source_root/scripts/build-runtime-rootfs.py"
test -x "$source_root/scripts/test-runtime-real-enospc.py"
for service in mkruntimed mknetd containerd docker; do
	[[ $(systemctl is-active "$service") = active ]]
	[[ $(systemctl show -p NRestarts --value "$service") = 0 ]]
done

mkdir -m 0700 "$mountpoint" "$small_root" "$large_root" "$inode_root"
mkdir -m 0700 "$small_root/etc" "$large_root/etc" "$inode_root/crowded"
printf 'small\n' >"$small_root/etc/value"
dd if=/dev/urandom of="$large_root/etc/incompressible" bs=1M count=8 status=none
for index in $(seq 1 96); do printf x >"$inode_root/crowded/entry-$index"; done

observe provenance "boot_id=$(cat /proc/sys/kernel/random/boot_id)
selector=$(readlink -f /usr/local/lib/multikernel/current)
qualifier_sha256=$(sha256sum "$source_root/scripts/test-runtime-capacity-enospc-live.sh" | awk '{print $1}')
real_enospc_helper_sha256=$(sha256sum "$source_root/scripts/test-runtime-real-enospc.py" | awk '{print $1}')"

"$source_root/scripts/audit-runtime-final-resources-live.sh"

(
	cd "$source_root"
	PYTHONDONTWRITEBYTECODE=1 TMPDIR="$scratch" python3 scripts/test-runtime-safe-publish.py
	PYTHONDONTWRITEBYTECODE=1 TMPDIR="$scratch" python3 scripts/test-runtime-storage-build.py
	PYTHONDONTWRITEBYTECODE=1 TMPDIR="$scratch" python3 scripts/test-runtime-rootfs-build.py
)
echo FOCUSED_CAPACITY_TESTS_PASS

mount_constrained size=96m,nr_inodes=4096
set +e
storage_high_output=$("$source_root/scripts/build-runtime-storage.py" "$small_root" "$mountpoint/root.ext4" "$mountpoint/root.json" \
	--image-id live-high-water --uuid 11111111-2222-4333-8444-555555555555 --port 4061 \
	--size $((64 << 20)) --inodes 4096 --min-free-bytes $((64 << 20)) --min-free-inodes 0 2>&1)
storage_high_status=$?
set -e
[[ $storage_high_status -ne 0 ]]
grep -Fq 'storage high-water refusal' <<<"$storage_high_output"
assert_empty_mount
observe storage-byte-high-water "status=$storage_high_status diagnostic_sha256=$(sha256sum <<<"$storage_high_output" | awk '{print $1}')"

set +e
storage_inode_output=$("$source_root/scripts/build-runtime-storage.py" "$small_root" "$mountpoint/root.ext4" "$mountpoint/root.json" \
	--image-id live-inode-high-water --uuid 11111111-2222-4333-8444-555555555555 --port 4061 \
	--size $((64 << 20)) --inodes 4096 --min-free-bytes 0 --min-free-inodes $((1 << 30)) 2>&1)
storage_inode_status=$?
set -e
[[ $storage_inode_status -ne 0 ]]
grep -Fq 'storage high-water refusal' <<<"$storage_inode_output"
assert_empty_mount
observe storage-inode-high-water "status=$storage_inode_status diagnostic_sha256=$(sha256sum <<<"$storage_inode_output" | awk '{print $1}')"

mount_constrained size=48m,nr_inodes=4096
PYTHONDONTWRITEBYTECODE=1 "$source_root/scripts/test-runtime-real-enospc.py" storage "$small_root" "$mountpoint"
assert_empty_mount
observe storage-real-block-enospc "result=clean"

mount_constrained size=96m,nr_inodes=32
PYTHONDONTWRITEBYTECODE=1 "$source_root/scripts/test-runtime-real-enospc.py" storage "$inode_root" "$mountpoint"
assert_empty_mount
observe storage-real-inode-enospc "result=clean"

mount_constrained size=4m,nr_inodes=4096
set +e
rootfs_high_output=$("$source_root/scripts/build-runtime-rootfs.py" "$large_root" "$mountpoint/initramfs" "$mountpoint/manifest" 2>&1)
rootfs_high_status=$?
set -e
[[ $rootfs_high_status -ne 0 ]]
grep -Fq 'high-water refusal' <<<"$rootfs_high_output"
assert_empty_mount
observe initramfs-byte-high-water "status=$rootfs_high_status diagnostic_sha256=$(sha256sum <<<"$rootfs_high_output" | awk '{print $1}')"

set +e
rootfs_inode_output=$("$source_root/scripts/build-runtime-rootfs.py" "$small_root" "$mountpoint/initramfs" "$mountpoint/manifest" \
	--min-free-inodes $((1 << 30)) 2>&1)
rootfs_inode_status=$?
set -e
[[ $rootfs_inode_status -ne 0 ]]
grep -Fq 'high-water refusal' <<<"$rootfs_inode_output"
assert_empty_mount
observe initramfs-inode-high-water "status=$rootfs_inode_status diagnostic_sha256=$(sha256sum <<<"$rootfs_inode_output" | awk '{print $1}')"

PYTHONDONTWRITEBYTECODE=1 "$source_root/scripts/test-runtime-real-enospc.py" rootfs "$large_root" "$mountpoint"
assert_empty_mount
observe initramfs-real-block-enospc "result=clean"

mount_constrained size=16m,nr_inodes=32
PYTHONDONTWRITEBYTECODE=1 "$source_root/scripts/test-runtime-real-enospc.py" rootfs-inodes "$small_root" "$mountpoint"
find "$mountpoint" -maxdepth 1 -type f -name 'filler-*' -delete
assert_empty_mount
observe initramfs-real-inode-enospc "result=clean"

sudo umount "$mountpoint"
"$source_root/scripts/audit-runtime-final-resources-live.sh"
for service in mkruntimed mknetd containerd docker; do
	[[ $(systemctl is-active "$service") = active ]]
	[[ $(systemctl show -p NRestarts --value "$service") = 0 ]]
done

trap - EXIT
sudo rm -rf -- "$scratch"
echo G4_CAPACITY_ENOSPC_LIVE_PASS
