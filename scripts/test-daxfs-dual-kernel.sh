#!/usr/bin/env bash
set -euo pipefail

kerf=${KERF:-"$HOME/src/kerf/.venv/bin/kerf"}
artifact_dir=${ARTIFACT_DIR:-"$HOME/multikernel-artifacts/daxfs-full-20260828"}
evidence_dir=${EVIDENCE_DIR:-"$artifact_dir/evidence"}
primary_release=${PRIMARY_RELEASE:-7.0.0-mk2-gce-lab}
alternate_release=${ALTERNATE_RELEASE:-7.0.0-mk2-gce-lab-alt}
name_a=docker-kernel-a
name_b=docker-kernel-b

mkdir -p "$evidence_dir"

cleanup_instance() {
    local name=$1 instance_dir="/sys/fs/multikernel/instances/$1" status
    [[ -d "$instance_dir" ]] || return 0
    status=$(cat "$instance_dir/status")
    if [[ "$status" = active ]]; then
        sudo "$kerf" kill "$name" --force --verbose || true
        status=$(cat "$instance_dir/status")
    fi
    if [[ "$status" = loaded ]]; then
        sudo "$kerf" unload "$name" --verbose || true
    fi
    sudo "$kerf" delete "$name" --verbose || true
}

cleanup() {
    set +e
    cleanup_instance "$name_b"
    cleanup_instance "$name_a"
    remaining=$(find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 \
        -type d 2>/dev/null | wc -l)
    if [[ $remaining = 0 ]]; then
        sudo "$kerf" init --cpus=none --memory=none --devices=none --verbose
    else
        echo "not releasing the pool: $remaining unrelated instance(s) remain" >&2
    fi
}
trap cleanup EXIT

create_instance() {
    local name=$1 id=$2 cpus=$3
    set +e
    sudo "$kerf" create "$name" --id="$id" --cpus="$cpus" \
        --memory=7GB --verbose
    local rc=$?
    set -e
    [[ -d /sys/fs/multikernel/instances/$name ]] || return "$rc"
}

host_boot_id=$(cat /proc/sys/kernel/random/boot_id)
host_release=$(uname -r)

sudo mkdir -p /sys/fs/multikernel
mountpoint -q /sys/fs/multikernel || \
    sudo mount -t multikernel none /sys/fs/multikernel
remaining=$(find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 \
    -type d 2>/dev/null | wc -l)
if [[ $remaining != 0 ]]; then
    echo "refusing to reconfigure the pool: $remaining instance(s) exist" >&2
    exit 1
fi
sudo "$kerf" init --cpus=8,10,12,14,9,11,13,15 \
    --memory=16GB --devices=none --verbose
create_instance "$name_a" 61 8,10,12,14
create_instance "$name_b" 62 9,11,13,15

sudo "$kerf" load "$name_a" \
    --kernel="$artifact_dir/kernels/vmlinux-primary" \
    --initrd="$artifact_dir/kernels/initramfs-primary.cpio.gz" \
    --image=daxfs-proof:local \
    --cmdline='rdinit=/init console=mktty0 loglevel=7 panic=-1' --verbose
sudo "$kerf" load "$name_b" \
    --kernel="$artifact_dir/kernels/vmlinux-alt" \
    --initrd="$artifact_dir/kernels/initramfs-alt.cpio.gz" \
    --image=daxfs-proof:local \
    --cmdline='rdinit=/init console=mktty0 loglevel=7 panic=-1' --verbose

set +e
sudo timeout 30s script -qefc \
    "$kerf exec $name_a --console --verbose" \
    "$evidence_dir/dual-kernel-a-console.txt" >/dev/null &
pid_a=$!
sleep 1
sudo timeout 30s script -qefc \
    "$kerf exec $name_b --console --verbose" \
    "$evidence_dir/dual-kernel-b-console.txt" >/dev/null &
pid_b=$!
wait "$pid_a"
rc_a=$?
wait "$pid_b"
rc_b=$?
set -e
[[ $rc_a = 0 || $rc_a = 124 ]]
[[ $rc_b = 0 || $rc_b = 124 ]]

test "$(cat /sys/fs/multikernel/instances/$name_a/status)" = active
test "$(cat /sys/fs/multikernel/instances/$name_b/status)" = active
grep -F 'DOCKER_IMAGE_READY' "$evidence_dir/dual-kernel-a-console.txt"
grep -F "release=$primary_release" "$evidence_dir/dual-kernel-a-console.txt"
grep -F 'none / daxfs ' "$evidence_dir/dual-kernel-a-console.txt"
grep -F 'DOCKER_IMAGE_READY' "$evidence_dir/dual-kernel-b-console.txt"
grep -F "release=$alternate_release" "$evidence_dir/dual-kernel-b-console.txt"
grep -F 'none / daxfs ' "$evidence_dir/dual-kernel-b-console.txt"
test "$(cat /proc/sys/kernel/random/boot_id)" = "$host_boot_id"
test "$(uname -r)" = "$host_release"
systemctl is-active --quiet google-guest-agent

{
    echo "DUAL_KERNEL_DOCKER_PROOF=PASS"
    echo "host_release=$host_release"
    echo "host_boot_id=$host_boot_id"
    echo "child_a_release=$primary_release"
    echo "child_b_release=$alternate_release"
    sha256sum "$artifact_dir/kernels/vmlinux-primary" \
        "$artifact_dir/kernels/vmlinux-alt"
    sudo "$kerf" show
} | tee "$evidence_dir/dual-kernel-summary.txt"

cleanup
trap - EXIT
test "$(cat /sys/devices/system/cpu/online)" = 0-15
systemctl is-active --quiet google-guest-agent
sudo "$kerf" show | tee "$evidence_dir/dual-kernel-cleanup.txt"
echo "DUAL_KERNEL_CLEANUP=PASS" | tee -a \
    "$evidence_dir/dual-kernel-cleanup.txt"
