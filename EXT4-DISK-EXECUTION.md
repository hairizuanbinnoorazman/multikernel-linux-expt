# Persistent ext4 child-root execution matrix

Execution date: 2026-08-30

This is the continuous execution record for every section of
[`EXT4-DISK-PLAN.md`](EXT4-DISK-PLAN.md). The run did not pause between stages;
the stage labels below identify dependency and outcome only. A blocked result
means the action was reviewed but could not be performed without violating the
plan's explicit boot-disk safety boundary.

| Plan section | Result | Observed outcome |
| --- | --- | --- |
| Assumptions/defaults | Used | Two child kernels, pinned primary/alternate builds, whole-disk ext4 roots, 10 GiB disks, and BusyBox bootstrap remained the target. |
| Fixed versions | Pass | Linux `3bdd35b6…` and Kerf `8b72b3e9…` were verified. No revision was changed to bypass the blocker. |
| Safety boundaries | Pass | Stable by-id names were used; no signature, format, mount, pool-device assignment, or boot-controller handoff occurred. |
| Stage 0 restore | Pass | The pre-DAXFS snapshot restored successfully into a new N2 VM. Custom kernel, 16 vCPUs, memory, root, NIC, metadata path, guest agent, SSH, serial configuration, and Kerf were healthy. |
| Stage 0 no-device regression | Pass | Two four-vCPU children ran concurrently and reached `CHILD_READY`; cleanup returned all CPUs and memory. |
| Stage 0 alternate kernel | Pass | A detached pinned worktree produced `7.0.0-mk2-gce-lab-alt`; it booted successfully. |
| Stage 1 N2/SCSI topology | Hard blocker | Boot SCSI target 1 and child target 2 share virtio-SCSI PCI function `0000:00:03.0`. Kerf and the kernel allocate the PCI function. |
| Stage 1 C3/NVMe alternative | Hard blocker | Boot namespace `nvme0n1` and child namespace `nvme0n2` share NVMe PCI function `0000:00:05.0`. Kerf models namespace metadata, but the pinned kernel pools only PCI domain/bus/devfn. |
| Stage 2 built-in support | Pass | Both kernels contain Multikernel, MKTTY, kexec-file, initramfs, devtmpfs, ext4, SCSI/virtio-SCSI, NVMe, PCI MSI, and IOMMU support. |
| Stage 2 bootstrap absent-root behavior | Pass | Both kernels waited for the specified UUID, reported `root-not-found`, and entered a bounded MKTTY diagnostic state. |
| Stage 2 actual controller/DMA path | Blocked | No independently assignable controller exists. SWIOTLB allocation warnings also remain and actual child block DMA cannot safely be tested. |
| Stage 3 disk creation | Partial | Both 10 GiB `pd-balanced` resources exist. A is attached for audit and B is unattached. Both remain blank. |
| Stage 3 formatting/population | Blocked | The plan explicitly prohibits formatting after a failed Stage 1 gate. No UUID, label, root tree, or ext4 manifest was created. |
| Stages 4–6 child ext4 roots | Blocked | Every single-child and dual-child root test depends on transferring an independently assignable disk. |
| Stage 7 persistence/recovery | Blocked | No child filesystem can be created or owned, so restart, dirty-filesystem, primary reboot, stop/start, and snapshot/clone persistence claims cannot be made. |
| Stage 8 safe automation | Partial pass | Idempotent cloud creation, read-only topology audit, bounded absent-root bootstrap, and concurrent distinct-kernel absent-root tests were implemented and live-tested. Destructive prepare/up/down automation was deliberately not added before a manual storage pass. |
| Final cleanup | Pass | All child instances and pools were removed. Final host health passed. Both probe VMs, their boot disks, and both blank child disks were deleted. |

## Decisive allocation evidence

```text
N2 / SCSI
0000:00:03.0 virtio_scsi
├── 0:0:1:0 /dev/sda  persistent-disk-0 (boot/root)
└── 0:0:2:0 /dev/sdb  mk-child-a-root (blank)

C3 / NVMe probe
0000:00:05.0 Google NVMe
├── nvme0n1  persistent-disk-0 (boot/root)
└── nvme0n2  mk-child-b-root (blank)
```

Pinned Multikernel's baseline parser reads `device-type = "pci"` and
`pci-id`, then calls `mk_pool_device_add(domain, bus, PCI_DEVFN(slot, func))`.
It does not parse or allocate the Kerf model's `namespace-id` as a separate
kernel resource. Consequently, neither observed GCE storage interface can
transfer one child disk while retaining the boot disk.

## Verified artifact ledger

| Artifact | Release | SHA-256 |
| --- | --- | --- |
| Primary `~/src/linux/vmlinux` | `7.0.0-mk2-gce-lab` | `5cdf26d0d34bfc8ab3d298d99f8a1e189aa6e2dba9be1f4cb2968078548a3c10` |
| Alternate `~/src/linux-alt-worktree/vmlinux` | `7.0.0-mk2-gce-lab-alt` | `fdbbb9cd1192761122d12167baed129eabd30cc790332e30a94e9ffb40891506` |

The bootstrap archive hash changes on each rebuild because the current generic
builder preserves fresh staging/archive timestamps. Its behavior was tested,
but it is not yet a bit-for-bit reproducible artifact.

## Final cloud disposition

- `mklinux-lab` and its auto-delete boot disk: deleted after final host-health,
  empty-pool, and blank-disk verification.
- `child-a-root` and `child-b-root`: verified blank, then deleted explicitly.
- `mklinux-ext4-nvme` and its auto-delete boot disk: deleted after the C3
  topology probe.
- Compute Engine inventory after cleanup: no instances and no disks.
- `mklinux-lab-stock-20260828` and
  `mklinux-lab-pre-daxfs-20260828-2030`: retained and `READY`.

Only the retained snapshots can continue to incur experiment-related charges.
