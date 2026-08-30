#!/usr/bin/env bash
set -euo pipefail

kerf=${KERF:-"$HOME/src/kerf/.venv/bin/kerf"}
kernel=${KERNEL:-"$HOME/src/linux/vmlinux"}
lab_dir=${LAB_DIR:-"$HOME/multikernel-linux-lab"}
initrd=${INITRD:-"$HOME/multikernel-artifacts/ext4-bootstrap.cpio.gz"}
name=ext4-no-disk
expected_uuid=00000000-0000-0000-0000-000000000001
cmdline="rdinit=/init console=mktty0 loglevel=6 panic=-1 mk.root_uuid=$expected_uuid"

cleanup() {
    set +e
    if [[ -d "/sys/fs/multikernel/instances/$name" ]]; then
        status=$(cat "/sys/fs/multikernel/instances/$name/status")
        [[ "$status" != active ]] || sudo "$kerf" kill "$name" --force
        status=$(cat "/sys/fs/multikernel/instances/$name/status")
        [[ "$status" != loaded ]] || sudo "$kerf" unload "$name"
        sudo "$kerf" delete "$name"
    fi
    sudo "$kerf" init --cpus=none --memory=none --devices=none >/dev/null
}
trap cleanup EXIT

sudo mkdir -p /sys/fs/multikernel
mountpoint -q /sys/fs/multikernel || \
    sudo mount -t multikernel none /sys/fs/multikernel
test ! -e /sys/fs/multikernel/instances/smoke-a
test ! -e /sys/fs/multikernel/instances/smoke-b

"$lab_dir/scripts/build-initramfs.sh" \
    "$lab_dir/guest/ext4-bootstrap-init" "$initrd"

sudo "$kerf" init --cpus=8,10,12,14 --memory=6GB --devices=none --dry-run
sudo "$kerf" init --cpus=8,10,12,14 --memory=6GB --devices=none

set +e
sudo "$kerf" create "$name" --id=3 --cpus=8,10,12,14 \
    --memory=4GB --verbose
create_rc=$?
set -e
if [[ ! -d "/sys/fs/multikernel/instances/$name" ]]; then
    exit "$create_rc"
fi

sudo "$kerf" load "$name" --kernel="$kernel" --initrd="$initrd" \
    --cmdline="$cmdline"

console_log=$(mktemp)
set +e
sudo timeout 18s script -qefc "$kerf exec $name --console" /dev/null \
    >"$console_log" 2>&1
console_rc=$?
set -e
if [[ $console_rc -ne 0 && $console_rc -ne 124 ]]; then
    sed -n '1,240p' "$console_log"
    exit "$console_rc"
fi

sed -n '1,240p' "$console_log"
grep -q 'EXT4_BOOTSTRAP_FAIL reason=root-not-found' "$console_log"
grep -q 'EXT4_DIAGNOSTIC_READY' "$console_log"
test "$(cat "/sys/fs/multikernel/instances/$name/status")" = active
echo 'ext4 bootstrap absent-disk refusal passed'
