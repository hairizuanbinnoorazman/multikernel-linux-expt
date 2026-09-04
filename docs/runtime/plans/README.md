# Multikernel container-runtime master plan

## Goal

Build the smallest defensible path from the completed Multikernel experiments
to a containerd and Kubernetes runtime. Each plan has a hard exit gate. Work
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
| [ ] provisional | G4 | Provide images and storage | OCI roots are deterministic, writable state has one owner, and teardown is clean | [`04-images-and-storage.md`](04-images-and-storage.md) |
| [ ] provisional | G5 | Provide networking | Primary-mediated networking works through normal CNI operations | [`05-networking.md`](05-networking.md) |
| [ ] provisional | G6 | Integrate containerd | An unmodified OCI bundle can be managed through containerd and `ctr` | [`06-containerd-shim.md`](06-containerd-shim.md) |
| [ ] | G7 | Integrate Kubernetes | A multi-container pod runs through a Multikernel `RuntimeClass` | [`07-kubernetes.md`](07-kubernetes.md) |
| [ ] | G8 | Harden security and reliability | Security, fault-injection, restart, and resource-leak tests pass | [`08-security-and-reliability.md`](08-security-and-reliability.md) |
| [ ] | G9 | Measure performance and density | Reproducible comparisons and raw evidence support runtime decisions | [`09-performance-and-density.md`](09-performance-and-density.md) |
| [ ] | G10 | Build the preview release | A reproducible, opt-in developer preview passes fresh-host validation | [`10-final-build-and-release.md`](10-final-build-and-release.md) |

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

The first useful MVP ends at G6: a trusted workload can be launched from an OCI
bundle using containerd, with no direct device assignment and with reliable
cleanup. Kubernetes, stronger failure handling, and performance work are
subsequent gates rather than requirements for the first executable proof.

The first end-to-end workload should use a stock `linux/amd64` BusyBox OCI
image. Containerd pulls and unpacks the image; the Multikernel runtime consumes
the resulting OCI configuration and root filesystem. A separate Docker Engine
compatibility test follows the containerd/`ctr` proof and uses the same Runtime
v2 shim.

## Final-build boundary

The first release is a developer preview for trusted single-tenant nodes. It
must not advertise hostile multi-tenant isolation, seamless upstream
compatibility, or production readiness.
