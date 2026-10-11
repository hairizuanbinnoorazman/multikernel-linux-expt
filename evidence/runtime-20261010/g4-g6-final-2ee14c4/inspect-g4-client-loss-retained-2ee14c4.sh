#!/usr/bin/env bash
set -euo pipefail
set -x

cat /proc/sys/kernel/random/boot_id
readlink -f /usr/local/lib/multikernel/current
systemctl show mkruntimed.service -p ActiveState -p SubState -p Result -p MainPID -p NRestarts --no-pager
sudo ctr tasks list
sudo ctr containers list
sudo find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 -printf '%f\n' | sort
sudo find /srv/multikernel-storage/runtime -mindepth 1 -maxdepth 2 -printf '%y %m %u:%g %s %p\n' | sort
for path in /var/lib/mkruntime/state.json /var/lib/mkruntime/journal.json \
  /var/lib/mkruntimed/rootfs/state.json /var/lib/mkruntimed/storage/state.json; do
  if sudo test -f "$path"; then
    sudo sha256sum "$path"
  else
    echo "absent $path"
  fi
done
sudo python3 - <<'PY'
import os
import json
paths = (
    '/var/lib/mkruntime/state.json',
    '/var/lib/mkruntime/journal.json',
    '/var/lib/mkruntimed/rootfs/state.json',
    '/var/lib/mkruntimed/storage/state.json',
)
for path in paths:
    if not os.path.isfile(path):
        print('STATE_FILE_ABSENT', path)
        continue
    value=json.load(open(path))
    print('STATE_FILE_BEGIN', path)
    print(json.dumps(value, sort_keys=True, separators=(',', ':')))
    print('STATE_FILE_END', path)
PY
ps -eo pid=,ppid=,stat=,comm=,args= | awk '$4 ~ /^(mkruntimed|containerd-shim|mkvsock-nbd|mk-agent-relay)$/ {print}'
sudo /opt/mkruntime/kerf-venv/bin/kerf show --verbose
