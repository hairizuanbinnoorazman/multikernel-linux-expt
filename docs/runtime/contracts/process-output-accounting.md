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
exit transition. Recovery that discovers a newly stopped guest without draining
its streams records `unavailable`. Legacy recovery records with no field remain
unobserved. Existing stopped reports survive reconstruction and are validated
against the persisted process and offsets; they grant no new lifecycle authority.
The optional field extends recovery schema 4; older strict readers refuse such
records and must not be used for a downgrade while they exist.

This is the guest-buffer accounting layer. Host FIFO disposition/retry behavior,
worker recovery, Docker logger rotation/loss and payload/heap/RSS peaks require
separate evidence. Full G7/G8 and end-to-end loss/peak accounting remain open.
