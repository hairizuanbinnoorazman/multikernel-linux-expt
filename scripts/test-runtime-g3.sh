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
generation=$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')
token=$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')
initrd=/tmp/runtime-g3-agent.cpio.gz
evidence=${EVIDENCE_DIR:-"$HOME/runtime-g3-evidence"}
console=${CONSOLE_LOG:-"$evidence/console.log"}
lifecycle=${LIFECYCLE_LOG:-"$evidence/lifecycle.json"}
control_error=${CONTROL_ERROR_LOG:-"$evidence/control.err"}
relay_log=${RELAY_LOG:-"$evidence/relay.log"}
module=${MK_TRANSPORT_MODULE:-"$HOME/src/linux/net/vmw_vsock/mk_transport.ko"}
ctl_pid=
relay_pid=

mkdir -p "$evidence"

cleanup(){ set +e; [[ -z ${ctl_pid:-} ]]||kill "$ctl_pid" 2>/dev/null; [[ -z ${relay_pid:-} ]]||kill "$relay_pid" 2>/dev/null; if [[ -d /sys/fs/multikernel/instances/$name ]];then status=$(cat /sys/fs/multikernel/instances/$name/status);[[ $status != active ]]||sudo "$kerf" kill "$name" --force;status=$(cat /sys/fs/multikernel/instances/$name/status);[[ $status != loaded ]]||sudo "$kerf" unload "$name";sudo "$kerf" delete "$name";fi;sudo "$kerf" init --cpus=none --memory=none --devices=none; if [[ -f $console ]]; then sed -i "s/$token/[REDACTED_TEST_TOKEN]/g" "$console"; fi; }
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
/tmp/mkvsock-relay server "$port" /tmp/g3-relay.sock >"$relay_log" 2>&1 &
relay_pid=$!
"$ctl" --port="$port" --unix-socket=/tmp/g3-relay.sock --sandbox-id="$name" --generation="$generation" \
	--token-hex="$token" --bundle=/bundle >"$lifecycle" 2>"$control_error" &
ctl_pid=$!
sudo timeout 45s script -qefc "$kerf exec $name --console" /dev/null >"$console" 2>&1 &
console_pid=$!
for _ in $(seq 1 40);do grep -q MK_AGENT_START "$console" 2>/dev/null&&break;sleep .5;done
grep -q MK_AGENT_START "$console"
wait "$ctl_pid"
ctl_pid=
wait "$relay_pid"
relay_pid=
LIFECYCLE_LOG="$lifecycle" python3 - <<'PY'
import json, os
x=json.load(open(os.environ['LIFECYCLE_LOG']))
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
if grep -R -F -- "$token" "$evidence"; then
	echo "generated credential remained in G3 evidence" >&2
	exit 1
fi
printf '%s\n' 'TOKEN_REDACTION_PASS: exact generated credential absent from all G3 evidence after cleanup' >"$evidence/token-redaction-check.txt"
echo RUNTIME_G3_OCI_LIFECYCLE_PASS
