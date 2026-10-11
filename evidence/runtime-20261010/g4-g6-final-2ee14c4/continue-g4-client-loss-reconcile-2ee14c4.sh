#!/usr/bin/env bash
set -Eeuo pipefail
set -x

source_root=/var/tmp/mksrc-2ee14c4
boot=772c6339-6ed7-494b-8d7f-71f3cb7394eb
sandbox_id=mk-mk-storage-client-loss-216e4611aa588b7e
sandbox_generation=c5106654766ab44b1af0bc9b1ab7a7f0
endpoint_generation=f5758a5bbfd04c4dfeebd0f224eb7280
task_key=task-216e4611aa588b7e8436be9484e3f8cd
container_id=mk-storage-client-loss
recovery=/var/lib/multikernel-recovery/g4-client-loss-772c6339-2ee14c4
disk_recovery=/srv/multikernel-storage/quarantine/g4-client-loss-772c6339-2ee14c4

test "$(id -u)" -ne 0
test "$(cat /proc/sys/kernel/random/boot_id)" = "$boot"
test "$(readlink -f /usr/local/lib/multikernel/current)" = /usr/local/lib/multikernel/releases/0.1.0-dev-2ee14c49a6dd674629ba18a0ca618c1870311ca1
sudo test -f "$recovery/SHA256SUMS"
sudo sha256sum -c "$recovery/SHA256SUMS"
sudo test -f "$disk_recovery/$task_key/root.ext4"
sudo test ! -e "/sys/fs/multikernel/instances/$sandbox_id"
test "$(sudo jq '.sandboxes|length' /var/lib/mkruntime/state.json)" = 0
test "$(sudo jq '.records|length' /var/lib/mkruntimed/rootfs/state.json)" = 0
test "$(sudo jq '[.exports[]|select(.state!="RELEASED")]|length' /var/lib/mkruntimed/storage/state.json)" = 0
test "$(sudo ctr tasks list | awk -v id="$container_id" '$1==id {print $3}')" = STOPPED
test "$(sudo ctr containers list | awk -v id="$container_id" '$1==id {print $1}')" = "$container_id"
sudo python3 - "$sandbox_id" "$sandbox_generation" "$endpoint_generation" "$container_id" <<'PY'
import json,sys
sandbox_id,generation,endpoint_generation,container_id=sys.argv[1:]
state=json.load(open('/var/lib/mknetd/state.json'))
assert len(state['endpoints']) == 1, state
endpoint=next(iter(state['endpoints'].values()))
assert endpoint['container_id'] == container_id, endpoint
assert endpoint['sandbox_id'] == sandbox_id and endpoint['sandbox_generation'] == generation, endpoint
assert endpoint['generation'] == endpoint_generation and endpoint['state'] == 'DISCONNECTED', endpoint
print('G4_CLIENT_LOSS_CONTINUATION_PRECONDITIONS_PASS')
PY

sudo timeout 60 ctr tasks rm "$container_id"
sudo ctr containers rm "$container_id"
for _ in $(seq 1 240); do
  tasks=$(sudo ctr tasks list -q | wc -l)
  containers=$(sudo ctr containers list -q | wc -l)
  endpoints=$(sudo jq '.endpoints|length' /var/lib/mknetd/state.json)
  shims=$(ps -eo comm= | awk '$1=="containerd-shim" {n++} END {print n+0}')
  [[ $tasks = 0 && $containers = 0 && $endpoints = 0 && $shims = 0 ]] && break
  sleep .25
done
test "$tasks $containers $endpoints $shims" = '0 0 0 0'

sudo systemctl restart mkruntimed.service
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

echo 'OBSERVATION_BEGIN key=g4-client-loss-operator-reconcile-complete'
printf 'boot=%s service_pid=%s recovery=%s disk_recovery=%s\n' "$boot" "$pid" "$recovery" "$disk_recovery"
sudo sha256sum -c "$recovery/SHA256SUMS"
sudo sha256sum "$recovery/SHA256SUMS" "$disk_recovery/$task_key/root.ext4"
sudo jq -c '{sequence, sandboxes:(.sandboxes|length), results:(.results|length)}' /var/lib/mkruntime/state.json
sudo jq -c '{records:(.records|length)}' /var/lib/mkruntimed/rootfs/state.json
sudo jq -c '{exports:(.exports|length),live:([.exports[]|select(.state!="RELEASED")]|length)}' /var/lib/mkruntimed/storage/state.json
sudo jq -c '{endpoints:(.endpoints|length)}' /var/lib/mknetd/state.json
echo 'OBSERVATION_END key=g4-client-loss-operator-reconcile-complete'
bash "$source_root/scripts/audit-runtime-final-resources-live.sh" "$source_root"
echo G4_ACCEPTED_CLIENT_OPERATOR_RECONCILE_PASS
