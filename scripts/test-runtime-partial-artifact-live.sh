#!/usr/bin/env bash
set -euo pipefail

if [[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]]; then
	PS4='+${BASH_SOURCE}:${LINENO}: '
	set -x
fi

source_root=${1:?usage: test-runtime-partial-artifact-live.sh SOURCE_ROOT}
runtime=${MK_RUNTIME:-io.containerd.multikernel.v2}
image=${MK_TEST_IMAGE:-docker.io/library/busybox:1.36}
task_id=mk-partial-artifact-fault
kerf=/opt/mkruntime/kerf-venv/bin/kerf
real_kerf=/opt/mkruntime/bin/kerf-real
fault_wrapper=/opt/mkruntime/bin/kerf-fault-wrapper
fault_control=/run/mkruntime-kerf-fault
manager=$source_root/scripts/manage-runtime-deployment.py
scratch=$(mktemp -d -p /var/tmp mk-partial-artifact.XXXXXX)
qualification_root=/etc/multikernel/qualification-partial-artifact-$$
original_deployment=$(basename "$(readlink -f /etc/multikernel/current)")
fault_deployment=
restore_required=0

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

wait_runtime_ready() {
	for _ in $(seq 1 60); do
		if systemctl is-active --quiet mkruntimed && sudo test -S /run/mkruntimed.sock; then
			return 0
		fi
		sleep 1
	done
	return 1
}

restore() {
	local status=$?
	set +e
	cleanup_task
	sudo rm -f -- "$fault_control"
	if (( restore_required )); then
		sudo python3 "$manager" activate "$original_deployment" >/dev/null
		sudo systemctl daemon-reload
		sudo systemctl restart mkruntimed
		wait_runtime_ready
	fi
	if [[ -n $fault_deployment && $fault_deployment != "$original_deployment" ]]; then
		sudo python3 "$manager" remove-deployment "$fault_deployment" --apply >/dev/null
	fi
	sudo rm -f -- "$fault_wrapper" "$real_kerf"
	sudo rm -rf -- "$qualification_root"
	rm -rf -- "$scratch"
	exit "$status"
}
trap restore EXIT

inventory() {
	local task_root=/run/containerd/io.containerd.runtime.v2.task
	local storage_root=/srv/multikernel-storage/runtime
	local children runtime_mounts runtime_artifacts bundle_artifacts fifos links routes nat_rules filter_rules
	local default_tasks default_containers moby_tasks moby_containers docker_containers rootfs_records endpoints
	local shim_processes nbd_processes relay_processes live_exports
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
	live_exports=$(sudo python3 -c 'import json; print(sum(v["state"] != "RELEASED" for v in json.load(open("/var/lib/mkruntimed/storage/state.json"))["exports"].values()))')
	endpoints=$(sudo python3 -c 'import json; print(len(json.load(open("/var/lib/mknetd/state.json"))["endpoints"]))')
	shim_processes=$(ps -eo comm= | awk '$1 == "containerd-shim" {n++} END {print n+0}')
	nbd_processes=$(ps -eo comm= | awk '$1 == "mkvsock-nbd" {n++} END {print n+0}')
	relay_processes=$(ps -eo comm= | awk '$1 == "mk-agent-relay" {n++} END {print n+0}')
	printf 'children=%s runtime_mounts=%s runtime_artifacts=%s bundle_artifacts=%s fifos=%s links=%s routes=%s nat_rules=%s filter_rules=%s default_tasks=%s default_containers=%s moby_tasks=%s moby_containers=%s docker_containers=%s rootfs_records=%s live_exports=%s endpoints=%s shim_processes=%s nbd_processes=%s relay_processes=%s' \
		"$children" "$runtime_mounts" "$runtime_artifacts" "$bundle_artifacts" "$fifos" "$links" "$routes" "$nat_rules" "$filter_rules" \
		"$default_tasks" "$default_containers" "$moby_tasks" "$moby_containers" "$docker_containers" "$rootfs_records" "$live_exports" \
		"$endpoints" "$shim_processes" "$nbd_processes" "$relay_processes"
}

assert_clean() {
	local key=$1 current kerf_state
	local expected='children=0 runtime_mounts=0 runtime_artifacts=0 bundle_artifacts=0 fifos=0 links=0 routes=0 nat_rules=0 filter_rules=0 default_tasks=0 default_containers=0 moby_tasks=0 moby_containers=0 docker_containers=0 rootfs_records=0 live_exports=0 endpoints=0 shim_processes=0 nbd_processes=0 relay_processes=0'
	current=$(inventory)
	kerf_state=$(sudo "$kerf" show --verbose)
	[[ $current = "$expected" ]]
	grep -Fq 'No memory pool configured' <<<"$kerf_state"
	grep -Fq 'No instances found' <<<"$kerf_state"
	observe "$key" "$current
pool_configured=0 children=0"
}

[[ $(id -u) -ne 0 ]] || { echo 'run as an ordinary sudo-capable user' >&2; exit 1; }
test -d "$source_root/runtime"
test -f "$manager"
test -x "$source_root/scripts/kerf-fault-wrapper.sh"
for path in "$real_kerf" "$fault_wrapper" "$fault_control" "$qualification_root"; do sudo test ! -e "$path"; done
for service in mkruntimed mknetd containerd docker; do
	[[ $(systemctl is-active "$service") = active ]]
	[[ $(systemctl show -p NRestarts --value "$service") = 0 ]]
done
cleanup_task
assert_clean clean-before-live-load-fault
before_exports=$(sudo python3 -c 'import json; print(len(json.load(open("/var/lib/mkruntimed/storage/state.json"))["exports"]))')
observe provenance "boot_id=$(cat /proc/sys/kernel/random/boot_id)
selector=$(readlink -f /usr/local/lib/multikernel/current)
support_deployment=$original_deployment
mkruntimed_sha256=$(sudo sha256sum /proc/$(systemctl show -p MainPID --value mkruntimed)/exe | awk '{print $1}')
qualifier_sha256=$(sha256sum "$0" | awk '{print $1}')"

sudo install -d -m 0755 /opt/mkruntime/bin
sudo install -m 0755 "$kerf" "$real_kerf"
sudo install -m 0755 "$source_root/scripts/kerf-fault-wrapper.sh" "$fault_wrapper"
sudo install -d -m 0700 "$qualification_root"
sudo install -m 0600 /etc/multikernel/current/runtime.env "$qualification_root/runtime.env"
sudo jq '.kerf_executable="/opt/mkruntime/bin/kerf-fault-wrapper"' /etc/mkruntime/config.json | sudo tee "$qualification_root/config.json" >/dev/null
sudo chown root:root "$qualification_root/config.json"
sudo chmod 0600 "$qualification_root/config.json"
fault_install=$(sudo python3 "$manager" install "$qualification_root/runtime.env" "$qualification_root/config.json")
fault_deployment=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["installed_and_active"])' "$fault_install")
restore_required=1
sudo systemctl daemon-reload
sudo systemctl restart mkruntimed
wait_runtime_ready
[[ $(sudo jq -r .kerf_executable /etc/mkruntime/config.json) = "$fault_wrapper" ]]
observe fault-deployment "original=$original_deployment
fault=$fault_deployment
kerf_executable=$fault_wrapper
wrapper_sha256=$(sudo sha256sum "$fault_wrapper" | awk '{print $1}')
real_kerf_sha256=$(sudo sha256sum "$real_kerf" | awk '{print $1}')"

printf 'load\n' | sudo tee "$fault_control" >/dev/null
sudo chmod 0600 "$fault_control"
set +e
fault_output=$(sudo ctr run --rm --runtime "$runtime" "$image" "$task_id" /bin/true 2>&1)
fault_status=$?
set -e
[[ $fault_status -ne 0 ]]
sudo test ! -e "$fault_control"
observe injected-load-failure "status=$fault_status
$(grep -E 'BACKEND_FAILURE|retained_output_sha256|exit status 42' <<<"$fault_output" | head -5)"

immediate=$(inventory)
observe immediate-post-failure-inventory "$immediate"
for _ in $(seq 1 120); do
	shim_processes=$(ps -eo comm= | awk '$1 == "containerd-shim" {n++} END {print n+0}')
	[[ $shim_processes -eq 0 ]] && break
	sleep .25
done
assert_clean clean-after-live-load-fault
after_exports=$(sudo python3 -c 'import json; print(len(json.load(open("/var/lib/mkruntimed/storage/state.json"))["exports"]))')
[[ $after_exports -eq $((before_exports + 1)) ]]
released=$(sudo python3 - "$before_exports" <<'PY'
import json,re,sys
before=int(sys.argv[1])
storage=json.load(open('/var/lib/mkruntimed/storage/state.json'))
assert len(storage['exports']) == before + 1
values=list(storage['exports'].values())
value=max(values,key=lambda x:x['created_at'])
assert value['state'] == 'RELEASED', value
assert re.fullmatch(r'e2fsck-clean-sha256:[0-9a-f]{64}',value['offline_check']), value
print(json.dumps({'state':value['state'],'offline_check':value['offline_check'],'counters':value['counters']},sort_keys=True,separators=(',',':')))
PY
)
observe released-failed-create-storage "$released"

sudo python3 "$manager" activate "$original_deployment" >/dev/null
sudo systemctl daemon-reload
sudo systemctl restart mkruntimed
wait_runtime_ready
restore_required=0
[[ $(sudo jq -r .kerf_executable /etc/mkruntime/config.json) = "$kerf" ]]
sudo python3 "$manager" remove-deployment "$fault_deployment" --apply >/dev/null
fault_deployment=
sudo rm -f -- "$fault_wrapper" "$real_kerf"
sudo rm -rf -- "$qualification_root"

shim_pattern='^(TestCreateAmbiguityCancellationRemovesPreparedArtifacts|TestCreateRejectsOCIValidationBeforeAllocationOrArtifacts|TestCreatePostValidationFailureRollbackMatrix)$'
rootfs_pattern='^(TestPrepareBuildFailureUnmountsAndRemovesArtifacts|TestMountFailureDefensivelyUnmountsAndRemovesArtifacts|TestUncertainPartialMountRemainsRecoverable|TestFinalStateFailureRemovesBuiltArtifactsAndRecoveryRecord|TestCleanupPersistenceFailurePreservesDiagnosableRecord|TestUnmountFailurePreservesRecoverableState)$'
lifecycle_pattern='^(TestCancelCreateTombstonesBeforeExternalCleanup|TestFirstCreateFailureReleasesNewPool|TestCancelCreateRetriesUncertainFirstPoolRelease|TestCancelCreateReleasesAmbiguousPoolInitialization)$'
(
	cd "$source_root/runtime"
	TMPDIR="$scratch" GOCACHE="$scratch/go-cache" go test -race -v -count=1 ./cmd/containerd-shim-multikernel-v2 -run "$shim_pattern"
	TMPDIR="$scratch" GOCACHE="$scratch/go-cache" go test -race -v -count=1 ./internal/rootfs -run "$rootfs_pattern"
	TMPDIR="$scratch" GOCACHE="$scratch/go-cache" go test -race -v -count=1 ./internal/lifecycle -run "$lifecycle_pattern"
)
echo FOCUSED_PARTIAL_ARTIFACT_MATRIX_PASS
assert_clean clean-after-focused-matrix
for service in mkruntimed mknetd containerd docker; do
	[[ $(systemctl is-active "$service") = active ]]
	[[ $(systemctl show -p NRestarts --value "$service") = 0 ]]
done

trap - EXIT
rm -rf -- "$scratch"
echo G4_PARTIAL_ARTIFACT_LIVE_PASS
