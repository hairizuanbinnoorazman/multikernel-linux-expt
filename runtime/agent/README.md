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
