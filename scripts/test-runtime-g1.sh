#!/usr/bin/env bash
set -euo pipefail
kerf=${KERF:-"$HOME/src/kerf/.venv/bin/kerf"}
kernel=${KERNEL:-"$HOME/src/linux/vmlinux"}
lab=${LAB_DIR:-"$HOME/multikernel-linux-lab"}
name=runtime-g1-crash
initrd=/tmp/runtime-g1-crash.cpio.gz
log=/tmp/runtime-g1-crash.log

cleanup() {
	set +e
	if [[ -d /sys/fs/multikernel/instances/$name ]]; then
		status=$(cat /sys/fs/multikernel/instances/$name/status)
		[[ $status != active ]] || sudo "$kerf" kill "$name" --force
		status=$(cat /sys/fs/multikernel/instances/$name/status)
		[[ $status != loaded ]] || sudo "$kerf" unload "$name"
		sudo "$kerf" delete "$name"
	fi
	sudo "$kerf" init --cpus=none --memory=none --devices=none
}
trap cleanup EXIT

"$lab/scripts/build-initramfs.sh" "$lab/guest/runtime-crash-init" "$initrd"
sudo "$kerf" init --cpus=8,10,12,14 --memory=8GB --devices=none
sudo "$kerf" create "$name" --id=21 --cpus=8,10 --memory=4GB
sudo "$kerf" load "$name" --kernel="$kernel" --initrd="$initrd" \
	--cmdline='rdinit=/init console=mktty0 panic=-1'
set +e
sudo timeout 20s script -qefc "$kerf exec $name --console" /dev/null >"$log" 2>&1
set -e
grep -q RUNTIME_G1_INTENTIONAL_INIT_EXIT "$log"
systemctl is-active --quiet google-guest-agent
test "$(cat /sys/fs/multikernel/instances/$name/status)" = loaded
cleanup
trap - EXIT
test "$(cat /sys/devices/system/cpu/online)" = 0-15
test ! -d "/sys/fs/multikernel/instances/$name"
echo RUNTIME_G1_CRASH_RECLAIM_PASS
