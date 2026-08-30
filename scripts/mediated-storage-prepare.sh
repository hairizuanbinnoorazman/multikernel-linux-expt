#!/usr/bin/env bash
set -euo pipefail

device_link=${MEDIATED_DEVICE:-/dev/disk/by-id/google-mk-mediated-storage}
expected_serial=${MEDIATED_SERIAL:-mk-mediated-storage}
expected_bytes=${MEDIATED_BYTES:-21474836480}
mountpoint=${MEDIATED_MOUNT:-/srv/multikernel-storage}
label=${MEDIATED_LABEL:-mk-mediated-host}

fail() {
    echo "MEDIATED_STORAGE_REFUSED reason=$1" >&2
    exit 2
}

[[ -L "$device_link" ]] || fail missing-by-id
device=$(readlink -f "$device_link")
[[ -b "$device" ]] || fail not-block-device
[[ "$(lsblk -dn -o TYPE "$device")" == disk ]] || fail not-whole-disk
[[ "$(lsblk -dn -b -o SIZE "$device")" == "$expected_bytes" ]] || fail size
[[ "$(lsblk -dn -o SERIAL "$device" | xargs)" == "$expected_serial" ]] || fail serial
[[ -z "$(lsblk -nr -o NAME "$device" | tail -n +2)" ]] || fail partitions
[[ -z "$(lsblk -dn -o MOUNTPOINTS "$device")" ]] || fail mounted
[[ -z "$(lsblk -dn -o FSTYPE "$device")" ]] || fail filesystem-present
[[ -z "$(wipefs -n "$device")" ]] || fail signature-present
[[ -z "$(ls -A "/sys/class/block/$(basename "$device")/holders")" ]] || fail holders

root_source=$(findmnt -n -o SOURCE /)
root_parent=$(lsblk -no PKNAME "$root_source" | head -1)
[[ "/dev/$root_parent" != "$device" ]] || fail root-parent
[[ ${MEDIATED_FIRST_FORMAT:-} == YES ]] || fail first-format-flag

echo "MEDIATED_STORAGE_IDENTITY device=$device serial=$expected_serial bytes=$expected_bytes"
mkfs.ext4 -L "$label" "$device"
mkdir -p "$mountpoint"
mount "$device" "$mountpoint"
uuid=$(blkid -s UUID -o value "$device")
echo "MEDIATED_STORAGE_READY device=$device mount=$mountpoint uuid=$uuid label=$label"
findmnt "$mountpoint"
tune2fs -l "$device" | sed -n '1,40p'
