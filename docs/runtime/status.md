# Project and runtime status

This page summarizes the results proved by the retained experiment and runtime
evidence. Detailed procedures, architecture, learnings, and evidence contracts
remain authoritative for their respective areas.

## Latest G8 checkpoint (2026-10-28)

Current test-host state: the 0c50de9 combined-doctor trial failed before reaching
its live holds. Its exact child/export generation remains retained, with one
open native backing descriptor and the candidate transport still loaded.
Normal cleanup and cold restoration are **not** qualified for this attempt.
The related agent stream reuse bug is independently reproduced and fixed in
a4b11660; local/full-runtime race and isolated GCE socket controls pass. A
[bounded actual-child capabilities probe](../../evidence/runtime-20261028/g8-retained-agent-capabilities-gce/README.md)
now receives the expected response through a CID-pinned relay with the checked
local peer and sealed credential descriptor. Primary state and retained resource
identities match before/after. This qualifies one response, without resolving
the startup or qualifying Docker health, normal cleanup or version-5 live doctor.
The later [retained task-state observation](../../evidence/runtime-20261028/g8-retained-process-state-gce/README.md)
reports the agent's `init` as STOPPED, guest PID 616/exit 137, under matched
capabilities and exact relay identity. The host namespace-holder zombie is a
separate identity. Termination cause/time and the child Docker engine's health
remain unproven; no child/export cleanup or installed-runtime replacement occurs.
The last passing earlier cases below retain their scope. Preserve this failed
private generation and follow the escalation contract before another live trial.


The approved data disk is now 50 GB, with all 92 retained files and 11 old child
images preserved. The [actual static-candidate trial](../../evidence/runtime-20261027/g8-live-static-candidate/README.md)
passes child Docker ENOSPC recovery, repeated builds, primary/child engine and
image separation, sampled authenticated endpoint CHECK and normal cleanup.
[Cold restoration](../../evidence/runtime-20261027/g8-live-static-restoration/README.md)
passes after one graceful reboot, restoring the original transport with all
primary services active and retained file hashes unchanged. A committed
size-only immutable deployment update repairs the stale 30 GB startup binding.
Docker engine/images/containers/volumes match across boot; built-in network IDs
changed, and prior-boot network identity parity remains unqualified.
The later [live network/storage correlation trial](../../evidence/runtime-20261027/g8-live-network-doctor/README.md)
also passes network-doctor v4 against the actual child at both holds and after
removal, correlating exact ownership with the storage/native-process doctor.
Its startup console limit is recorded; complete diagnostic coverage stays open.
[Separate cold restoration](../../evidence/runtime-20261027/g8-network-doctor-cold-restoration/README.md)
and [the user's subsequent restart](../../evidence/runtime-20261027/g8-user-restart-3ae4/README.md)
are both verified without replacing historical receipts. Current boot 3ae4f0d8
has the original transport, all four services active and all 92 retained files
unchanged. The protected 50 GB binding remains in place.

Full G7/G8 remains **OPEN**. Host-server queue-error retry remains
injection-qualified, the earlier stall's cause remains unproven, and crash,
concurrency and other readiness/diagnostic requirements remain open. This uses
direct child kernels without nested virtualization, with shared host trust after
child-kernel compromise. Earlier entries below retain their checkpoint scope.

## Proven results

| Theme | Result | Details |
| --- | --- | --- |
| GCE bring-up | Primary plus two concurrent child kernels passed; resources returned cleanly | [GCE runbook](../guides/gce-lab-runbook.md) |
| DAXFS roots | DAXFS and Docker-derived roots passed; shared writable coherence failed | [DAXFS report](../experiments/daxfs/report.md) |
| Direct ext4 devices | Rejected because child and boot disks share one allocatable controller | [Direct ext4 experiment](../experiments/ext4-direct/README.md) |
| Mediated ext4 roots | Two isolated persistent child roots passed while the primary retained every controller | [Mediated ext4 report](../experiments/ext4-mediated/report.md) |
| Container runtime | G4-G6 passed on a disposable GCE host with deterministic private roots, primary-mediated CNI networking, and the supported Runtime v2 lifecycle through both `ctr` and Docker | [Final G6 manifest](../../evidence/runtime-20261010/g4-g6-final-2ee14c4/manifest-g6.json) |

## G4-G6 `ctr` and Docker feature matrix

`Passed` means the final G4-G6 qualification exercised the operation on the
common `d7c5301` runtime revision and retained machine-readable evidence.
`Partial` and `Not implemented` are not release claims. Docker commands require
`--runtime io.containerd.multikernel.v2 --network none` because the runtime,
rather than Docker's bridge, owns the child link.

| Feature | `ctr` command/surface | Docker command/surface | Status |
| --- | --- | --- | --- |
| Pull and inspect BusyBox OCI image | `ctr images pull`, `images list` | `docker pull`, `image inspect` | Passed |
| Combined foreground run | `ctr run` | `docker run` | Passed |
| Split create and start | `containers create`, `tasks start` | `docker create`, `docker start` | Passed |
| State inspection | `ctr tasks list` | `docker inspect` | Passed |
| Additional process | `ctr task exec` | `docker exec` | Passed |
| Stdout and stderr | foreground run and exec FIFOs | foreground run and exec | Passed; output streams while the process is live |
| Wait and nonzero exit | blocking `ctr run` (containerd 2.2.2 has no standalone `tasks wait`) | blocking `docker run`, `docker wait`, exit inspection | Passed |
| TERM and exit observation | `ctr tasks kill`, task state | `docker kill`, `docker wait` | Passed |
| KILL and exit 137 | `ctr tasks kill --signal SIGKILL` | `docker kill --signal KILL` | Passed in the earlier [MVP proof](learnings/04-g4-g6-mvp.md) |
| Delete, cleanup, and name reuse | `tasks rm`, `containers rm` | `docker rm`; same name reused | Passed twice from clean state |
| Concurrent isolated sandboxes | two live Runtime v2 tasks | two live Runtime v2 tasks | Passed with distinct child boot IDs |
| Private writable OCI root | exec read/write proof | exec read/write proof | Passed; mutations are sandbox-private, not snapshot persistence |
| Outbound DNS/HTTP | child `mkn0` through primary NAT | child `mkn0` through primary NAT | Passed |
| Cross-sandbox network isolation | sibling `/30` unreachable | sibling `/30` unreachable | Passed |
| `mkruntimed` restart | running task retained its boot ID | running container retained its boot ID | Passed concurrently |
| Client daemon restart | containerd restart with running task | Docker daemon restart with running container | Passed with workload continuity and cleanup |
| Forced shim death | supervisor reconstructs the worker and reconnects | same Runtime v2 recovery path | Passed for stream continuity, task reconnect, and bounded fallback reclaim |
| Process list | Task `Pids` reports validated guest init/exec PIDs | Docker metadata can consume it | Passed in the exhaustive Task v2 matrix |
| Stdin/attach and `CloseIO` | foreground stdin and `tasks attach` | foreground stdin and `docker attach` | Passed; FIFO bytes drain before guest EOF |
| Terminal and resize | `--tty`, `ResizePty` | `--tty`, resize | Passed, including deliberate post-start resize |
| Pause and resume | `tasks pause/resume` | `docker pause/unpause` | Passed in the exhaustive Task v2 matrix |
| Metrics/stats | Task `Stats` | `docker stats` | Passed in the exhaustive Task v2 matrix |
| Runtime resource update | Task `Update` | `docker update` | Not implemented |
| Checkpoint/restore | Task `Checkpoint` | Docker checkpoint | Not implemented |
| Full OCI controls | capabilities, seccomp, namespaces, mounts, hooks, rlimits, read-only root | equivalent Docker flags | Partial: capabilities, rlimits, hostname/path policy, read-only root, standard mounts, and materialized read-only directory/file binds are enforced; unsupported seccomp, hooks, writable/propagating mounts, and other controls fail closed |
| CNI `ADD`/`CHECK`/`DEL` | CNI 1.0 adapter | CNI 1.0 adapter | Passed with partial rollback, repetition, isolation, restart, and cleanup |

The repeatable test is
[`scripts/test-runtime-g4-g6-feature-matrix.sh`](../../scripts/test-runtime-g4-g6-feature-matrix.sh).
Final claims are recorded in the [G4](../../evidence/runtime-20261010/g4-g6-final-2ee14c4/manifest-g4.json),
[G5](../../evidence/runtime-20261010/g4-g6-final-2ee14c4/manifest-g5.json), and
[G6](../../evidence/runtime-20261010/g4-g6-final-2ee14c4/manifest-g6.json)
manifests. The immutable bundle index passes the independent evidence auditor.

The final qualification VM and its auto-delete boot disk were deleted. Only
the pre-existing 20 GiB `mk-mediated-storage-20260830` disk was intentionally
retained and remains billable.

## Safety boundary

**Nested virtualization is not allowed in the target compute instances.**
KVM/microVMs inside the instance are not an available containment plan. The
primary and all direct-boot children share one trust domain; separate untrusted
domains require separate compute instances, or a separately reviewed execution
model if they must share an instance. See the
[deployment and containment constraint](architecture.md#deployment-and-containment-constraint).

The [G7 boundary assessment](research/g7-security-boundary-assessment.md)
reproduces the pinned source audit and records fresh portable agent checks.
It identifies an architectural limitation after child-kernel compromise and
baseline bootstrap credential exposure and default-root access to a synthetic
child-bootstrap file outside its OCI chroot. The
[remediation rerun](../../evidence/runtime-20261012/g7-authority-remediation/README.md)
verifies private credential handoff, least-privilege root defaults, mandatory
no-new-privileges, selected devices, and fail-closed privilege requests in six
bounded cases. See the [authority policy](contracts/workload-authority.md) for
compatibility changes. Mediated-root/Docker requalification and remaining
authority review are open; full G7 and a security advantage over gVisor are
not established. Historical G4-G6 passes apply to their recorded source versions.
The [rb10 host/child checkpoint](../../evidence/runtime-20261012/g7-kernel-rb10-live/README.md)
now runs all twenty-four kernel overlays, with verified profile activation,
structural refusal/real rollback and preserved recovery artifacts. Its [full nineteen-feature compatibility suite and final resource audit](../../evidence/runtime-20261012/g7-kernel-rb10-matrix/README.md)
pass; gate acceptance remains open.

The [SEC-12 deployment checkpoint](../../evidence/runtime-20261012/g7-sec12-deployment-live/README.md)
activates the patched Go1.26.9 runtime and artifact-admission guards from
clean revision0c80131 on the disposable rb11 baseline. Exact deployment,
qualification and failure receipts are retained. Google-agent findings,
other dependency triage, provenance and host/child ABI review remain open;
this checkpoint does not close G7 or whole SEC-12.

The [corrected SEC-08 deployment](../../evidence/runtime-20261012/g7-secret-handling-deployment/README.md)
qualified revision 10107cc, including all three credential-handling fixes.
The live secret trace, installed-client control, all 19 ctr/Docker feature rows
and independent resource audit with all 19 counts at zero pass on the same rb11 baseline.
Complete audit identity/retention and remaining diagnostic secret paths stay open.

The [audit deployment checkpoint](../../evidence/runtime-20261012/g7-audit-deployment-live/README.md)
qualified revision4f95ead with completion reservations and the negotiated lifecycle
audit view. All19shared ctr/Docker features, live audit identity/secret checks,
UID65534 socket refusal and independent zero/no-pool inventory pass. The final
read-only client validated660records over two pages. The
[canceled-retry recovery fix](../../evidence/runtime-20261012/g7-cancel-retry-recovery/README.md)
has source/race/archive and unprivileged VM proof. Its
[597e877 activation](../../evidence/runtime-20261012/g7-cancel-runtime-checkpoint/README.md)
is now selected and passes live trace/refusal and independent idle checks.
The next full feature run was deferred after reproducing old-key cancellation
of a replacement generation. The
[generation fix](../../evidence/runtime-20261012/g7-cancel-generation-ownership/README.md)
passes four ownership/reopen cases, full race/vet/docs and UID1001 VM proof;
its [combined release](../../evidence/runtime-20261012/g7-cancel-combined-live/README.md)
is now selected as revision665f64e. The corrected live trace and UID65534
refusal pass, along with all 19 ctr/Docker features and independent 19-zero/no-pool
resource proof. The final read-only audit validates 850 records across two pages.
Both cancellation fixes are deployed and compatibility-qualified; the complete
live failure/ownership matrix and whole STRIDE acceptance remain open.
Whole SEC-06, SEC-08 and G7 remain open.

The runtime targets trusted, single-tenant nodes. Multikernel sibling kernels
do not currently provide Firecracker's KVM/EPT hardware isolation boundary.
Never assign the primary boot disk, NIC, or a shared controller to a child, and
never allocate APIC ID 0. See the [runtime architecture](architecture.md) for
the complete trust boundary and ownership rules.

## Guest image-build prototype

An [experimental builder guest](research/guest-image-builds.md) passed offline
multi-stage Dockerfile execution, cache reuse, OCI export/content verification,
and failed-RUN propagation using `mk-build . image.tar`. The client needs no
privileged/security flags and retains UID 0 with zero capabilities and NNP=1.
Trusted bootstrap prepares the builder namespace automatically before workloads
start; the daemon maps namespace root to guest UID/GID 1000 and enables NNP.
The [evidence](../../evidence/runtime-20261012/g7-guest-builds/README.md) includes
the ordinary rootless-image startup failure, corrected bootstrap, authority
checks and cleanup. Default Docker integration, registry/package networking,
mediated storage, persistent cache and builder-service security/lifecycle
qualification remain open. This does not change the hostile-child boundary.

The [Docker-in-Docker follow-up](research/docker-in-docker-guests.md) additionally
passed ordinary `docker build -t ...` and `docker run --rm ...` against a guest-local
rootless Docker Engine 29.9.0, including cache reuse, failed-build propagation,
saved-image content verification and nested NNP/seccomp observations. Its approved
bootstrap prepares namespaces, VFS storage and guest bridge modules automatically;
the user supplies no privileged/capability/security flags or primary Docker socket.
This remains an offline dedicated-guest prototype; production integration,
egress/pulls/publishing, persistence and service/security qualification are open.

The [online Python follow-up](research/docker-online-python-guest.md) passed in
fresh guests: Docker Hub Python base pull, HTTPS PyPI installation of Requests
and four dependencies, ordinary Docker build/run, verified runtime HTTPS 200,
cache reuse, package hash comparison, client/daemon/nested authority observations
and host/cloud cleanup. Automatic bootstrap supplies slirp4netns and guest NAT
modules; the user needs no privilege flags. This uses a diagnostic static primary
TUN/NAT path and the unchanged authenticated agent packet interface. Production
profile/CNI/mediated storage integration, persistence/restart/concurrency/published
ports/IPv6/resource enforcement and independent security review remain open.

The [production-profile Docker build checkpoint](../../evidence/runtime-20261022/g8-production-docker-build/README.md)
uses the production builder and supervised agent path on a mediated rb11 direct
child. Offline multi-stage builds, cache repeat, built-image execution, deliberate
build failure and independent layer/content verification pass. The same invocation
passes authenticated storage shutdown, synced NBD export, offline filesystem
checking and full disposable child/resource cleanup while watched primary files
and service PIDs remain unchanged. Host Docker/Runtime v2 selection, recovery,
concurrency and adversarial integration remain open; no complete G7/G8 gate closes.

The later [host Docker lifecycle checkpoint](../../evidence/runtime-20261022/g8-host-docker-lifecycle/README.md)
passes host Docker runtime selection, the offline child build matrix and teardown
with independent daemon state. Raw firewall rules/policies, links and routes
return to their initial identity; child/pool/rootfs/export/network ownership is
released. A separate workload checkpoint precedes teardown. This advances the
previous direct credential-load fixture; recovery, concurrency, adversarial and
complete G7/G8 acceptance remain open.

The [graceful runtime-restart proof](../../evidence/runtime-20261022/g8-host-docker-runtime-recovery/README.md)
subsequently keeps the Docker child live across one private runtime process
restart. Child boot ID, local engine ID, saved-image digest and ownership
generation remain identical; final teardown and raw primary/network comparisons
pass. Crash/power loss, other service restarts, service privilege bounds and
complete G7/G8 acceptance remain open.

The [service-policy checkpoint](../../evidence/runtime-20261022/g8-host-docker-service-policy/README.md)
repeats build, graceful runtime restart and teardown with the documented runtime
and network capability/NNP/ambient settings enforced on private candidate daemons.
Actual process masks match the retained source units, including the replacement
runtime process. This closes the bounded compatibility check; other service
restrictions, concurrency, crash/pressure/adversarial cases and full G7/G8 remain open.

The [two-child build checkpoint](../../evidence/runtime-20261022/g8-concurrent-build-checkpoint/README.md)
records real overlapping BuildKit RUN steps, separate boot/engine/canary/catalog
and backing-file identities, completed offline build matrices, survivor continuity
and final cleanup. This used the old native NBD proxy. The earlier missing export
is retained as a failure control; one successful run does not resolve it.
The [proxy regression proof](../../evidence/runtime-20261022/g8-native-stream-proxy/README.md)
reproduces a blocking forwarding deadlock and verifies independent bounded queues
with byte integrity, half-close and broken-peer controls. Fresh GCE qualification
of that candidate is separate. Full concurrent reliability and G7/G8 stay open;
direct child kernels require no nested virtualization and share host trust.

The [candidate proxy run](../../evidence/runtime-20261022/g8-candidate-release-failure/README.md)
passes both build/export matrices but fails a sibling-release exec deadline.
Containerd reports an outstanding exec during init deletion. One stopped/loaded
child and ACTIVE export remain held; a single recovery attempt refuses unsafe
adoption after the test unit loses the server. This is a retained failure control,
not survivor/cleanup/recovery acceptance. The timeout-output retention fix is
`266a919`; investigation and explicit resource recovery remain outstanding.

The [exact-generation quarantine proof](../../evidence/runtime-20261023/g8-failed-generation-quarantine/README.md)
subsequently returns the stopped child's CPUs and pool while retaining its disk
byte-for-byte and preserving the failed ledgers. The user then restarted the
primary; all installed services become active and the retained bytes/ledgers are
unchanged. This does not claim automatic storage release or prior-boot network
cleanup. New fixtures use distinct protected work and storage directories.
The normal concurrency fixture now releases through an authenticated host Docker
signal (`eb40dc7`, `618ece2`), avoiding an exec whose completion races init exit.
`b23a6a9` refuses fixture resource effects unless the executing systemd invocation
has a private mount namespace and `KillMode=process`, as required to retain exact
storage helpers after daemon exit. The outstanding exec/exit reliability case
and full G7/G8 remain open.

The later [primary bridge failure control](../../evidence/runtime-20261023/g8-primary-bridge-failure/README.md)
passes corrected-proxy concurrent builds, signal release, survivor continuity,
synced export closure and offline checks, then fails the strict final network
comparison: the disposable host dockerd removed `docker0` and its route.
`40baa83` gives that daemon a separate network namespace while retaining the
primary namespace for containerd/Kerf/mknetd. The failed evidence is preserved;
a fresh pass of the original network comparison is required.

The [namespace-isolated candidate qualification](../../evidence/runtime-20261023/g8-concurrent-netns/README.md)
now passes the original raw network comparison with primary docker0 preserved,
plus overlapping builds, separate engines/storage, survivor continuity and final
resource release. Both raw native-server logs bind the expected export generations
and CIDs, end synced, and match durable counters and clean offline-check digests.
The final invocation-bound receipt preserves watched installed hashes/PIDs.
This closes that bounded checkpoint; it does not close repeated concurrency,
SIGKILL/power-loss/exec-exit recovery, pressure or full G7/G8. `0f367e6` adds an
explicit private-runtime SIGKILL fixture for separate live qualification.

The [runtime SIGKILL checkpoint](../../evidence/runtime-20261023/g8-runtime-sigkill/README.md)
now records an intentional private-daemon exit status -9 before replacement
startup. Exact Unix peer credentials prove the replacement owns its listener;
local stale/wrong-owner/dead-process controls are [retained separately](../../evidence/runtime-20261023/g8-socket-readiness/README.md).
Child boot/engine/archive/ownership remain unchanged across adoption. A complete
fresh build matrix then succeeds while the original archive remains intact.
Declared service bounds, primary file/PID/network identity, synced server closure,
offline checking and child/pool/rootfs/network release all pass. The unit cgroup
subsequently disappears. This closes one post-build control-plane SIGKILL case,
not crashes during allocation/build/teardown, primary power loss, other service
crashes, pressure or full G7/G8.

The [executing-RUN SIGKILL proof](../../evidence/runtime-20261023/g8-runtime-active-run/README.md)
subsequently observes real BEGIN without END before kill, after confirmed exit
-9 and after adoption. Timestamp-bound raw snapshots place the replacement about
0.25 seconds after the before-kill sample inside a RUN deliberately held for
45 seconds. The original build matrix completes, its first archive is preserved
while fresh builds pass, and child/engine/owner identity and final primary/resource
checks pass. This closes that bounded in-flight RUN case. It does not prove
crash safety at arbitrary build I/O/resource-mutation points, under disk/memory/log
pressure, across every service, or for host power loss and full G7/G8.

The [bounded storage-pressure attempt](../../evidence/runtime-20261023/g8-pressure-io-failure/README.md)
(`09ac776`) fails before ENOSPC: guest dd reports I/O error after 417 MiB,
sync and cleanup also fail, and no full-space hold/build pass occurs. The native
log reports a synced close after about 4 MiB of writes and no request refusal;
it does not establish the client/kernel cause. STOPPED/QUIESCING ownership is
retained. Local [report refusal controls](../../evidence/runtime-20261023/g8-pressure-controls/README.md)
pass independently and do not qualify physical pressure.

The [operator quarantine and read-only diagnosis](../../evidence/runtime-20261023/g8-pressure-quarantine/README.md)
then returns CPU/pool/network resources while preserving disk bytes and all
failed ledgers. e2fsck -fn exits 4 with uncorrected bitmap/count/orphan errors;
the disk hash stays identical. Watched installed hashes/PIDs and raw network
identity match baseline. This confirms the observed failed-check release hold,
not successful storage recovery. `6a191ef` retains write/sync exits before further
probe checks. Late client/kernel diagnostics must be retained before teardown
before reproducing and fixing the cause. Pressure and full G7/G8 remain open.

The [repeat pressure test with console capture](../../evidence/runtime-20261023/g8-pressure-console-failure/README.md)
retains a bounded exact-instance transcript: NBD send -104 and receive -32
precede ext4 journal abort. It failed before ENOSPC, with write exit 1 and sync
exit 127. This establishes event ordering, not the transport failure cause.
The pressure gate and full G7/G8 remain open. Native client terminal diagnostics
now reach the protected child console after successful peer validation; they
do not change forwarding limits or error policy.

The [client-instrumented pressure run](../../evidence/runtime-20261023/g8-pressure-client-timeout/README.md)
records an NBD connection timeout before proxy local-TCP EOF, ioctl return 0,
and journal abort. Docker cleanup fails with a zombie PID. The user's next
[external restart](../../evidence/runtime-20261024/g8-pressure-external-restart/README.md)
returns CPU/pool resources; failed ledgers and disk remain. This is not automatic
recovery. [Server timing controls](../../evidence/runtime-20261024/g8-server-io-controls/README.md)
pass with 17 forced slow writes, bounded details, errno preservation, full required
checks and unchanged forwarding/time-budget policy. Physical pressure and full
G7/G8 remain open. Tests continue with direct children and no nested virtualization.

The [server-timed physical pressure test](../../evidence/runtime-20261024/g8-server-io-pressure-failure/README.md)
fails its ENOSPC deadline and Docker cleanup. Its first 16 slow details are completed
WRITE payload receives of 0.318–2.690 seconds; 20 further details are suppressed,
so those details cannot rule out later backing latency. The
[exact operator cleanup proof](../../evidence/runtime-20261024/g8-server-io-quarantine/README.md)
returns CPUs/pool, preserves the disk and failed ledgers, and restores raw primary
rules/links/routes. Private network adoption first refuses a missing veth; exact
endpoint deletion succeeds from a separate protected recovery copy.
[Aggregate timing controls](../../evidence/runtime-20261024/g8-phase-summary-controls/README.md)
now retain six phase maxima/counts even after detail suppression and pass required
checks. Bootstrap queue observation is read-only and awaits a live result. The
rejected global queue-limit proposal was not applied. No request/queue/timeout
limit changed. Pressure, automatic cleanup and full G7/G8 remain open.

The [live queue/phase baseline](../../evidence/runtime-20261024/g8-queue-phase-pressure-failure/README.md)
observes software queue 256, 4 MiB requests and 15 s configured timeout. Proxy
ENOSPC comes from VSOCK send before journal abort, not the required filesystem
exhaustion. Complete phase maxima put WRITE payload receive at 6.23 s and backing
read/write/sync below 14 ms in that run. [Bounded retry controls](../../evidence/runtime-20261024/g8-ring-full-retry-controls/README.md)
pass for exact duplex bytes, paced timeout and endpoint-specific refusal.
A [readiness/release module candidate](../../evidence/runtime-20261024/g8-vsock-release-module-build/README.md)
removes linear queue walks and retires core socket-table refs on close. Local
actual-function checks and isolated compilation pass. An initial guarded trial
[refuses four old orphan refs](../../evidence/runtime-20261024/g8-vsock-orphan-guard/README.md)
before any module load. The [clean boot](../../evidence/runtime-20261024/g8-clean-boot-observation/README.md)
provides an empty baseline and preserves failed disk/ledger hashes. Candidate
pressure, live reference cleanup and full G7/G8 are still pending. No force unload,
queue-size change, request-limit change or deadline relaxation is used.

The [combined transport live trial](../../evidence/runtime-20261024/g8-coarse-fill-refusal/README.md)
now reaches filesystem ENOSPC without native I/O errors and retires socket-table
references on the same boot (zero refs, empty VSOCK table). It fails the unchanged
zero-available-space condition because coarse writes leave 496 KiB. The stopped
child/QUIESCING export require [operator quarantine](../../evidence/runtime-20261024/g8-coarse-fill-quarantine/README.md);
that returns CPU/pool/network resources while retaining disk/ledgers, and read-only
e2fsck returns 0 with unchanged image bytes. This is narrow transport progress,
not normal cleanup or pressure-gate closure. Probe commits 79b478a/ee87af0 add a
synchronized, bounded 4 KiB tail fill; live recovery qualification remains pending.

A [same-boot ee87af0 repeat](../../evidence/runtime-20261024/g8-warm-start-failure/README.md)
reaches storage bootstrap and agent start but never emits probe output. It fails
the existing 300-second hold-point deadline, and Docker cleanup reports a zombie
task. A live owned NBD helper remains; these references are not orphan proof.
[Exact operator recovery](../../evidence/runtime-20261024/g8-warm-start-quarantine/README.md)
gracefully halts that child, terminates its pinned helper, proves zero references
and an empty VSOCK table, and returns CPU/pool/network resources while preserving
the failed disk/ledgers. Read-only e2fsck passes.
[Bootstrap diagnostic controls](../../evidence/runtime-20261024/g8-bootstrap-marker-controls/README.md)
retain specific storage/agent startup markers with existing redaction and bounds.
The startup cause, tail-fill live qualification, normal cleanup, repeated
lifecycle and complete G7/G8 remain OPEN.

[Cold restoration](../../evidence/runtime-20261024/g8-cold-module-restoration/README.md)
is now verified on boot 77840555-d61f-4ad7-8f77-71ddeed4c407: original transport
srcversion 65C55BB38C669FF94A4B772, zero references, empty VSOCK, all CPUs online,
no child/pool and four active installed services. Original module/source and
installed file hashes, both failed image hashes and all eight failed ledger hashes
are unchanged. Reboot followed already-proved operator cleanup; it does not count
as automatic cleanup or pressure acceptance. The candidate was not installed
persistently.

The [981bc2a clean-boot pressure run](../../evidence/runtime-20261024/g8-pressure-recovery-cleanup-failure/README.md)
passes the strict zero-space hold and recovery workload: before/restored capacity
1,541,620 KiB, full capacity 0 KiB, same engine, complete build matrix.
[Saved-image inspection](../../evidence/runtime-20261024/g8-pressure-saved-image/README.md)
verifies both layer digests, expected output and exact BusyBox input without changing
the backing image. Overall cleanup fails and leaves STOPPED/QUIESCING ownership.
The exact native log reproduces the old parser's rejection of timing records.
[f2d8439 and its controls](../../evidence/runtime-20261024/g8-native-close-timing-controls/README.md)
validate the complete bounded timing grammar, retaining strict lifecycle/counter
validation; storage, daemon and shim race tests and required checks pass.
[Operator quarantine](../../evidence/runtime-20261024/g8-pressure-recovery-quarantine/README.md)
returns resources, preserves failed ledgers/image and passes read-only e2fsck.
Fixed runtime binaries are built from the pinned archive; live automatic cleanup
acceptance, repeated lifecycle/startup reliability and full G7/G8 remain OPEN.

The [f2d8439 fixed-runtime pressure case](../../evidence/runtime-20261024/g8-pressure-recovery-qualified/README.md)
is independently qualified on boot 024920df-59b6-43cb-8100-427ca1b7c792. Exact
zero-space ENOSPC, restored capacity, same-engine builds, matching native counters,
RELEASED storage with e2fsck-clean digest, empty rootfs/network/lifecycle inventory,
CPU/pool/child cleanup, unchanged primary state and zero VSOCK references all pass
before reboot. This closes the bounded pressure/recovery/automatic-cleanup case,
not full G7/G8. The [previous experiment's cold restoration](../../evidence/runtime-20261024/g8-pressure-cold-restoration/README.md)
is retained separately; the new candidate remains loaded pending a guarded
same-boot repeat and final idle cold restoration.

The [guarded same-boot repeat](../../evidence/runtime-20261024/g8-pressure-warm-repeat/README.md)
now also passes after normal automatic cleanup, with the same component pins,
CPUs/CID and primary boot, fresh empty engine/filesystem generations, no transport
reload, exact zero-space/recovery/build checks, clean release, empty resources
and unchanged primary state. Committed driver 19e6c45 and kernel registration
journal bind that sequence to the prior accepted receipt. This qualifies one
normal repeat; arbitrary startup after failed recovery, broader soak/concurrency/
crash coverage and full G7/G8 remain OPEN. Final idle cold restoration follows
after these independently observed cleanup results.

[Final series restoration](../../evidence/runtime-20261024/g8-qualified-series-restoration/README.md)
is verified on boot b333fa91-8298-4799-9639-d31c85417a3d: original transport
srcversion 65C55BB38C669FF94A4B772, zero references/sockets, no children/pool, all
CPUs online and four active installed services. Installed files, original kernel
source/module, three failed images and twelve failed ledgers are unchanged.
Twenty successful-trial receipt/archive/component hashes match the preboot
evidence. Private fixed binaries remain staged; no installed runtime binary or
module was replaced persistently. Both successful cleanup results precede this
reboot, which is not counted as cleanup. Full G7/G8 remains OPEN.

The [a262378 primary Docker boundary case](../../evidence/runtime-20261024/g8-primary-docker-boundary/README.md)
now qualifies an in-flight check: the guest holds its built image before pruning,
and the observer confirms its ID is absent from the installed primary engine.
Four bounded snapshots preserve engine identity and image/tag/digest, container,
volume and network inventory fingerprints before/full/image-held/after. Only
counts and fingerprints are published. Exact ENOSPC recovery, the build matrix,
clean release and complete automatic cleanup also pass with unchanged primary
files/PIDs/network. [Local controls and preflight](../../evidence/runtime-20261024/g8-primary-docker-controls/README.md)
cover retag drift, image presence, malformed metadata and bounded observation.
The observer/release share the existing build budget. This does not hash volume
contents or qualify all G7/G8 requirements.

[Original-module restoration](../../evidence/runtime-20261024/g8-primary-boundary-restoration/README.md)
is now verified on boot 00ac7aff-a963-4e30-a085-6811e83f0e12, with installed
bytes, three failed disks, twelve failed ledgers and twelve successful-case
artifact hashes preserved. Normal cleanup preceded the reboot. Postboot engine,
image, container and volume fingerprints match; the network-ID fingerprint
changed with its count still three. Its cause is unqualified. The live proof
compares snapshots within its original boot only. Code a14d789 now binds future
snapshots to boot identity and rejects cross-boot or legacy-schema comparisons;
[six controls and a read-only GCE check](../../evidence/runtime-20261024/g8-primary-epoch-controls/README.md)
verify this observer guard. Full G7/G8 remains OPEN.

Code da0adca adds `mkruntimed --doctor`, an early read-only path that compares
negotiated durable ownership and kernel instance names without opening state or
invoking Kerf. It reports missing/released/unrecorded instances, CPU/CID conflicts
and intermediate states; boot/sequence/generation/inventory changes refuse a
consistent result. [Race tests and GCE controls](../../evidence/runtime-20261025/g8-runtime-doctor/README.md)
pass, with exact private binary provenance and unchanged primary bytes/PIDs.
The installed daemon itself remains incompatible: Capabilities advertises only
`lifecycle-audit-v1`, and InspectOwnership returns `UNSUPPORTED`. Accordingly the
new client reports `inconclusive`, not healthy. Six private RPC fixtures qualify
the comparison/refusal behavior separately. A compatible daemon qualification,
kernel incarnation, complete storage/network ownership, pool/service readiness
and diagnostic collection remain required; full G7/G8 stays OPEN.

[Compatible-daemon inspection](../../evidence/runtime-20261025/g8-compatible-daemon-doctor/README.md)
now passes with source 45e784f and private production daemon state on the real
GCE kernel. Three reports are identical and consistent with the independent
ownership view and empty instance inventory. Private state is unchanged by the
reads; the daemon exits zero and removes its socket. Primary hashes, four active
service PIDs and idle kernel resources match before/after, without reboot or
transport reload. The installed daemon remains unchanged and incompatible; its
new diagnostic is `OWNERSHIP_UNSUPPORTED`. Failed observations retain the initial
boot ID and explicitly distinguish unconfirmed boot stability. This qualifies
idle presence inspection only: active-child/build inspection, live incarnation,
storage/network ownership and full readiness/diagnostics remain open.

The [8e97313 live doctor case](../../evidence/runtime-20261025/g8-live-child-doctor/README.md)
now qualifies presence inspection during the real child Docker pressure flow.
Parent 098a22b418644b579a864dff002319ee and child
942b78625fad436db0ae643fa39ed39f succeed on boot
00ac7aff-a963-4e30-a085-6811e83f0e12. The doctor reports the same RUNNING generation,
CID and exact kernel name during ENOSPC and the held built image; after removal
it reports no recorded sandbox or instance. Source/build hashes and all three
report hashes are retained. Four primary Docker inventory snapshots, installed
files, service PIDs and raw network comparisons match. Native zero-error close,
clean offline checking and complete resource return qualify normal cleanup.
The 1 MiB console collector saturated early (`output-limit`) and did not retain
late quiesce/disconnect markers; that diagnostic coverage remains open and is
not used as cleanup proof.
[Separate cold restoration](../../evidence/runtime-20261025/g8-live-doctor-restoration/README.md)
is verified on boot 6ff26ad7-fa58-4e40-a861-5fa6441661ef: original transport SRC65,
zero references/sockets/children, no pool, all 16 CPUs online and four active
installed services. Ten installed/original/candidate hashes, three quarantined
images and twelve failed ledgers remain unchanged; fifteen successful-case
artifact hashes, including all three doctor reports, match the preboot evidence.
Full health/ownership/diagnostic requirements and G7/G8 remain OPEN.

The [8a8a555 console-window qualification](../../evidence/runtime-20261025/g8-console-window-qualified/README.md)
now retains late diagnostics by selecting a fresh connection during the built
image hold, before releasing USR2. The preceding efcd79b attempt selected after
task exit and failed with ENOENT; its [failure](../../evidence/runtime-20261025/g8-console-window-late-attachment-failure/README.md)
and [idle restoration](../../evidence/runtime-20261025/g8-console-window-failure-restoration/README.md)
remain separate. The fix and nine-test lifetime/budget controls are committed.
Parent e7b6cb35d57f4ad78d18eb32f31c5b10 and child
fc8f5eb7dce44d07ab8c1645f99d2dae pass on boot
fa0ebb50-036e-44e7-a7dd-b06812f9ef23. Quiesce and disconnect markers occur once
each in the 370-byte teardown segment and zero times in the 278,945-byte startup
segment. The total cap stays 1 MiB; coverage is explicitly discontinuous and
this run did not reach either cap. Pipe tests cover flood bounds separately.
ENOSPC recovery, build matrix, three doctor reports, primary inventories/files/
PIDs/raw network parity, synced zero-error close, clean filesystem checking and
full normal resource return pass. Pinned 8e97313 runtime binaries are reused
with identical retained Git runtime-tree identities.
[Independent cold restoration](../../evidence/runtime-20261025/g8-console-window-restoration/README.md)
is verified on boot d27905ea-c79e-46ca-95a7-27bfc78a2f16, original SRC65, zero
references/sockets/children, no pool, all CPUs online and four active services.
Ten installed/original/candidate hashes, three quarantined images and twelve
ledgers are unchanged; eighteen successful-case hashes, including console
metadata/filtered events and phase-marker metadata, match the preboot evidence.
Full G7/G8 and complete diagnostic bundles remain OPEN.

Code 551f627 adds negotiated `storage-ownership-v1` / InspectStorageOwnership and
`mkruntimed --inspect-storage`. Its [metadata contract](contracts/storage-ownership.md)
excludes backing paths, image names/source digests, credentials, labels and raw
errors. It checks the 256-record limit before projection and uses a nonblocking
store lock: busy persistence yields retryable UNAVAILABLE, never a partial
inventory. Invalid/ambiguous claims refuse; no backend or service-operation lock
is used. Recorded generations, CID, quota/inode/port identity and validated close
check/counters remain visible to an authorized operator.
[Race tests and GCE proof](../../evidence/runtime-20261025/g8-storage-ownership/README.md)
pass, including three empty reads and three identical reads of a private copy of
the real 8a8a555 released export. Invocation 53a43d53767044bc8b78e992333133c2 exits
successfully on boot d27905ea-c79e-46ca-95a7-27bfc78a2f16. Both private daemons
exit zero and remove their sockets; original/copy state, ten primary hashes,
four service PIDs and idle kernel observations are unchanged. No child/export
workload, module reload or reboot is involved. Legacy installed services refuse
the view and remain unchanged. Active/intermediate states are unit fixtures;
live lifecycle/native correlation, full health and retention remain OPEN, as do
complete G7/G8.

Code 2a6f327 adds negotiated resource-ownership-v1 and doctor report version 2.
The [combined contract](contracts/resource-ownership.md) carries explicit
lifecycle storage expectations and opaque complete-descriptor fingerprints,
without projecting private paths, image names/source digests, labels or tokens.
Busy persistence fails promptly; lifecycle projections bracket storage, and
repeated combined views must match around kernel/boot observations. Missing
required, orphan live, generation/CID/export/descriptor and release mismatches
are findings. Ordinary storage-free sandboxes and historical released leases
are supported; intermediate states require rechecking. [Controls](../../evidence/runtime-20261025/g8-resource-correlation-controls/README.md)
and full suites pass. The [live GCE case](../../evidence/runtime-20261025/g8-live-resource-correlation/README.md)
passes parent 4842a2afa9224ddaa854ed1d9fffc308 / child
9bf088ba8bff47c0967739068e6b7536 on boot d27905ea-c79e-46ca-95a7-27bfc78a2f16.
ENOSPC/image-hold reports match the expected export generation, CID, descriptor
and backing metadata; removal yields empty lifecycle/kernel inventories and
the same released export with validated close counters. Primary inventories,
files, PIDs and raw network parity plus complete normal cleanup also pass.
Native process/file/socket health remains explicitly unobserved.
[Separate restoration](../../evidence/runtime-20261025/g8-resource-correlation-restoration/README.md)
passes on boot 96cb44a6-de0a-4c0f-af24-888f1b256c11: original SRC65, zero
references/sockets/children, no pool, all CPUs online and four active installed
services. Ten installed/original/candidate hashes, three quarantined images and
twelve ledgers are unchanged; eighteen successful-case artifact hashes match.
Reboot is not counted as normal cleanup. Full G7/G8 remains OPEN.

Code b68871a adds an internal read-only native process inspector, distinct from
recovery's mutating observer. Control bf311ac demonstrates preservation of a
stale record that recovery removes; fixture correction 0dde6da provisions fully
allocated images after formatting. [GCE evidence](../../evidence/runtime-20261025/g8-native-process-inspection/README.md)
retains both the initial allocation failure and four passing controls under
invocation bee21d4ecff240019177a08f2d1b69bd, with real pidfd/proc, inherited
backing-fd and executable identity checks. Ten watched hashes, installed service
PIDs, original transport and idle CPU/child/pool state match before and after;
no fixture process remains. Local race, complete Go and required suites pass.
This qualifies an internal primitive using a synthetic server, not an active
child/native VSOCK session. Doctor integration is qualified below; full native health remains
OPEN. Direct children need no nested virtualization and share the host trust
domain; complete G7/G8 remains OPEN.

Code 4f56e5d negotiates native-resource-identity-v1; e235fae makes --doctor version
3 require this complete view without legacy fallback. Repeated exact process
identity observations now supplement durable resource correlation. Missing active
records/processes are degraded; unavailable identity is inconclusive; released
history is explicitly not probed. [Controls](../../evidence/runtime-20261025/g8-native-doctor-controls/README.md)
pass focused race, complete Go, required repository and strict Python checkpoints.
The [intermediate GCE checkpoint](../../evidence/runtime-20261025/g8-native-identity-full-checkpoint/README.md)
was retained separately from final cleanup. The
[complete live case](../../evidence/runtime-20261025/g8-live-native-resource-identity/README.md)
passes under parent c118fa276f914f76950ead72e37769f9 / child
433cd01b4b0641299eefec7794eb45a6 on boot 96cb44a6-de0a-4c0f-af24-888f1b256c11.
Native identity matches at ENOSPC and held child image; metadata and descriptor
fingerprints agree. Child-local builds resume, the image remains absent from
primary Docker, four primary inventories agree, and normal cleanup is proven
before any reboot by native synced close, clean e2fsck, returned resources and
unchanged installed primary/network state. Native socket/session/content health,
further crash/concurrency cases and full G7/G8 remain OPEN. Direct children use no
nested virtualization and retain the shared host trust boundary.

[Native identity restoration](../../evidence/runtime-20261025/g8-native-resource-restoration/README.md)
passes on boot 28262509-6abe-4559-9c25-66d47eb18a86: original SRC65, zero module
references/sockets/children, no pool, all CPUs online and four active installed
services. Ten installed/original/candidate hashes and three quarantined images
with twelve ledgers match. Eighteen successful-case hashes sampled immediately
before reboot match the postboot sample and committed evidence. Reboot is not
counted as normal cleanup; full G7/G8 remains OPEN.

Code aac4a50 adds a guarded private record-fault helper; a9808ae selects its
bounded live child Docker scenario. [Controls](../../evidence/runtime-20261025/g8-native-record-loss-controls/README.md)
pass six real-process safety tests, six strict checkpoint tests and the required
suite. The [GCE case](../../evidence/runtime-20261025/g8-live-native-record-loss/README.md)
passes parent 3bd282e4ccc14a6fae99a5920c919975 / child
14fc583afd0f43e098549ba19f8fef98 on boot 28262509-6abe-4559-9c25-66d47eb18a86.
Two exact missing-record findings preserve recorded claims and five private
ledger/journal hashes. The same child engine inspects its image and runs/removes
a fresh inner container while metadata is hidden. The original record bytes
and inode are restored, doctor returns to its pre-fault consistent report,
and normal cleanup independently passes. Five primary Docker inventories,
installed hashes/PIDs and raw network parity agree. No held-record leftovers
remain. This qualifies metadata-loss detection/continuity, not native process
death or power-loss recovery. Full G7/G8 remains OPEN; direct children use no
nested virtualization and share the host trust domain.

[Record-loss restoration](../../evidence/runtime-20261025/g8-native-record-loss-restoration/README.md)
passes on boot e448e676-008f-47c5-a9d2-5cf5d6b1c0dc: original SRC65, zero
references/sockets/children, no pool, all CPUs online and four active installed
services. Ten watched hashes and three quarantined images with twelve ledgers
match. Twenty-six successful-case hashes sampled immediately before reboot
match postboot samples and committed evidence. Reboot is not normal cleanup.
Full G7/G8 remains OPEN.

Code f07e0cb prevents private-file open from waiting for an unpaired FIFO before
type validation. The [red/green controls](../../evidence/runtime-20261025/g8-fifo-open-controls/README.md)
reproduce seven old waits with the identical eight-method subprocess test;
all eight fixed controls, safefile/storage race, complete Go and required suites
pass. Eight guarded fault-helper and seven strict checkpoint tests also pass.
Build/source 4f3a36e adds the selected FIFO scenario. The
[live GCE proof](../../evidence/runtime-20261025/g8-live-fifo-record/README.md)
passes parent 148ac951f9854f17bc3bf34b463aabe5 / child
12de46bec3734e2ab0518cb0fa1b53cc on boot e448e676-008f-47c5-a9d2-5cf5d6b1c0dc.
Two exact inconclusive native-identity findings return while an owned unpaired
FIFO occupies the active record name, with no writer introduced. Recorded claims
and five private ledger/journal hashes remain unchanged; the same child engine
runs/removes a fresh inner container. The known FIFO is removed, original record
bytes/inode restored, doctor returns to consistent, and normal cleanup passes.
Five primary Docker inventories and installed hashes/PIDs/network parity agree.
This closes FIFO rendezvous refusal for these paths, not arbitrary filesystem/
device stalls, native process death or full session/content health. Complete
G7/G8 remains OPEN; direct children use no nested virtualization and share the
host trust domain after child-kernel compromise.

[FIFO-case restoration](../../evidence/runtime-20261025/g8-fifo-record-restoration/README.md)
passes on boot 00e5da43-7868-403b-b0f4-04bfcf8b216e: original SRC65, zero
references/sockets/children, no pool, all CPUs online and four active installed
services. Ten watched hashes and three quarantined images with twelve ledgers
match. Twenty-six successful-case hashes sampled immediately before reboot
match postboot samples and committed evidence. Reboot is not normal cleanup;
full G7/G8 remains OPEN.

The [actual native export SIGKILL proof](../../evidence/runtime-20261026/g8-live-native-export-crash/README.md)
now qualifies one post-build failure-retention case: two read-only doctor reports
identify the absent exact process, ownership stays fenced, and scoped deletion
returns nonretryable `OPERATOR_ACTION`. The image and original ledgers remain
retained with no authenticated CLOSED record or invented counters. The
[operator quarantine](../../evidence/runtime-20261026/g8-native-export-crash-quarantine/README.md)
returns child resources and preserves that failed image; read-only e2fsck exits 4.
This is not automatic recovery, successful normal cleanup, or storage release.
The test uses direct child kernels on GCE without nested virtualization. Shared
host trust after child-kernel compromise and full G7/G8 acceptance remain open.

The [cold restoration proof](../../evidence/runtime-20261026/g8-native-export-crash-restoration/README.md)
confirms the original transport, ten unchanged primary/module files, four retained
quarantined images, sixteen ledgers and four journals after graceful idle reboot.
All primary services are active, sixteen CPUs are online and no child or pool
remains. Reboot is not counted as normal cleanup or automatic crash recovery.

The [preparation-history controls](../../evidence/runtime-20261026/g8-preparation-history-controls/README.md)
reproduce and fix unsafe absent PREPARING restart when attachment history is
missing, and loss of dead-process records during missing-log observation.
Commits `82e1983` and `a957cef` require exact pre-acceptance history and retain
unknown state for operator action. Six actual SIGKILL cases pass locally and as
unprivileged host-process fixtures on GCE. This is not a new live child Docker
recovery qualification; full G7/G8 remains open.

The [ordered endpoint firewall proof](../../evidence/runtime-20261026/g8-ordered-endpoint-firewall/README.md)
reproduces required-rule-presence checks accepting policy bypasses. Code
`9aa3497` now compares the complete ordered private endpoint chain. Real iptables
ACCEPT and RETURN injections in an ephemeral GCE network namespace retain every
required rule yet are refused; removing each restores acceptance. The fixture
refuses the primary namespace, and primary rule identities/files/PIDs remain
unchanged. Global INPUT/FORWARD precedence, atomic firewall attestation and live
child Docker requalification remain open; this closes only private-chain drift
refusal. Full G7/G8 remains open under the direct-child, no-nested-virtualization
constraint and shared host trust boundary.

The [ingress guard precedence proof](../../evidence/runtime-20261026/g8-ingress-guard-precedence/README.md)
now qualifies sampled IPv4/IPv6 INPUT/FORWARD prefix inspection. Code `e6327b0`
refuses shadowed endpoint guards while allowing provably disjoint ingress rules.
The red/green suite covers 24 unsafe prefixes and twelve legitimate prefixes.
GCE controls reproduce a real simulated-primary TCP bypass with DROP still
present, prove CHECK refusal, and restore blocking and acceptance. Eight real
IPv4/IPv6 ACCEPT/RETURN cases pass in ephemeral namespaces; installed primary
rules/files/PIDs remain unchanged. Continuous enforcement, other firewall tables,
concurrent policy changes and live child Docker requalification remain open.
Full G7/G8 remains open; direct children require no nested virtualization and
share host trust after child-kernel compromise.


On 2026-10-26, source `017c52f` attempted combined live requalification of the
storage and firewall changes with new authenticated mknetd CHECK checkpoints at
ENOSPC and built-image holds. [The run failed before either checkpoint](../../evidence/runtime-20261026/g8-live-network-check-failure/README.md).
Its compound wait assertion does not identify attach exit versus deadline.
Forced Docker removal reported a zombie task, leaving a live NBD helper and
relay, ACTIVE/RUNNING ownership and an active child. No normal cleanup, synced
close or live network-check qualification is claimed; cause remains unproven.

The user restarted the instance. [Post-restart evidence](../../evidence/runtime-20261027/g8-failed-network-post-restart/README.md)
on boot `1c32f948-bf1f-4a8e-8774-24616691bf06` confirms SRC65, zero module
references/sockets/children, no pool, sixteen CPUs online and four active primary
services. Failed ledgers/journal and four older quarantined images/sixteen
ledgers match. The fifth image's inode/size match; its first stable hash is
post-restart, so unchanged content across reboot is unproven. Read-only e2fsck
exits 0 without changing that new hash, but missing native close and lifecycle
release evidence still prohibit reuse. Reboot is not normal cleanup.

Code `5530d39`, with process-confined controls in `5c8b627`, now records a
bounded [wait outcome before cleanup](../../evidence/runtime-20261027/g8-pressure-wait-outcomes/README.md):
reached, process-exited/return code, deadline-exceeded or output-limit, with
invocation/boot identity and output hashes. Real subprocess controls pass locally
and on the restarted GCE host; the required suite passes with pinned containerd
2.2.10. These controls do not reconstruct the previous failure or establish a
new live workload pass. Full G7/G8 stays open with direct children, no nested
virtualization and the shared host trust boundary.


The [delete-holder retention proof](../../evidence/runtime-20261027/g8-delete-holder-retention/README.md)
reproduces a separate shim cleanup-order defect: namespace-holder shutdown could
precede failed sandbox deletion. `7009445` retains the representative host PID
through host cleanup; `79a87fc` extends retention through delete-event and rootfs
cleanup steps. Source `2ba5a88` exercises five failures with real subprocesses,
pidfds and namespace inode observations. All five fail on original code and pass
on fixed code. Four transient retries retain and then release the exact original
holder; a nonretryable storage refusal remains retained. Final GCE controls pass
unprivileged with unchanged primary files/PIDs and idle resources. Complete Go
and shim race suites pass. The tests inject daemon/publication failures and do
not qualify real child deletion, the earlier pressure-stall cause, worker crash
handoff or a new live Docker build. G7/G8 remains open under direct-child,
no-nested-virtualization deployment and shared host trust after child compromise.


Code `de2cc39` adds the first positive-send-after-MK-queue-full event, with
[bounded diagnostic controls](../../evidence/runtime-20261027/g8-queue-resume-diagnostics/README.md)
in source `f0ea13b`. Per proxy invocation, at most one full and one resumed event
are emitted, including across repeated stalls. The latter contains only endpoint
and accepted byte count, not payload, peer consumption or completion. Identical
controls fail on the old header and pass after the change. Transient/repeated
injected full, duplex byte preservation, half-close, bounded permanent-full and
unmarked fatal ENOSPC cases pass locally and on GCE. Primary hashes/PIDs/resources
remain unchanged; required checks pass with pinned containerd 2.2.10.

The candidate native helper compiles but is not deployed. The qualified helper
and approved child artifacts remain pinned, and AF_UNIX injection is not actual
MK transport or Docker requalification. The data disk currently has roughly
2.7 GiB available, below image-plus-reserve admission; failed images and reserves
remain intact. Live storage-stall diagnosis and whole G7/G8 remain open under
the direct-child/no-nested-virtualization deployment constraint and shared host
trust boundary.


The [network ownership contract](contracts/network-ownership.md), implemented by
`b0a218f`, adds CAPABILITIES / INSPECT_OWNERSHIP and a separate read-only mknetd
CLI. `8489762` closes pending RPC reads on cancellation. The bounded projection
uses the store try-lock, omits private descriptor strings, reports separate
startup/endpoint boot claims and invokes no backend or reconciliation. Source
`8489762` passes [GCE qualification](../../evidence/runtime-20261027/g8-network-ownership/README.md):
two identical reports from an actual private daemon/CLI, explicit installed
legacy refusal, and projection of the retained failed endpoint without changing
its original ledger. Primary files/PIDs/resources remain unchanged; final race,
complete Go and required checks pass. The private test daemon is stopped and
installed services are unchanged. Doctor network correlation, live policy/
namespace attestation, populated live child endpoints and Docker requalification
remain open. Full G7/G8 remains open under the direct-child/no-nested-virtualization
constraint and shared host trust boundary.


`322155e` adds immutable network intent to new shim allocations and a separately
negotiated bounded expectations projection. Legacy omission remains unknown.
`392116b` adds the separate `--doctor-network` version-4 mode, comparing repeated
lifecycle intent, endpoint metadata, kernel names and boot observations. The
[network doctor proof](../../evidence/runtime-20261027/g8-network-doctor/README.md)
passes GCE invocation `240ed5dea2f44ae99fb4ed669d81d8f9`: two identical actual
private idle CLI reports, installed legacy refusal, unchanged version-3 ordinary
doctor output, seventeen populated correlation cases and six refusal/drift
cases. Focused race, complete Go and required checks pass. Private daemons stop
with unchanged primary files/PIDs, all CPUs online, no children/pool and original
transport idle. Findings distinguish missing required endpoints, orphan/stale
bindings, released owners, explicit no-network intent, boot mismatches and
recorded degradation; unknown intent and transitions stay inconclusive. Native
network policy/session/namespace health, expected descriptor/incarnation, storage
correlation, live child endpoints and Docker requalification remain open. Full
G7/G8 remains open with direct kernels, no nested virtualization and shared host
trust after child-kernel compromise.

Endpoint CHECK now rejects substring lookalikes (`1358d9a`) and has a committed
real namespace fixture (`cedd87e`). [Retained proof](../../evidence/runtime-20261027/g8-endpoint-state-values/README.md)
covers ten parser controls, four real MTU/gateway changes with restoration,
existing ordered-policy controls, all Go, network race and required suites. GCE qualification
unit b2e6ad5e842e42568e4d863fdd158c57 leaves primary identities and resources
unchanged. This tightens sampled native policy inspection; live child Docker
requalification and the full G7/G8 gates remain open.

Code `a123ad3` adds a minimum storage floor before module-trial transport switching
and disposable Docker setup. [Actual GCE refusal proof](../../evidence/runtime-20261027/g8-trial-capacity-admission/README.md)
records 2875645952 bytes available against 3221225472 required, expected driver
status 1 and successful external qualification, with unchanged primary identities,
original transport/resources and retained storage metadata. Neither trial-module
nor Docker integration directories are created. Local/GCE boundary, symlink and
ordering controls and the required suite pass. The floor preserves image/reserve
sizes and does not reserve capacity or replace builder staging admission. Full
Docker requalification still needs adequate capacity; no failed image is deleted
or reused. Whole G7/G8 remains open under the documented direct-child,
no-nested-virtualization/shared-host-trust constraint.

`5e052f4` fixes missing host NBD retry handling for MK ring-full handshake/reply/
read-payload sends. Only marked MK writes retry, within the existing absolute
frame deadline; unmarked transfers and image errors retain their policy.
[Real server-loop controls](../../evidence/runtime-20261027/g8-server-queue-retry/README.md)
in `dc30eac` pass locally/on GCE with partial sends, transient/recurrent pressure,
permanent-full timeout, unmarked ENOSPC and nonrenewable progress budgets, under
existing peer/hello checks, syscall filter and limits. Unit
13f9a7e8c48a467a81d72b326da749b6 exits successfully with unchanged watched primary
state, PIDs and idle transport/resources; required checks pass. Source review
retains the ring-full-before-publication path. The candidate helper compiles but
is undeployed; existing c9981a1/approved image pins remain unchanged. These are
injected errors on real helper paths, not actual MK pressure or proof of the
prior Docker stall's cause. Capacity and live Docker qualification remain open,
as do full G7/G8 under direct-child/no-nested-virtualization/shared-host-trust
constraints.

`efeb2e0` introduces fixed qualified/default and static server-queue-candidate
variants for disposable cold trials and their private Docker harness. [GCE proof](../../evidence/runtime-20261027/g8-static-trial-helper/README.md)
qualifies the static 591f79b candidate rebuild, both pins, real root-owned unsafe
artifact refusals and dynamic/static sandboxed native server controls. It retains
the initial staging-directory permissions refusal and its isolated 0700
correction. Qualified unit 0a00afd843264599813306a412d6c80d exits successfully
with unchanged watched primary state/PIDs and idle original transport/resources.
Required checks pass. Current runtime/shim binaries and the committed source
archive are staged at /opt/mkruntime/g8-host-efeb2e0; they are not installed and
no full child trial starts. The qualified helper remains the default. Capacity
expansion still awaits approval; reserves and failed images remain preserved.
Actual MK/child Docker pressure and full G7/G8 remain open under direct-child,
no-nested-virtualization/shared-host-trust constraints.

`a321026` replaces temporary staging recipes with a committed packet builder.
[Controls and GCE proof](../../evidence/runtime-20261027/g8-trial-stage-builder/README.md)
cover actual Go builds from committed snapshots despite dirty worktrees and
hostile ambient flags, protected umask-0002 output, no-overwrite/unsafe-parent
refusals, and unpublished failures for wrong pins or source symlinks. The actual
packet extracts on GCE with correct root-owned modes on the first attempt,
matching all source/binary hashes and explicit candidate selection. Unit
c12e100918bc44f6ab54830c9b67372e succeeds with unchanged watched primary state,
PIDs and idle transport/resources; required checks pass. The [staging instructions](trial-staging.md)
replace developer-local procedure dependencies for this preparation step. Current
artifacts are staged at /opt/mkruntime/g8-host-a321026, not installed. Fresh
capacity observation remains below the floor, and expansion awaits approval.
Full child Docker/MK qualification and full G7/G8 stay open under direct-child,
no-nested-virtualization/shared-host-trust constraints.

The user approved the 30-to-50 GB data-disk expansion. [Complete preservation proof](../../evidence/runtime-20261027/g8-data-disk-expansion/README.md)
retains the independent pre-mutation checkpoint 42c7e92 and completed result
d01906c: all 92 file hashes/identities, 11 old child images, directory/link
inventories and primary hashes/PIDs match. The successful online grow did not
retain the exact original reserved-block fraction, so a separate adjustment
restored 5 percent before independent read-only verification passed. The resize
was not repeated. Post-grow available bytes are 22931456000; the 2 GiB image and
1 GiB builder reserve remain unchanged. This removes the capacity blocker.

The first a321026 cold attempt correctly refused group-writable nested Git source
modes before a module switch or child launch. Code db62c25 forces tar.umask=0022;
a permissive Git-config regression fails before the fix and passes after it. A
fresh stage passes every extracted source mode/hash check, and its scripted
Docker pressure/build/recovery and normal cleanup run exits successfully. Final
public-proof verification and cold restoration remain pending at this checkpoint.

The completed live proof is committed as e6473d6 and separate cold-restoration
proof as 2c6c07d. The latter includes all 92 retained-file hashes/identities,
original transport/refcount, unchanged primary state hashes and active services.
The exact-size mount guard correctly refused the expanded disk's stale 30 GB
binding. Code 6d059f3 clones the verified immutable deployment and changes only
the size assignment; 45429c1 strengthens generation/mode protections. The
applied 50 GB binding retains every other asset byte/mode and the old generation.
Standalone, stale-size, shrink, alignment and unsafe-generation controls and
required checks pass. The initial cold verifier's cross-boot Docker network-ID
assertion failure is retained; the corrected read-only procedure passes exact
engine/images/containers/volumes and records changed network IDs separately.
It does not qualify prior-boot network configuration parity. Full G7/G8 stays open.

Code a29a1a1 adds strict private version-4 network-doctor checkpoints alongside
the version-3 storage/native-process doctor; c86382f retains adversarial and
independent Go/Python descriptor controls. The [actual live proof](../../evidence/runtime-20261027/g8-live-network-doctor/README.md)
57713e8 qualifies the same owner/generation/sequence/boot at real child Docker
ENOSPC, built-image hold and removal, including required intent and exact sampled
endpoint descriptor/counters. Both units exit successfully and normal cleanup
restores all CPUs, removes owned resources and preserves primary Docker engine,
image absence and exact same-boot inventories. Startup console hits its 768 KiB
cap; the separate 370-byte teardown window retains quiesce/disconnect markers.
The public proof records this limitation and does not claim complete console
coverage or kernel stability. Cold restoration is tracked separately; no full
G7/G8, atomic doctor or network-incarnation/complete-health gate closes.

After the passing a29a1a1 trial, the separate cold verifier d0a4865c on boot
1f78a4c0 succeeds with original transport/refcount zero, unchanged primary state,
all active services, the unchanged protected 50 GB binding and all 92 retained
file hashes/identities. The user then restarts again to boot 3ae4f0d8; this
historical result remains preserved. A distinct read-only verifier c433d8b3
rehashes the retained files for the user-restarted boot; it does not overwrite
the first receipt or claim a newly sampled pre-user-restart state.

The [additional user-restart proof](../../evidence/runtime-20261027/g8-user-restart-3ae4/README.md)
now passes independently: c433d8b3 terminates successfully with all 92 retained
file identities/hashes unchanged, original transport/refcount zero, all four
services active, unchanged primary state and protected 50 GB deployment. The
[earlier cold-restoration proof](../../evidence/runtime-20261027/g8-network-doctor-cold-restoration/README.md)
a53ebeb remains the original historical checkpoint. No extra workload or agent
reboot is used for the user-restarted boot. Cross-boot network identity parity
and full G7/G8 remain unqualified.

Production code 0c50de9 adds `--doctor-all` version 5 under a shared deadline,
requiring exact repeated native/storage and network lifecycle samples while
preserving degraded severity. [Control proof](../../evidence/runtime-20261027/g8-combined-doctor-controls/README.md)
d98d306 and [real GCE CLI preflight](../../evidence/runtime-20261027/g8-combined-doctor-cli/README.md)
6f69506e pass and are pushed. The actual child attempt subsequently fails before
any active-child combined-doctor invocation. The [failure proof](../../evidence/runtime-20261027/g8-combined-live-failure/README.md)
retains a 300-second wait with no workload output, agent sequence-mismatch/refused
reconnect diagnostics, zombie-PID Docker removal failure, ACTIVE/RUNNING storage
and lifecycle claims and a DISCONNECTED endpoint. Both units are terminal with
exit 1; MainPID zero does not mean the retained native server/backing fd or child
is gone. Primary state hashes/PIDs and primary Docker engine/inventories match,
but child resources and host networking are not claimed released. Root cause,
actual version-5 child qualification and full G7/G8 remain open. Private original
ledgers are copied without mutation; raw console and state stay private. No blind
retry, hot module unload or automatic storage/ledger release is performed.

[Agent stream invalidation proof](../../evidence/runtime-20261027/g8-agent-stream-invalidation/README.md)
408f790c reproduces a separate client defect against exact committed 0c95b5a9 on
both pipes and real Unix sockets: a canceled response read leaves a usable stream,
so the next call writes another request and consumes the previous reply as a
sequence mismatch. Code a4b11660 closes ambiguous framing/deadline failures and
requires explicit reconnect without resetting the request sequence or replaying
inside the client. Structured operation errors and complete typed-body rejection
retain synchronized streams. Full runtime race and required checks pass.
[GCE controls](../../evidence/runtime-20261027/g8-agent-stream-invalidation-gce/README.md)
cbbe69ca independently pass under f52117e5 in an isolated unit with synthetic
authenticated agent sessions and real Unix sockets. Primary files/PIDs/Docker
inventories, failed ledgers and native backing-process identity match before and
after. The original failed child/export and candidate transport remain retained.
This does not establish that stream reuse caused the 0c50de9 startup failure,
qualify actual MK/child recovery, or close full G7/G8.

On 2026-10-28, [checked-peer and sealed-descriptor controls](../../evidence/runtime-20261028/g8-agent-peer-probe-controls/README.md)
and [native CID controls](../../evidence/runtime-20261028/g8-relay-peer-controls/README.md)
precede the [actual retained-child probe](../../evidence/runtime-20261028/g8-retained-agent-capabilities-gce/README.md).
Code 30986549 retains the exact local relay identity policy across reconnect;
the bounded probe sends only Capabilities. GCE invocation 87b73fd6 returns
protocol 1, expected child kernel release and explicit root UID/GID claims.
The owned relay exits zero; primary hashes/PIDs/Docker inventories and retained
ledgers, child/CPU/module and native backing-process identities match. No backing
image contents are read. Requests are signed, but reply-side cryptographic
attestation and kernel incarnation are not established. That probe predates
the production factory's CID-mode adoption described below. The failed child/export remains
retained; startup cause, Docker recovery, normal cleanup and full G7/G8 remain
open. Direct child kernels use no nested virtualization and share host trust.

The shared production shim relay factory now requires the sandbox's declared
CID for initial startup, worker recovery and exited relay replacement, with no
wildcard fallback for older helpers. [Local controls](../../evidence/runtime-20261028/g8-production-relay-controls/README.md)
cover this selection, legacy-helper refusal and relay lifecycle races. The
[actual GCE factory qualification](../../evidence/runtime-20261028/g8-production-relay-gce/README.md)
passes under a12db1ee: declared CID 41 observes a real refusal without Unix
publication, then CID 40 returns matching capabilities. The
[earlier failed positive case](../../evidence/runtime-20261028/g8-production-relay-gce-failed/README.md)
and independently reproduced partial-schema fixture defect remain retained.
The corrected source is 3f2f8efb; primary and retained resource observations match
before/after. No installed runtime is replaced. That factory-only proof does not
qualify production local peer checks. Complete Docker lifecycle/deployment qualification, retained-startup recovery,
version-5 live doctor and full G7/G8 remain open. No nested virtualization or
containment after child-kernel compromise is claimed.

Production change 2b31b065 then requires protected captured socket identity and
the owned live relay's exact PID/UID/GID for initial dial and reconnect, with
relay ownership locked through verification. An explicit checked replacement
rebind preserves authentication sequence and the initially captured UID/GID;
ordinary reconnect remains pinned, and failed verification preserves policy and
sequence. [Local controls](../../evidence/runtime-20261028/g8-production-peer-controls/README.md)
verify zero writes on production wrong-peer refusal, missing/reaped ownership,
and authenticated continuity across distinct real proxy subprocesses. The found
typed-nil adapter failure is retained with its correction. Full runtime race and
required checks pass with qualified containerd 2.2.10.

[Actual GCE qualification](../../evidence/runtime-20261028/g8-production-peer-gce/README.md)
under 26412eea uses the production factory, checked dial, exited helper replacement
and checked reconnect against the retained MK child. The old helper exits, its
replacement has a new PID, and matching capabilities return through the production
reconnect loop. Primary hashes/PIDs/Docker inventories and retained resource
identities match before/after. The child/export remains retained, without image
content reads or installed runtime replacement. This qualifies these transport
methods; complete Docker creation/build/worker recovery, version-5 live doctor,
normal cleanup, readiness and full G7/G8 remain open. No nested virtualization
is used; direct kernels share host trust after compromise.

The version-2 [bounded process-state diagnostic](../../runtime/tools/mk-agent-capabilities-probe/README.md)
adds an explicit read-only StateProcess query after capabilities match, under
one deadline with no reconnect/replay. [Controls](../../evidence/runtime-20261028/g8-process-state-controls/README.md)
qualify request/identity/state/PID binding, absence semantics and private
output/error redaction; default version-1 single-request behavior is preserved.
The [actual retained-init observation](../../evidence/runtime-20261028/g8-retained-process-state-gce/README.md)
under 0d1717ab reports STOPPED/guest PID 616/exit 137 while capabilities match.
Exit zero means successful observation, including typed absence, rather than
readiness. It does not determine when/why the task stopped or whether the
separately started child Docker engine is healthy. Primary state and retained
resource identities match before/after; no image contents are read, guest
mutation requested or cleanup performed. Original failure diagnosis, fresh Docker
lifecycle qualification, version-5 live doctor and full G7/G8 remain open. Direct
children use no nested virtualization and share host trust after compromise.
