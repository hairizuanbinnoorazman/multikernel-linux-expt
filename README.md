# Multikernel Linux experiments and container runtime

This repository validates Multikernel Linux on Google Compute Engine and
develops a Kerf-backed containerd/Kubernetes runtime from those results.

**Deployment constraint: nested virtualization is not allowed in the target
compute instances.** KVM/microVMs inside those instances are not an available
containment solution. The primary and its Multikernel children form one trust
domain; the outer cloud hypervisor separates instances, not sibling kernels.
See the [deployment and containment constraint](docs/runtime/architecture.md#deployment-and-containment-constraint)
before proposing security architecture changes.

Host Docker selects the Multikernel runtime; Docker builds inside an approved
Docker guest use that child's local engine and storage, without the host Docker
socket. The latest [two-child qualification](evidence/runtime-20261023/g8-concurrent-netns/README.md)
passes overlapping builds, separate engines and writable storage, survivor
continuity, clean export shutdown and primary network restoration. Watched
installed files and service PIDs remain unchanged. The disposable host Docker
daemon has its own network namespace; the children still use direct kernels.

Earlier [production-profile builds](evidence/runtime-20261022/g8-production-docker-build/README.md),
[host Docker lifecycle](evidence/runtime-20261022/g8-host-docker-lifecycle/README.md),
[graceful runtime restart](evidence/runtime-20261022/g8-host-docker-runtime-recovery/README.md)
and [daemon capability/NNP checks](evidence/runtime-20261022/g8-host-docker-service-policy/README.md)
have separate pinned evidence. Export, exec-release and primary-bridge failures
remain retained alongside their subsequent changes; see the
[Docker guest design and evidence](docs/runtime/research/docker-in-docker-guests.md).

A [private runtime SIGKILL checkpoint](evidence/runtime-20261023/g8-runtime-sigkill/README.md)
also preserves the live child's boot, engine, archive and ownership, then passes
fresh child-local builds and cleanup through the recovered stack.

A [bounded executing-RUN crash case](evidence/runtime-20261023/g8-runtime-active-run/README.md)
also passes: SIGKILL and adoption occur inside an observed BuildKit RUN with a
45-second sleep, the interrupted matrix finishes, and fresh builds succeed.

The [strict storage-pressure recovery workload](evidence/runtime-20261024/g8-pressure-recovery-cleanup-failure/README.md)
now reaches zero available space with real ENOSPC, restores capacity, and completes
the build matrix on the same child-local engine. Its
[saved image contents](evidence/runtime-20261024/g8-pressure-saved-image/README.md)
verify against the expected output, layer digests and BusyBox input. Normal cleanup
still fails: the runtime's close parser rejects the helper's new I/O timing records.
The [committed parser fix and controls](evidence/runtime-20261024/g8-native-close-timing-controls/README.md)
validate those bounded records while preserving strict lifecycle checks; live
acceptance with the fixed runtime is pending.

[Operator quarantine](evidence/runtime-20261024/g8-pressure-recovery-quarantine/README.md)
returns resources and retains the recovered disk/failed ledgers; read-only filesystem
checking passes. A separate [same-boot startup failure](evidence/runtime-20261024/g8-warm-start-failure/README.md)
remains unresolved. Earlier transport failures, bounded retry/readiness fixes,
filtered diagnostics and module-restoration evidence are retained in the
[Docker guest investigation](docs/runtime/research/docker-in-docker-guests.md).

The profile remains experimental. Further crash points during resource mutation
or build I/O, host power loss, other service crashes, repeated concurrency,
hostile workload pressure and complete G7/G8 acceptance remain open. These checks do not establish
containment after child-kernel compromise.

Start with the [documentation map](docs/README.md). In particular:

- [Project and runtime status](docs/runtime/status.md)
- [Runtime operator quickstart](docs/guides/runtime-operator-quickstart.md)
- [Developer workflow](docs/guides/developer-workflow.md)
- [Experiment index](docs/experiments/README.md)
- [Project tasks and runtime gates](docs/project/TASKS.md)

Cloud experiments can create billable resources. Review the
[GCE laboratory runbook](docs/guides/gce-lab-runbook.md) before running them.
