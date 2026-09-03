#!/usr/bin/env bash
set -euo pipefail
runtime=${MKRUNTIMED:-"$HOME/mkruntimed"}
kerf=${KERF:-"$HOME/src/kerf/.venv/bin/kerf"}
kernel=${KERNEL:-"$HOME/src/linux/vmlinux"}
initrd=${INITRD:-"$HOME/multikernel-artifacts/child-initramfs.cpio.gz"}
socket=/tmp/mkruntimed-g2.sock
state=${STATE_DIR:-/tmp/mkruntimed-g2-state}
evidence=${EVIDENCE_DIR:-/tmp/runtime-g2-evidence}
daemon_log=${DAEMON_LOG:-$evidence/mkruntimed-g2.log}
pool_memory=${POOL_MEMORY:-12GB}
expected_second_create_error=${EXPECT_SECOND_CREATE_ERROR:-}
pid_file=/tmp/mkruntimed-g2.pid
pid=
supervisor_pid=

mkdir -p "$evidence"

retain_state() {
	local label=$1
	sudo cp -a "$state" "$evidence/$label"
	sudo chown -R "$(id -u):$(id -g)" "$evidence/$label"
}

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
	sudo rm -f "$pid_file"
	sudo sh -c 'pid_file=$1; shift; echo "$$" >"$pid_file"; exec "$@"' sh "$pid_file" \
		"$runtime" --socket="$socket" --state-dir="$state" \
		--kerf="$kerf" --pool-cpus=8,10,12,14 --pool-memory="$pool_memory" \
		--kernel="$kernel" --initrd="$initrd" \
		--cmdline='rdinit=/init console=mktty0 panic=-1' >>"$daemon_log" 2>&1 &
	supervisor_pid=$!
	for _ in $(seq 1 50); do
		if [[ -S $socket && -s $pid_file ]]; then
			pid=$(sudo cat "$pid_file")
			return
		fi
		sleep .1
	done
	return 1
}
cleanup() {
	set +e
	[[ -z ${pid:-} ]] || sudo kill "$pid" 2>/dev/null
	[[ -z ${supervisor_pid:-} ]] || wait "$supervisor_pid" 2>/dev/null
	for sandbox in runtime-g2 runtime-g2-b; do
		if [[ -d /sys/fs/multikernel/instances/$sandbox ]]; then
			status=$(cat "/sys/fs/multikernel/instances/$sandbox/status")
			[[ $status != active ]] || sudo "$kerf" kill "$sandbox" --force
			status=$(cat "/sys/fs/multikernel/instances/$sandbox/status")
			[[ $status != loaded ]] || sudo "$kerf" unload "$sandbox"
			sudo "$kerf" delete "$sandbox"
		fi
	done
	sudo "$kerf" init --cpus=none --memory=none --devices=none
}
trap cleanup EXIT
sudo rm -f "$socket"
sudo rm -rf "$state"
mkdir -p /tmp/runtime-g2-bundle /tmp/runtime-g2-b-bundle
start_daemon
create=$(request '{"version":1,"request_id":"1","method":"CreateSandbox","idempotency_key":"g2-create","body":{"schema_version":1,"id":"runtime-g2","cpus":[8,10],"memory_bytes":4294967296,"kernel_manifest":"gce-mk2","bundle":"/tmp/runtime-g2-bundle","agent_port":7102,"child_cid":22}}')
printf '%s\n' "$create" >"$evidence/g2-create.json"
generation=$(python3 -c 'import json,sys; x=json.load(sys.stdin); assert not x.get("error"), x; print(x["body"]["sandbox"]["generation"])' <<<"$create")
request "{\"version\":1,\"request_id\":\"2\",\"method\":\"LoadSandbox\",\"sandbox_id\":\"runtime-g2\",\"generation\":\"$generation\",\"idempotency_key\":\"g2-load\"}" >"$evidence/g2-load.json"
python3 -c 'import json,sys; x=json.load(open(sys.argv[1])); assert not x.get("error"), x' "$evidence/g2-load.json"
request "{\"version\":1,\"request_id\":\"3\",\"method\":\"StartSandbox\",\"sandbox_id\":\"runtime-g2\",\"generation\":\"$generation\",\"idempotency_key\":\"g2-start\"}" >"$evidence/g2-start.json"
python3 -c 'import json,sys; x=json.load(open(sys.argv[1])); assert not x.get("error"), x' "$evidence/g2-start.json"
test "$(cat /sys/fs/multikernel/instances/runtime-g2/status)" = active
create_b=$(request '{"version":1,"request_id":"1b","method":"CreateSandbox","idempotency_key":"g2b-create","body":{"schema_version":1,"id":"runtime-g2-b","cpus":[12,14],"memory_bytes":4294967296,"kernel_manifest":"gce-mk2","bundle":"/tmp/runtime-g2-b-bundle","agent_port":7104,"child_cid":24}}')
printf '%s\n' "$create_b" >"$evidence/g2b-create.json"
if [[ -n $expected_second_create_error ]]; then
	EXPECTED_ERROR="$expected_second_create_error" RESPONSE_PATH="$evidence/g2b-create.json" python3 - <<'PY'
import json, os
x = json.load(open(os.environ["RESPONSE_PATH"]))
assert x.get("error", {}).get("code") == os.environ["EXPECTED_ERROR"], x
PY
	echo RUNTIME_G2_POOL_MEMORY_PREFLIGHT_PASS
	exit 0
fi
generation_b=$(python3 -c 'import json,sys; x=json.load(sys.stdin); assert not x.get("error"), x; print(x["body"]["sandbox"]["generation"])' <<<"$create_b")
request "{\"version\":1,\"request_id\":\"2b\",\"method\":\"LoadSandbox\",\"sandbox_id\":\"runtime-g2-b\",\"generation\":\"$generation_b\",\"idempotency_key\":\"g2b-load\"}" >"$evidence/g2b-load.json"
python3 -c 'import json,sys; x=json.load(open(sys.argv[1])); assert not x.get("error"), x' "$evidence/g2b-load.json"
request "{\"version\":1,\"request_id\":\"3b\",\"method\":\"StartSandbox\",\"sandbox_id\":\"runtime-g2-b\",\"generation\":\"$generation_b\",\"idempotency_key\":\"g2b-start\"}" >"$evidence/g2b-start.json"
python3 -c 'import json,sys; x=json.load(open(sys.argv[1])); assert not x.get("error"), x' "$evidence/g2b-start.json"
test "$(cat /sys/fs/multikernel/instances/runtime-g2-b/status)" = active
sudo kill -KILL "$pid"
wait "$supervisor_pid" 2>/dev/null || true
pid=
supervisor_pid=
retain_state state-after-sigkill
sudo rm -f "$socket"
start_daemon
request "{\"version\":1,\"request_id\":\"4\",\"method\":\"StopSandbox\",\"sandbox_id\":\"runtime-g2\",\"generation\":\"$generation\",\"idempotency_key\":\"g2-stop\"}" >"$evidence/g2-stop.json"
python3 -c 'import json,sys; x=json.load(open(sys.argv[1])); assert not x.get("error"), x' "$evidence/g2-stop.json"
request "{\"version\":1,\"request_id\":\"5\",\"method\":\"DeleteSandbox\",\"sandbox_id\":\"runtime-g2\",\"generation\":\"$generation\",\"idempotency_key\":\"g2-delete\"}" >"$evidence/g2-delete.json"
python3 -c 'import json,sys; x=json.load(open(sys.argv[1])); assert not x.get("error"), x' "$evidence/g2-delete.json"
request "{\"version\":1,\"request_id\":\"4b\",\"method\":\"StopSandbox\",\"sandbox_id\":\"runtime-g2-b\",\"generation\":\"$generation_b\",\"idempotency_key\":\"g2b-stop\"}" >"$evidence/g2b-stop.json"
python3 -c 'import json,sys; x=json.load(open(sys.argv[1])); assert not x.get("error"), x' "$evidence/g2b-stop.json"
request "{\"version\":1,\"request_id\":\"5b\",\"method\":\"DeleteSandbox\",\"sandbox_id\":\"runtime-g2-b\",\"generation\":\"$generation_b\",\"idempotency_key\":\"g2b-delete\"}" >"$evidence/g2b-delete.json"
python3 -c 'import json,sys; x=json.load(open(sys.argv[1])); assert not x.get("error"), x' "$evidence/g2b-delete.json"
sudo kill "$pid"
wait "$supervisor_pid" || true
pid=
supervisor_pid=
retain_state state-after-cleanup
test "$(cat /sys/devices/system/cpu/online)" = 0-15
test ! -d /sys/fs/multikernel/instances/runtime-g2
test ! -d /sys/fs/multikernel/instances/runtime-g2-b
echo RUNTIME_G2_RESTART_RECONCILE_PASS
