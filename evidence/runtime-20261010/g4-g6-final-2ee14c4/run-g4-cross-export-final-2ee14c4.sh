#!/usr/bin/env bash
set -Eeuo pipefail
set -x

source_root=/var/tmp/mksrc-2ee14c4
expected_selector=/usr/local/lib/multikernel/releases/0.1.0-dev-2ee14c49a6dd674629ba18a0ca618c1870311ca1
test "$(id -u)" -ne 0
test "$(readlink -f /usr/local/lib/multikernel/current)" = "$expected_selector"
MK_EVIDENCE_XTRACE=0 bash "$source_root/scripts/audit-runtime-final-resources-live.sh" "$source_root"
bash "$source_root/scripts/test-runtime-cross-sandbox-storage-live.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=cross-export-isolation-and-cleanup passed=true'
sudo systemctl restart mkruntimed.service
for _ in $(seq 1 240); do
  sudo test -S /run/mkruntimed.sock && break
  sleep .25
done
sudo test -S /run/mkruntimed.sock
for _ in $(seq 1 240); do
  output=$(MK_EVIDENCE_XTRACE=0 bash "$source_root/scripts/audit-runtime-final-resources-live.sh" "$source_root" 2>&1) && break
  sleep .25
done
printf '%s\n' "$output"
MK_EVIDENCE_XTRACE=0 bash "$source_root/scripts/audit-runtime-final-resources-live.sh" "$source_root"
echo G4_FINAL_EXACT_RELEASE_PASS
