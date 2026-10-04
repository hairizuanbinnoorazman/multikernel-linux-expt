#!/usr/bin/env bash
set -euo pipefail

if [[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]]; then
	PS4='+${BASH_SOURCE}:${LINENO}: '
	set -x
fi

source_root=${1:?usage: test-runtime-writable-state-model-live.sh SOURCE_ROOT}
image=${MK_TEST_IMAGE:-docker.io/library/busybox:1.36}
runtime=${MK_RUNTIME:-io.containerd.multikernel.v2}
task_id=mk-ephemeral-state
reject_ctr_id=mk-writable-bind-reject
docker_name=mk-writable-bind-reject
scratch=$(mktemp -d -p /var/tmp mk-writable-state.XXXXXX)
host_input=$scratch/host-input

observe() {
	local key=$1
	shift
	printf 'OBSERVATION_BEGIN key=%s\n%s\nOBSERVATION_END key=%s\n' "$key" "$*" "$key"
}

cleanup_runtime() (
	set +e
	sudo docker rm -f "$docker_name" >/dev/null 2>&1 || true
	for id in "$task_id" "$reject_ctr_id"; do
		sudo ctr tasks kill --signal SIGKILL "$id" >/dev/null 2>&1 || true
		sudo ctr tasks rm -f "$id" >/dev/null 2>&1 || true
		sudo ctr containers rm "$id" >/dev/null 2>&1 || true
	done
)
cleanup() (
	set +e
	cleanup_runtime
	sudo rm -rf -- "$scratch"
)
trap cleanup EXIT

inventory() {
	local live_exports
	live_exports=$(sudo python3 -c 'import json; d=json.load(open("/var/lib/mkruntimed/storage/state.json")); print(sum(v["state"] != "RELEASED" for v in d["exports"].values()))')
	printf 'children=%s runtime_artifacts=%s default_tasks=%s default_containers=%s moby_tasks=%s moby_containers=%s docker_containers=%s rootfs_records=%s live_exports=%s endpoints=%s shim_processes=%s nbd_processes=%s relay_processes=%s' \
		"$(sudo find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 -type d | wc -l)" \
		"$( (sudo find /srv/multikernel-storage/runtime -mindepth 1 -print 2>/dev/null || true) | wc -l)" \
		"$(sudo ctr tasks list -q | wc -l)" "$(sudo ctr containers list -q | wc -l)" \
		"$(sudo ctr -n moby tasks list -q | wc -l)" "$(sudo ctr -n moby containers list -q | wc -l)" \
		"$(sudo docker ps -aq | wc -l)" \
		"$(sudo python3 -c 'import json; print(len(json.load(open("/var/lib/mkruntimed/rootfs/state.json"))["records"]))')" \
		"$live_exports" \
		"$(sudo python3 -c 'import json; print(len(json.load(open("/var/lib/mknetd/state.json"))["endpoints"]))')" \
		"$(ps -eo comm= | awk '$1 == "containerd-shim" {n++} END {print n+0}')" \
		"$(ps -eo comm= | awk '$1 == "mkvsock-nbd" {n++} END {print n+0}')" \
		"$(ps -eo comm= | awk '$1 == "mk-agent-relay" {n++} END {print n+0}')"
}

clean_expected='children=0 runtime_artifacts=0 default_tasks=0 default_containers=0 moby_tasks=0 moby_containers=0 docker_containers=0 rootfs_records=0 live_exports=0 endpoints=0 shim_processes=0 nbd_processes=0 relay_processes=0'

wait_clean() {
	local observed=
	for _ in $(seq 1 480); do
		observed=$(inventory)
		[[ $observed = "$clean_expected" ]] && { printf '%s' "$observed"; return 0; }
		sleep .25
	done
	printf '%s' "$observed"
	return 1
}

[[ $(id -u) -ne 0 ]] || { echo 'run as an ordinary sudo-capable user' >&2; exit 1; }
test -d "$source_root/runtime"
test -x "$source_root/scripts/test-runtime-oci-validation.py"
for service in mkruntimed mknetd containerd docker; do
	[[ $(systemctl is-active "$service") = active ]]
	[[ $(systemctl show -p NRestarts --value "$service") = 0 ]]
done
cleanup_runtime
observe clean-before "$(wait_clean)"
observe provenance "boot_id=$(cat /proc/sys/kernel/random/boot_id)
selector=$(readlink -f /usr/local/lib/multikernel/current)
mkruntimed_pid=$(systemctl show -p MainPID --value mkruntimed)
mkruntimed_sha256=$(sudo sha256sum /proc/$(systemctl show -p MainPID --value mkruntimed)/exe | awk '{print $1}')
qualifier_sha256=$(sha256sum "$source_root/scripts/test-runtime-writable-state-model-live.sh" | awk '{print $1}')"

(
	cd "$source_root"
	PYTHONDONTWRITEBYTECODE=1 TMPDIR="$scratch" python3 scripts/test-runtime-oci-validation.py
	PYTHONDONTWRITEBYTECODE=1 TMPDIR="$scratch" python3 scripts/test-runtime-bind-materialization.py
)
echo FOCUSED_WRITABLE_STATE_MODEL_TESTS_PASS

cycle_one=$(sudo ctr run --rm --runtime "$runtime" "$image" "$task_id" /bin/sh -c \
	'test ! -e /mk-unconfigured-state; printf generation-one >/mk-unconfigured-state; cat /mk-unconfigured-state')
[[ $cycle_one = generation-one ]]
observe ephemeral-cycle-one "task=$task_id private_write=$cycle_one"
observe cycle-one-clean "$(wait_clean)"

cycle_two=$(sudo ctr run --rm --runtime "$runtime" "$image" "$task_id" /bin/sh -c \
	'test ! -e /mk-unconfigured-state; printf generation-two >/mk-unconfigured-state; cat /mk-unconfigured-state')
[[ $cycle_two = generation-two ]]
observe ephemeral-cycle-two "task=$task_id prior_write=absent private_write=$cycle_two"
observe cycle-two-clean "$(wait_clean)"

mkdir -m 0755 "$host_input"
printf 'host-immutable\n' >"$host_input/value"
chmod 0644 "$host_input/value"

set +e
ctr_error=$(sudo ctr run --rm --runtime "$runtime" \
	--mount "type=bind,src=$host_input,dst=/opt/writable,options=rbind:rw" \
	"$image" "$reject_ctr_id" /bin/true 2>&1)
ctr_status=$?
set -e
[[ $ctr_status -ne 0 ]]
grep -Fq 'read-only bind' <<<"$ctr_error"
[[ $(<"$host_input/value") = host-immutable ]]
cleanup_runtime
observe ctr-writable-bind-rejected "status=$ctr_status diagnostic_sha256=$(sha256sum <<<"$ctr_error" | awk '{print $1}') host_value=$(<"$host_input/value")"
observe ctr-rejection-clean "$(wait_clean)"

set +e
docker_error=$(sudo docker run --rm --runtime "$runtime" --network none \
	--security-opt apparmor=unconfined --security-opt seccomp=unconfined \
	--sysctl net.ipv4.ip_unprivileged_port_start=1024 --sysctl 'net.ipv4.ping_group_range=1 0' \
	--device-cgroup-rule 'a *:* rwm' --name "$docker_name" \
	--mount "type=bind,src=$host_input,dst=/opt/writable" "$image" /bin/true 2>&1)
docker_status=$?
set -e
[[ $docker_status -ne 0 ]]
grep -Fq 'read-only bind' <<<"$docker_error"
[[ $(<"$host_input/value") = host-immutable ]]
cleanup_runtime
observe docker-writable-bind-rejected "status=$docker_status diagnostic_sha256=$(sha256sum <<<"$docker_error" | awk '{print $1}') host_value=$(<"$host_input/value")"
observe docker-rejection-clean "$(wait_clean)"

set +e
propagation_error=$(sudo ctr run --rm --runtime "$runtime" \
	--mount "type=bind,src=$host_input,dst=/opt/shared,options=rbind:ro:rshared" \
	"$image" "$reject_ctr_id" /bin/true 2>&1)
propagation_status=$?
set -e
[[ $propagation_status -ne 0 ]]
grep -Fq 'read-only bind' <<<"$propagation_error"
cleanup_runtime
observe shared-propagation-rejected "status=$propagation_status diagnostic_sha256=$(sha256sum <<<"$propagation_error" | awk '{print $1}')"
observe propagation-rejection-clean "$(wait_clean)"

for service in mkruntimed mknetd containerd docker; do
	[[ $(systemctl is-active "$service") = active ]]
	[[ $(systemctl show -p NRestarts --value "$service") = 0 ]]
done
trap - EXIT
sudo rm -rf -- "$scratch"
echo G4_WRITABLE_STATE_MODEL_LIVE_PASS
