# Documentation map

This repository has two connected purposes:

1. preserve reproducible Multikernel Linux experiments and their evidence; and
2. develop a containerd/Kubernetes runtime that uses a Multikernel child as a
   pod sandbox.

## Guides

- [`guides/gce-lab-runbook.md`](guides/gce-lab-runbook.md) contains the complete
  GCE build, bring-up, recovery, and cleanup procedure formerly embedded in the
  root README.

## Completed experiments

[`experiments/README.md`](experiments/README.md) is the experiment index. It
groups plans, reports, execution matrices, field notes, and raw evidence by
storage and lifecycle theme.

## Runtime development

- [`runtime/architecture.md`](runtime/architecture.md) defines the target
  system, trust boundary, component ownership, and lifecycle.
- [`runtime/decisions/0001-build-a-new-runtime.md`](runtime/decisions/0001-build-a-new-runtime.md)
  records why the project will not fork Firecracker.
- [`runtime/research/RELATED-WORK.md`](runtime/research/RELATED-WORK.md) records
  the public projects checked before selecting this direction.
- [`runtime/plans/README.md`](runtime/plans/README.md) is the ordered
  implementation and test roadmap.

## Project tracking

- [`project/TASKS.md`](project/TASKS.md) preserves completed experiment tasks
  and tracks runtime gates G0 through G10.

New runtime evidence should be stored under `evidence/runtime-YYYYMMDD/` with
an index describing the source revision, environment, command, result, and
cleanup state.
