# Guest process-output byte accounting

The optional capability `process-output-accounting-v1` adds authenticated
`ProcessOutputAccounting` with body `{"id":"PROCESS_ID"}`. Older agents remain
compatible with ordinary output reads. An absent feature or unavailable report
cannot be interpreted as zero loss. The request shares existing sandbox,
generation, endpoint, token and anti-replay checks. Use the owning control client
or a coordinated handoff; read-only buffer observation still advances protocol
sequence and must not invalidate another controller's session.

Version 1 returns process ID, guest PID/status and separate stdout/stderr counter
snapshots. Each stream is sampled coherently under its buffer lock. The pair and
process state are not an atomic process history. ID/PID/status are selector
metadata, not process-incarnation attestation or authority for completion,
cleanup, storage release or kernel operations. After deletion the method refuses
rather than fabricating empty counters. Quiesced agents retain existing refusal.

For each stream, the exact invariant is:

```text
observed_bytes = acknowledged_bytes + buffered_bytes + pending_bytes + dropped_bytes
```

Observed bytes count input admitted into the buffer writer. Pending bytes count
admitted input still awaiting buffer space. At the existing 100 ms backpressure
timeout, remaining input becomes dropped bytes; writer return values and existing
truncation behavior stay unchanged. A zero-copy counter observation acknowledges
nothing. `ReadProcessOutput` advances acknowledgement only through its existing
offset behavior; a replayed read does not acknowledge again.

The retained payload limit remains 4 MiB per stream. `buffered_peak_bytes` is the
maximum retained payload length and never resets on acknowledgement. It is not
allocated buffer capacity, peak RSS, host FIFO delivery or Docker-log retention.
The truncation flag is equivalent to a nonzero dropped count. Integer overflow
poisons accounting, refuses the new write before mutation and never wraps into
fresh allowance; `accounting_exhausted` reports cannot qualify exact loss.

The shared Go validator checks version, expected ID/PID, status, payload bounds,
high-water consistency, truncation and conservation using subtraction to avoid
forged-sum overflow. Wire counters are unsigned integers. Replies contain no
stdout/stderr, errors, command lines, paths, environment or credential material.
No observation consumes, clears or repairs retained output.

The host shim negotiates this optional feature through its owning client. After
the final empty output read and a separately validated `WaitProcess`, it requests
one final report under the ordinary bounded agent I/O deadline. A valid report
must be STOPPED, conserve bytes, have no pending or buffered input, and match the
process PID and both durable output offsets exactly. It is persisted alongside
the verified exit in the private recovery record as `final_output_accounting`
with state `validated`. This qualifies guest-buffer counters only; nil stream
destinations and FIFO replay do not become delivery evidence.

An older agent records `unsupported`; an advertised but unavailable, exhausted
or invalid report records `unavailable`. Neither carries a report or implies
zero loss. Failure of optional observation does not block the separately verified
exit transition. Recovery that discovers a newly stopped guest validates its
wait identity, reopens only the bound output destinations and starts the ordinary
drain monitor after durable reconstruction. It does not reopen stdin, resize or
signal that exited guest. The saved process remains nonterminal until remaining
output, final accounting and a separately checked exit commit through the monitor.
Unfinished write intents retain their conservative replay exposure; draining after
a crash does not establish exactly-once physical delivery. Output-path replacement
refuses recovery rather than discarding the tail to publish an early exit.
The final record optionally carries `recovery_drain` with the stdout/stderr
offsets and bounded in-flight byte counts observed after that stopped guest's
wait identity was validated. Initial offsets cannot exceed final offsets, and
intents cannot exceed 4,096 bytes or overflow their initial offset. The marker
describes this publishing worker's reconstruction; it does not attest to process
incarnation or grant recovery/cleanup authority. Ordinary monitors omit it, and
absence in older records remains unobserved. Unsupported/unavailable guest
accounting may retain the marker but still cannot qualify loss. Deleted-exec
retention copies the marker by value. The ordinary finite-exec trial refuses a
new recovered-drain marker rather than promoting a fault run into its ordinary
scope; an actual-child recovery checkpoint must bind the changed worker and
qualify that separate path.

The lab's `runtime_shim_worker_fault.WorkerFault` is a scoped injector primitive,
not a recovery or cleanup command. Its caller must be the exact live disposable
workload unit's main process and invocation, in that unit's private cgroup/mount
scope. Historical boot/epoch and wrong unit scope refuse before target lookup.
Fresh source/bundle/marker/parent/incarnation observations bind the worker and
supervisor to pidfds; every fault rechecks the live scope and identities. There
is no numeric-PID signal fallback or archive-to-authority handoff. It temporarily
stops the supervisor and worker, kills only that held worker, observes its pidfd
exit under the caller's deadline and resumes the bound supervisor. Context exit
attempts to resume both bound processes even after timeout, late observation or
another resume error. This primitive alone does not qualify an actual-child
fault: the complete trial still needs bounded producer/counter capture, a
changed live worker, full lifecycle evidence and independent epoch-bound audit.
Already persisted stopped records keep their original accounting; legacy records
with no field remain unobserved. Existing stopped reports are validated
against the persisted process and offsets; they grant no new lifecycle authority.
The optional fields extend recovery schema 4; older strict readers refuse such
records and must not be used for a downgrade while they exist.

Host recovery also records `output_disposition` version 1 with separate stdout
and stderr counters. For each stream, the durably acknowledged offset equals
`written_bytes + no_destination_bytes + pressure_dropped_bytes + unknown_prefix_bytes`.
Written bytes count committed complete-chunk writer success. Unconfigured
streams are explicitly discarded; they do not count as written. The existing
30-second pressure policy remains unchanged. Legacy offsets enter as an unknown
prefix and cannot become retrospectively qualified delivery.

Before a configured nonempty write, the shim persists `in_flight_bytes`, bounded
by the existing 4,096-byte chunk, without output contents. It makes no write if
that publication fails. Completion clears the intent and commits the disposition
with the offsets in one recovery-file replacement. An offset-publication failure
retains the durable intent and unchanged offsets; retry must first persist the
uncertainty. Recovery or retry of an unfinished intent adds its length to
`replay_exposure_bytes`. A partial or erroneous nonzero writer result also adds
the chunk length conservatively. A definite zero-byte result adds no exposure
when its completion publication succeeds. Snapshot updates never mutate the
previous durable accounting object.

Replay exposure is an upper bound for uncertain write attempts, not an exact
duplicate count. A crash before the write itself can produce exposure without
physical duplication. Exposure arithmetic saturates and sticks
`exposure_exhausted` on overflow; exhausted accounting cannot qualify a finite
bound. Unresolved intents, unknown prefixes and nonzero exposure remain visible
and cannot qualify exactly-once delivery. Observations and these counters grant
no lifecycle or cleanup authority. A newly recovered stopped process can retain
an unresolved intent; its mere stopped state does not settle that uncertainty.

Exec deletion retains a counter-only `completed_output_accounting` tail in the
same private recovery record while the container's init is retained. The process
removal and tail append commit atomically, or both roll back. At most 16 records
survive, with continuous sequence numbers and exact recorded-deletion/eviction
counts. These counts start when the tail is initialized; they do not claim a
complete history before an upgrade. Overflow leaves the existing tail intact
and sticks `accounting_exhausted`; further deletions cannot fabricate new
observations. Legacy absence remains unobserved. Deleted exec IDs can be reused;
sequence numbers distinguish retained observations and are not process-incarnation
authority. Container deletion may remove the entire bundle; this tail is not an
external archive or cleanup receipt.

Authorized host operators can inspect it without a second agent session:

```sh
containerd-shim-multikernel-v2 --inspect-completed-output \
  --bundle /absolute/private/bundle --namespace moby --task TASK_ID
```

The standalone command validates private file ownership/bounds, strict recovery
semantics, task/namespace selectors and the actual bundle identity before
returning a typed snapshot. It emits no stream contents, stdio paths, command
lines, environment or tokens. Its two-second observation and single 50 ms output
attempt bound the standalone process; do not use its worker as a resident poller.
Replies are limited to 64 KiB. Missing history is `unobserved`, exhausted history
is `unqualified`, and unavailable/changed/invalid state refuses with fixed codes.
An observed private snapshot does not attest to a live worker, child kernel or
Docker delivery. Caller-side trial/source/epoch and live identity checks remain
necessary before using it as evidence.

The held logging trial has a separate opt-in finite-exec checkpoint. The parent
requires `MK_MODULE_TRIAL_CHILD_OUTPUT_ACCOUNTING=1` with the logging scenario;
the workload uses `--child-output-accounting`, and independent audit must request
`--require-child-output-accounting`. Missing or mismatched scope refuses. The
probe runs after the complete nested build matrix while the child is retained.
It emits exactly 8,192 zero stdout bytes and 17 fixed stderr bytes, then exits 37.
The existing 30-second command bound applies. A separate five-second observation
window and at most 25 polls await Docker's ordinary exec deletion.

Before/after private snapshots must share the container, sandbox generation,
bundle identity and live worker binding, and show exactly one new retained exec.
The unchanged tail prefix, continuous sequence, final guest reports, logical
dispositions and received bytes must agree. Guest loss, host drops, unknown
prefixes, pending intents, replay exposure, exhaustion and numeric aliases refuse
qualification. Recomputed receipt checks bind the observations to the existing
child build/logging checkpoint, source executable, boot and workload invocation.
This qualifies only that finite probe when actually run; preparation/native tests
do not establish actual-child success. The full log writer, Docker logger
retention/loss and worker-fault mixtures need separate accounting evidence.

This covers guest-buffer and logical host-disposition accounting. Actual-child
integration, worker recovery under real faults, Docker logger rotation/loss and payload/heap/RSS peaks require
separate evidence. Full G7/G8 and end-to-end loss/peak accounting remain open.
