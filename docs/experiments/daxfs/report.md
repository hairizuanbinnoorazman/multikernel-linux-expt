# DAXFS implementation and full GCE test report

Tested: 2026-08-28  
Infrastructure cleanup verified: 2026-08-29  
Environment: `mklinux-lab`, `n2-standard-16`, `asia-southeast1-b`  
Evidence: [`evidence/daxfs-20260828/`](../../../evidence/daxfs-20260828/)

## Result

Every functional stage and later functional experiment in the
[DAXFS plan](plan.md) was
run on the live GCE VM. The planned performance category was also exercised,
but its direct child-initramfs comparison remains a documented partial
substitution. The central result is:

> **Confirmed:** two Docker-derived filesystems ran concurrently as DAXFS
> roots under two different child kernel binaries in one GCE VM.

This is not a change to conventional Docker semantics. A normal Docker
container shares its Docker host's kernel. In this implementation, Kerf uses
Docker as an OCI image acquisition and root-filesystem format, serializes that
root into DAXFS, and launches it as a Multikernel child. The independently
selected child `vmlinux` supplies the kernel. The proof therefore supports
"different kernel per Docker-derived workload," not "different kernel per
ordinary `runc` container."

Two limitations were also established rather than hidden:

- The pinned DAXFS revision does **not** provide reliable simultaneous
  multi-kernel writable coherence. A negative lookup in one child remained
  stale after its peer created the name, and two writers observed divergent
  versions of a contended file.
- DAXFS storage here is shared memory. Data survived a child restart while the
  allocation remained live, but durability across pool release, host reboot,
  VM stop, or host failure was neither expected nor claimed.

## Exact versions and recovery point

| Component | Tested revision/version |
| --- | --- |
| Multikernel Linux | `3bdd35b64413da0b4e089ce931bfc2e8b031cbf7` (`v7.0-mk2`) |
| Primary kernel | `7.0.0-mk2-gce-lab` |
| Alternate child kernel | `7.0.0-mk2-gce-lab-alt` |
| Kerf | `8b72b3e9b266f8d32e707e2c1743ad7afc50b1ec` (`v0.2.0`) plus the two explicit compatibility patches below |
| DAXFS | `0.1.0`, commit `11ab401585b79b4a7c9164019852e0219e197d13` |
| DAXFS on-disk formats | superblock 8, overlay 2, page cache 2 (the pinned README still says superblock 7) |
| Docker Engine | `29.1.3` |
| fio | `3.41` |

Before DAXFS installation, the 100 GB boot disk was snapshotted as
`mklinux-lab-pre-daxfs-20260828-2030`. GCE reports the snapshot as `READY`.
An earlier stock checkpoint, `mklinux-lab-stock-20260828`, was also `READY` at
the final check. At the time of testing, the stock Ubuntu kernel remained
installed as a GRUB recovery entry.

After the final audit run, `daxfs-demo` was halted, unloaded, and deleted; the
12 GB pool was returned; CPUs `0-15` were online; and Kerf showed no pool,
instances, or `/proc/kimage` entries. The final state is retained in
[`pre-stop-clean-state.txt`](../../../evidence/daxfs-20260828/pre-stop-clean-state.txt).
The VM was first stopped and then permanently deleted with its auto-delete
100 GB boot disk after the evidence was copied. GCE showed no matching VM or
disk afterward. Both snapshots and the local repository/evidence bundle were
retained; snapshot storage charges may continue. The active remote source
trees and post-snapshot DAXFS/alternate-kernel artifacts are no longer
directly available and must be rebuilt for a fresh run. The pre-DAXFS snapshot
may contain the earlier primary-kernel/Kerf workspace, but restoration and its
contents were not live-tested. The post-deletion inventory is retained in
[`resource-cleanup.txt`](../../../evidence/daxfs-20260828/resource-cleanup.txt).

## Full test matrix

| Plan item | Result | Proof or observation |
| --- | --- | --- |
| Compatibility and baseline | Pass | Host boot ID `483ddd87-fef5-440e-bf8d-5499a77938d9`; 16 CPUs; about 62 GiB; guest agent, NIC, disk, metadata, and SSH healthy. `CONFIG_FS_DAX`, `CONFIG_DAX`, `CONFIG_ZONE_DEVICE`, `CONFIG_MULTIKERNEL`, `CONFIG_MKTTY`, and `CONFIG_KEXEC_FILE` enabled. `/dev/dma_heap/multikernel` present; no `/dev/pmem*` expected. |
| Exact module/tool build | Pass | DAXFS module vermagic is `7.0.0-mk2-gce-lab SMP preempt mod_unload modversions`; SHA-256 `8c2dc0c4a5218de1a60f9765c770c280957d69d72537b7b680d3ecf2b6566de4`. Secure Boot and lockdown did not block it; the unsigned out-of-tree module produced the expected taint. |
| Upstream host suite | Pass | All 20 static, split/page-cache, overlay mutation, empty-mode, and inspector tests passed. See [`upstream-test-overlay.txt`](../../../evidence/daxfs-20260828/upstream-test-overlay.txt). |
| Deterministic root/image | Pass after audit correction | Static image was 2,306,048 bytes, format 8, SHA-256 `45e5a64a92cec4e6f0d1e9a340e102d692b0a3724c407b5b5e71d59283b05070`. The original checksum manifest accidentally included itself; the audited builder now excludes the checksum file and verifies every listed file before packaging. |
| Child mount without root switch | Pass for marker/payload; full-manifest proof added later | Child mounted region `0xd10c14000`, size 70,467,584, matched marker and payload hashes, and emitted `DAXFS_MOUNT_READY`. See [`stage3-console.txt`](../../../evidence/daxfs-20260828/stage3-console.txt). The later audited root run verified every supported file and emitted `DAXFS_MANIFEST_READY`. |
| DAXFS child root, two clean cycles | Pass | Both cycles showed PID 1 `/init`, `/` as DAXFS, the initramfs-only marker absent, and `DAXFS_ROOT_READY`. Physical regions differed (`0xd11414000`, `0xb92214000`), proving the address was rediscovered. See the [cycle 1](../../../evidence/daxfs-20260828/stage4-cycle1-console.txt) and [cycle 2](../../../evidence/daxfs-20260828/stage4-cycle2-console.txt) consoles. |
| Static/read-only and normal overlay operations | Pass | Regular/empty/nested files, hashes, metadata, large reads, write rejection, create, overwrite, append, truncate, rename, unlink, mkdir/rmdir, and symlink behavior passed. |
| Corrupted image validation | Pass | A copied root inode mode was changed at offset `0x1004`; the validated mount rejected it with `EINVAL`. See [`corruption-validation.txt`](../../../evidence/daxfs-20260828/corruption-validation.txt). |
| Overlay exhaustion | Pass | A 1 MiB pool accepted 128 4 KiB files, then returned `ENOSPC`; inspector reported 100% allocation. See [`exhaustion-result-1m.txt`](../../../evidence/daxfs-20260828/exhaustion-result-1m.txt). |
| Docker image as DAXFS root | Pass after explicit Kerf fixes | BusyBox image ID `sha256:913d8ae6b717b08d7d813d5538f6d516332054b5a32fb79cd9f15cd9eece9874`; child emitted `DOCKER_IMAGE_READY` and showed DAXFS at `/`. |
| Two children, same read-only DAXFS | Pass | Both mounted physical region `0x7b241c000`, matched hashes, rejected writes, and emitted `SHARED_RO_READY`. See the [A](../../../evidence/daxfs-20260828/shared-ro-a-console.txt) and [B](../../../evidence/daxfs-20260828/shared-ro-b-console-boot-attached.txt) consoles. |
| Two children, same writable DAXFS | **Coherence fail** | A failed to observe B's conflict-free create after first caching a negative lookup; A/B reported 200/100 lines and different hashes for a contended file. Host later saw both independent names and A's 200-line version. See [`shared-rw-summary.txt`](../../../evidence/daxfs-20260828/shared-rw-summary.txt). |
| Forced child stop and restart | Pass within live allocation | Reloaded B with the same physical region; it read both A and B markers and emitted `DAXFS_RESTART_READY`. See [`restart-console.txt`](../../../evidence/daxfs-20260828/restart-console.txt). This proves allocation-lifetime retention, not durable persistence. |
| Bounded performance observation | Partial substitution, no general claim | Five host-side cached sequential reads of a 256 MiB file: ext4 7.85 GB/s, tmpfs 7.90 GB/s, DAXFS 9.87 GB/s. The specifically planned existing child-initramfs baseline was not directly measured; tmpfs was used as the memory-filesystem baseline. Raw fio JSON is retained. |
| Different kernel per Docker-derived workload | **Pass** | Two active children simultaneously reported `7.0.0-mk2-gce-lab` and `7.0.0-mk2-gce-lab-alt`, with distinct kernel hashes and distinct DAXFS roots. See [`dual-kernel-summary.txt`](../../../evidence/daxfs-20260828/dual-kernel-summary.txt). |
| Automation and cleanup | Pass | `make daxfs-build`, `daxfs-up`, `daxfs-status`, and `daxfs-down` were executed live. Final state: CPUs `0-15`, no pool, no instances, guest agent active. |

## Artifact ledger

The dated original run and the later audit/remediation run are intentionally
distinguished. The build timestamp is embedded in `DAXFS-MARKER`, so rebuilding
produces a different marker, manifest, and initramfs hash even when source and
payload inputs are otherwise unchanged.

| Artifact | SHA-256 / observation |
| --- | --- |
| DAXFS module | `8c2dc0c4a5218de1a60f9765c770c280957d69d72537b7b680d3ecf2b6566de4` |
| `mkdaxfs` | `d1d7825a1fd9cd77bc459049b8ee3e94f2ab5adf38bf412f30ead613bc918acc` |
| `daxfs-inspect` | `8030561f9bec653db52876caab00a4392f9bb1aeb78b1fa4dd2804786859ade8` |
| Original marker | `dbcc28fbdc195d4b855d6a3e87c1d21f11fdc6c8346ce861a6407ee18b75b7ba` |
| Fixed payload | `314dd22185705bdae714cee804a178203aae47b63b617e22b07748523da09ebe` |
| Original metadata manifest | `d5aa52a9638c05222dacf70081a7a2e678e9b431cc614c7c097fc298e395467e` |
| Original checksum-manifest file | `4b06f08dcf881354908ed2b65f41f738b8256f4e4fd42561f81682264e5b409f` (contained an invalid self-entry) |
| Original static image | `45e5a64a92cec4e6f0d1e9a340e102d692b0a3724c407b5b5e71d59283b05070` |
| Initial bootstrap initramfs | `7b046c7034b4ae274bae9dc1456419bafefdfe3b0a1fc578303a45cc4beb5ba2` |
| Primary Docker-capable initramfs | `28e391fcb883add850ef5e2c1dd24f29dcd6471e125b56b8997e75a675d5c486` |
| Audit-remediated metadata/checksum manifests | `de7ad80efd95585ad720d31ee32c5ead807d31a1ffcc3aa310629349597cf30a` / `4185beec8483028b1ba5c793c6b038f2e8ac7834802b96d7b9b9c6d977b74ece` |
| Audit-remediated initramfs | `386384382a5d934733aac9723c9dac9e9d21d9dade7247844897ad04fc9e3000` |
| BusyBox 1.37.0 base digest | `sha256:9db7b59979c38555a39def84a31fb98b5296952f9e3afd4f6f11f05b07adfab0` |
| Final Docker image | `sha256:913d8ae6b717b08d7d813d5538f6d516332054b5a32fb79cd9f15cd9eece9874` |

The DAXFS checkout remained at the pinned commit. Its only untracked item after
testing was the generated `tests/test_mmap` binary. Kerf was intentionally
dirty only because the two checked-in compatibility patches were applied.

## Runtime allocation ledger

Every address came from that load's Kerf handoff; none is safe to hard-code or
reuse after teardown.

| Experiment | Physical DAX region | Size | Result |
| --- | --- | ---: | --- |
| Mount-only child | `0xd10c14000` | 70,467,584 | `DAXFS_MOUNT_READY` |
| Root cycle 1 | `0xd11414000` | 70,467,584 | `DAXFS_ROOT_READY` |
| Root cycle 2 | `0xb92214000` | 70,467,584 | `DAXFS_ROOT_READY` |
| Docker before hardlink fix | `0xd13414000` | 72,876,032 | rejected phantom inode 41 |
| Docker after both fixes | `0xd13414000` | 72,851,456 | `DOCKER_IMAGE_READY` |
| Shared read-only A/B | `0x7b241c000` | 70,467,584 | both ready, writes rejected |
| Shared writable/restart | `0xd1341c000` | 70,467,584 | coherence failure; restart retention passed |
| Dual-kernel A | `0x8b2418000` | 72,851,456 | primary release ready |
| Dual-kernel B | `0x8b6992000` | 72,851,456 | alternate release ready |
| Audit full-manifest run | `0xba2614000` | 70,467,584 | every listed file OK; `DAXFS_MANIFEST_READY` |

## Proof of different kernels

The alternate source was an isolated Git worktree at the same pinned Linux
commit. Only `CONFIG_LOCALVERSION` changed to `-gce-lab-alt`; it was built as a
separate `vmlinux`, with a separately built DAXFS module and bootstrap
initramfs. `scripts/diffconfig` confirmed that `LOCALVERSION` was the only
configuration delta. An initial attempt to use a separate `O=` output directory
against the already in-tree-built primary source was rejected as not clean;
running `mrproper` would have destroyed the verified primary build state, so an
isolated Git worktree was used instead. A `vmlinux`-only build generated
`vmlinux.symvers` but not
`Module.symvers` or `scripts/module.lds`; `make modules_prepare` plus the
vmlinux symbol table was required before the external DAXFS module could link.

| Artifact | Release / SHA-256 |
| --- | --- |
| Primary `vmlinux` | `7.0.0-mk2-gce-lab` / `5cdf26d0d34bfc8ab3d298d99f8a1e189aa6e2dba9be1f4cb2968078548a3c10` |
| Alternate `vmlinux` | `7.0.0-mk2-gce-lab-alt` / `b500e7cf43d56029066845654c953f35da532edf9664f57a251b37536e7fd658` |
| Primary DAXFS module | `8c2dc0c4a5218de1a60f9765c770c280957d69d72537b7b680d3ecf2b6566de4` |
| Alternate DAXFS module | vermagic `7.0.0-mk2-gce-lab-alt`; `46d6792209a0492e27b6a7d3506280b82dc0415942f72a8656cab33c72830d08` |

The simultaneous live state was:

| Child | APIC IDs | Memory | Docker root | DAXFS region | Reported release |
| --- | --- | --- | --- | --- | --- |
| `docker-kernel-a` | 8,10,12,14 | 7 GB | `daxfs-proof:local` | `0x8b2418000`, 72,851,456 bytes | `7.0.0-mk2-gce-lab` |
| `docker-kernel-b` | 9,11,13,15 | 7 GB | `daxfs-proof:local` | `0x8b6992000`, 72,851,456 bytes | `7.0.0-mk2-gce-lab-alt` |

Both consoles printed `DOCKER_IMAGE_READY` and `none / daxfs ...`; Kerf showed
both instances `active` at once. The primary's kernel release and boot ID did
not change. Checked teardown then produced `DUAL_KERNEL_CLEANUP=PASS` with no
pool or instances.

## Required pinned-Kerf compatibility patches

The first Docker-root mount failed with `inode 41: unsupported file type 00`.
The OCI rootfs contained device nodes plus 445 directory entries representing
only 40 unique inodes—405 entries were hardlink aliases—because BusyBox
applets are hardlinks. Two separately
documented fixes were needed:

1. [`kerf-v0.2.0-skip-special-files.patch`](../../../patches/kerf-v0.2.0-skip-special-files.patch)
   omits device nodes, FIFOs, and sockets. DAXFS supports directories, regular
   files, and symlinks; the child mounts devtmpfs at boot.
2. [`kerf-v0.2.0-hardlink-inode-count.patch`](../../../patches/kerf-v0.2.0-hardlink-inode-count.patch)
   sizes and reports the inode table by unique allocated inode count rather
   than directory-entry count. Without it, the serialized image contains
   phantom zero-mode inodes.

The special-file patch alone did not fix the mount: inode 41 remained because
the hardlink table-count defect was independent. Only the combination passed.

These patches deliberately modify the pinned Kerf checkout and are applied
idempotently by `daxfs-build.sh`; they are not represented as upstream Kerf.

## Writable-coherence diagnosis

The shared-memory overlay's atomic publication prevents obvious structure
tearing, but shared bytes alone do not invalidate each kernel's independent
VFS caches. In the pinned source, `daxfs_lookup()` calls `d_splice_alias()` but
no DAXFS dentry operations or `d_revalidate` callback are registered.
`daxfs_iget()` also returns an already cached inode without rereading all
metadata. The observed asymmetric peer visibility is therefore consistent
with a cached negative dentry, and the contended-file divergence is consistent
with missing cross-kernel VFS/page-cache invalidation. This is an inference
from the test plus source inspection, not a formal proof of every race.

A correct shared-write design needs a cross-kernel invalidation/versioning
protocol and tests for negative dentries, inode metadata, page data, rename,
unlink, and concurrent truncate/write. Until that exists, use one writer or
mount shared DAXFS read-only.

## Other failures and corrections

- Kerf `create` often applies the transaction, prints success, then raises
  `KeyError` while reading stale state. Scripts accept this only if the live
  instance directory exists; they never blindly retry.
- Pinned Kerf treats per-instance `--devices=none` as a device literally named
  `none` and rejects it. `--devices=none` belongs on pool initialization; the
  instance option is omitted to assign no devices.
- `--kernel=~/src/linux/vmlinux` does not expand `~` after `=`. Absolute paths
  are used.
- An exact child-memory split does not fit a nominal pool because bookkeeping
  consumes space; every tested layout leaves slack.
- A 20 GB pool apply once returned `ENOMEM` after a long wait but later live
  state showed the transaction had applied. Fresh kernel state is authoritative
  before any retry.
- `mkdaxfs -O 64K` was parsed as 64 bytes; this tool accepts raw bytes for this
  path. The exhaustion proof used an explicit 1 MiB value.
- `mmap.flush()` on the DMA mapping returned `EINVAL`; visibility was already
  coherent. The corruption helper no longer depends on unsupported flush.
- Attaching MKTTY after a child was already active did not replay complete
  early output. Proof runs attach the console as boot is triggered.
- Spawn kernels log SWIOTLB low-memory allocation warnings. No DMA devices were
  assigned and all proof workloads continued successfully.
- The host-only suite emitted a `path_noexec` warning during an exec test but
  completed 20/20; no panic or host-service degradation followed.
- The original `MANIFEST.sha256` included itself because shell redirection
  created it before `find` ran. All actual payload entries verified, but its
  self-entry failed. The builder now deletes stale manifests, excludes the
  checksum file from its own inputs, verifies it immediately, and the child
  requires `DAXFS_MANIFEST_READY` before `DAXFS_ROOT_READY`.
- The root is fully enumerated and hashed, but not bit-reproducible by default:
  its required build timestamp changes the marker and downstream hashes.
- The performance item used host ext4/tmpfs/DAXFS cached reads. It did not
  directly measure the existing child initramfs baseline, so it remains a
  documented partial substitution rather than a completed comparative claim.

## Reproduce the verified implementation

There is no existing lab VM to resume. Create a fresh VM with the root
runbook, or create a new disk/VM from one of the retained snapshots, before
running the following targets. Snapshot restoration itself was not exercised
in this run and should be validated independently.

On a host provisioned with the pinned primary kernel and sources:

```bash
make daxfs-build
make daxfs-up
make daxfs-status
make daxfs-down
```

`daxfs-up` leaves one DAXFS-root child active for inspection. `daxfs-down`
removes only known DAXFS experiment names and will not release the shared pool
while an unrelated instance remains. The full dual-kernel proof requires the
primary and alternate artifacts under the dated remote artifact directory:

```bash
make daxfs-dual-kernel-proof
```

Those remote artifacts were deleted with the boot disk. The report above
records their exact revisions, configuration delta, hashes, and the worktree,
`modules_prepare`, and `Module.symvers` corrections needed to rebuild them,
but the current dual-kernel script does not automate that build. Reconstruct
the artifacts first; do not treat their recorded hashes as files retained in
this repository. The checked patches, orchestration scripts, and proof logs do
remain local.

The checked bootstrap and proof pieces are:

- [`guest/daxfs-bootstrap-init`](../../../guest/daxfs-bootstrap-init)
- [`guest/daxfs-proof.sh`](../../../guest/daxfs-proof.sh)
- [`guest/docker-proof.sh`](../../../guest/docker-proof.sh)
- [`scripts/daxfs-build.sh`](../../../scripts/daxfs-build.sh)
- [`scripts/daxfs-up.sh`](../../../scripts/daxfs-up.sh)
- [`scripts/daxfs-status.sh`](../../../scripts/daxfs-status.sh)
- [`scripts/daxfs-down.sh`](../../../scripts/daxfs-down.sh)
- [`scripts/test-daxfs-dual-kernel.sh`](../../../scripts/test-daxfs-dual-kernel.sh)

## Primary-source references

- [Pinned DAXFS source](https://github.com/multikernel/daxfs/tree/11ab401585b79b4a7c9164019852e0219e197d13)
- [Pinned Kerf source](https://github.com/multikernel/kerf/tree/8b72b3e9b266f8d32e707e2c1743ad7afc50b1ec)
- [Docker: what is a container?](https://docs.docker.com/get-started/docker-concepts/the-basics/what-is-a-container/)
- [Linux VFS documentation, including dentry revalidation](https://docs.kernel.org/filesystems/vfs.html)
