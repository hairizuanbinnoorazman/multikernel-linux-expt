# Docker scope-stop failure diagnostic

Source `1c309569` bounds the failure diagnostic emitted when
`dockerengine.Service.Stop` cannot quiesce its owned scope. The existing marker
and ordinary classification fields remain:

```text
MK_DOCKER_SCOPE_STOP_FAIL timeout=false canceled=false identity=true errno=0
```

Only three boolean classifications and errno 0..4095 enter the reporter. Errno
outside that range becomes zero. Original error text, engine output, paths,
credentials, request data and environments are neither formatted nor retained by
the writer. Classification still uses the original cause internally.

The default reporter has one attempt for the entire process lifetime. First
failure consumes it, including failed/short/unavailable delivery. There is no
queue or retry. At most one record of 128 bytes, one writer goroutine and one
private descriptor can remain blocked. Delivery wait selects the caller's context
and a 50 ms timer budget; encoding and scheduling are not real-time guarantees.
A canceled caller can leave even if its one best-effort write remains pending.

The writer duplicates stderr to a CLOEXEC descriptor at least 3, preserving
inherited flags and signal policy. Closed readers return EPIPE rather than
changing control flow through Go's SIGPIPE handling for descriptor 2. The private
descriptor closes on completion or process exit. Late completion is possible
following timeout; absence/presence is not a full failure history or readiness.

The scope stop still returns `Docker descendants have not quiesced` and leaves
completion unpublished. Failed cleanup retains its scope/pidfd/mount authority;
logging success, failure or silence authorizes no release. Existing retry and
[stop queue admission](docker-stop-queue-budget.md) rules remain. Kernel/cgroup/
filesystem calls themselves have separate timing and ownership contracts.

This reporter is independent of the agent's one terminal and two shutdown-stream
writers. Across those three producers, at most four blocked diagnostic writers/
descriptors can remain in one agent process. This is not a node-wide admission
bound or complete producer inventory; engine tail retention and other process/
console streams keep separate contracts.

[Local exact-parent controls](../../../evidence/runtime-20261105/g8-docker-scope-log-controls/README.md)
use zero-authority scopes: all three captured/full/closed-stderr cases fail before
the fix and pass after it across 1,000 stop calls, with unchanged failure/ownership
state. The blocked case checks one private CLOEXEC alias and inherited flags.
Classification/privacy, caller/timer budgets, 1,000 concurrent retries per budget
case, failed/short writes, existing queue controls, full runtime race and required
checks pass. No real engine or cgroup/filesystem release occurs in these controls.

Original timeout causality, actual source-qualified Docker execution/recovery,
aggregate node pressure/retention and full G7/G8 remain open. No nested
virtualization is used; direct child kernels share host trust after compromise.

The [GCE root fixture](../../../evidence/runtime-20261105/g8-docker-scope-log-gce/README.md)
passes success/zero (`8fa95336`) for the same immutable source. Seven top-level
controls cover diagnostic delivery and existing stop queue/ownership behavior;
retained ledgers/resources remain unchanged. No actual Docker stop, cgroup/
filesystem release or guest RPC is performed.

The [caller-budget proof correction](../../../evidence/runtime-20261105/g8-docker-scope-context-controls/README.md)
for source `9d6787e0` distinguishes context response from timer expiry. It uses
a one-second bounded private test budget and a 200 ms observation bound; removing
only the context branch fails both cancellation cases. Production still calls
the wrapper fixed at 50 ms, without runtime configuration. Earlier short-timer
controls did not independently distinguish those paths and remain retained.

The separate [corrected GCE fixture](../../../evidence/runtime-20261105/g8-docker-scope-context-gce/README.md)
passes success/zero (`0637da03`) for `9d6787e0`, including the independent caller/
timer cases and earlier stderr/ownership controls. Retained failed ownership is
unchanged; actual engine/cgroup/filesystem release and guest RPC remain excluded.
