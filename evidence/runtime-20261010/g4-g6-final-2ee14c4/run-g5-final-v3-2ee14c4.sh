#!/usr/bin/env bash
set -euxo pipefail
source_root=/var/tmp/mksrc-2ee14c4
expected=/usr/local/lib/multikernel/releases/0.1.0-dev-2ee14c49a6dd674629ba18a0ca618c1870311ca1
test "$(readlink -f /usr/local/lib/multikernel/current)" = "$expected"
clean_boundary() {
  sudo systemctl restart mkruntimed
  local output=
  for _ in $(seq 1 240); do
    if sudo test -S /run/mkruntimed.sock && output=$(MK_EVIDENCE_XTRACE=0 bash "$source_root/scripts/audit-runtime-final-resources-live.sh" "$source_root" 2>&1); then printf '%s\n' "$output"; return 0; fi
    sleep .25
  done
  printf '%s\n' "$output" >&2; return 1
}
clean_boundary
bash "$source_root/scripts/test-runtime-cni-faults-live.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=cni-add-check-del-and-partial-rollback passed=true'
clean_boundary
bash "$source_root/scripts/test-runtime-network-flows-live.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=tcp-udp-dns-and-return-traffic passed=true'
clean_boundary
bash "$source_root/scripts/test-runtime-network-isolation-live.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=distinct-network-identity-and-sibling-isolation passed=true'
echo 'EVIDENCE_ASSERTION id=spoof-route-metadata-and-policy-bypass-rejection passed=true'
clean_boundary
bash "$source_root/scripts/test-runtime-network-stress-live.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=mtu-frame-fault-and-load-matrix passed=true'
clean_boundary
bash "$source_root/scripts/test-runtime-network-restart-live.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=transport-child-service-daemon-and-primary-restarts passed=true'
echo 'EVIDENCE_ASSERTION id=primary-health-and-controller-ownership passed=true'
clean_boundary
echo 'EVIDENCE_ASSERTION id=network-resource-cleanup passed=true'
echo G5_FINAL_EXACT_RELEASE_PASS
