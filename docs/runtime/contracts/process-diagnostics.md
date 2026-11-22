# Bounded shim process and network diagnostics

Namespace-holder setup failures also emit `NETWORK_NAMESPACE_HOLDER_FAILED`
(`prepare-network-namespace-holder`) using the endpoint's sandbox/generation and
init-process ownership. The record contains no namespace path, helper stderr or
underlying error. The actual `nsenter` subprocess sends stderr directly to
`/dev/null`, without a buffering pipe or drain worker, so it cannot expose raw
paths or block helper termination on an unread shim log FIFO. Setup retains its
internal error causes, holder validation, rejected-holder cleanup and refusal
to replace a holder whose stop failed. Success emits no failure record.
The existing bounded lossy sink handles delivery; this is not durable evidence
or authority to release an uncertain network owner. Vendored shim startup errors
remain a separate boundary.
The [source-bound comparison and GCE proof](../../../evidence/runtime-20261109/g8-namespace-helper-diagnostics/README.md)
reproduce the old actual-nsenter raw-path/full-pipe failures and qualify the
corrected refusal transports, fixed owner schema, blocked writer and retained
holder contracts. Actual successful namespace entry and child-Docker execution
remain separate qualifications.

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

The shared sink also reserves at most 1 MiB of encoded diagnostic bytes,
including newlines, over its lifetime. Fresh process/monitor identities and the
library, event, network and supervisor producers all spend that same sink budget.
A continuously drained writer cannot bypass it. Valid submissions reserve bytes
before queue admission; queue-full refusals are not refunded, so this counter
is a conservative reservation budget rather than delivered-byte accounting.
Reservations use at most eight compare-and-swap attempts and refuse contention
without waiting for writer I/O or a lifecycle lock. The counter cannot wrap to
restore allowance. Budget refusals increment the existing dropped-record count.
The first frame that cannot fit closes the allowance permanently; a smaller
later frame cannot reclaim its unused tail. Exact-limit reservations still
deliver their last frame. Contention refusals do not close the allowance.
After closure, producers skip timestamp formatting, validation and JSON encoding;
library entries are still sanitized before this early return.
Once exhausted, the process retains its durable lifecycle behavior and loses
further best-effort diagnostics. A new process has a new budget; this is not a
host-wide or sandbox-wide lifetime quota. Task stdout/stderr retains its separate
output/logging policy.

The [source-bound native comparison](../../../evidence/runtime-20261122/g8-diagnostic-lifetime-budget/README.md)
proves the prior fast-reader bypass and the new lifetime cap on GCE, alongside
concurrency/boundary/producer-sharing controls and the full shim race suite.
The subsequent [ordinary child compatibility summary](../../../evidence/runtime-20261122/g8-diagnostic-budget-child/README.md)
qualifies nested builds and cold restoration at this source. Diagnostic-budget
exhaustion during actual child execution remains a separate checkpoint.
The [exhaustion progress controls](../../../evidence/runtime-20261122/g8-diagnostic-exhaustion-progress/README.md)
subsequently qualify zero-allocation refusal and exact durable monitor completion
under exhaustion on GCE using a synthetic guest backend; they do not upgrade the
older actual-child source qualification.

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
successful guest teardown with warnings, and deferred Task-event publication/
startup replay. Network monitors snapshot endpoint
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
not qualify later network/teardown or deferred-event changes; their separate
source-specific results follow below. Supervisor diagnostics still need separate
redaction/pressure qualification.

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
diagnostic artifacts preserved. New producer real-stderr-pipe coverage remains
unqualified. A subsequent [actual child-Docker regression](../../../evidence/runtime-20261030/g8-live-docker-network-teardown-diagnostics/README.md)
now passes ENOSPC recovery, builds, exact engine/image boundary, report binding
and normal cleanup for source 038ce648. It does not inject the private network/
teardown faults during Docker. Earlier d88680b5 results retain their source and
scope. No additional queue or per-record
writer goroutine is introduced.

The [cold restoration](../../../evidence/runtime-20261030/g8-live-docker-network-teardown-diagnostics-cold-restoration/README.md)
for source `038ce648` also passes with original transport, retained data, all eighteen current and 75
prior diagnostic artifacts, and both private fault fixtures preserved. This does
not extend new producer real-stderr-pipe coverage or the remaining Task RPC,
event/supervisor/daemon log, rotation/retention, aggregate pressure and full G7/G8
qualification scope.

Event publication/replay diagnostics use `EVENT_PUBLICATION_DEFERRED` and
`EVENT_REPLAY_DEFERRED`. Lifecycle setup installs an immutable, validated owner
snapshot after CreateSandbox or recovered bundle-identity validation; successful
create rollback clears it. Atomic access avoids re-entering the service or event
mutex from lifecycle callers. The record identifies the worker's observed owner,
**not the owner or sequence of a particular pending event**. It carries no topic,
sequence, event payload or broker error and grants no recovery authority.

Each of those two codes receives at most one enqueue attempt per service/worker,
including rejection by a full/disabled sink. An owner change does not reset that
quota. Unknown/invalid ownership is suppressed without consuming the attempt;
later validated ownership can still report. Startup replay retains the existing
durable queue and retry behavior. Queue persistence, ordered replay, broker
acceptance and acknowledgement remain independent of this best-effort logging.
No additional sink or per-record writer goroutine is introduced. Local controls
for this extension cover actual durable queue reconstruction, exact payload order,
unchanged queue bytes on failed startup replay, blocked injected-writer behavior,
concurrent quotas and ownership replacement, and create/recovery/rollback wiring.
The [local race controls](../../../evidence/runtime-20261030/g8-event-diagnostics-controls/README.md)
and [GCE private test-binary fixture](../../../evidence/runtime-20261030/g8-event-diagnostics-gce/README.md)
for source `3dbf1cf5` now pass this checkpoint, preserving primary state and all 93
earlier diagnostic artifacts, with no fixture helpers left. A subsequent [actual child-Docker regression](../../../evidence/runtime-20261030/g8-live-docker-event-diagnostics/README.md)
now passes ENOSPC recovery, builds, exact engine/image boundary, report binding
and normal cleanup for source 3dbf1cf5. It does not inject private broker faults
during Docker; durable replay under injected-writer pressure retains its separate
fixture proof. New event real-stderr-pipe coverage remains unqualified. Earlier
proofs retain their original source and scope. Supervisor/daemon logs, Task RPC errors, log rotation/retention, aggregate
pressure and full G7/G8 remain open.
Direct child kernels require no nested virtualization and share host trust after
child-kernel compromise.

The [cold restoration](../../../evidence/runtime-20261030/g8-live-docker-event-diagnostics-cold-restoration/README.md)
for source `3dbf1cf5` also passes with original transport, retained data, eighteen
current and 93 prior diagnostic artifacts, and all three private fault fixtures
preserved. This does not extend injected broker failure during Docker, new event
real-stderr-pipe coverage, Task RPC error redaction, supervisor/daemon logs,
rotation/retention, aggregate pressure or full G7/G8 qualification.

The separate [supervisor producer](supervisor-diagnostics.md) now uses the same
bounded per-process transport with its own strict schema. It explicitly reports
unknown workload ownership and does not accept errors, paths or payloads. The
parent supervisor and exec-created workers each own one sink in their separate
OS processes; this does not increase the worker's existing queue limits.

The [default stderr transport](diagnostic-stderr.md) also survives reader loss
without SIGPIPE termination, using one private CLOEXEC descriptor per in-flight write.

## Instrumented worker qualification

An explicitly tagged Go test executable can consume the default sink's actual
allowance before entering the production worker. Build it with `go test -c
-tags=diagnostic_exhaustion_worker` for the shim package. Its synthetic preload
does not change the cap, guest transport, monitors, task journals or lifecycle
code. A worker writes an exclusive mode-0600 private fixture receipt before
production entry; existing files, symlinks and hard links refuse replacement.
A worker restart therefore cannot silently reuse that receipt.

This entry exists only in `_test.go` files. Ordinary `go build`, including a
build given the same tag, excludes it. Version/info discovery does not exhaust
the sink; test flags retain the normal test runner. The explicit
`--diagnostic-exhaustion-probe` command qualifies preload/receipt behavior only.
It is not proof of a child lifecycle.

This executable is an instrumented fault fixture. It must not replace a normal
trial packet's shim or inherit that packet's acceptance. A real-child trial must
explicitly admit the variant, bind its binary/source and worker receipts, and
verify the complete Docker build lifecycle plus cleanup and restoration. That
integration and execution remain pending.

The committed packet builder now exposes `--shim-variant diagnostic-exhaustion-test-worker` for that fixture. Its manifest declares the
variant and `instrumented_test_executable: true`; production remains the default.
Only the shim changes build form (`go test -c` with the explicit tag). The other
seven packet binaries retain their ordinary build/copy paths. Read-only staging
can measure the packet without granting execution authority. Unknown, partial
or contradictory variant metadata is refused. Ordinary live-trial and audit
entry points explicitly reject the instrumented variant; historical production
packets without variant fields remain supported. Receipt-bound real-child trial
admission is still pending.

The [receipt-bound integration](../../../evidence/runtime-20261122/g8-diagnostic-child-binding/README.md)
adds explicit parent, workload and audit admission for the logging scenario.
Version-2 private preload receipts bind native boot, start ticks and bundle
identity. Live captures check the supervisor marker and exact executable before
and after the held child matrix; archive acceptance links both captures to the
complete build/writer checkpoint and parent lifecycle. These captures grant no
cleanup, reuse or current-process authority after the workload exits. Actual
new-source child execution and cold restoration remain unqualified until their
independent evidence is collected.

The instrumented executable also has a native qualification-only
`--diagnostic-exhaustion-binding-hold` mode. It writes the real preload receipt
and a fixed readiness marker, then stays alive until its owning test terminates
and reaps it. The live-binding tests use that exact staged binary through
`MK_DIAGNOSTIC_WORKER_TEST_BINARY` and its pinned
`MK_DIAGNOSTIC_WORKER_TEST_SOURCE`; local controls build the same explicit tag.
This avoids using a renamed host utility as the live producer. Normal `go build`
excludes this hold mode, and the hold creates no child kernel or Docker task.
