#!/usr/bin/env bash
set -euxo pipefail

source_root=/var/tmp/mksrc-d7c5301
revision=d7c530103b43c63d97518b0d6f5aa5442e7b15a3
release_id=0.1.0-dev-d7c530103b43c63d97518b0d6f5aa5442e7b15a3
release_root=/usr/local/lib/multikernel/releases/$release_id

echo G6_STREAM_KEEPER_CANDIDATE_ACTIVATION_BEGIN
date -u +%FT%TZ
printf 'boot_id=%s\n' "$(cat /proc/sys/kernel/random/boot_id)"
test "$(sha256sum "$source_root/runtime/release-manifest.json" | awk '{print $1}')" = \
  612f1165f93ac7d5f2ee4e3d71db493824605c8b678c811f9e5be14b48bb9cd1
test "$(sha256sum "$source_root/runtime/bin/containerd-shim-multikernel-v2" | awk '{print $1}')" = \
  8ad23ba2e3fe7c6fda6df51f873682fd6aac74306dcb68e227d612dc38df16b2
sudo -u "$SUDO_USER" bash "$source_root/scripts/audit-runtime-final-resources-live.sh" "$source_root"

python3 "$source_root/scripts/manage-runtime-binaries.py" install \
  "$source_root/runtime/release-manifest.json" "$source_root/runtime/bin"
python3 "$source_root/scripts/manage-runtime-binaries.py" inspect
test "$(readlink -f /usr/local/lib/multikernel/current)" = "$release_root"
cmp "$source_root/runtime/release-manifest.json" "$release_root/release-manifest.json"
for binary in "$release_root"/bin/*; do
  "$binary" --version
done
sha256sum "$release_root"/bin/* "$release_root/release-manifest.json"

systemctl restart mknetd.service mkruntimed.service
for attempt in $(seq 1 60); do
  if systemctl is-active --quiet mknetd.service && \
     systemctl is-active --quiet mkruntimed.service && \
     test -S /run/mkruntimed.sock; then
    break
  fi
  sleep 1
done
for service in mkruntimed mknetd containerd docker; do
  test "$(systemctl is-active "$service")" = active
  test "$(systemctl show -p NRestarts --value "$service")" = 0
  systemctl show "$service" -p Id -p ActiveState -p SubState -p MainPID -p NRestarts --no-pager
done
test "$(/usr/local/bin/containerd-shim-multikernel-v2 --version)" = \
  "containerd-shim-multikernel-v2 version=0.1.0-dev revision=$revision"
test "$(/usr/local/sbin/mkruntimed --version)" = \
  "mkruntimed version=0.1.0-dev revision=$revision"
test "$(/usr/local/sbin/mknetd --version)" = \
  "mknetd version=0.1.0-dev revision=$revision"
sha256sum /proc/"$(systemctl show -p MainPID --value mkruntimed)"/exe \
  /proc/"$(systemctl show -p MainPID --value mknetd)"/exe \
  /usr/local/bin/containerd-shim-multikernel-v2
sudo -u "$SUDO_USER" bash "$source_root/scripts/audit-runtime-final-resources-live.sh" "$source_root"
echo G6_STREAM_KEEPER_CANDIDATE_ACTIVATION_PASS
