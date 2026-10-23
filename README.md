# Multikernel Linux experiments and container runtime

This repository validates Multikernel Linux on Google Compute Engine and
develops a Kerf-backed containerd/Kubernetes runtime from those results.

**Deployment constraint: nested virtualization is not allowed in the target
compute instances.** KVM/microVMs inside those instances are not an available
containment solution. The primary and its Multikernel children form one trust
domain; the outer cloud hypervisor separates instances, not sibling kernels.
See the [deployment and containment constraint](docs/runtime/architecture.md#deployment-and-containment-constraint)
before proposing security architecture changes.

This deployment uses direct child kernels because nested virtualization is
unavailable under its cloud-instance constraints. Host Docker selects the
Multikernel runtime; Docker builds inside an approved Docker guest must use
that child's local engine and storage, without the host Docker socket.
The approved profile has [passed offline multi-stage Docker builds and clean
mediated-storage shutdown on a direct child](evidence/runtime-20261022/g8-production-docker-build/README.md).
The [host Docker selection and cleanup check](evidence/runtime-20261022/g8-host-docker-lifecycle/README.md)
also passes with independent test daemon state. Crash recovery, concurrency and
adversarial release qualification remain open; the profile is experimental.
A [graceful private runtime restart](evidence/runtime-20261022/g8-host-docker-runtime-recovery/README.md)
also preserves the live child, engine and saved image; crash and other service
restart cases remain separate. The documented daemon capability and NNP bounds
also [pass build and recovery qualification](evidence/runtime-20261022/g8-host-docker-service-policy/README.md). Passing Docker builds does not prove containment
after child-kernel compromise. A [two-child concurrency checkpoint](evidence/runtime-20261022/g8-concurrent-build-checkpoint/README.md)
records overlapping builds, separate engines/storage and survivor cleanup.
An earlier export failure remains retained; the [NBD proxy regression proof](evidence/runtime-20261022/g8-native-stream-proxy/README.md)
reproduces and fixes a bidirectional backpressure defect. Concurrent reliability
and complete gate acceptance remain open. The [fresh candidate run](evidence/runtime-20261022/g8-candidate-release-failure/README.md)
passes both child build/export matrices but fails sibling release. An
[exact-generation quarantine](evidence/runtime-20261023/g8-failed-generation-quarantine/README.md)
returns the child resources while preserving the failed disk and ownership;
automatic recovery acceptance remains open.
See the
[Docker guest design and evidence](docs/runtime/research/docker-in-docker-guests.md).

Start with the [documentation map](docs/README.md). In particular:

- [Project and runtime status](docs/runtime/status.md)
- [Runtime operator quickstart](docs/guides/runtime-operator-quickstart.md)
- [Developer workflow](docs/guides/developer-workflow.md)
- [Experiment index](docs/experiments/README.md)
- [Project tasks and runtime gates](docs/project/TASKS.md)

Cloud experiments can create billable resources. Review the
[GCE laboratory runbook](docs/guides/gce-lab-runbook.md) before running them.
