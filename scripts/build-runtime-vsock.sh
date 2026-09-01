#!/usr/bin/env bash
set -euo pipefail
linux_tree=${LINUX_TREE:-"$HOME/src/linux"}
lab=${LAB_DIR:-"$HOME/multikernel-linux-lab"}
patch="$lab/patches/linux-v7.0-mk2-vsock-build-safety.patch"

cd "$linux_tree"
test "$(git rev-parse HEAD)" = 3bdd35b64413da0b4e089ce931bfc2e8b031cbf7
if git apply --check "$patch" 2>/dev/null; then
	git apply "$patch"
elif ! git apply --reverse --check "$patch" 2>/dev/null; then
	echo 'transport patch is neither applicable nor already applied' >&2
	exit 1
fi
scripts/config --module MULTIKERNEL_VSOCKETS
# Applying the pinned out-of-tree safety patch makes the source worktree dirty.
# Do not let CONFIG_LOCALVERSION_AUTO append "-dirty" and produce a module
# that the already-running, otherwise identical kernel refuses to load.
scripts/config --disable LOCALVERSION_AUTO
make LOCALVERSION= olddefconfig
make LOCALVERSION= -j"$(nproc)" net/vmw_vsock/mk_transport.ko
test "$(make LOCALVERSION= -s kernelrelease)" = "$(uname -r)"
modinfo net/vmw_vsock/mk_transport.ko | grep -E '^(filename|vermagic):'
sha256sum net/vmw_vsock/mk_transport.ko
