#!/usr/bin/env bash
set -euo pipefail

if [[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]]; then
	PS4='+${BASH_SOURCE}:${LINENO}: '
	set -x
fi

# Focused disposable-host proof that containerd reconnects to a live
# Multikernel Task v2 shim. Run as an ordinary sudo-capable user on an idle,
# qualified host. The Docker-daemon restart is intentionally a separate test.
image=${MK_TEST_IMAGE:-docker.io/library/busybox:1.36}
runtime=${MK_RUNTIME:-io.containerd.multikernel.v2}
kerf=${MK_KERF:-/opt/mkruntime/kerf-venv/bin/kerf}
task_id=mk-containerd-restart
scratch=$(mktemp -d)
pre_events=$scratch/events-before-restart.log
post_events=$scratch/events-after-restart.log
pre_event_pid=
post_event_pid=

stop_event_reader() {
	local pid=${1:-}
	if [[ -n $pid ]]; then
		kill "$pid" >/dev/null 2>&1 || true
		wait "$pid" >/dev/null 2>&1 || true
	fi
}

cleanup_task() {
	sudo ctr tasks kill --signal SIGKILL "$task_id" >/dev/null 2>&1 || true
	for _ in $(seq 1 120); do
		if ! sudo ctr tasks list -q | grep -Fxq "$task_id"; then
			break
		fi
		state=$(sudo ctr tasks list | awk -v id="$task_id" '$1 == id {print $3}')
		[[ $state != RUNNING && $state != PAUSED ]] && break
		sleep .25
	done
	sudo ctr tasks rm -f "$task_id" >/dev/null 2>&1 || true
	sudo ctr containers rm "$task_id" >/dev/null 2>&1 || true
}

cleanup() {
	set +e
	stop_event_reader "$pre_event_pid"
	stop_event_reader "$post_event_pid"
	cleanup_task
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
	printf 'pool_configured=%s children=%s links=%s nat_rules=%s filter_rules=%s ctr_tasks=%s moby_tasks=%s moby_containers=%s docker_containers=%s runtime_artifacts=%s rootfs_records=%s endpoints=%s shim_processes=%s helper_processes=%s' \
		"$(pool_configured)" \
		"$(sudo find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 -type d | wc -l)" \
		"$(ip -o link show | awk -F': ' '$2 ~ /^mkv[0-9a-f]+$/ {count++} END {print count+0}')" \
		"$(sudo iptables -t nat -S POSTROUTING | grep -c '172\.31\.' || true)" \
		"$(sudo iptables -S | grep -c '^\(-N\|-A\) MK-' || true)" \
		"$(sudo ctr tasks list -q | wc -l)" \
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
	for _ in $(seq 1 240); do
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

clean_resources='children=0 links=0 nat_rules=0 filter_rules=0 ctr_tasks=0 moby_tasks=0 moby_containers=0 docker_containers=0 runtime_artifacts=0 rootfs_records=0 endpoints=0 shim_processes=0 helper_processes=0'
clean_released="pool_configured=0 $clean_resources"
clean_retained="pool_configured=1 $clean_resources"

[[ $(id -u) -ne 0 ]] || {
	echo 'run as an ordinary sudo-capable user' >&2
	exit 1
}
for service in mkruntimed mknetd containerd docker; do
	[[ $(systemctl is-active "$service") = active ]]
done
cleanup_task
initial=$(wait_inventory "$clean_released")
observe initial-inventory "$initial"

host_boot_before=$(cat /proc/sys/kernel/random/boot_id)
containerd_pid_before=$(systemctl show -p MainPID --value containerd)
mkruntimed_pid_before=$(systemctl show -p MainPID --value mkruntimed)
observe identities-before "host_boot=$host_boot_before containerd_pid=$containerd_pid_before mkruntimed_pid=$mkruntimed_pid_before"

sudo ctr images pull "$image" >/dev/null
sudo timeout 600 stdbuf -oL ctr events >"$pre_events" 2>&1 &
pre_event_pid=$!
sleep 1

sudo ctr run --detach --runtime "$runtime" "$image" "$task_id" /bin/sh -c \
	'echo ctr-init-before; echo ctr-init-before-err >&2; while [ ! -e /tmp/restart-release ]; do sleep 1; done; echo ctr-init-after; echo ctr-init-after-err >&2'
for _ in $(seq 1 240); do
	state=$(sudo ctr tasks list | awk -v id="$task_id" '$1 == id {print $3}')
	[[ $state = RUNNING ]] && break
	sleep .25
done
[[ $state = RUNNING ]]

child_boot_before=$(sudo ctr task exec --exec-id restart-boot-before "$task_id" /bin/cat /proc/sys/kernel/random/boot_id)
exec_before=$(sudo ctr task exec --exec-id restart-exec-before "$task_id" /bin/sh -c \
	'echo ctr-exec-before; echo ctr-exec-before-err >&2' 2>&1)
grep -Fxq ctr-exec-before <<<"$exec_before"
grep -Fxq ctr-exec-before-err <<<"$exec_before"
observe task-before-restart "state=$state child_boot=$child_boot_before exec=$exec_before"

restart_started=$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)
sudo systemctl restart containerd
[[ $(systemctl is-active containerd) = active ]]
containerd_pid_after=$(systemctl show -p MainPID --value containerd)
host_boot_after=$(cat /proc/sys/kernel/random/boot_id)
mkruntimed_pid_after=$(systemctl show -p MainPID --value mkruntimed)
[[ $containerd_pid_after != "$containerd_pid_before" ]]
[[ $host_boot_after = "$host_boot_before" ]]
[[ $mkruntimed_pid_after = "$mkruntimed_pid_before" ]]
stop_event_reader "$pre_event_pid"
pre_event_pid=

sudo timeout 600 stdbuf -oL ctr events >"$post_events" 2>&1 &
post_event_pid=$!
sleep 1
for _ in $(seq 1 120); do
	state=$(sudo ctr tasks list | awk -v id="$task_id" '$1 == id {print $3}')
	[[ $state = RUNNING ]] && break
	sleep .25
done
[[ $state = RUNNING ]]
child_boot_after=$(sudo ctr task exec --exec-id restart-boot-after "$task_id" /bin/cat /proc/sys/kernel/random/boot_id)
exec_after=$(sudo ctr task exec --exec-id restart-exec-after "$task_id" /bin/sh -c \
	'echo ctr-exec-after; echo ctr-exec-after-err >&2' 2>&1)
[[ $child_boot_after = "$child_boot_before" ]]
grep -Fxq ctr-exec-after <<<"$exec_after"
grep -Fxq ctr-exec-after-err <<<"$exec_after"
observe identities-after "restart_started=$restart_started host_boot=$host_boot_after containerd_pid=$containerd_pid_after mkruntimed_pid=$mkruntimed_pid_after"
observe task-after-restart "state=$state child_boot=$child_boot_after exec=$exec_after"

sudo ctr task exec --exec-id restart-release "$task_id" /bin/touch /tmp/restart-release
init_stream=$(sudo ctr tasks attach "$task_id" 2>&1)
for marker in ctr-init-before ctr-init-before-err ctr-init-after ctr-init-after-err; do
	grep -Fxq "$marker" <<<"$init_stream"
done
observe init-stream-across-restart "$init_stream"

for _ in $(seq 1 120); do
	state=$(sudo ctr tasks list | awk -v id="$task_id" '$1 == id {print $3}')
	if [[ $state = STOPPED ]]; then
		break
	fi
	if ! sudo ctr tasks list -q | grep -Fxq "$task_id"; then
		state=ABSENT
		break
	fi
	sleep .25
done
if [[ $state = STOPPED ]]; then
	delete_output=$(sudo ctr tasks rm "$task_id")
else
	[[ $state = ABSENT ]]
	delete_output=task-already-absent-after-attach
fi
sudo ctr containers rm "$task_id"
sleep 2
stop_event_reader "$post_event_pid"
post_event_pid=

grep -Fq "$task_id" "$pre_events"
grep -Fq '/tasks/create' "$pre_events"
grep -Fq '/tasks/start' "$pre_events"
grep -Fq "$task_id" "$post_events"
grep -Fq '/tasks/exit' "$post_events"
grep -Fq '/tasks/delete' "$post_events"
observe events-before-restart "$(grep -F "$task_id" "$pre_events")"
observe events-after-restart "$(grep -F "$task_id" "$post_events")"
observe deletion "state_before_delete=$state output=$delete_output"

retained=$(wait_inventory "$clean_retained")
observe post-delete-retained-pool "$retained"
idle_mkruntimed_pid=$(systemctl show -p MainPID --value mkruntimed)
sudo systemctl restart mkruntimed
[[ $(systemctl is-active mkruntimed) = active ]]
released_mkruntimed_pid=$(systemctl show -p MainPID --value mkruntimed)
[[ $released_mkruntimed_pid != "$idle_mkruntimed_pid" ]]
released=$(wait_inventory "$clean_released")
observe final-released-inventory "$released"
printf 'G6_CONTAINERD_RESTART_CONTINUITY_PASS\n'
