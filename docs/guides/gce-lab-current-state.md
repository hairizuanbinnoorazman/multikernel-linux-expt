# Current verified GCE lab state

Revalidate this checkpoint before live admission.
Run the read-only observer from a checkout with authenticated `gcloud` access:

```sh
python3 evidence/runtime-20261122/g8-finite-child-writer/current-status.py
```

It pins the qualified source libraries explicitly, verifies the current boot and
fresh primary file/service/Docker baseline, requires the original idle transport
and empty child/pool/VSOCK resources, and remeasures the complete committed trial
packet with the unchanged 8 GiB staging floor. A later restart or state change
requires fresh verification; historical PIDs and successful units grant no
process, cleanup or reuse authority.

| Item | Latest verified value |
| --- | --- |
| Project | `new-demo-project-462517` |
| Zone | `asia-southeast1-b` |
| Existing instance | `mklinux-g7-storage-20261012` |
| Boot | `143300f7-d035-4506-872e-8027be260f1e` |
| Transport | Original `65C55BB38C669FF94A4B772`, idle, zero references |
| CPUs | `0-15` online; no allocated child or memory pool |
| Data filesystem | 200 GiB; exactly 5% reserved; mounted availability checked |
| Private baseline | `/srv/multikernel-storage/stages/g8-finite-writer-2ad6cda/cold-after-primary.json` |

The [finite child-writer proof](../../evidence/runtime-20261122/g8-finite-child-writer/README.md)
qualifies source `2ad6cda1`: complete child-local build/cache/run/save/failure
matrix, exact finite 16 MiB producer completion, metadata-only rotation, primary
preservation and native offline cleanup. Two local log files total 1,797,083
bytes below the 2,131,072-byte sample bound. Independent cold restoration
preserves 280 artifacts/111 absences and all previous/failed/native sets,
requalifies archived v2 success only in its original boot and establishes this
fresh primary baseline. Original failed-start and verifier-refusal evidence
remain retained in their original scopes.

These are scoped observations. Loss/peak accounting, broader readiness,
production log coverage, fault mixtures and full G7/G8 remain open. Direct child
kernels use no nested virtualization and share primary trust after arbitrary
child-kernel compromise. See the [lab runbook](gce-lab-runbook.md) for setup and
the [runtime plans](../runtime/plans/README.md) for remaining requirements.

The [readiness-budget controls](../../evidence/runtime-20261122/g8-child-log-budget/README.md)
qualify source `506c145c` on prior boot `54ed0c5f` without loading a candidate
or starting a child. Their 51-artifact/3-absence seal survives the later cold
restoration. That control proof keeps its native deadline scope; the actual
finite child lifecycle above uses source `2ad6cda1`.

The [deployment logging candidate proof](../../evidence/runtime-20261122/g8-deployment-logging/README.md)
then validates source `fbddde6d` on this same boot: ten CLI groups and four
installed-`dockerd` parser checks pass with primary invariants unchanged and
capacity available. No configuration is installed and no daemon/child is
started. Preserve both private stage directories
`g8-deployment-logging-fbddde6d` and `g8-deployment-logging-fbddde6d-v2`
in subsequent retention inventories. The first collector refusal remains
retained; candidate generation does not grant runtime enforcement or full-gate
acceptance.
