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
It remains experimental; host Docker/production shim integration is open. Passing Docker builds does not prove containment
after child-kernel compromise. See the
[Docker guest design and evidence](docs/runtime/research/docker-in-docker-guests.md).

Start with the [documentation map](docs/README.md). In particular:

- [Project and runtime status](docs/runtime/status.md)
- [Runtime operator quickstart](docs/guides/runtime-operator-quickstart.md)
- [Developer workflow](docs/guides/developer-workflow.md)
- [Experiment index](docs/experiments/README.md)
- [Project tasks and runtime gates](docs/project/TASKS.md)

Cloud experiments can create billable resources. Review the
[GCE laboratory runbook](docs/guides/gce-lab-runbook.md) before running them.
