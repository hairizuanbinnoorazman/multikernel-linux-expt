#!/usr/bin/env bash
set -euo pipefail

kerf=${KERF:-"$HOME/src/kerf/.venv/bin/kerf"}
kernel=${KERNEL:-"$HOME/src/linux/vmlinux"}
lab_dir=${LAB_DIR:-"$HOME/multikernel-linux-lab"}
initrd=${INITRD:-"$HOME/multikernel-artifacts/child-initramfs.cpio.gz"}
cmdline='rdinit=/init console=mktty0 loglevel=7 panic=-1'

sudo mkdir -p /sys/fs/multikernel
mountpoint -q /sys/fs/multikernel || \
    sudo mount -t multikernel none /sys/fs/multikernel

"$lab_dir/scripts/build-initramfs.sh" "$lab_dir/guest/init" "$initrd"

# Do not add --report here: Kerf v0.2.0 returns after printing a report and
# never applies the requested pool.
sudo "$kerf" init \
    --cpus=8,10,12,14,9,11,13,15 --memory=16GB --devices=none --dry-run
sudo "$kerf" init \
    --cpus=8,10,12,14,9,11,13,15 --memory=16GB --devices=none --verbose

create_instance() {
    local name=$1 id=$2 cpus=$3 memory=$4
    if [[ -d "/sys/fs/multikernel/instances/$name" ]]; then
        return
    fi
    sudo "$kerf" create "$name" --id="$id" --cpus="$cpus" \
        --memory="$memory" --dry-run
    set +e
    sudo "$kerf" create "$name" --id="$id" --cpus="$cpus" \
        --memory="$memory" --verbose
    local rc=$?
    set -e
    if [[ ! -d "/sys/fs/multikernel/instances/$name" ]]; then
        return "$rc"
    fi
}

# Keep pool slack: an exact 8GB + 8GB split fails because kernel bookkeeping
# consumes a small part of the nominal 16GB pool.
create_instance smoke-a 1 8,10,12,14 8GB
create_instance smoke-b 2 9,11,13,15 7GB

sudo "$kerf" load smoke-a --kernel="$kernel" --initrd="$initrd" \
    --cmdline="$cmdline" --verbose
sudo "$kerf" load smoke-b --kernel="$kernel" --initrd="$initrd" \
    --cmdline="$cmdline" --verbose

capture_boot() {
    local name=$1
    set +e
    sudo timeout 20s script -qefc "$kerf exec $name --console --verbose" /dev/null
    local rc=$?
    set -e
    if [[ $rc -ne 0 && $rc -ne 124 ]]; then
        return "$rc"
    fi
    test "$(cat "/sys/fs/multikernel/instances/$name/status")" = active
}

capture_boot smoke-a
capture_boot smoke-b

test "$(cat /sys/fs/multikernel/instances/smoke-a/status)" = active
test "$(cat /sys/fs/multikernel/instances/smoke-b/status)" = active
systemctl is-active --quiet google-guest-agent
sudo "$kerf" show
echo "both child kernels are active; run make smoke-down to release resources"
