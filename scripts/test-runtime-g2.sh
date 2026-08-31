#!/usr/bin/env bash
set -euo pipefail
runtime=${MKRUNTIMED:-"$HOME/mkruntimed"}
kerf=${KERF:-"$HOME/src/kerf/.venv/bin/kerf"}
kernel=${KERNEL:-"$HOME/src/linux/vmlinux"}
initrd=${INITRD:-"$HOME/multikernel-artifacts/child-initramfs.cpio.gz"}
socket=/tmp/mkruntimed-g2.sock
state=/tmp/mkruntimed-g2-state
daemon_log=/tmp/mkruntimed-g2.log
pid=

request() {
	local json=$1
	sudo python3 - "$socket" "$json" <<'PY'
import socket, sys
s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
s.connect(sys.argv[1])
s.sendall(sys.argv[2].encode())
s.shutdown(socket.SHUT_WR)
data = b""
while True:
    part = s.recv(65536)
    if not part: break
    data += part
print(data.decode(), end="")
PY
}
start_daemon() {
	sudo "$runtime" --socket="$socket" --state-dir="$state" \
		--kerf="$kerf" --pool-cpus=8,10,12,14 --pool-memory=8GB \
		--kernel="$kernel" --initrd="$initrd" \
		--cmdline='rdinit=/init console=mktty0 panic=-1' >>"$daemon_log" 2>&1 &
	pid=$!
	for _ in $(seq 1 50); do [[ -S $socket ]] && return; sleep .1; done
	return 1
}
cleanup() {
	set +e
	[[ -z ${pid:-} ]] || sudo kill "$pid" 2>/dev/null
	if [[ -d /sys/fs/multikernel/instances/runtime-g2 ]]; then
		status=$(cat /sys/fs/multikernel/instances/runtime-g2/status)
		[[ $status != active ]] || sudo "$kerf" kill runtime-g2 --force
		status=$(cat /sys/fs/multikernel/instances/runtime-g2/status)
		[[ $status != loaded ]] || sudo "$kerf" unload runtime-g2
		sudo "$kerf" delete runtime-g2
	fi
	sudo "$kerf" init --cpus=none --memory=none --devices=none
}
trap cleanup EXIT
sudo rm -f "$socket"
sudo rm -rf "$state"
mkdir -p /tmp/runtime-g2-bundle
start_daemon
create=$(request '{"version":1,"request_id":"1","method":"CreateSandbox","idempotency_key":"g2-create","body":{"schema_version":1,"id":"runtime-g2","cpus":[8,10],"memory_bytes":4294967296,"kernel_manifest":"gce-mk2","bundle":"/tmp/runtime-g2-bundle","agent_port":7102,"child_cid":22}}')
generation=$(python3 -c 'import json,sys; x=json.load(sys.stdin); assert not x.get("error"), x; print(x["body"]["sandbox"]["generation"])' <<<"$create")
request "{\"version\":1,\"request_id\":\"2\",\"method\":\"LoadSandbox\",\"sandbox_id\":\"runtime-g2\",\"generation\":\"$generation\",\"idempotency_key\":\"g2-load\"}" >/tmp/g2-load.json
python3 -c 'import json; x=json.load(open("/tmp/g2-load.json")); assert not x.get("error"), x'
request "{\"version\":1,\"request_id\":\"3\",\"method\":\"StartSandbox\",\"sandbox_id\":\"runtime-g2\",\"generation\":\"$generation\",\"idempotency_key\":\"g2-start\"}" >/tmp/g2-start.json
python3 -c 'import json; x=json.load(open("/tmp/g2-start.json")); assert not x.get("error"), x'
test "$(cat /sys/fs/multikernel/instances/runtime-g2/status)" = active
sudo kill -KILL "$pid"
wait "$pid" 2>/dev/null || true
pid=
sudo rm -f "$socket"
start_daemon
request "{\"version\":1,\"request_id\":\"4\",\"method\":\"StopSandbox\",\"sandbox_id\":\"runtime-g2\",\"generation\":\"$generation\",\"idempotency_key\":\"g2-stop\"}" >/tmp/g2-stop.json
python3 -c 'import json; x=json.load(open("/tmp/g2-stop.json")); assert not x.get("error"), x'
request "{\"version\":1,\"request_id\":\"5\",\"method\":\"DeleteSandbox\",\"sandbox_id\":\"runtime-g2\",\"generation\":\"$generation\",\"idempotency_key\":\"g2-delete\"}" >/tmp/g2-delete.json
python3 -c 'import json; x=json.load(open("/tmp/g2-delete.json")); assert not x.get("error"), x'
sudo kill "$pid"
wait "$pid" || true
pid=
test "$(cat /sys/devices/system/cpu/online)" = 0-15
test ! -d /sys/fs/multikernel/instances/runtime-g2
echo RUNTIME_G2_RESTART_RECONCILE_PASS
