# Implementation tasks

Last updated: 2026-08-29

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

## Safety constraints

- Keep the original Ubuntu kernel installed and available through GRUB.
- Enable GCE serial-port output before installing the custom kernel.
- Never allocate APIC ID 0 to a spawned kernel.
- Leave at least four vCPUs and ample memory with the primary kernel.
- Do not assign the GCE NIC, boot disk, or other PCI devices to children.
- Do not enable Secure Boot until the unsigned custom-kernel path is proven.
- Capture logs before deleting or rebuilding a failed VM.
