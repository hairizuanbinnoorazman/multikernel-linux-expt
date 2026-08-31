#!/usr/bin/env bash
set -euo pipefail
kerf=${KERF:-"$HOME/src/kerf/.venv/bin/kerf"}
kernel=${KERNEL:-"$HOME/src/linux/vmlinux"}
lab=${LAB_DIR:-"$HOME/multikernel-linux-lab"}
agent=${MK_AGENT:-"$HOME/mk-agent"}
ctl=${MK_AGENTCTL:-"$HOME/mk-agentctl"}
name=runtime-g3
cid=23
port=7103
generation=0123456789abcdef0123456789abcdef
token=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
initrd=/tmp/runtime-g3-agent.cpio.gz
console="$HOME/runtime-g3-console.log"
module=${MK_TRANSPORT_MODULE:-"$HOME/src/linux/net/vmw_vsock/mk_transport.ko"}
ctl_pid=
relay_pid=

cleanup(){ set +e; [[ -z ${ctl_pid:-} ]]||kill "$ctl_pid" 2>/dev/null; [[ -z ${relay_pid:-} ]]||kill "$relay_pid" 2>/dev/null; if [[ -d /sys/fs/multikernel/instances/$name ]];then status=$(cat /sys/fs/multikernel/instances/$name/status);[[ $status != active ]]||sudo "$kerf" kill "$name" --force;status=$(cat /sys/fs/multikernel/instances/$name/status);[[ $status != loaded ]]||sudo "$kerf" unload "$name";sudo "$kerf" delete "$name";fi;sudo "$kerf" init --cpus=none --memory=none --devices=none; }
trap cleanup EXIT
sudo modprobe vsock
lsmod | grep -q '^mk_transport ' || sudo insmod "$module"
musl-gcc -static -O2 -Wall -Wextra -Werror "$lab/tools/mkvsock-relay.c" -o /tmp/mkvsock-relay
"$lab/scripts/build-agent-initramfs.sh" "$agent" "$initrd" "$module" /tmp/mkvsock-relay
sudo "$kerf" init --cpus=8,10,12,14 --memory=8GB --devices=none
sudo "$kerf" create "$name" --id="$cid" --cpus=8,10 --memory=4GB
sudo "$kerf" load "$name" --kernel="$kernel" --initrd="$initrd" \
	--cmdline="rdinit=/init console=mktty0 panic=-1 mk.sandbox_id=$name mk.generation=$generation mk.token=$token mk.agent_port=$port"
rm -f /tmp/g3-relay.sock
/tmp/mkvsock-relay server "$port" /tmp/g3-relay.sock >"$HOME/g3-relay.log" 2>&1 &
relay_pid=$!
"$ctl" --port="$port" --unix-socket=/tmp/g3-relay.sock --sandbox-id="$name" --generation="$generation" \
	--token-hex="$token" --bundle=/bundle >"$HOME/g3-lifecycle.json" 2>"$HOME/g3-control.err" &
ctl_pid=$!
sudo timeout 45s script -qefc "$kerf exec $name --console" /dev/null >"$console" 2>&1 &
console_pid=$!
for _ in $(seq 1 40);do grep -q MK_AGENT_START "$console" 2>/dev/null&&break;sleep .5;done
grep -q MK_AGENT_START "$console"
wait "$ctl_pid"
ctl_pid=
wait "$relay_pid"
relay_pid=
python3 - <<'PY'
import json, os
x=json.load(open(os.path.join(os.environ['HOME'], 'g3-lifecycle.json')))
body=x['WaitProcess']
assert body['exit_code']==23, body
assert 'OCI_STDOUT' in body['stdout'], body
assert 'OCI_STDERR' in body['stderr'], body
assert '7.0.0-mk2-gce-lab' in body['stdout'], body
PY
cleanup
trap - EXIT
wait "$console_pid" 2>/dev/null || true
test "$(cat /sys/devices/system/cpu/online)" = 0-15
echo RUNTIME_G3_OCI_LIFECYCLE_PASS
