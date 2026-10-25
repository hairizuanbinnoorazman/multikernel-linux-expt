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
installed services. Active-child/build inspection still requires qualification.
Storage/network ownership, live incarnation, pool capacity, full service
readiness and diagnostic bundles remain unobserved by this command and required
for the complete gate.

Pass when an operator can determine runtime health and resource ownership from
bounded machine-readable output, collect safe diagnostics, follow tested
recovery guidance, and run the services with documented least privileges.
