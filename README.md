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
covers empty state and a copy of a real completed export. The
[version-2 doctor](evidence/runtime-20261025/g8-live-resource-correlation/README.md)
now correlates lifecycle expectations with recorded export generations, CID and
full-descriptor fingerprints during real child Docker activity. Native-process,
current-file and socket health remain unobserved.
The [separate restoration proof](evidence/runtime-20261025/g8-resource-correlation-restoration/README.md)
confirms the original module and retained correlation artifacts after reboot.

An internal [native process inspector](evidence/runtime-20261025/g8-native-process-inspection/README.md)
now checks exact export process and backing-file identity without invoking
recovery or removing stale records. Its GCE controls pass; the initial fixture
allocation failure is retained with its correction. The
[version-3 doctor](docs/runtime/contracts/native-resource-identity.md) now requires
negotiated native identity observations. Its
[live GCE proof](evidence/runtime-20261025/g8-live-native-resource-identity/README.md)
passes ENOSPC, child-local builds and normal cleanup, with matched active export
identity and unchanged primary Docker inventories. Full native export/socket
health remains open. The [separate restoration proof](evidence/runtime-20261025/g8-native-resource-restoration/README.md)
confirms the original transport and eighteen artifact hashes after reboot.
These checks use direct kernels without
nested virtualization and retain the shared host trust boundary.

A [live missing-record test](evidence/runtime-20261025/g8-live-native-record-loss/README.md)
now proves the negative diagnostic path: the exact active export record is
hidden, doctor reports degraded without changing ownership, and the same child
engine still runs a fresh inner container. Restoring the original record returns
diagnostics to consistent; normal cleanup and primary inventories also pass.
This is metadata loss, with direct child kernels and no nested virtualization;
process-death recovery and hostile child-kernel containment remain open. Its
[restoration proof](evidence/runtime-20261025/g8-native-record-loss-restoration/README.md)
confirms the original transport and twenty-six retained artifact hashes.

Private regular-file validation now rejects unpaired FIFOs before opening can
wait for a writer. The [red/green controls](evidence/runtime-20261025/g8-fifo-open-controls/README.md)
reproduce the old waits and verify the fix. Its
[live GCE case](evidence/runtime-20261025/g8-live-fifo-record/README.md) returns
inconclusive diagnostics promptly while child Docker keeps working, restores
the exact record, and passes normal cleanup with unchanged primary inventories.
This qualifies FIFO refusal, with direct kernels and no nested virtualization;
arbitrary filesystem stalls and full G7/G8 remain open. The
[separate restoration proof](evidence/runtime-20261025/g8-fifo-record-restoration/README.md)
confirms the original transport and twenty-six retained artifact hashes.

An [actual native export SIGKILL test](evidence/runtime-20261026/g8-live-native-export-crash/README.md)
now proves process absence detection after an inner build and nonretryable
`OPERATOR_ACTION` on deletion. The failed image and ownership records remain
fenced. [Operator quarantine](evidence/runtime-20261026/g8-native-export-crash-quarantine/README.md)
returns child resources while preserving those records; read-only e2fsck exits 4.
This qualifies failure retention, with direct child kernels on GCE and no nested
virtualization. Automatic storage recovery and normal cleanup after this crash
remain unqualified. The [cold restoration proof](evidence/runtime-20261026/g8-native-export-crash-restoration/README.md)
confirms the original transport and unchanged quarantined images and ledgers.

[Ordered network-policy checks](evidence/runtime-20261026/g8-ordered-endpoint-firewall/README.md)
now refuse private endpoint chains with ACCEPT/RETURN bypasses despite all
required rules being present. Real iptables controls pass in an isolated GCE
network namespace. [Ingress guard precedence](evidence/runtime-20261026/g8-ingress-guard-precedence/README.md)
now also detects earlier IPv4/IPv6 INPUT/FORWARD bypasses, with an actual TCP
control. Continuous enforcement, other firewall tables and live Docker
requalification remain open.

The latest [combined live requalification](evidence/runtime-20261026/g8-live-network-check-failure/README.md)
failed before ENOSPC and the new network CHECK points; forced Docker removal
reported a zombie task and left orphan helpers. The
[post-restart record](evidence/runtime-20261027/g8-failed-network-post-restart/README.md)
confirms the host is idle on the original transport while failed ownership and
the image remain retained. Reboot and a clean read-only filesystem check do not
qualify normal cleanup or authorize reuse. New
[bounded wait-outcome controls](evidence/runtime-20261027/g8-pressure-wait-outcomes/README.md)
record process exit, deadline or output-limit failures before teardown. The
previous failure's cause and live Docker requalification remain open.

The shim now [retains its host task PID through failed deletion](evidence/runtime-20261027/g8-delete-holder-retention/README.md).
Real-process controls cover five failure points and transient retries, locally
and on GCE. This fixes a cleanup-order defect; it does not establish the pressure
stall's cause or prevent Docker from externally killing a cached PID.

A [bounded queue recovery diagnostic](evidence/runtime-20261027/g8-queue-resume-diagnostics/README.md)
now distinguishes the first positive send after MK queue-full from the initial
queue-full observation. Local and GCE socket controls pass; the candidate helper
is not deployed and this does not resolve the earlier live stall.

A separate [network ownership inspector](docs/runtime/contracts/network-ownership.md)
now exposes bounded generation/boot metadata and opaque descriptor fingerprints
through mknetd's existing protected socket. GCE CLI, retained-ledger and privacy
controls pass; recorded READY state is not a network-health or isolation pass.

The separate [network doctor](evidence/runtime-20261027/g8-network-doctor/README.md)
now correlates explicit lifecycle network intent with repeated endpoint and
kernel-name observations. New shim tasks declare required networking; legacy
intent remains unknown. GCE idle CLI and injected mismatch/drift controls pass.
This does not attest live network policy, child isolation or Docker builds.

Native endpoint CHECK now rejects substring lookalikes in MTU, route, address,
link flags and forwarding values. [GCE state-value proof](evidence/runtime-20261027/g8-endpoint-state-values/README.md)
covers real MTU/gateway changes, refusal and restoration in disposable network
namespaces. These qualify sampled checks; the later active-child trial is linked below.

The [early trial capacity guard](evidence/runtime-20261027/g8-trial-capacity-admission/README.md)
now refuses insufficient storage before transport switching or Docker setup.
It preserves the 2 GiB image and 1 GiB reserve; passing its minimum floor still
requires the builder's staging and capacity checks. The approved data-disk
expansion has now removed the previous minimum-capacity blocker.

The candidate host NBD helper now retries MK ring-full reply/read sends within
the original frame deadline. [Real server-loop GCE controls](evidence/runtime-20261027/g8-server-queue-retry/README.md)
pass with injected pressure, partial writes and unchanged primary state. The
installed helper pins remain unchanged; live candidate qualification is tracked
in the runtime status.

Disposable trials now have an explicit [static candidate helper pin](evidence/runtime-20261027/g8-static-trial-helper/README.md)
while retaining the qualified helper as the default. GCE checks both pins,
unsafe-artifact refusals and static/dynamic native server controls. Current
runtime binaries and source are staged with installed defaults unchanged. The
[private candidate trial](evidence/runtime-20261027/g8-live-static-candidate/README.md)
now passes actual ENOSPC recovery, repeated builds, child-engine/image separation
and normal cleanup. [Cold restoration proof](evidence/runtime-20261027/g8-live-static-restoration/README.md)
confirms the original transport, all retained file hashes and primary services
after one graceful reboot. Full G7/G8 remains open.

The [committed trial-packet builder](docs/runtime/trial-staging.md) now prepares
protected artifacts from a Git snapshot, excluding dirty sources and ambient Go
workspace/overlay flags.
[Real GCE extraction proof](evidence/runtime-20261027/g8-trial-stage-builder/README.md)
checks source/binary hashes and outer root-owned modes. The later
[source-mode refusal and fix](evidence/runtime-20261027/g8-source-archive-modes/README.md)
qualifies every nested source mode; packet preparation itself starts no child.

The [approved 30-to-50 GB data-disk expansion](evidence/runtime-20261027/g8-data-disk-expansion/README.md)
now passes full preservation checks for all 92 retained files, including 11 child
images, with the original ext4 reserve percentage restored. Image and builder
reserves remain unchanged. The source archive also now forces protected modes
before a cold trial can pass its source-directory guard. The deployment manager
now supports a protected, size-only immutable storage-binding update; see the
[operator procedure](docs/guides/runtime-operator-quickstart.md#after-an-approved-storage-grow).

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
