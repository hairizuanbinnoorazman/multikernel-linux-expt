#!/usr/bin/env bash
set -euo pipefail
mkdir -p /var/tmp/mk-g7-src /var/tmp/mk-g7-evidence
cd /var/tmp/mk-g7-src
tar -xzf /var/tmp/mk-g7-bin/fixtures.tar.gz
kernel_tree=${OPERATOR_HOME}/src/linux
kerf_tree=${OPERATOR_HOME}/src/kerf
[[ $(uname -r) == 7.0.0-mk2-gce-lab ]]
[[ $(sudo git -C "$kernel_tree" rev-parse HEAD) == 3bdd35b64413da0b4e089ce931bfc2e8b031cbf7 ]]
[[ $(sudo git -C "$kerf_tree" rev-parse HEAD) == 8b72b3e9b266f8d32e707e2c1743ad7afc50b1ec ]]
sudo env LINUX_TREE="$kernel_tree" LAB_DIR=/var/tmp/mk-g7-src bash scripts/build-runtime-vsock.sh
musl-gcc -static -O2 -Wall -Wextra -Werror tools/mkvsock-relay.c -o /var/tmp/mk-g7-bin/mkvsock-relay
sudo mkdir -p /sys/fs/multikernel
mountpoint -q /sys/fs/multikernel || sudo mount -t multikernel none /sys/fs/multikernel
sha256sum /var/tmp/mk-g7-bin/mk-agent /var/tmp/mk-g7-bin/mk-agentctl /var/tmp/mk-g7-bin/mkvsock-relay
sudo sha256sum "$kernel_tree/vmlinux" "$kernel_tree/net/vmw_vsock/mk_transport.ko"
sudo git -C "$kernel_tree" diff > /var/tmp/mk-g7-evidence/kernel-build.diff
sudo cp "$kernel_tree/.config" /var/tmp/mk-g7-evidence/kernel.config
sudo git -C "$kernel_tree" rev-parse HEAD
sudo git -C "$kerf_tree" rev-parse HEAD
uname -r
cat /proc/sys/kernel/random/boot_id
systemctl is-active apt-daily.timer apt-daily-upgrade.timer unattended-upgrades.service
printf 'SECURITY_AUTHORITY_HOST_PREPARED\n'
