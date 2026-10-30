# Bounded shim process and network diagnostics

The shim's stdin/output/wait and network monitors, plus successful init teardown
with guest warnings, emit version-1 JSON diagnostic records to
worker stderr. Each record contains only `component`, `operation`, a fixed `code`,
validated sandbox ID, generation and agent process ID, UTC `at`, and optional
numeric `bytes`, alongside `version: 1`. Sandbox/process identifiers are 1–128
ASCII identifier characters starting with an alphanumeric character; generations
are exactly 32 lowercase hexadecimal characters. Invalid contexts are suppressed
without a raw-error fallback. Operation/code pairs come from the fixed host table.

Raw agent/backend errors, stdout/stderr, stdin, command lines, environments,
configuration, paths, credentials and arbitrary messages are never accepted by
this diagnostic interface. Workload stdout/stderr still follows the existing
Task v2 output path; it is not copied into these host diagnostic records.

A monitor invocation (or successful init-delete attempt) makes at most one
enqueue attempt per code, even when the sink rejects it. This replaces retry-loop error printing and repeated output-drop
messages. The optional byte count is a sample from the first reported chunk,
not an aggregate loss count. Restarting a worker or starting a new monitor resets
that monitor's attempts; this is not a lifetime quota per sandbox or host.

Each worker lazily creates one sink goroutine with a 64-record queue. Encoded
records, including newline, are capped at 1024 bytes. A blocked writer can hold
one additional in-flight record; queued record payload is at most 64 KiB.
Enqueue is nonblocking, with full-queue rejection. The sink never acquires the
service lock or calls a guest, backend, filesystem repair or task/event journal.
A writer error or short write disables the sink, without retrying or appending
later records to a possibly incomplete JSON line. Close does not wait for a
blocked writer. That single goroutine can remain blocked until the writer is
released or the worker process exits; no per-record goroutines accumulate.

Delivery is best effort. Queued or in-flight records can be lost at shutdown,
queue pressure or sink failure; no flush, durability, complete stream or health
claim is made. In particular, silence cannot establish health. The durable exit
and event/recovery semantics are unchanged: diagnostic pressure must not fabricate
completion, advance output offsets early or prevent the exact exit from being
committed after guest recovery.

The code table covers stdin replay/intent/delivery/read/drain/close, output
read/acknowledgement, stdout/stderr pressure/truncation, process wait/validation,
exit persistence, signal acknowledgement/persistence and exit-event queue/
acknowledgement, fatal network-device read/write errors, network REPORT failure
and successful guest teardown with warnings. Network monitors snapshot endpoint
sandbox ID/generation before starting and use process ID `init`, the sandbox's
network owner. They never acquire the service mutex for diagnostics while a
lifecycle caller may be joining them. Network state, addresses, descriptors,
packets, backend errors and guest teardown notes are excluded. Teardown emits
`GUEST_TEARDOWN_WARNINGS` only after host cleanup succeeds; failures still follow
the existing Task RPC error path, which this log interface does not redact.
Other shim/supervisor/daemon log sources and host log rotation,
retention, aggregate node pressure and complete G8 diagnostics still require
separate qualification. Direct child kernels require no nested virtualization
and share host trust after child-kernel compromise.

The [local race controls](../../../evidence/runtime-20261030/g8-process-diagnostics-controls/README.md)
and [GCE private agent-fault fixture](../../../evidence/runtime-20261030/g8-process-diagnostics-gce/README.md)
qualify this bounded monitor checkpoint for source d88680b5. The GCE fixture
uses actual stderr capture and a full undrained OS pipe while the production
monitor recovers and commits exact exit 37 and its event. It allocates no child
kernel and does not qualify a new actual Docker run, all host logs or full G8.

A subsequent [actual child-Docker regression](../../../evidence/runtime-20261030/g8-live-docker-process-diagnostics/README.md)
now passes for the same source, including ENOSPC recovery, builds, guest/primary
engine-image separation and normal cleanup. It qualifies ordinary lifecycle
compatibility with this implementation; it does not inject the private monitor
faults during Docker or extend coverage to other log producers.

Its separate [cold restoration](../../../evidence/runtime-20261030/g8-live-docker-process-diagnostics-cold-restoration/README.md)
passes with original transport restored, retained data and all current/prior
diagnostic artifacts preserved, including the private fault fixture. This does
not qualify the subsequent network/teardown extension. Event publication and
supervisor diagnostics still need separate redaction/pressure qualification.

The subsequent network/teardown extension adds `NETWORK_DEVICE_READ_FAILED`,
`NETWORK_DEVICE_WRITE_FAILED`, `NETWORK_REPORT_FAILED` and
`GUEST_TEARDOWN_WARNINGS` to the fixed table. Local focused race controls cover
repeated private report errors, fatal device faults, joins while the service
mutex is held, and exact exit-37 init deletion with warning text excluded.
The blocked-writer controls for these new producers use an injected sink writer;
the older process-monitor controls retain their real OS stderr-pipe scope.
The source-038ce648 [local race controls](../../../evidence/runtime-20261030/g8-network-teardown-diagnostics-controls/README.md)
and [GCE private test-binary fixture](../../../evidence/runtime-20261030/g8-network-teardown-diagnostics-gce/README.md)
now pass this private-writer checkpoint with primary state and all 75 prior
diagnostic artifacts preserved. New producer real-stderr-pipe controls and a
fresh child-Docker regression remain pending; the earlier source-d88680b5
Docker/restoration proof is not relabelled. No additional queue or per-record
writer goroutine is introduced.
