# `terminallog`

One typed terminal diagnostic for a daemon process. The fixed table alone selects
component, code and operation; no error or workload data enters the producer.
A single atomic attempt owns at most one record, goroutine and private CLOEXEC
stderr descriptor. Delivery has a 50 ms timer budget and may fail or be lost.
There is no queue or retry. The private descriptor prevents a closed stderr reader
from turning an intended exit status into SIGPIPE without changing signal policy.

Keep the real closed/full stderr tests. Do not call this terminal interface from
RPC loops or use silence as readiness. Inspectors retain separate output contracts.
See the [contract](../../../docs/runtime/contracts/daemon-terminal-diagnostics.md).
