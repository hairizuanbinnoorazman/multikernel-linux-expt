#!/usr/bin/env bash
set -euo pipefail
[[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]] && { PS4='+${BASH_SOURCE}:${LINENO}: '; set -x; }

image=${MK_TEST_IMAGE:-docker.io/library/busybox:1.36}
runtime=${MK_RUNTIME:-io.containerd.multikernel.v2}
kerf=${MK_KERF:-/opt/mkruntime/kerf-venv/bin/kerf}
task_id=mk-clean-shim-restart
scratch=$(mktemp -d)
events=$scratch/events
event_pid=
attach_pid=

stop_reader() { local pid=${1:-}; [[ -z $pid ]] || { kill "$pid" >/dev/null 2>&1 || true; wait "$pid" >/dev/null 2>&1 || true; }; }
cleanup_task() {
	sudo ctr tasks kill --signal SIGKILL "$task_id" >/dev/null 2>&1 || true
	sudo ctr tasks rm -f "$task_id" >/dev/null 2>&1 || true
	sudo ctr containers rm "$task_id" >/dev/null 2>&1 || true
}
cleanup() { set +e; stop_reader "$attach_pid"; stop_reader "$event_pid"; cleanup_task; rm -rf -- "$scratch"; }
trap cleanup EXIT
observe() { local key=$1 value=$2; printf 'OBSERVATION_BEGIN key=%s\n%s\nOBSERVATION_END key=%s\n' "$key" "$value" "$key"; }
pool_configured() { local value; value=$(sudo "$kerf" show); if grep -Fq 'No memory pool configured' <<<"$value"; then printf 0; else printf 1; fi; }
inventory() {
	printf 'pool_configured=%s children=%s ctr_tasks=%s ctr_containers=%s moby_tasks=%s moby_containers=%s docker_containers=%s runtime_artifacts=%s rootfs_records=%s endpoints=%s shim_processes=%s helper_processes=%s' \
		"$(pool_configured)" "$(sudo find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 -type d | wc -l)" \
		"$(sudo ctr tasks list -q | wc -l)" "$(sudo ctr containers list -q | wc -l)" \
		"$(sudo ctr -n moby tasks list -q | wc -l)" "$(sudo ctr -n moby containers list -q | wc -l)" "$(sudo docker ps -aq | wc -l)" \
		"$( (sudo find /srv/multikernel-storage/runtime -mindepth 1 -print 2>/dev/null || true) | wc -l)" \
		"$(sudo python3 -c 'import json; print(len(json.load(open("/var/lib/mkruntimed/rootfs/state.json"))["records"]))')" \
		"$(sudo python3 -c 'import json; print(len(json.load(open("/var/lib/mknetd/state.json"))["endpoints"]))')" \
		"$( (pgrep -f '^/usr/local/lib/multikernel/.*/containerd-shim-multikernel-v2' || true) | wc -l)" \
		"$( (pgrep -f '^/usr/local/libexec/multikernel/(mkvsock-nbd|mk-agent-relay)' || true) | wc -l)"
}
wait_inventory() { local expected=$1 observed=; for _ in $(seq 1 480); do observed=$(inventory); [[ $observed = "$expected" ]] && { printf %s "$observed"; return; }; sleep .25; done; printf %s "$observed"; return 1; }
parent_pid() { sudo awk '{print $4}' "/proc/$1/stat"; }
task_holder_pid() { sudo ctr tasks list | awk -v id="$task_id" '$1 == id {print $2}'; }
wait_task_running() { local state=; for _ in $(seq 1 480); do state=$(sudo ctr tasks list | awk -v id="$task_id" '$1 == id {print $3}'); [[ $state = RUNNING ]] && return; sleep .25; done; return 1; }
wait_process_gone() { local pid=$1; for _ in $(seq 1 240); do [[ ! -d /proc/$pid ]] && return; sleep .25; done; return 1; }

clean='children=0 ctr_tasks=0 ctr_containers=0 moby_tasks=0 moby_containers=0 docker_containers=0 runtime_artifacts=0 rootfs_records=0 endpoints=0 shim_processes=0 helper_processes=0'
clean_released="pool_configured=0 $clean"
clean_retained="pool_configured=1 $clean"
[[ $(id -u) -ne 0 ]] || { echo 'run as an ordinary sudo-capable user' >&2; exit 1; }
for service in mkruntimed mknetd containerd docker; do [[ $(systemctl is-active "$service") = active ]]; done
cleanup_task
observe initial-inventory "$(wait_inventory "$clean_released")"
observe provenance "selector=$(readlink -f /usr/local/lib/multikernel/current) boot_id=$(cat /proc/sys/kernel/random/boot_id) containerd_pid=$(systemctl show -p MainPID --value containerd) mkruntimed_pid=$(systemctl show -p MainPID --value mkruntimed)"
sudo ctr images pull "$image" >/dev/null
sudo timeout 900 stdbuf -oL ctr events >"$events" 2>&1 & event_pid=$!
sleep 1

cycle_supervisor= cycle_worker= cycle_holder= cycle_boot=
run_cycle() {
	local cycle=$1 stream=$scratch/stream-$cycle holder worker supervisor boot state attach_status
	sudo ctr run --detach --runtime "$runtime" "$image" "$task_id" /bin/sh -c "echo clean-$cycle-start; while [ ! -e /tmp/clean-release ]; do sleep 1; done; echo clean-$cycle-exit"
	wait_task_running
	holder=$(task_holder_pid); worker=$(parent_pid "$holder"); supervisor=$(parent_pid "$worker")
	boot=$(sudo ctr task exec --exec-id "clean-boot-$cycle" "$task_id" /bin/cat /proc/sys/kernel/random/boot_id)
	sudo timeout 600 ctr tasks attach "$task_id" >"$stream" 2>&1 & attach_pid=$!
	sleep 1
	sudo ctr task exec --exec-id "clean-release-$cycle" "$task_id" /bin/touch /tmp/clean-release
	set +e; wait "$attach_pid"; attach_status=$?; attach_pid=; set -e
	[[ $attach_status -eq 0 ]]
	grep -Fxq "clean-$cycle-start" "$stream"; grep -Fxq "clean-$cycle-exit" "$stream"
	state=$(sudo ctr tasks list | awk -v id="$task_id" '$1 == id {print $3}')
	if [[ $state = STOPPED ]]; then sudo ctr tasks rm "$task_id" >/dev/null; else [[ -z $state ]]; fi
	sudo ctr containers rm "$task_id"
	wait_process_gone "$holder"; wait_process_gone "$worker"; wait_process_gone "$supervisor"
	observe "cycle-$cycle" "supervisor_pid=$supervisor worker_pid=$worker namespace_holder_pid=$holder child_boot=$boot attach_exit=$attach_status output=$(<"$stream") processes_gone=true"
	cycle_supervisor=$supervisor; cycle_worker=$worker; cycle_holder=$holder; cycle_boot=$boot
}

run_cycle 1
supervisor1=$cycle_supervisor; worker1=$cycle_worker; holder1=$cycle_holder; boot1=$cycle_boot
observe after-cycle-1 "$(wait_inventory "$clean_retained")"
run_cycle 2
supervisor2=$cycle_supervisor; worker2=$cycle_worker; holder2=$cycle_holder; boot2=$cycle_boot
[[ $supervisor2 != "$supervisor1" && $worker2 != "$worker1" && $holder2 != "$holder1" ]]
[[ $boot2 != "$boot1" ]]
observe replacement "old_supervisor=$supervisor1 new_supervisor=$supervisor2 old_worker=$worker1 new_worker=$worker2 old_holder=$holder1 new_holder=$holder2 old_child_boot=$boot1 new_child_boot=$boot2 same_task_name=$task_id"
observe after-cycle-2 "$(wait_inventory "$clean_retained")"
sleep 2; stop_reader "$event_pid"; event_pid=
[[ $(grep -Fc "/tasks/create" "$events") -ge 2 ]]
[[ $(grep -Fc "/tasks/start" "$events") -ge 2 ]]
[[ $(grep -Fc "/tasks/exit" "$events") -ge 2 ]]
[[ $(grep -Fc "/tasks/delete" "$events") -ge 2 ]]
observe task-events "$(grep -F "$task_id" "$events")"
idle_pid=$(systemctl show -p MainPID --value mkruntimed); sudo systemctl restart mkruntimed; released_pid=$(systemctl show -p MainPID --value mkruntimed); [[ $released_pid != "$idle_pid" ]]
observe released-final "$(wait_inventory "$clean_released")"
trap - EXIT; rm -rf -- "$scratch"
echo G6_CLEAN_SHIM_RESTART_PASS
