#!/usr/bin/env bash
set -euxo pipefail

source_root=/var/tmp/mksrc-d7c5301
expected=/usr/local/lib/multikernel/releases/0.1.0-dev-d7c530103b43c63d97518b0d6f5aa5442e7b15a3
test "$(readlink -f /usr/local/lib/multikernel/current)" = "$expected"
MK_EVIDENCE_XTRACE=0 bash "$source_root/scripts/audit-runtime-final-resources-live.sh" "$source_root"
bash "$source_root/scripts/test-runtime-cross-sandbox-storage-live.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=cross-export-isolation-and-cleanup passed=true'
sudo systemctl restart mkruntimed.service
for _ in $(seq 1 240); do sudo test -S /run/mkruntimed.sock && break; sleep .25; done
sudo test -S /run/mkruntimed.sock
MK_EVIDENCE_XTRACE=0 bash "$source_root/scripts/audit-runtime-final-resources-live.sh" "$source_root"
echo G4_FINAL_D7C5301_PASS
