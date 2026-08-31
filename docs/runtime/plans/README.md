# Multikernel container-runtime master plan

## Goal

Build the smallest defensible path from the completed Multikernel experiments
to a containerd and Kubernetes runtime. Each plan has a hard exit gate. Work
may be prototyped ahead of a gate, but later stages cannot be declared complete
until all earlier gates pass.

## Ordered plans

| Order | Plan | Outcome | Gate |
| ---: | --- | --- | --- |
| 0 | [`00-scope-and-contracts.md`](00-scope-and-contracts.md) | Frozen MVP semantics and evidence rules | G0 |
| 1 | [`01-host-and-isolation.md`](01-host-and-isolation.md) | Qualified host and honest isolation boundary | G1 |
| 2 | [`02-control-plane.md`](02-control-plane.md) | Recoverable daemon and Kerf adapter | G2 |
| 3 | [`03-agent-and-processes.md`](03-agent-and-processes.md) | OCI process lifecycle inside one child | G3 |
| 4 | [`04-images-and-storage.md`](04-images-and-storage.md) | Deterministic OCI roots and safe teardown | G4 |
| 5 | [`05-networking.md`](05-networking.md) | CNI-compatible mediated networking | G5 |
| 6 | [`06-containerd-shim.md`](06-containerd-shim.md) | `ctr` and containerd Runtime v2 operation | G6 |
| 7 | [`07-kubernetes.md`](07-kubernetes.md) | Kubernetes pod through `RuntimeClass` | G7 |
| 8 | [`08-security-and-reliability.md`](08-security-and-reliability.md) | Failure matrix and bounded security claim | G8 |
| 9 | [`09-performance-and-density.md`](09-performance-and-density.md) | Decision-quality comparative measurements | G9 |
| 10 | [`10-final-build-and-release.md`](10-final-build-and-release.md) | Reproducible preview release | G10 |

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

## Final-build boundary

The first release is a developer preview for trusted single-tenant nodes. It
must not advertise hostile multi-tenant isolation, seamless upstream
compatibility, or production readiness.
