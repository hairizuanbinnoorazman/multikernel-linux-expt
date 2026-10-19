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
