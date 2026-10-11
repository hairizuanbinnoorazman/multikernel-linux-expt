# Plan 08: Operability, diagnostics, and maintainability

## Purpose

Turn the proven runtime into a system that an operator can inspect, diagnose,
and recover without reading implementation code or relying on test-only shell
harnesses.

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

## Gate G8

Pass when an operator can determine runtime health and resource ownership from
bounded machine-readable output, collect safe diagnostics, follow tested
recovery guidance, and run the services with documented least privileges.
