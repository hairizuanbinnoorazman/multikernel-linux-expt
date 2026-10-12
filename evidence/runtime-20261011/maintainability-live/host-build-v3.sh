#!/usr/bin/env bash
set -euo pipefail
export PATH=/opt/go/bin:$PATH
cd /var/tmp/mk-maint-src
scratch=$(mktemp -d -p /var/tmp mkt.XXXXXX)
trap 'rm -rf "$scratch"' EXIT
export TMPDIR=/var/tmp GOTMPDIR=/var/tmp GOCACHE=/var/tmp/mk-maint-go-cache
mkdir -p "$GOCACHE"
cd runtime
go test -race ./...
go vet ./...
cd ..
make runtime-manifest RUNTIME_VERSION=0.1.0-maint-test RUNTIME_REVISION="$(git rev-parse HEAD)"
sudo scripts/manage-runtime-binaries.py install runtime/release-manifest.json runtime/bin
sudo install -d -m 0755 /opt/mkruntime/artifacts /opt/mkruntime/bin /etc/mkruntime/kernels /usr/local/libexec/multikernel
linux_tree=$(sudo find /home -maxdepth 3 -type d -name linux | head -1)
kerf_tree=$(sudo find /home -maxdepth 3 -type d -name kerf | head -1)
test -n "$linux_tree" && test -n "$kerf_tree"
sudo env GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=safe.directory GIT_CONFIG_VALUE_0="$linux_tree" LINUX_TREE="$linux_tree" LAB_DIR=/var/tmp/mk-maint-src bash scripts/build-runtime-vsock.sh
sudo make -C "$linux_tree" LOCALVERSION= -j16 drivers/block/nbd.ko
sudo install -m 0644 "$linux_tree/vmlinux" /opt/mkruntime/artifacts/vmlinux
sudo install -m 0644 "$linux_tree/net/vmw_vsock/mk_transport.ko" /opt/mkruntime/artifacts/mk_transport.ko
sudo install -m 0644 "$linux_tree/drivers/block/nbd.ko" /opt/mkruntime/artifacts/nbd.ko
sudo install -d -m 0755 "/lib/modules/$(uname -r)/extra"
sudo install -m 0644 /opt/mkruntime/artifacts/mk_transport.ko /opt/mkruntime/artifacts/nbd.ko "/lib/modules/$(uname -r)/extra/"
sudo depmod -a
sudo modprobe mk_transport
sudo modprobe nbd
musl-gcc -O2 -static -Wall -Wextra tools/mkvsock-nbd.c -o "$scratch/mkvsock-nbd"
musl-gcc -O2 -static -Wall -Wextra tools/mkvsock-relay.c -o "$scratch/mkvsock-relay"
sudo install -m 0755 "$scratch/mkvsock-nbd" /usr/local/libexec/multikernel/mkvsock-nbd
sudo install -m 0755 "$scratch/mkvsock-relay" /opt/mkruntime/bin/mkvsock-relay
sudo install -m 0755 "$scratch/mkvsock-relay" /usr/local/libexec/multikernel/mk-agent-relay
sudo install -m 0755 runtime/bin/mk-agent /opt/mkruntime/bin/mk-agent
sudo bash scripts/build-agent-initramfs.sh /opt/mkruntime/bin/mk-agent /opt/mkruntime/artifacts/child-initramfs.cpio.gz /opt/mkruntime/artifacts/mk_transport.ko /opt/mkruntime/bin/mkvsock-relay
# Retain the exact pinned Kerf source and expose its distro-compatible venv.
sudo cp -a "$kerf_tree" /opt/mkruntime/kerf
sudo chown -R root:root /opt/mkruntime/kerf
sudo python3 -m venv --system-site-packages /opt/mkruntime/kerf-venv
sudo /opt/mkruntime/kerf-venv/bin/pip install rdtsc==0.2.1
sudo /opt/mkruntime/kerf-venv/bin/pip install --no-deps -e /opt/mkruntime/kerf
sudo /opt/mkruntime/kerf-venv/bin/kerf --version
printf 'MAINT_HOST_BUILD_PASS\n'
