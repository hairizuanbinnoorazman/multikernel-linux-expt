#!/usr/bin/env bash
set -euxo pipefail

source_root=/var/tmp/mksrc-87ef4fb-v3
revision=87ef4fbcf47f7dc41c17c774a5490b3fa2b1312a
release_id=0.1.0-dev-87ef4fbcf47f7dc41c17c774a5490b3fa2b1312a
release_root=/usr/local/lib/multikernel/releases/$release_id

echo G6_RECONNECT_CANDIDATE_ACTIVATION_BEGIN
date -u +%FT%TZ
printf 'boot_id=%s\n' "$(cat /proc/sys/kernel/random/boot_id)"
test "$(sha256sum "$source_root/runtime/release-manifest.json" | awk '{print $1}')" = \
  3c709c33e9550867d9d927c4f2347dd65ee110387d60eea64279fd226d0bacfb
test "$(sha256sum "$source_root/runtime/bin/containerd-shim-multikernel-v2" | awk '{print $1}')" = \
  ea3536b1a3e88c7e598956da3b2a084d074e04cf5f91312c6bc6f1e4296c9d85
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
echo G6_RECONNECT_CANDIDATE_ACTIVATION_PASS
