# Implementation tasks

Last updated: 2026-08-28

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

## Safety constraints

- Keep the original Ubuntu kernel installed and available through GRUB.
- Enable GCE serial-port output before installing the custom kernel.
- Never allocate APIC ID 0 to a spawned kernel.
- Leave at least four vCPUs and ample memory with the primary kernel.
- Do not assign the GCE NIC, boot disk, or other PCI devices to children.
- Do not enable Secure Boot until the unsigned custom-kernel path is proven.
- Capture logs before deleting or rebuilding a failed VM.
