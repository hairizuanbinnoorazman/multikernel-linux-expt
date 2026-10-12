#!/usr/bin/env bash
# Disposable diagnostic, using a pinned exported BuildKit image as guest root.
set -euo pipefail
umask 022
[[ $EUID == 0 ]]
agent=${1:?AGENT RELAY MODULE ROOTFS_TAR MODE OUTPUT}
relay=${2:?}; module=${3:?}; image=${4:?}; mode=${5:?}; output=${6:?}
kind=${7:-buildkit}
[[ $mode == baseline || $mode == prepared ]]
[[ $kind == buildkit || $kind == docker || $kind == docker-online ]]
account=user
[[ $kind == buildkit ]] || account=rootless
repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
# The archive's '.' entry becomes the guest root. Bootstrap drops to UID 1000.
chmod 0755 "$scratch"
mkdir -p "$scratch"/{bin,dev,proc,sys,tmp,bundle/rootfs}
chmod 1777 "$scratch/tmp"
tar --numeric-owner -xf "$image" -C "$scratch/bundle/rootfs"
install -m 0755 "${BUSYBOX:-$(command -v busybox)}" "$scratch/bin/busybox"
install -m 0755 "$agent" "$scratch/mk-agent"
install -m 0755 "$relay" "$scratch/mkvsock-relay"
install -m 0644 "$module" "$scratch/mk_transport.ko"
install -m 0755 "$repo/guest/mk-agent-init" "$scratch/init"
install -m 0755 "$repo/guest/$kind-probe.sh" "$scratch/bundle/rootfs/probe.sh"
install -m 0755 "$repo/guest/security-authority-probe.sh" "$scratch/bundle/rootfs/authority-probe.sh"
printf '%s\n' MK_SYNTHETIC_BOOTSTRAP_CANARY > "$scratch/security-canary"
chmod 0600 "$scratch/security-canary"
if [[ $kind == buildkit ]]; then
    install -m 0755 "$repo/guest/buildkit-build" "$scratch/bundle/rootfs/usr/local/bin/mk-build"
fi
install -m 0755 "${BUSYBOX:-$(command -v busybox)}" "$scratch/bundle/rootfs/usr/local/bin/build-probe-busybox"
mkdir -p "$scratch/bundle/rootfs/work" "$scratch/bundle/rootfs/home/$account/.local/tmp"
chmod 0777 "$scratch/bundle/rootfs/work"
chown -R 1000:1000 "$scratch/bundle/rootfs/home/$account"
if [[ $mode == prepared ]]; then
    # Docker export/newc do not preserve security.capability xattrs. Trusted
    # bootstrap needs the standard setuid mapping helpers before enabling NNP.
    chown 0:0 "$scratch/bundle/rootfs/usr/bin/newuidmap" "$scratch/bundle/rootfs/usr/bin/newgidmap"
    chmod 4755 "$scratch/bundle/rootfs/usr/bin/newuidmap" "$scratch/bundle/rootfs/usr/bin/newgidmap"
    # Only the pinned builder-image fixture receives these bootstrap binaries.
    # They run before accepting workloads, not as user-requested root commands.
    mkdir -p "$scratch/etc" "$scratch/usr"
    for name in passwd group subuid subgid; do
        cp "$scratch/bundle/rootfs/etc/$name" "$scratch/etc/$name"
    done
    ln -s /bundle/rootfs/usr/bin "$scratch/usr/bin"
    ln -s /bundle/rootfs/usr/lib "$scratch/usr/lib"
    ln -s /bundle/rootfs/lib "$scratch/lib"
    ln -s /bundle/rootfs/home "$scratch/home"
    if [[ $kind == docker || $kind == docker-online ]]; then
        mkdir -p "$scratch/usr/local"
        ln -s /bundle/rootfs/usr/local/bin "$scratch/usr/local/bin"
        mkdir -p "$scratch/bootstrap-modules"
        for component in llc stp bridge veth; do
            case $component in
                llc) relative=net/llc/llc.ko ;;
                stp) relative=net/802/stp.ko ;;
                bridge) relative=net/bridge/bridge.ko ;;
                veth) relative=drivers/net/veth.ko ;;
            esac
            install -m 0600 "/lib/modules/$(uname -r)/kernel/$relative" "$scratch/bootstrap-modules/$component.ko"
            sha256sum "/lib/modules/$(uname -r)/kernel/$relative"
        done
    fi
    if [[ $kind == docker-online ]]; then
        # Resolve dependency order against this exact installed guest kernel.
        for target in iptable_nat iptable_filter iptable_raw iptable_mangle ip6table_filter xt_addrtype xt_conntrack xt_MASQUERADE xt_tcpudp; do
            modprobe --show-depends "$target"
        done > "$scratch/netfilter-dependencies.txt"
        mkdir -p "$scratch/bootstrap-netfilter" "$scratch/bundle/rootfs/online-app"
        while read -r operation path parameters; do
            [[ $operation == insmod ]] || continue
            install -m 0600 "$path" "$scratch/bootstrap-netfilter/$(basename "$path")"
            sha256sum "$path"
        done < "$scratch/netfilter-dependencies.txt"
        awk '$1 == "insmod" {n=split($2,p,"/"); print "/bootstrap-netfilter/" p[n]}' "$scratch/netfilter-dependencies.txt" | awk '!seen[$0]++' > "$scratch/netfilter-load-order"
        cp "$repo"/examples/guest-python-online/{Dockerfile,requirements.txt,app.py} "$scratch/bundle/rootfs/online-app/"
        ln -s /bundle/rootfs/sbin "$scratch/sbin"
        rm "$scratch/bundle/rootfs/usr/bin/nsenter"
        install -m 0755 "$repo/guest/docker-online-nsenter" "$scratch/bundle/rootfs/usr/bin/nsenter"
        printf 'nameserver 8.8.8.8\n' > "$scratch/etc/resolv.conf"
        printf '127.0.0.1 localhost\n' > "$scratch/etc/hosts"
    fi
    install -m 0755 "$repo/guest/$kind-bootstrap.sh" "$scratch/$kind-bootstrap"
    # This hook exists only in the diagnostic image, not the production init.
    sed -i "/^sandbox_id=$/i /$kind-bootstrap" "$scratch/init"
fi
python3 - "$scratch/bundle/config.json" "$mode" "$kind" "$account" <<'PY'
import json,pathlib,sys
mode=sys.argv[2]
uid=1000 if mode=='baseline' else 0
if sys.argv[3]!='buildkit': uid=0
config={'ociVersion':'1.1.0','root':{'path':'rootfs'},'process':{
    'user':{'uid':uid,'gid':uid},'args':['/bin/sh','/probe.sh'],'cwd':'/work',
    'env':['PATH=/usr/local/bin:/usr/bin:/bin','HOME=/home/'+sys.argv[4],'USER='+sys.argv[4],
           'XDG_RUNTIME_DIR=/run/user/1000','TMPDIR=/home/'+sys.argv[4]+'/.local/tmp',
           'MK_BUILD_PROBE_MODE='+mode]}}
if sys.argv[3]!='buildkit':
    config['process']['env']=['PATH=/usr/local/bin:/usr/bin:/bin','HOME=/root','USER=root',
                              'TMPDIR=/work','MK_BUILD_PROBE_MODE='+mode]
pathlib.Path(sys.argv[1]).write_text(json.dumps(config,indent=2)+'\n')
PY
chmod 0600 "$scratch/bundle/config.json"
(cd "$scratch" && find . -print0 | sort -z | cpio --null -o --format=newc 2>/dev/null) | gzip -n -1 > "$output"
sha256sum "$output"
