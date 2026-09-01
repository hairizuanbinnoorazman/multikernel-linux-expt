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
| Container runtime | G4-G6 MVP path passed concurrently through `ctr` and Docker with one child kernel per container; full gate matrices remain open | [G4-G6 MVP report](docs/runtime/learnings/04-g4-g6-mvp.md) |

The final experimental VM and its auto-delete boot disk were deleted. No GCE
instances remain. The 20 GiB mediated-storage disk was intentionally retained
and remains billable.

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
