# GCE bring-up and DAXFS field notes

This is a chronological laboratory log for the Multikernel Linux 7.0 GCE
bring-up. It distinguishes observed results from assumptions and records
failed approaches as well as successful ones.

## 2026-08-28: starting state

### Verified Google Cloud state

- The configured Google Cloud project was active and billing-enabled.
- The Compute Engine API is enabled.
- The active operator has the Project Owner role.
- No Compute Engine instances or disks existed before this experiment.
- `asia-southeast1` had 200 unused N2 vCPUs and 500 unused general vCPUs.
- `n2-standard-16` is available in `asia-southeast1-b`.
- Ubuntu image `ubuntu-2604-resolute-amd64-v20260821` is available.

### Source versions selected

- Multikernel Linux: `v7.0-mk2`, commit
  `3bdd35b64413da0b4e089ce931bfc2e8b031cbf7`.
- Kerf: `v0.2.0`, commit
  `8b72b3e9b266f8d32e707e2c1743ad7afc50b1ec`.
- DAXFS is intentionally excluded from the first child-kernel proof.

### Pre-test source findings

- `v7.0-mk2` uses an in-tree contiguous page allocator and no longer needs
  `lazy_cma`, although the public getting-started page still describes it.
- Kerf needs physical APIC IDs rather than Linux logical CPU numbers.
- Kerf uses `kexec_file_load`; the host kernel therefore needs
  `CONFIG_KEXEC_FILE=y` in addition to `CONFIG_MULTIKERNEL=y`.
- MKTTY supplies the safe initial child console without assigning a GCE device.
- The first child will use an uncompressed `vmlinux` and a self-contained
  initramfs to avoid Secure Boot, disk-sharing, networking, and DAXFS variables.

### Known infrastructure risk

The project's default VPC has broad ingress rules, including SSH from
`0.0.0.0/0`. The lab VM needs outbound internet access to download Ubuntu and
GitHub dependencies. For this disposable experiment it will use normal
key-based GCE SSH, but the firewall should be tightened for any persistent or
production-like deployment.

## 2026-08-28: GCE instance creation and baseline

### Instance created

- Name: `mklinux-lab`.
- Zone: `asia-southeast1-b`.
- Machine type: `n2-standard-16`.
- Boot disk: 100 GB balanced Persistent Disk.
- Image family: `ubuntu-2604-lts-amd64`.
- Secure Boot: disabled.
- vTPM and integrity monitoring: enabled.
- Serial-port access metadata: enabled.
- External IP address: assigned for package and GitHub access.

This is the first billable resource created by the experiment.

### Baseline checks passed

- The VM reached `RUNNING` and accepted `gcloud compute ssh`.
- Running stock kernel before modification:
  `7.0.0-1010-gcp #10-Ubuntu SMP PREEMPT`.
- `google-guest-agent` reported `active`.
- The root filesystem expanded automatically to approximately 96 GB despite
  the source image being 10 GB.
- The GCE metadata server returned the correct instance name.
- The guest reported 16 vCPUs and approximately 62 GiB of RAM.
- No swap was configured.

### APIC topology observation

GCE did not expose APIC IDs in logical CPU order:

```text
logical 0-7  -> APIC 0,2,4,6,8,10,12,14
logical 8-15 -> APIC 1,3,5,7,9,11,13,15
```

This proves that a logical range such as CPUs `8-15` cannot automatically be
used as Kerf's `--cpus=8-15`. The actual APIC IDs must be selected from
`/proc/cpuinfo`. APIC ID 0 remains reserved for the primary kernel.

### Local access side effect

The first `gcloud compute ssh` invocation generated the normal local
`google_compute_engine` SSH key pair and added its public key to project SSH
metadata. This was required to establish administrative access to the VM.

### Serial recovery verified

`gcloud compute instances get-serial-port-output` successfully returned the
stock Ubuntu boot and service log. Serial logging is therefore available as a
recovery channel before the custom kernel is installed.

## 2026-08-28: source checkout and kernel configuration

### Exact revisions verified on the VM

- Linux checkout resolved to
  `3bdd35b64413da0b4e089ce931bfc2e8b031cbf7`.
- Kerf checkout resolved to
  `8b72b3e9b266f8d32e707e2c1743ad7afc50b1ec`.

Both repositories use annotated tags, which caused harmless shallow-clone
warnings that the tag object itself was not a commit. Git correctly peeled the
tags to the expected commits.

### Ubuntu GCE configuration reused successfully

The stock `7.0.0-1010-gcp` configuration already enabled:

- `CONFIG_KEXEC=y` and `CONFIG_KEXEC_FILE=y`.
- `CONFIG_KVM_GUEST=y`.
- `CONFIG_VIRTIO_PCI=y`.
- `CONFIG_SCSI_VIRTIO=y`.
- `CONFIG_VIRTIO_NET=y`.
- `CONFIG_PCI_MSI=y`.

The copied Ubuntu configuration referred to
`debian/canonical-certs.pem` and `debian/canonical-revoked-certs.pem`. Those
distribution build files are absent from the upstream Multikernel checkout.
Their config paths were cleared to avoid a build failure. Secure Boot is off,
but normal `CONFIG_KEXEC_SIG=y` was not deliberately removed.

The configured Multikernel release string is `7.0.0-mk2-gce-lab`, with:

- `CONFIG_MULTIKERNEL=y`.
- `CONFIG_MKTTY=y`.
- `CONFIG_MULTIKERNEL_VSOCKETS=y`.
- Initramfs, devtmpfs, procfs, sysfs, tmpfs, and ELF execution enabled.

`make olddefconfig` warned that the inherited modular values for Android Binder
symbols were invalid. It resolved them automatically; they are unrelated to
the GCE Multikernel path.

### Build attempt 1: missing `dwarf.h`

The first `make -j16` stopped while compiling `scripts/gendwarfksyms`:

```text
fatal error: dwarf.h: No such file or directory
```

Installing `dwarves` and `libelf-dev` is not sufficient for the Ubuntu 26.04
configuration because `gendwarfksyms` also needs the elfutils development
headers. Add `libdw-dev` to the dependency list before retrying the incremental
build.

### Build attempt 2: optional Multikernel VSOCK does not compile

After `libdw-dev` was installed, the build progressed through the full driver
tree but failed in `net/vmw_vsock/mk_transport.c`:

```text
error: initialization of ‘bool (*)(struct vsock_sock *, u32, u32)’
from incompatible pointer type ‘bool (*)(u32, u32)’
```

The `mk_transport_stream_allow` function in the tagged Multikernel tree does
not match the Linux 7.0 `vsock_transport.stream_allow` callback signature.
This is a source incompatibility in the optional
`CONFIG_MULTIKERNEL_VSOCKETS` feature, not a GCE failure.

AF_VSOCK is not required for the CPU, memory, kexec, or MKTTY proof. Disable
`CONFIG_MULTIKERNEL_VSOCKETS` for the first working build and leave the source
unpatched. This keeps the experiment on the exact released kernel commit and
avoids claiming that an unverified networking patch is part of `v7.0-mk2`.

### Build attempt 3: success

With only `CONFIG_MULTIKERNEL_VSOCKETS` disabled, the incremental build
completed successfully. Verified artifacts:

- Release: `7.0.0-mk2-gce-lab`.
- `vmlinux`: x86-64 statically linked ELF with debug information.
- `arch/x86/boot/bzImage`: recognized Linux x86 boot image with the expected
  release string.
- `vmlinux` SHA-256:
  `5cdf26d0d34bfc8ab3d298d99f8a1e189aa6e2dba9be1f4cb2968078548a3c10`.
- `bzImage` SHA-256:
  `0f124d9ff9f1efe0f4285002f39beabd55273a4627ca7c9c8a8081a1c1d016a6`.

The unstripped `vmlinux` is approximately 490 MB and the compressed boot image
is approximately 17 MB. Required Multikernel, MKTTY, kexec, KVM guest, Virtio,
and PCI MSI config values remained enabled in the completed build.

### Recovery point

Before installing the custom kernel, a snapshot named
`mklinux-lab-stock-20260828` was created from boot disk `mklinux-lab` in
`asia-southeast1-b`. At that point `/boot` contained only the stock
`7.0.0-1010-gcp` kernel, config, and initramfs.

### Installation checkpoint

`make modules_install` and `make install` both completed successfully. The
installation retained the stock GCE kernel and added:

- `/boot/vmlinuz-7.0.0-mk2-gce-lab` (approximately 17 MB).
- `/boot/initrd.img-7.0.0-mk2-gce-lab` (approximately 314 MB, generated by
  dracut).
- `/boot/config-7.0.0-mk2-gce-lab` and its `System.map`.
- `/lib/modules/7.0.0-mk2-gce-lab` (approximately 8.5 GB because the inherited
  Ubuntu configuration builds a very large module set with debug data).

The original `7.0.0-1010-gcp` kernel and initramfs remain in `/boot`. GRUB's
generated first Ubuntu entry selects the newer custom kernel, while the stock
kernel remains selectable under the advanced entries. The Google guest agent
was still active immediately before reboot.

## 2026-08-28: custom host-kernel boot proof

The first SSH verification command returned the stock release because it won a
race with shutdown and connected before the reboot actually occurred. Do not
treat the return of `systemctl reboot` as proof that the next SSH connection is
on the next boot. Compare the boot ID or boot time, then check `uname -r`.

After the new boot completed, all of the following were verified:

- `uname -r` and `/proc/version` report `7.0.0-mk2-gce-lab`.
- Serial output names the same custom kernel and boot image.
- `/boot/config-7.0.0-mk2-gce-lab` contains `CONFIG_KEXEC=y`,
  `CONFIG_MULTIKERNEL=y`, and `CONFIG_MKTTY=y`.
- The root filesystem remains `/dev/sda1`, the `ens4` interface is up, and the
  metadata server returns `mklinux-lab`.
- `google-guest-agent` is active.

The kernel log reports successful initialization of the Multikernel messaging,
hotplug, overlay, filesystem, memory heap, and MKTTY host driver. The control
filesystem is not mounted automatically; the kernel explicitly advertises
`mount -t multikernel none /sys/fs/multikernel`.

## 2026-08-28: Kerf and child-kernel proof

### Kerf on Ubuntu 26.04 / Python 3.14

Kerf's static `kerf-init` helper compiled successfully with `musl-gcc`.
Installing the Python project normally did not work because PyPI's
`pylibfdt 1.7.2` generated wrapper refers to `PyInt_AsLong` and
`PyString_FromString`, which are unavailable in Python 3.14.

The unpatched `v0.2.0` Kerf checkout works with Ubuntu's packaged
`python3-libfdt` and `python3-pyudev`. The verified installation creates the
venv with `--system-site-packages`, installs `rdtsc==0.2.1`, and installs the
pinned Kerf tree editable with `--no-deps`. Imports of `libfdt`, `click`,
`pyudev`, `yaml`, and `rdtsc` all passed.

### Kerf `--report` is a no-op for pool application

Both dry-run and non-dry-run `kerf init ... --report` printed a valid report
and returned success, but did not alter the live pool. Inspection of
`src/kerf/init/main.py` showed an unconditional `return` immediately after the
report is printed, before `reconcile_pool()` is called. Do not use `--report`
on the apply invocation with Kerf `v0.2.0`. Use it only to inspect a plan, then
apply the same arguments without that flag.

Without `--report`, the verified pool command moved physical APIC IDs 8-15 and
16 GB on NUMA node 0 into the Multikernel pool. The primary kernel's online
logical CPUs changed from `0-15` to `0-3,8-11`, exactly matching the logical
CPUs whose physical APIC IDs were 0-7. The GCE guest agent remained active.

### Instance creation behaviors

Kerf's `create` dry-run validated both instance requests. On a successful live
create, Kerf `v0.2.0` applied the overlay and printed the transaction number,
then sometimes exited 1 with `KeyError: 'smoke-a'` (or `smoke-b`) while using a
stale in-memory tree to display the new instance. The kernel filesystem and a
fresh `kerf show` proved that the instance existed and its resources were
allocated. Automation must check the live instance directory after this
specific post-write error instead of blindly retrying the create.

An exact 8 GB + 8 GB child split does not fit a nominal 16-GB pool. The first
8-GB instance succeeded, while the second failed because only
`0x1fffe8000` bytes remained; Multikernel bookkeeping consumes a small amount
of the pool. The successful split was:

- `smoke-a`: ID 1, APIC IDs 8, 10, 12, and 14, 8 GB.
- `smoke-b`: ID 2, APIC IDs 9, 11, 13, and 15, 7 GB.
- Pool slack: 1 GB.

No PCI or platform devices were assigned to either child.

### Independent boot evidence

Both instances loaded the same uncompressed `vmlinux` through
`kexec_file_load`, each with four segments and its own Multikernel ID. Each
used the 1.1-MB BusyBox-static initramfs and command line
`rdinit=/init console=mktty0 loglevel=7 panic=-1`.

MKTTY console output independently proved:

| Observation | `smoke-a` | `smoke-b` |
| --- | ---: | ---: |
| Kernel release | `7.0.0-mk2-gce-lab` | `7.0.0-mk2-gce-lab` |
| Online logical CPUs | `0-3` | `0-3` |
| `/proc/cpuinfo` count | 4 | 4 |
| `MemTotal` | 8,190,804 kB | 7,158,612 kB |
| PID 1 executable | `/bin/busybox` | `/bin/busybox` |
| Init marker | `CHILD_READY` | `CHILD_READY` |

The children logged non-fatal SWIOTLB allocation warnings because their memory
starts above 4 GB and no low memory was assigned. This minimal proof assigns no
DMA devices, and both children continued through `/init` successfully.

While both child status files read `active`, a new SSH command proved that the
primary kernel was still reachable on the same boot ID, still reported
`7.0.0-mk2-gce-lab`, retained logical CPUs `0-3,8-11`, had an active Google
guest agent, an up `ens4` NIC, and working metadata access. Host dmesg contained
separate `Multikernel instance 1 is now active` and instance 2 messages.

### Lifecycle cleanup proof

Both children were force-halted, unloaded, and deleted. Kerf returned their
eight APIC IDs and 15 GB to the pool. Running
`kerf init --cpus=none --memory=none --devices=none` then removed the full
16-GB pool and returned physical APIC IDs 8-15 to the host. Final checks showed:

- Host logical CPUs `0-15` online again.
- Approximately 61 GB host memory available.
- No child instances and no entries in `/proc/kimage`.
- No configured Multikernel memory pool.
- Google guest agent, NIC, metadata, and custom host kernel still healthy.

### Reproducibility result

The root Makefile and scripts were tested against the live VM. `make sync
verify-host`, `make smoke-up`, and `make smoke-down` all completed
successfully. The second automated smoke run reproduced both `CHILD_READY`
proofs and the cleanup returned all CPUs and memory to the host.

At this intermediate checkpoint, the VM `mklinux-lab` remained running and
billable so the custom host-kernel setup was available for further experiments.
The later DAXFS work, stop, and permanent deletion are recorded below.

## 2026-08-28: complete DAXFS and per-workload-kernel proof

Every functional stage and later functional experiment in the
[DAXFS plan](daxfs/plan.md) was
executed. The performance category used a documented host-side substitute
rather than the specifically planned child-initramfs baseline.
The [DAXFS report](daxfs/report.md) is the concise result matrix; this section preserves
the chronological failures and corrections.

### Baseline, recovery, and exact build

- Baseline capture time: `2026-08-28T12:28:55Z`.
- Pre-test boot ID: `483ddd87-fef5-440e-bf8d-5499a77938d9`.
- Recovery snapshot `mklinux-lab-pre-daxfs-20260828-2030` is `READY`.
- Host configuration has `CONFIG_FS_DAX=y`, `CONFIG_DAX=y`,
  `CONFIG_ZONE_DEVICE=y`, `CONFIG_DEV_DAX=m`, `CONFIG_MULTIKERNEL=y`,
  `CONFIG_MKTTY=y`, and `CONFIG_KEXEC_FILE=y`.
- There is no conventional `/dev/pmem*`. The valid path is
  `/dev/dma_heap/multikernel`; Kerf obtains the physical region and injects
  `rootflags=phys=...,size=...` for the child.
- The upstream host-only suite allocates from `/dev/dma_heap/system`; the
  child-root path is different and uses Kerf plus the Multikernel heap. These
  successful paths should not be conflated with a conventional pmem disk.
- DAXFS commit `11ab401585b79b4a7c9164019852e0219e197d13` built without a
  source compatibility change against the exact host tree. Module SHA-256 is
  `8c2dc0c4a5218de1a60f9765c770c280957d69d72537b7b680d3ecf2b6566de4`.
- DAXFS identifies itself as release `0.1.0`; `mkdaxfs` and `daxfs-inspect`
  report superblock 8, overlay 2, and page-cache 2. The pinned README's
  superblock-7 statement is stale. Tool SHA-256 values are
  `d1d7825a1fd9cd77bc459049b8ee3e94f2ab5adf38bf412f30ead613bc918acc`
  and `8030561f9bec653db52876caab00a4392f9bb1aeb78b1fa4dd2804786859ade8`.
- Pinned Kerf's live `load --help` and source both confirmed `--image` and
  `--rootfs-dir`; it allocates exact DMA-heap memory, discovers its physical
  range through `/proc/iomem`, mounts the host view, and passes
  `rootflags=phys=...,size=...` to the child.
- The unsigned module loads because Secure Boot and lockdown are disabled. Its
  out-of-tree/signature taint is expected and was retained in the logs.
- `tests/test_overlay.sh -v` passed all 20 tests. This includes static,
  split/page-cache, normal overlay mutation, empty-mode, and inspector paths.

### Mount and root proof

The root tree contains a static BusyBox, Kerf init, ordinary mountpoint
directories, marker/payload files, and sorted metadata/SHA manifests. The
static DAXFS image is 2,306,048 bytes with SHA-256
`45e5a64a92cec4e6f0d1e9a340e102d692b0a3724c407b5b5e71d59283b05070`.
Original marker, payload, metadata-manifest, and checksum-manifest hashes were
`dbcc28fb...`, `314dd221...`, `d5aa52a9...`, and `4b06f08d...`. The first
bootstrap initramfs was `7b046c70...`; after the Docker/root bootstrap update,
the primary initramfs was `28e391fc...`.

The mount-only child used APIC IDs 8,10,12,14, 6 GB, no devices, and
runtime-discovered region `0xd10c14000`, size 70,467,584. It emitted
`DAXFS_MOUNT_READY`. Two subsequent
root cycles used newly discovered physical regions `0xd11414000` and
`0xb92214000`. Both proved PID 1 `/init`, a DAXFS `/`, absence of the
initramfs marker, correct file hashes, and `DAXFS_ROOT_READY`; both cleanups
returned all resources.

Passing `--kernel=~/src/linux/vmlinux` did not expand the tilde because it
followed `=`. All checked scripts use absolute or `$HOME`-expanded paths.
Kerf's known post-create `KeyError` also recurred after successful kernel-side
transactions, so scripts accept the nonzero result only when the live instance
directory proves creation actually completed.

The first automated dual-kernel create used per-instance `--devices=none` and
failed validation because pinned Kerf treated `none` as a literal nonexistent
device. The corrected rule is: set `--devices=none` on `kerf init`, then omit
the option from `kerf create`; the resulting device list is empty.

### Validation, exhaustion, and bounded measurement

- Changing the copied root inode mode at offset `0x1004` from `0x41fd` to zero
  caused the validated mount to reject the image with `EINVAL`.
- Calling `mmap.flush()` on the DMA mapping returned `EINVAL`; coherent
  visibility did not require that unsupported operation, so the corruption
  helper was corrected to avoid it.
- `mkdaxfs -O 64K` created a 64-byte pool rather than 64 KiB. The verified
  exhaustion run supplied 1 MiB explicitly. It accepted 128 4 KiB files and
  then returned `ENOSPC`, with inspector showing 1,048,576/1,048,576 bytes
  allocated and 386/512 buckets used.
- The fio run read a cached 256 MiB zero file five times with 1 MiB buffered,
  synchronous reads: ext4 7,848,989,941 B/s, tmpfs 7,895,160,470 B/s, and
  DAXFS 9,868,950,588 B/s. This is a lab microbenchmark, not evidence of broad
  application performance or storage durability.

### Docker image failure and explicit Kerf patches

The image `daxfs-proof:local` was built from `busybox:1.37.0`; its ID is
`sha256:913d8ae6b717b08d7d813d5538f6d516332054b5a32fb79cd9f15cd9eece9874`.
The pinned base digest was
`sha256:9db7b59979c38555a39def84a31fb98b5296952f9e3afd4f6f11f05b07adfab0`.
The first child mount failed with `inode 41: unsupported file type 00`.

Inspection found two independent serializer problems in pinned Kerf:

1. OCI roots contain special files under `/dev`, while DAXFS supports only
   directories, regular files, and symlinks. The explicit
   `kerf-v0.2.0-skip-special-files.patch` omits special files; the bootstrap
   mounts devtmpfs in the child.
2. The BusyBox root contained 445 directory entries but only 40 unique inodes,
   making 405 entries hardlink aliases. Kerf preserved shared inode numbers but used
   directory-entry count for the inode table and superblock, leaving phantom
   zero-mode inodes. `kerf-v0.2.0-hardlink-inode-count.patch` uses allocated
   unique inode count in all three calculations.

The first special-file patch alone still produced phantom inode 41. After both
independent patches, the Docker-derived root emitted `DOCKER_IMAGE_READY`, ran
its configured `/docker-proof` entrypoint, and showed DAXFS at `/`.

### Shared mounts and coherence limitation

Two children mounted the same physical region `0x7b241c000` read-only. Both
matched file hashes, rejected writes, and emitted `SHARED_RO_READY`.

The shared writable test used region `0xd1341c000`. Both children created
conflict-free files and the host eventually saw both. However, A first looked
up B's future filename, cached the miss, and still reported
`SHARED_RW_PEER_VISIBLE=no` after B created it; B saw A's already-present
file. Concurrent operations on `contended.log` diverged: A and the host saw
200 lines with SHA-256 `c91e3b3...`, while B saw 100 lines with SHA-256
`48fce89e...`.

Source inspection explains the likely mechanism. Pinned `daxfs_lookup()` does
not install dentry revalidation operations, and each kernel has an independent
VFS dentry/inode/page cache. Shared coherent bytes do not invalidate a negative
dentry or all locally cached inode/page state in another kernel. This test
therefore disproves full multi-writer coherence for the pinned revision. Use
shared read-only or a single writer until a cross-kernel invalidation protocol
is implemented and tested.

One 20 GB pool apply printed `ENOMEM` after a long wait, but a fresh live-state
read showed that the requested pool had subsequently applied. As with Kerf's
post-create error, automation must inspect kernel state before retrying a
possibly completed transaction.

MKTTY late attachment also did not replay all early output for one already
active child. Restarting that proof with the console attached while boot was
triggered captured the full transcript. The checked dual-kernel script starts
both console sessions around the boot calls.

### Restart and different-kernel proof

Child B was force-stopped, unloaded, and reloaded against the still-live shared
region. It recovered both marker files and emitted `DAXFS_RESTART_READY`.
This is allocation-lifetime retention only; releasing the pool destroys the
storage contract.

For the decisive kernel-selection test, an isolated Git worktree at the same
Linux commit changed only the local version to
`7.0.0-mk2-gce-lab-alt`; `scripts/diffconfig` confirmed this was the only
configuration difference. Trying a separate `O=` build against the already
in-tree-built primary checkout first failed because the source was not clean.
The verified primary tree was not subjected to `mrproper`; a detached worktree
preserved it. Building only `vmlinux` produced
`vmlinux.symvers` but not `Module.symvers` or `scripts/module.lds`; the first
external DAXFS module attempt therefore failed at modpost, and the second
failed because the linker script did not exist. `make modules_prepare`, plus
using the completed vmlinux symbol table as `Module.symvers`, resolved the
external-module build without touching the primary source tree.

The simultaneous proof used a 16 GB pool and two 7 GB children:

- `docker-kernel-a`, APIC 8,10,12,14, primary `vmlinux`, DAXFS
  `0x8b2418000`.
- `docker-kernel-b`, APIC 9,11,13,15, alternate `vmlinux`, DAXFS
  `0x8b6992000`.

Both Kerf status records were `active` together. Their consoles independently
printed `DOCKER_IMAGE_READY`, DAXFS at `/`, and releases
`7.0.0-mk2-gce-lab` / `7.0.0-mk2-gce-lab-alt`. Kernel SHA-256 values were
`5cdf26d0...` and `b500e7cf...`, so this was not one binary with misleading
console text. The primary remained on the original boot ID throughout.

This confirms a different selected kernel per Docker-derived workload. It
does not mean ordinary Docker/runc containers gained separate kernels: Kerf
uses the Docker image as the rootfs input, then Multikernel supplies the child
kernel.

### Final automation and state

During the final documentation audit, `sha256sum -c` exposed that the original
checksum manifest included its own partially written file. All eight payload
entries were valid, but the self-entry necessarily failed. The generator now
removes stale manifests, excludes `MANIFEST.sha256` from its own input, and
checks every entry. The child proof now gates readiness on the same check. A
fresh 6 GB child at region `0xba2614000` reported every listed file `OK`, then
`DAXFS_MANIFEST_READY` and `DAXFS_ROOT_READY`. The audit-remediated metadata
manifest, checksum manifest, and initramfs hashes are `de7ad80e...`,
`4185beec...`, and `38638438...`.

The required build timestamp means a rebuilt root is fully enumerated and
verified but is not bit-identical to an earlier build. The performance run is
also recorded precisely as a partial substitution: it compared host ext4,
tmpfs, and DAXFS cached reads, not the existing child initramfs directly.

The following newly checked targets all passed live:

```text
make daxfs-build
make daxfs-up
make daxfs-status
make daxfs-down
make daxfs-dual-kernel-proof
```

`daxfs-build` reproduced the original DAXFS module and Docker image hashes.
`daxfs-up` emitted `DAXFS_ROOT_READY`; status showed the child active and its
host DAXFS allocation; cleanup restored online CPUs `0-15`. A second cleanup
on the already-clean host also passed. The final host has no Multikernel pool,
no instances, an empty `/proc/kimage`, and an active Google guest agent.

After the final manifest audit, the last child and 12 GB pool were removed and
the clean state was captured. The VM was stopped at
`2026-08-28T06:49:01.366-07:00`, then permanently deleted with its auto-delete
100 GB `pd-balanced` boot disk after the evidence was copied. A final GCE
inventory showed no matching instance or disk. Snapshots
`mklinux-lab-stock-20260828` and
`mklinux-lab-pre-daxfs-20260828-2030` remained `READY`, and the checked local
scripts, patches, documentation, and evidence were retained. Snapshot storage
may continue to incur charges. The timestamped post-deletion inventory is
[`resource-cleanup.txt`](../../evidence/daxfs-20260828/resource-cleanup.txt).

Deleting the boot disk removed the active copies of the remote Linux, Kerf,
and DAXFS workspaces and the compiled DAXFS proof artifacts. The pre-DAXFS
snapshot may contain the earlier primary-kernel/Kerf workspace, but it predates
the DAXFS and alternate-kernel work. A later run must recreate those later
artifacts from the pinned revisions and recorded procedure. The single-kernel
build path is automated, but the dual-kernel proof script still expects a
dated artifact directory and does not yet build the alternate worktree,
matching DAXFS module, or alternate initramfs. Snapshot restoration was not
tested and should not be assumed to be a verified recovery path merely because
the snapshots are `READY`.
