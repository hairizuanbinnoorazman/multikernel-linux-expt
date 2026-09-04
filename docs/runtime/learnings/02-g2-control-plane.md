# G2 learnings: recoverable control plane

## Result and current status

The 2026-08-31 work remains a **provisional historical G2 milestone**. The
2026-09-04 failure, admission, recovery, and post-deletion cloud evidence closes
G2.
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
3. The pinned 7.0 host exposes a newly created instance as sysfs status `ready`,
   while earlier fixtures assumed `created`. Both raw values now map to the one
   runtime state `CREATED`; the live failing transcript and isolated probe are
   retained as regression evidence.

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

The implementation audit found and closed the earlier reconciliation gaps. The
remediation persists complete sandboxes in intents, resumes
create/load/start/stop/delete from observed backend state, detects unknown
backend instances, treats durable `STOPPED` as compatible with Kerf `loaded`,
and returns `OPERATOR_ACTION` when safe recovery cannot be established.
Snapshot rename now fsyncs the containing directory. A 45-case crash matrix
exercises five mutations at nine journal/backend/observation/snapshot/completion
boundaries. `WatchEvents` is a bounded
durable sequence-cursor stream, strict root-owned host configuration supplies
paths/timeouts/frame limits, and `ALLOCATING`, `STOPPING`, and `RELEASING` are
persisted before external mutation. Approved-manifest resolution now verifies
strict identity, pinned revisions, ownership/mode/type, all hashes, amd64 ELF
artifacts, kernel config, modules, protocol range, and OCI features before
allocation; arbitrary kernel/initramfs flags are removed.

The second synchronization pass added focused daemon-wire rejection tests,
`BACKEND_TIMEOUT`, stable mutation operation IDs, checked error-state
persistence, secret-safe backend/state errors, static bundle/manifest/label/key
validation, exact terminal delete replay, and configured Kerf-pool CPU
membership enforcement. Socket/state ownership, safe-parent checks, and
approved-manifest resolution are now enforced and focused-tested. The manifest
also pins the C relay and exact transport identity/direction. Pool memory is
parsed, an explicit 1 GB allocator reserve is
subtracted, and non-absent allocations are summed under the allocation lock
before Kerf is called. A corrected live pair proves an 8 GB pool returns
`RESOURCE_EXHAUSTED` before the backend while 12 GB admits the two 4 GiB
children. The harness also now records the real privileged daemon PID; this
found and removed an orphan daemon from the earlier attempt, whose cleanup
claim has been superseded by the corrected run.

The 2026-09-04 failure-path pass observes Kerf after every failed mutation and
restores a known retryable durable state. Load, start, and stop retries have
focused coverage; an uncommitted failed create is removed from the snapshot so
it cannot block a later create, while its operation ID and error remain in the
journal. The new live harness injects one-shot pre-commit Kerf failures for
load, boot/start, and stop and explicitly proves a one-shot client may disappear
while the child remains active.

## Current audited verdict

G2 supports the happy-path lifecycle, exact mutation replay, generation checks,
basic overlap prevention, a fsynced intent journal and directory-durable atomic
snapshot replacement, configured-pool CPU enforcement, classified secret-safe
errors, durable events, strict host configuration, and transition-specific
restart recovery.

The 2026-09-04 run found that this exact pinned sysfs reports a new
instance as `ready`, not the fixture vocabulary `created`. The adapter now maps
both to runtime `CREATED`, with focused coverage. After redeployment, live
failure injection restored and retried `CREATED`, `LOADED`, and `RUNNING` for
load/start/stop respectively; a one-shot client exited while the child stayed
active; two children survived daemon `SIGKILL`; and an 8 GB pool rejected the
second 4 GiB request with `RESOURCE_EXHAUSTED` before backend creation. The
downloaded [G2 manifest and raw evidence](../../../evidence/runtime-20260904/g0-g3-final/README.md)
retain the API responses, raw `ready` observation, journal/snapshot at daemon
death and cleanup, and final resource return. This closes G2's documented
daemon-SIGKILL boundary. Host power-loss campaigns remain later hardening work.
