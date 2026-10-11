# Multikernel container-runtime master plan

## Goal

Build the smallest defensible path from the completed Multikernel experiments
to a standalone containerd/Docker runtime and, later, a Kubernetes
distribution. Each plan has a hard exit gate. Work
may be prototyped ahead of a gate, but later stages cannot be declared complete
until all earlier gates pass.

This directory is the working folder for the next implementation steps. Start
with this file, then follow the numbered plan for the first unchecked gate.

## Master gate checklist

This is the canonical section-level checklist for the runtime. Change a gate
to `[x]` only when the `Gate` section in its linked plan passes; partial
experiments and prototype code do not complete a gate. Detailed experiment
tasks remain in [`../../project/TASKS.md`](../../project/TASKS.md).

| Status | Gate | Work | Required outcome | Plan |
| --- | --- | --- | --- | --- |
| [x] pass | G0 | Freeze scope and contracts | Ownership, lifecycle, protocol, errors, kernel policy, and evidence rules are reviewable | [`00-scope-and-contracts.md`](00-scope-and-contracts.md) |
| [x] pass | G1 | Qualify the host | Required host features pass and the actual isolation boundary is documented | [`01-host-and-isolation.md`](01-host-and-isolation.md) |
| [x] pass | G2 | Build the control plane | `mkruntimed` and its Kerf adapter recover state and return resources safely | [`02-control-plane.md`](02-control-plane.md) |
| [x] pass | G3 | Build the child agent | One OCI bundle runs through `mk-agent` with correct process and shutdown semantics | [`03-agent-and-processes.md`](03-agent-and-processes.md) |
| [x] pass | G4 | Provide images and storage | OCI roots are deterministic, writable state has one owner, and teardown is clean | [`04-images-and-storage.md`](04-images-and-storage.md) |
| [x] pass | G5 | Provide networking | Primary-mediated networking works through normal CNI operations | [`05-networking.md`](05-networking.md) |
| [x] pass | G6 | Integrate containerd and Docker | Unmodified OCI bundles pass the supported Runtime v2 lifecycle through both clients | [`06-containerd-shim.md`](06-containerd-shim.md) |
| [ ] | G7 | Harden security and reliability | Security, fuzzing, fault-injection, and resource-leak tests pass | [`07-security-and-reliability.md`](07-security-and-reliability.md) |
| [ ] | G8 | Operationalize the runtime | Health, diagnostics, recovery, service hardening, and maintainability gates pass | [`08-operability-and-diagnostics.md`](08-operability-and-diagnostics.md) |
| [ ] | G9 | Freeze compatibility and release engineering | The support matrix, CI, reproducible artifacts, SBOM, and lifecycle tooling pass | [`09-compatibility-and-release-engineering.md`](09-compatibility-and-release-engineering.md) |
| [ ] | G10 | Measure performance and density | Reproducible comparisons and raw evidence support runtime decisions | [`10-performance-and-density.md`](10-performance-and-density.md) |
| [ ] | G11 | Build the standalone preview | A reproducible, opt-in containerd/Docker preview passes fresh-host validation | [`11-standalone-preview.md`](11-standalone-preview.md) |
| [ ] deferred | G12 | Distribute through Kubernetes | A multi-container pod runs through a Multikernel `RuntimeClass` | [`12-kubernetes-distribution.md`](12-kubernetes-distribution.md) |

## Cross-plan rules

- Every cloud test begins with a cost and resource ledger and ends with a
  cleanup ledger.
- Every failure test uses disposable images or a recoverable snapshot.
- Every test records exact kernel, Kerf, runtime, agent, and helper revisions.
- Unit and contract tests run without GCE. Privileged integration tests are
  explicitly labeled and never hidden inside the default test target.
- Evidence is a required output, not an optional troubleshooting artifact.
- Passing a happy-path boot never substitutes for stop, delete, reconciliation,
  and resource-return tests.

## MVP boundary

The executable MVP ends at G6: a trusted workload can be launched from an OCI
bundle using containerd, with no direct device assignment and with reliable
cleanup. G4-G6 closed against the audited evidence bundle under
`evidence/runtime-20261010/g4-g6-final-2ee14c4`. Security, operability,
compatibility, performance, and release work are subsequent standalone gates.
Kubernetes is deliberately deferred to G12 and does not block the standalone
preview.

The first end-to-end workload should use a stock `linux/amd64` BusyBox OCI
image. Containerd pulls and unpacks the image; the Multikernel runtime consumes
the resulting OCI configuration and root filesystem. A separate Docker Engine
compatibility test follows the containerd/`ctr` proof and uses the same Runtime
v2 shim.

## Final-build boundary

G11 produces a standalone developer preview for trusted single-tenant nodes.
G12 may later distribute that runtime through Kubernetes. The preview
must not advertise hostile multi-tenant isolation, seamless upstream
compatibility, or production readiness.
