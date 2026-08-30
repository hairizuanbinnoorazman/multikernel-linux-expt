# Persistent ext4 child-root experiment plan

Last updated: 2026-08-30

## Status

Executed as one continuous run on 2026-08-30. Every section was evaluated and
all independent prerequisites were run, including snapshot restore, the
two-child regression, alternate-kernel rebuild, bounded absent-root behavior,
and a C3/NVMe topology probe. Both N2/SCSI and C3/NVMe expose boot and child
disks through one PCI function, while pinned Kerf/Multikernel allocates PCI
functions. The dependent ext4 stages are therefore blocked. No filesystem was
created and no device handoff was attempted. See
[`EXT4-DISK-EXECUTION.md`](EXT4-DISK-EXECUTION.md) and
[`EXT4-DISK-LEARNINGS.md`](EXT4-DISK-LEARNINGS.md).

## Objective

Extend the verified Multikernel Linux experiment to run two child kernels at
the same time, each with an exclusively owned, persistent ext4 root disk:

```text
GCE VM
├── primary: 7.0.0-mk2-gce-lab
│   ├── GCE boot disk (never assigned to a child)
│   └── GCE NIC and management services
├── child-a: kernel build A + ext4 root disk A
└── child-b: kernel build B + ext4 root disk B
```

The primary kernel creates, identifies, formats, and populates both child
disks before any device handoff. Kerf continues to load each child kernel and
its bootstrap initramfs; the assigned disk supplies the persistent root
filesystem. DAXFS is not used in the decisive test.

The experiment is successful only if:

- Both children are active concurrently under distinct kernel binaries and
  report their expected, distinct kernel releases.
- Each child mounts its own expected ext4 filesystem as `/`, by UUID, read-write.
- Neither child can see or modify the other child's root disk.
- Data written by each child survives child stop/delete/re-create and a full
  primary-kernel reboot. A later subtest covers GCE stop/start.
- The primary retains its boot disk, NIC, SSH, metadata access, guest agent,
  serial console, expected boot ID until an intentional reboot, and sufficient
  CPUs and memory.
- Normal teardown returns child devices, CPUs, and memory to the primary
  without filesystem errors or a host reboot.

## Assumptions and decisions that still need confirmation

The plan uses the following defaults so that planning can proceed, but these
are not silently treated as facts:

| Topic | Planning default | Why it remains open |
| --- | --- | --- |
| Number of kernels | Two concurrent **child** kernels plus the primary | “Two different running” could instead mean the primary plus one child. |
| Kernel difference | First use two builds of pinned `v7.0-mk2` with different `LOCALVERSION` values | This isolates disk/device risk. Truly older Linux or Multikernel revisions add an unproven host/child ABI variable and should be a follow-up matrix. |
| Disk role | Each ext4 disk is the child's `/`, not merely `/data` | A data-disk-only test is simpler and could precede root boot if desired. |
| Root contents | Minimal static BusyBox root with a deterministic manifest | No distribution, OCI image, service, or application payload has been selected. |
| Disk type and size | Two 10 GiB zonal `pd-balanced` disks, not auto-deleted with the VM | This is sufficient for a minimal root but may be too small for a distribution or application. It incurs storage charges while retained. |
| Filesystem layout | ext4 directly on the whole disk; no partition table | This removes a partition-discovery variable. A GPT layout can be added if it is an explicit requirement. |
| Recovery VM | Restore the latest suitable retained snapshot, then rebuild missing alternate artifacts | The pre-DAXFS snapshot restore passed on 2026-08-30; the stock-snapshot path remains untested. Post-snapshot alternate artifacts must be rebuilt. |

The largest unresolved feasibility question is device granularity. The tested
N2 machine family normally exposes GCE Persistent Disk through SCSI. Multiple
logical disks can share one virtual SCSI controller. Kerf documents exclusive
device allocation, but the exact pinned kernel/Kerf revisions have not been
shown to hand an individual SCSI LUN to a child while the primary safely keeps
the same controller and its boot LUN. **No disk is to be formatted and no
device is to be assigned until Stage 1 resolves this from source and live
topology evidence.**

If assignment is controller-wide and the controller also owns the boot disk,
this topology is a hard stop. Do not attempt the handoff. The alternatives are
to find a GCE machine/storage interface that exposes an independently
assignable controller, add a supported paravirtual block transport between
kernels, or keep storage in the primary and expose it through a tested proxy.

## Fixed version baseline

The first persistent-disk run keeps the already verified pins:

| Component | Revision |
| --- | --- |
| Multikernel Linux | `v7.0-mk2` / `3bdd35b64413da0b4e089ce931bfc2e8b031cbf7` |
| Kerf | `v0.2.0` / `8b72b3e9b266f8d32e707e2c1743ad7afc50b1ec` |
| Primary release | `7.0.0-mk2-gce-lab` |
| Child A release | `7.0.0-mk2-gce-lab` |
| Child B release | `7.0.0-mk2-gce-lab-alt` |

Do not move a component to its default branch to make device assignment work.
Record the incompatibility first and treat a version change or patch as a new,
identified experiment.

## Safety boundaries

1. Use newly created, disposable secondary disks with unique GCE names and
   unique attachment device names. Never use the GCE boot disk, a snapshot, or
   a disk containing valued data as a format target.
2. The cloud-side creation step and guest-side formatting step must be
   separate. Creating a disk is repeatable; formatting is destructive and
   must never happen merely because a provisioning script was rerun.
3. Resolve disks through `/dev/disk/by-id/google-*`, then cross-check GCE disk
   name, attachment index, size, serial, sysfs parent, current signatures,
   mount state, and the host root device. Never select a target from `/dev/sdX`
   or discovery order alone.
4. A formatting function must refuse unless all expected identity checks pass,
   the device has no partitions or signatures, it is not mounted or held open,
   it is not the parent of `/`, and an explicit per-disk first-format flag was
   supplied. It must not use a broad glob as the target.
5. Use unique filesystem labels and UUIDs. The child bootstrap must use the
   recorded UUID, not `/dev/sdb`, `/dev/sdc`, or an assumed LUN number.
6. The primary must run `sync`, unmount the child filesystem, and verify that
   it is no longer mounted before device handoff. A disk has exactly one
   kernel owner at a time.
7. Never mount the same ordinary ext4 filesystem from two kernels
   concurrently. GCE multi-writer attachment does not make ext4 a clustered
   filesystem.
8. Keep the boot disk controller and NIC outside the Kerf pool. If the child
   disk cannot be separated from either at the allocation boundary, stop.
9. Enable and verify GCE serial output and retain the stock Ubuntu GRUB entry.
   Take a boot-disk snapshot before the first device-assignment test.
10. Start with one child, one disk, four vCPUs, and conservative memory. Only
    proceed to the second child after device return and host health pass.
11. Scripts stop on an unexplained failure and collect evidence before
    cleanup. They do not delete persistent disks by default.

## Proposed automation

Automation is added only after the underlying manual commands are understood:

| Script/target | Responsibility |
| --- | --- |
| `disk-roots-create` | Create and attach two named GCE disks with auto-delete disabled; no guest formatting. |
| `disk-roots-audit` | Map GCE resources to persistent by-id names, block devices, controllers/LUNs, drivers, IOMMU groups, and Kerf device names. Read-only. |
| `disk-roots-prepare` | With explicit first-format authorization, make ext4, mount on the primary, populate deterministic roots, record UUIDs/manifests, unmount, and sync. |
| `disk-root-up-a` | Dry-run allocation, assign only disk A, boot child A, and capture MKTTY evidence. |
| `disk-root-down-a` | Stop/unload/delete child A, return its device and resources, then verify the filesystem from the primary. |
| `disk-roots-up` | Boot both children with distinct kernels and disks after both single-child gates pass. |
| `disk-roots-status` | Report host health, Kerf state, device ownership, child console markers, filesystem UUIDs, and unexpected mounts. |
| `disk-roots-down` | Stop both children, return devices/CPUs/memory, run primary-side read-only filesystem checks, and retain disks. |
| `disk-roots-delete` | Optional, separately invoked cloud cleanup with explicit disk names and confirmation; never part of normal down. |

The scripts should keep a small state manifest containing the GCE project,
zone, VM, disk resource names, attachment device names, expected byte sizes,
filesystem UUIDs, labels, root-tree hashes, kernel releases, and kernel hashes.
Observed state must still be checked against the manifest on every run.

## Experiment stages

Each stage adds one new variable. Do not proceed after an unexplained failure.

### Stage 0: restore and baseline gate

- Choose whether to restore `mklinux-lab-pre-daxfs-20260828-2030` or perform a
  clean rebuild. Record the new VM, boot disk, zone, machine type, and image or
  snapshot source.
- Prove the running primary is the expected Multikernel build and repeat
  `make verify-host`.
- Confirm all vCPUs are online, no instances or pool exist, `/` and `/boot`
  backing devices are recorded, and the GCE NIC and disk driver bindings are
  known.
- Rebuild the primary and alternate child `vmlinux` artifacts from the pinned
  commit. Record release strings, configs, Git state, and SHA-256 hashes.
- Capture the host boot ID and take a recovery snapshot before introducing
  device allocation.

Pass: the original two-child no-device initramfs smoke test still passes and
cleans up on the restored/rebuilt VM.

### Stage 1: source and topology feasibility gate

Before creating or formatting child filesystems:

- Inspect the exact pinned Kerf and Multikernel source paths that discover,
  describe, unbind, assign, and return devices. Determine whether `--devices`
  means a PCI function, an NVMe namespace, a SCSI target/LUN, or another unit
  for this host.
- Inspect `kerf init --help`, `kerf create --help`, dry-run/report output, and
  the generated/dumped device tree. Do not infer pinned behavior from current
  website examples.
- Attach at most one blank disposable secondary disk and capture `lsblk`,
  `/dev/disk/by-id`, `lspci -nnk`, `lsscsi`, `udevadm info`, relevant sysfs
  ancestry, `/proc/iomem`, IOMMU groups, driver binding, and host dmesg.
- Map the boot disk and secondary disk to controller, target, LUN/namespace,
  and Kerf resource names. Identify exactly what the kernel will remove from
  the primary and expose to the child.
- Confirm how a returned device is reprobed or rebound in the primary after a
  child exits, and how recovery works if the child crashes while owning it.
- Confirm that the child receives any required interrupt/MSI, DMA/IOMMU, PCI,
  and low-memory resources. The earlier no-device run logged SWIOTLB warnings,
  so a successful DMA path must be proven rather than assumed.

Pass: source plus live evidence prove the secondary disk is independently
assignable and returnable without transferring or disrupting the boot disk or
NIC. Otherwise stop without formatting it.

### Stage 2: child kernel and bootstrap storage support

For each child build, verify the live controller driver and all dependencies,
not merely generic ext4 support. Expected candidates on the N2/SCSI path
include PCI, Virtio PCI, Virtio SCSI, SCSI disk, block layer, partition/UUID
parsing, ext4, devtmpfs, initramfs, and MKTTY; the observed host driver decides
the exact list.

Prefer building the boot-critical storage stack and ext4 into each kernel.
Build a small bootstrap initramfs that:

1. mounts devtmpfs, procfs, and sysfs;
2. prints kernel release and enumerates block/controller devices;
3. waits for one expected filesystem UUID with a bounded timeout;
4. refuses unexpected or multiple matches;
5. runs a non-destructive ext4 check or reports why it was skipped;
6. mounts the expected filesystem read-write;
7. verifies the disk/root manifest and child identity marker; and
8. uses `switch_root` to start the disk's `/init`, retaining an MKTTY
   diagnostic shell on failure.

Pass: both artifacts contain the required built-in options or an audited
module closure, and their bootstrap initramfs can fail safely when no disk is
assigned.

### Stage 3: create, identify, format, and populate disks

- Create `child-a-root` and `child-b-root` as separate 10 GiB zonal
  `pd-balanced` resources with explicit attachment device names and auto-delete
  disabled. Record their resource URLs and attachment indices.
- Run the read-only identity audit and save its output before formatting.
- Format only a blank, positively identified whole-disk target:
  `ext4`, distinct labels such as `mk-child-a-root` and `mk-child-b-root`, and
  default modern ext4 features supported by both child kernels. Record the
  exact `mkfs.ext4` version and command.
- Mount each disk one at a time on a dedicated primary mountpoint. Populate a
  static BusyBox root with normal directories, `/init`, a child-specific
  marker, a fixed test corpus, and writable `/var` and `/root` directories.
- Generate a sorted manifest of paths, types, modes, sizes, and hashes. Record
  ext4 UUID, label, feature list, superblock metadata, and initial `fsck -fn`.
- Unmount, run `sync`, and prove neither filesystem is mounted or held open.

Pass: both unique UUIDs and manifests are recorded; the primary can mount,
validate, and cleanly unmount each disk independently; rerunning preparation
detects an existing filesystem and refuses to reformat it.

### Stage 4: one child with one persistent ext4 root

- Reconfirm that disk A is unmounted and the host is healthy.
- Initialize the smallest pool containing child A's CPUs, memory, and only the
  proven independently assignable disk-A resource. Inspect the dry run before
  applying it.
- Create child A, load kernel A plus its bootstrap initramfs, and pass only the
  expected root UUID and diagnostic console parameters.
- Start through MKTTY. Confirm kernel A's release, the expected disk identity,
  ext4 at `/`, PID 1 from the disk root, read-write operation, and a unique
  persistent counter/marker write followed by `sync`.
- Confirm the primary remains reachable and cannot mount or issue filesystem
  operations to disk A while the child owns it.
- Shut down cleanly, unload/delete the instance, return the device, and mount
  disk A on the primary to verify the marker and clean ext4 state.
- Repeat once from a clean pool, then force-stop once after a synced write and
  define whether the returned filesystem needs `fsck` before reuse.

Pass: clean and forced child lifecycle behavior is understood; data survives;
the disk returns; the boot disk, NIC, CPUs, and memory remain healthy.

### Stage 5: repeat independently for child B

Repeat Stage 4 with kernel B, initramfs B, disk B, different APIC IDs, and a
different root UUID. Do not reuse A's recorded device name or inferred Linux
block name.

Pass: the console reports the alternate kernel release and disk-B identity,
and no disk-A marker or UUID is visible.

### Stage 6: two concurrent kernels and two ext4 roots

- Begin from no instances, an empty pool, both disks returned and unmounted,
  and a passing host health report.
- Allocate the previously verified APIC split and memory with slack, plus both
  independently named devices in the pool.
- Assign disk A only to child A and disk B only to child B. Validate the
  effective device tree before loading either kernel.
- Boot child A, verify it, then boot child B. After sequential success, repeat
  with concurrent starts.
- In each child, record kernel release/hash marker, root UUID/label, ext4 mount
  options, block topology, `df`, and a child-specific monotonically increasing
  persistence marker. Run bounded reads/writes and `sync`.
- Prove cross-isolation: no other child root UUID, label, device node, marker,
  or filesystem is visible. Prove the host boot ID and management services are
  unchanged.
- Stop B and confirm A continues operating, then restart B and verify its
  persisted marker. Repeat in the opposite order.

Pass: both distinct child kernels and exclusively owned ext4 roots operate
concurrently without cross-visibility, corruption, or primary degradation.

### Stage 7: persistence and recovery matrix

Run separately, collecting evidence after every boundary:

1. clean child shutdown and reload;
2. forced child stop after a completed `sync`;
3. both children stopped, primary rebooted, devices rediscovered and children
   relaunched by UUID;
4. GCE VM stop/start, then the same rediscovery and relaunch;
5. one child boot attempted with its disk absent or intentionally not assigned;
6. one filesystem made dirty in a disposable run to prove the bootstrap's
   refusal/fsck policy; and
7. disk snapshot and clone mounted read-only on the primary to prove a
   recoverable backup path.

Do not automate filesystem repair until the observed failure modes are known.
An absent, ambiguous, wrong-UUID, or damaged root must lead to a bounded MKTTY
diagnostic failure, never formatting or mounting another disk.

### Stage 8: automation, documentation, and cleanup

After the manual one-child and two-child procedures each pass twice:

- Implement idempotent Makefile targets and scripts listed above.
- Add shell tests for every destructive-format refusal condition.
- Preserve raw commands, kernel and root manifests, cloud inventory, device
  tree dumps, controller topology, MKTTY transcripts, dmesg, filesystem checks,
  and cleanup proof under a timestamped evidence directory.
- Update `LEARNINGS.md`, `TASKS.md`, and `README.md` with observed results, not
  planned claims.
- Stop/delete all children and release the pool. Return disks to the primary,
  mount them read-only for final verification, unmount, and snapshot if they
  are to be retained.
- Explicitly choose whether to retain (ongoing charges), detach, snapshot and
  delete, or delete the two experimental disks. Normal cleanup must not make
  that choice automatically.

## Expected problems and required responses

| Risk | Detection | Response |
| --- | --- | --- |
| Boot and child disks share one assignable SCSI controller | Same PCI/sysfs parent; pinned Kerf identifies only the controller | Hard stop. Never assign it. Change topology or storage design. |
| Website and pinned Kerf device semantics differ | Pinned CLI/source lacks documented namespace/LUN behavior | Follow pinned behavior; propose a separately versioned upgrade experiment. |
| Device disappears from primary but not child | MKTTY enumeration plus host/child dmesg | Stop, collect logs, return the device, and fix interrupt/DMA/device-tree handoff before mounting. |
| Assigning a disk disrupts the primary boot disk or SSH | I/O errors, remount-ro, guest-agent/SSH failure | Use serial console, stop testing, restore from snapshot; do not retry the same allocation. |
| Wrong format target | Identity guard disagrees, existing signatures, root ancestry, unexpected size/serial | Refuse before `mkfs`; require manual investigation. |
| Preparation rerun would erase persistent data | `blkid` or `wipefs -n` finds ext4/signatures | Treat as already initialized and verify; never automatically reformat. |
| Child lacks the actual GCE storage driver | No expected block device in bounded wait | Rebuild from observed driver/config dependency closure; do not guess a new root device. |
| `/dev/sdX` ordering changes | UUID/by-id maps to a different ephemeral name | Expected; use UUID in child and by-id plus cloud identity in primary scripts. |
| ext4 is mounted by two kernels | Host mount remains during handoff or child sees another owned UUID | Abort allocation/start immediately; never test concurrent ordinary-ext4 writers. |
| Forced stop leaves a dirty filesystem | ext4 journal/fsck report on return | Retain evidence; use journal replay/fsck policy established in Stage 7 before remount. |
| Device cannot be returned/reprobed | Missing primary block device after child teardown | Collect state and use the documented recovery path; reboot only after evidence capture. |
| Different older kernel cannot consume the handoff | Early boot failure despite known-good disk and current child build | Keep the disk result separate; record this as a Multikernel ABI/backport failure. |

## Evidence required per run

- GCE project, zone, VM/machine type, image/snapshot source, disk resource URLs,
  attachment indices, attachment device names, sizes, types, and auto-delete
  settings.
- Linux/Kerf commits and dirty states; host and child releases/configs; kernel
  and initramfs hashes.
- Host root/boot/NIC identities; `lsblk`, by-id links, controller/LUN/namespace
  mapping, PCI drivers, IOMMU groups, sysfs ancestry, and Kerf device names.
- Pre-format refusal audit, exact format commands, `mkfs.ext4` version,
  UUIDs/labels/features, root manifests, and `fsck -fn` output.
- Kerf dry runs, pool/instance device-tree dumps, instance state, CPU/memory
  assignment, and device ownership before/during/after each child.
- Complete MKTTY output showing expected kernel release, expected UUID, ext4 at
  `/`, PID 1 source, persistence markers, and safe failure output.
- Primary health and dmesg while each and both children run.
- Persistence marker values across child restart, primary reboot, and GCE
  stop/start.
- Final proof of no instances, empty pool, all expected host CPUs/memory,
  returned and unmounted disks, clean filesystem checks, guest agent/NIC/SSH
  health, and explicit disk retention/deletion disposition.

## Later compatibility experiment: genuinely older kernels

Only after the two pinned-`v7.0-mk2` child builds pass the full disk matrix,
replace one child kernel at a time with an older target. An arbitrary stock
kernel is not sufficient; the target must contain or receive compatible
Multikernel startup, resource, device-tree, kexec, and MKTTY changes.

For each target, record the upstream base version, Multikernel patch source,
configuration, local version, toolchain, storage driver, initramfs modules,
and whether the host/child handoff ABI is expected to match `v7.0-mk2`. Keep
the already proven disk image and hardware assignment fixed so a failure can
be attributed to kernel compatibility rather than filesystem preparation.

## References

- Existing no-device runbook: [`README.md`](README.md)
- Existing DAXFS experiment: [`DAXFS-PLAN.md`](DAXFS-PLAN.md)
- Pinned Kerf source: <https://github.com/multikernel/kerf/tree/8b72b3e9b266f8d32e707e2c1743ad7afc50b1ec>
- GCE Persistent Disk interfaces: <https://docs.cloud.google.com/compute/docs/disks/persistent-disks>
- GCE persistent device names: <https://docs.cloud.google.com/compute/docs/disks/set-persistent-device-name-in-linux-vm>
- GCE format/mount guidance: <https://docs.cloud.google.com/compute/docs/disks/format-mount-disk-linux>
