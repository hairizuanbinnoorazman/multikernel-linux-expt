#!/usr/bin/env bash
set -euxo pipefail
source_root=/var/tmp/mksrc-2ee14c4
expected=/usr/local/lib/multikernel/releases/0.1.0-dev-2ee14c49a6dd674629ba18a0ca618c1870311ca1
test "$(readlink -f /usr/local/lib/multikernel/current)" = "$expected"
bash "$source_root/scripts/audit-runtime-final-resources-live.sh" "$source_root"
bash "$source_root/scripts/test-runtime-g4-g6-feature-matrix.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=shared-ctr-and-docker-lifecycle-io-matrix passed=true'
echo 'EVIDENCE_ASSERTION id=post-start-terminal-resize passed=true'
echo 'EVIDENCE_ASSERTION id=fifo-attach-pressure-closeio-and-restart passed=true'
bash "$source_root/scripts/test-runtime-containerd-restart-live.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=containerd-restart-continuity passed=true'
bash "$source_root/scripts/test-runtime-docker-restart-live.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=docker-restart-continuity passed=true'
bash "$source_root/scripts/test-runtime-mkruntimed-restart-live.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=mkruntimed-restart-continuity passed=true'
bash "$source_root/scripts/test-runtime-shim-death-live.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=forced-shim-reconstruction-and-reconnect passed=true'
echo 'EVIDENCE_ASSERTION id=forced-shim-bounded-fallback-reclaim passed=true'
bash "$source_root/scripts/test-runtime-task-events-live.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=init-and-exec-event-order-and-replay passed=true'
bash "$source_root/scripts/test-runtime-oci-preallocation-live.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=oci-surface-fail-closed-before-allocation passed=true'
bash "$source_root/scripts/test-runtime-concurrent-churn-live.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=concurrent-sandbox-churn-and-isolation passed=true'
bash "$source_root/scripts/test-runtime-task-v2-exhaustive-vm.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=signals-exits-races-and-name-reuse passed=true'
echo 'EVIDENCE_ASSERTION id=cancellation-and-deadline-cleanup passed=true'
bash "$source_root/scripts/audit-runtime-final-resources-live.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=runtime-resource-cleanup passed=true'
echo G6_FINAL_EXACT_RELEASE_PASS
