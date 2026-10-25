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

The [bounded storage-pressure case](evidence/runtime-20261024/g8-pressure-recovery-qualified/README.md)
and its [same-boot repeat](evidence/runtime-20261024/g8-pressure-warm-repeat/README.md)
now pass on GCE with the fixed runtime: exact zero-space ENOSPC, capacity
recovery, same-engine child builds, clean filesystem release and automatic
CPU/pool/network/storage cleanup. Installed service PIDs/files and raw network
state match their baseline; VSOCK references return to zero. The
[parser fix and controls](evidence/runtime-20261024/g8-native-close-timing-controls/README.md)
address the earlier timing-record cleanup refusal. A separate
[saved-image content proof](evidence/runtime-20261024/g8-pressure-saved-image/README.md)
verifies the preceding recovery build's output, layer digests and BusyBox input.
The repeat uses fresh engine/storage generations on the same CPUs/CID without
reloading the transport. This accepts the bounded case and one normal repeat;
broader lifecycle/recovery coverage and full G7/G8 remain open.
[Final restoration proof](evidence/runtime-20261024/g8-qualified-series-restoration/README.md)
verifies the original module is restored and successful receipts/binaries plus
retained failed storage survive unchanged.

[Operator quarantine](evidence/runtime-20261024/g8-pressure-recovery-quarantine/README.md)
returns resources and retains the recovered disk/failed ledgers; read-only filesystem
checking passes. A separate [same-boot startup failure](evidence/runtime-20261024/g8-warm-start-failure/README.md)
remains unresolved. Earlier transport failures, bounded retry/readiness fixes,
filtered diagnostics and module-restoration evidence are retained in the
[Docker guest investigation](docs/runtime/research/docker-in-docker-guests.md).

A [live primary-store boundary check](evidence/runtime-20261024/g8-primary-docker-boundary/README.md)
also holds a child-built image before pruning and proves its ID is absent from
the primary Docker engine. Primary image/tag, container, volume and network
inventory fingerprints match before, during pressure, while that image exists,
and after cleanup. These are inventory identities, not a hash of volume contents.
The separate [cold-restoration proof](evidence/runtime-20261024/g8-primary-boundary-restoration/README.md)
confirms the original module and retained artifacts after reboot. Network IDs
changed across that reboot; future inventory comparisons require the same boot
identity, with [guard controls](evidence/runtime-20261024/g8-primary-epoch-controls/README.md).

The read-only `mkruntimed --doctor` command now compares recorded ownership with
kernel instance names. Its [initial proof](evidence/runtime-20261025/g8-runtime-doctor/README.md)
covers bounded inspection and explicit inconclusive results; complete health,
storage/network lease checks and live kernel incarnation remain open.
A [compatible-daemon check](evidence/runtime-20261025/g8-compatible-daemon-doctor/README.md)
now verifies three identical presence reports against the real idle GCE kernel
inventory. Unsupported installed daemons receive `OWNERSHIP_UNSUPPORTED`;
failed reports retain the initial boot ID without asserting boot stability.
The [live child qualification](evidence/runtime-20261025/g8-live-child-doctor/README.md)
now checks the same sandbox generation during ENOSPC and the built-image hold,
then verifies empty ownership after removal. Primary Docker inventories remain
unchanged. This run's console hit its bounded output cap before teardown;
independent native close, filesystem and resource checks prove normal cleanup.
The [separate restoration proof](evidence/runtime-20261025/g8-live-doctor-restoration/README.md)
confirms the original transport and retained artifacts after an idle reboot.

The [reserved console window](evidence/runtime-20261025/g8-console-window-qualified/README.md)
now retains shutdown markers by attaching before the held workload exits.
Startup and teardown share the existing 1 MiB cap and report the unobserved
interval explicitly. The earlier late-attachment failure remains recorded;
complete diagnostics and live arbitrary-console-flood coverage remain open.
Its [cold-restoration proof](evidence/runtime-20261025/g8-console-window-restoration/README.md)
confirms the original transport and eighteen retained artifact hashes.

The negotiated `mkruntimed --inspect-storage` view now exposes recorded storage
ownership without backing paths or credentials. Its [GCE proof](evidence/runtime-20261025/g8-storage-ownership/README.md)
covers empty state and a copy of a real completed export. Live storage/lifecycle
correlation and native-process health remain open.

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
