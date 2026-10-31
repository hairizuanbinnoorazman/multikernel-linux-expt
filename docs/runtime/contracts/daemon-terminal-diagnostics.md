# Bounded daemon terminal diagnostics

In daemon mode, `mkruntimed` reports host configuration/pool validation, durable
state, service initialization, recovery/reconciliation, listener and final idle
pool release failures through fixed terminal records. `mknetd` reports egress,
durable state, boot-ID read, service initialization/reconciliation, socket-parent
and listener failures through the same interface. Its internal stage wrapper
retains the cause for classification but does not format it into the record.
The fixed table currently contains fifteen runtime and eight network codes.

Each version-1 JSON record has exactly eight fields: `version`, `component`
(`mkruntimed` or `mknetd`), `scope: "daemon-terminal"`, fixed `code` and `operation`,
positive current `pid`, `owner_known: false`, and canonical UTC `at`. No error,
path, durable-state key, workload ID, command output, environment, request or
arbitrary string can be passed to the producer. PID/time observations neither
attest process incarnation nor authorize signaling, repair or release.

The process attempts one record total, at most 1024 bytes including newline.
A valid attempt consumes the quota even on a missing/broken/blocked writer.
Concurrent or repeated calls cannot add writer goroutines or records. Invalid
codes do not consume quota. There is no queue, retry or accumulating log stream.
The caller waits for delivery for at most a fifty-millisecond timer budget;
scheduling and ordinary fixed-size encoding are not real-time guarantees.
A blocked write can retain one goroutine, one record and one private descriptor
until stderr is released or the process exits. The private descriptor is at
least 3 and CLOEXEC, so a closed stderr reader returns EPIPE instead of Go's
special SIGPIPE termination behavior for writes to descriptors 1/2. The reporter
does not change inherited descriptor flags or process-wide signal handling.
The private descriptor closes when its write finishes or the process exits.

Delivery is best effort: timeout, early exit, closed stderr, write failure or
short write may lose or truncate the sole record. A timed-out write may complete
later if the process remains alive. There is no flush, durability, full history
or readiness guarantee. Silent logs do not establish a healthy service.
Existing exit statuses and cleanup order remain authoritative: runtime listener
shutdown attempts idle-pool release before reporting its terminal result, and
network listener defers server closure before returning to the main reporter.
Host allocation refusal no longer automatically dumps the full host-check report
into the daemon journal; the explicit `mk-host-check` CLI remains separate.

Version output, flag/parser/inspection-mode validation and explicitly requested
inspector/doctor/bundle output retain their existing CLI contracts. In particular,
inspector RPC/backend errors and flag parse errors are not redacted or bounded by
this daemon-mode terminal interface. RPC error bodies, other producers, aggregate
node pressure, rotation/retention and complete readiness/provenance still need
separate qualification. These are terminal control diagnostics, not a filter for
all subprocess or workload bytes.

Local controls exercise the fixed table, quota, concurrent calls, writer failures,
short writes, blocked writes, actual SIGPIPE refusal, descriptor bounds/CLOEXEC
and real daemon CLI failures with captured, full-undrained and closed stderr.
Source-specific GCE and new actual child-Docker qualification remain pending.
Earlier source-qualified proofs are not relabelled. Direct child kernels require
no nested virtualization; the target compute instances cannot use nested
virtualization as a containment solution. Primary and child kernels share host
trust after child-kernel compromise; full G7/G8 remain open.
