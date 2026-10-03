#!/usr/bin/env bash
set -euo pipefail
[[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]] && { PS4='+${BASH_SOURCE}:${LINENO}: '; set -x; }

binary_manager=${MK_BINARY_MANAGER:-/var/tmp/mk-manager-3fd1238/manage-runtime-binaries.py}
deployment_manager=${MK_DEPLOYMENT_MANAGER:-/var/tmp/mk-manager-3fd1238/manage-runtime-deployment.py}
candidate_release=${MK_CANDIDATE_RELEASE:-0.1.0-dev-11a65f08f07b6bb88a6eb3088301582a37f55005}
candidate_revision=${candidate_release#0.1.0-dev-}
candidate_deployment=${MK_CANDIDATE_DEPLOYMENT:-b4d185c68b567c8d44882b34978cd18ac29a67d22f264cc9b4d719971a84371f}
candidate_daemon_sha=${MK_CANDIDATE_DAEMON_SHA:-0e1c87c34a346b44e4ca18a5f0084c6d69a3178dcc3ef4ade58922762e6a6e59}
candidate_shim_sha=${MK_CANDIDATE_SHIM_SHA:-326a827417e429b8c3ecf54603bde413ed8c6ebd8a243655aafebbe104549d6e}
candidate_builder_sha=${MK_CANDIDATE_BUILDER_SHA:-aae6496c7c8c12eaf8a2f7dc0bcc795b4e69f5fd1b9ab57e3079397224fc2377}
rollback_release=${MK_ROLLBACK_RELEASE:-0.1.0-dev-1edd368f640f28e880480c638c0936a5cbaba6b0}
rollback_revision=${rollback_release#0.1.0-dev-}
rollback_deployment=${MK_ROLLBACK_DEPLOYMENT:-25e6d343e1e18d7f5d5a55a7d029163a9289960ffe18c078f529467cfeb6beb7}
rollback_daemon_sha=${MK_ROLLBACK_DAEMON_SHA:-c7090da61fb4b546b7d55d8d0e476098d886c79f0727499f4450e7dc81245048}
rollback_shim_sha=${MK_ROLLBACK_SHIM_SHA:-45fd9f3a26cb1d323d20f9d9a5e56a2e828ab117668007fcc13deaec2cb9def8}
rollback_builder_sha=${MK_ROLLBACK_BUILDER_SHA:-0ded581c3444c023913f84df52323a3ec95a044ab1891488d4778965a82773d8}
image=${MK_TEST_IMAGE:-docker.io/library/busybox:1.36}
runtime=${MK_RUNTIME:-io.containerd.multikernel.v2}
kerf=${MK_KERF:-/opt/mkruntime/kerf-venv/bin/kerf}
task_root=/run/containerd/io.containerd.runtime.v2.task
storage_root=/srv/multikernel-storage/runtime
rollback_id=mk-packaging-rollback-$$
forward_id=mk-packaging-forward-$$
restore_required=0
expected_inventory='children=0 runtime_mounts=0 runtime_artifacts=0 bundle_artifacts=0 fifos=0 links=0 routes=0 nat_rules=0 filter_rules=0 default_tasks=0 default_containers=0 moby_tasks=0 moby_containers=0 docker_containers=0 rootfs_records=0 endpoints=0 shim_processes=0 nbd_processes=0 relay_processes=0'

[[ $(id -u) -ne 0 ]] || { echo 'run as an ordinary sudo-capable user' >&2; exit 1; }

observe() {
	local key=$1
	shift
	printf 'OBSERVATION_BEGIN key=%s\n%s\nOBSERVATION_END key=%s\n' "$key" "$*" "$key"
}

inventory() {
	local children runtime_mounts runtime_artifacts bundle_artifacts fifos links routes
	local nat_rules filter_rules default_tasks default_containers moby_tasks moby_containers
	local docker_containers rootfs_records endpoints shim_processes nbd_processes relay_processes
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
	printf 'children=%s runtime_mounts=%s runtime_artifacts=%s bundle_artifacts=%s fifos=%s links=%s routes=%s nat_rules=%s filter_rules=%s default_tasks=%s default_containers=%s moby_tasks=%s moby_containers=%s docker_containers=%s rootfs_records=%s endpoints=%s shim_processes=%s nbd_processes=%s relay_processes=%s' \
		"$children" "$runtime_mounts" "$runtime_artifacts" "$bundle_artifacts" "$fifos" "$links" "$routes" "$nat_rules" "$filter_rules" "$default_tasks" "$default_containers" "$moby_tasks" "$moby_containers" "$docker_containers" "$rootfs_records" "$endpoints" "$shim_processes" "$nbd_processes" "$relay_processes"
}

wait_clean() {
	local current kerf_state
	for _ in $(seq 1 120); do
		current=$(inventory)
		kerf_state=$(sudo "$kerf" show)
		if [[ $current = "$expected_inventory" ]] &&
			grep -Fq 'No memory pool configured' <<<"$kerf_state" &&
			grep -Fq 'No instances found' <<<"$kerf_state"; then
			observe "$1" "$kerf_state
$current"
			return 0
		fi
		sleep 1
	done
	echo "clean-state timeout: $current" >&2
	return 1
}

cleanup_task() {
	local id=$1
	sudo ctr tasks kill --signal SIGKILL "$id" >/dev/null 2>&1 || true
	sudo ctr tasks rm -f "$id" >/dev/null 2>&1 || true
	sudo ctr containers rm "$id" >/dev/null 2>&1 || true
}

activate_generation() {
	local label=$1 release=$2 deployment=$3 revision=$4 daemon_sha=$5 shim_sha=$6 builder_sha=$7
	sudo systemctl stop mkruntimed mknetd
	sudo "$deployment_manager" activate "$deployment"
	sudo "$binary_manager" activate "$release"
	sudo systemctl daemon-reload
	sudo systemctl start mknetd mkruntimed
	for service in mkruntimed mknetd containerd docker; do
		[[ $(systemctl is-active "$service") = active ]]
	done
	local binary_selector support_selector pid version observed_daemon_sha observed_shim_sha builder_path observed_builder_sha
	binary_selector=$(readlink -f /usr/local/lib/multikernel/current)
	support_selector=$(readlink -f /etc/multikernel/current)
	[[ $binary_selector = "/usr/local/lib/multikernel/releases/$release" ]]
	[[ $support_selector = "/etc/multikernel/deployments/$deployment" ]]
	pid=$(systemctl show -p MainPID --value mkruntimed)
	version=$(sudo /usr/local/sbin/mkruntimed --version)
	grep -Fq "$revision" <<<"$version"
	observed_daemon_sha=$(sudo sha256sum "/proc/$pid/exe" | awk '{print $1}')
	observed_shim_sha=$(sudo sha256sum /usr/local/bin/containerd-shim-multikernel-v2 | awk '{print $1}')
	builder_path=$(readlink -f /usr/local/libexec/multikernel/build-runtime-container-initramfs.sh)
	observed_builder_sha=$(sudo sha256sum "$builder_path" | awk '{print $1}')
	[[ $observed_daemon_sha = "$daemon_sha" ]]
	[[ $observed_shim_sha = "$shim_sha" ]]
	[[ $observed_builder_sha = "$builder_sha" ]]
	[[ $(sudo docker info --format '{{.DefaultRuntime}}') = runc ]]
	sudo containerd config dump | grep -F "runtimes.multikernel" >/dev/null
	observe "$label" "binary_selector=$binary_selector
support_selector=$support_selector
mkruntimed_pid=$pid
mkruntimed_version=$version
mkruntimed_sha256=$observed_daemon_sha
shim_sha256=$observed_shim_sha
builder_path=$builder_path
builder_sha256=$observed_builder_sha
docker_default_runtime=runc
containerd_named_runtime=multikernel"
}

restore_candidate() {
	local original_status=$?
	trap - EXIT
	if ((restore_required)); then
		set +e
		cleanup_task "$rollback_id"
		cleanup_task "$forward_id"
		sudo systemctl stop mkruntimed mknetd
		sudo "$deployment_manager" activate "$candidate_deployment"
		sudo "$binary_manager" activate "$candidate_release"
		sudo systemctl daemon-reload
		sudo systemctl start mknetd mkruntimed
	fi
	exit "$original_status"
}
trap restore_candidate EXIT

for manager in "$binary_manager" "$deployment_manager"; do
	[[ $(sudo stat -c '%U:%G:%a' "$manager") = root:root:755 ]]
done
sudo "$binary_manager" inspect
sudo "$deployment_manager" inspect
wait_clean clean-before-packaging-rollback

restore_required=1
activate_generation rollback-generation "$rollback_release" "$rollback_deployment" "$rollback_revision" \
	"$rollback_daemon_sha" "$rollback_shim_sha" "$rollback_builder_sha"
rollback_output=$(sudo ctr run --rm --runtime "$runtime" "$image" "$rollback_id" /bin/sh -c 'printf MK_PACKAGING_ROLLBACK_PASS')
[[ $rollback_output = MK_PACKAGING_ROLLBACK_PASS ]]
observe rollback-workload "output=$rollback_output"
sudo systemctl restart mkruntimed
wait_clean clean-after-rollback-workload

activate_generation forward-restored-generation "$candidate_release" "$candidate_deployment" "$candidate_revision" \
	"$candidate_daemon_sha" "$candidate_shim_sha" "$candidate_builder_sha"
forward_output=$(sudo ctr run --rm --runtime "$runtime" "$image" "$forward_id" /bin/sh -c 'printf MK_PACKAGING_FORWARD_PASS')
[[ $forward_output = MK_PACKAGING_FORWARD_PASS ]]
observe forward-restored-workload "output=$forward_output"
sudo systemctl restart mkruntimed
wait_clean clean-after-forward-restoration

for service in mkruntimed mknetd containerd docker; do
	systemctl show "$service.service" -p Id -p ActiveState -p SubState -p NRestarts -p MainPID --no-pager
done
restore_required=0
echo G6_PACKAGING_ROLLBACK_LIVE_PASS
