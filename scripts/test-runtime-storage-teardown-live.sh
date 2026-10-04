#!/usr/bin/env bash
set -euo pipefail

if [[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]]; then
	PS4='+${BASH_SOURCE}:${LINENO}: '
	set -x
fi

task_id=mk-storage-teardown
image=${MK_TEST_IMAGE:-docker.io/library/busybox:1.36}
runtime=${MK_RUNTIME:-io.containerd.multikernel.v2}
kerf=${MK_KERF:-/opt/mkruntime/kerf-venv/bin/kerf}
scratch=$(mktemp -d -p /var/tmp mk-storage-teardown.XXXXXX)
console_raw=$scratch/console.raw
console_pid=

observe() {
	local key=$1
	shift
	printf 'OBSERVATION_BEGIN key=%s\n%s\nOBSERVATION_END key=%s\n' "$key" "$*" "$key"
}

cleanup_task() {
	sudo ctr tasks kill --signal SIGKILL "$task_id" >/dev/null 2>&1 || true
	for _ in $(seq 1 120); do
		local state
		state=$(sudo ctr tasks list | awk -v id="$task_id" '$1 == id {print $3}')
		[[ $state != RUNNING && $state != PAUSED ]] && break
		sleep .25
	done
	sudo ctr tasks rm -f "$task_id" >/dev/null 2>&1 || true
	sudo ctr containers rm "$task_id" >/dev/null 2>&1 || true
}

cleanup() (
	set +e
	[[ -z $console_pid ]] || kill "$console_pid" >/dev/null 2>&1 || true
	[[ -z $console_pid ]] || wait "$console_pid" >/dev/null 2>&1 || true
	cleanup_task
	rm -rf -- "$scratch"
)
trap cleanup EXIT

wait_task_state() {
	local expected=$1 state=
	for _ in $(seq 1 240); do
		state=$(sudo ctr tasks list | awk -v id="$task_id" '$1 == id {print $3}')
		[[ $state = "$expected" ]] && return 0
		sleep .25
	done
	return 1
}

live_counts() {
	sudo python3 - <<'PY'
import json
rootfs=json.load(open('/var/lib/mkruntimed/rootfs/state.json'))
storage=json.load(open('/var/lib/mkruntimed/storage/state.json'))
print('rootfs_records=%d live_exports=%d' % (
    len(rootfs['records']),
    sum(v['state'] != 'RELEASED' for v in storage['exports'].values())))
PY
}

wait_counts() {
	local expected=$1 observed=
	for _ in $(seq 1 240); do
		observed=$(live_counts)
		[[ $observed = "$expected" ]] && { printf '%s' "$observed"; return 0; }
		sleep .25
	done
	printf '%s' "$observed"
	return 1
}

active_summary() {
	sudo python3 - <<'PY'
import glob,json,os
storage=json.load(open('/var/lib/mkruntimed/storage/state.json'))
active=[v for v in storage['exports'].values() if v['state'] == 'ACTIVE']
assert len(active) == 1, active
value=active[0]
records=[]
for path in glob.glob('/run/mkstorage/*.json'):
    record=json.load(open(path))
    if record['export_generation'] == value['export_generation']:
        records.append((path,record))
assert len(records) == 1, records
record_path,record=records[0]
log_path=record_path[:-5]+'.log'
assert os.path.isfile(log_path), log_path
print(json.dumps({
  'sandbox_id':value['sandbox_id'],
  'sandbox_generation':value['sandbox_generation'],
  'export_generation':value['export_generation'],
  'image_path':value['path'],
  'record_path':record_path,
  'log_path':log_path,
},sort_keys=True,separators=(',',':')))
PY
}

released_summary() {
	sudo python3 - "$1" "$2" "$3" <<'PY'
import json,re,sys
sandbox_id,sandbox_generation,export_generation=sys.argv[1:]
key=sandbox_id+'\0'+sandbox_generation
rootfs=json.load(open('/var/lib/mkruntimed/rootfs/state.json'))
storage=json.load(open('/var/lib/mkruntimed/storage/state.json'))
value=storage['exports'][key]
assert not rootfs['records'], rootfs
assert sum(v['state'] != 'RELEASED' for v in storage['exports'].values()) == 0
assert value['state'] == 'RELEASED'
assert value['export_generation'] == export_generation
assert re.fullmatch(r'e2fsck-clean-sha256:[0-9a-f]{64}',value['offline_check'])
counters=value['counters']
assert counters['flushes'] > 0 and counters['writes'] > 0 and counters['written_bytes'] > 0, counters
print(json.dumps({
  'state':value['state'], 'offline_check':value['offline_check'],
  'counters':counters, 'rootfs_records':0, 'live_exports':0,
},sort_keys=True,separators=(',',':')))
PY
}

[[ $(id -u) -ne 0 ]] || { echo 'run as an ordinary sudo-capable user' >&2; exit 1; }
test -x "$kerf"
umask 077
cleanup_task
observe clean-before "$(wait_counts 'rootfs_records=0 live_exports=0')"
observe provenance "boot_id=$(cat /proc/sys/kernel/random/boot_id)
selector=$(readlink -f /usr/local/lib/multikernel/current)
mkruntimed_sha256=$(sudo sha256sum /proc/$(systemctl show -p MainPID --value mkruntimed)/exe | awk '{print $1}')
qualifier_sha256=$(sha256sum "$0" | awk '{print $1}')"

guest_program='set -eu; dd if=/dev/zero of=/tmp/teardown-data bs=4096 count=32 status=none; sync; touch /tmp/teardown-ready; while [ ! -e /tmp/teardown-release ]; do sleep 1; done'
sudo ctr run --detach --runtime "$runtime" "$image" "$task_id" /bin/sh -c "$guest_program"
wait_task_state RUNNING
for _ in $(seq 1 120); do
	sudo ctr task exec --exec-id "teardown-ready-$RANDOM" "$task_id" /bin/test -e /tmp/teardown-ready >/dev/null 2>&1 && break
	sleep .25
done
sudo ctr task exec --exec-id teardown-ready-final "$task_id" /bin/test -e /tmp/teardown-ready
wait_counts 'rootfs_records=1 live_exports=1' >/dev/null
summary=$(active_summary)
observe active-generation "$summary"
sandbox_id=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["sandbox_id"])' "$summary")
sandbox_generation=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["sandbox_generation"])' "$summary")
export_generation=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["export_generation"])' "$summary")
image_path=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["image_path"])' "$summary")
record_path=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["record_path"])' "$summary")
log_path=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["log_path"])' "$summary")

sudo timeout --signal=TERM 120s script -qefc "$kerf console $sandbox_id --verbose" /dev/null >"$console_raw" 2>&1 &
console_pid=$!
sleep 1
kill -0 "$console_pid"
sudo ctr task exec --exec-id teardown-release "$task_id" /bin/touch /tmp/teardown-release
wait_task_state STOPPED
sudo ctr tasks rm "$task_id" >/dev/null
sudo ctr containers rm "$task_id"

for _ in $(seq 1 240); do
	grep -aFq 'MK_STORAGE_ROOT_QUIESCE_PASS stages=sync,remount-ro,sync device=/dev/nbd0' "$console_raw" &&
		grep -aFq 'MK_STORAGE_NBD_DISCONNECT_PASS device=/dev/nbd0' "$console_raw" && break
	sleep .25
done
quiesce_line=$(grep -anF 'MK_STORAGE_ROOT_QUIESCE_PASS stages=sync,remount-ro,sync device=/dev/nbd0' "$console_raw" | tail -1 | cut -d: -f1)
disconnect_line=$(grep -anF 'MK_STORAGE_NBD_DISCONNECT_PASS device=/dev/nbd0' "$console_raw" | tail -1 | cut -d: -f1)
[[ $quiesce_line =~ ^[0-9]+$ && $disconnect_line =~ ^[0-9]+$ && $quiesce_line -lt $disconnect_line ]]
observe guest-shutdown-order "MK_STORAGE_ROOT_QUIESCE_PASS stages=sync,remount-ro,sync device=/dev/nbd0
MK_STORAGE_NBD_DISCONNECT_PASS device=/dev/nbd0
console_line_order=$quiesce_line,$disconnect_line"
kill "$console_pid" >/dev/null 2>&1 || true
wait "$console_pid" >/dev/null 2>&1 || true
console_pid=

sudo test ! -e "$record_path"
terminal=$(sudo tail -n 1 "$log_path")
[[ $terminal =~ ^MKNBD_SERVER_CLOSED\ synced=1\ reads=[0-9]+\ read_bytes=[0-9]+\ writes=[1-9][0-9]*\ write_bytes=[1-9][0-9]*\ flushes=[1-9][0-9]*$ ]]
observe primary-storage-close "$terminal"
observe durable-release "$(released_summary "$sandbox_id" "$sandbox_generation" "$export_generation")"
sudo test ! -e "$image_path"
observe clean-after-release "$(wait_counts 'rootfs_records=0 live_exports=0')"

sudo systemctl restart mkruntimed
for _ in $(seq 1 60); do systemctl is-active --quiet mkruntimed && break; sleep 1; done
[[ $(systemctl is-active mkruntimed) = active ]]
[[ $(systemctl show -p NRestarts --value mkruntimed) = 0 ]]
sudo "$kerf" show --verbose | grep -Fq 'No memory pool configured'
observe idle-pool-returned "pool_configured=0 mkruntimed=active mkruntimed_restarts=0"
for service in mkruntimed mknetd containerd docker; do
	[[ $(systemctl is-active "$service") = active ]]
	[[ $(systemctl show -p NRestarts --value "$service") = 0 ]]
done

trap - EXIT
rm -rf -- "$scratch"
echo G4_STORAGE_TEARDOWN_LIVE_PASS
