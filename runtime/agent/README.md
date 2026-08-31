# Child agent

`mk-agent` will run as the child init process or as the first managed service.
Its first contract is defined by
[`../../docs/runtime/plans/03-agent-and-processes.md`](../../docs/runtime/plans/03-agent-and-processes.md).

Do not add a general-purpose shell or remote login service to the production
agent image. Debug images must be separate, visibly labeled artifacts.
