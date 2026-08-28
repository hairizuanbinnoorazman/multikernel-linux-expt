# Learnings

This is a chronological laboratory log for the Multikernel Linux 7.0 GCE
bring-up. It distinguishes observed results from assumptions and records
failed approaches as well as successful ones.

## 2026-08-28: starting state

### Verified Google Cloud state

- Project `REDACTED_GCP_PROJECT_ID` is active and billing-enabled.
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

The VM `mklinux-lab` remains running and billable after the proof so the custom
host-kernel setup is available for further experiments. Use `make vm-stop` to
stop vCPU/RAM charges while retaining the disk, or `make vm-delete` only when
the lab and its boot disk are no longer needed.
