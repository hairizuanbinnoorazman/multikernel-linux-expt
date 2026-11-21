# Current verified GCE lab state

This is the latest verified checkpoint, not a replacement for live admission.
Run the read-only observer from a checkout with authenticated `gcloud` access:

```sh
python3 evidence/runtime-20261121/g8-child-log-pressure-corrected/current-status.py
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
| Boot | `54ed0c5f-701a-4f1e-aefa-d72f3bebcab9` |
| Transport | Original `65C55BB38C669FF94A4B772`, idle, zero references |
| CPUs | `0-15` online; no allocated child or memory pool |
| Data filesystem | 200 GiB; exactly 5% reserved; mounted availability checked |
| Private baseline | `/srv/multikernel-storage/stages/g8-child-logging-live-75e5e18/cold-after-primary.json` |

The [child build/log-pressure proof](../../evidence/runtime-20261121/g8-child-log-pressure-corrected/README.md)
qualifies source `75e5e188`: complete child-local Docker build/cache/run/save/
controlled-failure matrix, configured 16 MiB detached writer, metadata-only
rotation, unchanged primary state and native offline cleanup. Two local log
files total 1,503,596 bytes below the 2,131,072-byte sample bound. Independent
cold restoration preserves 264 artifacts/107 absences plus all prior and failed
sets, requalifies archived success only in its original boot and establishes
this fresh primary baseline. Original failed-start evidence remains unqualified.

These are scoped observations. Loss/peak accounting, broader readiness,
production log coverage, fault mixtures and full G7/G8 remain open. Direct child
kernels use no nested virtualization and share primary trust after arbitrary
child-kernel compromise. See the [lab runbook](gce-lab-runbook.md) for setup and
the [runtime plans](../runtime/plans/README.md) for remaining requirements.
