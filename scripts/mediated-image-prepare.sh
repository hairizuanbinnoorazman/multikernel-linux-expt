#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
    echo "usage: $0 CHILD_NAME IMAGE_ID" >&2
    exit 2
fi

child=$1
image_id=$2
outer_mount=${MEDIATED_MOUNT:-/srv/multikernel-storage}
expected_outer_uuid=${MEDIATED_OUTER_UUID:?set MEDIATED_OUTER_UUID}
image_bytes=${MEDIATED_IMAGE_BYTES:-4294967296}
root_init=${MEDIATED_ROOT_INIT:-"$HOME/multikernel-linux-lab/guest/mediated-disk-root-init"}
image_dir="$outer_mount/$child"
image="$image_dir/root.ext4"
label="mk-$child-root"
loop=
image_mount=$(mktemp -d)

cleanup() {
    set +e
    mountpoint -q "$image_mount" && umount "$image_mount"
    [[ -z "$loop" ]] || losetup -d "$loop"
    rmdir "$image_mount"
}
trap cleanup EXIT

[[ ${MEDIATED_FIRST_IMAGE_FORMAT:-} == YES ]] || {
    echo 'MEDIATED_IMAGE_REFUSED reason=first-format-flag' >&2
    exit 2
}
mountpoint -q "$outer_mount" || {
    echo 'MEDIATED_IMAGE_REFUSED reason=outer-not-mounted' >&2
    exit 2
}
[[ "$(findmnt -n -o UUID "$outer_mount")" == "$expected_outer_uuid" ]] || {
    echo 'MEDIATED_IMAGE_REFUSED reason=outer-uuid' >&2
    exit 2
}
[[ ! -e "$image" ]] || {
    echo 'MEDIATED_IMAGE_REFUSED reason=image-exists' >&2
    exit 2
}
[[ -x "$root_init" ]] || {
    echo 'MEDIATED_IMAGE_REFUSED reason=root-init' >&2
    exit 2
}

mkdir -p "$image_dir"
fallocate -l "$image_bytes" "$image"
loop=$(losetup --find --show "$image")
[[ -z "$(wipefs -n "$loop")" ]] || {
    echo 'MEDIATED_IMAGE_REFUSED reason=unexpected-signature' >&2
    exit 2
}
mkfs.ext4 -L "$label" "$loop"
mount "$loop" "$image_mount"
mkdir -p "$image_mount"/{bin,dev,etc,proc,root,run,sys,tmp,var/lib/multikernel}
cp /usr/bin/busybox "$image_mount/bin/busybox"
cp "$root_init" "$image_mount/init"
chmod 0755 "$image_mount/bin/busybox" "$image_mount/init"
printf '%s\n' "$child" >"$image_mount/etc/mk-child-id"
printf '%s\n' "$image_id" >"$image_mount/etc/mk-image-id"
printf '0\n' >"$image_mount/var/lib/multikernel/persistence-counter"
printf 'mediated persistent root for %s\n' "$child" >"$image_mount/root/corpus.txt"
sync
uuid=$(blkid -s UUID -o value "$loop")
find "$image_mount" -xdev -printf '%P\t%y\t%m\t%s\n' | LC_ALL=C sort \
    >"$image_dir/root-manifest.txt"
sha256sum "$image_mount/bin/busybox" "$image_mount/init" \
    "$image_mount/etc/mk-child-id" "$image_mount/etc/mk-image-id" \
    "$image_mount/root/corpus.txt" >"$image_dir/root-content-sha256.txt"
umount "$image_mount"
losetup -d "$loop"
loop=
fallocate -l "$image_bytes" "$image"
allocated_bytes=$(($(stat -c %b "$image") * 512))
[[ "$allocated_bytes" -ge "$image_bytes" ]] || {
    echo "MEDIATED_IMAGE_REFUSED reason=not-fully-allocated allocated=$allocated_bytes" >&2
    exit 2
}
sync
echo "MEDIATED_IMAGE_READY child=$child image=$image image_id=$image_id bytes=$image_bytes allocated=$allocated_bytes uuid=$uuid label=$label"
stat --format='inode=%i size=%s blocks=%b' "$image"
sha256sum "$image_dir/root-manifest.txt" "$image_dir/root-content-sha256.txt"
