# Documentation map

This repository has two connected purposes:

1. preserve reproducible Multikernel Linux experiments and their evidence; and
2. develop a containerd/Kubernetes runtime that uses a Multikernel child as a
   pod sandbox.

## Runtime design

- [`architecture/TARGET.md`](architecture/TARGET.md) defines the target system,
  trust boundary, component ownership, and lifecycle.
- [`decisions/0001-build-a-new-runtime.md`](decisions/0001-build-a-new-runtime.md)
  records why the project will not fork Firecracker.
- [`research/RELATED-WORK.md`](research/RELATED-WORK.md) records the public
  projects checked before selecting this direction.
- [`plans/README.md`](plans/README.md) is the ordered implementation and test
  roadmap.

## Completed experiments

The existing top-level documents remain in place so their links, hashes, and
evidence references do not change:

- [`../LEARNINGS.md`](../LEARNINGS.md): primary-kernel and child bring-up.
- [`../DAXFS-PLAN.md`](../DAXFS-PLAN.md) and
  [`../DAXFS-IMPLEMENTATION.md`](../DAXFS-IMPLEMENTATION.md): DAXFS and
  Docker-derived child roots.
- [`../EXT4-DISK-PLAN.md`](../EXT4-DISK-PLAN.md),
  [`../EXT4-DISK-EXECUTION.md`](../EXT4-DISK-EXECUTION.md), and
  [`../EXT4-DISK-LEARNINGS.md`](../EXT4-DISK-LEARNINGS.md): direct-device
  feasibility and rejection.
- [`../EXT4-MEDIATED-IMPLEMENTATION.md`](../EXT4-MEDIATED-IMPLEMENTATION.md):
  primary-managed persistent ext4 roots.

New runtime evidence should be stored under `evidence/runtime-YYYYMMDD/` with
an index describing the source revision, environment, command, result, and
cleanup state.
