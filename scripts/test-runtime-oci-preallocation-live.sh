#!/usr/bin/env bash
set -euo pipefail
[[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]] && { PS4='+${BASH_SOURCE}:${LINENO}: '; set -x; }

expected_revision=${MK_EXPECTED_REVISION:-11a65f08f07b6bb88a6eb3088301582a37f55005}
expected_daemon_sha256=${MK_EXPECTED_DAEMON_SHA256:-0e1c87c34a346b44e4ca18a5f0084c6d69a3178dcc3ef4ade58922762e6a6e59}
expected_shim_sha256=${MK_EXPECTED_SHIM_SHA256:-326a827417e429b8c3ecf54603bde413ed8c6ebd8a243655aafebbe104549d6e}
expected_builder_sha256=${MK_EXPECTED_BUILDER_SHA256:-aae6496c7c8c12eaf8a2f7dc0bcc795b4e69f5fd1b9ab57e3079397224fc2377}
image=${MK_TEST_IMAGE:-docker.io/library/busybox:1.36}
runtime=${MK_RUNTIME:-io.containerd.multikernel.v2}
kerf=${MK_KERF:-/opt/mkruntime/kerf-venv/bin/kerf}
daemon_bin=${MK_DAEMON_BIN:-/usr/local/sbin/mkruntimed}
task_root=/run/containerd/io.containerd.runtime.v2.task
storage_root=/srv/multikernel-storage/runtime
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

assert_clean() {
	local key=$1 current kerf_state
	kerf_state=$(sudo "$kerf" show)
	grep -Fq 'No memory pool configured' <<<"$kerf_state"
	grep -Fq 'No instances found' <<<"$kerf_state"
	current=$(inventory)
	[[ $current = "$expected_inventory" ]]
	observe "$key" "$kerf_state
$current"
}

wait_clean() {
	local current kerf_state
	for _ in $(seq 1 60); do
		current=$(inventory)
		kerf_state=$(sudo "$kerf" show)
		if [[ $current = "$expected_inventory" ]] &&
			grep -Fq 'No memory pool configured' <<<"$kerf_state" &&
			grep -Fq 'No instances found' <<<"$kerf_state"; then
			return 0
		fi
		sleep 1
	done
	return 1
}

for service in mkruntimed mknetd containerd docker; do
	[[ $(systemctl is-active "$service") = active ]]
done

daemon_pid=$(systemctl show -p MainPID --value mkruntimed)
selector=$(readlink -f /usr/local/lib/multikernel/current)
daemon_version=$(sudo "$daemon_bin" --version)
daemon_sha256=$(sudo sha256sum "/proc/$daemon_pid/exe" | awk '{print $1}')
shim_path=/usr/local/bin/containerd-shim-multikernel-v2
shim_sha256=$(sudo sha256sum "$shim_path" | awk '{print $1}')
builder_path=$(readlink -f /usr/local/libexec/multikernel/build-runtime-container-initramfs.sh)
builder_sha256=$(sudo sha256sum "$builder_path" | awk '{print $1}')
grep -Fq "$expected_revision" <<<"$selector $daemon_version"
[[ $daemon_sha256 = "$expected_daemon_sha256" ]]
[[ $shim_sha256 = "$expected_shim_sha256" ]]
[[ $builder_sha256 = "$expected_builder_sha256" ]]
observe provenance "boot_id=$(cat /proc/sys/kernel/random/boot_id)
selector=$selector
daemon_pid=$daemon_pid
daemon_version=$daemon_version
daemon_sha256=$daemon_sha256
shim_sha256=$shim_sha256
shim_path=$(readlink -f "$shim_path")
builder_path=$builder_path
builder_sha256=$builder_sha256"

sudo ctr images list -q | grep -Fxq "$image"
assert_clean clean-before-unsupported-oci

unsupported_id=mk-oci-reject-$$
set +e
unsupported_output=$(sudo ctr run --rm --runtime "$runtime" \
	--apparmor-profile multikernel-deliberately-unsupported \
	"$image" "$unsupported_id" /bin/true 2>&1)
unsupported_status=$?
set -e
[[ $unsupported_status -ne 0 ]]
grep -Fq 'process.apparmorProfile must be the explicit unconfined compatibility value' <<<"$unsupported_output"
observe unsupported-oci-rejection "status=$unsupported_status
$unsupported_output"
immediate_kerf=$(sudo "$kerf" show)
grep -Fq 'No memory pool configured' <<<"$immediate_kerf"
grep -Fq 'No instances found' <<<"$immediate_kerf"
immediate_inventory=$(inventory)
normalized_immediate=$(sed -E 's/shim_processes=[0-9]+/shim_processes=0/' <<<"$immediate_inventory")
[[ $normalized_immediate = "$expected_inventory" ]]
observe no-allocation-immediately-after-rejection "$immediate_kerf
$immediate_inventory"
wait_clean
assert_clean clean-after-unsupported-oci

supported_id=mk-oci-supported-$$
supported_output=$(sudo ctr run --rm --runtime "$runtime" "$image" "$supported_id" \
	/bin/sh -c 'printf MK_OCI_SUPPORTED_PASS')
[[ $supported_output = MK_OCI_SUPPORTED_PASS ]]
observe supported-oci-workload "output=$supported_output"

# The daemon intentionally retains its empty reusable memory pool. A clean
# administrative restart releases that pool so the final audit can prove that
# every allocatable host and child resource returns to the pre-test state.
sudo systemctl restart mkruntimed
wait_clean
assert_clean clean-after-supported-oci-and-pool-release

for service in mkruntimed mknetd containerd docker; do
	systemctl show "$service.service" -p Id -p ActiveState -p SubState -p NRestarts -p MainPID --no-pager
done
echo G6_OCI_PREALLOCATION_LIVE_PASS
