# Child agent

`mk-agent` will run as the child init process or as the first managed service.
Its first contract is defined by
[`../../docs/runtime/plans/03-agent-and-processes.md`](../../docs/runtime/plans/03-agent-and-processes.md).

Do not add a general-purpose shell or remote login service to the production
agent image. Debug images must be separate, visibly labeled artifacts.

The authenticated process protocol supports bounded stdin writes, explicit
stdin close, incremental stdout/stderr reads, PTY allocation, and terminal
resize. Containerd attachment remains primary-owned FIFO plumbing; the child
agent has no independent login or attach listener.

Agent failures carry typed categories or execution stages from their source to
the RPC boundary. `errors.go` maps these to fixed public codes and messages;
diagnostic wording, paths, command output, and tokens never select or populate
a wire error. Wrapped and joined causes remain available locally. Unknown
errors and stages fall back to `INTERNAL`. Request decoding failures are
`INVALID_ARGUMENT`; missing processes, unsupported operations, authentication,
and state preconditions have explicit categories. A failed execution stage is
`INTERNAL` and exposes only its approved stage name.

After a request begins wire I/O, incomplete or invalid framing, failed reads/
writes, cancellation, or a bad reply version/sequence closes that connection.
A failed deadline setup also closes it. Later calls return the local
`ErrReconnectRequired` before writing or advancing the sequence. Only a successful
explicit `ReconnectContext` makes the client usable again, preserving the
existing authenticated sequence. Failed reconnects retain the unusable state.
The client itself performs no dial or replay in `CallContext`.

A complete validly bound remote error or typed-body validation error consumes its
frame and leaves the transport usable. Pre-call cancellation and local request
marshal rejection leave an untouched stream usable too. The original call error
and existing timeout/caller-deadline minimum are preserved. An interrupted request
may already have executed; recovery must use the existing operation identity and
reconciliation rules. Sequence validation remains strict.
