#!/usr/bin/env bash
set -euo pipefail

kerf=${KERF:-"$HOME/src/kerf/.venv/bin/kerf"}
kernel=${KERNEL:-"$HOME/src/linux/vmlinux"}
artifact_dir=${ARTIFACT_DIR:-"$HOME/multikernel-artifacts/daxfs-current"}
name=${DAXFS_INSTANCE:-daxfs-demo}
instance_dir="/sys/fs/multikernel/instances/$name"

sudo mkdir -p /sys/fs/multikernel
mountpoint -q /sys/fs/multikernel || \
    sudo mount -t multikernel none /sys/fs/multikernel

if [[ -d "$instance_dir" ]]; then
    echo "$name already exists with status $(cat "$instance_dir/status")"
    sudo "$kerf" show
    exit 0
fi

remaining=$(find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 \
    -type d 2>/dev/null | wc -l)
if [[ $remaining != 0 ]]; then
    echo "refusing to reconfigure the pool: $remaining unrelated instance(s) exist" >&2
    exit 1
fi

sudo "$kerf" init --cpus=8,10,12,14 --memory=12GB \
    --devices=none --verbose
set +e
sudo "$kerf" create "$name" --id=51 --cpus=8,10,12,14 \
    --memory=6GB --verbose
create_rc=$?
set -e
[[ -d "$instance_dir" ]] || exit "$create_rc"

sudo "$kerf" load "$name" --kernel="$kernel" \
    --initrd="$artifact_dir/initramfs.cpio.gz" \
    --rootfs-dir="$artifact_dir/rootfs" \
    --cmdline='rdinit=/init daxfs.phase=root loglevel=7 panic=-1' \
    --entrypoint=/bin/daxfs-proof --console=mktty0 --verbose

set +e
sudo timeout 30s script -qefc "$kerf exec $name --console --verbose" \
    "$artifact_dir/$name-console.txt"
console_rc=$?
set -e
[[ $console_rc = 0 || $console_rc = 124 ]]
test "$(cat "$instance_dir/status")" = active
grep -F DAXFS_ROOT_READY "$artifact_dir/$name-console.txt"
grep -F DAXFS_MANIFEST_READY "$artifact_dir/$name-console.txt"
grep -F 'none / daxfs ' "$artifact_dir/$name-console.txt"
sudo "$kerf" show
echo "$name is active; use make daxfs-down to return its resources"
