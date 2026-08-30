#!/usr/bin/env bash
set -euo pipefail

kerf=${KERF:-"$HOME/src/kerf/.venv/bin/kerf"}
lab_dir=${LAB_DIR:-"$HOME/multikernel-linux-lab"}
artifact_dir=${ARTIFACT_DIR:-"$HOME/multikernel-artifacts"}
primary_tree=${PRIMARY_TREE:-"$HOME/src/linux"}
alternate_tree=${ALTERNATE_TREE:-"$HOME/src/linux-alt-worktree"}
image_a=/srv/multikernel-storage/child-a/root.ext4
image_b=/srv/multikernel-storage/child-b/root.ext4
uuid_a=352be7e6-ef7c-44d2-9f42-fe84303cddf4
uuid_b=0fc9d98e-3902-44a1-b4be-7a648fd28e46
size=4294967296
generation_a=dual-a-1
generation_b=dual-b-1
server_a=
server_b=

delete_instance() {
    local name=$1 status
    [[ -d "/sys/fs/multikernel/instances/$name" ]] || return
    status=$(cat "/sys/fs/multikernel/instances/$name/status")
    [[ "$status" != active ]] || sudo "$kerf" kill "$name" --force
    status=$(cat "/sys/fs/multikernel/instances/$name/status")
    [[ "$status" != loaded ]] || sudo "$kerf" unload "$name"
    sudo "$kerf" delete "$name"
}

cleanup() {
    set +e
    delete_instance mediated-root-b
    delete_instance mediated-root-a
    sudo "$kerf" init --cpus=none --memory=none --devices=none >/dev/null
    for pid in "$server_a" "$server_b"; do
        [[ -n "$pid" ]] || continue
        for _ in $(seq 1 200); do
            kill -0 "$pid" 2>/dev/null || break
            sleep 0.1
        done
        kill "$pid" 2>/dev/null
        wait "$pid" 2>/dev/null
    done
}
trap cleanup EXIT

mkdir -p "$artifact_dir"
for image in "$image_a" "$image_b"; do
    [[ -f "$image" && "$(stat -c %s "$image")" == "$size" ]] || exit 2
    [[ $(($(stat -c %b "$image") * 512)) -ge "$size" ]] || exit 2
done
[[ -x "$primary_tree/vmlinux" && -x "$alternate_tree/vmlinux" ]] || exit 2

musl-gcc -static -O2 -Wall -Wextra -Werror \
    "$lab_dir/tools/mkvsock-nbd.c" -o "$artifact_dir/mkvsock-nbd"
for variant in a b; do
    tree=$primary_tree
    [[ "$variant" == a ]] || tree=$alternate_tree
    "$lab_dir/scripts/build-mediated-root-initramfs.sh" \
        "$lab_dir/guest/mediated-root-bootstrap-init" \
        "$artifact_dir/mkvsock-nbd" "$tree/net/vmw_vsock/mk_transport.ko" \
        "/lib/modules/$(uname -r)/kernel/drivers/block/nbd.ko" /usr/bin/busybox \
        "$artifact_dir/mediated-root-$variant.cpio.gz"
done

sudo mkdir -p /sys/fs/multikernel
mountpoint -q /sys/fs/multikernel || sudo mount -t multikernel none /sys/fs/multikernel
lsmod | grep -q '^mk_transport ' || \
    sudo insmod "$primary_tree/net/vmw_vsock/mk_transport.ko"

: >"$artifact_dir/$generation_a-server.txt"
: >"$artifact_dir/$generation_b-server.txt"
sudo "$artifact_dir/mkvsock-nbd" server "$image_a" 4061 image-child-a-v1 "$generation_a" \
    >"$artifact_dir/$generation_a-server.txt" 2>&1 &
server_a=$!
sudo "$artifact_dir/mkvsock-nbd" server "$image_b" 4062 image-child-b-v1 "$generation_b" \
    >"$artifact_dir/$generation_b-server.txt" 2>&1 &
server_b=$!
for log in "$artifact_dir/$generation_a-server.txt" "$artifact_dir/$generation_b-server.txt"; do
    for _ in $(seq 1 50); do
        grep -q MKNBD_SERVER_READY "$log" && break
        sleep 0.1
    done
    grep -q MKNBD_SERVER_READY "$log"
done

sudo "$kerf" init --cpus=8,10,12,14,9,11,13,15 --memory=16GB --devices=none --dry-run
sudo "$kerf" init --cpus=8,10,12,14,9,11,13,15 --memory=16GB --devices=none
create_child() {
    local name=$1 id=$2 cpus=$3 memory=$4
    set +e
    sudo "$kerf" create "$name" --id="$id" --cpus="$cpus" --memory="$memory"
    local rc=$?
    set -e
    [[ -d "/sys/fs/multikernel/instances/$name" ]] || return "$rc"
}
create_child mediated-root-a 3 8,10,12,14 8GB
create_child mediated-root-b 4 9,11,13,15 7GB

cmd_a="rdinit=/init console=mktty0 loglevel=6 panic=-1 mk.child=child-a mk.image_id=image-child-a-v1 mk.generation=$generation_a mk.root_uuid=$uuid_a mk.port=4061 mk.size=$size"
cmd_b="rdinit=/init console=mktty0 loglevel=6 panic=-1 mk.child=child-b mk.image_id=image-child-b-v1 mk.generation=$generation_b mk.root_uuid=$uuid_b mk.port=4062 mk.size=$size"
sudo "$kerf" load mediated-root-a --kernel="$primary_tree/vmlinux" \
    --initrd="$artifact_dir/mediated-root-a.cpio.gz" --cmdline="$cmd_a"
sudo "$kerf" load mediated-root-b --kernel="$alternate_tree/vmlinux" \
    --initrd="$artifact_dir/mediated-root-b.cpio.gz" --cmdline="$cmd_b"

capture() {
    local name=$1 log=$2
    : >"$log"
    set +e
    sudo timeout 25s script -qefc "$kerf exec $name --console" /dev/null >"$log" 2>&1
    local rc=$?
    set -e
    [[ $rc -eq 0 || $rc -eq 124 ]] || return "$rc"
    grep -q MEDIATED_ROOT_CHILD_READY "$log"
    [[ "$(cat "/sys/fs/multikernel/instances/$name/status")" == active ]]
}
capture mediated-root-a "$artifact_dir/$generation_a-console.txt"
capture mediated-root-b "$artifact_dir/$generation_b-console.txt"

grep -q 'child=child-a' "$artifact_dir/$generation_a-console.txt"
grep -q 'release=7.0.0-mk2-gce-lab' "$artifact_dir/$generation_a-console.txt"
grep -q "root_uuid=$uuid_a" "$artifact_dir/$generation_a-console.txt"
! grep -q "$uuid_b" "$artifact_dir/$generation_a-console.txt"
grep -q 'child=child-b' "$artifact_dir/$generation_b-console.txt"
grep -q 'release=7.0.0-mk2-gce-lab-alt' "$artifact_dir/$generation_b-console.txt"
grep -q "root_uuid=$uuid_b" "$artifact_dir/$generation_b-console.txt"
! grep -q "$uuid_a" "$artifact_dir/$generation_b-console.txt"
systemctl is-active --quiet google-guest-agent
sudo "$kerf" show >"$artifact_dir/mediated-dual-state.txt"
grep -q 'Status:.*active' "$artifact_dir/mediated-dual-state.txt"
if grep -qE 'pci-device|namespace-id|scsi' "$artifact_dir/mediated-dual-state.txt"; then
    echo 'unexpected physical storage device in child device tree' >&2
    exit 1
fi

cat "$artifact_dir/$generation_a-console.txt"
cat "$artifact_dir/$generation_b-console.txt"
cat "$artifact_dir/mediated-dual-state.txt"
delete_instance mediated-root-b
delete_instance mediated-root-a
sudo "$kerf" init --cpus=none --memory=none --devices=none
for pid in "$server_a" "$server_b"; do
    for _ in $(seq 1 200); do
        kill -0 "$pid" 2>/dev/null || break
        sleep 0.1
    done
    kill -0 "$pid" 2>/dev/null && exit 1
    wait "$pid"
done
server_a=
server_b=
grep -q MKNBD_SERVER_CLOSED "$artifact_dir/$generation_a-server.txt"
grep -q MKNBD_SERVER_CLOSED "$artifact_dir/$generation_b-server.txt"
cat "$artifact_dir/$generation_a-server.txt"
cat "$artifact_dir/$generation_b-server.txt"
echo MEDIATED_DUAL_ROOT_PASS
