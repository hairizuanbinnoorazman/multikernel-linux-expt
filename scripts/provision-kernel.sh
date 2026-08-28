#!/usr/bin/env bash
set -euo pipefail

linux_dir=${LINUX_DIR:-"$HOME/src/linux"}
linux_tag=${LINUX_TAG:-v7.0-mk2}
linux_commit=${LINUX_COMMIT:-3bdd35b64413da0b4e089ce931bfc2e8b031cbf7}
local_version=${LOCAL_VERSION:--gce-lab}

sudo apt-get update
sudo DEBIAN_FRONTEND=noninteractive apt-get install -y \
    bc bison build-essential busybox-static cpio device-tree-compiler \
    dwarves flex git kmod libdw-dev libelf-dev libfdt-dev libncurses-dev \
    libssl-dev libudev-dev musl-tools pkg-config python3-dev python3-pip \
    python3-venv rsync swig

mkdir -p "$(dirname "$linux_dir")"
if [[ ! -d "$linux_dir/.git" ]]; then
    git clone --depth=1 --branch "$linux_tag" \
        https://github.com/multikernel/linux.git "$linux_dir"
fi

cd "$linux_dir"
actual_commit=$(git rev-parse HEAD)
if [[ "$actual_commit" != "$linux_commit" ]]; then
    echo "unexpected Linux commit: $actual_commit (wanted $linux_commit)" >&2
    exit 1
fi

cp "/boot/config-$(uname -r)" .config
scripts/config --set-str LOCALVERSION "$local_version"
scripts/config --set-str SYSTEM_TRUSTED_KEYS ""
scripts/config --set-str SYSTEM_REVOCATION_KEYS ""
scripts/config --enable KEXEC
scripts/config --enable KEXEC_FILE
scripts/config --enable MULTIKERNEL
scripts/config --enable MKTTY
scripts/config --disable MULTIKERNEL_VSOCKETS
scripts/config --enable DEVTMPFS
scripts/config --enable DEVTMPFS_MOUNT
scripts/config --enable BLK_DEV_INITRD
scripts/config --enable RD_GZIP
scripts/config --enable PROC_FS
scripts/config --enable SYSFS
scripts/config --enable TMPFS
scripts/config --enable BINFMT_ELF
make olddefconfig

make -j"$(nproc)"
kernel_release=$(make -s kernelrelease)
sudo make modules_install
sudo make install
sudo update-grub

echo "installed kernel $kernel_release"
echo "stock kernels were retained in /boot"
ls -lh "/boot/vmlinuz-$kernel_release" "/boot/initrd.img-$kernel_release"
