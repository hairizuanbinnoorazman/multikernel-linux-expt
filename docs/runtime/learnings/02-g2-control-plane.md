# G2 learnings: recoverable control plane

## Result

Gate G2 passed locally and on GCE. `mkruntimed` now exposes a strict v1 JSON
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
