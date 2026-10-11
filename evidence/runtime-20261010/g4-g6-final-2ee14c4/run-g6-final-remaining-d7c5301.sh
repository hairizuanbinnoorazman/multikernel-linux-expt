#!/usr/bin/env bash
set -euxo pipefail

source_root=/var/tmp/mksrc-d7c5301
revision=d7c530103b43c63d97518b0d6f5aa5442e7b15a3
expected=/usr/local/lib/multikernel/releases/0.1.0-dev-${revision}
daemon_sha=f0634fbed5e70f6bb2464a27dc32fe80d0645dd784a42b38d508b23943425d97
shim_sha=8ad23ba2e3fe7c6fda6df51f873682fd6aac74306dcb68e227d612dc38df16b2
builder_sha=633fcfc08206840a3546f824a5f2a0d27508e2cfcc78499fa01a193ece52ccb9

test "$(readlink -f /usr/local/lib/multikernel/current)" = "$expected"
daemon_pid=$(systemctl show -p MainPID --value mkruntimed)
test "$(sudo sha256sum "/proc/${daemon_pid}/exe" | awk '{print $1}')" = "$daemon_sha"
test "$(sha256sum "$expected/bin/containerd-shim-multikernel-v2" | awk '{print $1}')" = "$shim_sha"
builder_path=$(readlink -f /usr/local/libexec/multikernel/build-runtime-container-initramfs.sh)
test "$(sudo sha256sum "$builder_path" | awk '{print $1}')" = "$builder_sha"

clean_boundary() {
  sudo systemctl restart mkruntimed
  local output=
  for _ in $(seq 1 240); do
    if sudo test -S /run/mkruntimed.sock && \
       output=$(MK_EVIDENCE_XTRACE=0 bash "$source_root/scripts/audit-runtime-final-resources-live.sh" "$source_root" 2>&1); then
      printf '%s\n' "$output"
      return 0
    fi
    sleep .25
  done
  printf '%s\n' "$output" >&2
  return 1
}

clean_boundary
MK_EXPECTED_REVISION="$revision" \
MK_EXPECTED_DAEMON_SHA256="$daemon_sha" \
MK_EXPECTED_SHIM_SHA256="$shim_sha" \
MK_EXPECTED_BUILDER_SHA256="$builder_sha" \
  bash "$source_root/scripts/test-runtime-oci-preallocation-live.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=oci-surface-fail-closed-before-allocation passed=true'
clean_boundary
bash "$source_root/scripts/test-runtime-concurrent-churn-live.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=concurrent-sandbox-churn-and-isolation passed=true'
clean_boundary
bash "$source_root/scripts/test-runtime-task-v2-exhaustive-vm.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=signals-exits-races-and-name-reuse passed=true'
echo 'EVIDENCE_ASSERTION id=cancellation-and-deadline-cleanup passed=true'
clean_boundary
echo 'EVIDENCE_ASSERTION id=runtime-resource-cleanup passed=true'
echo G6_FINAL_EXACT_RELEASE_REMAINING_PASS
