#!/usr/bin/env bash
set -Eeuo pipefail

source_root=${1:?usage: test-runtime-root-provenance-live.sh SOURCE_ROOT}
image=${MK_TEST_IMAGE:-docker.io/library/busybox:1.36}
runtime=${MK_RUNTIME:-io.containerd.multikernel.v2}
task_id=mk-root-provenance
manifest=/etc/mkruntime/kernels/gce-mk2.json
scratch=

observe() {
	local key=$1
	shift
	printf 'OBSERVATION_BEGIN key=%s\n%s\nOBSERVATION_END key=%s\n' "$key" "$*" "$key"
}

cleanup() (
	trap - ERR
	set +e
	sudo ctr tasks kill --signal SIGKILL "$task_id" >/dev/null 2>&1
	for _ in $(seq 1 240); do
		state=$(sudo ctr tasks list | awk -v id="$task_id" '$1 == id {print $3}')
		[[ $state != RUNNING && $state != PAUSED ]] && break
		sleep .25
	done
	sudo ctr tasks rm -f "$task_id" >/dev/null 2>&1
	sudo ctr containers rm "$task_id" >/dev/null 2>&1
	[[ -z ${scratch:-} ]] || sudo rm -rf -- "$scratch"
)
on_exit() {
	local status=$? line=${BASH_LINENO[0]:-unknown}
	trap - EXIT
	if [[ $status -ne 0 ]]; then
		printf 'G4_ROOT_PROVENANCE_QUALIFIER_FAIL line=%s exit_status=%s\n' "$line" "$status" >&2
	fi
	cleanup
	exit "$status"
}
on_error() {
	local status=$1 line=$2
	printf 'G4_ROOT_PROVENANCE_COMMAND_FAIL line=%s exit_status=%s\n' "$line" "$status" >&2
	return "$status"
}
trap 'on_error "$?" "$LINENO"' ERR
trap on_exit EXIT

state_counts() {
	sudo python3 - <<'PY'
import json,os
root=0
live=0
if os.path.exists('/var/lib/mkruntimed/rootfs/state.json'):
    root=len(json.load(open('/var/lib/mkruntimed/rootfs/state.json'))['records'])
if os.path.exists('/var/lib/mkruntimed/storage/state.json'):
    live=sum(x['state'] != 'RELEASED' for x in json.load(open('/var/lib/mkruntimed/storage/state.json'))['exports'].values())
print('rootfs_records=%d live_exports=%d' % (root,live))
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

wait_state() {
	local wanted=$1 observed=
	for _ in $(seq 1 360); do
		observed=$(sudo ctr tasks list | awk -v id="$task_id" '$1 == id {print $3}')
		[[ $observed = "$wanted" ]] && return 0
		sleep .25
	done
	echo "task state is '$observed', wanted '$wanted'" >&2
	return 1
}

inventory() {
	local counts
	counts=$(state_counts)
	printf 'instances=%s ctr_tasks=%s ctr_containers=%s %s helpers=%s links=%s nat_rules=%s filter_rules=%s' 		"$(sudo find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 -type d | wc -l)" 		"$(sudo ctr tasks list -q | wc -l)" "$(sudo ctr containers list -q | wc -l)" "$counts" 		"$( (pgrep -f '^/usr/local/libexec/multikernel/(mkvsock-nbd|mk-agent-relay)' || true) | wc -l)" 		"$(ip -o link show | awk -F': ' '$2 ~ /^mkv[0-9a-f]+$/ {n++} END {print n+0}')" 		"$(sudo iptables -t nat -S POSTROUTING | grep -c '172\.31\.' || true)" 		"$(sudo iptables -S | grep -c '^\(-N\|-A\) MK-' || true)"
}

value() {
	local key=$1
	sed -n "s/^$key=//p" | tail -n 1
}

[[ $(id -u) -ne 0 ]] || { echo 'run as an ordinary sudo-capable user' >&2; exit 1; }
test -d "$source_root/runtime"
test -f "$manifest"
for service in mkruntimed mknetd containerd docker; do
	[[ $(systemctl is-active "$service") = active ]]
	[[ $(systemctl show -p NRestarts --value "$service") = 0 ]]
done
cleanup
scratch=$(mktemp -d -p /var/tmp mk-root-provenance.XXXXXX)
[[ $(wait_counts 'rootfs_records=0 live_exports=0') = 'rootfs_records=0 live_exports=0' ]]
initial=$(inventory)
[[ $initial = 'instances=0 ctr_tasks=0 ctr_containers=0 rootfs_records=0 live_exports=0 helpers=0 links=0 nat_rules=0 filter_rules=0' ]]
observe initial-inventory "$initial"
observe provenance "boot_id=$(cat /proc/sys/kernel/random/boot_id)
selector=$(readlink -f /usr/local/lib/multikernel/current)
mkruntimed_sha256=$(sudo sha256sum /proc/$(systemctl show -p MainPID --value mkruntimed)/exe | awk '{print $1}')
manifest_sha256=$(sha256sum "$manifest" | awk '{print $1}')
qualifier_sha256=$(sha256sum "$source_root/scripts/test-runtime-root-provenance-live.sh" | awk '{print $1}')"

agent_path=$(jq -er .agent.path "$manifest")
agent_sha=$(jq -er .agent.sha256 "$manifest")
relay_path=$(jq -er .relay.path "$manifest")
relay_sha=$(jq -er .relay.sha256 "$manifest")
module_path=$(jq -er .transport.module.path "$manifest")
module_sha=$(jq -er .transport.module.sha256 "$manifest")
initramfs_sha=$(jq -er .initramfs.sha256 "$manifest")
kernel_sha=$(jq -er .kernel.sha256 "$manifest")
[[ $(sha256sum "$agent_path" | awk '{print $1}') = "$agent_sha" ]]
[[ $(sha256sum "$relay_path" | awk '{print $1}') = "$relay_sha" ]]
[[ $(sha256sum "$module_path" | awk '{print $1}') = "$module_sha" ]]
guest_init=/usr/local/libexec/multikernel/guest/mk-agent-init
test -f "$guest_init"
guest_init_sha=$(sha256sum "$guest_init" | awk '{print $1}')
observe approved-bootstrap "agent_path=$agent_path agent_sha256=$agent_sha
relay_path=$relay_path relay_sha256=$relay_sha
module_path=$module_path module_sha256=$module_sha
initramfs_sha256=$initramfs_sha
kernel_sha256=$kernel_sha
guest_init_path=$guest_init guest_init_sha256=$guest_init_sha"

sudo ctr run --detach --runtime "$runtime" "$image" "$task_id" /bin/sh -c 	'printf ready >/tmp/root-provenance-ready; while [ ! -e /tmp/root-provenance-release ]; do sleep 1; done'
wait_state RUNNING
for attempt in $(seq 1 360); do
	if sudo ctr task exec --exec-id "provenance-ready-$attempt" "$task_id" 		/bin/test -e /tmp/root-provenance-ready >/dev/null 2>&1; then
		break
	fi
	sleep .25
done
sudo ctr task exec --exec-id provenance-ready-final "$task_id" /bin/test -e /tmp/root-provenance-ready
[[ $(wait_counts 'rootfs_records=1 live_exports=1') = 'rootfs_records=1 live_exports=1' ]]

identity=$(printf 'default\0%s' "$task_id" | sha256sum | awk '{print "task-" substr($1,1,32)}')
bundle=$(sudo jq -er --arg id "$identity" '.records[$id].request.bundle' /var/lib/mkruntimed/rootfs/state.json)
runtime_dir=$(sudo jq -er --arg id "$identity" '.records[$id].runtime_dir' /var/lib/mkruntimed/rootfs/state.json)
source_manifest=$runtime_dir/initramfs.source-manifest.json
source_manifest_sha=$(sudo sha256sum "$source_manifest" | awk '{print $1}')
source_manifest_recorded=$(sudo jq -er --arg id "$identity" '.records[$id].build_result.source_scan_before.manifest_sha256' /var/lib/mkruntimed/rootfs/state.json)
[[ $source_manifest_sha = "$source_manifest_recorded" ]]
source_busybox=$(sudo jq -cer '.entries[] | select(.path == "bin/busybox" and .type == "regular") | {mode,uid,gid,size,sha256}' "$source_manifest")
source_busybox_sha=$(jq -er .sha256 <<<"$source_busybox")
source_busybox_size=$(jq -er .size <<<"$source_busybox")
[[ -n $source_busybox_sha && $source_busybox_size -gt 0 ]]

image_path=$(sudo python3 - "$identity" <<'PY'
import json,sys
r=json.load(open('/var/lib/mkruntimed/rootfs/state.json'))['records'][sys.argv[1]]
x=[v for v in json.load(open('/var/lib/mkruntimed/storage/state.json'))['exports'].values() if v['state'] != 'RELEASED' and v['path'] == r['storage']['path']]
assert len(x) == 1
print(x[0]['path'])
PY
)
for name in mk-agent mkvsock-relay init oci-busybox; do
	sudo test ! -e "$scratch/$name"
done
sudo debugfs -R "dump -p /mk-agent $scratch/mk-agent" "$image_path" >/dev/null 2>&1
sudo debugfs -R "dump -p /mkvsock-relay $scratch/mkvsock-relay" "$image_path" >/dev/null 2>&1
sudo debugfs -R "dump -p /init $scratch/init" "$image_path" >/dev/null 2>&1
sudo debugfs -R "dump -p /bundle/rootfs/bin/busybox $scratch/oci-busybox" "$image_path" >/dev/null 2>&1
sudo chmod 0600 "$scratch/mk-agent" "$scratch/mkvsock-relay" "$scratch/init" "$scratch/oci-busybox"
image_agent_sha=$(sudo sha256sum "$scratch/mk-agent" | awk '{print $1}')
image_relay_sha=$(sudo sha256sum "$scratch/mkvsock-relay" | awk '{print $1}')
image_init_sha=$(sudo sha256sum "$scratch/init" | awk '{print $1}')
image_busybox_sha=$(sudo sha256sum "$scratch/oci-busybox" | awk '{print $1}')
[[ $image_agent_sha = "$agent_sha" && $image_relay_sha = "$relay_sha" ]]
[[ $image_init_sha = "$guest_init_sha" && $image_busybox_sha = "$source_busybox_sha" ]]
[[ $(sudo stat -c %s "$scratch/oci-busybox") = "$source_busybox_size" ]]
! sudo readelf -l "$scratch/oci-busybox" | grep -q 'Requesting program interpreter'
! sudo readelf -d "$scratch/oci-busybox" 2>/dev/null | grep -q '(NEEDED)'
image_busybox_stat=$(sudo debugfs -R 'stat /bundle/rootfs/bin/busybox' "$image_path" 2>/dev/null | tr '\n' ' ')
image_root_stat=$(sudo debugfs -R 'stat /bundle/rootfs' "$image_path" 2>/dev/null | tr '\n' ' ')
image_agent_stat=$(sudo debugfs -R 'stat /mk-agent' "$image_path" 2>/dev/null | tr '\n' ' ')
observe live-image-provenance "image_path=$image_path
outer_agent_sha256=$image_agent_sha outer_agent_stat=$image_agent_stat
outer_relay_sha256=$image_relay_sha outer_init_sha256=$image_init_sha
outer_oci_busybox_sha256=$image_busybox_sha outer_oci_busybox_stat=$image_busybox_stat
outer_oci_root_stat=$image_root_stat"

guest_probe='
set -eu
outer=/proc/1/root
guest_busybox=/bin/busybox
guest_stat=$(stat -Lc "%d:%i:%f:%s" "$guest_busybox")
guest_sha=$(sha256sum "$guest_busybox" | cut -d" " -f1)
for path in /mk-agent /mkvsock-relay /init /mk_transport.ko /bin/mkvsock-nbd /lib/mk_transport.ko; do test ! -e "$path"; done
set +e
stat "$outer/bundle/rootfs/bin/busybox" >/tmp/proc-root-access.out 2>&1
outer_access_rc=$?
set -e
test "$outer_access_rc" -ne 0
printf "guest_root_stat=%s\n" "$(stat -Lc "%d:%i:%f:%s" /)"
printf "guest_busybox_stat=%s\nguest_busybox_sha256=%s\n" "$guest_stat" "$guest_sha"
printf "outer_proc_root_access_status=%s\n" "$outer_access_rc"
printf "CHILD_MOUNT_TABLE_BEGIN\n"
cat /proc/mounts
printf "CHILD_MOUNT_TABLE_END\n"
printf "nbd0_dev=%s\nnbd0_sectors=%s\n" "$(cat /sys/class/block/nbd0/dev)" "$(cat /sys/class/block/nbd0/size)"
printf "mk_transport_module=%s\n" "$(grep "^mk_transport " /proc/modules | tr " " ":")"
printf "nbd_module=%s\n" "$(grep "^nbd " /proc/modules | tr " " ":")"
'
guest_audit=$(sudo ctr task exec --exec-id provenance-audit "$task_id" /bin/sh -c "$guest_probe")
observe child-provenance "$guest_audit"
grep -Fq CHILD_MOUNT_TABLE_BEGIN <<<"$guest_audit"
grep -Fq CHILD_MOUNT_TABLE_END <<<"$guest_audit"

guest_busybox_sha=$(value guest_busybox_sha256 <<<"$guest_audit")
guest_busybox_stat=$(value guest_busybox_stat <<<"$guest_audit")
guest_root_stat=$(value guest_root_stat <<<"$guest_audit")
outer_access_rc=$(value outer_proc_root_access_status <<<"$guest_audit")
nbd0_dev=$(value nbd0_dev <<<"$guest_audit")
[[ -n $guest_busybox_sha && -n $guest_busybox_stat && -n $guest_root_stat && -n $outer_access_rc ]]
[[ $guest_busybox_sha = "$source_busybox_sha" && $guest_busybox_sha = "$image_busybox_sha" ]]
[[ ${guest_busybox_stat%%:*} = ${guest_root_stat%%:*} ]]
image_busybox_inode=$(sed -n 's/.*Inode: \([0-9][0-9]*\).*/\1/p' <<<"$image_busybox_stat")
image_root_inode=$(sed -n 's/.*Inode: \([0-9][0-9]*\).*/\1/p' <<<"$image_root_stat")
guest_busybox_inode=$(cut -d: -f2 <<<"$guest_busybox_stat")
guest_root_inode=$(cut -d: -f2 <<<"$guest_root_stat")
[[ -n $image_busybox_inode && -n $image_root_inode ]]
[[ $guest_busybox_inode = "$image_busybox_inode" && $guest_root_inode = "$image_root_inode" ]]
[[ $outer_access_rc -ne 0 ]]
root_device=$(cut -d: -f1 <<<"$guest_root_stat")
expected_root_device=$(python3 - "$nbd0_dev" <<'PY'
import os,sys
major,minor=map(int,sys.argv[1].split(':'))
print(os.makedev(major,minor))
PY
)
[[ $root_device = "$expected_root_device" ]]
[[ $(value mk_transport_module <<<"$guest_audit") = mk_transport:* ]]
[[ $(value nbd_module <<<"$guest_audit") = nbd:* ]]
observe provenance-comparison "bundle=$bundle source_busybox=$source_busybox guest_busybox_stat=$guest_busybox_stat ext4_busybox_inode=$image_busybox_inode ext4_oci_root_inode=$image_root_inode elf_interpreter=none dynamic_needed=none source_manifest_sha256=$source_manifest_sha approved_agent_sha256=$agent_sha approved_relay_sha256=$relay_sha approved_module_sha256=$module_sha guest_init_sha256=$guest_init_sha outer_proc_root_access_status=$outer_access_rc exact_match=1"

sudo ctr task exec --exec-id provenance-release "$task_id" /bin/touch /tmp/root-provenance-release
wait_state STOPPED
sudo ctr tasks rm "$task_id" >/dev/null
sudo ctr containers rm "$task_id"
[[ $(wait_counts 'rootfs_records=0 live_exports=0') = 'rootfs_records=0 live_exports=0' ]]
for _ in $(seq 1 240); do
	[[ $(sudo find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 -type d | wc -l) = 0 ]] && break
	sleep .25
done
final=$(inventory)
[[ $final = 'instances=0 ctr_tasks=0 ctr_containers=0 rootfs_records=0 live_exports=0 helpers=0 links=0 nat_rules=0 filter_rules=0' ]]
observe final-inventory "$final"
for service in mkruntimed mknetd containerd docker; do
	[[ $(systemctl is-active "$service") = active ]]
	[[ $(systemctl show -p NRestarts --value "$service") = 0 ]]
done

trap - EXIT
trap - ERR
sudo rm -rf -- "$scratch"
echo G4_ROOT_PROVENANCE_LIVE_PASS
