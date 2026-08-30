#!/usr/bin/env bash
set -euo pipefail

kerf=${KERF:-"$HOME/src/kerf/.venv/bin/kerf"}
kernel=${KERNEL:-"$HOME/src/linux/vmlinux"}
linux_tree=${LINUX_TREE:-"$HOME/src/linux"}
lab_dir=${LAB_DIR:-"$HOME/multikernel-linux-lab"}
artifact_dir=${ARTIFACT_DIR:-"$HOME/multikernel-artifacts"}
image=${MEDIATED_IMAGE:-/srv/multikernel-storage/child-a/root.ext4}
image_id=${MEDIATED_IMAGE_ID:-image-child-a-v1}
generation=${MEDIATED_GENERATION:-child-a-cycle-1}
root_uuid=${MEDIATED_ROOT_UUID:-352be7e6-ef7c-44d2-9f42-fe84303cddf4}
image_size=${MEDIATED_IMAGE_BYTES:-4294967296}
port=${MEDIATED_PORT:-4061}
name=mediated-root-a
server_log="$artifact_dir/$generation-server.txt"
console_log="$artifact_dir/$generation-console.txt"
server_pid=

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
    if [[ -n "$server_pid" ]]; then
        for _ in $(seq 1 200); do
            kill -0 "$server_pid" 2>/dev/null || break
            sleep 0.1
        done
        kill "$server_pid" 2>/dev/null
        wait "$server_pid" 2>/dev/null
    fi
}
trap cleanup EXIT

mkdir -p "$artifact_dir"
[[ -f "$image" ]] || { echo "missing image: $image" >&2; exit 2; }
[[ "$(stat -c %s "$image")" == "$image_size" ]] || exit 2
[[ $(($(stat -c %b "$image") * 512)) -ge "$image_size" ]] || {
    echo 'image is not fully allocated' >&2
    exit 2
}
: >"$server_log"
: >"$console_log"

musl-gcc -static -O2 -Wall -Wextra -Werror \
    "$lab_dir/tools/mkvsock-nbd.c" -o "$artifact_dir/mkvsock-nbd"
"$lab_dir/scripts/build-mediated-root-initramfs.sh" \
    "$lab_dir/guest/mediated-root-bootstrap-init" \
    "$artifact_dir/mkvsock-nbd" \
    "$linux_tree/net/vmw_vsock/mk_transport.ko" \
    "/lib/modules/$(uname -r)/kernel/drivers/block/nbd.ko" \
    /usr/bin/busybox "$artifact_dir/mediated-root.cpio.gz"

sudo mkdir -p /sys/fs/multikernel
mountpoint -q /sys/fs/multikernel || sudo mount -t multikernel none /sys/fs/multikernel
lsmod | grep -q '^mk_transport ' || sudo insmod "$linux_tree/net/vmw_vsock/mk_transport.ko"
sudo "$artifact_dir/mkvsock-nbd" server "$image" "$port" "$image_id" "$generation" \
    >"$server_log" 2>&1 &
server_pid=$!
for _ in $(seq 1 50); do
    grep -q MKNBD_SERVER_READY "$server_log" && break
    kill -0 "$server_pid" 2>/dev/null || break
    sleep 0.1
done
grep -q MKNBD_SERVER_READY "$server_log"

sudo "$kerf" init --cpus=8,10,12,14 --memory=6GB --devices=none --dry-run
sudo "$kerf" init --cpus=8,10,12,14 --memory=6GB --devices=none
set +e
sudo "$kerf" create "$name" --id=3 --cpus=8,10,12,14 --memory=4GB
create_rc=$?
set -e
[[ -d "/sys/fs/multikernel/instances/$name" ]] || exit "$create_rc"
cmdline="rdinit=/init console=mktty0 loglevel=6 panic=-1 mk.child=child-a mk.image_id=$image_id mk.generation=$generation mk.root_uuid=$root_uuid mk.port=$port mk.size=$image_size"
sudo "$kerf" load "$name" --kernel="$kernel" \
    --initrd="$artifact_dir/mediated-root.cpio.gz" --cmdline="$cmdline"

set +e
sudo timeout 35s script -qefc "$kerf exec $name --console" /dev/null \
    >"$console_log" 2>&1
console_rc=$?
set -e
if [[ $console_rc -ne 0 && $console_rc -ne 124 ]]; then
    sed -n '1,320p' "$console_log"
    exit "$console_rc"
fi
sed -n '1,320p' "$console_log"
cat "$server_log"
grep -q 'MEDIATED_ROOT_BOOTSTRAP_PASS' "$console_log"
grep -q 'MEDIATED_ROOT_CHILD_READY' "$console_log"
grep -q 'root_device=/dev/nbd0' "$console_log"
grep -q 'root_fstype=ext4' "$console_log"
grep -q 'MKNBD_SERVER_CLIENT_ACCEPTED' "$server_log"
[[ "$(cat "/sys/fs/multikernel/instances/$name/status")" == active ]]
systemctl is-active --quiet google-guest-agent

sudo "$kerf" kill "$name" --force
sudo "$kerf" unload "$name"
sudo "$kerf" delete "$name"
sudo "$kerf" init --cpus=none --memory=none --devices=none
for _ in $(seq 1 200); do
    kill -0 "$server_pid" 2>/dev/null || break
    sleep 0.1
done
if kill -0 "$server_pid" 2>/dev/null; then
    echo 'server did not observe child disconnect' >&2
    exit 1
fi
wait "$server_pid"
server_pid=
cat "$server_log"
grep -q MKNBD_SERVER_CLOSED "$server_log"
echo "MEDIATED_ROOT_CYCLE_PASS generation=$generation"
