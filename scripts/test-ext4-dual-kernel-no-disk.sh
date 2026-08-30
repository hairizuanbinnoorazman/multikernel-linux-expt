#!/usr/bin/env bash
set -euo pipefail

kerf=${KERF:-"$HOME/src/kerf/.venv/bin/kerf"}
kernel_a=${KERNEL_A:-"$HOME/src/linux/vmlinux"}
kernel_b=${KERNEL_B:-"$HOME/src/linux-alt-worktree/vmlinux"}
lab_dir=${LAB_DIR:-"$HOME/multikernel-linux-lab"}
initrd=${INITRD:-"$HOME/multikernel-artifacts/ext4-bootstrap.cpio.gz"}

cleanup() {
    set +e
    for name in ext4-kernel-a ext4-kernel-b; do
        if [[ ! -d "/sys/fs/multikernel/instances/$name" ]]; then
            continue
        fi
        status=$(cat "/sys/fs/multikernel/instances/$name/status")
        [[ "$status" != active ]] || sudo "$kerf" kill "$name" --force
        status=$(cat "/sys/fs/multikernel/instances/$name/status")
        [[ "$status" != loaded ]] || sudo "$kerf" unload "$name"
        sudo "$kerf" delete "$name"
    done
    sudo "$kerf" init --cpus=none --memory=none --devices=none >/dev/null
}
trap cleanup EXIT

test -f "$kernel_a"
test -f "$kernel_b"
"$lab_dir/scripts/build-initramfs.sh" \
    "$lab_dir/guest/ext4-bootstrap-init" "$initrd"

sudo "$kerf" init \
    --cpus=8,10,12,14,9,11,13,15 --memory=16GB --devices=none --dry-run
sudo "$kerf" init \
    --cpus=8,10,12,14,9,11,13,15 --memory=16GB --devices=none

create_instance() {
    local name=$1 id=$2 cpus=$3 memory=$4
    set +e
    sudo "$kerf" create "$name" --id="$id" --cpus="$cpus" \
        --memory="$memory"
    local create_rc=$?
    set -e
    if [[ ! -d "/sys/fs/multikernel/instances/$name" ]]; then
        return "$create_rc"
    fi
}

create_instance ext4-kernel-a 4 8,10,12,14 8GB
create_instance ext4-kernel-b 5 9,11,13,15 7GB

sudo "$kerf" load ext4-kernel-a --kernel="$kernel_a" --initrd="$initrd" \
    --cmdline='rdinit=/init console=mktty0 loglevel=6 panic=-1 mk.root_uuid=aaaaaaaa-0000-0000-0000-000000000001'
sudo "$kerf" load ext4-kernel-b --kernel="$kernel_b" --initrd="$initrd" \
    --cmdline='rdinit=/init console=mktty0 loglevel=6 panic=-1 mk.root_uuid=bbbbbbbb-0000-0000-0000-000000000002'

capture_and_check() {
    local name=$1 expected_release=$2 expected_uuid=$3
    local console_log
    console_log=$(mktemp)
    set +e
    sudo timeout 18s script -qefc "$kerf exec $name --console" /dev/null \
        >"$console_log" 2>&1
    local console_rc=$?
    set -e
    if [[ $console_rc -ne 0 && $console_rc -ne 124 ]]; then
        sed -n '1,240p' "$console_log"
        return "$console_rc"
    fi
    sed -n '1,240p' "$console_log"
    grep -q "release=$expected_release" "$console_log"
    grep -q "mk.root_uuid=$expected_uuid" "$console_log"
    grep -q 'EXT4_BOOTSTRAP_FAIL reason=root-not-found' "$console_log"
    grep -q 'EXT4_DIAGNOSTIC_READY' "$console_log"
}

capture_and_check ext4-kernel-a 7.0.0-mk2-gce-lab \
    aaaaaaaa-0000-0000-0000-000000000001
capture_and_check ext4-kernel-b 7.0.0-mk2-gce-lab-alt \
    bbbbbbbb-0000-0000-0000-000000000002

test "$(cat /sys/fs/multikernel/instances/ext4-kernel-a/status)" = active
test "$(cat /sys/fs/multikernel/instances/ext4-kernel-b/status)" = active
systemctl is-active --quiet google-guest-agent
echo 'dual distinct-kernel absent-disk isolation passed'
