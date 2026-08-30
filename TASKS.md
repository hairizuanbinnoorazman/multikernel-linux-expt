# Implementation tasks

Last updated: 2026-08-30

## Live GCE proof

- [x] Confirm that `mklinux-lab` does not already exist.
- [x] Create an Ubuntu 26.04 `n2-standard-16` VM in
  `asia-southeast1-b` with Secure Boot disabled and serial output enabled.
- [x] Confirm SSH, guest-agent, disk, network, CPU topology, and serial output.
- [x] Install kernel and Kerf build dependencies.
- [x] Clone Multikernel Linux at exact tag `v7.0-mk2`.
- [x] Configure Multikernel, MKTTY, kexec, GCE guest, and initramfs support.
- [x] Build and install the kernel while retaining the stock Ubuntu kernel.
- [x] Reboot and prove the running primary kernel is `v7.0-mk2`.
- [x] Prove the Google guest agent, network, disk, and serial console still work.
- [x] Install Kerf at exact tag `v0.2.0`.
- [x] Build a static minimal child initramfs.
- [x] Map logical vCPUs to physical APIC IDs.
- [x] Dry-run, then create a CPU and memory resource pool without devices.
- [x] Boot child kernel `smoke-a` and record its CPU and memory view.
- [x] Boot child kernel `smoke-b` concurrently and record its resource view.
- [x] Prove the primary kernel remains reachable while both children run.
- [x] Stop both children and prove their resources return to the primary kernel.

## Reproducibility

- [x] Record every material observation and correction in `LEARNINGS.md`.
- [x] Add scripts only where they make a verified manual procedure repeatable.
- [x] Create a root Makefile for VM creation, provisioning, verification,
  Multikernel smoke tests, log collection, and safe cleanup.
- [x] Update `README.md` where live results differ from the research plan.

## DAXFS and Docker-image experiment

- [x] Audit all of `DAXFS-PLAN.md`, including every later experiment.
- [x] Create and verify the pre-DAXFS recovery snapshot.
- [x] Confirm the exact host kernel config, Multikernel DMA heap, DAXFS/Kerf
  interfaces, revisions, module vermagic, Secure Boot, and lockdown state.
- [x] Build pinned DAXFS and pass its full 20-test upstream host suite.
- [x] Build and hash the minimal root, manifest, bootstrap initramfs, and static
  format-8 DAXFS image.
- [x] Audit the checksum manifest, fix its self-reference, and prove every
  listed supported file from inside a fresh DAXFS-root child.
- [x] Prove a child can mount DAXFS without changing root.
- [x] Prove DAXFS as `/` in two complete clean lifecycle cycles.
- [x] Exercise normal overlay mutations and read-only rejection.
- [x] Prove mount validation rejects a deliberately corrupted image copy.
- [x] Prove bounded overlay exhaustion returns `ENOSPC` and inspect utilization.
- [x] Boot a Docker-derived BusyBox filesystem as a DAXFS child root.
- [x] Record and implement the pinned-Kerf special-file and hardlink fixes
  required for real OCI root filesystems.
- [x] Prove two children can mount the same DAXFS image read-only.
- [x] Execute two-child conflict-free and contended shared-write tests; record
  the observed coherence failure rather than claiming a pass.
- [x] Force-stop/restart a child and prove data remains while its DAXFS
  allocation remains live.
- [x] Run and retain the bounded host ext4/tmpfs/DAXFS cached-read
  microbenchmark.
- [ ] Directly compare performance with the existing child-initramfs baseline;
  the retained host tmpfs measurement is a documented partial substitute.
- [x] Build a distinct alternate kernel and matching DAXFS module/initramfs.
- [x] Run two Docker-derived roots concurrently under two distinct kernel
  binaries/releases and retain both console transcripts and Kerf state.
- [x] Reconfirm the primary boot ID and services remain healthy while both run.
- [x] Return all CPUs/memory, leaving no pool or instances.
- [x] Add and live-test `daxfs-build`, `daxfs-up`, `daxfs-status`,
  `daxfs-down`, and the dual-kernel proof target.
- [x] Download the complete GCE evidence bundle and add an evidence index.
- [x] Document the exact result, limitations, failures, fixes, and source-based
  writable-coherence diagnosis in `DAXFS-IMPLEMENTATION.md`.
- [x] Remove the final child and pool, capture the clean state, then delete the
  GCE VM and auto-delete boot disk while retaining recovery snapshots and
  local evidence.
- [ ] Automate reconstruction of the alternate kernel, matching DAXFS module,
  and initramfs; the current dual-kernel target expects deleted remote build
  artifacts.
- [ ] Validate creation of a fresh disk/VM from each retained snapshot before
  treating either snapshot as a tested recovery workflow.

## Persistent ext4 child-root experiment

- [x] Review `EXT4-DISK-PLAN.md` and retain its controller-granularity hard
  gate.
- [x] Restore `mklinux-lab-pre-daxfs-20260828-2030` into a fresh boot disk and
  `n2-standard-16` VM.
- [x] Verify the restored custom kernel, vCPUs, memory, root filesystem, guest
  agent, Kerf installation, and exact source commits.
- [x] Create two blank 10 GiB `pd-balanced` child disks with retention enabled.
- [x] Attach only child A and positively map its Google by-id name, serial,
  size, SCSI target, sysfs ancestry, PCI function, and signatures.
- [x] Inspect pinned Kerf and Multikernel device allocation semantics and run a
  non-applying Kerf PCI-device report.
- [x] Stop at Stage 1 after proving the child and boot disk share allocatable
  PCI function `0000:00:03.0`.
- [x] Add and live-test idempotent cloud provisioning and the read-only
  topology gate.
- [x] Record results and resource disposition in `EXT4-DISK-LEARNINGS.md` and
  `evidence/ext4-disk-20260830/README.md`.
- [x] Run the restored two-child no-device regression and return all resources.
- [x] Probe C3/NVMe and record that both namespaces still share one allocatable
  PCI function; delete the temporary probe VM and boot disk.
- [x] Rebuild and boot `7.0.0-mk2-gce-lab-alt` from a detached pinned
  worktree without disturbing the primary artifact.
- [x] Implement and live-test bounded absent-root bootstrap behavior on both
  kernels.
- [x] Run both distinct kernels concurrently with separate absent UUIDs and
  verify isolation, safe refusal, host health, and cleanup.
- [x] Add the full continuous execution matrix in `EXT4-DISK-EXECUTION.md`.
- [x] Verify final host health and blank disk state, then delete both probe VMs,
  all boot disks, and both disposable child disks; retain only recovery
  snapshots and local evidence.
- [ ] Select and separately approve an alternate storage topology or primary-
  mediated block transport before formatting or handing off either disk.

## Safety constraints

- Keep the original Ubuntu kernel installed and available through GRUB.
- Enable GCE serial-port output before installing the custom kernel.
- Never allocate APIC ID 0 to a spawned kernel.
- Leave at least four vCPUs and ample memory with the primary kernel.
- Do not assign the GCE NIC, boot disk, or other PCI devices to children.
- Do not enable Secure Boot until the unsigned custom-kernel path is proven.
- Capture logs before deleting or rebuilding a failed VM.
