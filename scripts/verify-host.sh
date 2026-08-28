#!/usr/bin/env bash
set -euo pipefail

expected_release=${EXPECTED_RELEASE:-7.0.0-mk2-gce-lab}

test "$(uname -r)" = "$expected_release"
grep -q '^CONFIG_MULTIKERNEL=y$' "/boot/config-$(uname -r)"
grep -q '^CONFIG_MKTTY=y$' "/boot/config-$(uname -r)"
grep -q '^CONFIG_KEXEC=y$' "/boot/config-$(uname -r)"
systemctl is-active --quiet google-guest-agent
test "$(curl -fsS -H Metadata-Flavor:Google \
    http://metadata.google.internal/computeMetadata/v1/instance/name)" = \
    "${INSTANCE:-mklinux-lab}"

sudo mkdir -p /sys/fs/multikernel
mountpoint -q /sys/fs/multikernel || \
    sudo mount -t multikernel none /sys/fs/multikernel
test -e /sys/fs/multikernel/device_tree
test -c /dev/mktty

echo "host_release=$(uname -r)"
echo "host_online_cpus=$(cat /sys/devices/system/cpu/online)"
findmnt /sys/fs/multikernel
ip -brief address show scope global
echo "host verification passed"
