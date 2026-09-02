# Multikernel Linux experiments and container runtime

This repository validates Multikernel Linux on Google Compute Engine and uses
those results as the foundation for a Kerf-backed containerd/Kubernetes
runtime. The runtime tree now contains a live-tested MVP control plane, child
agent, primary-mediated network, and containerd Runtime v2 shim.

## What has been proven

| Theme | Result | Details |
| --- | --- | --- |
| GCE bring-up | Primary plus two concurrent child kernels passed; resources returned cleanly | [GCE runbook](docs/guides/gce-lab-runbook.md) |
| DAXFS roots | DAXFS and Docker-derived roots passed; shared writable coherence failed | [DAXFS report](docs/experiments/daxfs/report.md) |
| Direct ext4 devices | Rejected because child and boot disks share one allocatable controller | [Direct ext4 experiment](docs/experiments/ext4-direct/README.md) |
| Mediated ext4 roots | Two isolated persistent child roots passed while the primary retained every controller | [Mediated ext4 report](docs/experiments/ext4-mediated/report.md) |
| Container runtime | The shared G4-G6 `ctr`/Docker feature matrix passed on a disposable GCE host with one child kernel per container; unsupported Task v2 and full-gate items remain explicit | [Feature matrix](#g4-g6-ctr-and-docker-feature-matrix) |

## G4-G6 `ctr` and Docker feature matrix

Except where a historical proof is named, `Passed` means the operation was
exercised through both clients on disposable GCE instances, most recently
`mklinux-g4-g6-io-20260901`. `Partial` and `Not implemented` are not
release claims. Docker commands require `--runtime
io.containerd.multikernel.v2 --network none` because the runtime, rather than
Docker's bridge, owns the child link.

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
| KILL and exit 137 | `ctr tasks kill --signal SIGKILL` | `docker kill --signal KILL` | Passed in the earlier [MVP proof](docs/runtime/learnings/04-g4-g6-mvp.md) |
| Delete, cleanup, and name reuse | `tasks rm`, `containers rm` | `docker rm`; same name reused | Passed twice from clean state |
| Concurrent isolated sandboxes | two live Runtime v2 tasks | two live Runtime v2 tasks | Passed with distinct child boot IDs |
| Private writable OCI root | exec read/write proof | exec read/write proof | Passed; mutations are sandbox-private, not snapshot persistence |
| Outbound DNS/HTTP | child `mkn0` through primary NAT | child `mkn0` through primary NAT | Passed |
| Cross-sandbox network isolation | sibling `/30` unreachable | sibling `/30` unreachable | Passed |
| `mkruntimed` restart | running task retained its boot ID | running container retained its boot ID | Passed concurrently |
| Client daemon restart | system containerd restart retained as an operator observation | Docker daemon restart not proved | Partial; raw restart evidence is absent |
| Forced shim death | safe reclaim retained as an operator observation | not separately proved through Docker | Partial; raw evidence and running-task reconnect are absent |
| Process list | Task `Pids` reports the shim PID | Docker metadata can consume it | Partial; guest PID fidelity is not implemented |
| Stdin/attach and `CloseIO` | foreground stdin and `tasks attach` | foreground stdin and `docker attach` | Passed; FIFO bytes drain before guest EOF |
| Terminal and resize | `--tty`, `ResizePty` | `--tty`, resize | Partial; PTYs and initial 91×37 propagation passed live, while deliberate post-start resize has only local agent coverage |
| Pause and resume | `tasks pause/resume` | `docker pause/unpause` | Not implemented; live rejection proved for both |
| Metrics/stats | Task `Stats` | `docker stats` | Not implemented |
| Runtime resource update | Task `Update` | `docker update` | Not implemented |
| Checkpoint/restore | Task `Checkpoint` | Docker checkpoint | Not implemented |
| Full OCI controls | capabilities, seccomp, namespaces, mounts, hooks, rlimits, read-only root | equivalent Docker flags | Not implemented; the current adapter can silently discard unsupported fields, so end-to-end fail-closed handling remains open |
| CNI `ADD`/`CHECK`/`DEL` | no CNI adapter | no CNI adapter | Not implemented; static mediated networking only |

The repeatable test is
[`scripts/test-runtime-g4-g6-feature-matrix.sh`](scripts/test-runtime-g4-g6-feature-matrix.sh),
with the [latest GCE transcript and final clean inventory](evidence/runtime-20260902/g4-g6-io-live/README.md).
The G4-G6 gates remain provisional; see the
[robustness learnings](docs/runtime/learnings/05-g4-g6-robustness.md).

The final experimental VM and its auto-delete boot disk were deleted. No GCE
instances remain. Only the pre-existing 20 GiB
`mk-mediated-storage-20260830` disk was intentionally retained and remains
billable.

## Start here

- [Documentation map](docs/README.md)
- [Experiment index](docs/experiments/README.md)
- [Runtime architecture](docs/runtime/architecture.md)
- [Runtime implementation plans](docs/runtime/plans/README.md)
- [Open and completed tasks](docs/project/TASKS.md)
- [Runtime source-tree contract](runtime/README.md)

For the verified GCE workflow:

```bash
make help
make docs-check
```

Creating or starting GCE resources is billable. Read the
[GCE runbook](docs/guides/gce-lab-runbook.md) and inspect the active project,
firewall, machine type, and disk lifecycle before using a cloud target.

## Repository layout

```text
docs/
├── experiments/   completed plans, execution records, reports, and notes
├── guides/        reproducible operational procedures
├── project/       task and project tracking
└── runtime/       architecture, decisions, research, and future plans
runtime/           daemon, Runtime v2 shim, child agent, protocol, and tests
scripts/           verified host and GCE experiment automation
guest/             child init and proof programs
tools/             Multikernel VSOCK/NBD helpers
patches/           pinned upstream compatibility patches
evidence/          raw indexed experiment output
docker/            Docker-derived DAXFS proof input
```

## Safety boundary

The planned runtime targets trusted, single-tenant nodes. Multikernel sibling
kernels do not currently provide Firecracker's KVM/EPT hardware isolation
boundary. Never assign the primary boot disk, NIC, or a shared controller to a
child, and never allocate APIC ID 0.
