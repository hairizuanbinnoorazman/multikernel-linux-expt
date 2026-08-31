#!/usr/bin/env bash
set -euo pipefail

agent=${1:?usage: build-agent-initramfs.sh MK_AGENT OUTPUT}
output=${2:?usage: build-agent-initramfs.sh MK_AGENT OUTPUT}
transport_module=${3:-}
relay=${4:-}
repo_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
busybox=${BUSYBOX:-$(command -v busybox)}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

mkdir -p "$work"/{bin,dev,proc,sys,tmp,bundle/rootfs/bin}
install -m 0755 "$busybox" "$work/bin/busybox"
install -m 0755 "$busybox" "$work/bundle/rootfs/bin/busybox"
install -m 0755 "$agent" "$work/mk-agent"
if [[ -n "$relay" ]]; then install -m 0755 "$relay" "$work/mkvsock-relay"; fi
if [[ -n "$transport_module" ]]; then
	install -m 0644 "$transport_module" "$work/mk_transport.ko"
fi
install -m 0755 "$repo_dir/guest/mk-agent-init" "$work/init"
ln -s busybox "$work/bundle/rootfs/bin/sh"

cat >"$work/bundle/config.json" <<'EOF'
{
  "ociVersion": "1.1.0",
  "process": {
    "user": {"uid": 0, "gid": 0},
    "args": ["/bin/busybox", "sh", "-c", "printf OCI_STDOUT; printf OCI_STDERR >&2; uname -r; exit 23"],
    "env": ["PATH=/bin"],
    "cwd": "/"
  },
  "root": {"path": "rootfs"}
}
EOF

(cd "$work" && find . -print0 | sort -z | cpio --null -o --format=newc --owner=0:0 2>/dev/null) | gzip -n -9 >"$output"
sha256sum "$output"
