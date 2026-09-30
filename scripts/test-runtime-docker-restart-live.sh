#!/usr/bin/env bash
set -euo pipefail

if [[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]]; then
	PS4='+${BASH_SOURCE}:${LINENO}: '
	set -x
fi

# Focused disposable-host proof that Docker reconnects to a live Multikernel
# container. Requires an explicitly validated live-restore=true Docker policy.
image=${MK_TEST_IMAGE:-docker.io/library/busybox:1.36}
runtime=${MK_RUNTIME:-io.containerd.multikernel.v2}
kerf=${MK_KERF:-/opt/mkruntime/kerf-venv/bin/kerf}
container_name=mk-docker-restart
scratch=$(mktemp -d)
events=$scratch/events.log
event_pid=
docker_isolation=(
	--network none
	--security-opt apparmor=unconfined
	--security-opt seccomp=unconfined
	--sysctl net.ipv4.ip_unprivileged_port_start=1024
	--sysctl 'net.ipv4.ping_group_range=1 0'
	--device-cgroup-rule 'a *:* rwm'
)

stop_event_reader() {
	if [[ -n ${event_pid:-} ]]; then
		kill "$event_pid" >/dev/null 2>&1 || true
		wait "$event_pid" >/dev/null 2>&1 || true
		event_pid=
	fi
}

cleanup() {
	set +e
	stop_event_reader
	sudo docker rm -f "$container_name" >/dev/null 2>&1 || true
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
[[ $(sudo docker info --format '{{.LiveRestoreEnabled}}') = true ]]
sudo docker rm -f "$container_name" >/dev/null 2>&1 || true
initial=$(wait_inventory "$clean_released")
observe initial-inventory "$initial"

host_boot_before=$(cat /proc/sys/kernel/random/boot_id)
docker_pid_before=$(systemctl show -p MainPID --value docker)
containerd_pid_before=$(systemctl show -p MainPID --value containerd)
mkruntimed_pid_before=$(systemctl show -p MainPID --value mkruntimed)
observe identities-before "host_boot=$host_boot_before docker_pid=$docker_pid_before containerd_pid=$containerd_pid_before mkruntimed_pid=$mkruntimed_pid_before live_restore=true"

sudo docker image inspect "$image" >/dev/null 2>&1 || sudo docker pull "$image" >/dev/null
sudo timeout 600 stdbuf -oL ctr -n moby events >"$events" 2>&1 &
event_pid=$!
sleep 1

sudo docker run --detach --runtime "$runtime" "${docker_isolation[@]}" \
	--name "$container_name" "$image" /bin/sh -c \
	'echo docker-init-before; echo docker-init-before-err >&2; while [ ! -e /tmp/restart-release ]; do sleep 1; done; echo docker-init-after; echo docker-init-after-err >&2' >/dev/null
for _ in $(seq 1 240); do
	state=$(sudo docker inspect --format '{{.State.Status}}' "$container_name" 2>/dev/null || true)
	[[ $state = running ]] && break
	sleep .25
done
[[ $state = running ]]
container_id=$(sudo docker inspect --format '{{.Id}}' "$container_name")
child_boot_before=$(sudo docker exec "$container_name" /bin/cat /proc/sys/kernel/random/boot_id)
exec_before=$(sudo docker exec "$container_name" /bin/sh -c \
	'echo docker-exec-before; echo docker-exec-before-err >&2' 2>&1)
logs_before=$(sudo docker logs "$container_name" 2>&1)
for marker in docker-exec-before docker-exec-before-err; do
	grep -Fxq "$marker" <<<"$exec_before"
done
for marker in docker-init-before docker-init-before-err; do
	grep -Fxq "$marker" <<<"$logs_before"
done
observe container-before-restart "id=$container_id state=$state child_boot=$child_boot_before exec=$exec_before logs=$logs_before"

restart_started=$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)
sudo systemctl restart docker
[[ $(systemctl is-active docker) = active ]]
docker_pid_after=$(systemctl show -p MainPID --value docker)
containerd_pid_after=$(systemctl show -p MainPID --value containerd)
mkruntimed_pid_after=$(systemctl show -p MainPID --value mkruntimed)
host_boot_after=$(cat /proc/sys/kernel/random/boot_id)
[[ $docker_pid_after != "$docker_pid_before" ]]
[[ $containerd_pid_after = "$containerd_pid_before" ]]
[[ $mkruntimed_pid_after = "$mkruntimed_pid_before" ]]
[[ $host_boot_after = "$host_boot_before" ]]
[[ $(sudo docker info --format '{{.LiveRestoreEnabled}}') = true ]]

for _ in $(seq 1 240); do
	state=$(sudo docker inspect --format '{{.State.Status}}' "$container_name" 2>/dev/null || true)
	[[ $state = running ]] && break
	sleep .25
done
[[ $state = running ]]
child_boot_after=$(sudo docker exec "$container_name" /bin/cat /proc/sys/kernel/random/boot_id)
exec_after=$(sudo docker exec "$container_name" /bin/sh -c \
	'echo docker-exec-after; echo docker-exec-after-err >&2' 2>&1)
[[ $child_boot_after = "$child_boot_before" ]]
for marker in docker-exec-after docker-exec-after-err; do
	grep -Fxq "$marker" <<<"$exec_after"
done
observe identities-after "restart_started=$restart_started host_boot=$host_boot_after docker_pid=$docker_pid_after containerd_pid=$containerd_pid_after mkruntimed_pid=$mkruntimed_pid_after live_restore=true"
observe container-after-restart "id=$container_id state=$state child_boot=$child_boot_after exec=$exec_after"

sudo docker exec "$container_name" /bin/touch /tmp/restart-release
exit_status=$(sudo docker wait "$container_name")
[[ $exit_status = 0 ]]
logs_after=$(sudo docker logs "$container_name" 2>&1)
for marker in docker-init-before docker-init-before-err docker-init-after docker-init-after-err; do
	grep -Fxq "$marker" <<<"$logs_after"
done
observe logs-across-restart "$logs_after"
sudo docker rm "$container_name" >/dev/null
sleep 2
stop_event_reader

grep -Fq "$container_id" "$events"
for topic in /tasks/create /tasks/start /tasks/exit /tasks/delete; do
	grep -Fq "$topic" "$events"
done
observe events-across-restart "$(grep -F "$container_id" "$events")"
observe deletion "exit_status=$exit_status docker_container_removed=true"

retained=$(wait_inventory "$clean_retained")
observe post-delete-retained-pool "$retained"
idle_mkruntimed_pid=$(systemctl show -p MainPID --value mkruntimed)
sudo systemctl restart mkruntimed
[[ $(systemctl is-active mkruntimed) = active ]]
released_mkruntimed_pid=$(systemctl show -p MainPID --value mkruntimed)
[[ $released_mkruntimed_pid != "$idle_mkruntimed_pid" ]]
released=$(wait_inventory "$clean_released")
observe final-released-inventory "$released"
printf 'G6_DOCKER_RESTART_CONTINUITY_PASS\n'
