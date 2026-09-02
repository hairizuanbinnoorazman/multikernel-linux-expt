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

The G2 journal lived under `/tmp` and was lost during later G3 compatibility
resets, so the retained live artifact is the pass log rather than the raw G2
journal. The same journal/restart semantics remain covered by repository unit
tests using durable temporary directories.

## Claim-to-proof audit

The local lifecycle, replay, stale-generation, overlap, concurrent allocation,
second-sandbox pool reuse, backend adapter, and known-state reconciliation
tests pass under `cd runtime && GOCACHE=/tmp/mk-go-cache go test ./...`. The
live file [`g2-control-plane.log`](../../../evidence/runtime-20260831/g0-g3-gce/g2-control-plane.log)
contains only a 71-byte restart/reconcile pass marker and cleanup message. It
does not retain the journal, snapshot, request/response transcript, daemon
logs, or operation-boundary assertions needed to independently reproduce the
broader prose claim.

Code inspection also confirms the important remaining gaps: reconciliation
iterates snapshotted sandboxes rather than backend inventory or orphan journal
intents; every disagreement becomes `OPERATOR_ACTION`; snapshot rename does not
fsync the containing directory; some error-path store writes are ignored;
`WatchEvents`, tombstones, strict host-config loading, approved-manifest
resolution, and observable intermediate states are absent. Consequently only
the named unit tests and narrow live marker are retained claims; all stronger
G2 conformance statements remain open in the remediation checklist.

The second synchronization pass also confirmed that backend timeouts are still
reported generically as `BACKEND_FAILURE`, mutation errors do not reliably
carry stable operation IDs, backend/path details can cross the API in raw error
strings, and socket/state ownership and safe-parent checks are incomplete.
Create validates a small sandbox subset, but not the approved kernel manifest,
bundle/path trust, label and key bounds, live APIC eligibility, configured Kerf
pool membership, or usable pool memory. These are current implementation gaps,
not only missing historical evidence.

## Current audited verdict

G2 supports the happy-path lifecycle, exact create replay, generation checks,
basic overlap prevention, a fsynced intent journal, atomic snapshot replacement,
and conservative known-sandbox reconciliation. It remains open because orphan
external resources and pre-snapshot crash windows can be missed, failure
classification and persistence are incomplete, the event/config/manifest
contracts are not enforced, and the retained GCE log cannot independently
audit the restart sequence.
