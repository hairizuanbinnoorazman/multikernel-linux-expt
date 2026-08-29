# DAXFS child-root experiment plan

Last updated: 2026-08-29

## Execution status

Every stage and later experiment category was exercised on the live GCE
laboratory VM on 2026-08-28; testing did not stop after the single-child
portion. All functional tests completed. A subsequent documentation audit
found and remediated a
self-referential checksum manifest, then proved every supported root file in a
fresh child. One planned comparison remains partial: the performance run used
host tmpfs as the memory-filesystem baseline rather than directly measuring
the existing child initramfs. The staged text below is retained as the original
safety procedure and acceptance criteria.

After all live work and the manifest remediation proof, the clean host state
was captured and the GCE VM plus its auto-delete boot disk were deleted. The
two recovery snapshots and local evidence remain. Post-snapshot DAXFS and
alternate-kernel artifacts do not remain as directly usable files.

The complete implementation, pass/fail matrix, exact hashes, discovered
limitations, compatibility patches, and evidence index are in
[`DAXFS-IMPLEMENTATION.md`](DAXFS-IMPLEMENTATION.md). The headline findings
are:

- Two Docker-derived DAXFS-root workloads ran concurrently with distinct
  kernel binaries/releases: `7.0.0-mk2-gce-lab` and
  `7.0.0-mk2-gce-lab-alt`.
- This uses Docker as the image/rootfs input to Kerf; it does not change the
  fact that ordinary Docker containers share their Docker host's kernel.
- Single-child root boot, two clean repetitions, Docker import, shared
  read-only mounts, restart, corruption rejection, exhaustion, and cleanup
  passed.
- Simultaneous shared-write coherence did **not** pass. Negative dentry and
  contended-write observations diverged between kernels. Shared DAXFS should
  remain read-only or single-writer at this revision.

## Objective

Extend the verified Multikernel smoke test by booting one child kernel with a
small DAXFS-backed root filesystem, while keeping the GCE boot disk, NIC, and
all other devices owned by the primary kernel.

This is a shared-memory filesystem experiment, not a persistent-disk test.
DAXFS operates on byte-addressable memory and does not turn the GCE Persistent
Disk into a child disk. Persistence across VM stop, reboot, or DAXFS-memory
release is out of scope.

The experiment is successful only if all of the following are observed:

- The primary remains reachable over SSH and its guest agent, boot disk, NIC,
  metadata access, and serial output remain healthy.
- One child boots with the expected CPUs and memory without an assigned device.
- The child mounts DAXFS and reads a known directory tree and marker file.
- In the root-filesystem phase, PID 1 and its executable come from DAXFS rather
  than the bootstrap initramfs.
- Stopping and deleting the child releases its CPUs, child memory, and any
  DAXFS/shared-memory allocation without rebooting the primary.
- The test can be repeated from a clean pool with the same result.

## Fixed inputs and boundaries

Use the versions already selected by this repository:

| Component | Revision |
| --- | --- |
| Multikernel Linux | `v7.0-mk2` / `3bdd35b64413da0b4e089ce931bfc2e8b031cbf7` |
| Kerf | `v0.2.0` / `8b72b3e9b266f8d32e707e2c1743ad7afc50b1ec` |
| DAXFS | `11ab401585b79b4a7c9164019852e0219e197d13` |
| Host kernel release | `7.0.0-mk2-gce-lab` |

Do not silently move any component to its default branch to resolve an
incompatibility. Record the failure first, then decide whether to patch the
pinned revisions or run a separately identified version-upgrade experiment.

The initial DAXFS test must not include:

- Docker image extraction.
- Two children sharing or writing the same filesystem.
- Networking inside the child.
- GCE disk, NIC, PCI, or platform-device assignment.
- Performance claims or benchmarks.
- Persistence claims.

## Safety and recovery rules

1. Start with no child instances, no Multikernel pool, and all 16 host vCPUs
   online. Run `make verify-host` and retain its output.
2. Create a new GCE boot-disk snapshot before installing DAXFS or replacing a
   child initramfs. Keep the existing stock-kernel recovery entry in GRUB.
3. Continue to reserve physical APIC IDs, never Linux logical CPU numbers, and
   never allocate APIC ID 0.
4. Use one child with four vCPUs and a conservative memory allocation. Leave at
   least four vCPUs and ample memory with the primary.
5. Set `--devices=none` on pool initialization and omit `--devices` from
   `kerf create`; pinned Kerf interprets a per-instance value of `none` as a
   literal device name and rejects it. In particular, never give the child the
   GCE boot disk controller or `ens4`.
6. Keep the bootstrap initramfs and MKTTY console functional until DAXFS has
   mounted successfully. A failed root switch must fall back to a diagnostic
   shell rather than panic-loop silently.
7. Collect host dmesg, Kerf state, child MKTTY output, DAXFS inspection output,
   CPU state, memory state, and serial output before cleanup after any failure.
8. Stop the child before unloading DAXFS or releasing its memory. If normal
   teardown fails, collect logs and reboot into the known-good custom or stock
   kernel instead of forcing an uncertain memory reuse.

## Experiment stages

Each stage introduces one new variable. Do not continue after an unexplained
failure.

### Stage 0: compatibility and baseline gate

Before building or changing the live pool:

- Confirm the VM is on `7.0.0-mk2-gce-lab` and the current boot ID is recorded.
- Confirm `CONFIG_FS_DAX` in the installed and build-tree configurations.
- Confirm the DAXFS revision's kernel-module build uses the exact Multikernel
  source/build tree that produced the running kernel, including its
  `Module.symvers` and generated headers.
- Inspect `kerf load --help` and the pinned Kerf source. Confirm which of
  `--image`, `--rootfs-dir`, or a lower-level physical-address interface is
  actually implemented in `v0.2.0`; current website examples are not evidence
  that a pinned older CLI supports the same flags.
- Confirm the Multikernel DMA heap or other DAXFS backing interface exists on
  this build. Do not assume a normal GCE Persistent Disk is DAX-capable.
- Record relevant device nodes, `/proc/iomem`, loaded modules, DAXFS module
  parameters, and Kerf state.

Stop here if `CONFIG_FS_DAX`, the Multikernel memory-backing interface, or a
workable pinned Kerf handoff is absent. That result determines whether the
next task is a kernel rebuild, a small integration script, or a version change.

### Stage 1: build and host-only module proof

Clone DAXFS at the exact pinned commit and record the resolved commit. Build
the module and userspace tools against the running Multikernel kernel.

Verify before loading:

- `modinfo` reports a vermagic compatible with the running kernel.
- All module dependencies resolve.
- The image-building and inspection tools execute and report their versions or
  help successfully.
- Secure Boot remains disabled and lockdown does not prevent loading the
  unsigned out-of-tree module.

Load the module on the primary and confirm that its filesystem and expected
memory-backing interfaces register. If supported by the pinned revision,
create and mount a tiny DAXFS image on the primary, validate it, compare its
contents to the source tree, unmount it, and unload the module. This is only a
module/image-format check; it is not the child proof.

### Stage 2: deterministic root tree

Build a small root tree under a dedicated artifact directory. It should
contain only what is needed for the proof:

- Static BusyBox and its required applet links.
- `/init` plus `/bin`, `/dev`, `/proc`, `/sys`, `/run`, `/tmp`, and `/mnt`.
- A marker file containing the DAXFS commit, build timestamp, and a fixed test
  string.
- A small set of regular files, nested directories, an empty file, and a file
  with known permissions for integrity checks.

Generate and retain a sorted manifest containing paths, file types, modes,
sizes, and SHA-256 hashes. DAXFS does not support special nodes such as device
nodes, FIFOs, or sockets, so `/dev` must be an ordinary empty directory and
the child must mount devtmpfs after boot.

Create the smallest practical static, read-only DAXFS image first. Record its
size, DAXFS format/version information, and SHA-256 hash. Writable overlays and
split/backing-file mode come later.

### Stage 3: child mount proof without changing root

Adapt the known-good bootstrap initramfs rather than replacing it outright.
Include the DAXFS module and any required modules, then have `/init`:

1. Mount procfs, sysfs, devtmpfs, and tmpfs as before.
2. Load DAXFS and print module/mount diagnostics to MKTTY.
3. Mount the provided DAXFS region at `/mnt/daxfs` with validation enabled when
   supported by the pinned revision.
4. Read and hash the marker and test files.
5. Print an unambiguous `DAXFS_MOUNT_READY` marker.
6. Remain in the diagnostic initramfs so teardown can be tested safely.

Use the already verified one-child CPU/memory allocation pattern and no
devices. The exact physical address, size, and kernel command-line parameters
must come from the observed Kerf/DAXFS handoff; do not hard-code an address
copied from documentation.

Pass criteria:

- DAXFS appears in the child's `/proc/filesystems` and `/proc/mounts`.
- The child manifest matches the host-generated manifest for all supported
  entries.
- Repeated reads return identical hashes.
- The primary remains healthy throughout.
- Normal child teardown and full pool return succeed.

### Stage 4: DAXFS as the child root filesystem

Only after Stage 3 passes, change `/init` to mount DAXFS and use
`switch_root` into it. The DAXFS root's `/init` should remount the necessary
pseudo-filesystems, print identity and mount information, re-check the marker,
and emit `DAXFS_ROOT_READY`.

Evidence must distinguish a real root switch from merely mounting DAXFS:

- `/proc/1/exe` resolves to the DAXFS root's init/BusyBox.
- `/proc/mounts` or `/proc/self/mountinfo` identifies DAXFS at `/`.
- The initramfs-only marker is absent after the switch, while the DAXFS marker
  is present.
- The child can execute at least one BusyBox applet from the DAXFS root.

Run at least two complete create/load/execute/kill/unload/delete cycles, with a
clean pool between runs.

### Stage 5: limited filesystem behavior

Keep this separate from the root-boot milestone. Test the pinned revision's
documented static/read-only behavior first:

- Regular and empty files, nested directories, metadata, and large sequential
  reads.
- Expected rejection of writes to a read-only image.
- Mount-time validation behavior for a deliberately corrupted copy of the
  image. Never mutate the only known-good image.

If the pinned revision supports a writable overlay, run it as a separate
subtest with bounded space. Verify create, overwrite, append, truncate, rename,
and delete behavior, then inspect overlay utilization and exhaustion handling.
Do not call this persistent storage: overlay contents may disappear when its
shared-memory allocation is released or the VM reboots.

### Stage 6: automation and documentation

After the manual procedure passes twice:

- Add idempotent build, start, status, log-collection, and cleanup scripts.
- Add Makefile targets such as `daxfs-build`, `daxfs-up`, `daxfs-status`, and
  `daxfs-down` only after their underlying commands are verified.
- Make `daxfs-down` tolerate partially created instances and always report
  remaining instances, pool memory, and offline CPUs.
- Record exact commands, hashes, resource assignments, observed addresses,
  output markers, failures, and corrections in `LEARNINGS.md`.
- Update `TASKS.md` only with tests that were actually completed.

## Expected problems and responses

| Risk or likely failure | Detection | Response |
| --- | --- | --- |
| `CONFIG_FS_DAX` is absent or modular dependencies are missing | Config check, build error, or unresolved symbols | Rebuild the pinned kernel with the minimum required options; retain the existing working kernel as a GRUB entry. |
| DAXFS module was built against the wrong kernel tree | Vermagic mismatch, unknown symbols, or load rejection | Build against the exact source/output tree for `7.0.0-mk2-gce-lab`; do not force-load it. |
| Pinned Kerf lacks current `--image`/`--rootfs-dir` behavior | CLI/source audit fails | Implement and document the smallest lower-level handoff supported by the pinned versions, or stop and propose an explicit version upgrade. |
| GCE exposes no conventional DAX device | No `/dev/pmem*` or DAX block device | Use only the Multikernel shared-memory/DMA-heap path supported by DAXFS; do not repurpose the boot disk or assume it is DAX. |
| DAXFS cannot build against Linux 7.0 APIs | Compile-time API errors | Record the incompatibility. Review a minimal compatibility patch separately; do not silently alter the pinned source. |
| Child cannot load `daxfs.ko` | Missing dependency, module format error, or initramfs omission | Include the module plus dependency closure in the bootstrap initramfs and print `modprobe`/`dmesg` failures to MKTTY. |
| Root switch fails | PID 1 remains in initramfs or child panics | Keep a diagnostic fallback shell, log mount options and addresses, and fix Stage 3 before retrying Stage 4. |
| `/dev` or other runtime paths are missing | Init/app failures after root switch | Create ordinary mountpoints in the image and mount devtmpfs, procfs, sysfs, and tmpfs at boot; do not embed unsupported device nodes. |
| Image format or tool/module versions disagree | Validation or mount rejects the superblock | Build `mkdaxfs`, inspector, and module from the same pinned commit and record the format version. |
| DAXFS allocation consumes child memory unexpectedly | Lower `MemTotal`, allocation failure, or overlap report | Measure before/after ranges and sizes; leave explicit pool slack and reduce the child/image allocation. |
| Stale physical address is reused | Mount corruption, wrong marker, or kernel fault | Derive address/size for every load, bind them to the instance transaction, and never reuse values after teardown without revalidation. |
| Kerf reports its known post-create `KeyError` | Nonzero exit after the overlay was applied | Check the live instance directory and fresh `kerf show` before deciding whether to retry. Never blindly create the same instance twice. |
| Kerf `--report` leaves the pool unchanged | Report succeeds but live pool is empty | Use `--report` only for inspection; apply again without it, as already documented. |
| Per-instance `--devices=none` is rejected | Validation says the instance references nonexistent device `none` | Put `--devices=none` on `kerf init`; omit the instance option. An omitted instance device list is empty. |
| Overlay fills or fixed hash table saturates | ENOSPC/write errors and inspector utilization | Use bounded writes, capture inspection output, and recreate with a deliberately larger overlay/bucket count. |
| Host SSH or guest agent degrades | Failed health probe or metadata request | Stop the test, use GCE serial output, collect logs, and do not assign additional resources. |
| Child teardown leaves CPUs or memory reserved | Offline CPUs, remaining instance paths, or nonempty pool | Collect evidence before attempting recovery; use the verified cleanup order and reboot only if normal teardown cannot restore a known state. |
| Test is mistaken for durable storage | Data disappears after memory release or reboot | Treat this as expected unless a separate persistence mechanism is deliberately designed and tested. |

## Evidence to retain per run

Store each run under a timestamped directory and include:

- Git commit IDs and dirty-state reports for Linux, Kerf, and DAXFS.
- Host kernel release, boot ID, config checks, module metadata, and module list.
- Root-tree manifest plus DAXFS image size, format/version, and SHA-256.
- Kerf dry-run output, applied pool, instance description, assigned physical
  APIC IDs, memory ranges, and DAXFS address/size handoff.
- Full MKTTY child console from boot through its ready marker.
- Host health checks while the child is active.
- Host and child dmesg relevant to Multikernel, DAX, DAXFS, DMA heap, kexec,
  memory allocation, and CPU hotplug.
- DAXFS inspection output before and after filesystem operations.
- Cleanup proof: no instances, empty pool, host CPUs `0-15` online, expected
  host memory restored, and guest agent/NIC/metadata healthy.
- GCE serial output for failures or any host reachability incident.

## Later experiments (all functionally executed)

After the single-child read-only DAXFS root passed reliably, these were run in
the listed order:

1. Bounded writable-overlay behavior — passed for one mount.
2. A Docker/rootfs-directory workflow through the exact Kerf interface —
   passed after the two recorded Kerf compatibility patches.
3. Two children mounting the same read-only DAXFS image — passed.
4. Two-child shared-write and coherence tests with conflict-free files first,
   then deliberately contended operations — executed and exposed a coherence
   failure; this is a negative result, not an omitted test.
5. Crash/restart and exhaustion behavior — passed within the lifetime of the
   shared-memory allocation; exhaustion returned `ENOSPC` as expected.
6. Performance comparison — partially completed as a bounded host ext4,
   tmpfs, and DAXFS cached-read microbenchmark. The specifically named existing
   child-initramfs baseline was not directly measured; no general performance
   claim is made.
7. Two simultaneous Docker-derived workloads with separate kernel binaries —
   passed and is the decisive per-workload-kernel capability proof.

Networking, a host-side proxy, and virtual-device assignment remain separate
experiments. DAXFS success does not make assigning the GCE boot disk or NIC to
a child safe.

## Source notes

- The repository's existing bring-up and recovery record is in `README.md`,
  `LEARNINGS.md`, and `TASKS.md`.
- Pinned DAXFS source and documentation:
  <https://github.com/multikernel/daxfs/tree/11ab401585b79b4a7c9164019852e0219e197d13>
- Pinned Kerf source:
  <https://github.com/multikernel/kerf/tree/8b72b3e9b266f8d32e707e2c1743ad7afc50b1ec>
