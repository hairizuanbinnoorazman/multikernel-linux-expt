# G2 learnings: recoverable control plane

## Result and current status

The 2026-08-31 work is a **provisional G2 milestone**, not a closed gate.
`mkruntimed` exposes a strict v1 JSON
API over a mode-0660 Unix socket, maintains a fsynced write-ahead JSONL
journal and atomic snapshot, serializes global/per-sandbox allocation, and
invokes Kerf with explicit argv, a sanitized environment, and timeouts.

Local tests cover full create/load/start/stop/delete, exact replay,
idempotency-key conflict, stale generations, concurrent CPU allocation,
backend failure, and restart reconciliation. State/backend disagreement is
marked `OPERATOR_ACTION`; the daemon never guesses that it may delete an
unowned resource.

The live run created, loaded, and started one child only through the daemon,
killed the daemon with `SIGKILL`, restarted it against the durable state, then
stopped/deleted the child and returned CPUs/memory. The pass marker is retained
in [`g2-control-plane.log`](../../../evidence/runtime-20260831/g0-g3-gce/g2-control-plane.log).

## Compatibility findings

Two live corrections were required:

1. Kerf accepts a bare integer for byte quantities, not a `B` suffix.
2. Kerf v0.2.0 can create the exact requested instance and subsequently exit
   nonzero with `KeyError`. The adapter now accepts that outcome only after a
positive observation of the exact sysfs instance; absence remains failure.

## Replacement live evidence (2026-09-03)

The daemon `SIGKILL` scenario reproduced with state under
`/var/lib/multikernel-proof-g2` instead of `/tmp`. The replacement harness
retained all five API responses and copied the journal/snapshot immediately
after daemon death and after restart-driven stop/delete. At death the snapshot
records sandbox state `RUNNING` with six complete journal records; after
recovery it records `ABSENT` with ten intent/complete records. CPUs, memory,
pool, and instance state returned cleanly. The [G2 manifest](../../../evidence/runtime-20260903/g0-g3-proof/manifest-g2.json)
closes the missing durable-artifact rerun, not the orphan, operation-boundary,
or host-reset gaps described below.

A continuing remediation run then exercised two disjoint children through the
daemon. Both were `RUNNING` when the daemon was killed with `SIGKILL`; the
retained snapshot/journal contains both records, and a restarted daemon stopped
and deleted both before returning CPUs `0-15`. The first attempt used an 8 GiB
pool for two 4 GiB requests and correctly failed the second allocation; that
failed transcript is retained separately. A 12 GiB pool passed, demonstrating
that configured capacity must include allocator slack rather than merely equal
the sum of guest requests. See the current [remediation evidence](../../../evidence/runtime-20260903/g0-g3-remediation/README.md).

## Claim-to-proof audit

The local lifecycle, replay, stale-generation, overlap, concurrent allocation,
second-sandbox pool reuse, backend adapter, and known-state reconciliation
tests pass under `cd runtime && GOCACHE=/tmp/mk-go-cache go test ./...`. The
live file [`g2-control-plane.log`](../../../evidence/runtime-20260831/g0-g3-gce/g2-control-plane.log)
contains only a 71-byte restart/reconcile pass marker and cleanup message. It
does not retain the journal, snapshot, request/response transcript, daemon
logs, or operation-boundary assertions needed to independently reproduce the
broader prose claim.

Code inspection confirms the important remaining gaps: reconciliation
iterates snapshotted sandboxes rather than backend inventory or orphan journal
intents; every disagreement becomes `OPERATOR_ACTION`; snapshot rename does not
fsync the containing directory; `WatchEvents`, strict host-config loading, approved-manifest
resolution, and observable intermediate states are absent. Consequently only
the named unit tests and narrow live marker are retained claims; all stronger
G2 conformance statements remain open in the remediation checklist.

The second synchronization pass added focused daemon-wire rejection tests,
`BACKEND_TIMEOUT`, stable mutation operation IDs, checked error-state
persistence, secret-safe backend/state errors, static bundle/manifest/label/key
validation, exact terminal delete replay, and configured Kerf-pool CPU
membership enforcement. Socket/state ownership and safe-parent checks remain
incomplete. Approved-manifest resolution and live APIC eligibility are still
absent. Pool memory is now parsed, an explicit 1 GB allocator reserve is
subtracted, and non-absent allocations are summed under the allocation lock
before Kerf is called. A corrected live pair proves an 8 GB pool returns
`RESOURCE_EXHAUSTED` before the backend while 12 GB admits the two 4 GiB
children. The harness also now records the real privileged daemon PID; this
found and removed an orphan daemon from the earlier attempt, whose cleanup
claim has been superseded by the corrected run.

## Current audited verdict

G2 supports the happy-path lifecycle, exact mutation replay, generation checks,
basic overlap prevention, a fsynced intent journal, atomic snapshot replacement,
configured-pool CPU enforcement, classified secret-safe errors, and
conservative known-sandbox reconciliation. It remains open because orphan
external resources and pre-snapshot crash windows can be missed, directory
fsync is absent, and the event/config/manifest contracts are not fully enforced.
