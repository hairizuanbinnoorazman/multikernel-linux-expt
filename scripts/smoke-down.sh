#!/usr/bin/env bash
set -euo pipefail

kerf=${KERF:-"$HOME/src/kerf/.venv/bin/kerf"}

for name in smoke-a smoke-b; do
    instance_dir="/sys/fs/multikernel/instances/$name"
    [[ -d "$instance_dir" ]] || continue
    status=$(cat "$instance_dir/status")
    if [[ "$status" = active ]]; then
        sudo "$kerf" kill "$name" --force --verbose
        status=$(cat "$instance_dir/status")
    fi
    if [[ "$status" = loaded ]]; then
        sudo "$kerf" unload "$name" --verbose
    fi
    sudo "$kerf" delete "$name" --verbose
done

sudo "$kerf" init --cpus=none --memory=none --devices=none --verbose
test "$(cat /sys/devices/system/cpu/online)" = 0-15
systemctl is-active --quiet google-guest-agent
sudo "$kerf" show
echo "child resources returned to host"
