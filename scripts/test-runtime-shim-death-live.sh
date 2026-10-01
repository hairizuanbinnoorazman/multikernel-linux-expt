#!/usr/bin/env bash
set -euo pipefail

if [[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]]; then
	PS4='+${BASH_SOURCE}:${LINENO}: '
	set -x
fi

# Disposable-host proof for both G6 forced-shim outcomes: supervised worker
# reconstruction and bounded fallback reclaim after the supervisor is killed.
image=${MK_TEST_IMAGE:-docker.io/library/busybox:1.36}
runtime=${MK_RUNTIME:-io.containerd.multikernel.v2}
kerf=${MK_KERF:-/opt/mkruntime/kerf-venv/bin/kerf}
case_mode=${MK_SHIM_DEATH_CASE:-matrix}
reconnect_id=mk-shim-reconnect
reclaim_id=mk-shim-reclaim
scratch=$(mktemp -d)
events=$scratch/reconnect-events.log
stream=$scratch/reconnect-stream.log
event_pid=
attach_pid=

stop_event_reader() {
	if [[ -n ${event_pid:-} ]]; then
		kill "$event_pid" >/dev/null 2>&1 || true
		wait "$event_pid" >/dev/null 2>&1 || true
		event_pid=
	fi
}

stop_attach_reader() {
	if [[ -n ${attach_pid:-} ]]; then
		kill "$attach_pid" >/dev/null 2>&1 || true
		wait "$attach_pid" >/dev/null 2>&1 || true
		attach_pid=
	fi
}

cleanup_id() {
	local id=$1
	sudo ctr tasks kill --signal SIGKILL "$id" >/dev/null 2>&1 || true
	for _ in $(seq 1 120); do
		state=$(sudo ctr tasks list | awk -v id="$id" '$1 == id {print $3}')
		[[ $state != RUNNING && $state != PAUSED ]] && break
		sleep .25
	done
	sudo ctr tasks rm -f "$id" >/dev/null 2>&1 || true
	sudo ctr containers rm "$id" >/dev/null 2>&1 || true
}

cleanup() {
	set +e
	stop_event_reader
	stop_attach_reader
	cleanup_id "$reconnect_id"
	cleanup_id "$reclaim_id"
	rm -rf -- "$scratch"
}
trap cleanup EXIT

observe() {
	local key=$1 value=$2
	printf 'OBSERVATION_BEGIN key=%s\n%s\nOBSERVATION_END key=%s\n' "$key" "$value" "$key"
}

pool_configured() {
	local state
	state=$(sudo "$kerf" show)
	if grep -Fq 'No memory pool configured' <<<"$state"; then
		printf 0
	else
		printf 1
	fi
}

inventory() {
	printf 'pool_configured=%s children=%s links=%s nat_rules=%s filter_rules=%s ctr_tasks=%s ctr_containers=%s moby_tasks=%s moby_containers=%s docker_containers=%s runtime_artifacts=%s rootfs_records=%s endpoints=%s shim_processes=%s helper_processes=%s' \
		"$(pool_configured)" \
		"$(sudo find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 -type d | wc -l)" \
		"$(ip -o link show | awk -F': ' '$2 ~ /^mkv[0-9a-f]+$/ {count++} END {print count+0}')" \
		"$(sudo iptables -t nat -S POSTROUTING | grep -c '172\.31\.' || true)" \
		"$(sudo iptables -S | grep -c '^\(-N\|-A\) MK-' || true)" \
		"$(sudo ctr tasks list -q | wc -l)" \
		"$(sudo ctr containers list -q | wc -l)" \
		"$(sudo ctr -n moby tasks list -q | wc -l)" \
		"$(sudo ctr -n moby containers list -q | wc -l)" \
		"$(sudo docker ps -aq | wc -l)" \
		"$(sudo find /srv/multikernel-storage/runtime -mindepth 1 -print | wc -l)" \
		"$(sudo python3 -c 'import json; print(len(json.load(open("/var/lib/mkruntimed/rootfs/state.json"))["records"]))')" \
		"$(sudo python3 -c 'import json; print(len(json.load(open("/var/lib/mknetd/state.json"))["endpoints"]))')" \
		"$( (pgrep -f '^/usr/local/lib/multikernel/.*/containerd-shim-multikernel-v2' || true) | wc -l)" \
		"$( (pgrep -f '^/usr/local/libexec/multikernel/(mkvsock-nbd|mk-agent-relay)' || true) | wc -l)"
}

wait_inventory() {
	local expected=$1 observed=
	for _ in $(seq 1 480); do
		observed=$(inventory)
		[[ $observed = "$expected" ]] && {
			printf '%s' "$observed"
			return 0
		}
		sleep .25
	done
	printf '%s' "$observed"
	return 1
}

task_holder_pid() {
	local id=$1
	sudo ctr tasks list | awk -v id="$id" '$1 == id {print $2}'
}

worker_pidfile() {
	local supervisor=$1
	sudo cat "/proc/$supervisor/cwd/.multikernel-worker.pid"
}

parent_pid() {
	local pid=$1
	sudo awk '{print $4}' "/proc/$pid/stat"
}

assert_process_roles() {
	local holder=$1 worker=$2 supervisor=$3
	[[ -n $holder && -n $worker && -n $supervisor ]]
	[[ $holder != "$worker" && $worker != "$supervisor" && $holder != "$supervisor" ]]
	[[ $(parent_pid "$holder") = "$worker" ]]
	[[ $(parent_pid "$worker") = "$supervisor" ]]
	[[ $(worker_pidfile "$supervisor") = "$worker" ]]
}

recovery_summary() {
	local supervisor=$1
	sudo python3 - "/proc/$supervisor/cwd/.multikernel/sandbox.json" <<'PY'
import json
import os
import sys

path = sys.argv[1]
value = json.load(open(path, encoding="utf-8"))
network = value.get("network", {})
summary = {
    "schema_version": value.get("schema_version"),
    "id": value.get("id"),
    "generation": value.get("generation"),
    "task_identity": value.get("task_identity"),
    "bundle_identity": value.get("bundle_identity"),
    "network": {
        key: network.get(key)
        for key in ("sandbox_id", "sandbox_generation", "generation", "address", "host_address", "mtu")
    },
    "processes": [
        {
            key: process.get(key)
            for key in ("id", "status", "pid", "terminal", "stdin_offset", "stdout_offset", "stderr_offset")
        }
        for process in value.get("processes", [])
    ],
}
stat = os.stat(path)
summary["file"] = {"mode": oct(stat.st_mode & 0o777), "uid": stat.st_uid, "links": stat.st_nlink}
print(json.dumps(summary, sort_keys=True, separators=(",", ":")))
PY
}

filtered_containerd_journal() {
	local since=$1 id=$2
	sudo journalctl -u containerd --since "$since" --no-pager -o short-iso-precise |
		grep -E "($id|multikernel shim supervisor|shim disconnected|cleanup)" || true
}

clean_resources='children=0 links=0 nat_rules=0 filter_rules=0 ctr_tasks=0 ctr_containers=0 moby_tasks=0 moby_containers=0 docker_containers=0 runtime_artifacts=0 rootfs_records=0 endpoints=0 shim_processes=0 helper_processes=0'
clean_released="pool_configured=0 $clean_resources"
clean_retained="pool_configured=1 $clean_resources"

[[ $(id -u) -ne 0 ]] || {
	echo 'run as an ordinary sudo-capable user' >&2
	exit 1
}
case "$case_mode" in
	matrix | reconnect | reclaim) ;;
	*)
		echo "invalid MK_SHIM_DEATH_CASE: $case_mode" >&2
		exit 2
		;;
esac
for service in mkruntimed mknetd containerd docker; do
	[[ $(systemctl is-active "$service") = active ]]
done
cleanup_id "$reconnect_id"
cleanup_id "$reclaim_id"
initial=$(wait_inventory "$clean_released")
observe initial-inventory "$initial"
observe host "case_mode=$case_mode boot_id=$(cat /proc/sys/kernel/random/boot_id) kernel=$(uname -r) containerd_pid=$(systemctl show -p MainPID --value containerd) mkruntimed_pid=$(systemctl show -p MainPID --value mkruntimed)"
sudo ctr images pull "$image" >/dev/null

# Case 1: only the serving worker dies. The supervisor retains the Task v2
# listener and must reconstruct from authenticated durable state.
if [[ $case_mode != reclaim ]]; then
sudo timeout 900 stdbuf -oL ctr events >"$events" 2>&1 &
event_pid=$!
sleep 1
sudo ctr run --detach --runtime "$runtime" "$image" "$reconnect_id" /bin/sh -c \
	'while [ ! -e /tmp/stream-ready ]; do sleep 1; done; echo reconnect-init-before; echo reconnect-init-before-err >&2; while [ ! -e /tmp/recovery-release ]; do sleep 1; done; echo reconnect-init-after; echo reconnect-init-after-err >&2'
for _ in $(seq 1 480); do
	reconnect_state=$(sudo ctr tasks list | awk -v id="$reconnect_id" '$1 == id {print $3}')
	[[ $reconnect_state = RUNNING ]] && break
	sleep .25
done
[[ $reconnect_state = RUNNING ]]
sudo timeout 600 ctr tasks attach "$reconnect_id" >"$stream" 2>&1 &
attach_pid=$!
sleep 1
sudo ctr task exec --exec-id shim-stream-ready "$reconnect_id" /bin/touch /tmp/stream-ready
for _ in $(seq 1 120); do
	if grep -Fxq reconnect-init-before "$stream" && grep -Fxq reconnect-init-before-err "$stream"; then
		break
	fi
	sleep .25
done
grep -Fxq reconnect-init-before "$stream"
grep -Fxq reconnect-init-before-err "$stream"
reconnect_holder_before=$(task_holder_pid "$reconnect_id")
reconnect_worker_before=$(parent_pid "$reconnect_holder_before")
reconnect_supervisor=$(parent_pid "$reconnect_worker_before")
assert_process_roles "$reconnect_holder_before" "$reconnect_worker_before" "$reconnect_supervisor"
reconnect_boot_before=$(sudo ctr task exec --exec-id shim-boot-before "$reconnect_id" /bin/cat /proc/sys/kernel/random/boot_id)
reconnect_exec_before=$(sudo ctr task exec --exec-id shim-exec-before "$reconnect_id" /bin/sh -c \
	'echo reconnect-exec-before; echo reconnect-exec-before-err >&2' 2>&1)
reconnect_recovery_before=$(recovery_summary "$reconnect_supervisor")
for marker in reconnect-exec-before reconnect-exec-before-err; do
	grep -Fxq "$marker" <<<"$reconnect_exec_before"
done
observe reconnect-before "supervisor_pid=$reconnect_supervisor worker_pid=$reconnect_worker_before namespace_holder_pid=$reconnect_holder_before state=$reconnect_state child_boot=$reconnect_boot_before exec=$reconnect_exec_before recovery=$reconnect_recovery_before"

reconnect_fault_started=$(date -u '+%Y-%m-%d %H:%M:%S UTC')
sudo kill -KILL "$reconnect_worker_before"
reconnect_worker_after=
reconnect_holder_after=
[[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]] && set +x
for _ in $(seq 1 480); do
	reconnect_worker_after=$(worker_pidfile "$reconnect_supervisor" 2>/dev/null || true)
	reconnect_holder_after=$(task_holder_pid "$reconnect_id")
	reconnect_state=$(sudo ctr tasks list | awk -v id="$reconnect_id" '$1 == id {print $3}')
	if [[ $reconnect_state = RUNNING && -n $reconnect_worker_after && -n $reconnect_holder_after ]] &&
		[[ $reconnect_worker_after != "$reconnect_worker_before" && $reconnect_holder_after != "$reconnect_holder_before" ]] &&
		[[ -d /proc/$reconnect_worker_after && -d /proc/$reconnect_holder_after ]] &&
		[[ $(parent_pid "$reconnect_worker_after" 2>/dev/null || true) = "$reconnect_supervisor" ]] &&
		[[ $(parent_pid "$reconnect_holder_after" 2>/dev/null || true) = "$reconnect_worker_after" ]]; then
		break
	fi
	sleep .25
done
[[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]] && set -x
[[ $reconnect_state = RUNNING ]]
[[ -n $reconnect_worker_after && $reconnect_worker_after != "$reconnect_worker_before" ]]
[[ -n $reconnect_holder_after && $reconnect_holder_after != "$reconnect_holder_before" ]]
[[ ! -d /proc/$reconnect_worker_before ]]
[[ ! -d /proc/$reconnect_holder_before ]]
assert_process_roles "$reconnect_holder_after" "$reconnect_worker_after" "$reconnect_supervisor"
reconnect_boot_after=$(sudo ctr task exec --exec-id shim-boot-after "$reconnect_id" /bin/cat /proc/sys/kernel/random/boot_id)
reconnect_exec_after=$(sudo ctr task exec --exec-id shim-exec-after "$reconnect_id" /bin/sh -c \
	'echo reconnect-exec-after; echo reconnect-exec-after-err >&2' 2>&1)
reconnect_recovery_after=$(recovery_summary "$reconnect_supervisor")
[[ $reconnect_boot_after = "$reconnect_boot_before" ]]
for marker in reconnect-exec-after reconnect-exec-after-err; do
	grep -Fxq "$marker" <<<"$reconnect_exec_after"
done
observe reconnect-after "supervisor_pid=$reconnect_supervisor old_worker_pid=$reconnect_worker_before replacement_worker_pid=$reconnect_worker_after old_namespace_holder_pid=$reconnect_holder_before replacement_namespace_holder_pid=$reconnect_holder_after state=$reconnect_state child_boot=$reconnect_boot_after exec=$reconnect_exec_after recovery=$reconnect_recovery_after"
observe reconnect-journal "$(filtered_containerd_journal "$reconnect_fault_started" "$reconnect_id")"

sudo ctr task exec --exec-id shim-release "$reconnect_id" /bin/touch /tmp/recovery-release
wait "$attach_pid"
attach_pid=
reconnect_stream=$(<"$stream")
for marker in reconnect-init-before reconnect-init-before-err reconnect-init-after reconnect-init-after-err; do
	grep -Fxq "$marker" <<<"$reconnect_stream"
done
observe reconnect-stream "$reconnect_stream"
sudo ctr tasks rm -f "$reconnect_id" >/dev/null 2>&1 || true
sudo ctr containers rm "$reconnect_id"
sleep 2
stop_event_reader
grep -Fq "$reconnect_id" "$events"
for topic in /tasks/create /tasks/start /tasks/exit /tasks/delete; do
	grep -Fq "$topic" "$events"
done
observe reconnect-events "$(grep -F "$reconnect_id" "$events")"
reconnect_clean=$(wait_inventory "$clean_retained")
observe reconnect-final-inventory "$reconnect_clean"
printf 'G6_FORCED_SHIM_RECONNECT_PASS\n'
fi

# Case 2: killing the supervisor also kills its worker by parent-death policy,
# deliberately removing the only in-place reconstruction owner. Containerd's
# cleanup-only path must safely reclaim the durable resources.
if [[ $case_mode != reconnect ]]; then
sudo ctr run --detach --runtime "$runtime" "$image" "$reclaim_id" /bin/sleep 300
for _ in $(seq 1 480); do
	reclaim_state=$(sudo ctr tasks list | awk -v id="$reclaim_id" '$1 == id {print $3}')
	[[ $reclaim_state = RUNNING ]] && break
	sleep .25
done
[[ $reclaim_state = RUNNING ]]
reclaim_holder=$(task_holder_pid "$reclaim_id")
reclaim_worker=$(parent_pid "$reclaim_holder")
reclaim_supervisor=$(parent_pid "$reclaim_worker")
assert_process_roles "$reclaim_holder" "$reclaim_worker" "$reclaim_supervisor"
reclaim_boot=$(sudo ctr task exec --exec-id reclaim-boot "$reclaim_id" /bin/cat /proc/sys/kernel/random/boot_id)
reclaim_recovery=$(recovery_summary "$reclaim_supervisor")
observe reclaim-before "supervisor_pid=$reclaim_supervisor worker_pid=$reclaim_worker namespace_holder_pid=$reclaim_holder state=$reclaim_state child_boot=$reclaim_boot recovery=$reclaim_recovery"

reclaim_fault_started=$(date -u '+%Y-%m-%d %H:%M:%S UTC')
sudo kill -KILL "$reclaim_supervisor"
[[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]] && set +x
for _ in $(seq 1 720); do
	children=$(sudo find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 -type d | wc -l)
	artifacts=$(sudo find /srv/multikernel-storage/runtime -mindepth 1 -print | wc -l)
	roots=$(sudo python3 -c 'import json; print(len(json.load(open("/var/lib/mkruntimed/rootfs/state.json"))["records"]))')
	endpoints=$(sudo python3 -c 'import json; print(len(json.load(open("/var/lib/mknetd/state.json"))["endpoints"]))')
	shims=$( (pgrep -f '^/usr/local/lib/multikernel/.*/containerd-shim-multikernel-v2' || true) | wc -l)
	helpers=$( (pgrep -f '^/usr/local/libexec/multikernel/(mkvsock-nbd|mk-agent-relay)' || true) | wc -l)
	if [[ $children = 0 && $artifacts = 0 && $roots = 0 && $endpoints = 0 && $shims = 0 && $helpers = 0 ]]; then
		break
	fi
	sleep .25
done
[[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]] && set -x
[[ ! -d /proc/$reclaim_supervisor ]]
[[ ! -d /proc/$reclaim_worker ]]
[[ ! -d /proc/$reclaim_holder ]]
[[ $children = 0 && $artifacts = 0 && $roots = 0 && $endpoints = 0 && $shims = 0 && $helpers = 0 ]]
set +e
reclaim_exec_error=$(sudo timeout 30 ctr task exec --exec-id reclaim-after "$reclaim_id" /bin/true 2>&1)
reclaim_exec_status=$?
set -e
[[ $reclaim_exec_status -ne 0 ]]
reclaim_task_state=$(sudo ctr tasks list | awk -v id="$reclaim_id" '$1 == id {print $3}')
observe reclaim-after "supervisor_dead=true worker_dead=true namespace_holder_dead=true task_state=${reclaim_task_state:-ABSENT} exec_status=$reclaim_exec_status exec_error=$reclaim_exec_error children=$children artifacts=$artifacts rootfs_records=$roots endpoints=$endpoints shims=$shims helpers=$helpers"
observe reclaim-journal "$(filtered_containerd_journal "$reclaim_fault_started" "$reclaim_id")"
sudo ctr tasks rm -f "$reclaim_id" >/dev/null 2>&1 || true
sudo ctr containers rm "$reclaim_id" >/dev/null 2>&1 || true
reclaim_clean=$(wait_inventory "$clean_retained")
observe reclaim-final-inventory "$reclaim_clean"
printf 'G6_FORCED_SHIM_RECLAIM_PASS\n'
fi

idle_mkruntimed_pid=$(systemctl show -p MainPID --value mkruntimed)
sudo systemctl restart mkruntimed
[[ $(systemctl is-active mkruntimed) = active ]]
released_mkruntimed_pid=$(systemctl show -p MainPID --value mkruntimed)
[[ $released_mkruntimed_pid != "$idle_mkruntimed_pid" ]]
released=$(wait_inventory "$clean_released")
observe final-released-inventory "$released"
if [[ $case_mode = matrix ]]; then
	printf 'G6_FORCED_SHIM_DEATH_MATRIX_PASS\n'
else
	printf 'G6_FORCED_SHIM_DEATH_%s_MODE_PASS\n' "${case_mode^^}"
fi
