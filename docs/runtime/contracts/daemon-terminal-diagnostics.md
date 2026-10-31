# Bounded daemon terminal diagnostics

In daemon mode, `mkruntimed` reports host configuration/pool validation, durable
state, service initialization, recovery/reconciliation, listener and final idle
pool release failures through fixed terminal records. `mknetd` reports egress,
durable state, boot-ID read, service initialization/reconciliation, socket-parent
and listener failures through the same interface. Its internal stage wrapper
retains the cause for classification but does not format it into the record.
The fixed table currently contains fifteen runtime and eight network codes.

| Code | First operator inspection |
| --- | --- |
| `RUNTIME_HOST_CONFIG_FAILED` | Selected host configuration, schema, ownership and protected parents. |
| `RUNTIME_POOL_CPUS_INVALID` | Explicit comma-separated pool APIC selection. |
| `RUNTIME_POOL_CPU_FORBIDDEN` | APIC policy; APIC 0 must never enter the pool. |
| `RUNTIME_POOL_MEMORY_INVALID` | Configured pool memory quantity. |
| `RUNTIME_POOL_RESERVE_INVALID` | Reserve quantity and its relation to pool memory. |
| `RUNTIME_STATE_OPEN_FAILED` | Lifecycle state directory identity, permissions and journal validation. |
| `RUNTIME_IDLE_POOL_RECOVERY_FAILED` | Read-only host inventory and retained pool ownership; preserve the refusal. |
| `RUNTIME_HOST_POLICY_REFUSED` | Explicit `mk-host-check` report, requested APICs and primary CPU/memory headroom. |
| `RUNTIME_STORAGE_STATE_OPEN_FAILED` | Storage ledger identity, permissions and validation. |
| `RUNTIME_ROOTFS_STATE_OPEN_FAILED` | Prepared-root ledger identity, permissions and validation. |
| `RUNTIME_ROOTFS_SERVICE_FAILED` | Configured prepared-root storage and service restrictions. |
| `RUNTIME_RECONCILE_FAILED` | Durable lifecycle ownership, journal and actual backend inventory. |
| `RUNTIME_ROOTFS_RECONCILE_FAILED` | Prepared-root records and their retained storage owners. |
| `RUNTIME_LISTEN_FAILED` | Selected Unix socket, protected parent, ownership and listener conflicts. |
| `RUNTIME_IDLE_POOL_RELEASE_FAILED` | Actual idle-pool resources and references; retain uncertain ownership. |
| `NETWORK_EGRESS_CHECK_FAILED` | Explicit egress interface through a read-only link query. |
| `NETWORK_STATE_OPEN_FAILED` | Network ledger identity, permissions and endpoint validation. |
| `NETWORK_BOOT_ID_READ_FAILED` | Availability of the real primary boot ID in procfs. |
| `NETWORK_SERVICE_FAILED` | Subnet, MTU, DNS and primary boot configuration. |
| `NETWORK_RECONCILE_FAILED` | Durable endpoint generations, descriptors and namespace ownership. |
| `NETWORK_SOCKET_PARENT_FAILED` | Selected network socket's protected parent and permissions. |
| `NETWORK_LISTEN_FAILED` | Selected Unix socket ownership and listener conflicts. |
| `NETWORK_TERMINAL_FAILED` | Preserve code, PID/time and source identity for an unclassified terminal failure. |

These are inspection starting points, not repair authority. Do not clear ledgers,
release uncertain resources or repeatedly restart a retained failed generation to
make a code disappear. Explicit inspector output has its own disclosure contract.

Each version-1 JSON record has exactly eight fields: `version`, `component`
(`mkruntimed` or `mknetd`), `scope: "daemon-terminal"`, fixed `code` and `operation`,
positive current `pid`, `owner_known: false`, and canonical UTC `at`. No error,
path, durable-state key, workload ID, command output, environment, request or
arbitrary string can be passed to the producer. PID/time observations neither
attest process incarnation nor authorize signaling, repair or release.

The process attempts one record total, at most 1024 bytes including newline.
A valid attempt consumes the quota even on a missing/broken/blocked writer.
Concurrent or repeated calls cannot add writer goroutines or records. Invalid
codes do not consume quota. There is no queue, retry or accumulating log stream.
The caller waits for delivery for at most a fifty-millisecond timer budget;
scheduling and ordinary fixed-size encoding are not real-time guarantees.
A blocked write can retain one goroutine, one record and one private descriptor
until stderr is released or the process exits. The private descriptor is at
least 3 and CLOEXEC, so a closed stderr reader returns EPIPE instead of Go's
special SIGPIPE termination behavior for writes to descriptors 1/2. The reporter
does not change inherited descriptor flags or process-wide signal handling.
The private descriptor closes when its write finishes or the process exits.

Delivery is best effort: timeout, early exit, closed stderr, write failure or
short write may lose or truncate the sole record. A timed-out write may complete
later if the process remains alive. There is no flush, durability, full history
or readiness guarantee. Silent logs do not establish a healthy service.
Existing exit statuses and cleanup order remain authoritative: runtime listener
shutdown attempts idle-pool release before reporting its terminal result, and
network listener defers server closure before returning to the main reporter.
Host allocation refusal no longer automatically dumps the full host-check report
into the daemon journal; the explicit `mk-host-check` CLI remains separate.

Version output, flag/parser/inspection-mode validation and explicitly requested
inspector/doctor/bundle output retain their existing CLI contracts. In particular,
inspector RPC/backend errors and flag parse errors are not redacted or bounded by
this daemon-mode terminal interface. RPC error bodies, other producers, aggregate
node pressure, rotation/retention and complete readiness/provenance still need
separate qualification. These are terminal control diagnostics, not a filter for
all subprocess or workload bytes.

Local controls exercise the fixed table, quota, concurrent calls, writer failures,
short writes, blocked writes, actual SIGPIPE refusal, descriptor bounds/CLOEXEC
and real daemon CLI failures with captured, full-undrained and closed stderr.
The [local evidence](../../../evidence/runtime-20261031/g8-daemon-terminal-controls/README.md)
and [GCE private fixture](../../../evidence/runtime-20261031/g8-daemon-terminal-gce/README.md)
now qualify these fault controls for source `312bb5e2`. The GCE unit exits
success/zero, primary state and 129 prior diagnostic artifacts match, and no
installed service or kernel backend is modified. New actual child-Docker
qualification remains pending. Earlier source-qualified proofs are not relabelled. Direct child kernels require
no nested virtualization; the target compute instances cannot use nested
virtualization as a containment solution. Primary and child kernels share host
trust after child-kernel compromise; full G7/G8 remain open.
