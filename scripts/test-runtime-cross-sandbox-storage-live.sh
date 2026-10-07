#!/usr/bin/env bash
set -euo pipefail

# Exercise the storage boundary from two real, simultaneously running child
# kernels.  Deliberately do not enable xtrace: exact export generations are
# bearer credentials and must never enter the retained transcript.
source_root=${1:?usage: test-runtime-cross-sandbox-storage-live.sh SOURCE_ROOT}
base_image=${MK_TEST_BASE_IMAGE:-docker.io/library/busybox:1.36}
image=${MK_TEST_IMAGE:-docker.io/library/mk-storage-isolation:g4-live}
runtime=${MK_RUNTIME:-io.containerd.multikernel.v2}
task_a=mk-storage-isolation-a
task_b=mk-storage-isolation-b
scratch=$(mktemp -d -p /var/tmp mk-storage-isolation.XXXXXX)
helper=$scratch/mkvsock-nbd
image_context=$scratch/image
image_archive=$scratch/image.tar

observe() {
	local key=$1
	shift
	printf 'OBSERVATION_BEGIN key=%s\n%s\nOBSERVATION_END key=%s\n' "$key" "$*" "$key"
}

task_identity() {
	printf 'default\0%s' "$1" | sha256sum | awk '{print "task-" substr($1,1,32)}'
}

cleanup_task() {
	local id=$1 state=
	sudo ctr tasks kill --signal SIGKILL "$id" >/dev/null 2>&1 || true
	for _ in $(seq 1 240); do
		state=$(sudo ctr tasks list | awk -v id="$id" '$1 == id {print $3}')
		[[ $state != RUNNING && $state != PAUSED ]] && break
		sleep .25
	done
	sudo ctr tasks rm -f "$id" >/dev/null 2>&1 || true
	sudo ctr containers rm "$id" >/dev/null 2>&1 || true
}

cleanup() (
	set +e
	cleanup_task "$task_a"
	cleanup_task "$task_b"
	sudo ctr images rm "$image" >/dev/null 2>&1 || true
	sudo docker image rm "$image" >/dev/null 2>&1 || true
	sudo rm -rf -- "$scratch"
)
trap cleanup EXIT

state_counts() {
	sudo python3 - <<'PY'
import json, os
def records(path, key):
    if not os.path.exists(path):
        return []
    return list(json.load(open(path))[key].values())
rootfs=records('/var/lib/mkruntimed/rootfs/state.json','records')
exports=records('/var/lib/mkruntimed/storage/state.json','exports')
print('rootfs_records=%d live_exports=%d' % (
    len(rootfs), sum(v['state'] != 'RELEASED' for v in exports)))
PY
}

wait_counts() {
	local wanted=$1 observed=
	for _ in $(seq 1 360); do
		observed=$(state_counts)
		[[ $observed = "$wanted" ]] && { printf '%s' "$observed"; return 0; }
		sleep .25
	done
	printf '%s' "$observed"
	return 1
}

wait_task_state() {
	local id=$1 wanted=$2 observed=
	for _ in $(seq 1 360); do
		observed=$(sudo ctr tasks list | awk -v id="$id" '$1 == id {print $3}')
		[[ $observed = "$wanted" ]] && return 0
		sleep .25
	done
	echo "task $id state is '$observed', wanted '$wanted'" >&2
	return 1
}

export_field() {
	local identity=$1 field=$2
	sudo python3 - "$identity" "$field" <<'PY'
import json, sys
identity,field=sys.argv[1:]
root=json.load(open('/var/lib/mkruntimed/rootfs/state.json'))['records'][identity]
exports=json.load(open('/var/lib/mkruntimed/storage/state.json'))['exports'].values()
matches=[x for x in exports if x['state'] != 'RELEASED' and x['path'] == root['storage']['path']]
assert len(matches) == 1, matches
value=matches[0]
print(value[field])
PY
}

process_field() {
	local port=$1 field=$2
	sudo python3 - "$port" "$field" <<'PY'
import glob,json,sys
port=int(sys.argv[1]); field=sys.argv[2]
matches=[]
for path in glob.glob('/run/mkstorage/*.json'):
    value=json.load(open(path))
    if value['port'] == port:
        matches.append((path,value))
assert len(matches) == 1, matches
path,value=matches[0]
print(path if field == '_path' else value[field])
PY
}

server_io() {
	local pid=$1
	sudo awk '/^(syscr|syscw|read_bytes|write_bytes):/ {printf "%s=%s ",$1,$2} END {print ""}' "/proc/$pid/io"
}

file_hash() {
	sudo sha256sum "$1" | awk '{print $1}'
}

runtime_inventory() {
	local state_files=0 live_exports=0 rootfs_records=0
	if sudo test -e /var/lib/mkruntimed/storage/state.json; then
		state_files=1
		live_exports=$(sudo python3 -c 'import json; x=json.load(open("/var/lib/mkruntimed/storage/state.json")); print(sum(v["state"] != "RELEASED" for v in x["exports"].values()))')
	fi
	if sudo test -e /var/lib/mkruntimed/rootfs/state.json; then
		rootfs_records=$(sudo python3 -c 'import json; print(len(json.load(open("/var/lib/mkruntimed/rootfs/state.json"))["records"]))')
	fi
	printf 'instances=%s ctr_tasks=%s rootfs_records=%s live_exports=%s storage_state_files=%s helpers=%s' \
		"$(sudo find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 -type d | wc -l)" \
		"$(sudo ctr tasks list -q | wc -l)" "$rootfs_records" "$live_exports" "$state_files" \
		"$( (pgrep -f '^/usr/local/libexec/multikernel/(mkvsock-nbd|mk-agent-relay)' || true) | wc -l)"
}

guest_exec() {
	local id=$1 exec_id=$2
	shift 2
	sudo ctr task exec --exec-id "$exec_id" "$id" "$@"
}

wait_guest_ready() {
	local id=$1 label=$2 attempt
	for attempt in $(seq 1 360); do
		if guest_exec "$id" "ready-$label-$attempt" /bin/test -e \
			/tmp/storage-isolation-marker >/dev/null 2>&1; then
			return 0
		fi
		sleep .25
	done
	echo "guest $id did not publish its workload readiness marker" >&2
	return 1
}

attack() {
	local attacker=$1 label=$2 port=$3 size=$4 image_id=$5 generation=$6
	local output rc start end duration class mount_rc
	output=$scratch/attack-$label.log
	install -m 0600 /dev/null "$output"
	start=$(date +%s)
	set +e
	timeout 25s sudo ctr task exec --exec-id "attack-$label" "$attacker" \
		/usr/local/bin/mkvsock-nbd client 0 "$port" /dev/nbd1 "$size" "$image_id" "$generation" reserved \
		>"$output" 2>&1
	rc=$?
	set -e
	end=$(date +%s)
	duration=$((end-start))
	[[ $rc -ne 0 ]]
	if grep -Fq 'MKNBD_CLIENT_DEVICE_READY' "$output"; then
		class=unexpected-device-ready
	elif grep -Fq 'MKNBD_CLIENT_CONNECTED' "$output"; then
		class=unexpected-handshake-accepted
	elif grep -Fq 'MKNBD_CLIENT_REFUSED reason=identity' "$output"; then
		class=identity-refused
	elif grep -Fq 'hello response eof' "$output"; then
		class=occupied-listener-no-response
	elif grep -Fq 'connect:' "$output"; then
		class=connect-refused
	elif [[ $rc -eq 124 ]]; then
		class=outer-timeout
	else
		class=other-bounded-failure
	fi
	[[ $class != unexpected-device-ready && $class != unexpected-handshake-accepted ]]
	[[ $(guest_exec "$attacker" "nbd1-size-$label" /bin/cat /sys/class/block/nbd1/size) = 0 ]]
	set +e
	guest_exec "$attacker" "mount-$label" /bin/sh -c \
		'mkdir -p /tmp/peer-mount; mount -t ext4 /dev/nbd1 /tmp/peer-mount >/dev/null 2>&1' \
		>/dev/null 2>&1
	mount_rc=$?
	set -e
	[[ $mount_rc -ne 0 ]]
	guest_exec "$attacker" "unmounted-$label" /bin/sh -c \
		'! awk '\''$2 == "/tmp/peer-mount" {found=1} END {exit found ? 0 : 1}'\'' /proc/mounts'
	observe "attack-$label" "attacker=$attacker peer_port=$port identity_mode=${label##*-} exit_status=$rc duration_seconds=$duration result=$class output_bytes=$(wc -c <"$output") output_sha256=$(sha256sum "$output" | awk '{print $1}') nbd1_sectors=0 mount_exit_status=$mount_rc mounted=0"
}

[[ $(id -u) -ne 0 ]] || { echo 'run as an ordinary sudo-capable user' >&2; exit 1; }
test -d "$source_root/runtime"
for service in mkruntimed mknetd containerd docker; do
	[[ $(systemctl is-active "$service") = active ]]
	[[ $(systemctl show -p NRestarts --value "$service") = 0 ]]
done
cleanup_task "$task_a"
cleanup_task "$task_b"
[[ $(wait_counts 'rootfs_records=0 live_exports=0') = 'rootfs_records=0 live_exports=0' ]]
[[ $(sudo find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 -type d | wc -l) = 0 ]]
install -d -m 0700 "$image_context"
install -m 0755 /usr/local/libexec/multikernel/mkvsock-nbd "$helper"
install -m 0755 "$helper" "$image_context/mkvsock-nbd"
cat >"$image_context/Dockerfile" <<EOF
FROM $base_image
COPY mkvsock-nbd /usr/local/bin/mkvsock-nbd
RUN chmod 0755 /usr/local/bin/mkvsock-nbd
EOF
sudo docker build --pull=false --network=none -t "$image" "$image_context"
sudo docker save -o "$image_archive" "$image"
sudo ctr images import "$image_archive" >/dev/null
sudo ctr images list -q | grep -Fxq "$image"
helper_hash=$(sha256sum /usr/local/libexec/multikernel/mkvsock-nbd | awk '{print $1}')
observe provenance "boot_id=$(cat /proc/sys/kernel/random/boot_id)
selector=$(readlink -f /usr/local/lib/multikernel/current)
mkruntimed_sha256=$(sudo sha256sum /proc/$(systemctl show -p MainPID --value mkruntimed)/exe | awk '{print $1}')
installed_nbd_sha256=$helper_hash
image_nbd_sha256=$(sha256sum "$image_context/mkvsock-nbd" | awk '{print $1}')
docker_image_id=$(sudo docker image inspect --format '{{.Id}}' "$image")
containerd_image_digest=$(sudo ctr images list | awk -v image="$image" '$1 == image {print $3}')
qualifier_sha256=$(sha256sum "${BASH_SOURCE[0]}" | awk '{print $1}')"
observe initial-inventory "$(runtime_inventory)"

(
	cd "$source_root"
	scripts/test-mkvsock-nbd-timeouts.sh
)

guest_program='set -eu; printf child-ready > /tmp/storage-isolation-marker; while [ ! -e /tmp/storage-isolation-release ]; do sleep 1; done'
for id in "$task_a" "$task_b"; do
	sudo ctr containers create --runtime "$runtime" \
		"$image" "$id" /bin/sh -c "$guest_program"
	sudo ctr tasks start --detach "$id"
	wait_task_state "$id" RUNNING
	wait_guest_ready "$id" "${id##*-}"
done
[[ $(wait_counts 'rootfs_records=2 live_exports=2') = 'rootfs_records=2 live_exports=2' ]]

identity_a=$(task_identity "$task_a")
identity_b=$(task_identity "$task_b")
port_a=$(export_field "$identity_a" port)
port_b=$(export_field "$identity_b" port)
size_a=$(export_field "$identity_a" size_bytes)
size_b=$(export_field "$identity_b" size_bytes)
image_a=$(export_field "$identity_a" image_id)
image_b=$(export_field "$identity_b" image_id)
generation_a=$(export_field "$identity_a" export_generation)
generation_b=$(export_field "$identity_b" export_generation)
path_a=$(export_field "$identity_a" path)
path_b=$(export_field "$identity_b" path)
pid_a=$(process_field "$port_a" pid)
pid_b=$(process_field "$port_b" pid)
record_a=$(process_field "$port_a" _path)
record_b=$(process_field "$port_b" _path)
[[ $port_a != "$port_b" && $path_a != "$path_b" && $generation_a != "$generation_b" ]]

for id in "$task_a" "$task_b"; do
	[[ $(guest_exec "$id" "marker-${id##*-}" /bin/cat /tmp/storage-isolation-marker) = child-ready ]]
	[[ $(guest_exec "$id" "helper-hash-${id##*-}" /bin/sha256sum /usr/local/bin/mkvsock-nbd | awk '{print $1}') = "$helper_hash" ]]
	[[ $(guest_exec "$id" "nbd0-${id##*-}" /bin/cat /sys/class/block/nbd0/size) -gt 0 ]]
	[[ $(guest_exec "$id" "nbd1-${id##*-}" /bin/cat /sys/class/block/nbd1/size) = 0 ]]
	guest_exec "$id" "own-write-${id##*-}" /bin/sh -c \
		'printf own-root-healthy > /tmp/own-root-health; printf "%s" "$0" > /tmp/storage-isolation-canary; mkdir -p /tmp/peer-mount; sync; test "$(cat /tmp/own-root-health)" = own-root-healthy' "$id"
done
sleep 2

canary_a_before=$(guest_exec "$task_a" canary-a-before /bin/sha256sum /tmp/storage-isolation-canary | awk '{print $1}')
canary_b_before=$(guest_exec "$task_b" canary-b-before /bin/sha256sum /tmp/storage-isolation-canary | awk '{print $1}')

state_before=$(file_hash /var/lib/mkruntimed/storage/state.json)
rootfs_before=$(file_hash /var/lib/mkruntimed/rootfs/state.json)
image_hash_a_before=$(file_hash "$path_a")
image_hash_b_before=$(file_hash "$path_b")
record_hash_a_before=$(file_hash "$record_a")
record_hash_b_before=$(file_hash "$record_b")
io_a_before=$(server_io "$pid_a")
io_b_before=$(server_io "$pid_b")
observe live-owners "task_a=$task_a task_a_identity=$identity_a port_a=$port_a image_a_sha256=$image_hash_a_before generation_a_sha256=$(printf %s "$generation_a" | sha256sum | awk '{print $1}') pid_a=$pid_a task_b=$task_b task_b_identity=$identity_b port_b=$port_b image_b_sha256=$image_hash_b_before generation_b_sha256=$(printf %s "$generation_b" | sha256sum | awk '{print $1}') pid_b=$pid_b durable_state_sha256=$state_before rootfs_state_sha256=$rootfs_before server_a_io='$io_a_before' server_b_io='$io_b_before'"

wrong_image=cross-sandbox-invalid-image
wrong_generation=00000000000000000000000000000000
attack "$task_a" a-to-b-wrong "$port_b" "$size_b" "$wrong_image" "$wrong_generation"
attack "$task_a" a-to-b-exact "$port_b" "$size_b" "$image_b" "$generation_b"
attack "$task_b" b-to-a-wrong "$port_a" "$size_a" "$wrong_image" "$wrong_generation"
attack "$task_b" b-to-a-exact "$port_a" "$size_a" "$image_a" "$generation_a"

state_after=$(file_hash /var/lib/mkruntimed/storage/state.json)
rootfs_after=$(file_hash /var/lib/mkruntimed/rootfs/state.json)
image_hash_a_after=$(file_hash "$path_a")
image_hash_b_after=$(file_hash "$path_b")
record_hash_a_after=$(file_hash "$record_a")
record_hash_b_after=$(file_hash "$record_b")
io_a_after=$(server_io "$pid_a")
io_b_after=$(server_io "$pid_b")
canary_a_after=$(guest_exec "$task_a" canary-a-after /bin/sha256sum /tmp/storage-isolation-canary | awk '{print $1}')
canary_b_after=$(guest_exec "$task_b" canary-b-after /bin/sha256sum /tmp/storage-isolation-canary | awk '{print $1}')
[[ $state_before = "$state_after" && $rootfs_before = "$rootfs_after" ]]
[[ $record_hash_a_before = "$record_hash_a_after" && $record_hash_b_before = "$record_hash_b_after" ]]
[[ $canary_a_before = "$canary_a_after" && $canary_b_before = "$canary_b_after" ]]
[[ $(process_field "$port_a" pid) = "$pid_a" && $(process_field "$port_b" pid) = "$pid_b" ]]
observe post-attack-integrity "durable_state_sha256=$state_after rootfs_state_sha256=$rootfs_after image_a_before_sha256=$image_hash_a_before image_a_after_sha256=$image_hash_a_after image_b_before_sha256=$image_hash_b_before image_b_after_sha256=$image_hash_b_after process_record_a_sha256=$record_hash_a_after process_record_b_sha256=$record_hash_b_after canary_a_sha256=$canary_a_after canary_b_sha256=$canary_b_after server_a_io_before='$io_a_before' server_a_io_after='$io_a_after' server_b_io_before='$io_b_before' server_b_io_after='$io_b_after' ownership_records_and_canaries_unchanged=1 server_pids_unchanged=1"

for id in "$task_a" "$task_b"; do
	[[ $(guest_exec "$id" "health-${id##*-}" /bin/cat /tmp/own-root-health) = own-root-healthy ]]
	[[ $(guest_exec "$id" "nbd1-final-${id##*-}" /bin/cat /sys/class/block/nbd1/size) = 0 ]]
	guest_exec "$id" "release-${id##*-}" /bin/touch /tmp/storage-isolation-release
	done
for id in "$task_a" "$task_b"; do
	wait_task_state "$id" STOPPED
	sudo ctr tasks rm "$id" >/dev/null
	sudo ctr containers rm "$id"
done
[[ $(wait_counts 'rootfs_records=0 live_exports=0') = 'rootfs_records=0 live_exports=0' ]]
for _ in $(seq 1 240); do
	[[ $(sudo find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 -type d | wc -l) = 0 ]] && break
	sleep .25
done
old_mkruntimed_pid=$(systemctl show -p MainPID --value mkruntimed)
sudo systemctl restart mkruntimed
[[ $(systemctl is-active mkruntimed) = active ]]
new_mkruntimed_pid=$(systemctl show -p MainPID --value mkruntimed)
[[ $new_mkruntimed_pid != "$old_mkruntimed_pid" ]]
kerf_state=$(sudo /opt/mkruntime/kerf-venv/bin/kerf show)
grep -Fq 'No memory pool configured' <<<"$kerf_state"
final_inventory=$(runtime_inventory)
[[ $final_inventory = 'instances=0 ctr_tasks=0 rootfs_records=0 live_exports=0 storage_state_files=1 helpers=0' ]]
observe final-inventory "old_mkruntimed_pid=$old_mkruntimed_pid new_mkruntimed_pid=$new_mkruntimed_pid $final_inventory"
for service in mkruntimed mknetd containerd docker; do
	[[ $(systemctl is-active "$service") = active ]]
	[[ $(systemctl show -p NRestarts --value "$service") = 0 ]]
done

trap - EXIT
sudo ctr images rm "$image" >/dev/null
sudo docker image rm "$image" >/dev/null
sudo rm -rf -- "$scratch"
echo G4_CROSS_SANDBOX_STORAGE_ISOLATION_PASS
