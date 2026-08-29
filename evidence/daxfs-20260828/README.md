# DAXFS GCE evidence index

These files were captured from `mklinux-lab` on 2026-08-28. The concise result
and interpretation are in [`../../DAXFS-IMPLEMENTATION.md`](../../DAXFS-IMPLEMENTATION.md).

Key proof files:

- [`dual-kernel-summary.txt`](dual-kernel-summary.txt): both distinct kernels
  active with their Docker-derived DAXFS roots.
- [`dual-kernel-a-console.txt`](dual-kernel-a-console.txt) and
  [`dual-kernel-b-console.txt`](dual-kernel-b-console.txt): complete boot/MKTTY
  transcripts.
- [`dual-kernel-cleanup.txt`](dual-kernel-cleanup.txt): no pool or instances
  after teardown.
- [`upstream-test-overlay.txt`](upstream-test-overlay.txt): DAXFS 20/20 suite.
- [`stage3-console.txt`](stage3-console.txt): mount-only proof.
- [`stage4-cycle1-console.txt`](stage4-cycle1-console.txt) and
  [`stage4-cycle2-console.txt`](stage4-cycle2-console.txt): repeated root proof.
- [`shared-rw-summary.txt`](shared-rw-summary.txt): writable-coherence failure.
- [`restart-console.txt`](restart-console.txt): allocation-lifetime restart.
- [`corruption-validation.txt`](corruption-validation.txt) and
  [`exhaustion-result-1m.txt`](exhaustion-result-1m.txt): negative tests.
- [`performance-summary.txt`](performance-summary.txt): bounded microbenchmark.
- [`final-host-health.txt`](final-host-health.txt): final host state and relevant
  kernel messages.
- [`build-proof.txt`](build-proof.txt) and
  [`daxfs-demo-console.txt`](daxfs-demo-console.txt): live-tested Stage 6 build
  and `daxfs-up` proof.
- [`manifest-remediation-build.txt`](manifest-remediation-build.txt) and
  [`manifest-remediation-console.txt`](manifest-remediation-console.txt):
  corrected non-self-referential manifest and full in-child checksum proof.
- [`pre-stop-clean-state.txt`](pre-stop-clean-state.txt): all CPUs returned, no
  pool or instances, and guest agent active immediately before infrastructure
  cleanup.
- [`resource-cleanup.txt`](resource-cleanup.txt): post-deletion GCE checks show
  the VM and boot disk absent and both retained snapshots `READY`.

Raw files contain terminal control characters where they came directly from
`script(1)`/MKTTY. No result was edited out of the transcripts.

After this bundle was copied, `mklinux-lab` and its auto-delete 100 GB boot
disk were permanently deleted. The stock and pre-DAXFS GCE snapshots were
retained, but the post-snapshot DAXFS and alternate-kernel artifacts are no
longer directly available. This local bundle is proof of the completed run,
not a binary backup from which the dual-kernel experiment can be launched
directly.
