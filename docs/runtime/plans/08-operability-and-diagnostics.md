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

Pass when an operator can determine runtime health and resource ownership from
bounded machine-readable output, collect safe diagnostics, follow tested
recovery guidance, and run the services with documented least privileges.
