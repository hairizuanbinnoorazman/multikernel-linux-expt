#!/usr/bin/env bash
set -euo pipefail

if [[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]]; then
	PS4='+${BASH_SOURCE}:${LINENO}: '
	set -x
fi

task_id=mk-storage-client-loss
image=${MK_TEST_IMAGE:-docker.io/library/busybox:1.36}
runtime=${MK_RUNTIME:-io.containerd.multikernel.v2}
kerf=${MK_KERF:-/opt/mkruntime/kerf-venv/bin/kerf}
faulted=0

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

cleanup() {
	if [[ $faulted = 0 ]]; then
		cleanup_task
	fi
}
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
		[[ $observed = "$expected" ]] && {
			printf '%s' "$observed"
			return 0
		}
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
  'server_pid':record['pid'],
  'server_start_time':record['start_time'],
},sort_keys=True,separators=(',',':')))
PY
}

durable_summary() {
	sudo python3 - "$1" "$2" <<'PY'
import json,sys
sandbox_id,generation=sys.argv[1:]
key=sandbox_id+'\0'+generation
storage=json.load(open('/var/lib/mkruntimed/storage/state.json'))
value=storage['exports'][key]
print(json.dumps({
  'state':value['state'],
  'sandbox_id':value['sandbox_id'],
  'sandbox_generation':value['sandbox_generation'],
  'export_generation':value['export_generation'],
  'path':value['path'],
  'offline_check':value.get('offline_check',''),
  'counters':value['counters'],
},sort_keys=True,separators=(',',':')))
PY
}

[[ $(id -u) -ne 0 ]] || {
	echo 'run as an ordinary sudo-capable user' >&2
	exit 1
}
for service in mkruntimed mknetd containerd docker; do
	[[ $(systemctl is-active "$service") = active ]]
	[[ $(systemctl show -p NRestarts --value "$service") = 0 ]]
done
sudo test -S /run/mkruntimed.sock
cleanup_task
observe clean-before "$(wait_counts 'rootfs_records=0 live_exports=0')"
host_boot=$(cat /proc/sys/kernel/random/boot_id)
selector=$(readlink -f /usr/local/lib/multikernel/current)
daemon_pid=$(systemctl show -p MainPID --value mkruntimed)
daemon_hash=$(sudo sha256sum "/proc/$daemon_pid/exe" | awk '{print $1}')
observe provenance "boot_id=$host_boot selector=$selector mkruntimed_pid=$daemon_pid mkruntimed_sha256=$daemon_hash qualifier_sha256=$(sha256sum "$0" | awk '{print $1}')"

sudo ctr images pull "$image" >/dev/null
guest_program='set -eu; dd if=/dev/zero of=/tmp/client-loss-read-seed bs=1048576 count=64 conv=fsync status=none; touch /tmp/client-loss-ready; while :; do dd if=/tmp/client-loss-read-seed of=/dev/null bs=4096 status=none; done & while :; do dd if=/dev/zero of=/tmp/client-loss-write-target bs=4096 count=16384 conv=fsync status=none; done & while :; do sync; done & wait'
sudo ctr run --detach --runtime "$runtime" "$image" "$task_id" /bin/sh -c "$guest_program"
wait_task_state RUNNING
for _ in $(seq 1 240); do
	sudo ctr task exec --exec-id "client-loss-ready-$RANDOM" "$task_id" /bin/test -e /tmp/client-loss-ready >/dev/null 2>&1 && break
	sleep .25
done
sudo ctr task exec --exec-id client-loss-ready-final "$task_id" /bin/test -e /tmp/client-loss-ready
wait_counts 'rootfs_records=1 live_exports=1' >/dev/null
summary=$(active_summary)
observe active-generation "$summary"
sandbox_id=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["sandbox_id"])' "$summary")
sandbox_generation=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["sandbox_generation"])' "$summary")
image_path=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["image_path"])' "$summary")
record_path=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["record_path"])' "$summary")
log_path=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["log_path"])' "$summary")
server_pid=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["server_pid"])' "$summary")

sudo grep -Fq "MKNBD_SERVER_READY image=/proc/self/fd/3 image_id=" "$log_path"
sudo grep -Fxq 'MKNBD_SERVER_CLIENT_ACCEPTED' "$log_path"
if sudo grep -Fq 'MKNBD_SERVER_CLOSED' "$log_path"; then
	echo 'storage server closed before fault injection' >&2
	exit 1
fi
guest_processes=$(sudo ctr task exec --exec-id client-loss-processes "$task_id" /bin/ps)
grep -Fq 'client-loss-read-seed' <<<"$guest_processes"
grep -Fq 'client-loss-write-target' <<<"$guest_processes"
server_io=$(sudo cat "/proc/$server_pid/io")
record_hash_before=$(sudo sha256sum "$record_path" | awk '{print $1}')
log_hash_before=$(sudo sha256sum "$log_path" | awk '{print $1}')
state_hash_before=$(sudo sha256sum /var/lib/mkruntimed/storage/state.json | awk '{print $1}')
durable_before=$(durable_summary "$sandbox_id" "$sandbox_generation")
observe outstanding-io "guest_processes=$guest_processes
server_proc_io=$server_io
record_sha256=$record_hash_before log_sha256=$log_hash_before storage_state_sha256=$state_hash_before
durable=$durable_before"

faulted=1
sudo kill -KILL "$server_pid"
for _ in $(seq 1 120); do
	[[ ! -e /proc/$server_pid ]] && break
	sleep .1
done
sudo test ! -e "/proc/$server_pid"
sudo test -f "$record_path"
sudo grep -Fxq 'MKNBD_SERVER_CLIENT_ACCEPTED' "$log_path"
if sudo grep -Fq 'MKNBD_SERVER_CLOSED' "$log_path"; then
	echo 'killed storage server emitted false close evidence' >&2
	exit 1
fi
record_hash_after_kill=$(sudo sha256sum "$record_path" | awk '{print $1}')
[[ $record_hash_after_kill = "$record_hash_before" ]]
observe killed-server "server_pid=$server_pid absent=1 record_retained=1 record_sha256=$record_hash_after_kill terminal_close=0"

set +e
guest_probe=$(sudo timeout 10 ctr task exec --exec-id client-loss-after-kill "$task_id" /bin/sh -c 'dd if=/tmp/client-loss-read-seed of=/dev/null bs=4096 count=256 status=none; sync' 2>&1)
guest_probe_rc=$?
set -e
observe guest-after-server-loss "exit_status=$guest_probe_rc output=$guest_probe"

restart_since=$(date -u '+%Y-%m-%d %H:%M:%S UTC')
sudo systemctl stop mkruntimed
set +e
start_output=$(sudo timeout 30 systemctl start mkruntimed 2>&1)
start_rc=$?
set -e
sleep 1
if systemctl is-active --quiet mkruntimed; then
	echo 'mkruntimed remained active after accepted-client server loss' >&2
	exit 1
fi
sudo systemctl stop mkruntimed >/dev/null 2>&1 || true
journal=$(sudo journalctl -u mkruntimed --since "$restart_since" --no-pager -n 120)
grep -Fq 'active storage server disappeared after client acceptance; automatic session recovery is unsafe' <<<"$journal"
sudo test -f "$record_path"
[[ $(sudo sha256sum "$record_path" | awk '{print $1}') = "$record_hash_before" ]]
[[ $(sudo find /proc -maxdepth 2 -path '*/comm' -type f -exec grep -lFx mkvsock-nbd '{}' + 2>/dev/null | wc -l) = 0 ]]
durable_after=$(durable_summary "$sandbox_id" "$sandbox_generation")
[[ $durable_after = "$durable_before" ]]
sudo test -f "$image_path"
observe fail-closed-reconcile "systemctl_start_exit=$start_rc output=$start_output
record_retained=1 replacement_servers=0 durable_unchanged=1
durable=$durable_after
journal=$journal"
observe retained-kernel-state "$(sudo "$kerf" show --verbose)"

trap - EXIT
echo G4_STORAGE_ACCEPTED_CLIENT_LOSS_PASS
