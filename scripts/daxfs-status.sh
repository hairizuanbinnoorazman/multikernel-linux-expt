#!/usr/bin/env bash
set -euo pipefail

kerf=${KERF:-"$HOME/src/kerf/.venv/bin/kerf"}

echo "host_release=$(uname -r)"
echo "host_boot_id=$(cat /proc/sys/kernel/random/boot_id)"
echo "online_cpus=$(cat /sys/devices/system/cpu/online)"
echo "offline_cpus=$(cat /sys/devices/system/cpu/offline)"
echo "guest_agent=$(systemctl is-active google-guest-agent)"
findmnt -t daxfs || true
sudo "$kerf" show

