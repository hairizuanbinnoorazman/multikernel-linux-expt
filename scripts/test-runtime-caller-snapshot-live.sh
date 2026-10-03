#!/usr/bin/env bash
set -euo pipefail

if [[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]]; then
	PS4='+${BASH_SOURCE}:${LINENO}: '
	set -x
fi

source_root=${1:?usage: test-runtime-caller-snapshot-live.sh SOURCE_ROOT}
qualification_source_root=$source_root
image=${MK_TEST_IMAGE:-docker.io/library/busybox:1.36}
runtime=${MK_RUNTIME:-io.containerd.multikernel.v2}
expected_index=${MK_EXPECTED_IMAGE_INDEX:-sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662}
expected_snapshot=${MK_EXPECTED_SNAPSHOT:-sha256:97e4ece8f8a4d01b66df2c55c1f1e8a2860597b54dffbbc0df2f9942062cb34d}
task_root=/run/containerd/io.containerd.runtime.v2.task
storage_root=/srv/multikernel-storage/runtime
ctr_id=mk-snapshot-relative
docker_name=mk-snapshot-absolute
scratch=$(mktemp -d -p /var/tmp mk-caller-snapshot.XXXXXX)
before_view=$scratch/view-before
after_view=$scratch/view-after
before_manifest=$scratch/source-before.json
after_manifest=$scratch/source-after.json
docker_id=
captured_manifest_sha=

observe() {
	local key=$1
	shift
	printf 'OBSERVATION_BEGIN key=%s\n%s\nOBSERVATION_END key=%s\n' "$key" "$*" "$key"
}

remove_view() {
	local view=$1
	sudo ctr -n default images unmount "$view" >/dev/null 2>&1 || sudo umount "$view" >/dev/null 2>&1 || true
	sudo ctr -n default snapshots rm "$view" >/dev/null 2>&1 || true
	sudo rmdir "$view" >/dev/null 2>&1 || true
}

cleanup() (
	set +e
	sudo docker rm -f "$docker_name" >/dev/null 2>&1 || true
	sudo ctr tasks kill --signal SIGKILL "$ctr_id" >/dev/null 2>&1 || true
	sudo ctr tasks rm -f "$ctr_id" >/dev/null 2>&1 || true
	sudo ctr containers rm "$ctr_id" >/dev/null 2>&1 || true
	remove_view "$before_view"
	remove_view "$after_view"
	sudo rm -rf -- "$scratch"
)
trap cleanup EXIT

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
		"$children" "$runtime_mounts" "$runtime_artifacts" "$bundle_artifacts" "$fifos" "$links" "$routes" \
		"$nat_rules" "$filter_rules" "$default_tasks" "$default_containers" "$moby_tasks" "$moby_containers" \
		"$docker_containers" "$rootfs_records" "$endpoints" "$shim_processes" "$nbd_processes" "$relay_processes"
}

clean_expected='children=0 runtime_mounts=0 runtime_artifacts=0 bundle_artifacts=0 fifos=0 links=0 routes=0 nat_rules=0 filter_rules=0 default_tasks=0 default_containers=0 moby_tasks=0 moby_containers=0 docker_containers=0 rootfs_records=0 endpoints=0 shim_processes=0 nbd_processes=0 relay_processes=0'

wait_clean() {
	local observed=
	for _ in $(seq 1 240); do
		observed=$(inventory)
		[[ $observed = "$clean_expected" ]] && { printf '%s' "$observed"; return 0; }
		sleep .5
	done
	printf '%s' "$observed"
	return 1
}

wait_ctr_running() {
	local state=
	for _ in $(seq 1 240); do
		state=$(sudo ctr tasks list | awk -v id="$ctr_id" '$1 == id {print $3}')
		[[ $state = RUNNING ]] && return 0
		sleep .5
	done
	return 1
}

wait_ctr_ready() {
	for _ in $(seq 1 120); do
		sudo ctr task exec --exec-id "snapshot-ready-$RANDOM" "$ctr_id" /bin/test -f /tmp/snapshot-ready >/dev/null 2>&1 && return 0
		sleep .5
	done
	return 1
}

wait_docker_ready() {
	for _ in $(seq 1 120); do
		sudo docker exec "$docker_name" /bin/test -f /tmp/snapshot-ready >/dev/null 2>&1 && return 0
		sleep .5
	done
	return 1
}

capture_build_result() {
	local namespace=$1 identity=$2 expected_kind=$3 expected_manifest_sha=$4
	local bundle="$task_root/$namespace/$identity"
	local configured_root requested_root source_root before_sha after_sha before_entries after_entries
	local independent_check source_mount direct_manifest direct_result direct_sha direct_entries
	configured_root=$(sudo jq -er '.root.path' "$bundle/config.json")
	requested_root=$(sudo jq -er '.requested_root' "$bundle/.multikernel/build-result.json")
	source_root=$(sudo jq -er '.source_root' "$bundle/.multikernel/build-result.json")
	before_sha=$(sudo jq -er '.source_scan_before.manifest_sha256' "$bundle/.multikernel/build-result.json")
	after_sha=$(sudo jq -er '.source_scan_after.manifest_sha256' "$bundle/.multikernel/build-result.json")
	before_entries=$(sudo jq -er '.source_scan_before.entries' "$bundle/.multikernel/build-result.json")
	after_entries=$(sudo jq -er '.source_scan_after.entries' "$bundle/.multikernel/build-result.json")
	[[ $configured_root = "$requested_root" ]]
	[[ $before_sha = "$after_sha" ]]
	[[ $before_entries = "$after_entries" ]]
	if [[ $expected_kind = relative ]]; then
		[[ $configured_root = rootfs && $configured_root != /* ]]
		[[ $before_sha = "$expected_manifest_sha" ]]
		independent_check="committed_view_manifest_sha256=$expected_manifest_sha"
	else
		[[ $configured_root = /var/lib/docker/rootfs/overlayfs/* && $configured_root = /* ]]
		direct_manifest=$scratch/docker-source-after-guest-write.json
		direct_result=$(sudo "$qualification_source_root/scripts/build-runtime-rootfs.py" "$source_root" "$scratch/unused-docker" "$direct_manifest" \
			--manifest-only --max-bytes 1073741824 --max-inodes 131072)
		direct_sha=$(sudo sha256sum "$direct_manifest" | awk '{print $1}')
		direct_entries=$(jq -er '.entries' <<<"$direct_result")
		[[ $direct_sha = "$before_sha" && $direct_entries = "$before_entries" ]]
		source_mount=$(sudo findmnt -rn -T "$source_root" -o TARGET,SOURCE,FSTYPE,OPTIONS)
		independent_check="live_source_after_guest_write=$direct_result
live_source_mount=$source_mount"
	fi
	observe "$namespace-build-result" "bundle=$bundle
configured_root=$configured_root
requested_root=$requested_root
source_root=$source_root
source_scan_before_sha256=$before_sha
source_scan_after_sha256=$after_sha
source_entries=$before_entries
$independent_check"
}

capture_view() {
	local label=$1 view=$2 manifest=$3
	local mount_row options root_identity busybox_identity result manifest_sha
	mkdir -p "$view"
	sudo ctr -n default images mount --platform linux/amd64 "$image" "$view"
	mountpoint -q "$view"
	mount_row=$(findmnt -rn -M "$view" -o TARGET,SOURCE,FSTYPE,OPTIONS)
	options=$(findmnt -rn -M "$view" -o OPTIONS)
	[[ ,$options, = *,ro,* ]]
	root_identity=$(sudo stat -c '%d:%i mode=%a owner=%u:%g' "$view")
	busybox_identity=$(sudo stat -c '%d:%i mode=%a owner=%u:%g sha256=' "$view/bin/busybox")$(sudo sha256sum "$view/bin/busybox" | awk '{print $1}')
	result=$(sudo "$source_root/scripts/build-runtime-rootfs.py" "$view" "$scratch/unused" "$manifest" \
		--manifest-only --max-bytes 1073741824 --max-inodes 131072)
	manifest_sha=$(sudo sha256sum "$manifest" | awk '{print $1}')
	[[ $manifest_sha = "$(jq -er '.manifest_sha256' <<<"$result")" ]]
	observe "$label-readonly-view" "mount=$mount_row
root_identity=$root_identity
busybox_identity=$busybox_identity
manifest=$result
manifest_file_sha256=$manifest_sha"
	sudo ctr -n default images unmount "$view"
	! mountpoint -q "$view"
	sudo ctr -n default snapshots rm "$view"
	sudo rmdir "$view"
	captured_manifest_sha=$manifest_sha
}

[[ $(id -u) -ne 0 ]] || { echo 'run as an ordinary sudo-capable user' >&2; exit 1; }
test -d "$source_root/runtime"
test -x "$source_root/scripts/build-runtime-rootfs.py"
for service in mkruntimed mknetd containerd docker; do
	[[ $(systemctl is-active "$service") = active ]]
	[[ $(systemctl show -p NRestarts --value "$service") = 0 ]]
done
test -S /run/mkruntimed.sock
test -S /run/mknetd.sock
cleanup
mkdir -p "$scratch"
observe clean-before "$(wait_clean)"

snapshot_before=$(sudo ctr -n default snapshots ls | awk 'NR > 1 && $1 != "" {print $1}' | sort)
[[ $snapshot_before = "$expected_snapshot" ]]
snapshot_info=$(sudo ctr -n default snapshots info "$expected_snapshot")
jq -e --arg snapshot "$expected_snapshot" \
	'.Kind == "Committed" and .Name == $snapshot and ((has("Parent") | not) or .Parent == "")' \
	<<<"$snapshot_info" >/dev/null

image_index=$(sudo ctr -n default images ls | awk -v image="$image" '$1 == image {print $3}')
[[ $image_index = "$expected_index" ]]
index_json=$(sudo ctr -n default content get "$image_index")
manifest_digest=$(jq -er '[.manifests[] | select(.platform.os == "linux" and .platform.architecture == "amd64" and ((.platform.variant // "") == ""))] | if length == 1 then .[0].digest else error("amd64 manifest is not unique") end' <<<"$index_json")
manifest_json=$(sudo ctr -n default content get "$manifest_digest")
config_digest=$(jq -er '.config.digest' <<<"$manifest_json")
layer_digest=$(jq -er 'if (.layers | length) == 1 then .layers[0].digest else error("expected one layer") end' <<<"$manifest_json")
config_json=$(sudo ctr -n default content get "$config_digest")
diff_id=$(jq -er 'if (.rootfs.diff_ids | length) == 1 then .rootfs.diff_ids[0] else error("expected one diff id") end' <<<"$config_json")
[[ $diff_id = "$expected_snapshot" ]]
observe oci-snapshot-identity "image=$image
index_digest=$image_index
linux_amd64_manifest_digest=$manifest_digest
config_digest=$config_digest
layer_digest=$layer_digest
rootfs_diff_id=$diff_id
snapshot=$snapshot_info"

rootfs_pattern='^(TestMountPinsFilesystemInputsAcrossPathReplacement|TestMountRejectsSymlinkSubstitutionAtDescriptorOpen|TestMountRejectsReplacedTargetIdentityBeforeMount|TestValidateMountsRejectsHostileInputBeforeBackendUse|TestPrepareJournalsBuildUnmountAndReplays|TestPrepareBuildFailureUnmountsAndRemovesArtifacts|TestMountFailureDefensivelyUnmountsAndRemovesArtifacts|TestRejectedMountTargetDoesNotUnmountReplacement|TestUncertainPartialMountRemainsRecoverable|TestUnmountFailurePreservesRecoverableState|TestRootfsOperationsRejectPreCancelledContextWithoutMutation)$'
(
	cd "$source_root/runtime"
	GOCACHE="$scratch/go-cache" go test -race -v -count=1 ./internal/rootfs -run "$rootfs_pattern"
	GOCACHE="$scratch/go-cache" go test -race -v -count=1 ./cmd/containerd-shim-multikernel-v2 -run '^TestRootfsMountsAreSanitizedForDaemonAndValidated$'
)
echo FOCUSED_ROOTFS_MOUNT_UNMOUNT_TESTS_PASS

capture_view before "$before_view" "$before_manifest"
before_sha=$captured_manifest_sha
guest_program='set -eu; before=$(sha256sum /etc/passwd | cut -d" " -f1); printf "multikernel-private-write\n" >>/etc/passwd; after=$(sha256sum /etc/passwd | cut -d" " -f1); test "$before" != "$after"; printf "private-root-only\n" >/mk-snapshot-write; printf "%s %s\n" "$before" "$after" >/tmp/snapshot-hashes; touch /tmp/snapshot-ready; while [ ! -e /tmp/snapshot-release ]; do sleep 1; done'

sudo ctr run --detach --runtime "$runtime" "$image" "$ctr_id" /bin/sh -c "$guest_program"
wait_ctr_running
wait_ctr_ready
capture_build_result default "$ctr_id" relative "$before_sha"
ctr_marker=$(sudo ctr task exec --exec-id snapshot-read-relative "$ctr_id" /bin/cat /mk-snapshot-write)
ctr_hashes=$(sudo ctr task exec --exec-id snapshot-hashes-relative "$ctr_id" /bin/cat /tmp/snapshot-hashes)
[[ $ctr_marker = private-root-only ]]
observe relative-root-guest-write "marker=$ctr_marker
passwd_before_after=$ctr_hashes"
sudo ctr task exec --exec-id snapshot-release-relative "$ctr_id" /bin/touch /tmp/snapshot-release
for _ in $(seq 1 120); do
	ctr_state=$(sudo ctr tasks list | awk -v id="$ctr_id" '$1 == id {print $3}')
	[[ $ctr_state = STOPPED || -z $ctr_state ]] && break
	sleep .5
done
[[ $ctr_state = STOPPED || -z $ctr_state ]]
[[ -z $ctr_state ]] || sudo ctr tasks rm "$ctr_id" >/dev/null
sudo ctr containers rm "$ctr_id"

docker_isolation=(
	--network none
	--security-opt apparmor=unconfined
	--security-opt seccomp=unconfined
	--sysctl net.ipv4.ip_unprivileged_port_start=1024
	--sysctl 'net.ipv4.ping_group_range=1 0'
	--device-cgroup-rule 'a *:* rwm'
)
sudo docker run --detach --runtime "$runtime" "${docker_isolation[@]}" --name "$docker_name" "$image" /bin/sh -c "$guest_program" >/dev/null
wait_docker_ready
docker_id=$(sudo docker inspect --format '{{.Id}}' "$docker_name")
capture_build_result moby "$docker_id" absolute "$before_sha"
docker_marker=$(sudo docker exec "$docker_name" /bin/cat /mk-snapshot-write)
docker_hashes=$(sudo docker exec "$docker_name" /bin/cat /tmp/snapshot-hashes)
[[ $docker_marker = private-root-only ]]
observe absolute-root-guest-write "container_id=$docker_id
marker=$docker_marker
passwd_before_after=$docker_hashes"
sudo docker exec "$docker_name" /bin/touch /tmp/snapshot-release
[[ $(sudo docker wait "$docker_name") = 0 ]]
sudo docker rm "$docker_name" >/dev/null
docker_id=

capture_view after "$after_view" "$after_manifest"
after_sha=$captured_manifest_sha
[[ $before_sha = "$after_sha" ]]
sudo cmp -s "$before_manifest" "$after_manifest"
snapshot_after=$(sudo ctr -n default snapshots ls | awk 'NR > 1 && $1 != "" {print $1}' | sort)
[[ $snapshot_after = "$snapshot_before" ]]
observe caller-snapshot-unchanged "before_manifest_sha256=$before_sha
after_manifest_sha256=$after_sha
manifest_bytes_identical=true
snapshot_inventory_before=$snapshot_before
snapshot_inventory_after=$snapshot_after"

# A completed workload may leave the intentionally reusable empty pool. A
# clean daemon restart releases it so the final audit matches the pre-run host.
sudo systemctl restart mkruntimed
observe clean-after "$(wait_clean)"
for service in mkruntimed mknetd containerd docker; do
	systemctl show "$service.service" -p Id -p ActiveState -p SubState -p NRestarts -p MainPID --no-pager
done

trap - EXIT
sudo rm -rf -- "$scratch"
echo G4_CALLER_SNAPSHOT_LIVE_PASS
