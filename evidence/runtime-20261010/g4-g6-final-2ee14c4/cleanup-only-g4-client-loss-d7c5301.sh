#!/usr/bin/env bash
set -euxo pipefail

source_root=/var/tmp/mksrc-d7c5301
boot=ca1337df-2163-4621-a931-be59076b2683
container_id=mk-storage-client-loss
sandbox_id=mk-mk-storage-client-loss-216e4611aa588b7e
task_key=task-216e4611aa588b7e8436be9484e3f8cd
recovery=/var/lib/multikernel-recovery/g4-client-loss-ca1337df-d7c5301
disk_recovery=/srv/multikernel-storage/quarantine/g4-client-loss-ca1337df-d7c5301
expected_exe=/usr/local/lib/multikernel/releases/0.1.0-dev-d7c530103b43c63d97518b0d6f5aa5442e7b15a3/bin/containerd-shim-multikernel-v2

test "$(cat /proc/sys/kernel/random/boot_id)" = "$boot"
sudo sha256sum -c "$recovery/SHA256SUMS"
sudo test -f "$disk_recovery/$task_key/root.ext4"
sudo test ! -e "/sys/fs/multikernel/instances/$sandbox_id"
test "$(sudo jq '.sandboxes|length' /var/lib/mkruntime/state.json)" = 0
test "$(sudo jq '.records|length' /var/lib/mkruntimed/rootfs/state.json)" = 0
test "$(sudo jq '[.exports[]|select(.state!="RELEASED")]|length' /var/lib/mkruntimed/storage/state.json)" = 0
test "$(sudo jq '.endpoints|length' /var/lib/mknetd/state.json)" = 1
holder=$(sudo ctr tasks list | awk -v id="$container_id" '$1==id && $3=="STOPPED" {print $2}')
test -n "$holder"
worker=$(sudo awk '{print $4}' "/proc/$holder/stat")
supervisor=$(sudo awk '{print $4}' "/proc/$worker/stat")
for pid in "$holder" "$worker" "$supervisor"; do
  test "$(sudo readlink -f "/proc/$pid/exe")" = "$expected_exe"
done
sudo tr '\0' ' ' <"/proc/$supervisor/cmdline" | grep -Fq -- "-id $container_id"
sudo tr '\0' ' ' <"/proc/$worker/cmdline" | grep -Fq -- "-id $container_id"
echo "OBSERVATION cleanup_only_roles supervisor=$supervisor worker=$worker holder=$holder"

fault_started=$(date -u '+%Y-%m-%d %H:%M:%S UTC')
sudo kill -KILL "$supervisor"
for _ in $(seq 1 720); do
  tasks=$(sudo ctr tasks list -q | wc -l)
  endpoints=$(sudo jq '.endpoints|length' /var/lib/mknetd/state.json)
  children=$(sudo find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 -type d | wc -l)
  shims=$(ps -eo comm= | awk '$1=="containerd-shim" {n++} END {print n+0}')
  [[ $tasks = 0 && $endpoints = 0 && $children = 0 && $shims = 0 ]] && break
  sleep .25
done
test "$tasks $endpoints $children $shims" = '0 0 0 0'
sudo ctr tasks rm -f "$container_id" >/dev/null 2>&1 || true
sudo ctr containers rm "$container_id"
test -z "$(sudo ctr containers list -q)"

sudo systemctl restart mkruntimed.service
for _ in $(seq 1 240); do sudo test -S /run/mkruntimed.sock && break; sleep .25; done
sudo test -S /run/mkruntimed.sock
pid=$(systemctl show mkruntimed.service -p MainPID --value)
test "$pid" -gt 0
test "$(systemctl show mkruntimed.service -p NRestarts --value)" = 0
sleep 3
test "$(systemctl show mkruntimed.service -p MainPID --value)" = "$pid"

echo 'OBSERVATION_BEGIN key=g4-d7-client-loss-cleanup-only-reclaim'
printf 'boot=%s old_supervisor=%s old_worker=%s old_holder=%s service_pid=%s\n' "$boot" "$supervisor" "$worker" "$holder" "$pid"
sudo journalctl -u containerd.service --since "$fault_started" --no-pager -n 120 | grep -E "($container_id|shim disconnected|cleanup)" || true
sudo sha256sum -c "$recovery/SHA256SUMS"
sudo sha256sum "$recovery/SHA256SUMS" "$disk_recovery/$task_key/root.ext4"
echo 'OBSERVATION_END key=g4-d7-client-loss-cleanup-only-reclaim'
MK_EVIDENCE_XTRACE=0 bash "$source_root/scripts/audit-runtime-final-resources-live.sh" "$source_root"
echo G4_D7_CLIENT_LOSS_CLEANUP_ONLY_RECLAIM_PASS
