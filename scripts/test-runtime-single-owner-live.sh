#!/usr/bin/env bash
set -euo pipefail

if [[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]]; then
	PS4='+${BASH_SOURCE}:${LINENO}: '
	set -x
fi

source_root=${1:?usage: test-runtime-single-owner-live.sh SOURCE_ROOT}
image=${MK_TEST_IMAGE:-docker.io/library/busybox:1.36}
runtime=${MK_RUNTIME:-io.containerd.multikernel.v2}
task_id=mk-single-owner
scratch=$(mktemp -d -p /var/tmp mk-single-owner.XXXXXX)
lock_ready=$scratch/lock-ready
lock_release=$scratch/lock-release
lock_result=$scratch/lock-result
locker_pid=

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
	cleanup_task
	touch "$lock_release"
	[[ -z $locker_pid ]] || wait "$locker_pid" >/dev/null 2>&1 || true
	sudo rm -rf -- "$scratch"
)
trap cleanup EXIT

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

wait_task_state() {
	local expected=$1 state=
	for _ in $(seq 1 240); do
		state=$(sudo ctr tasks list | awk -v id="$task_id" '$1 == id {print $3}')
		[[ $state = "$expected" ]] && return 0
		sleep .25
	done
	return 1
}

state_summary() {
	sudo python3 - <<'PY'
import json, os, re
rootfs=json.load(open('/var/lib/mkruntimed/rootfs/state.json'))
storage=json.load(open('/var/lib/mkruntimed/storage/state.json'))
assert rootfs['version'] == 4 and len(rootfs['records']) == 1, rootfs
assert storage['version'] == 2
live=[(k,v) for k,v in storage['exports'].items() if v['state'] != 'RELEASED']
assert len(live) == 1, live
root_key, root=next(iter(rootfs['records'].items()))
export_key, export=live[0]
assert root_key == root['request']['task_identity']
assert root['phase'] == 'PREPARED'
assert root['storage']['path'] == export['path']
assert root['storage']['port'] == export['port']
assert root['storage']['image_id'] == export['image_id']
assert root['storage']['filesystem_uuid'] == export['filesystem_uuid']
assert export['state'] == 'ACTIVE'
for key in ('sandbox_generation','export_generation'):
    assert re.fullmatch(r'[a-f0-9]{32}', export[key]), (key, export[key])
assert export['sandbox_generation'] != export['export_generation']
runtime_dir='/run/mkstorage'
records=[x for x in os.listdir(runtime_dir) if x.endswith('.json')]
assert len(records) == 1, records
record_path=os.path.join(runtime_dir, records[0])
record=json.load(open(record_path))
assert record['path'] == export['path']
assert record['port'] == export['port']
assert record['image_id'] == export['image_id']
assert record['export_generation'] == export['export_generation']
pid=record['pid']
with open('/proc/%d/comm' % pid) as f:
    assert f.read().strip() == 'mkvsock-nbd'
image_stat=os.stat(export['path'])
fd_stat=os.stat('/proc/%d/fd/3' % pid)
assert (image_stat.st_dev,image_stat.st_ino) == (fd_stat.st_dev,fd_stat.st_ino)
assert (record['image_device'],record['image_inode']) == (image_stat.st_dev,image_stat.st_ino)
summary={
 'rootfs_version':rootfs['version'], 'rootfs_record_key':root_key,
 'rootfs_phase':root['phase'], 'bundle':root['request']['bundle'],
 'storage_state_version':storage['version'], 'historical_export_count':len(storage['exports']),
 'live_export_key':export_key, 'sandbox_id':export['sandbox_id'],
 'sandbox_generation':export['sandbox_generation'], 'export_generation':export['export_generation'],
 'state':export['state'], 'path':export['path'], 'port':export['port'],
 'image_id':export['image_id'], 'filesystem_uuid':export['filesystem_uuid'],
 'image_identity':export['image_identity'], 'process_record':record_path,
 'process_pid':pid, 'process_start_time':record['start_time'],
 'fd3_identity':'%d:%d' % (fd_stat.st_dev,fd_stat.st_ino),
}
print(json.dumps(summary,sort_keys=True,separators=(',',':')))
PY
}

released_summary() {
	sudo python3 - "$1" "$2" "$3" <<'PY'
import json,re,sys
sandbox_id,sandbox_generation,generation=sys.argv[1:]
key=sandbox_id+'\0'+sandbox_generation
rootfs=json.load(open('/var/lib/mkruntimed/rootfs/state.json'))
storage=json.load(open('/var/lib/mkruntimed/storage/state.json'))
assert rootfs['version'] == 4 and not rootfs['records']
assert storage['version'] == 2
assert sum(v['state'] != 'RELEASED' for v in storage['exports'].values()) == 0
value=storage['exports'][key]
assert value['state'] == 'RELEASED'
assert value['export_generation'] == generation
assert re.fullmatch(r'e2fsck-clean-sha256:[a-f0-9]{64}',value['offline_check'])
print(json.dumps({
 'rootfs_records':0, 'live_exports':0, 'historical_export_count':len(storage['exports']),
 'exact_export_key':key, 'sandbox_generation':value['sandbox_generation'],
 'export_generation':value['export_generation'], 'state':value['state'],
 'offline_check':value['offline_check'], 'counters':value['counters'],
},sort_keys=True,separators=(',',':')))
PY
}

[[ $(id -u) -ne 0 ]] || { echo 'run as an ordinary sudo-capable user' >&2; exit 1; }
test -d "$source_root/runtime"
for service in mkruntimed mknetd containerd docker; do
	[[ $(systemctl is-active "$service") = active ]]
	[[ $(systemctl show -p NRestarts --value "$service") = 0 ]]
done
cleanup_task
observe clean-before "$(wait_counts 'rootfs_records=0 live_exports=0')"
observe provenance "boot_id=$(cat /proc/sys/kernel/random/boot_id)
selector=$(readlink -f /usr/local/lib/multikernel/current)
mkruntimed_pid=$(systemctl show -p MainPID --value mkruntimed)
mkruntimed_sha256=$(sudo sha256sum /proc/$(systemctl show -p MainPID --value mkruntimed)/exe | awk '{print $1}')
qualifier_sha256=$(sha256sum "$source_root/scripts/test-runtime-single-owner-live.sh" | awk '{print $1}')"

rootfs_pattern='^TestRootfsStoreRejectsDuplicateLiveClaims$'
service_pattern='^(TestProvisionIsGenerationBoundIdempotentAndSingleOwner|TestReleaseRequiresExactGenerationAndOfflineCheck|TestProvisionAndReleaseFailuresRemainFailClosed|TestReconcileRestartsRetainedPreparationWithExactGeneration|TestRetainedPreparationRefusesConflictingLiveGeneration|TestReconcileRestartsOnlyAbsentExactActiveExport)$'
store_pattern='^(TestStoreRejectsForgedSemanticStateBeforeReconciliation|TestStoreEnforcesStorageStateTransitions|TestStoreRejectsSymlinkAndUnknownOrDuplicateState)$'
backend_pattern='^(TestLinuxBackendProcessIdentityGracefulStopAndOfflineCheck|TestStorageStartProtectsExistingArtifactsAndCleansOwnFailures|TestOfflineCheckIsBoundedAndHashesCombinedEvidence|TestMissingEphemeralRuntimeDirectoryObservesExportAbsent)$'
(
	cd "$source_root/runtime"
	GOCACHE="$scratch/go-cache" go test -race -v -count=1 ./internal/rootfs -run "$rootfs_pattern"
	GOCACHE="$scratch/go-cache" go test -race -v -count=1 ./internal/storage -run "$service_pattern|$store_pattern|$backend_pattern"
)
echo FOCUSED_SINGLE_OWNER_GENERATION_LOCK_TESTS_PASS

guest_program='set -eu; touch /tmp/single-owner-ready; while [ ! -e /tmp/single-owner-release ]; do sleep 1; done'
sudo ctr run --detach --runtime "$runtime" "$image" "$task_id" /bin/sh -c "$guest_program"
wait_task_state RUNNING
for _ in $(seq 1 120); do
	sudo ctr task exec --exec-id "single-owner-ready-$RANDOM" "$task_id" /bin/test -e /tmp/single-owner-ready >/dev/null 2>&1 && break
	sleep .25
done
sudo ctr task exec --exec-id single-owner-ready-final "$task_id" /bin/test -e /tmp/single-owner-ready
wait_counts 'rootfs_records=1 live_exports=1' >/dev/null
live_summary=$(state_summary)
observe durable-live-owner "$live_summary"
image_path=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["path"])' "$live_summary")
sandbox_id=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["sandbox_id"])' "$live_summary")
sandbox_generation=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["sandbox_generation"])' "$live_summary")
export_generation=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["export_generation"])' "$live_summary")

sudo bash -c '
set -eu
image=$1; ready=$2; release=$3; result=$4
exec 9<>"$image"
if flock -n 9; then
	printf "unexpected-second-owner\n" >"$result"
	exit 1
fi
printf "live_owner_lock=contended held_identity=%s\n" "$(stat -Lc "%d:%i" /proc/self/fd/9)" >"$result"
touch "$ready"
while [ ! -e "$release" ]; do sleep .05; done
flock -n 9
printf "post_teardown_lock=acquired held_identity=%s links=%s\n" "$(stat -Lc "%d:%i" /proc/self/fd/9)" "$(stat -Lc "%h" /proc/self/fd/9)" >>"$result"
' bash "$image_path" "$lock_ready" "$lock_release" "$lock_result" &
locker_pid=$!
for _ in $(seq 1 120); do [[ -e $lock_ready ]] && break; sleep .05; done
[[ -e $lock_ready ]]
grep -Fxq "live_owner_lock=contended held_identity=$(sudo stat -Lc '%d:%i' "$image_path")" "$lock_result"
observe live-duplicate-attach-rejected "$(sudo cat "$lock_result")"

sudo ctr task exec --exec-id single-owner-release "$task_id" /bin/touch /tmp/single-owner-release
wait_task_state STOPPED
sudo ctr tasks rm "$task_id" >/dev/null
sudo ctr containers rm "$task_id"
observe durable-released-owner "$(released_summary "$sandbox_id" "$sandbox_generation" "$export_generation")"
[[ ! -e $image_path ]]
touch "$lock_release"
wait "$locker_pid"
locker_pid=
lock_transcript=$(sudo cat "$lock_result")
held_identity=$(sed -n 's/^live_owner_lock=contended held_identity=//p' <<<"$lock_transcript")
grep -Fqx "post_teardown_lock=acquired held_identity=$held_identity links=0" <<<"$lock_transcript"
observe stale-lock-released "$lock_transcript"
observe clean-after "$(wait_counts 'rootfs_records=0 live_exports=0')"
for service in mkruntimed mknetd containerd docker; do
	[[ $(systemctl is-active "$service") = active ]]
	[[ $(systemctl show -p NRestarts --value "$service") = 0 ]]
done

trap - EXIT
sudo rm -rf -- "$scratch"
echo G4_SINGLE_OWNER_LIVE_PASS
