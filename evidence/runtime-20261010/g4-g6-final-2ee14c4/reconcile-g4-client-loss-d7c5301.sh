#!/usr/bin/env bash
set -euxo pipefail

source_root=/var/tmp/mksrc-d7c5301
boot=ca1337df-2163-4621-a931-be59076b2683
sandbox_id=mk-mk-storage-client-loss-216e4611aa588b7e
generation=2c602eaec39a361b3e1066210b261b68
export_generation=a061f9b735093774fcf3ee898ae001f5
task_key=task-216e4611aa588b7e8436be9484e3f8cd
container_id=mk-storage-client-loss
disk_dir=/srv/multikernel-storage/runtime/${task_key}
record=/run/mkstorage/mk-mk-storage-client-loss-216e4611aa588b7e-2c602eaec39a.json
server_log=/run/mkstorage/mk-mk-storage-client-loss-216e4611aa588b7e-2c602eaec39a.log
recovery=/var/lib/multikernel-recovery/g4-client-loss-ca1337df-d7c5301
disk_recovery=/srv/multikernel-storage/quarantine/g4-client-loss-ca1337df-d7c5301
kerf=/opt/mkruntime/kerf-venv/bin/kerf

test "$(cat /proc/sys/kernel/random/boot_id)" = "$boot"
test "$(readlink -f /usr/local/lib/multikernel/current)" = /usr/local/lib/multikernel/releases/0.1.0-dev-d7c530103b43c63d97518b0d6f5aa5442e7b15a3
sudo test ! -e "$recovery"
sudo test ! -e "$disk_recovery"
sudo test -f "$disk_dir/root.ext4"
test "$(sudo ctr tasks list | awk -v id="$container_id" '$1==id {print $3}')" = STOPPED
sudo test -f "$record"
sudo test -f "$server_log"
sudo grep -Fxq MKNBD_SERVER_CLIENT_ACCEPTED "$server_log"
if sudo grep -Fq MKNBD_SERVER_CLOSED "$server_log"; then
  echo 'accepted-client record contains an invalid terminal close' >&2
  exit 1
fi

sudo python3 - "$sandbox_id" "$generation" "$export_generation" "$task_key" <<'PY'
import json,sys
sandbox_id,generation,export_generation,task_key=sys.argv[1:]
life=json.load(open('/var/lib/mkruntime/state.json'))
assert list(life['sandboxes']) == [sandbox_id]
assert life['sandboxes'][sandbox_id]['generation'] == generation
assert life['sandboxes'][sandbox_id]['state'] == 'RUNNING'
root=json.load(open('/var/lib/mkruntimed/rootfs/state.json'))
assert list(root['records']) == [task_key]
assert root['records'][task_key]['phase'] == 'PREPARED'
storage=json.load(open('/var/lib/mkruntimed/storage/state.json'))
key=sandbox_id+'\0'+generation
live=[k for k,v in storage['exports'].items() if v['state'] != 'RELEASED']
assert live == [key]
assert storage['exports'][key]['export_generation'] == export_generation
print('G4_D7_CLIENT_LOSS_RECONCILE_PRECONDITIONS_PASS')
PY

sudo systemctl stop mkruntimed.service
sudo systemctl reset-failed mkruntimed.service
sudo install -d -m 0700 -o root -g root /var/lib/multikernel-recovery "$recovery"
sudo install -d -m 0700 -o root -g root /srv/multikernel-storage/quarantine "$disk_recovery"
for item in \
  /var/lib/mkruntime/state.json \
  /var/lib/mkruntime/journal.jsonl \
  /var/lib/mkruntimed/rootfs/state.json \
  /var/lib/mkruntimed/storage/state.json \
  /var/lib/mknetd/state.json \
  "$record" "$server_log"; do
  sudo cp --preserve=mode,ownership,timestamps "$item" "$recovery/"
done
sudo find "$recovery" -maxdepth 1 -type f -print0 | sudo sort -z | sudo xargs -0 sha256sum | sudo tee "$recovery/SHA256SUMS" >/dev/null
sudo chmod 0600 "$recovery/SHA256SUMS"

sudo "$kerf" kill "$sandbox_id" --force
sudo "$kerf" unload "$sandbox_id"
sudo "$kerf" delete "$sandbox_id"
sudo test ! -e "/sys/fs/multikernel/instances/$sandbox_id"
sudo mv "$disk_dir" "$disk_recovery/$task_key"

sudo python3 - "$sandbox_id" "$generation" "$export_generation" "$task_key" <<'PY'
import json,os,sys,tempfile
sandbox_id,generation,export_generation,task_key=sys.argv[1:]
def rewrite(path, mutate):
    with open(path,encoding='utf-8') as stream: value=json.load(stream)
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
    assert list(value['sandboxes']) == [sandbox_id]
    item=value['sandboxes'][sandbox_id]
    assert item['generation'] == generation and item['state'] == 'RUNNING'
    del value['sandboxes'][sandbox_id]
def rootfs(value):
    assert list(value['records']) == [task_key]
    assert value['records'][task_key]['phase'] == 'PREPARED'
    del value['records'][task_key]
def storage(value):
    key=sandbox_id+'\0'+generation
    live=[k for k,v in value['exports'].items() if v['state'] != 'RELEASED']
    assert live == [key]
    assert value['exports'][key]['export_generation'] == export_generation
    del value['exports'][key]
rewrite('/var/lib/mkruntime/state.json',lifecycle)
rewrite('/var/lib/mkruntimed/rootfs/state.json',rootfs)
rewrite('/var/lib/mkruntimed/storage/state.json',storage)
PY

sudo unlink "$record"
sudo unlink "$server_log"
sudo systemctl start mkruntimed.service
for _ in $(seq 1 240); do sudo test -S /run/mkruntimed.sock && break; sleep .25; done
sudo test -S /run/mkruntimed.sock
sudo timeout 60 ctr tasks rm -f "$container_id"
sudo ctr containers rm "$container_id"
for _ in $(seq 1 240); do
  test -z "$(sudo ctr tasks list -q | grep -Fx "$container_id" || true)" && \
    test -z "$(sudo ctr containers list -q | grep -Fx "$container_id" || true)" && \
    test "$(ps -eo comm= | awk '$1=="containerd-shim" {n++} END {print n+0}')" = 0 && break
  sleep .25
done
sudo systemctl restart mkruntimed.service
for _ in $(seq 1 240); do sudo test -S /run/mkruntimed.sock && break; sleep .25; done
sudo test -S /run/mkruntimed.sock
MK_EVIDENCE_XTRACE=0 bash "$source_root/scripts/audit-runtime-final-resources-live.sh" "$source_root"
echo 'OBSERVATION_BEGIN key=g4-d7-client-loss-operator-reconcile'
printf 'boot=%s recovery=%s disk_recovery=%s\n' "$boot" "$recovery" "$disk_recovery"
sudo sha256sum "$recovery/SHA256SUMS" "$disk_recovery/$task_key/root.ext4"
echo 'OBSERVATION_END key=g4-d7-client-loss-operator-reconcile'
echo G4_D7_CLIENT_LOSS_OPERATOR_RECONCILE_PASS
