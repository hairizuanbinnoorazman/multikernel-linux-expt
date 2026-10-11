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

The runtime targets trusted, single-tenant nodes. Multikernel sibling kernels
do not currently provide Firecracker's KVM/EPT hardware isolation boundary.
Never assign the primary boot disk, NIC, or a shared controller to a child, and
never allocate APIC ID 0. See the [runtime architecture](architecture.md) for
the complete trust boundary and ownership rules.
