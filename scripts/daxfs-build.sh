#!/usr/bin/env bash
set -euo pipefail

lab_dir=${LAB_DIR:-"$HOME/multikernel-linux-lab"}
linux_dir=${LINUX_DIR:-"$HOME/src/linux"}
kerf_dir=${KERF_DIR:-"$HOME/src/kerf"}
daxfs_dir=${DAXFS_DIR:-"$HOME/src/daxfs"}
artifact_dir=${ARTIFACT_DIR:-"$HOME/multikernel-artifacts/daxfs-current"}
daxfs_commit=${DAXFS_COMMIT:-11ab401585b79b4a7c9164019852e0219e197d13}
kerf_commit=${KERF_COMMIT:-8b72b3e9b266f8d32e707e2c1743ad7afc50b1ec}

mkdir -p "$HOME/src" "$artifact_dir"
if [[ ! -d "$daxfs_dir/.git" ]]; then
    git clone https://github.com/multikernel/daxfs.git "$daxfs_dir"
    git -C "$daxfs_dir" checkout --detach "$daxfs_commit"
fi
test "$(git -C "$daxfs_dir" rev-parse HEAD)" = "$daxfs_commit"
test "$(git -C "$kerf_dir" rev-parse HEAD)" = "$kerf_commit"
test "$(make -s -C "$linux_dir" kernelrelease)" = "$(uname -r)"

apply_kerf_patch() {
    local patch=$1
    if git -C "$kerf_dir" apply --reverse --check "$patch" 2>/dev/null; then
        return 0
    fi
    git -C "$kerf_dir" apply --check "$patch"
    git -C "$kerf_dir" apply "$patch"
}

# Kerf v0.2.0 needs both fixes to serialize normal OCI root filesystems.
apply_kerf_patch "$lab_dir/patches/kerf-v0.2.0-skip-special-files.patch"
apply_kerf_patch "$lab_dir/patches/kerf-v0.2.0-hardlink-inode-count.patch"

make -C "$daxfs_dir/daxfs" clean KDIR="$linux_dir"
make -C "$daxfs_dir" -j"$(nproc)" KDIR="$linux_dir" daxfs tools
test "$(modinfo -F vermagic "$daxfs_dir/daxfs/daxfs.ko" | awk '{print $1}')" = \
    "$(uname -r)"

"$lab_dir/scripts/build-daxfs-root.sh" "$artifact_dir/rootfs"
"$lab_dir/scripts/build-daxfs-initramfs.sh" \
    "$lab_dir/guest/daxfs-bootstrap-init" "$daxfs_dir/daxfs/daxfs.ko" \
    "$artifact_dir/initramfs.cpio.gz"
sudo docker build -f "$lab_dir/docker/daxfs-proof.Dockerfile" \
    -t daxfs-proof:local "$lab_dir"

{
    echo "DAXFS_COMMIT=$daxfs_commit"
    echo "KERF_COMMIT=$kerf_commit"
    echo "KERNEL_RELEASE=$(uname -r)"
    modinfo "$daxfs_dir/daxfs/daxfs.ko" | grep '^vermagic:'
    sha256sum "$linux_dir/vmlinux" "$daxfs_dir/daxfs/daxfs.ko" \
        "$artifact_dir/initramfs.cpio.gz"
    sudo docker image inspect daxfs-proof:local --format='docker_image_id={{.Id}}'
} | tee "$artifact_dir/build-proof.txt"
