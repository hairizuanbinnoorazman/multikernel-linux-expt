#!/usr/bin/env bash
# Build a visibly diagnostic child fixture using the production agent/bootstrap.
set -euo pipefail
umask 022
agent=${1:?usage: build-security-authority-initramfs.sh AGENT RELAY MODULE CASE OUTPUT}
relay=${2:?}
module=${3:?}
fixture=${4:?}
output=${5:?}
repo_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
case "$fixture" in nonroot|root-empty-caps|root-default|root-ordinary-caps|root-unsafe-cap|root-disable-nnp) ;; *) exit 2 ;; esac
scratch=$(mktemp -d)
archive=
trap 'rm -rf -- "$scratch"; [[ -z ${archive:-} ]] || rm -f -- "$archive"' EXIT
archive=$(mktemp)
mkdir -p "$scratch"/{bin,dev,proc,sys,tmp,bundle/rootfs/bin,bundle/rootfs/work}
if [[ -n ${MK_AGENT_TOKEN_FILE:-} ]]; then
    mkdir -m 0700 "$scratch/.mk-agent"
    install -m 0600 "$MK_AGENT_TOKEN_FILE" "$scratch/.mk-agent/token"
fi
chmod 0777 "$scratch/bundle/rootfs/work"
install -m 0755 "${BUSYBOX:-$(command -v busybox)}" "$scratch/bin/busybox"
install -m 0755 "${BUSYBOX:-$(command -v busybox)}" "$scratch/bundle/rootfs/bin/busybox"
install -m 0755 "$agent" "$scratch/mk-agent"
install -m 0755 "$relay" "$scratch/mkvsock-relay"
install -m 0644 "$module" "$scratch/mk_transport.ko"
install -m 0755 "$repo_dir/guest/mk-agent-init" "$scratch/init"
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
if sys.argv[2] in ['root-ordinary-caps','root-unsafe-cap']:
    values=['CAP_CHOWN','CAP_DAC_OVERRIDE','CAP_FOWNER','CAP_FSETID','CAP_KILL',
            'CAP_SETGID','CAP_SETUID','CAP_SETPCAP','CAP_NET_BIND_SERVICE','CAP_NET_RAW',
            'CAP_SYS_CHROOT','CAP_MKNOD','CAP_AUDIT_WRITE','CAP_SETFCAP']
    if sys.argv[2]=='root-unsafe-cap': values=['CAP_SYS_PTRACE']
    process['capabilities']={name:values if name in ['bounding','effective','permitted'] else []
        for name in ['bounding','effective','permitted','inheritable','ambient']}
if sys.argv[2]=='root-disable-nnp': process['noNewPrivileges']=False
pathlib.Path(sys.argv[1]).write_text(json.dumps({'ociVersion':'1.1.0',
    'process':process,'root':{'path':'rootfs'}},indent=2)+'\n')
PY
chmod 0600 "$scratch/bundle/config.json"
(cd "$scratch" && find . -print0 | sort -z | cpio --null -o --format=newc --owner=0:0 2>/dev/null) | gzip -n -9 > "$archive"
python3 -I "$repo_dir/scripts/publish-private-initramfs.py" "$output" < "$archive"
sha256sum "$output"
