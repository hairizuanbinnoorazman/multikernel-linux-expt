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

Pass when an operator can determine runtime health and resource ownership from
bounded machine-readable output, collect safe diagnostics, follow tested
recovery guidance, and run the services with documented least privileges.
