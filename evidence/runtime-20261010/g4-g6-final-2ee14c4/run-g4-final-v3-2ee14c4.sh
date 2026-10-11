#!/usr/bin/env bash
set -euxo pipefail
source_root=/var/tmp/mksrc-2ee14c4
expected=/usr/local/lib/multikernel/releases/0.1.0-dev-2ee14c49a6dd674629ba18a0ca618c1870311ca1
test "$(readlink -f /usr/local/lib/multikernel/current)" = "$expected"
clean_boundary() {
  sudo systemctl restart mkruntimed
  local output=
  for _ in $(seq 1 240); do
    if sudo test -S /run/mkruntimed.sock && output=$(MK_EVIDENCE_XTRACE=0 bash "$source_root/scripts/audit-runtime-final-resources-live.sh" "$source_root" 2>&1); then
      printf '%s\n' "$output"
      return 0
    fi
    sleep .25
  done
  printf '%s\n' "$output" >&2
  return 1
}
clean_boundary
bash "$source_root/scripts/test-runtime-caller-snapshot-live.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=oci-source-digests-and-snapshot-immutability passed=true'
clean_boundary
bash "$source_root/scripts/test-runtime-initramfs-repro-live.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=deterministic-root-rebuild-and-change-control passed=true'
clean_boundary
bash "$source_root/scripts/test-runtime-g4-input-boundaries-live.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=root-metadata-and-hostile-input-matrix passed=true'
clean_boundary
bash "$source_root/scripts/test-runtime-image-architecture-live.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=architecture-and-kernel-compatibility passed=true'
clean_boundary
bash "$source_root/scripts/test-runtime-writable-state-model-live.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=writable-root-owner-generation-and-isolation passed=true'
clean_boundary
bash "$source_root/scripts/test-runtime-readonly-binds-live.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=readonly-bind-and-persistence-semantics passed=true'
clean_boundary
bash "$source_root/scripts/test-runtime-capacity-enospc-live.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=capacity-quota-high-water-and-enospc passed=true'
clean_boundary
bash "$source_root/scripts/test-runtime-storage-teardown-live.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=storage-teardown-offline-check-and-recovery passed=true'
clean_boundary
bash "$source_root/scripts/test-runtime-storage-client-loss-live.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=corruption-server-loss-host-reset-and-clone passed=true'
clean_boundary
bash "$source_root/scripts/test-runtime-cross-sandbox-storage-live.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=cross-export-isolation-and-cleanup passed=true'
clean_boundary
echo G4_FINAL_EXACT_RELEASE_PASS
