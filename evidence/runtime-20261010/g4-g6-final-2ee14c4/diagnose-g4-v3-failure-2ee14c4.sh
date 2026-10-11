#!/usr/bin/env bash
set -euo pipefail
set -x

source_root=/var/tmp/mksrc-2ee14c4
date --utc --iso-8601=seconds
cat /proc/sys/kernel/random/boot_id
readlink -f /usr/local/lib/multikernel/current
systemctl show mkruntimed.service \
  -p Id -p LoadState -p ActiveState -p SubState -p Result -p ExecMainCode \
  -p ExecMainStatus -p MainPID -p NRestarts -p ActiveEnterTimestamp \
  -p InactiveEnterTimestamp --no-pager
sudo test -S /run/mkruntimed.sock && echo socket_present=1 || echo socket_present=0
sudo ls -la /run/mkruntimed.sock /run/mkruntimed 2>&1 || true
sudo journalctl -u mkruntimed.service --since '15 minutes ago' --no-pager || true
ps -eo pid=,ppid=,stat=,comm=,args= | awk '$4 ~ /^(mkruntimed|containerd-shim|mkvsock-nbd|mk-agent-relay)$/ {print}'
MK_EVIDENCE_XTRACE=0 bash "${source_root}/scripts/audit-runtime-final-resources-live.sh" "${source_root}"
