# Project and runtime status

This page summarizes the results proved by the retained experiment and runtime
evidence. Detailed procedures, architecture, learnings, and evidence contracts
remain authoritative for their respective areas.

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
