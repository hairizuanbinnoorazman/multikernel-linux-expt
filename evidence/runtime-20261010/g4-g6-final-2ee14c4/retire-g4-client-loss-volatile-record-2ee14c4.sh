#!/usr/bin/env bash
set -Eeuo pipefail
set -x

source_root=/var/tmp/mksrc-2ee14c4
boot=772c6339-6ed7-494b-8d7f-71f3cb7394eb
record=/run/mkstorage/mk-mk-storage-client-loss-216e4611aa588b7e-c5106654766a.json
log=/run/mkstorage/mk-mk-storage-client-loss-216e4611aa588b7e-c5106654766a.log
recovery=/var/lib/multikernel-recovery/g4-client-loss-772c6339-2ee14c4

test "$(id -u)" -ne 0
test "$(cat /proc/sys/kernel/random/boot_id)" = "$boot"
sudo sha256sum -c "$recovery/SHA256SUMS"
sudo test -f "$record"
sudo test -f "$log"
sudo python3 - "$record" <<'PY'
import json,os,sys
record=json.load(open(sys.argv[1]))
assert record == {
 'version':3,
 'pid':27158,
 'start_time':87223,
 'path':'/srv/multikernel-storage/runtime/task-216e4611aa588b7e8436be9484e3f8cd/root.ext4',
 'port':4061,
 'image_id':'root-65714ed0553a3b57aad3a59757bfbd10',
 'export_generation':'6a6e1c0a7de7fa9472d800dcacdda8ac',
 'image_device':2064,
 'image_inode':655421,
 'binary_device':2049,
 'binary_inode':6079391,
}, record
assert not os.path.exists('/proc/27158'), 'recorded server PID unexpectedly exists'
print('G4_CLIENT_LOSS_VOLATILE_RECORD_PRECONDITIONS_PASS')
PY
sudo grep -Fxq MKNBD_SERVER_CLIENT_ACCEPTED "$log"
if sudo grep -Fq MKNBD_SERVER_CLOSED "$log"; then
  echo 'dead accepted-client log contains false terminal close' >&2
  exit 1
fi

sudo systemctl stop mkruntimed.service
sudo cp --preserve=mode,ownership,timestamps "$record" "$recovery/storage-server-record.before.json"
sudo cp --preserve=mode,ownership,timestamps "$log" "$recovery/storage-server-log.before.log"
sudo sha256sum "$recovery/storage-server-record.before.json" "$recovery/storage-server-log.before.log" | sudo tee "$recovery/STORAGE_SERVER_SHA256SUMS" >/dev/null
sudo chmod 0600 "$recovery/STORAGE_SERVER_SHA256SUMS"
test "$(sudo sha256sum "$record" | awk '{print $1}')" = "$(sudo sha256sum "$recovery/storage-server-record.before.json" | awk '{print $1}')"
test "$(sudo sha256sum "$log" | awk '{print $1}')" = "$(sudo sha256sum "$recovery/storage-server-log.before.log" | awk '{print $1}')"
sudo unlink "$record"
sudo unlink "$log"
sudo test ! -e "$record"
sudo test ! -e "$log"

sudo systemctl reset-failed mkruntimed.service
sudo systemctl start mkruntimed.service
for _ in $(seq 1 240); do
  sudo test -S /run/mkruntimed.sock && break
  sleep .25
done
sudo test -S /run/mkruntimed.sock
pid=$(systemctl show mkruntimed.service -p MainPID --value)
test "$pid" -gt 0
test "$(systemctl show mkruntimed.service -p NRestarts --value)" = 0
sleep 3
test "$(systemctl show mkruntimed.service -p MainPID --value)" = "$pid"

echo 'OBSERVATION_BEGIN key=g4-client-loss-volatile-record-retired'
sudo sha256sum -c "$recovery/SHA256SUMS"
sudo sha256sum -c "$recovery/STORAGE_SERVER_SHA256SUMS"
echo 'OBSERVATION_END key=g4-client-loss-volatile-record-retired'
bash "$source_root/scripts/audit-runtime-final-resources-live.sh" "$source_root"
echo G4_CLIENT_LOSS_VOLATILE_RECORD_RETIRE_PASS
