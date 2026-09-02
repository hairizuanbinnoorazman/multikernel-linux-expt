# Implementation tasks

Last updated: 2026-09-01

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

- [x] Record every material observation and correction in the
  [field notes](../experiments/field-notes-20260828.md).
- [x] Add scripts only where they make a verified manual procedure repeatable.
- [x] Create a root Makefile for VM creation, provisioning, verification,
  Multikernel smoke tests, log collection, and safe cleanup.
- [x] Update the [GCE runbook](../guides/gce-lab-runbook.md) where live results
  differ from the research plan.

## DAXFS and Docker-image experiment

- [x] Audit the complete [DAXFS plan](../experiments/daxfs/plan.md), including
  every later experiment.
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
  writable-coherence diagnosis in the
  [DAXFS report](../experiments/daxfs/report.md).
- [x] Remove the final child and pool, capture the clean state, then delete the
  GCE VM and auto-delete boot disk while retaining recovery snapshots and
  local evidence.
- [ ] Automate reconstruction of the alternate kernel, matching DAXFS module,
  and initramfs; the current dual-kernel target expects deleted remote build
  artifacts.
- [ ] Validate creation of a fresh disk/VM from each retained snapshot before
  treating either snapshot as a tested recovery workflow.

## Persistent ext4 child-root experiment

- [x] Review the [direct ext4 plan](../experiments/ext4-direct/plan.md) and
  retain its controller-granularity hard gate.
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
- [x] Record results and resource disposition in the
  [direct ext4 learnings](../experiments/ext4-direct/learnings.md) and
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
- [x] Add the full continuous
  [execution matrix](../experiments/ext4-direct/execution.md).
- [x] Verify final host health and blank disk state, then delete both probe VMs,
  all boot disks, and both disposable child disks; retain only recovery
  snapshots and local evidence.
- [x] Select and implement the primary-mediated block transport without
  handing a GCE storage controller to a child.
- [x] Prove one child mounts its image-backed ext4 filesystem as read-write `/`
  and retains a counter across recreation, primary reset, and GCE stop/start.
- [x] Prove two distinct kernels concurrently mount isolated ext4 roots with no
  storage device in either Kerf device tree.
- [x] Capture request/flush counters, partial-write disconnect behavior, final
  filesystem checks, and stopped-VM resource disposition.
- [ ] Implement a clean child-driven remount/disconnect/shutdown path.
- [ ] Complete mediated ENOSPC, malformed-packet, server-failure, damaged-image,
  stale-lock, sustained-load, and offline snapshot/clone recovery tests.

## Multikernel container runtime

The detailed exit criteria and ordering are in
[`../runtime/plans/README.md`](../runtime/plans/README.md).

- [x] Record the decision to build a new runtime around Kerf rather than fork
  Firecracker.
- [x] Define the target pod-sandbox architecture and component boundaries.
- [x] Create gated plans for host qualification, control plane, agent, storage,
  networking, containerd, Kubernetes, security, reliability, performance, and
  release.
- [x] Add a non-code runtime workspace that prevents accidental coupling
  between the shim, Kerf adapter, agent, and device services.
- [x] G0: freeze the lifecycle, protocol, configuration, error, threat-model,
  and evidence contracts.
- [x] G1: implement and pass read-only host qualification and isolation checks.
- [x] G2: implement the recoverable `mkruntimed` daemon and Kerf adapter.
- [x] G3: implement `mk-agent` and the minimal OCI process lifecycle in one child.
- [ ] Close the
  [G0-G3 remediation and revalidation checklist](../runtime/learnings/g0-g3-remediation-checklist.md)
  before treating the four historical milestone passes as full conformance to
  their plans and frozen contracts.
- [ ] G4: provide deterministic OCI images and safe single-owner storage.
  - [x] MVP: consume containerd/Docker-prepared BusyBox roots without mutating
    their snapshots, create a private per-sandbox initramfs, and prove cleanup.
- [ ] G5: provide primary-mediated CNI-compatible networking.
  - [x] MVP: provide isolated static `/30` TUN links, primary NAT, DNS and
    outbound HTTP without assigning the GCE NIC to a child.
  - [ ] Add normal CNI `ADD`, `CHECK`, and `DEL` operations.
- [ ] G6: pass containerd Runtime v2 lifecycle tests through `ctr`.
  - [x] MVP: pass concurrent `ctr` and Docker create/start/exec/signal/wait/
    delete with distinct child-kernel boot IDs and leak-free teardown.
  - [x] Preserve the running child and boot identity across containerd and
    `mkruntimed` restarts.
  - [x] Reclaim the child, TUN, and iptables state after forced shim death.
  - [x] Pass and retain a command-by-command shared `ctr`/Docker feature
    matrix covering every currently supported lifecycle, root, network,
    restart, name-reuse, and cleanup path.
  - [ ] Preserve/reconnect the running task after forced shim death.
  - [x] Implement child PTY terminal mode and Task v2 terminal resize, including
    resize requests received before process start.
  - [x] Implement and live-test guest stdin, `CloseIO`, and detach/reattach
    through both `ctr` and Docker.
  - [x] Revalidate terminal mode and resize through `ctr` and Docker on a
    disposable qualified Multikernel host.
- [ ] Close the
  [G4-G6 remediation and live-evidence checklist](../runtime/learnings/g4-g6-remediation-checklist.md)
  before treating the checked MVP and feature rows as full G4, G5, or G6 gate
  conformance.
- [ ] G7: run a multi-container Kubernetes pod through `RuntimeClass`.
- [ ] G8: pass the security, fuzzing, failure, and resource-leak matrix.
- [ ] G9: publish reproducible performance and density comparisons.
- [ ] G10: produce and validate the opt-in developer-preview release.

## Safety constraints

- Keep the original Ubuntu kernel installed and available through GRUB.
- Enable GCE serial-port output before installing the custom kernel.
- Never allocate APIC ID 0 to a spawned kernel.
- Leave at least four vCPUs and ample memory with the primary kernel.
- Do not assign the GCE NIC, boot disk, or other PCI devices to children.
- Do not enable Secure Boot until the unsigned custom-kernel path is proven.
- Capture logs before deleting or rebuilding a failed VM.
