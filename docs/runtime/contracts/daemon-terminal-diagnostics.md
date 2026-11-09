# Bounded daemon terminal diagnostics

In daemon mode, `mkruntimed` reports host configuration/pool validation, durable
state, service initialization, recovery/reconciliation, listener and final idle
pool release failures through fixed terminal records. `mknetd` reports egress,
durable state, boot-ID read, service initialization/reconciliation, socket-parent
and listener failures through the same interface. Its internal stage wrapper
retains the cause for classification but does not format it into the record.
The fixed table contains fifteen runtime, eight network, thirteen agent and one
shim discovery code.
Agent startup, re-exec and final serve/shutdown failures use the same typed
terminal reporter; their underlying errors are not passed to it.

The shim's `-info` discovery failure reports `SHIM_RUNTIME_INFO_FAILED`
(`write-runtime-info`) through this same bounded reporter, retaining exit 1.
Successful discovery still writes the original protobuf response to stdout.
Malformed/oversized options and input/output failures share this fixed code;
the underlying error is not supplied to the reporter. This covers the discovery
branch only. Direct startup errors inside the vendored shim runner retain their
separate, unresolved stderr behavior; FIFO setup and manager delegation are
unchanged. No terminal record establishes readiness or closes G7/G8.
The [committed-source comparison and GCE native proof](../../../evidence/runtime-20261109/g8-shim-info-diagnostics/README.md)
qualify malformed/oversized options and input read failures with captured,
closed-reader and full-undrained stderr, plus two successful discovery controls.
Output write faults and real-child execution are not injected by this proof.

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
(`mkruntimed`, `mknetd` or `mk-agent`), `scope: "daemon-terminal"`, fixed `code` and `operation`,
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
installed service or kernel backend is modified. The subsequent [actual child-Docker regression](../../../evidence/runtime-20261031/g8-live-docker-daemon-terminal-diagnostics/README.md)
also passes for this source with engine/image separation and normal cleanup.
Terminal stderr faults were not injected during Docker and retain their separate
fixture proof. Cold restoration remains a separate checkpoint. Earlier
source-qualified proofs are not relabelled. Direct child kernels require
no nested virtualization; the target compute instances cannot use nested
virtualization as a containment solution. Primary and child kernels share host
trust after child-kernel compromise; full G7/G8 remain open.

## Agent terminal extension

Source `34f1caf0` routes the thirteen terminal sites in mk-agent through the same
single-attempt reporter. Existing exit statuses, credential consumption, Docker
cleanup order, transport policy and shutdown evidence are unchanged. Codes are:

| Code | Operation |
| --- | --- |
| `AGENT_DOCKER_BOOTSTRAP_FAILED` | exec-docker-bootstrap |
| `AGENT_CREDENTIAL_EXEC_FAILED` | exec-oci-credential |
| `AGENT_OCI_EXEC_FAILED` | exec-oci |
| `AGENT_ARGUMENTS_INVALID` | validate-agent-arguments |
| `AGENT_PROTECTION_FAILED` | protect-agent |
| `AGENT_CREDENTIAL_REFUSED` | consume-agent-credential |
| `AGENT_SOCKET_REQUIRED` | validate-agent-socket |
| `AGENT_DOCKER_ROOT_REFUSED` | validate-docker-root |
| `AGENT_DOCKER_START_FAILED` | start-docker-engine |
| `AGENT_DOCKER_CLEANUP_FAILED` | stop-docker-engine |
| `AGENT_LISTEN_FAILED` | listen-agent |
| `AGENT_SHUTDOWN_FAILED` | shutdown-guest |
| `AGENT_SERVE_FAILED` | serve-agent |

The [local proof](../../../evidence/runtime-20261105/g8-agent-terminal-controls/README.md)
compares the exact parent against 24 real entry-point cases: eight invocation/
validation/listener refusals with captured, full-undrained and closed stderr.
They preserve intended exits 126/2/1 and one-shot credential consumption, and
exclude invocation paths, constraint data and credential bytes from the record.
Shared reporter controls cover every typed code, one attempt, descriptor bounds
and CLOEXEC, concurrent calls and failed/short/blocked writers. Full runtime race
and required checks pass.

Docker start/cleanup, protection-syscall failure, serving failure and final shutdown
failure are not injected by that entry-point matrix. The reporter itself has a
bounded wait; work performed before it, including existing Docker stop/storage
quiesce diagnostics and cleanup calls, has separate contracts. Parser errors,
workload stdout/stderr, engine logs, console evidence, aggregate node pressure and
actual source-qualified Docker qualification remain outside this checkpoint.
Do not use a terminal record as signal, shutdown or resource-release authority.
No nested virtualization is used; direct child kernels share host trust after compromise.

The separate [GCE root fixture](../../../evidence/runtime-20261105/g8-agent-terminal-gce/README.md)
passes success/zero (`1b229539`) for the same immutable source. It checks the 24
entry-point cases, six reporter tests, production executable refusal and synthetic
Docker stop/retention ordering, while preserving the retained failed ownership.
No actual child-Docker qualification or recovery is claimed by this fixture.

The subsequent [bounded shutdown-marker checkpoint](shutdown-diagnostics.md)
qualifies the six Docker stop/root-quiesce/NBD-disconnect evidence stages for
source `4adc7f6f` under local and GCE synthetic pressure controls. Actual
source-qualified Docker recovery and aggregate node pressure remain separate.
