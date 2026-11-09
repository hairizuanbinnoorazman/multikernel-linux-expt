# Shim library diagnostic boundary

The shim installs its default-logrus boundary before entering either the worker
supervisor or the vendored shim runner. The existing shim log FIFO setup,
subreaper, signal handling, RPC behavior and fatal exit policy stay in place.
Library entries cross the boundary before any formatter or writer sees them.

Only fixed message classes select diagnostic codes: RPC receive/send, inactive
stream, message handling, peer/connection/server refusal, reaper, listener and
signal-loop events. Unknown text becomes `SHIM_LIBRARY_EVENT`. A pinned upstream
stack-dump envelope becomes `SHIM_STACK_DUMP_REDACTED`; no stack content survives.
Records carry version 1, component `containerd-shim-multikernel-v2`, scope
`shim-library`, fixed code/operation/severity, the host process PID, host UTC
time, and `owner_known=false`. They carry no sandbox/container names, paths,
namespace, request, credentials, cause, arbitrary fields or caller/context data.
PID/time remain observations, not process-incarnation or ownership authority.

Whole-entry replacement also clears logrus's hidden field-formatting error. Raw
error/Stringer methods are never evaluated by the boundary or downstream
formatter. The caller's retained parent entry is not modified. The replacement
entry uses a private no-output logger, so later upstream output/formatter
configuration cannot restore a synchronous raw write. Log level selection and
the original caller's Fatal exit function are unchanged; a recovered Panic
still contains an entry, with private state removed.

One atomic attempt per fixed code and severity per process feeds the existing
64-record, 1024-byte diagnostic sink. A blocked pipe holds only its existing
single sink worker/private descriptor; producers do not wait. Full queues,
writer errors, short writes and process exit can lose diagnostics, without
retry or fallback to raw output. Delivery is neither durable nor guaranteed for
fatal exits. The vendored runner still redirects stderr through its normal FIFO;
the sink uses its existing CLOEXEC descriptor duplication and reader-loss rules.

Controls exercise hostile messages/fields/causes and hidden errors; unchanged
parent entries, panic objects and fatal status; formatter/output replacement;
concurrent deduplication and blocked writers; actual Unix TTRPC peer refusal with
continued inspection; and real stderr capture/closure/full-pipe behavior. The
compiled no-child probe additionally exercises debug logs, SIGUSR1 stack dumps,
its actual supervisor/worker FIFO, exact owned pidfd/parentage and continued
Task/Connect plus independent inspection under a full FIFO. Its controlled legacy
mode can reproduce the previous raw-path log, but cannot qualify redaction.

This covers entries through the pinned shim process's default logrus pipeline.
It does not sanitize direct stdout/stderr writes, pre-main initialization output,
the vendored runner's direct startup-error printing, or arbitrary host-installed
logging hooks. It cannot prevent allocation/formatting done before a library
creates an entry, nor establish CPU fairness, durable logging, readiness or
hostile child-kernel containment. Those broader G7/G8 requirements remain open.
Direct child kernels require no nested virtualization and share primary trust
after arbitrary child-kernel compromise.
