# Protocol v1

Daemon messages use length-bounded JSON over a root-owned Unix stream socket.
Agent messages use the same envelope over Multikernel AF_VSOCK. Exactly one
JSON object is permitted per frame; duplicate and unknown fields are rejected.
Framing writes must either deliver the complete frame or fail. Shared write
handling completes injected short-success writes and rejects zero progress so
a faulty wrapped transport cannot silently truncate a request or response.
Server cancellation closes both its listener and every accepted connection,
including a peer stalled before completing a frame. Returning for any reason
unregisters and joins those callbacks; concurrent servers cancel their derived
handler context when the accept loop ends.
Daemon and mknetd accept loops default to 128 concurrent handlers, cannot be
configured above 1,024, close an authenticated connection when no slot remains,
and join every admitted handler before server return.

Every request contains `version: 1`, request ID, method, and a typed body.
Mutations also contain sandbox ID, generation when one exists, and idempotency
key. Every response echoes version/request ID and contains either a typed body
or one structured error, never both or neither. Response envelopes and typed
bodies reject unknown and duplicate fields. Daemon responses exceeding the
one-MiB wire bound, or bodies that cannot be encoded, become a bounded INTERNAL
error retaining the originating request ID. Agent envelopes additionally contain sandbox ID,
generation, endpoint, monotonically increasing sequence, and an HMAC-SHA256
over the canonical frame using the sandbox token. Frames over 1 MiB and
sequence replay are rejected before body decoding.

V1 authenticates requests but does not add a reply MAC. A reply is bound to the
authenticated, point-to-point session by its sequence number and is accepted
only on the connection that carried the request. This is a trusted-node design,
not protection from a hostile sibling kernel or a peer able to tamper with the
inter-kernel transport. Adding cryptographic reply integrity requires a new
advertised protocol capability or major version rather than an unannounced wire
change.

Daemon methods are `NodeInfo`, `ListSandboxes`, `CreateSandbox`,
`CancelCreateSandbox`, `LoadSandbox`, `StartSandbox`, `StopSandbox`,
`DeleteSandbox`, `SandboxState`, and `WatchEvents`. `CancelCreateSandbox` is an
internal rollback operation bound to the exact original create idempotency key
and full configuration fingerprint; it is not a general deletion shortcut.
Agent methods are `Capabilities`, `CreateProcess`,
`ExecProcess`, `StartProcess`, `SignalProcess`, `AcknowledgeSignal`, `ResizeProcess`, `WriteProcess`,
`CloseProcessStdin`, `ReadProcessOutput`, `WaitProcess`, `StateProcess`,
`DeleteProcess`, `ConfigureNetwork`, `ExchangeNetwork`, `CloseNetwork`,
`Quiesce`, and `Shutdown`. Stdin writes and output reads are limited to 64 KiB
per request; network exchange carries at most one 65,535-byte packet in each
direction.
The advertised `stdin-offset-v1` capability adds an `offset` to `WriteProcess`
and returns the next acknowledged offset. The guest accepts only the exact next
offset, except that it idempotently acknowledges a replay of the most recently
accepted offset when both length and SHA-256 match. This makes response-loss
replay safe without admitting changed bytes, gaps, or reordering. If the local
process writer accepts a prefix and returns an error, the guest retains that
prefix position and an exact replay resumes with the unaccepted suffix; the
acknowledged offset advances only after the complete chunk is accepted.
Omitting the offset retains the original protocol-v1 behavior for an older
controller but does not provide reconnect-safe replay.
The advertised `signal-operation-id-v1` capability adds a generation-scoped
32-character lowercase hexadecimal `operation_id` to `SignalProcess`. The
guest retains a bounded ledger of the exact process ID, signal, and result. An
exact replay returns that result without signaling again, including after the
process has exited; reuse for another process or signal is rejected. The guest
refuses a new mutation before signaling when the ledger is full and never
evicts an unresolved result. `AcknowledgeSignal` idempotently retires an exact
result only after the controller has durably observed it; process deletion also
retires that process's results. Omitting the field retains legacy one-shot
behavior and is not reply-loss safe.
The advertised `readonly-bind-inputs-v1` capability means the projected OCI
configuration may contain sanitized bind records whose source equals their
absolute guest destination and whose ordered options are exactly
`bind,nodev,noexec,nosuid,ro`. These records refer only to primary-materialized
content in the private guest root; they never authorize guest access to the
original host source. The agent rejects overlapping or runtime-owned
destinations and bind-remounts each real directory or regular file read-only before process
creation.
`ExecProcess` names an existing parent process and inherits its already
validated container root; it cannot supply an arbitrary root path. Process
state and wait replies contain metadata only. Output is retrieved through
independent offset-based `ReadProcessOutput` chunks, so retained output cannot
make a state reply exceed the frame limit. Each stream retains at most 4 MiB,
briefly backpressures a full buffer, and then marks explicit truncation rather
than blocking a child indefinitely when its controller disappears.
For each returned stream, the next offset is exactly the requested offset plus
the returned byte count without unsigned overflow. The controller rejects a
chunk beyond its requested limit, a gap or regression, or an output-state value
other than `RUNNING` or `STOPPED` before writing output or advancing recovery.

An agent transport disconnect leaves managed processes and the last accepted
sequence intact. The same authenticated controller may reconnect and continue
with the next sequence; restarting at sequence one is rejected as replay. A
completed connection unregisters its server-cancellation callback immediately,
so reconnect churn cannot retain one waiter and connection reference per old
session. Cancellation still closes and unblocks a currently active connection.
`Quiesce` is an idempotent authenticated commit point: it requires an empty
process registry, performs the storage flush/read-only transition once, and
seals the agent against subsequent process or network mutation. It remains
available across transport reconnect so a lost reply can be retried safely.
`Shutdown` retains the legacy direct-quiesce behavior, but the runtime first
confirms `Quiesce`; after that confirmation, a lost terminal reply is safe to
treat as completion. A successful `Shutdown` reply is the final reply on the
session. The agent then syncs and invokes child poweroff; the pinned
Multikernel spawn-kernel machine operations convert that action into a
child-scoped notification and CPU park, not a platform reset.

`WatchEvents` is a resumable bounded event stream: its body contains an
exclusive `after_sequence` cursor and optional `limit` (default 128, maximum
1024). The response contains ordered version-1 completion events from the
durable journal. A client repeats the call with the last observed sequence;
restarts therefore neither invent events nor require an in-memory subscriber.

Minor additive evolution requires an advertised capability and optional field
defined by the schema. A peer that cannot safely ignore an addition rejects
it as `UNSUPPORTED`. Major-version mismatch is always rejected.

The machine-readable envelope schemas are
[`daemon-protocol-v1.schema.json`](schemas/daemon-protocol-v1.schema.json) and
[`agent-protocol-v1.schema.json`](schemas/agent-protocol-v1.schema.json).
Method-body semantics remain normative in this document and the Go protocol
types. Duplicate object names are rejected by the shared strict decoder before
an envelope or body is dispatched.
