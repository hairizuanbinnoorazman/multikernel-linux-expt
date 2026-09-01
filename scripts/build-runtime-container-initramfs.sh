#!/usr/bin/env bash
set -euo pipefail

bundle=${1:?usage: build-runtime-container-initramfs.sh BUNDLE OUTPUT}
output=${2:?usage: build-runtime-container-initramfs.sh BUNDLE OUTPUT}
agent=${MK_AGENT:-/usr/local/libexec/multikernel/mk-agent}
relay=${MK_RELAY:-/usr/local/libexec/multikernel/mkvsock-relay}
module=${MK_TRANSPORT_MODULE:-/usr/local/libexec/multikernel/mk_transport.ko}
busybox=${BUSYBOX:-$(command -v busybox)}
script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
guest_init=${MK_GUEST_INIT:-"$script_dir/mk-agent-init"}
if [[ ! -f "$guest_init" ]]; then
	guest_init=$(cd "$script_dir/.." && pwd)/guest/mk-agent-init
fi
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

test -f "$bundle/config.json"
test -x "$agent"
test -x "$relay"
test -f "$module"

mkdir -p "$work"/{bin,dev,proc,sys,tmp,bundle/rootfs}
install -m 0755 "$busybox" "$work/bin/busybox"
install -m 0755 "$agent" "$work/mk-agent"
install -m 0755 "$relay" "$work/mkvsock-relay"
install -m 0644 "$module" "$work/mk_transport.ko"
install -m 0755 "$guest_init" "$work/init"

# containerd remains the image/snapshot owner. ctr passes snapshot mounts for
# bundle/rootfs; Docker's containerd image store can instead pass an absolute
# OCI root.path. Copy either prepared root into a private per-sandbox artifact
# without modifying the snapshot.
root_path=$(jq -er '.root.path' "$bundle/config.json")
if [[ "$root_path" = /* ]]; then
	source_root=$root_path
else
	source_root=$bundle/$root_path
fi
test -d "$source_root"
cp -a "$source_root/." "$work/bundle/rootfs/"
jq '{ociVersion:(.ociVersion // "1.1.0"), process:{terminal:false,user:(.process.user // {uid:0,gid:0}),args:.process.args,env:(.process.env // []),cwd:(.process.cwd // "/")},root:{path:"rootfs"}}' \
	"$bundle/config.json" >"$work/bundle/config.json"

(cd "$work" && find . -xdev -print0 | sort -z | cpio --null -o --format=newc --owner=0:0 2>/dev/null) | gzip -n -9 >"$output"
chmod 0600 "$output"
sha256sum "$output"
