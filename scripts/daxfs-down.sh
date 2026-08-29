#!/usr/bin/env bash
set -euo pipefail

kerf=${KERF:-"$HOME/src/kerf/.venv/bin/kerf"}
names=(
    daxfs-demo daxfs-mount daxfs-root-1 daxfs-root-2 docker-busybox
    shared-ro-a shared-ro-b shared-rw-a shared-rw-b
    docker-kernel-a docker-kernel-b
)

for name in "${names[@]}"; do
    instance_dir="/sys/fs/multikernel/instances/$name"
    [[ -d "$instance_dir" ]] || continue
    status=$(cat "$instance_dir/status")
    if [[ "$status" = active ]]; then
        sudo "$kerf" kill "$name" --force --verbose || true
        status=$(cat "$instance_dir/status")
    fi
    if [[ "$status" = loaded ]]; then
        sudo "$kerf" unload "$name" --verbose || true
    fi
    sudo "$kerf" delete "$name" --verbose || true
done

remaining=$(find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 \
    -type d 2>/dev/null | wc -l)
if [[ $remaining = 0 ]]; then
    sudo "$kerf" init --cpus=none --memory=none --devices=none --verbose
else
    echo "not releasing the pool: $remaining unrelated instance(s) remain" >&2
fi

echo "online_cpus=$(cat /sys/devices/system/cpu/online)"
echo "offline_cpus=$(cat /sys/devices/system/cpu/offline)"
echo "guest_agent=$(systemctl is-active google-guest-agent)"
sudo "$kerf" show
