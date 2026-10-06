#!/usr/bin/env bash
set -euo pipefail

if [[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]]; then
	PS4='+${BASH_SOURCE}:${LINENO}: '
	set -x
fi

task_id=mk-storage-primary-owner
image=${MK_TEST_IMAGE:-docker.io/library/busybox:1.36}
runtime=${MK_RUNTIME:-io.containerd.multikernel.v2}
kerf=${MK_KERF:-/opt/mkruntime/kerf-venv/bin/kerf}
scratch=$(mktemp -d -p /var/tmp mk-storage-primary-owner.XXXXXX)
console_raw=$scratch/console.raw
console_pid=

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

cleanup() (
	set +e
	[[ -z $console_pid ]] || kill "$console_pid" >/dev/null 2>&1 || true
	[[ -z $console_pid ]] || wait "$console_pid" >/dev/null 2>&1 || true
	cleanup_task
	rm -rf -- "$scratch"
)
trap cleanup EXIT

wait_task_state() {
	local expected=$1 state=
	for _ in $(seq 1 240); do
		state=$(sudo ctr tasks list | awk -v id="$task_id" '$1 == id {print $3}')
		[[ $state = "$expected" ]] && return 0
		sleep .25
	done
	return 1
}

live_counts() {
	sudo python3 - <<'PY'
import json, os
def load(path, empty):
    if not os.path.exists(path):
        return empty
    with open(path, encoding='utf-8') as stream:
        return json.load(stream)
rootfs=load('/var/lib/mkruntimed/rootfs/state.json', {'records': {}})
storage=load('/var/lib/mkruntimed/storage/state.json', {'exports': {}})
print('rootfs_records=%d live_exports=%d' % (
    len(rootfs['records']),
    sum(v['state'] != 'RELEASED' for v in storage['exports'].values())))
PY
}

wait_counts() {
	local expected=$1 observed=
	for _ in $(seq 1 240); do
		observed=$(live_counts)
		[[ $observed = "$expected" ]] && { printf '%s' "$observed"; return 0; }
		sleep .25
	done
	printf '%s' "$observed"
	return 1
}

primary_storage_inventory() {
	sudo bash -c '
set -euo pipefail
export LC_ALL=C
echo BLOCK_DEVICES
lsblk -bdn -e7 -o KNAME,MAJ:MIN,SIZE,TYPE,MODEL,SERIAL,HCTL | sort
echo BLOCK_ANCESTRY
for path in /sys/class/block/sd*/device; do
  test -e "$path"
  printf "%s=%s\n" "${path#/sys/class/block/}" "$(readlink -f "$path")"
done | sort
echo STORAGE_CONTROLLERS
lspci -Dnn | grep -Ei "storage|scsi|sata|nvme" | sort
echo PRIMARY_MOUNTS
findmnt -rn -S /dev/sda1 -o TARGET,SOURCE,FSTYPE,OPTIONS
findmnt -rn -S /dev/sdb -o TARGET,SOURCE,FSTYPE,OPTIONS
'
}

active_summary() {
	sudo python3 - <<'PY'
import json, os, re
with open('/var/lib/mkruntimed/rootfs/state.json', encoding='utf-8') as stream:
    rootfs=json.load(stream)
with open('/var/lib/mkruntimed/storage/state.json', encoding='utf-8') as stream:
    storage=json.load(stream)
assert rootfs['version'] == 4 and len(rootfs['records']) == 1, rootfs
active=[v for v in storage['exports'].values() if v['state'] == 'ACTIVE']
assert len(active) == 1, active
value=active[0]
for key in ('sandbox_generation','export_generation'):
    assert re.fullmatch(r'[a-f0-9]{32}', value[key]), (key,value[key])
assert value['sandbox_generation'] != value['export_generation'], value
assert value['size_bytes'] == value['quota_bytes'], value
assert value['inode_limit'] >= 128, value
st=os.stat(value['path'])
assert st.st_size == value['quota_bytes'], (st.st_size,value)
assert st.st_blocks * 512 >= value['quota_bytes'], (st.st_blocks,value)
vfs=os.statvfs(os.path.dirname(value['path']))
free_bytes=vfs.f_bavail*vfs.f_frsize
free_inodes=vfs.f_favail
assert free_bytes >= 1073741824 and free_inodes >= 1024, (free_bytes,free_inodes)
print(json.dumps({
  'sandbox_id':value['sandbox_id'],
  'sandbox_generation':value['sandbox_generation'],
  'export_generation':value['export_generation'],
  'image_path':value['path'],
  'image_identity':value['image_identity'],
  'image_size_bytes':st.st_size,
  'allocated_bytes':st.st_blocks*512,
  'quota_bytes':value['quota_bytes'],
  'inode_limit':value['inode_limit'],
  'filesystem_uuid':value['filesystem_uuid'],
  'storage_free_bytes_after_allocation':free_bytes,
  'storage_free_inodes_after_allocation':free_inodes,
  'production_min_free_bytes':1073741824,
  'production_min_free_inodes':1024,
},sort_keys=True,separators=(',',':')))
PY
}

released_summary() {
	sudo python3 - "$1" "$2" "$3" <<'PY'
import json,re,sys
sandbox_id,sandbox_generation,export_generation=sys.argv[1:]
key=sandbox_id+'\0'+sandbox_generation
with open('/var/lib/mkruntimed/rootfs/state.json', encoding='utf-8') as stream:
    rootfs=json.load(stream)
with open('/var/lib/mkruntimed/storage/state.json', encoding='utf-8') as stream:
    storage=json.load(stream)
value=storage['exports'][key]
assert not rootfs['records'], rootfs
assert sum(v['state'] != 'RELEASED' for v in storage['exports'].values()) == 0, storage
assert value['state'] == 'RELEASED', value
assert value['export_generation'] == export_generation, value
assert re.fullmatch(r'e2fsck-clean-sha256:[0-9a-f]{64}',value['offline_check']), value
c=value['counters']
assert c['reads'] > 0 and c['read_bytes'] > 0, c
assert c['writes'] > 0 and c['written_bytes'] > 0 and c['flushes'] > 0, c
print(json.dumps({
  'sandbox_id':value['sandbox_id'],
  'sandbox_generation':value['sandbox_generation'],
  'export_generation':value['export_generation'],
  'state':value['state'],
  'counters':c,
  'offline_check':value['offline_check'],
  'rootfs_records':0,
  'live_exports':0,
},sort_keys=True,separators=(',',':')))
PY
}

[[ $(id -u) -ne 0 ]] || { echo 'run as an ordinary sudo-capable user' >&2; exit 1; }
test -x "$kerf"
umask 077
cleanup_task
observe clean-before "$(wait_counts 'rootfs_records=0 live_exports=0')"
observe provenance "boot_id=$(cat /proc/sys/kernel/random/boot_id)
selector=$(readlink -f /usr/local/lib/multikernel/current)
mkruntimed_sha256=$(sudo sha256sum /proc/$(systemctl show -p MainPID --value mkruntimed)/exe | awk '{print $1}')
qualifier_sha256=$(sha256sum "$0" | awk '{print $1}')"

before_inventory=$(primary_storage_inventory)
grep -Fq 'persistent-disk-0' <<<"$before_inventory"
grep -Fq 'mk-mediated-storage-20260830' <<<"$before_inventory"
grep -Eq '^sda/device=.*/0000:00:03\.0/virtio0/' <<<"$before_inventory"
grep -Eq '^sdb/device=.*/0000:00:03\.0/virtio0/' <<<"$before_inventory"
grep -Fq '0000:00:03.0' <<<"$before_inventory"
grep -Eq '^/ /dev/sda1 ext4 ' <<<"$before_inventory"
grep -Eq '^/srv/multikernel-storage /dev/sdb ext4 ' <<<"$before_inventory"
observe primary-storage-before "$before_inventory"

guest_program='set -eu; dd if=/dev/zero of=/tmp/ownership-evidence bs=4096 count=64 status=none; sync; dd if=/tmp/ownership-evidence of=/dev/null bs=4096 status=none; touch /tmp/ownership-ready; while [ ! -e /tmp/ownership-release ]; do sleep 1; done'
sudo ctr run --detach --runtime "$runtime" "$image" "$task_id" /bin/sh -c "$guest_program"
wait_task_state RUNNING
for _ in $(seq 1 120); do
	sudo ctr task exec --exec-id "ownership-ready-$RANDOM" "$task_id" /bin/test -e /tmp/ownership-ready >/dev/null 2>&1 && break
	sleep .25
done
sudo ctr task exec --exec-id ownership-ready-final "$task_id" /bin/test -e /tmp/ownership-ready
wait_counts 'rootfs_records=1 live_exports=1' >/dev/null

during_inventory=$(primary_storage_inventory)
[[ $during_inventory = "$before_inventory" ]]
observe primary-storage-during "$during_inventory"

summary=$(active_summary)
observe backing-allocation-owner-quota "$summary"
sandbox_id=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["sandbox_id"])' "$summary")
sandbox_generation=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["sandbox_generation"])' "$summary")
export_generation=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["export_generation"])' "$summary")

child_probe='set -eu
test -e /sys/class/block/nbd0
test ! -e /sys/class/block/sda
test ! -e /sys/class/block/sdb
test ! -e /sys/bus/pci/devices/0000:00:03.0
printf "CHILD_BLOCK_TABLE_BEGIN\n"
for path in /sys/class/block/*; do printf "%s dev=%s sectors=%s\n" "${path##*/}" "$(cat "$path/dev")" "$(cat "$path/size")"; done
printf "CHILD_BLOCK_TABLE_END\n"
printf "CHILD_STORAGE_PCI_BEGIN\n"
for path in /sys/bus/pci/devices/*; do
  test -e "$path/class" || continue
  class=$(cat "$path/class")
  case "$class" in 0x01*) printf "%s class=%s\n" "${path##*/}" "$class";; esac
done
printf "CHILD_STORAGE_PCI_END\n"
printf "CHILD_MOUNT_TABLE_BEGIN\n"
cat /proc/mounts
printf "CHILD_MOUNT_TABLE_END\n"'
child_inventory=$(sudo ctr task exec --exec-id ownership-child-inventory "$task_id" /bin/sh -c "$child_probe")
grep -Fq 'nbd0 dev=' <<<"$child_inventory"
! grep -Eq '^(sda|sdb) dev=' <<<"$child_inventory"
[[ $(sed -n '/CHILD_STORAGE_PCI_BEGIN/,/CHILD_STORAGE_PCI_END/{/CHILD_STORAGE_PCI_/d;p}' <<<"$child_inventory") = '' ]]
observe child-virtual-storage-and-mounts "$child_inventory"

sudo timeout --signal=TERM 120s script -qefc "$kerf console $sandbox_id --verbose" /dev/null >"$console_raw" 2>&1 &
console_pid=$!
sleep 1
kill -0 "$console_pid"
sudo ctr task exec --exec-id ownership-release "$task_id" /bin/touch /tmp/ownership-release
wait_task_state STOPPED
sudo ctr tasks rm "$task_id" >/dev/null
sudo ctr containers rm "$task_id"

for _ in $(seq 1 240); do
	grep -aFq 'MK_STORAGE_ROOT_QUIESCE_PASS stages=sync,remount-ro,sync device=/dev/nbd0' "$console_raw" &&
		grep -aFq 'MK_STORAGE_NBD_DISCONNECT_PASS device=/dev/nbd0' "$console_raw" && break
	sleep .25
done
quiesce_line=$(grep -anF 'MK_STORAGE_ROOT_QUIESCE_PASS stages=sync,remount-ro,sync device=/dev/nbd0' "$console_raw" | tail -1 | cut -d: -f1)
disconnect_line=$(grep -anF 'MK_STORAGE_NBD_DISCONNECT_PASS device=/dev/nbd0' "$console_raw" | tail -1 | cut -d: -f1)
[[ $quiesce_line =~ ^[0-9]+$ && $disconnect_line =~ ^[0-9]+$ && $quiesce_line -lt $disconnect_line ]]
observe guest-teardown-order "quiesce_line=$quiesce_line disconnect_line=$disconnect_line stages=sync,remount-ro,sync,nbd-disconnect"
kill "$console_pid" >/dev/null 2>&1 || true
wait "$console_pid" >/dev/null 2>&1 || true
console_pid=

observe durable-release-counters-offline-check "$(released_summary "$sandbox_id" "$sandbox_generation" "$export_generation")"
observe clean-after-release "$(wait_counts 'rootfs_records=0 live_exports=0')"

after_inventory=$(primary_storage_inventory)
[[ $after_inventory = "$before_inventory" ]]
observe primary-storage-after "$after_inventory"

sudo systemctl restart mkruntimed
for _ in $(seq 1 60); do systemctl is-active --quiet mkruntimed && break; sleep 1; done
[[ $(systemctl is-active mkruntimed) = active ]]
[[ $(systemctl show -p NRestarts --value mkruntimed) = 0 ]]
kerf_final=$(sudo "$kerf" show --verbose)
grep -Fq 'No instances found' <<<"$kerf_final"
if grep -Fq 'No memory pool configured' <<<"$kerf_final"; then
	pool_state=unconfigured
else
	grep -Fq 'Pool Allocated:  0.00 GB (0 bytes)' <<<"$kerf_final"
	pool_state=configured-zero
fi
for service in mkruntimed mknetd containerd docker; do
	[[ $(systemctl is-active "$service") = active ]]
	[[ $(systemctl show -p NRestarts --value "$service") = 0 ]]
done
observe final-primary-ownership "physical_inventory_unchanged=1 child_physical_disks=0 child_storage_pci=0 pool_state=$pool_state pool_allocated_bytes=0 instances=0 services_healthy=4"

trap - EXIT
rm -rf -- "$scratch"
echo G4_STORAGE_PRIMARY_OWNERSHIP_LIVE_PASS
