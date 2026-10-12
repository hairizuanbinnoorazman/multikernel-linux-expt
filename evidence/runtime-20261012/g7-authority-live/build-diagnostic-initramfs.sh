#!/usr/bin/env bash
# Build a visibly diagnostic child fixture using the production agent/bootstrap.
set -euo pipefail
agent=${1:?usage: build-security-authority-initramfs.sh AGENT RELAY MODULE CASE OUTPUT}
relay=${2:?}
module=${3:?}
fixture=${4:?}
output=${5:?}
repo_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
case "$fixture" in nonroot|root-empty-caps|root-default) ;; *) exit 2 ;; esac
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
mkdir -p "$scratch"/{bin,dev,proc,sys,tmp,bundle/rootfs/bin,bundle/rootfs/work}
chmod 0777 "$scratch/bundle/rootfs/work"
install -m 0755 "${BUSYBOX:-$(command -v busybox)}" "$scratch/bin/busybox"
install -m 0755 "${BUSYBOX:-$(command -v busybox)}" "$scratch/bundle/rootfs/bin/busybox"
install -m 0755 "$agent" "$scratch/mk-agent"
install -m 0755 "$relay" "$scratch/mkvsock-relay"
install -m 0644 "$module" "$scratch/mk_transport.ko"
install -m 0755 "$repo_dir/guest/mk-agent-init" "$scratch/production-init"
install -m 0755 /var/tmp/mk-g7-bin/bundle-audit "$scratch/bundle-audit"
cat > "$scratch/init" <<'AUDIT'
#!/bin/busybox sh
/bundle-audit
exec /production-init
AUDIT
chmod 0755 "$scratch/init"
install -m 0755 "$repo_dir/guest/security-authority-probe.sh" "$scratch/bundle/rootfs/probe.sh"
printf '%s\n' MK_SYNTHETIC_BOOTSTRAP_CANARY > "$scratch/security-canary"
chmod 0600 "$scratch/security-canary"
python3 - "$scratch/bundle/config.json" "$fixture" <<'PY'
import json,sys,pathlib
uid=1234 if sys.argv[2]=='nonroot' else 0
process={'user':{'uid':uid,'gid':uid},'args':['/bin/busybox','sh','/probe.sh'],
         'env':['PATH=/bin'],'cwd':'/work'}
if sys.argv[2]=='root-empty-caps':
    process.update(noNewPrivileges=True,capabilities={name:[] for name in
        ['bounding','effective','permitted','inheritable','ambient']})
pathlib.Path(sys.argv[1]).write_text(json.dumps({'ociVersion':'1.1.0',
    'process':process,'root':{'path':'rootfs'}},indent=2)+'\n')
PY
(cd "$scratch" && find . -print0 | sort -z | cpio --null -o --format=newc --owner=0:0 2>/dev/null) | gzip -n -9 > "$output"
sha256sum "$output"
