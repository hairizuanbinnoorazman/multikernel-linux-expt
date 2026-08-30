#!/usr/bin/env bash
set -euo pipefail

kerf=${KERF:-"$HOME/src/kerf/.venv/bin/kerf"}
kernel=${KERNEL:-"$HOME/src/linux/vmlinux"}
linux_tree=${LINUX_TREE:-"$HOME/src/linux"}
lab_dir=${LAB_DIR:-"$HOME/multikernel-linux-lab"}
artifact_dir=${ARTIFACT_DIR:-"$HOME/multikernel-artifacts"}
name=mediated-transport
server_log="$artifact_dir/mediated-transport-server.txt"
console_log="$artifact_dir/mediated-transport-console.txt"
server_pid=

cleanup() {
    set +e
    [[ -z "$server_pid" ]] || kill "$server_pid" 2>/dev/null
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

mkdir -p "$artifact_dir"
: >"$server_log"
: >"$console_log"
musl-gcc -static -O2 -Wall -Wextra -Werror \
    "$lab_dir/tools/mkvsock-probe.c" -o "$artifact_dir/mkvsock-probe"
"$lab_dir/scripts/build-mediated-transport-initramfs.sh" \
    "$lab_dir/guest/mediated-transport-init" \
    "$artifact_dir/mkvsock-probe" \
    "$linux_tree/net/vmw_vsock/mk_transport.ko" \
    "$artifact_dir/mediated-transport.cpio.gz"

sudo mkdir -p /sys/fs/multikernel
mountpoint -q /sys/fs/multikernel || \
    sudo mount -t multikernel none /sys/fs/multikernel
sudo modprobe vsock
lsmod | grep -q '^mk_transport ' || \
    sudo insmod "$linux_tree/net/vmw_vsock/mk_transport.ko"

"$artifact_dir/mkvsock-probe" server 4050 >"$server_log" 2>&1 &
server_pid=$!
for _ in $(seq 1 50); do
    grep -q MKVSOCK_SERVER_READY "$server_log" && break
    sleep 0.1
done
grep -q MKVSOCK_SERVER_READY "$server_log"

sudo "$kerf" init --cpus=8,10,12,14 --memory=6GB --devices=none --dry-run
sudo "$kerf" init --cpus=8,10,12,14 --memory=6GB --devices=none
set +e
sudo "$kerf" create "$name" --id=3 --cpus=8,10,12,14 --memory=4GB
create_rc=$?
set -e
[[ -d "/sys/fs/multikernel/instances/$name" ]] || exit "$create_rc"
sudo "$kerf" load "$name" --kernel="$kernel" \
    --initrd="$artifact_dir/mediated-transport.cpio.gz" \
    --cmdline='rdinit=/init console=mktty0 loglevel=6 panic=-1'

set +e
sudo timeout 25s script -qefc "$kerf exec $name --console" /dev/null \
    >"$console_log" 2>&1
console_rc=$?
set -e
if [[ $console_rc -ne 0 && $console_rc -ne 124 ]]; then
    sed -n '1,260p' "$console_log"
    exit "$console_rc"
fi

sed -n '1,260p' "$console_log"
cat "$server_log"
grep -q MEDIATED_TRANSPORT_CHILD_PASS "$console_log"
for _ in $(seq 1 50); do
    kill -0 "$server_pid" 2>/dev/null || break
    sleep 0.1
done
if kill -0 "$server_pid" 2>/dev/null; then
    echo 'transport server did not terminate after client completion' >&2
    exit 1
fi
wait "$server_pid"
server_pid=
grep -q MKVSOCK_SERVER_PASS "$server_log"
systemctl is-active --quiet google-guest-agent
echo MEDIATED_TRANSPORT_TEST_PASS
