# Persistent ext4 child-root experiment learnings

This log records observed results from the 2026-08-30 execution of
[`EXT4-DISK-PLAN.md`](EXT4-DISK-PLAN.md). Planned behavior is not presented as
completed behavior.

## Result

The VM and two blank child disks were created. Stage 0 passed, but the Stage 1
device-granularity gate failed. Testing stopped before formatting, pooling, or
device handoff, exactly as required by the plan's safety boundary.

The blocking topology is:

```text
PCI 0000:00:03.0 (virtio-pci / virtio_scsi)
├── SCSI 0:0:1:0 -> /dev/sda -> boot/root disk
└── SCSI 0:0:2:0 -> /dev/sdb -> child-a-root (blank)
```

Pinned Kerf can describe `0000:00:03.0`, but it cannot describe `/dev/sdb`,
its persistent Google by-id name, or SCSI target/LUN 2 as an independently
allocatable device. Pinned Multikernel code transfers PCI devices using PCI
domain, bus, and `devfn`. Assigning `0000:00:03.0` would therefore assign the
controller that also owns the live boot disk. It was not attempted.

## Cloud resources created

- Project: `new-demo-project-462517`.
- Zone: `asia-southeast1-b`.
- Instance: `mklinux-lab`, `n2-standard-16`, restored from snapshot
  `mklinux-lab-pre-daxfs-20260828-2030`.
- Boot disk: `mklinux-lab`, 100 GiB `pd-balanced`, auto-delete enabled with the
  VM.
- Child disks: `child-a-root` and `child-b-root`, each 10 GiB `pd-balanced`.
- `child-a-root` is attached as `mk-child-a-root` with `autoDelete: false`.
- `child-b-root` is created but deliberately left unattached until Stage 1.
- The VM has an external IPv4 address and uses the existing default-network
  firewall posture documented in `README.md`.

These resources were billable during the run and were deleted after final
verification.

## Baseline observations

- Snapshot restoration into a new boot disk and instance worked on the first
  live attempt; this recovery path had not previously been tested.
- The restored VM booted `7.0.0-mk2-gce-lab` and reported boot ID
  `b0f9c023-ffac-4f47-ba4d-8062ce8b08d3`.
- All 16 vCPUs and 65,836,300 KiB of memory were visible.
- `google-guest-agent` was active, `/` was read-write ext4 on `/dev/sda1`,
  Multikernel sysfs was present, and the Kerf virtual environment was present.
- Source pins were reverified: Linux
  `3bdd35b64413da0b4e089ce931bfc2e8b031cbf7` and Kerf
  `8b72b3e9b266f8d32e707e2c1743ad7afc50b1ec`.
- No Multikernel memory pool or child instance existed before or after the
  audit.

## Disk identity and topology observations

- `/dev/disk/by-id/google-mk-child-a-root` resolved to `/dev/sdb`.
- Udev reported serial `mk-child-a-root`, model `PersistentDisk`, size exactly
  10,737,418,240 bytes, and SCSI address `0:0:2:0`.
- The disk had no partitions, filesystem type, label, UUID, mount, holder, or
  signature reported by `lsblk`, `findmnt`, and `wipefs -n`.
- The boot disk was SCSI `0:0:1:0`; both disks shared sysfs ancestor
  `/devices/pci0000:00/0000:00:03.0/virtio0/host0`.
- PCI function `0000:00:03.0` used `virtio-pci`; SCSI host 0 reported
  `virtio_scsi` and `running`.
- The running kernel has the relevant built-ins: `CONFIG_VIRTIO_PCI=y`,
  `CONFIG_SCSI_VIRTIO=y`, `CONFIG_SCSI=y`, `CONFIG_BLK_DEV_SD=y`,
  `CONFIG_EXT4_FS=y`, `CONFIG_DEVTMPFS=y`, and `CONFIG_PCI_MSI=y`.

## Source and tooling observations

- Kerf's `detect_pci_device()` recognizes a PCI BDF, a network interface with a
  PCI parent, or a direct child of a PCI udev device. The SCSI block disk is not
  a direct PCI child, so both `mk-child-a-root` and its by-id path are rejected.
- `kerf init --devices=0000:00:03.0 --report` produced a valid report containing
  one `pci-device` with PCI ID `0000:00:03.0`. It did not apply the pool.
- Multikernel's baseline parser requires PCI device IDs and calls
  `mk_pool_device_add(domain, bus, PCI_DEVFN(slot, func))`; its transfer and
  return bookkeeping is PCI-function based. There is no SCSI LUN allocation
  unit in these pinned paths.
- This `gcloud` version does not accept `--no-auto-delete` on
  `instances attach-disk`. Omitting `--auto-delete` attached the child disk with
  the verified default `autoDelete: false`.
- One initial read-only sysfs audit loop failed to terminate after reaching an
  empty parent path. It was interrupted without state changes. The committed
  audit script explicitly converts the empty parent to `/` and terminates.

## Implementation and disposition

- `scripts/disk-roots-create.sh` provides idempotent cloud creation and only
  attaches child A. It never formats a block device.
- `scripts/disk-roots-audit.sh` resolves a persistent by-id link, verifies the
  expected whole-disk size and identity evidence, maps root and target PCI
  ancestors, and exits 2 on a shared PCI function.
- No preparation, `mkfs.ext4`, or disk-device up/down script was added because
  that would imply an executable continuation beyond the failed hard gate. A
  failure-safe bootstrap was added and tested without a device; no child disk
  was formatted.
- Both child disks remained blank throughout the experiment and were deleted
  during final cleanup. They were never handed to a child.

## Required next design decision

Continuation needs a storage topology that presents the child disk through a
PCI function distinct from the boot disk, or a separately versioned transport
that safely proxies block I/O through the primary. Candidate machine/storage
interfaces must be proven from a new live topology audit; switching machine
family, disk interface, Kerf revision, or kernel patch is a new experiment,
not a retry of this pinned baseline.

## Continuous full-document run

The follow-up run evaluated the rest of the document without waiting between
stages. Dependent actions remained subject to the original hard safety gates.
The complete pass/blocked matrix is in
[`EXT4-DISK-EXECUTION.md`](EXT4-DISK-EXECUTION.md).

### Alternate C3/NVMe topology

- A temporary `c3-standard-22` VM was restored from the same snapshot with its
  boot and blank child-B disks explicitly attached using NVMe.
- The custom primary kernel booted and `google-guest-agent` was active.
- GCE exposed the boot disk as `nvme0n1` and child B as `nvme0n2`; both were
  namespaces of controller `nvme0` on PCI function `0000:00:05.0`.
- Kerf has namespace fields in its Python model and validator, but the pinned
  Multikernel baseline parser consumes only PCI BDFs and transfers a whole
  `devfn`. Namespace metadata therefore does not provide namespace-granular
  ownership in this kernel.
- The probe VM was stopped immediately after the audit and then deleted; its
  auto-delete boot disk was removed. Child B remained blank until final cleanup.

### Restored-host and child regression

- `verify-host.sh` passed on the restored N2 VM.
- Two no-device children ran concurrently with four vCPUs each and reached
  `CHILD_READY`; the primary kernel, SSH, and guest agent remained healthy.
- Cleanup returned APIC IDs 8–15 and the complete 16 GiB pool allocation.
- The child boot logs again showed inability to allocate SWIOTLB memory. This
  was harmless for the no-device proof but reinforces that block-device DMA
  remains unproven.
- Kerf `create` repeatedly applied the overlay and then raised
  `KeyError: <instance-name>` while rereading current state. Existing scripts
  tolerate this only after proving the instance directory exists. This is a
  pinned-Kerf race/visibility defect, not a failed child creation.

### Alternate kernel reconstruction

- The snapshot contained only the primary `vmlinux`; no alternate artifact was
  present.
- An initial external-output (`O=`) build was rejected because the restored
  source tree contains in-tree build products. Cleaning it would have risked
  the verified primary artifact.
- A detached worktree at commit `3bdd35b6…` preserved the primary tree and
  successfully built `7.0.0-mk2-gce-lab-alt`.
- Alternate `vmlinux` SHA-256:
  `fdbbb9cd1192761122d12167baed129eabd30cc790332e30a94e9ffb40891506`.
- The alternate kernel booted and reported its expected release through MKTTY.

### Bootstrap behavior and automation findings

- `guest/ext4-bootstrap-init` mounts devtmpfs/proc/sysfs, parses
  `mk.root_uuid`, rejects multiple matches, waits ten seconds for exactly one
  UUID, mounts ext4 only after a match, validates root markers, and otherwise
  enters an MKTTY diagnostic state.
- Both primary and alternate kernels passed the no-device failure test with
  `EXT4_BOOTSTRAP_FAIL reason=root-not-found`.
- Both kernels then ran concurrently with different expected UUIDs. Each
  reported only its own release/UUID and refused safely; host health passed.
- The first bootstrap invocation failed before pool creation because the
  Makefile's explicit sync list omitted the new guest file. Adding it to the
  manifest fixed the test.
- Bootstrap archive hashes changed across builds because the generic builder
  includes fresh filesystem/archive timestamps. A deterministic builder is
  still needed before treating the archive hash as a stable manifest field.
- The bootstrap reports that `e2fsck` is unavailable and skips filesystem
  checking. A production root bootstrap needs an audited static e2fsck binary
  or a clearly defined primary-side check policy.

### Why later tests remain blocked

Stages 3 through 7 require a child-owned ext4 block device. Formatting a disk
despite the failed allocation gate would create data that can never be safely
handed off on either tested topology and would contradict the plan. Therefore
no labels, UUIDs, filesystem manifests, persistence markers, forced-stop ext4
checks, primary-reboot persistence run, GCE stop/start run, dirty-filesystem
repair test, or snapshot clone was claimed or simulated.

### Final cleanup and recoverability

- Final checks proved no child instances or Multikernel pool, all 16 N2 CPUs
  online, the guest agent active, and child A still free of signatures.
- `mklinux-lab` was deleted; its auto-delete boot disk and remote primary/
  alternate build trees were removed with it.
- Both blank child disks were deleted explicitly. They contained no filesystem
  or experiment data and are not recoverable because no snapshots were taken.
- The two earlier recovery snapshots remain `READY`. They can recreate the
  verified primary baseline, but the alternate kernel must be rebuilt using the
  detached-worktree procedure recorded above.
- Post-cleanup inventory contained no Compute Engine instance or disk. Only
  snapshot storage remains billable.
