# Primary-mediated ext4 evidence index

This directory contains raw transcripts from the 2026-08-30 GCE run of
Alternative approach 2 in `EXT4-DISK-PLAN.md`.

Key files:

- `mediated-transport-console.txt` and `mediated-transport-server.txt`: the
  successful 1-byte through 1-MiB AF_VSOCK integrity gate.
- `mediated-storage-preformat-audit.txt`, `mediated-storage-prepare.txt`, and
  `mediated-storage-rerun.txt`: blank-disk identity, outer ext4 creation, and
  destructive rerun refusal.
- `mediated-image-a-prepare.txt`, `mediated-image-b-prepare.txt`, and the rerun
  log: distinct inner UUIDs, manifests, full allocation, and refusal behavior.
- `child-a-cycle-1*.txt`: preserved progressive failures (direct NBD socket,
  loopback-down, missing `blkid`) and the first successful root cycle.
- `child-a-cycle-2*.txt` and `child-a-cycle-3*.txt`: counter persistence and
  final request/flush counters.
- `dual-a-1-console.txt`, `dual-b-1-console.txt`, and
  `mediated-dual-state.txt`: the decisive simultaneous distinct-kernel proof.
- `dual-a-1-server.txt` and `dual-b-1-server.txt`: per-export I/O counters and
  the explicitly discarded partial B write during forced teardown.
- `child-a-post-primary-reset-*` and `child-a-post-gce-stop-start-*`: counters
  6 and 7 after the two primary lifecycle boundaries.
- `child-a-final-fsck.txt`, `child-b-final-fsck.txt`,
  `outer-final-fsck.txt`, and `final-host-state.txt`: final filesystem,
  resource-return, disk identity, CPU, and host-health proof.
- `cloud-disposition.txt`: stopped VM and retained billable disk state.

The earlier failed transcripts are retained intentionally. They show that each
unsafe or unsupported assumption failed before an incorrect filesystem mount
or cross-export write occurred.
