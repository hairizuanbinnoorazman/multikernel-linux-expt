# Multikernel Linux experiments and container runtime

This repository validates Multikernel Linux on Google Compute Engine and
develops a Kerf-backed containerd/Kubernetes runtime from those results.

**Deployment constraint: nested virtualization is not allowed in the target
compute instances.** KVM/microVMs inside those instances are not an available
containment solution. The primary and its Multikernel children form one trust
domain; the outer cloud hypervisor separates instances, not sibling kernels.
See the [deployment and containment constraint](docs/runtime/architecture.md#deployment-and-containment-constraint)
before proposing security architecture changes.

The latest [actual version-7 Docker qualification](evidence/runtime-20261029/g8-live-operational-doctor/README.md)
passes host-selected direct-child Docker pressure/build/image-engine isolation,
correlated resource/network/pool and compiled daemon views at both live holds
and after removal, and normal cleanup. Primary files, PIDs, exact Docker
inventories and network state match. Select `--doctor-all --doctor-format=7`;
default v5 and explicit v6 keep their scopes. Its [cold restoration](evidence/runtime-20261029/g8-live-operational-doctor-cold-restoration/README.md)
also passes: original transport restored, all 93 retained files / 12 images and
primary persistent Docker inventories verified. Full G7/G8 readiness, provenance,
health and diagnostics remain open.

The earlier [actual unified-doctor Docker qualification](evidence/runtime-20261029/g8-live-unified-doctor/README.md)
passes host-selected direct-child Docker ENOSPC recovery, multistage/repeat
builds, guest/primary engine and image separation, version-6 resource/network/
pool correlation at both live holds and after removal, and normal cleanup.
Primary file hashes, PIDs and exact Docker inventories match. Its [cold restoration](evidence/runtime-20261029/g8-live-unified-doctor-cold-restoration/README.md)
also passes: original transport restored, all 93 retained files / 12 images and
primary persistent Docker inventories verified. Complete G7/G8 readiness, health,
provenance, diagnostics and wider failure/concurrency requirements remain open.

The earlier [actual Docker/combined-doctor qualification](evidence/runtime-20261028/g8-live-combined-doctor/README.md)
passes host Docker → approved child Docker multistage/repeat builds, ENOSPC
recovery, child/primary engine and image separation, version-5 doctor checks at
both live holds and after removal, and normal resource cleanup. Primary file
hashes, service PIDs and exact Docker inventories remain unchanged. The corrected
packet includes the pinned CID relay. Its [cold restoration](evidence/runtime-20261028/g8-live-combined-cold-restoration/README.md)
also passes, restoring the original transport and preserving all 93 retained
files / 12 images and primary persistent Docker inventories. Full G7/G8
readiness, capacity and diagnostic requirements remain open. These checks use direct kernels and do not establish containment after
child-kernel compromise.

The new [pool-capacity command](docs/runtime/pool-capacity.md) correlates configured
limits and durable ownership with actual kernel CPU free lists and registered
memory use. [GCE reservation-level proof](evidence/runtime-20261028/g8-pool-capacity-gce/README.md)
passes candidate bounds and normal resource return with unchanged primary state.
The [actual Docker pressure/build capacity checkpoint](evidence/runtime-20261028/g8-live-docker-pool-capacity/README.md)
also passes with matching live ownership and normal cleanup. Its [separate cold restoration](evidence/runtime-20261028/g8-live-docker-pool-capacity-cold-restoration/README.md)
restores the original transport and verifies all 93 retained files / 12 images
and primary persistent Docker inventories. Admission and
service readiness are not guaranteed; full G7/G8 remains open.

The explicit `--doctor-all-pool` command now correlates storage/native/network
and pool-capacity views under one deadline. [Local race and CLI controls](evidence/runtime-20261029/g8-unified-doctor-controls/README.md)
pass. [Actual version-6 Docker qualification](evidence/runtime-20261029/g8-live-unified-doctor/README.md)
also passes all three holds and normal cleanup, followed by its linked cold
restoration. Existing separate
version-5/version-1 commands retain their qualified scope.

The separate `--inspect-components` command now reads bounded compiled daemon
build labels through protected sockets; see the [self-report contract](docs/runtime/contracts/component-builds.md).
Unknown, unsupported or drifting labels cannot become a complete success.
[Actual private-daemon GCE qualification](evidence/runtime-20261029/g8-component-build-gce/README.md)
passes repeated queries, legacy refusal, declared privilege policy and clean
shutdown with unchanged primary state. Artifact/kernel attestation and health
remain unobserved by the API. [Component reporting during real Docker holds](evidence/runtime-20261029/g8-live-operational-doctor/README.md)
now passes with the explicit version-7 operator report and normal cleanup,
followed by its linked cold restoration.

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

An earlier [combined live requalification](evidence/runtime-20261026/g8-live-network-check-failure/README.md)
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

An earlier [live network/storage doctor checkpoint](evidence/runtime-20261027/g8-live-network-doctor/README.md)
now matches the actual child generation, required endpoint and descriptor at
ENOSPC and built-image holds, then confirms endpoint removal. Child-local builds
and primary Docker inventory separation still pass. [Cold restoration](evidence/runtime-20261027/g8-network-doctor-cold-restoration/README.md)
and a separate [user-restart verification](evidence/runtime-20261027/g8-user-restart-3ae4/README.md)
preserve all 92 retained files and the original transport. Startup console
truncation remains explicit; complete health/diagnostics and full G7/G8 stay open.

The new `mkruntimed --doctor-all` command combines native/storage and network
ownership under one deadline. [Controls](evidence/runtime-20261027/g8-combined-doctor-controls/README.md)
and [real CLI preflight](evidence/runtime-20261027/g8-combined-doctor-cli/README.md)
pass. Its [child trial's host wait fails before live doctor checks](evidence/runtime-20261027/g8-combined-live-failure/README.md),
with a retained active child/export and unproven startup cause. At that failed checkpoint, version-5 live
qualification and normal cleanup remained open; later passing trials above use
different generations and do not relabel the failure; the failed generation is retained
for diagnosis. Earlier passing child Docker proofs retain their own scope.

An independently reproduced [agent stream reuse defect](evidence/runtime-20261027/g8-agent-stream-invalidation/README.md)
is now fixed: interrupted frames close the stream, and later calls require
explicit reconnect while preserving the request sequence. [GCE socket controls](evidence/runtime-20261027/g8-agent-stream-invalidation-gce/README.md)
pass with retained ownership and primary state unchanged. The failed startup's
cause and recovery remain unproven.

The [CID-gated relay controls](evidence/runtime-20261028/g8-relay-peer-controls/README.md)
cover a new explicit native helper mode. A [bounded agent diagnostic](runtime/tools/mk-agent-capabilities-probe/README.md)
checks the local relay identity before one capabilities request, using a sealed
credential descriptor. The current shim factory also requires the declared
child CID, without wildcard fallback; deployment and a fresh Docker lifecycle
qualification remain required. These controls do not close the retained startup
failure or full G7/G8 gates.

The [actual retained-child probe](evidence/runtime-20261028/g8-retained-agent-capabilities-gce/README.md)
now receives the expected capabilities over the real MK transport, with primary
state and retained resource identities unchanged. This qualifies one bounded
agent response; Docker recovery, version-5 live doctor and normal cleanup remain
open. The child and its open storage export remain retained for diagnosis.

The [production relay factory qualification](evidence/runtime-20261028/g8-production-relay-gce/README.md)
now observes an actual wrong-peer refusal without bridge publication and a
matched capabilities reply for the declared CID. Startup, worker recovery and
replacement share this factory. The first failed positive-case fixture and its
correction are retained separately; complete Docker lifecycle qualification and
deployment of the shim/helper pair remain open.

[Production local-peer and replacement qualification](evidence/runtime-20261028/g8-production-peer-gce/README.md)
also passes on GCE: the shim verifies the owned relay's PID/UID/GID before
sending credentials and receives matching capabilities after replacing an
exited helper with a new PID. The authentication sequence is preserved.
Primary state and retained resource identities match before/after. This covers
the transport methods; a complete fresh Docker lifecycle remains required.

The [bounded retained-task observation](evidence/runtime-20261028/g8-retained-process-state-gce/README.md)
now reports guest `init` stopped at PID 616 with exit 137 while its agent answers.
This separates guest task state from the failed host namespace-holder identity;
the termination cause/time and child Docker engine health remain unproven.
At that observation the child and storage export remained active for diagnosis;
the later graceful-stop result below supersedes that physical state.

A [non-consuming output summary](evidence/runtime-20261028/g8-retained-stopped-output-gce/README.md)
also finds workload-begin and full-pressure markers in retained guest stdout,
despite zero output at the original host wait. Raw output stays out of the report.
This directs diagnosis toward output delivery/observation; markers do not
retroactively qualify ENOSPC, builds, live doctor or normal cleanup.

[Explicit private preservation](evidence/runtime-20261028/g8-private-bundle-gce/README.md)
now retains the bounded guest output window in a root-only artifact, with only
redacted metadata published. The [next-trial relay admission fix](evidence/runtime-20261028/g8-relay-trial-admission-gce/README.md)
also removes a legacy helper mismatch: both trial callers require the qualified
staged CID relay before lifecycle mutation.

The [exact-generation graceful stop](evidence/runtime-20261028/g8-exact-child-graceful-stop/README.md)
now parks the child: its kernel is `loaded`, the original NBD process is terminal,
and transport references are zero. Primary state and failed ledgers are unchanged.
The [native-close and filesystem proof](evidence/runtime-20261028/g8-retained-native-close/README.md)
then validates strict synced close, no backing-image aliases and clean read-only
e2fsck with an unchanged post-close hash. [Exact operator resource return](evidence/runtime-20261028/g8-retained-resource-release/README.md)
unloads/deletes the parked kernel, releases its pool and restores network rules,
links and routes. That phase returns all 16 CPUs with no children and leaves the candidate
transport loaded at zero references. Historical RUNNING/ACTIVE records
and the failed image remain unchanged and excluded from reuse. [Cold restoration](evidence/runtime-20261028/g8-retained-cold-restoration/README.md)
then restores the original transport after one guarded idle reboot and verifies
all 93 retained files / 12 images, private output and primary persistent Docker
inventories. Fresh Docker qualification remains required; normal cleanup of
this failed trial and full G7/G8 are not qualified. Direct child kernels require no nested virtualization and share host
trust after a child-kernel compromise.

The [subsequent user restart](evidence/runtime-20261028/g8-user-restart-28b94738/README.md)
also preserves all 93 files / 12 images, private output, failed ledgers and primary
persistent Docker inventories on boot `28b94738…`. The [packet-builder fix](evidence/runtime-20261028/g8-trial-packet-cid-relay/README.md)
now requires and includes the qualified CID relay alongside protected source
modes; missing/corrupt relay inputs refuse. Corrected packets are retained in a
protected persistent directory before launch. These are staged-test artifacts;
full G7/G8 and fresh live Docker/doctor acceptance remain separate gates.

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
