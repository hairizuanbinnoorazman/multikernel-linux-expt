# Protocol v1

Daemon messages use length-bounded JSON over a root-owned Unix stream socket.
Agent messages use the same envelope over Multikernel AF_VSOCK. Exactly one
JSON object is permitted per frame; duplicate and unknown fields are rejected.

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
`LoadSandbox`, `StartSandbox`, `StopSandbox`, `DeleteSandbox`, `SandboxState`,
and `WatchEvents`. Agent methods are `Capabilities`, `CreateProcess`,
`ExecProcess`, `StartProcess`, `SignalProcess`, `ResizeProcess`, `WriteProcess`,
`CloseProcessStdin`, `ReadProcessOutput`, `WaitProcess`, `StateProcess`,
`DeleteProcess`, `ConfigureNetwork`, `ExchangeNetwork`, `CloseNetwork`, and
`Shutdown`. Stdin writes and output reads are limited to 64 KiB per request;
network exchange carries at most one 65,535-byte packet in each direction.

Minor additive evolution requires an advertised capability and optional field
defined by the schema. A peer that cannot safely ignore an addition rejects
it as `UNSUPPORTED`. Major-version mismatch is always rejected.

The machine-readable envelope schemas are
[`daemon-protocol-v1.schema.json`](schemas/daemon-protocol-v1.schema.json) and
[`agent-protocol-v1.schema.json`](schemas/agent-protocol-v1.schema.json).
Method-body semantics remain normative in this document and the Go protocol
types. Duplicate object names are rejected by the shared strict decoder before
an envelope or body is dispatched.
