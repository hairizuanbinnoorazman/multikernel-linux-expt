# Multikernel Linux experiments and container runtime

This repository validates Multikernel Linux on Google Compute Engine and
develops a Kerf-backed containerd/Kubernetes runtime from those results.

**Deployment constraint: nested virtualization is not allowed in the target
compute instances.** KVM/microVMs inside those instances are not an available
containment solution. The primary and its Multikernel children form one trust
domain; the outer cloud hypervisor separates instances, not sibling kernels.
See the [deployment and containment constraint](docs/runtime/architecture.md#deployment-and-containment-constraint)
before proposing security architecture changes.

Start with the [documentation map](docs/README.md). In particular:

- [Project and runtime status](docs/runtime/status.md)
- [Runtime operator quickstart](docs/guides/runtime-operator-quickstart.md)
- [Developer workflow](docs/guides/developer-workflow.md)
- [Experiment index](docs/experiments/README.md)
- [Project tasks and runtime gates](docs/project/TASKS.md)

Cloud experiments can create billable resources. Review the
[GCE laboratory runbook](docs/guides/gce-lab-runbook.md) before running them.
