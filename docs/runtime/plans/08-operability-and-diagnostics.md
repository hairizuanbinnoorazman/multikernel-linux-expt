# Plan 08: Operability, diagnostics, and maintainability

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
fault checkpoint. New event real-stderr-pipe coverage and actual child Docker
for this source remain unqualified. Supervisor/daemon logs, Task RPC errors,
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
