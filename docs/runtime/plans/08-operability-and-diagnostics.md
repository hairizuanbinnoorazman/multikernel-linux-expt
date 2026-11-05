# Plan 08: Operability, diagnostics, and maintainability

## Docker scope-stop diagnostic checkpoint (2026-11-05)

Source `1c309569` adds [one bounded scope-stop failure diagnostic](../contracts/docker-scope-stop-diagnostics.md).
The legacy classification marker keeps one process-wide attempt, caller/timer
budget and private CLOEXEC descriptor, without original error text. [Three
exact-parent failures and green real-stderr/1,000-stop controls](../../../evidence/runtime-20261105/g8-docker-scope-log-controls/README.md)
preserve refusal and ownership without publishing completion. Classification,
context/quota/CLOEXEC, stop queue regression, full runtime race and required checks
pass. Actual source-qualified Docker release, original timeout causality, retained
recovery and full G7/G8 remain open. Capture authorization is still pending.
No nested virtualization is used; direct child kernels share host trust after compromise.

## Docker stop admission checkpoint (2026-11-05)

Source `a2a99088` makes [Docker service stop admission context-aware](../contracts/docker-stop-queue-budget.md).
A canceled/deadline waiter can leave while the active stop retains its clone-bound
pidfd and cleanup authority. [Two exact-parent failures and green controlled-process
cases](../../../evidence/runtime-20261105/g8-docker-stop-queue-controls/README.md) preserve owner result,
live authority, subsequent admission and completed idempotence. Concurrent gate,
full runtime race and required checks pass. [GCE fresh-process controls](../../../evidence/runtime-20261105/g8-docker-stop-queue-gce/README.md)
also pass success/zero (`c7cb4e24`), preserving retained ownership without guest
RPCs or retained-child signals. No actual engine/cgroup/mount release is performed
by these controls; original timeout causality remains unproven.
Actual Docker qualification, retained recovery and full G7/G8 remain open. The
capture authorization is still pending. No nested virtualization is used; direct
child kernels share host trust after compromise.

## Docker shutdown diagnostic checkpoint (2026-11-05)

Source `4adc7f6f` adds [bounded delivery for six legacy shutdown markers](../contracts/shutdown-diagnostics.md).
Each stage has one attempt; each stream retains at most one active writer and
private CLOEXEC descriptor, with no queue or busy-stream retries. [Seven exact-parent
marker passes, fourteen pressure failures and 21 green descriptor cases](../../../evidence/runtime-20261105/g8-shutdown-diagnostics-controls/README.md)
retain error/operation ordering and failure boundaries. Quota/CLOEXEC, full runtime
race and required checks pass. [GCE root controls](../../../evidence/runtime-20261105/g8-shutdown-diagnostics-gce/README.md)
also pass success/zero (`15d6034c`), preserving retained ownership without guest
RPCs or actual shutdown syscalls. Missing markers remain missing evidence. Actual
Docker qualification, live recovery and full G7/G8 remain open; the pending capture
authorization and retained failed generation are unchanged. No nested virtualization
is used; direct child kernels share host trust after compromise.

## Agent terminal diagnostic checkpoint (2026-11-05)

Source `34f1caf0` routes thirteen agent terminal failure sites through fixed,
bounded diagnostics. [Twenty-four exact-parent failures and green real-entry
stderr controls](../../../evidence/runtime-20261105/g8-agent-terminal-controls/README.md)
preserve exit statuses and one-shot credential consumption with captured,
full-undrained and closed stderr. Reporter quota/descriptor controls, full runtime
race and required checks pass. [GCE root controls](../../../evidence/runtime-20261105/g8-agent-terminal-gce/README.md)
also pass success/zero (`1b229539`), preserving failed ledgers/resources and native
identity without actual guest RPCs or retained credentials. Actual Docker
qualification and other stop/quiesce producers remain separate. The retained failed generation and pending cloud-capture
authorization are unchanged; normal cleanup and full G7/G8 remain open. No nested
virtualization is used; direct child kernels share host trust after compromise.

## Guarded pending-signal replay controls (2026-11-05)

Source `1d658f9c` adds an [explicit operator replay tool](../contracts/pending-signal-replay.md)
for the exact persisted, unacknowledged signal. Sealed inputs bind boot, ownership,
process/PID, protected record identity and the original operation ID. An exclusive
attempt marker precedes the single request; duplicate attempts refuse before key
read/dial and uncertain replies stay unknown. [Authenticated Unix controls, full
runtime race and required checks](../../../evidence/runtime-20261105/g8-signal-replay-controls/README.md)
pass. The [immutable GCE control fixture](../../../evidence/runtime-20261105/g8-signal-replay-gce/README.md)
also passes success/zero (`d0b1c2df`), preserving failed ownership and native
identity without credential reads or guest RPCs. No actual guest signal or
recovery is claimed by these controls. The [VM-side private cloud-capture attempt](../../../evidence/runtime-20261105/g8-signal-replay-cloud-capture-failure/README.md)
fails on missing SDK authentication (`3cd21352`); automatic approval review rejects
raw serial/project-inventory transfer using local credentials pending explicit
authorization. No live recovery is attempted. Normal Docker cleanup and full G7/G8 remain open. No nested virtualization is used; direct child
kernels share host trust after compromise.

## Purpose

Turn the proven runtime into a system that an operator can inspect, diagnose,
and recover without reading implementation code or relying on test-only shell
harnesses.

The [deployment constraint](../architecture.md#deployment-and-containment-constraint)
excludes nested virtualization inside the cloud instance. G8 must operationalize
the direct-child-kernel runtime within that constraint. For Docker-in-Docker,
inspect generation-bound child ownership and preserve the guest-local engine,
socket and build-storage boundary; never substitute the host Docker socket.
Operational checks do not establish containment after child-kernel compromise.

## Agent caller queue-budget checkpoint (2026-11-05)

Source `9b5bf7dd` makes [agent stream admission context-aware](../contracts/agent-client-queue-budget.md)
for calls and both reconnect APIs. Queued cancellation/deadline returns without
waiting for an unrelated frame, sending/dialing, consuming a sequence or changing
the active stream/peer policy. [Six exact-parent failures, green Unix-wire/owner
preservation, concurrent framing, race and required checks](../../../evidence/runtime-20261105/g8-agent-client-gate-controls/README.md)
pass. The separate [GCE Unix-control fixture](../../../evidence/runtime-20261105/g8-agent-client-gate-gce/README.md)
passes success/zero (`bfae34de`) without credential reads or guest RPCs, preserving
failed ledgers/resources and native identity. Client.Timeout remains wire-phase
only; callers need a bounded context for queue admission. The defect's causal role
in the original GCE timeout is unproven;
retained recovery, normal Docker cleanup and full G7/G8 remain open. No nested
virtualization is used; direct child kernels share host trust after compromise.

## Retained process-output diagnostic checkpoint (2026-11-05)

Source `255c3908` adds an explicit [running/stopped output-summary diagnostic](../contracts/process-output-diagnostic.md)
for an exact retained guest task. Four read-only calls share one deadline; zero
output offsets, final state bracketing, fixed whole-line markers and payload/hash
exclusion remain enforced. Private capture and conflicting modes refuse before
authentication. [Unix-wire/privacy/legacy/race/required controls](../../../evidence/runtime-20261105/g8-process-output-controls/README.md)
pass; the first archive-builder VCS refusal is retained, with a separate hash-bound
v2 build. The [one-shot retained-agent observation](../../../evidence/runtime-20261105/g8-retained-process-output-gce/README.md)
passes success/zero (`d60d43fd`): `init` reports RUNNING PID 613 and its bounded tail
contains full-pressure/fine-sync markers missing from host capture. Host/persisted
stdout/stderr offsets are 11711/191; an unacknowledged signal-9 intent remains.
Failed ownership is unchanged. This narrows delivery/timing/pending-signal diagnosis,
without proving chronology or signal delivery and without passing the failed gate.
Generation-bound recovery, normal Docker cleanup and full G7/G8 remain open. No nested virtualization is used;
direct child kernels share host trust after compromise.

## Shim diagnostic reader-loss checkpoint (2026-11-05)

Source `4f6a4810` protects [shared shim diagnostic stderr](../contracts/diagnostic-stderr.md)
with a private CLOEXEC duplicate. A closed reader now disables the lossy sink
without SIGPIPE termination; full pipes retain one bounded writer/descriptor and
64 queued records. [Exact-parent red/green, 16 real-pipe default-sink cases,
lifecycle recovery, race and required controls](../../../evidence/runtime-20261105/g8-diagnostic-stderr-controls/README.md)
pass. Process exact exit 37, supervisor crash restart/PID cleanup and durable event
reconstruction/ordered acknowledgement remain intact. The [GCE fixture](../../../evidence/runtime-20261105/g8-diagnostic-stderr-gce/README.md)
passes success/zero (`9303f71c`) on boot `9c7a1110`, preserving all 219 artifacts and
19 current proof files without installed-service changes or child allocation.
The [new-source actual Docker attempt](../../../evidence/runtime-20261105/g8-live-docker-diagnostic-stderr-failure/README.md)
fails: workload `4051ff4c` and parent `fbc85042` exit 1 after the ENOSPC-phase
deadline, before observed coarse-sync completion. One child/ACTIVE export, two
module references and three VSOCK rows remain. No normal cleanup/restoration pass
is claimed and no retry/reboot/hot unload is performed. Custody audit `b7530dc8`
passes, preserving all 219 prior artifacts, 79 prior proof files and failures;
installed files/PIDs remain unchanged. Diagnosis, generation-bound recovery,
actual Docker qualification, node aggregate pressure/retention, other producers
and full G7/G8 remain open.
No nested virtualization is used; direct child kernels share host trust after compromise.

## Task RPC privacy checkpoint (2026-11-05)

Source `18c97b4f` adds [fixed Task RPC failure replies](../contracts/task-rpc-errors.md)
for all 17 methods through the production shim factory. Failure text/details and
partial bodies are omitted; pinned wire status codes, successful calls, internal
recovery and Manager forwarding retain their contract. [Local red/green, Unix-wire
oracle, constructor, race and required checks](../../../evidence/runtime-20261105/g8-task-rpc-controls/README.md)
pass. The initial unsafe fixture permission refusal is retained; production bundle
permissions were not weakened. The [GCE fixture](../../../evidence/runtime-20261105/g8-task-rpc-gce/README.md)
passes success/zero (`5bd529bd`) on boot `3db378f4`, preserving all 201 artifacts,
19 current proof files and prior failure generations. It uses controlled delegates
without installed-service changes or kernel allocation. The separate [actual Docker
regression](../../../evidence/runtime-20261105/g8-live-docker-task-rpc/README.md)
passes: workload `550b1776` and parent `9c6827f8` exit success/zero; build/recovery,
engine/image separation, authority, diagnostics and normal cleanup pass. Eight-report
holds take 115.601 / 108.403 / 97.689 ms. Private faults were not injected during
Docker. [Cold restoration](../../../evidence/runtime-20261105/g8-live-docker-task-rpc-cold-restoration/README.md)
passes success/zero (`4538903f`) on boot `9c7a1110`: original transport, 93 retained
files / 12 images, all 219 diagnostic artifacts and prior fault/failure generations
are preserved. Other producers,
aggregate diagnostics, provenance/readiness and full G7/G8 remain separate.
No nested virtualization is used; direct child kernels share host trust after compromise.

## Rootfs RPC error checkpoint (2026-11-04)

Runtime source `e467a53b` adds [fixed rootfs RPC failure replies](../contracts/rootfs-rpc-errors.md).
Prepare/Validate/CleanupRootfs no longer reflect private backend/filesystem errors;
failed preparation omits its result body. Codes/retry flags and internal ownership,
rollback/reconciliation behavior remain intact. [Local wire/state/race/required controls](../../../evidence/runtime-20261104/g8-rootfs-rpc-controls/README.md)
pass. The first GCE qualification loader fails before testing; [its original failure](../../../evidence/runtime-20261104/g8-rootfs-rpc-fixture-loader-failure/README.md)
is preserved, with no test log or success receipt. Helper source `8604f450` selects
retained definitions by AST assignment boundary, avoiding literal markers inside
strings without replaying phase branches. A separate [v2 GCE fixture](../../../evidence/runtime-20261104/g8-rootfs-rpc-gce/README.md)
passes success/zero (`def59f35`) on boot `fd5f3a6d`: concrete rootfs service/store
recovery, stale-identity refusal and classification pass with controlled backends.
All 183 prior artifacts and six failed-v1 files match; installed services and
kernel backends are unchanged in that fixture. The separate [actual Docker run](../../../evidence/runtime-20261104/g8-live-docker-rootfs-rpc/README.md)
now passes for the same source: workload `b139efcd` and parent `95bcaaef` exit
success/zero. Build/ENOSPC recovery, engine/image separation, authority, normal
cleanup and timed holds (110.172, 110.735, 98.993 ms) pass. Private backend faults
were not injected during Docker. The [cold restoration](../../../evidence/runtime-20261105/g8-live-docker-rootfs-rpc-cold-restoration/README.md)
passes on November 5, boot `3db378f4`, post invocation `d7d2dd94`: original
transport, 93 retained files / 12 images, all 201 diagnostic artifacts and both
rootfs fixture generations are preserved.
Task errors, other producers, aggregate pressure/retention, provenance/readiness
and full G7/G8 remain open. No nested virtualization is used; direct child kernels
share host trust after child-kernel compromise.

## RPC client errors and ATTACH cancellation (2026-11-04)

Source `7e973e47` adds [fixed local RPC client errors](../contracts/rpc-client-errors.md).
Daemon/network connection, deadline, write and read failures no longer expose
local socket paths or transport payloads. The result does not retain an unwrap
chain to the private cause. Standard context sentinels, daemon codes/retry flags
and authenticated server replies retain their semantics. Network clients refuse
pre-cancelled dialing and failed deadline setup; ATTACH now promptly cancels a
blocked real Unix descriptor receive and joins its socket-close callback.
[Local red/green, focused/full runtime race and required controls](../../../evidence/runtime-20261104/g8-rpc-client-controls/README.md)
and the [GCE private fixture](../../../evidence/runtime-20261104/g8-rpc-client-gce/README.md) pass, including
successful descriptor transfer and descriptor closure after truncation. GCE
invocation `3009023f` exits success/zero on restored boot `50deb61b`; primary state
and all 165 diagnostic artifacts/prior fault and failure proofs remain unchanged.
Installed services/kernel backends are not changed in that fixture. The separate
[actual Docker regression](../../../evidence/runtime-20261104/g8-live-docker-rpc-client/README.md)
now passes for the same source: workload `31f3baaf` and parent `9d95bea8` exit
success/zero. Builds/ENOSPC recovery, engine/image boundary, authority and normal
cleanup pass; eight-report holds take 112.137, 112.328 and 99.627 ms. Private
faults were not injected during Docker. Its [cold restoration](../../../evidence/runtime-20261104/g8-live-docker-rpc-client-cold-restoration/README.md)
now passes on boot `fd5f3a6d`, post invocation `5c72a892`, with original transport,
93 retained files / 12 images and all 183 diagnostic artifacts/faults preserved.
Server-side rootfs/Task errors, other producers,
aggregate pressure/retention, provenance/readiness and full G7/G8 remain open.
No nested virtualization is used; direct child kernels share host trust after
child-kernel compromise.

## Network mutation replies and restart audit (2026-11-04)

Source `9362d5a4` adds [fixed network mutation errors](../contracts/network-mutation-errors.md):
backend, namespace, device and persistence failures no longer reflect private
error text into replies. Existing codes, retry flags and generation-qualified
cleanup remain intact; incomplete rollback explicitly requires reconciliation.
[Red/green controls, network race tests, full runtime race tests and required checks](../../../evidence/runtime-20261104/g8-network-mutation-controls/README.md)
pass. The [GCE private fault fixture](../../../evidence/runtime-20261104/g8-network-mutation-gce/README.md)
also passes with unchanged primary services and transport. [Actual child-Docker qualification](../../../evidence/runtime-20261104/g8-live-docker-network-mutation/README.md)
now passes for the same source: workload `e490f774` and parent `a71e19e8` exit
success/zero. ENOSPC/recovery, builds/run/save/exit-37, engine/image separation,
normal cleanup and three eight-report holds (113.055, 113.012, 95.793 ms) pass.
Private faults retain their separate fixture scope. A taint-baseline preflight
refusal is preserved; code `bad7da32` adds the exact origin-taint validator and its
required controls. The separate [cold restoration](../../../evidence/runtime-20261104/g8-live-docker-network-mutation-cold-restoration/README.md)
passes on boot `50deb61b`, post invocation `69ac319e`, with original transport,
93 retained files / 12 images and all 165 diagnostic artifacts preserved. Prior
faults and the new failed preflight remain intact. Other RPC producers, aggregate
pressure/retention and full G7/G8 remain open; earlier results retain their scope.

The [read-only GCE restart audit](../../../evidence/runtime-20261104/g8-user-restart-preservation/README.md)
passes on boot `3f4121cf`: original transport, 93 retained files / 12 images,
147 diagnostic artifacts and prior fault/failure evidence are preserved.
The saved source-`312bb5e2` restoration receipt and persistent journal establish
historical completion on boot `89c37f4a`; after the user's further restart its
transient unit is absent, so no historical numeric exit status or immediate
post-restoration cloud-ledger comparison is claimed. Exported proof is an
allowlisted summary; richer receipts remain private on the VM. Cross-boot network
identity parity and full G7/G8 remain open. No nested virtualization is used;
direct child kernels share host trust after child-kernel compromise.

## Daemon terminal diagnostic checkpoint (2026-10-31)

Source `312bb5e2` adds [bounded daemon terminal diagnostics](../contracts/daemon-terminal-diagnostics.md).
Daemon-mode configuration/state/reconciliation/listener/pool failures emit one
fixed-code record, with no raw backend error, path, durable key or command output.
A fifty-millisecond delivery budget bounds caller waiting; one private CLOEXEC
stderr descriptor avoids SIGPIPE without changing signal policy or inherited
flags. The sole record can be lost; silence does not establish readiness.
[Local non-race/race and required controls](../../../evidence/runtime-20261031/g8-daemon-terminal-controls/README.md)
and [GCE real-stderr proof](../../../evidence/runtime-20261031/g8-daemon-terminal-gce/README.md)
pass captured, full-undrained and closed stderr with original daemon exit statuses,
private-state redaction and descriptor bounds. Development SIGPIPE failure and
sandbox netlink refusal remain separately retained. GCE primary state and all
129 prior diagnostic artifact identities/content hashes match; installed services
and kernel backends were not changed during that private fixture. A subsequent
[actual child-Docker regression](../../../evidence/runtime-20261031/g8-live-docker-daemon-terminal-diagnostics/README.md)
now passes for this source: workload 6c818661 and parent a80c7a95 exit success/zero.
ENOSPC/recovery, multistage/repeat/cache builds, run/save, intentional exit-37
rejection, exact guest/primary engine-image separation and normal cleanup pass.
The three eight-report holds take 119.906, 118.827 and 105.115 ms within their
shared ten-second budgets; network health and journal 6/6/10 joins pass. Private
terminal stderr faults were not injected during Docker and retain their separate
source-qualified fixture proof. Cold restoration is recorded in the qualified restart audit above; no hot unload
follows traffic. Earlier source-qualified Docker
results retain their scope. Inspector/RPC/flag output, other producers, retention/aggregate pressure,
readiness/provenance and full G7/G8 remain open. No nested virtualization is used;
direct child kernels share host trust after child-kernel compromise.

## Fresh baseline after user restart (2026-10-31)

Source `eb405c97` adds an explicit persistent Docker comparison across distinct
boot epochs. It checks engine/images/containers/volumes; the workload comparison
still requires the same boot and exact network inventory. Malformed metadata and
persistent changes fail closed without echoing private names. [Controls](../../../evidence/runtime-20261031/g8-docker-restart-comparison-controls/README.md)
and a [GCE restart preflight](../../../evidence/runtime-20261031/g8-supervisor-docker-restart-preflight/README.md)
pass, including rehashing all 93 retained files / 12 images, fresh service PIDs,
111 prior diagnostic artifacts and failed-fixture preservation. Cross-boot network
identity parity is outside this comparison. The frozen child-Docker runtime
packet remains separately pinned to `6e303b6c`; this host-side helper does not
relabel its source. No nested virtualization is used; direct kernels share host
trust after compromise. Full G7/G8 remain open.

## Supervisor diagnostic checkpoint (2026-10-31)

Source `0318bc5b` replaces supervisor control errors with seventeen fixed JSON
codes, sharing the bounded nonblocking process transport. Each code gets one
attempt per supervision invocation across retries and handoffs. No error text,
paths, argv, environment or guest payload is accepted. The supervisor has no
validated workload owner (`owner_known: false`); numeric PID/time fields neither
attest incarnation nor authorize cleanup. Existing worker stdio forwarding remains
unchanged; best-effort diagnostics can be lost on exit or pressure.

The first GCE non-race fixture exposed an idle test helper's empty-select deadlock.
It remains failed with its original invocation/log/binary/driver and no qualifying
receipt. Source `6e303b6c` fixes the helper using a pending sleep timer. Local
non-race/race controls, full runtime race and required checks pass. A separate new
GCE unit, invocation `c3141d4f01b145598b685f474563f8e3`, exits success/zero.
Actual SIGKILL restart/PID cleanup succeeds with captured stderr and a full
undrained real stderr pipe; private proxy-error reap and ten-crash exhaustion
cleanup also pass. Fixture children are gone before the driver takes its final
snapshot; terminal MainPID is zero.

The user restart is observed as boot `223a37e4`, distinct from prior cold
restoration `1fca807b`. Four fresh service PIDs, seven file hashes, same-boot
Docker/network snapshots and 111 prior diagnostic artifact hashes match before
and after. File hashes and persistent Docker engine/images/containers/volumes also
match the prior restoration; cross-boot network identity parity and retained-data
content rehashing are outside this fixture. Original transport remains at zero
references, all CPUs online, with no child/pool/VSOCK. Installed services and
kernel backends were not modified.

This closes the supervisor diagnostic fault checkpoint, with a separate source
and scope from prior actual Docker proofs. Remaining daemon/Task RPC errors,
aggregate pressure, rotation/retention,
readiness/provenance and full G7/G8 remain open. No nested virtualization is used;
direct child kernels share host trust after child-kernel compromise.

The [actual child-Docker regression](../../../evidence/runtime-20261031/g8-live-docker-supervisor-diagnostics/README.md)
now passes for 6e303b6c: workload b857691a and parent 74b5e6ea exit success/zero.
ENOSPC/recovery, multistage/repeat/cache builds, run/save, intentional exit 37,
exact engine/image boundary and normal cleanup pass. The three eight-report
sequences take 121.370, 118.827 and 105.190 ms within shared ten-second budgets;
network health and journal sequence 6/6/10 joins pass. No child, pool, VSOCK or
owned process remains, all CPUs return, native close is synced, I/O errors zero,
offline ext4 check clean and candidate transport references zero. Supervisor
private error/restart/full stderr faults were not injected during Docker and
retain separate source-qualified fixture proof. The [initial restoration preflight refusal](../../../evidence/runtime-20261031/g8-supervisor-cold-preflight-failure/README.md)
is preserved. Its repair uses a separate procedure, directory and unit; the
[cold restoration](../../../evidence/runtime-20261031/g8-live-docker-supervisor-diagnostics-cold-restoration/README.md)
now passes on boot 971c0b9b, post invocation 62e84fc0. Original transport has zero
references, all CPUs are online, four primary services active and no child/pool/
VSOCK remains. All 93 retained files / 12 image contents, eighteen current and
111 prior diagnostic artifacts, four private fault fixtures and both failed
cases survive. Persistent Docker inventories match through the frozen restart
comparator; cross-boot network-ID parity remains unqualified. The successful VM
baseline is `g8-host-6e303b6/cold-restoration-v2/after.json`; the earlier failed
`cold-restoration` directory remains without a success receipt. No hot unload
follows traffic.

See the [supervisor contract](../contracts/supervisor-diagnostics.md),
[local controls](../../../evidence/runtime-20261031/g8-supervisor-helper-controls/README.md),
[retained failure](../../../evidence/runtime-20261031/g8-supervisor-nonrace-helper-failure/README.md)
and [GCE proof](../../../evidence/runtime-20261031/g8-supervisor-diagnostics-gce/README.md).

## Deferred Task-event diagnostic extension (2026-10-30)

Source 3dbf1cf5 adds two fixed codes for deferred publication and startup replay.
The [contract](../contracts/process-diagnostics.md), [local race controls](../../../evidence/runtime-20261030/g8-event-diagnostics-controls/README.md)
and [GCE private fixture](../../../evidence/runtime-20261030/g8-event-diagnostics-gce/README.md)
prove actual durable reconstruction, exact payload order and unchanged durable
queue bytes on failed startup replay under a pre-blocked injected writer.
Healthy replay acknowledges all eight events and removes the journal. Atomics
avoid taking lifecycle locks for emission; immutable ownership is installed by
create/recovery and cleared on successful rollback. A code gets one enqueue
attempt per service/worker, even if the sink rejects it. Unknown ownership is
suppressed without consuming that attempt. Owner changes do not reset quota;
context does not attest the owner/sequence of a particular pending event.

The GCE unit exits success/zero with no fixture helpers left; primary state and
all 93 earlier diagnostic artifacts match. This closes this private-writer event
fault checkpoint. A subsequent [actual child-Docker regression](../../../evidence/runtime-20261030/g8-live-docker-event-diagnostics/README.md)
now passes for this source: ENOSPC/recovery, builds, exact engine/image boundary,
receipt-bound reports and normal cleanup. Private broker faults were not injected
during Docker; durable replay fault behavior retains its separate fixture scope.
Its [cold restoration](../../../evidence/runtime-20261030/g8-live-docker-event-diagnostics-cold-restoration/README.md)
also passes: original transport, all 93 retained files / 12 image contents,
eighteen current and 93 prior diagnostic artifacts, failed cases and all three
private fault fixtures survive the graceful idle reboot. Coverage of a real stderr
pipe for the new event producer remains unqualified. Supervisor/daemon logs,
Task RPC errors,
rotation/retention, aggregate pressure, readiness/provenance and full G8 remain
open. No nested virtualization is used; direct child kernels share host trust
after compromise.

## Network and teardown diagnostic extension (2026-10-30)

Source 038ce648 adds four fixed codes to the same bounded sink, eliminating raw
network device/report errors and successful teardown warnings. The [contract](../contracts/process-diagnostics.md),
[local race controls](../../../evidence/runtime-20261030/g8-network-teardown-diagnostics-controls/README.md)
and [GCE private fixture](../../../evidence/runtime-20261030/g8-network-teardown-diagnostics-gce/README.md)
prove repeated private REPORT errors, fatal device faults, joins with the service
mutex held under a pre-blocked injected writer and exact exit-37 init deletion
with host ownership release. The GCE unit exits success/zero; primary state and
all 75 prior diagnostic artifacts match before/after. Network diagnostic context
is snapshotted from endpoint ownership before monitor start, with process ID init.

This closes the private-writer network/teardown fault checkpoint. New producer
real-stderr-pipe coverage remains unqualified. A subsequent
[actual child-Docker regression](../../../evidence/runtime-20261030/g8-live-docker-network-teardown-diagnostics/README.md)
now passes for source 038ce648: ENOSPC/recovery, builds, exact engine/image
boundary, receipt-bound reports and normal cleanup. Private network/teardown
faults were not injected during Docker. Its [cold restoration](../../../evidence/runtime-20261030/g8-live-docker-network-teardown-diagnostics-cold-restoration/README.md)
also passes: original transport, 93 retained files / 12 image contents, eighteen
current and 75 prior artifacts, failed cases and both private fault fixtures
survive the graceful idle reboot. Earlier d88680b5 results retain their source
and scope. Task RPC
errors and event/supervisor/daemon log producers, rotation/retention, aggregate
pressure and full G8 remain open. No nested virtualization is used; direct child
kernels share host trust after compromise.

## Process-stream diagnostic checkpoint (2026-10-30)

The [contract](../contracts/process-diagnostics.md) and
[local race controls](../../../evidence/runtime-20261030/g8-process-diagnostics-controls/README.md)
now have [GCE private agent-fault proof](../../../evidence/runtime-20261030/g8-process-diagnostics-gce/README.md)
for source d88680b5. Raw backend errors are excluded from process-monitor events;
validated fixed-code metadata is attempted once per code per monitor invocation.
The worker has 64 queued records plus one in flight, each at most 1024 bytes.
Full queues reject immediately; writer error/short write disables the sink.
Real stderr redaction and blocked-pipe recovery preserve exact exit 37 and its
exit event. Host state and 57 prior diagnostic artifacts match before/after.

This closes the bounded process-monitor diagnostic fault checkpoint only.
Queued diagnostics can be lost, a single writer goroutine can stay blocked until
worker exit, and silence provides no health guarantee. The subsequent
[actual child-Docker regression](../../../evidence/runtime-20261030/g8-live-docker-process-diagnostics/README.md)
now passes for this source: ENOSPC/recovery, builds, engine/image boundary,
receipt-bound reports and normal cleanup. It does not inject the private monitor
faults during Docker. Its [cold restoration](../../../evidence/runtime-20261030/g8-live-docker-process-diagnostics-cold-restoration/README.md)
also passes: original transport, 93 retained files / 12 image contents, eighteen
current and 57 prior diagnostic artifacts and the private fault fixture survive
the graceful idle reboot. Teardown warnings, network retries, event publication
and supervisor diagnostics remain outside this process-monitor checkpoint.
Other shim/supervisor/network log producers, rotation,
retention, aggregate node pressure and full G8 remain unqualified. No nested
virtualization is used; direct child kernels share host trust after compromise.

## Operational interface

- Add a read-only doctor/inspect command that reports component revisions,
  service health, configured and available pool capacity, active ownership,
  storage exports, network endpoints, and orphan candidates.
- Provide machine-readable health and readiness output for `mkruntimed` and
  `mknetd` without weakening Unix-socket ownership or authentication.
- Use structured logs carrying sandbox ID, generation, component, operation,
  and stable failure code. Never log agent tokens or complete environments.
- Add a bounded diagnostic-bundle collector with explicit redaction and file
  size limits.
- Define log retention, rotation, pressure, and slow-reader behavior.
- The disposable [console-window contract](../contracts/diagnostic-console.md)
  reserves a bounded teardown window within the existing total cap and reports
  its unobserved interval explicitly; it is not the complete diagnostic bundle.
- Keep reconciliation generation-bound and inspection-first; do not provide a
  broad destructive repair command.

## Service hardening

- Document the exact privileges required by each daemon and helper.
- Apply the strongest compatible systemd filesystem, process, and capability
  restrictions, with tests that prove required Kerf, network, storage, and
  restart behavior still works.
- Report unhealthy or unreconciled durable state distinctly from temporary
  startup delay.
- Ensure every documented recovery action is idempotent or explicitly refuses
  unsafe repetition.

## Maintainability

- Split the Runtime v2 shim implementation into focused task, process-I/O,
  recovery, supervisor, network, rootfs, and event files while initially
  retaining one Go package and identical protocol behavior.
- Preserve the race, fault, live-restart, and evidence suites during the split.
- Document package ownership and the invariants shared across components.

## Tests

- Doctor output against clean, active, degraded, stale, and identity-mismatch
  fixtures.
- Structured-log schema, token-redaction, truncation, and pressure tests.
- Diagnostic collection with unavailable services and oversized inputs.
- Service hardening on a qualified host, including restart reconciliation.
- Operator runbook execution by a fresh checkout with no developer-local state.

Bounded [storage-pressure recovery/cleanup](../../../evidence/runtime-20261024/g8-pressure-recovery-qualified/README.md)
and a [normal same-boot repeat](../../../evidence/runtime-20261024/g8-pressure-warm-repeat/README.md)
have qualified live evidence, including service-policy observations, generation
release, filesystem checking and same-boot primary/resource parity. These cover
part of the failure/recovery and hardening tests above. The complete health,
diagnostics, recovery, maintainability and fresh-checkout operator requirements
still govern the full exit gate below.

## Gate G8

The [structured diagnostic collector](../contracts/diagnostic-bundle.md) now
retains bounded component and version-7 reports in a protected exclusive JSON
artifact, with a byte-count/SHA-256 receipt. [Local controls](../../../evidence/runtime-20261029/g8-diagnostic-bundle-controls/README.md)
pass cancellation, source-error redaction, size limits, unsafe destinations,
concurrent publication and the complete runtime race suite. [Actual GCE collection](../../../evidence/runtime-20261029/g8-diagnostic-bundle-gce/README.md)
also passes idle legacy/current-daemon collection, exact private modes, refusal
and primary preservation. [Actual Docker bundle collection](../../../evidence/runtime-20261029/g8-live-diagnostic-bundles/README.md)
also passes ENOSPC/build/removal holds, original receipt/identity checks and normal
cleanup. Source 5412c4be retains its inherited wider built-image budget; the
subsequent [corrected timed execution](../../../evidence/runtime-20261029/g8-live-timed-diagnostic-bundles/README.md)
passes all three eight-report sequences below ten seconds and the receipt-bound
integrated parent gate. The initial root-receipt path failure and its restoration
remain separately retained. This closes local, idle cloud, active collection and
bounded sequence/parent-gate checkpoints.
Its [cold restoration](../../../evidence/runtime-20261029/g8-live-diagnostic-bundles-cold-restoration/README.md)
also passes with original transport and exact live artifact/retained-state preservation.
Full health/readiness, broader safe diagnostic sources, log retention/pressure,
recovery and the remaining requirements below still govern complete G8.

The initial [read-only doctor](../../../runtime/cmd/mkruntimed/README.md) compares
negotiated durable ownership with bounded kernel instance names, reports stable
findings and rejects observed boot/ownership/inventory drift. Its
[GCE controls](../../../evidence/runtime-20261025/g8-runtime-doctor/README.md)
retain an inconclusive installed-daemon observation and synthetic RPC cases.
The subsequent [compatible-daemon check](../../../evidence/runtime-20261025/g8-compatible-daemon-doctor/README.md)
qualifies idle presence inspection against the real GCE kernel inventory with
private production state, explicit compatibility/boot diagnostics and unchanged
installed services. [Live child presence inspection](../../../evidence/runtime-20261025/g8-live-child-doctor/README.md)
subsequently passes during ENOSPC, the built-image hold and after removal.
Its early console saturation leaves late-console coverage open; independent
synced native close, filesystem and resource observations qualify normal cleanup.
Independent native storage/network ownership, live incarnation, pool capacity, full service
readiness and diagnostic bundles remain unobserved by this command and required
for the complete gate.

The separate [durable storage view](../contracts/storage-ownership.md) now provides
bounded generation/CID/quota/inode/release metadata through negotiated protected
RPC. [Tests and GCE qualification](../../../evidence/runtime-20261025/g8-storage-ownership/README.md)
cover read-only projection, empty state and a copied real completed record.
The [combined resource contract](../contracts/resource-ownership.md) and
[live correlation proof](../../../evidence/runtime-20261025/g8-live-resource-correlation/README.md)
subsequently qualify declared storage expectations, generations, CID and full
descriptor fingerprints during actual child Docker pressure. The doctor detects
missing/orphan/mismatched recorded claims and ignores released history as live
authority. Independent native health, current-file/socket attestation and
retention/pagination remain required.

A separate [network ownership CLI](../contracts/network-ownership.md) now
provides bounded recorded endpoint generations, boot claims, states, counters and
opaque descriptor fingerprints through mknetd's existing protected socket.
[GCE controls](../../../evidence/runtime-20261027/g8-network-ownership/README.md)
qualify actual idle CLI/RPC inspection, installed legacy refusal, cancellation
and an unchanged retained failed ledger without reconciliation. This does not
change the doctor's network-unobserved scope or prove live endpoint health,
policy attestation, combined ownership correlation or Docker requalification.

The separate version-4 [network doctor](../contracts/network-ownership.md#explicit-lifecycle-intent-and-correlated-doctor)
now correlates declared intent with repeated endpoint/lifecycle/kernel/boot
samples. [GCE proof](../../../evidence/runtime-20261027/g8-network-doctor/README.md)
qualifies actual idle CLI operation, compatibility and injected populated claims.
It leaves native network health, expected descriptor/incarnation, active child
endpoints, storage combination and Docker requalification open. The ordinary
version-3 doctor's scope remains unchanged.

The later [actual child network-doctor proof](../../../evidence/runtime-20261027/g8-live-network-doctor/README.md)
qualifies version-4 endpoint intent/generation/descriptor/counter correlation
against the separately checked version-3 storage/native-process report during
real child Docker ENOSPC, built-image hold and normal removal. Primary engine,
image absence and exact same-boot inventories remain checked. This closes that
bounded active-child checkpoint; it does not provide an atomic combined doctor,
lifecycle-declared descriptor intent, native network/kernel-incarnation health
or complete readiness/diagnostics. Startup console saturation is explicitly
retained while the reserved teardown markers remain observed.

The production `--doctor-all` version-5 command now combines these views under
one deadline, preserving native/storage severity and rejecting lifecycle or
component drift. [Controls](../../../evidence/runtime-20261027/g8-combined-doctor-controls/README.md)
and [actual CLI preflight](../../../evidence/runtime-20261027/g8-combined-doctor-cli/README.md)
pass. Its [child startup attempt](../../../evidence/runtime-20261027/g8-combined-live-failure/README.md)
fails before a live report, retaining an active child/export and unproven agent
transport failure. It does not qualify version-5 live builds, cleanup or health.
The failed generation remains retained; complete G7/G8 acceptance remains open.

The [bounded retained-agent probe](../../../evidence/runtime-20261028/g8-retained-agent-capabilities-gce/README.md)
qualifies one actual capabilities response through a CID-pinned native relay,
with exact local peer checks, sealed credential FD, and unchanged primary and
retained resource identities. This narrows diagnosis to a child that answers the
probe; it does not qualify the failed Docker startup, readiness, recovery,
version-5 live doctor, normal cleanup or complete G8 diagnostics. That probe alone
does not qualify production callers. The child/export remains
retained. Direct kernels use no nested virtualization and share host trust.

The shared production relay factory subsequently adopts declared-CID enforcement
without wildcard fallback. [Local controls](../../../evidence/runtime-20261028/g8-production-relay-controls/README.md)
and [actual GCE qualification](../../../evidence/runtime-20261028/g8-production-relay-gce/README.md)
prove real wrong-peer refusal before bridge publication and a matched response
for the retained child's CID. The first failed positive case and fixture
correction are retained separately. That factory-only proof does not qualify
production local relay identity checks. Deployment and a complete fresh Docker lifecycle remain required; this factory
probe does not qualify all startup/recovery/replacement paths on a live workload.

Subsequent [production peer controls](../../../evidence/runtime-20261028/g8-production-peer-controls/README.md)
and [actual GCE peer/replacement qualification](../../../evidence/runtime-20261028/g8-production-peer-gce/README.md)
qualify exact owned PID/UID/GID verification before credentials, zero-byte
refusal, and sequence-preserving explicit rebind after an owned relay exits.
The real child returns matching capabilities through the production reconnect
loop after a new helper PID is verified. Primary and retained resource identities
match before/after. These transport methods are qualified within that test;
complete Docker workload/worker recovery, readiness, diagnostics, normal cleanup
and full G7/G8 still require evidence. No nested virtualization is used, and the
shared host trust boundary remains explicit.

A [bounded process-state diagnostic](../../../evidence/runtime-20261028/g8-retained-process-state-gce/README.md)
subsequently reports the retained guest task STOPPED/guest PID 616/exit 137 after
matching capabilities. Its explicit mode validates and redacts state under one
deadline; absence and stopped states are observations, not readiness success.
Default capabilities-only behavior remains unchanged. This separates the guest
task from the host namespace-holder zombie without proving termination cause/time
or child Docker health, and preserves primary and retained resource identities.
It does not authorize cleanup or close the complete operational gate.

The later [stopped-output summary](../../../evidence/runtime-20261028/g8-retained-stopped-output-gce/README.md)
observes workload-begin/full-pressure markers in retained guest stdout despite
zero output at the original failed host wait. The explicit mode uses zero offsets
to avoid acknowledging/discarding bytes, enforces a per-stream limit, brackets
stopped process identity, and reports only counts/offsets/flags/allowlisted lines.
These are output clues, not independent build/ENOSPC attestation; the historical
checkpoint remains failed. This narrows output-delivery/observation diagnosis
while preserving primary and retained resource identities. Complete diagnostics,
fresh Docker lifecycle/recovery, cleanup and full G7/G8 remain open.

[Private stopped-output preservation](../../../evidence/runtime-20261028/g8-private-bundle-gce/README.md)
now qualifies explicit bounded payload retention in a protected owner-only,
non-overwriting artifact while public reports remain redacted. It is one sampled
window, not the complete diagnostic bundle or cleanup authority. The
[fresh-trial relay guard](../../../evidence/runtime-20261028/g8-relay-trial-admission-gce/README.md)
also qualifies the staged compatible helper before lifecycle mutation, fixing a
legacy fixture path. At that checkpoint, the retained active child/export,
scoped graceful recovery, full Docker/doctor lifecycle, readiness and full G8
remained open; later recovery and fresh-trial proofs follow below. No nested virtualization or hostile-kernel containment is claimed.

Pass when an operator can determine runtime health and resource ownership from
bounded machine-readable output, collect safe diagnostics, follow tested
recovery guidance, and run the services with documented least privileges.


The retained 0c50de9 case now has separate [graceful-stop](../../../evidence/runtime-20261028/g8-exact-child-graceful-stop/README.md),
[strict native-close/filesystem](../../../evidence/runtime-20261028/g8-retained-native-close/README.md),
[exact operator resource-return](../../../evidence/runtime-20261028/g8-retained-resource-release/README.md)
and [cold-restoration](../../../evidence/runtime-20261028/g8-retained-cold-restoration/README.md)
proofs. Original failed ledgers, private output and the image remain preserved
and excluded from reuse. These close the exact operator recovery checkpoints;
they do not qualify that failed attempt's normal cleanup, fresh version-5 live
Docker lifecycle or the complete G8 interface/diagnostics requirements above.
The corrected staged relay prerequisite is qualified before another trial.
Direct child kernels use no nested virtualization and retain shared host trust.


The [fresh 8c048020 live proof](../../../evidence/runtime-20261028/g8-live-combined-doctor/README.md)
now qualifies version-5 combined resource/network/native-process correlation
during actual child Docker ENOSPC and built-image holds and after normal removal.
Guest/primary engine separation and child-image absence in primary Docker are
independently checked with unchanged primary inventories, file hashes and PIDs.
Normal native synced close, offline filesystem check, ownership release and
resource/network return pass. This closes that bounded fresh lifecycle gap; its
[separate cold restoration](../../../evidence/runtime-20261028/g8-live-combined-cold-restoration/README.md)
also passes with preserved primary/retained state. The full interface/readiness/
capacity/diagnostic requirements remain open. No nested virtualization is used; these direct kernels
retain shared host trust and no hostile-child containment claim is added.


The separate [pool-capacity interface](../pool-capacity.md) now compares negotiated
configured limits/durable lifecycle with independent kernel free-list and
registered-memory observations. [GCE reservation proof](../../../evidence/runtime-20261028/g8-pool-capacity-gce/README.md)
qualifies real 3 GiB/CPU allocation, conservative 4 GiB/7 GiB candidates, exact
delete/pool return, legacy refusal and unchanged primary/failed state. Every
report keeps admission_guaranteed false; missing metadata and degraded state
cannot offer candidates. Live Docker capacity holds, combined-doctor integration,
readiness and diagnostic collection still govern complete G8 acceptance.
No nested virtualization or hostile-child containment is claimed.


The [actual Docker-capacity checkpoint](../../../evidence/runtime-20261028/g8-live-docker-pool-capacity/README.md)
subsequently qualifies the pool view during ENOSPC, built-image and post-removal
holds, independently recomputing bounds and correlating exact lifecycle state.
Normal cleanup, guest/primary engine/image separation and unchanged primary
state pass. The version-5 command stays unchanged; its separate pool report does
not establish readiness or admission. Its [separate cold restoration](../../../evidence/runtime-20261028/g8-live-docker-pool-capacity-cold-restoration/README.md)
also passes. Unified capacity integration, service/component health and diagnostics remain separate
full G8 requirements. No nested virtualization or hostile-child containment is
claimed.

The explicit [version-6 unified command](../pool-capacity.md#unified-inspection)
now correlates repeated full combined and pool views under one deadline,
refusing lifecycle/sequence/boot mismatch and suppressing all capacity candidates
for any degraded/inconclusive aggregate. Local race controls and [actual
version-6 Docker holds/normal cleanup](../../../evidence/runtime-20261029/g8-live-unified-doctor/README.md)
pass. The earlier separate v5/v1 proofs retain their own scope. [Cold restoration](../../../evidence/runtime-20261029/g8-live-unified-doctor-cold-restoration/README.md)
also passes. Service/component readiness, build provenance, diagnostics and wider
failure/concurrency coverage still govern full G8; it remains open.

The separate [compiled component build inspection](../contracts/component-builds.md)
now negotiates bounded daemon labels through existing protected runtime/network
sockets and distinguishes them from the inspector revision. Repeated unknown,
unsupported or drifting views cannot become a complete success. Self-reported
labels leave executable artifact identity, process incarnation, compatibility,
kernel identity and health unobserved. This is component revision reporting,
not complete provenance/readiness. [Actual private-daemon GCE proof](../../../evidence/runtime-20261029/g8-component-build-gce/README.md)
now passes repeated compiled-label queries, legacy refusal, executable/policy
observations, clean shutdown and primary preservation. Component queries during
actual Docker holds remain pending; full provenance/readiness and G8 stay open. No nested virtualization or containment
boundary is introduced.

The explicit [version-7 operator report](../contracts/operational-doctor.md)
now correlates repeated full resource/pool and compiled component views under
one caller deadline and boot bracket. Unknown or degraded aggregate state
cannot expose capacity candidates; incomplete or drifting views expose neither
payload. Default v5 and explicit v6 semantics remain unchanged. Local race/CLI
controls pass; actual version-7 Docker holds, health/provenance and full G8 stay
open. Direct child kernels still use no nested virtualization or hostile-child
containment boundary.

The [actual version-7 Docker proof](../../../evidence/runtime-20261029/g8-live-operational-doctor/README.md)
subsequently passes all three scoped holds, separate compiled-label reports,
engine/image isolation and normal cleanup, with unchanged primary state.
Default/explicit v5 equality, v6 alias equality and v7 legacy refusal also pass
in the preceding staging proof. This closes the bounded component/resource
correlation checkpoint during real child activity. [Cold restoration](../../../evidence/runtime-20261029/g8-live-operational-doctor-cold-restoration/README.md)
also passes. Health/readiness, ongoing executable/process/kernel provenance,
compatibility, diagnostics and wider failure/concurrency coverage still govern
full G8. No nested virtualization or containment boundary is introduced.

The corrected timed trial's [cold restoration](../../../evidence/runtime-20261029/g8-live-timed-diagnostic-bundles-cold-restoration/README.md)
also passes with original transport, twelve diagnostic artifacts, both successful
and failed parent receipts, and all 93 retained files / 12 images preserved.
This restoration does not close the remaining G7/G8 requirements.

The [recorded endpoint health API](../contracts/network-health.md) adds a bounded
host-backend CHECK projection through mknetd's protected socket. It distinguishes
empty inventory, passed checks, stale/non-ready claims and failed backend checks,
with generation/digest binding and redacted failures. This advances native network
inspection. [Local controls](../../../evidence/runtime-20261030/g8-network-health-controls/README.md)
and [GCE native fault/CLI proof](../../../evidence/runtime-20261030/g8-network-health-gce/README.md)
pass missing-primary-guard degradation without repair and restored checked status,
with unchanged primary state. The Docker fixture now binds health to independently
qualified generation/descriptor references at all three holds.
[Actual child Docker qualification](../../../evidence/runtime-20261030/g8-live-docker-network-health/README.md)
passes checked observations at ENOSPC/built-image holds and idle after removal,
with exact receipt hashes, primary engine/image separation and normal cleanup.
This closes the recorded host-backend health checkpoint during that lifecycle;
integration into full readiness/diagnostic collection and end-to-end health still
remain required for complete G8.

The actual Docker health trial's [separate cold restoration](../../../evidence/runtime-20261030/g8-live-docker-network-health-cold-restoration/README.md)
also passes with original transport, fifteen current diagnostic artifacts,
both prior cases and 93 retained files / 12 images preserved. Complete service
readiness and G7/G8 remain open.


The [journal-status API](../contracts/journal-status.md) exposes in-process
observed failure and acknowledged pending intent counts without reading payloads
or repairing state. [Local controls](../../../evidence/runtime-20261030/g8-journal-status-controls/README.md),
[GCE identity-failure/CLI proof](../../../evidence/runtime-20261030/g8-journal-status-gce/README.md)
and later [canonical-key/size controls](../../../evidence/runtime-20261030/g8-journal-status-canonical-controls/README.md)
pass. Metadata RPCs can remain responsive after journal failure; they are not a
mutation-readiness signal. [Actual child Docker qualification](../../../evidence/runtime-20261030/g8-live-docker-journal-status/README.md)
now passes idle journal reports at all three holds, exact lifecycle sequence
binding, receipt hashes and normal cleanup. This qualifies observed journal-state
inspection during that lifecycle, not full service readiness, current filesystem
writability, recovery completion, process incarnation or admission. Those and the
remaining diagnostic/log-pressure/provenance requirements still govern full G8.

The actual journal-status trial's [cold restoration](../../../evidence/runtime-20261030/g8-live-docker-journal-status-cold-restoration/README.md)
also passes: original transport, eighteen current artifacts, all three prior
cases and 93 retained files / 12 images preserved. Full G7/G8 remain open.
