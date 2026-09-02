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
