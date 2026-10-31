# Bounded supervisor diagnostics

The supervisor emits version-1 JSON records with component
`containerd-shim-multikernel-v2`, scope `worker-supervisor`, fixed operation/code,
positive numeric `supervisor_pid`, canonical UTC `at`, and `owner_known: false`.
It has no validated workload generation and does not invent sandbox ownership.
PID/time metadata is an observation, not a process-incarnation attestation or
permission to signal, release or repair anything. Existing PID-file identity,
held-directory, process ownership and restart/reap rules remain authoritative.

The fixed table covers missing/inaccessible inherited listeners, executable and
directory discovery, containerd accept, worker PID residue/publication/removal,
private listener/connect, start/wait/unrecoverable exit, proxy failure, restart
and exhausted budget. Only `SUPERVISOR_RESTART` additionally carries positive
numeric `worker_pid`, first `attempt` (1–10) and `signal` (1–64). All other codes
exclude these optional fields. Records reject unknown code/operation pairs,
invalid numeric bounds, noncanonical timestamps or `owner_known: true`.

There is no error/message, argv, environment, bundle/executable/socket path,
request, event, guest output or arbitrary string argument to this producer.
Existing worker stdio forwarding is unchanged; this interface qualifies the
supervisor's own control diagnostics, not every byte written by a child process.

One supervision invocation attempts each code at most once, even after a full
or disabled sink refuses it. Crash retries and clean containerd handoffs within
that invocation do not reset the attempts. Numeric restart fields are the first
sample, not a complete restart history. A new invocation/process resets quota.
Existing ten-crash limits, backoff, clean handoff accounting and return statuses
remain unchanged.

Strict typed supervisor and [worker producers](process-diagnostics.md) share one
bounded transport per OS process: 64 queued records plus one in flight, each at
most 1024 bytes including newline. The parent supervisor and exec-created workers
have separate address spaces and therefore separate sinks. No additional sink
per record or supervision attempt is introduced. Enqueue is nonblocking. A writer
error/short write disables that process's sink; close never waits for a blocked
writer. One writer goroutine can remain blocked until release/process exit.

Delivery is best effort: startup/fatal exit, shutdown, queue pressure or writer
failure may lose records, including the first failure. No flush, durable stream,
full history or health guarantee is made. Silence does not establish readiness.
Rotation/retention, aggregate node pressure, remaining daemon/Task RPC error
paths, production deployment compatibility and full G7/G8 remain separate work.
Direct child kernels require no nested virtualization and share host trust after
child-kernel compromise.

Local controls cover all seventeen fixed codes and quota/metadata rejection,
full-queue refusal, actual ten-crash exhaustion with exact PID cleanup, private
proxy error reaping, and actual SIGKILL restart/PID cleanup with real stderr
capture or a full undrained OS stderr pipe. The [GCE fixture](../../../evidence/runtime-20261031/g8-supervisor-diagnostics-gce/README.md)
now passes for source `6e303b6c`, after the [idle-helper repair](../../../evidence/runtime-20261031/g8-supervisor-helper-controls/README.md).
The [initial failed fixture](../../../evidence/runtime-20261031/g8-supervisor-nonrace-helper-failure/README.md)
remains retained. The [actual child-Docker regression](../../../evidence/runtime-20261031/g8-live-docker-supervisor-diagnostics/README.md)
also passes for this source with normal cleanup and engine/image separation.
Supervisor faults during Docker were not injected; they retain their separate
fixture proof. The separate [cold restoration](../../../evidence/runtime-20261031/g8-live-docker-supervisor-diagnostics-cold-restoration/README.md)
also passes, preserving retained data, current/prior artifacts, four fault
fixtures and both failed cases. Earlier source-qualified Docker proofs are not
relabelled.
