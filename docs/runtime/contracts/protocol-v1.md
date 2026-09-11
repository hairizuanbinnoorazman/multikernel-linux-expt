# Protocol v1

Daemon messages use length-bounded JSON over a root-owned Unix stream socket.
Agent messages use the same envelope over Multikernel AF_VSOCK. Exactly one
JSON object is permitted per frame; duplicate and unknown fields are rejected.
Framing writes must either deliver the complete frame or fail. Shared write
handling completes injected short-success writes and rejects zero progress so
a faulty wrapped transport cannot silently truncate a request or response.

Every request contains `version: 1`, request ID, method, and a typed body.
Mutations also contain sandbox ID, generation when one exists, and idempotency
key. Every response echoes version/request ID and contains either a typed body
or one structured error. Agent envelopes additionally contain sandbox ID,
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
`ExecProcess`, `StartProcess`, `SignalProcess`, `ResizeProcess`, `WriteProcess`,
`CloseProcessStdin`, `ReadProcessOutput`, `WaitProcess`, `StateProcess`,
`DeleteProcess`, `ConfigureNetwork`, `ExchangeNetwork`, `CloseNetwork`, and
`Shutdown`. Stdin writes and output reads are limited to 64 KiB per request;
network exchange carries at most one 65,535-byte packet in each direction.
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
`ExecProcess` names an existing parent process and inherits its already
validated container root; it cannot supply an arbitrary root path. Process
state and wait replies contain metadata only. Output is retrieved through
independent offset-based `ReadProcessOutput` chunks, so retained output cannot
make a state reply exceed the frame limit. Each stream retains at most 4 MiB,
briefly backpressures a full buffer, and then marks explicit truncation rather
than blocking a child indefinitely when its controller disappears.

An agent transport disconnect leaves managed processes and the last accepted
sequence intact. The same authenticated controller may reconnect and continue
with the next sequence; restarting at sequence one is rejected as replay. A
completed connection unregisters its server-cancellation callback immediately,
so reconnect churn cannot retain one waiter and connection reference per old
session. Cancellation still closes and unblocks a currently active connection.
successful quiescent `Shutdown` reply is the final reply on the session. The
agent then syncs and invokes child poweroff; the pinned Multikernel spawn-kernel
machine operations convert that action into a child-scoped notification and
CPU park, not a platform reset.

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
