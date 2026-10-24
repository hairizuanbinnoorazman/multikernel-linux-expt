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

The [storage-pressure attempt](evidence/runtime-20261023/g8-pressure-io-failure/README.md)
currently fails with guest I/O errors before ENOSPC. A read-only check found
uncorrected guest filesystem errors. Its disk and ledgers are
[preserved for diagnosis](evidence/runtime-20261023/g8-pressure-quarantine/README.md),
with CPU/pool/network resources returned. Pressure recovery is unqualified. A repeat with
[bounded child-console capture](evidence/runtime-20261023/g8-pressure-console-failure/README.md)
records NBD connection reset/broken pipe before the filesystem journal abort.
A [client-instrumented repeat](evidence/runtime-20261023/g8-pressure-client-timeout/README.md)
records an NBD timeout before journal abort and failed Docker cleanup. The
[external restart observation](evidence/runtime-20261024/g8-pressure-external-restart/README.md)
confirms resources returned and failed storage retained. The underlying timeout
cause remains under investigation. An earlier
[server-timed run](evidence/runtime-20261024/g8-server-io-pressure-failure/README.md)
records slow WRITE payload transfers and fails pressure/cleanup. Its
[operator recovery proof](evidence/runtime-20261024/g8-server-io-quarantine/README.md)
returns resources and restores raw primary network state while retaining the disk
and failed ledgers. [Per-phase summary controls](evidence/runtime-20261024/g8-phase-summary-controls/README.md)
pass. A later [combined transport trial](evidence/runtime-20261024/g8-coarse-fill-refusal/README.md)
reaches filesystem ENOSPC with zero native I/O errors and reclaims socket references,
but its coarse writes leave 496 KiB available, so the strict pressure gate refuses it.
[Operator quarantine](evidence/runtime-20261024/g8-coarse-fill-quarantine/README.md)
returns resources and preserves the image; read-only filesystem checking passes.
The committed probe now flushes coarse writes and consumes the remaining tail
with bounded 4 KiB writes. Full pressure recovery and normal cleanup remain unqualified.

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
