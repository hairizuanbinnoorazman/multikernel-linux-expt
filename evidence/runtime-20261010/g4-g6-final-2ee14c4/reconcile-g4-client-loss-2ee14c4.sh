#!/usr/bin/env bash
set -Eeuo pipefail
set -x

source_root=/var/tmp/mksrc-2ee14c4
boot=772c6339-6ed7-494b-8d7f-71f3cb7394eb
sandbox_id=mk-mk-storage-client-loss-216e4611aa588b7e
sandbox_generation=c5106654766ab44b1af0bc9b1ab7a7f0
export_generation=6a6e1c0a7de7fa9472d800dcacdda8ac
endpoint_generation=f5758a5bbfd04c4dfeebd0f224eb7280
task_key=task-216e4611aa588b7e8436be9484e3f8cd
container_id=mk-storage-client-loss
disk_dir=/srv/multikernel-storage/runtime/task-216e4611aa588b7e8436be9484e3f8cd
recovery=/var/lib/multikernel-recovery/g4-client-loss-772c6339-2ee14c4
disk_recovery=/srv/multikernel-storage/quarantine/g4-client-loss-772c6339-2ee14c4
kerf=/opt/mkruntime/kerf-venv/bin/kerf

test "$(id -u)" -ne 0
test "$(cat /proc/sys/kernel/random/boot_id)" = "$boot"
test "$(readlink -f /usr/local/lib/multikernel/current)" = /usr/local/lib/multikernel/releases/0.1.0-dev-2ee14c49a6dd674629ba18a0ca618c1870311ca1
sudo test ! -e "$recovery"
sudo test ! -e "$disk_recovery"
sudo test -f "$disk_dir/root.ext4"
test "$(sudo stat -c %s "$disk_dir/root.ext4")" = 2147483648
test "$(sudo ctr tasks list | awk -v id="$container_id" '$1==id {print $3}')" = STOPPED
test "$(sudo ctr containers list | awk -v id="$container_id" '$1==id {print $1}')" = "$container_id"
sudo test -d "/sys/fs/multikernel/instances/$sandbox_id"
test "$(sudo cat "/sys/fs/multikernel/instances/$sandbox_id/status")" = active
test "$(sudo jq '.endpoints|length' /var/lib/mknetd/state.json)" = 1

sudo python3 - "$sandbox_id" "$sandbox_generation" "$export_generation" "$endpoint_generation" "$task_key" "$container_id" <<'PY'
import json,sys
sandbox_id,generation,export_generation,endpoint_generation,task_key,container_id=sys.argv[1:]
lifecycle=json.load(open('/var/lib/mkruntime/state.json'))
assert list(lifecycle['sandboxes']) == [sandbox_id], lifecycle['sandboxes']
sandbox=lifecycle['sandboxes'][sandbox_id]
assert sandbox['generation'] == generation and sandbox['state'] == 'RUNNING', sandbox
rootfs=json.load(open('/var/lib/mkruntimed/rootfs/state.json'))
assert list(rootfs['records']) == [task_key], rootfs['records']
assert rootfs['records'][task_key]['phase'] == 'PREPARED', rootfs['records'][task_key]
storage=json.load(open('/var/lib/mkruntimed/storage/state.json'))
key=sandbox_id+'\0'+generation
live=[k for k,v in storage['exports'].items() if v['state'] != 'RELEASED']
assert live == [key], live
assert storage['exports'][key]['export_generation'] == export_generation, storage['exports'][key]
assert storage['exports'][key]['path'].endswith('/'+task_key+'/root.ext4'), storage['exports'][key]
network=json.load(open('/var/lib/mknetd/state.json'))
assert len(network['endpoints']) == 1, network['endpoints']
endpoint=next(iter(network['endpoints'].values()))
assert endpoint['container_id'] == container_id, endpoint
assert endpoint['sandbox_id'] == sandbox_id and endpoint['sandbox_generation'] == generation, endpoint
assert endpoint['generation'] == endpoint_generation and endpoint['state'] == 'READY', endpoint
assert endpoint['primary_boot_id'] == open('/proc/sys/kernel/random/boot_id').read().strip(), endpoint
print('G4_CLIENT_LOSS_EXACT_PRECONDITIONS_PASS')
PY

sudo systemctl stop mkruntimed.service
test "$(systemctl show mkruntimed.service -p ActiveState --value)" = inactive
test "$(systemctl show mkruntimed.service -p MainPID --value)" = 0

sudo install -d -m 0700 -o root -g root /var/lib/multikernel-recovery "$recovery"
sudo install -d -m 0700 -o root -g root /srv/multikernel-storage/quarantine "$disk_recovery"
sudo cp --preserve=mode,ownership,timestamps /var/lib/mkruntime/state.json "$recovery/lifecycle-state.before.json"
sudo cp --preserve=mode,ownership,timestamps /var/lib/mkruntime/journal.jsonl "$recovery/lifecycle-journal.before.jsonl"
sudo cp --preserve=mode,ownership,timestamps /var/lib/mkruntimed/rootfs/state.json "$recovery/rootfs-state.before.json"
sudo cp --preserve=mode,ownership,timestamps /var/lib/mkruntimed/storage/state.json "$recovery/storage-state.before.json"
sudo cp --preserve=mode,ownership,timestamps /var/lib/mknetd/state.json "$recovery/network-state.before.json"
sudo sha256sum \
  "$recovery/lifecycle-state.before.json" \
  "$recovery/lifecycle-journal.before.jsonl" \
  "$recovery/rootfs-state.before.json" \
  "$recovery/storage-state.before.json" \
  "$recovery/network-state.before.json" | sudo tee "$recovery/SHA256SUMS" >/dev/null
sudo chmod 0600 "$recovery/SHA256SUMS"

sudo "$kerf" kill "$sandbox_id" --force
test "$(sudo cat "/sys/fs/multikernel/instances/$sandbox_id/status")" = loaded
sudo "$kerf" unload "$sandbox_id"
sudo "$kerf" delete "$sandbox_id"
sudo test ! -e "/sys/fs/multikernel/instances/$sandbox_id"
sudo mv "$disk_dir" "$disk_recovery/$task_key"

sudo python3 - "$sandbox_id" "$sandbox_generation" "$export_generation" "$task_key" <<'PY'
import json,os,sys,tempfile
sandbox_id,generation,export_generation,task_key=sys.argv[1:]
def rewrite(path, mutate):
    with open(path,'rb') as stream: value=json.load(stream)
    mutate(value)
    parent=os.path.dirname(path)
    fd,tmp=tempfile.mkstemp(prefix='.operator-reconcile-',dir=parent)
    try:
        os.fchmod(fd,0o600); os.fchown(fd,0,0)
        with os.fdopen(fd,'w',encoding='utf-8') as stream:
            json.dump(value,stream,sort_keys=True,indent=2,separators=(',',': '))
            stream.write('\n'); stream.flush(); os.fsync(stream.fileno())
        os.replace(tmp,path)
        directory=os.open(parent,os.O_RDONLY|os.O_DIRECTORY)
        try: os.fsync(directory)
        finally: os.close(directory)
    finally:
        if os.path.exists(tmp): os.unlink(tmp)
def lifecycle(value):
    sandboxes=value['sandboxes']; assert list(sandboxes)==[sandbox_id],sandboxes
    item=sandboxes[sandbox_id]
    assert item['generation']==generation and item['state']=='RUNNING',item
    del sandboxes[sandbox_id]
def rootfs(value):
    records=value['records']; assert list(records)==[task_key],records
    assert records[task_key]['phase']=='PREPARED',records[task_key]
    del records[task_key]
def storage(value):
    key=sandbox_id+'\0'+generation
    exports=value['exports']; live=[k for k,v in exports.items() if v['state']!='RELEASED']
    assert live==[key],live
    assert exports[key]['export_generation']==export_generation,exports[key]
    del exports[key]
rewrite('/var/lib/mkruntime/state.json',lifecycle)
rewrite('/var/lib/mkruntimed/rootfs/state.json',rootfs)
rewrite('/var/lib/mkruntimed/storage/state.json',storage)
PY

sudo test ! -e "$disk_dir"
sudo test -f "$disk_recovery/$task_key/root.ext4"
test "$(sudo jq '.sandboxes|length' /var/lib/mkruntime/state.json)" = 0
test "$(sudo jq '.records|length' /var/lib/mkruntimed/rootfs/state.json)" = 0
test "$(sudo jq '[.exports[]|select(.state!="RELEASED")]|length' /var/lib/mkruntimed/storage/state.json)" = 0

sudo systemctl reset-failed mkruntimed.service
sudo systemctl start mkruntimed.service
for _ in $(seq 1 240); do
  sudo test -S /run/mkruntimed.sock && break
  sleep .25
done
sudo test -S /run/mkruntimed.sock
sudo timeout 60 ctr tasks rm -f "$container_id"
sudo ctr containers rm "$container_id"
for _ in $(seq 1 240); do
  test -z "$(sudo ctr tasks list | awk -v id="$container_id" '$1==id {print $1}')" && \
    test -z "$(sudo ctr containers list | awk -v id="$container_id" '$1==id {print $1}')" && \
    test "$(ps -eo comm= | awk '$1=="containerd-shim" {n++} END {print n+0}')" = 0 && break
  sleep .25
done
test -z "$(sudo ctr tasks list | awk -v id="$container_id" '$1==id {print $1}')"
test -z "$(sudo ctr containers list | awk -v id="$container_id" '$1==id {print $1}')"

sudo systemctl restart mkruntimed.service
for _ in $(seq 1 240); do
  sudo test -S /run/mkruntimed.sock && break
  sleep .25
done
sudo test -S /run/mkruntimed.sock
pid=$(systemctl show mkruntimed.service -p MainPID --value)
restarts=$(systemctl show mkruntimed.service -p NRestarts --value)
test "$pid" -gt 0
test "$restarts" = 0
sleep 3
test "$(systemctl show mkruntimed.service -p MainPID --value)" = "$pid"
test "$(systemctl show mkruntimed.service -p NRestarts --value)" = 0

echo 'OBSERVATION_BEGIN key=g4-client-loss-operator-reconcile'
printf 'boot=%s service_pid=%s restarts=%s recovery=%s disk_recovery=%s\n' "$boot" "$pid" "$restarts" "$recovery" "$disk_recovery"
sudo sha256sum \
  "$recovery/lifecycle-state.before.json" \
  "$recovery/lifecycle-journal.before.jsonl" \
  "$recovery/rootfs-state.before.json" \
  "$recovery/storage-state.before.json" \
  "$recovery/network-state.before.json" \
  "$recovery/SHA256SUMS" \
  "$disk_recovery/$task_key/root.ext4"
sudo jq -c '{sequence, sandboxes:(.sandboxes|length), results:(.results|length)}' /var/lib/mkruntime/state.json
sudo jq -c '{records:(.records|length)}' /var/lib/mkruntimed/rootfs/state.json
sudo jq -c '{exports:(.exports|length),live:([.exports[]|select(.state!="RELEASED")]|length)}' /var/lib/mkruntimed/storage/state.json
systemctl show mknetd.service mkruntimed.service containerd.service docker.service -p Id -p ActiveState -p SubState -p MainPID -p NRestarts -p Result -p ExecMainStatus
echo 'OBSERVATION_END key=g4-client-loss-operator-reconcile'
bash "$source_root/scripts/audit-runtime-final-resources-live.sh" "$source_root"
echo G4_ACCEPTED_CLIENT_OPERATOR_RECONCILE_PASS
