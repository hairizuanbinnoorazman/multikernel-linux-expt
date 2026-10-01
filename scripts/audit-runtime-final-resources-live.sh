#!/usr/bin/env bash
set -euo pipefail
[[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]] && { PS4='+${BASH_SOURCE}:${LINENO}: '; set -x; }

kerf=${MK_KERF:-/opt/mkruntime/kerf-venv/bin/kerf}
task_root=/run/containerd/io.containerd.runtime.v2.task
storage_root=/srv/multikernel-storage/runtime

[[ $(id -u) -ne 0 ]] || { echo 'run as an ordinary sudo-capable user' >&2; exit 1; }
for service in mkruntimed mknetd containerd docker; do [[ $(systemctl is-active "$service") = active ]]; done

kerf_state=$(sudo "$kerf" show)
grep -Fq 'No memory pool configured' <<<"$kerf_state"
grep -Fq 'No instances found' <<<"$kerf_state"

children=$(sudo find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 -type d | wc -l)
runtime_mounts=$(findmnt -rn -o TARGET,SOURCE | awk -v storage="$storage_root" -v tasks="$task_root" \
	'index($1,storage "/") == 1 || index($1,tasks "/default/") == 1 || index($1,tasks "/moby/") == 1 || $2 ~ /^\/dev\/nbd[0-9]+(p[0-9]+)?$/ {n++} END {print n+0}')
runtime_artifacts=$( (sudo find "$storage_root" -mindepth 1 -print 2>/dev/null || true) | wc -l)
bundle_artifacts=$( (sudo find "$task_root/default" "$task_root/moby" -path '*/.multikernel/*' -print 2>/dev/null || true) | wc -l)
fifos=$( (sudo find "$task_root/default" "$task_root/moby" -type p -print 2>/dev/null || true) | wc -l)
links=$(ip -o link show | awk -F': ' '$2 ~ /^mkv[0-9a-f]+$/ {n++} END {print n+0}')
routes=$(ip -o route show table all | awk '$0 ~ /(^| )mkv[0-9a-f]+( |$)/ || $0 ~ /(^| )172\.31\./ {n++} END {print n+0}')
nat_rules=$(sudo iptables -t nat -S POSTROUTING | grep -c '172\.31\.' || true)
filter_rules=$(sudo iptables -S | grep -c '^\(-N\|-A\) MK-' || true)
default_tasks=$(sudo ctr tasks list -q | wc -l)
default_containers=$(sudo ctr containers list -q | wc -l)
moby_tasks=$(sudo ctr -n moby tasks list -q | wc -l)
moby_containers=$(sudo ctr -n moby containers list -q | wc -l)
docker_containers=$(sudo docker ps -aq | wc -l)
rootfs_records=$(sudo python3 -c 'import json; print(len(json.load(open("/var/lib/mkruntimed/rootfs/state.json"))["records"]))')
endpoints=$(sudo python3 -c 'import json; print(len(json.load(open("/var/lib/mknetd/state.json"))["endpoints"]))')
shim_processes=$(ps -eo comm= | awk '$1 == "containerd-shim" {n++} END {print n+0}')
nbd_processes=$(ps -eo comm= | awk '$1 == "mkvsock-nbd" {n++} END {print n+0}')
relay_processes=$(ps -eo comm= | awk '$1 == "mk-agent-relay" {n++} END {print n+0}')

inventory="children=$children runtime_mounts=$runtime_mounts runtime_artifacts=$runtime_artifacts bundle_artifacts=$bundle_artifacts fifos=$fifos links=$links routes=$routes nat_rules=$nat_rules filter_rules=$filter_rules default_tasks=$default_tasks default_containers=$default_containers moby_tasks=$moby_tasks moby_containers=$moby_containers docker_containers=$docker_containers rootfs_records=$rootfs_records endpoints=$endpoints shim_processes=$shim_processes nbd_processes=$nbd_processes relay_processes=$relay_processes"
expected='children=0 runtime_mounts=0 runtime_artifacts=0 bundle_artifacts=0 fifos=0 links=0 routes=0 nat_rules=0 filter_rules=0 default_tasks=0 default_containers=0 moby_tasks=0 moby_containers=0 docker_containers=0 rootfs_records=0 endpoints=0 shim_processes=0 nbd_processes=0 relay_processes=0'
[[ $inventory = "$expected" ]]

pid=$(systemctl show -p MainPID --value mkruntimed)
printf 'OBSERVATION_BEGIN key=provenance\nselector=%s boot_id=%s mkruntimed_pid=%s mkruntimed_sha256=%s\nOBSERVATION_END key=provenance\n' \
	"$(readlink -f /usr/local/lib/multikernel/current)" "$(cat /proc/sys/kernel/random/boot_id)" "$pid" \
	"$(sudo sha256sum "/proc/$pid/exe" | awk '{print $1}')"
printf 'OBSERVATION_BEGIN key=kerf-resource-return\n%s\nOBSERVATION_END key=kerf-resource-return\n' "$kerf_state"
printf 'OBSERVATION_BEGIN key=final-resource-inventory\n%s\nOBSERVATION_END key=final-resource-inventory\n' "$inventory"
for service in mkruntimed mknetd containerd docker; do
	systemctl show "$service.service" -p Id -p ActiveState -p SubState -p NRestarts -p MainPID --no-pager
done
echo G6_FINAL_RESOURCE_RETURN_PASS
