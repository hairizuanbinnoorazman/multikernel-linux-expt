# Bounded process-stream diagnostics

The shim's stdin/output/wait monitors emit version-1 JSON diagnostic records to
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

A monitor invocation makes at most one enqueue attempt per code, even when the
sink rejects it. This replaces retry-loop error printing and repeated output-drop
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
acknowledgement. Other shim/supervisor/daemon log sources and host log rotation,
retention, aggregate node pressure and complete G8 diagnostics still require
separate qualification. Direct child kernels require no nested virtualization
and share host trust after child-kernel compromise.
