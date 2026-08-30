# Multikernel Linux 7.0 on Google Compute Engine

This repository documents how to evaluate Multikernel Linux `v7.0-mk2` on a
Google Compute Engine (GCE) virtual machine and now serves as the foundation
for a Multikernel-backed containerd/Kubernetes runtime.

The initial objective is deliberately narrow:

> Boot the `v7.0-mk2` primary kernel on GCE, allocate a small set of virtual
> CPUs and memory to a child kernel, and prove through the MKTTY console that
> the child received the expected resources.

Networking, persistent child storage, DAXFS, Docker images, accelerators, and
device assignment were deliberately excluded from the first bring-up. The
complete DAXFS functional and Docker-image follow-up has now also been
executed; see
[`DAXFS-IMPLEMENTATION.md`](DAXFS-IMPLEMENTATION.md).

The host and two-child proof described below was completed successfully on GCE
on 2026-08-28. Child resources were returned to the primary kernel before
cleanup. That VM and boot disk were deleted. On 2026-08-30, the pre-DAXFS
snapshot was successfully restored as a new `mklinux-lab` for the persistent
ext4 experiment. After the run, the VM, boot disk, and two blank child disks
were deleted. See [`EXT4-DISK-LEARNINGS.md`](EXT4-DISK-LEARNINGS.md).
The complete pass/blocked matrix is in
[`EXT4-DISK-EXECUTION.md`](EXT4-DISK-EXECUTION.md).
The follow-up primary-mediated implementation passed its core two-child
persistent-root objective; see
[`EXT4-MEDIATED-IMPLEMENTATION.md`](EXT4-MEDIATED-IMPLEMENTATION.md).

## Container-runtime direction

The next objective is an opt-in container runtime in which one Multikernel
child represents one pod sandbox. The runtime will be a new project around
Kerf, not a Firecracker fork. The primary retains devices, storage, networking,
and global resource policy; a small agent manages OCI processes inside each
child.

- [`docs/architecture/TARGET.md`](docs/architecture/TARGET.md) defines the
  target architecture and trust boundary.
- [`docs/plans/README.md`](docs/plans/README.md) links the ordered, gated plans
  from host qualification through the final developer-preview build.
- [`runtime/README.md`](runtime/README.md) defines the source-tree boundaries
  before implementation begins.
- [`docs/research/RELATED-WORK.md`](docs/research/RELATED-WORK.md) records the
  GitHub audit that found no public Kerf-backed containerd runtime.

The first release target is explicitly for trusted, single-tenant nodes.
Multikernel sibling kernels must not be described as providing Firecracker's
KVM/EPT security boundary without separate evidence.

## Status

Research, implementation, and live account checks were performed on
2026-08-28. Resource cleanup was verified on 2026-08-29.

| Item | Status |
| --- | --- |
| Google Cloud project | The project selected by `MK_PROJECT` or the active `gcloud` configuration is used |
| Billing | Enabled |
| Compute Engine API | Enabled |
| Operator permissions | Project Owner |
| Laboratory VM | `mklinux-mediated-20260830` is stopped after the mediated-root run |
| Boot disk | Retained 100 GiB `pd-balanced`, auto-delete enabled if the stopped VM is deleted |
| Mediated storage disk | Retained 20 GiB `pd-balanced`, auto-delete disabled; contains outer ext4 and two 4 GiB images |
| Direct-device ext4 disks | Two blank 10 GiB probe disks were tested, then deleted |
| Retained snapshots | `mklinux-lab-stock-20260828` and `mklinux-lab-pre-daxfs-20260828-2030`, both `READY` at final check |
| Primary-kernel GCE functions | SSH, guest agent, NIC, disk, metadata, and serial passed |
| Concurrent child proof | Passed with two four-vCPU children |
| DAXFS child root | Passed twice from clean pools |
| Docker image as child root | Passed after two documented pinned-Kerf fixes |
| Different kernel per Docker-derived workload | Passed concurrently with two distinct `vmlinux` files |
| Shared read-only DAXFS | Passed with two children |
| Shared writable DAXFS | Coherence failed; do not use as multi-writer storage |
| Cleanup proof | Passed; all 16 vCPUs and host memory restored |
| Current disposition | Mediated VM stopped; its two disks and the earlier recovery snapshots remain billable |
| Target region | `asia-southeast1` |
| N2 vCPU quota at initial check | 200 available, 0 used |
| General vCPU quota at initial check | 500 available, 0 used |
| `n2-standard-16` | Available in `asia-southeast1-b` |
| Ubuntu image | `ubuntu-2604-resolute-amd64-v20260821` |

The project's default VPC currently has ingress rules that expose SSH, RDP,
HTTP, TCP port 3000, and ICMP to `0.0.0.0/0`. Tighten those rules or use an IAP
access design before giving the test VM an external IP.

## DAXFS and Docker-image outcome

DAXFS is a **direct-access (DAX), shared-memory-backed filesystem**. In this
experiment, Kerf serializes a directory or Docker/OCI root filesystem into a
DAXFS image, Multikernel places that image in byte-addressable shared physical
memory, and a child kernel mounts it directly, including as its `/` root. It is
not a virtual block device or GCE Persistent Disk: its contents survive only
while the shared-memory allocation remains live unless a separate persistence
mechanism is provided.

The live run confirmed the intended capability, with an important wording
boundary: Docker supplied OCI filesystems, while Multikernel/Kerf supplied an
independently selected kernel for each workload. Ordinary Docker/runc
containers still share their Docker host kernel.

| Capability | Result |
| --- | --- |
| Pinned DAXFS upstream suite | 20/20 passed |
| DAXFS as a child root | Passed twice from clean pools; later full-manifest audit also passed |
| Docker-derived DAXFS root | Passed after the two recorded pinned-Kerf fixes |
| Two concurrent Docker-derived roots with distinct kernels | Passed with `7.0.0-mk2-gce-lab` and `7.0.0-mk2-gce-lab-alt` |
| Shared read-only DAXFS | Passed with two children |
| Shared writable DAXFS | Coherence failed; use read-only or a single writer at this revision |
| Child restart | Data survived only while the shared-memory allocation remained live |
| Corruption rejection and bounded exhaustion | Passed; exhaustion returned `ENOSPC` |
| Performance comparison | Partial: host ext4/tmpfs/DAXFS cached reads were measured, but the planned child-initramfs baseline was not |
| Final cleanup | All CPUs and memory returned, then the VM and boot disk were deleted |

The complete matrix, exact hashes, physical allocation ledger, failures,
patches, and interpretation are in
[`DAXFS-IMPLEMENTATION.md`](DAXFS-IMPLEMENTATION.md). Raw transcripts are
indexed under [`evidence/daxfs-20260828/`](evidence/daxfs-20260828/), including
the [post-deletion GCE inventory](evidence/daxfs-20260828/resource-cleanup.txt).

## Reproduce the verified path

No laboratory VM currently exists. The root `Makefile` wraps fresh VM creation,
source provisioning, host verification, the two-child smoke test, cleanup, log
collection, and the ext4 disk topology audit. Review its variables and the
network warning above before creating billable resources:

```bash
make help
make vm-create
make provision-kernel
make snapshot
make reboot
make verify-host
make install-kerf
make smoke-up
make smoke-status
make smoke-down
```

The retained snapshots are recovery inputs, not a finalized DAXFS appliance.
`mklinux-lab-stock-20260828` is the stock checkpoint;
`mklinux-lab-pre-daxfs-20260828-2030` is the custom-primary-kernel checkpoint
immediately before DAXFS work. The deleted boot disk contained the DAXFS and
alternate-kernel source trees and compiled artifacts. A fresh run must rebuild
those from the pinned revisions, checked patches, scripts, and recorded build
procedure. In particular, the dual-kernel target currently consumes prebuilt
primary/alternate artifacts; automating their reconstruction remains an open
task. Restoring `mklinux-lab-pre-daxfs-20260828-2030` as a new disk/VM passed on
2026-08-30. The stock snapshot restore path remains untested.

`make smoke-up` is intentionally specific to the verified
`n2-standard-16` topology. It allocates physical APIC IDs 8-15, never APIC ID
0, assigns no devices, and leaves both children running until
`make smoke-down` is invoked.

## Version pins

This project is changing quickly. Use matching, known revisions rather than
mixing whatever happens to be on each repository's default branch.

| Component | Version or revision | Date |
| --- | --- | --- |
| Multikernel Linux | `v7.0-mk2` / `3bdd35b64413da0b4e089ce931bfc2e8b031cbf7` | 2026-08-25 |
| Kerf | `v0.2.0` / `8b72b3e9b266f8d32e707e2c1743ad7afc50b1ec` | 2026-08-25 |
| DAXFS, optional | `0.1.0` / `11ab401585b79b4a7c9164019852e0219e197d13`; formats superblock 8, overlay 2, page cache 2 | 2026-08-15 |

`v7.0-mk2` moved memory-pool allocation into the kernel. It does **not** need
the out-of-tree `lazy_cma` module. The public getting-started page still
contains the older `lazy_cma` workflow, so do not follow that part of the page
for `mk2`.

## Architecture

Multikernel is not nested virtualization. The layout on GCE is:

```text
Google physical host
└── Google KVM hypervisor
    └── One GCE virtual machine
        ├── Primary v7.0-mk2 Linux kernel
        ├── Spawn kernel 1
        └── Spawn kernel 2
```

The primary kernel uses extensions to Linux `kexec` and CPU hotplug to park
selected CPUs, reserve contiguous guest-physical memory, and start another
kernel alongside itself. The child kernels operate within the resources of one
GCE VM. They do not become separate GCE instances.

Consequences on GCE:

- Assigned CPUs are GCE vCPUs, not guaranteed dedicated physical cores.
- Assigned memory is guest-physical memory backed by the GCE VM's memory.
- Devices visible to Multikernel are virtual devices exposed by GCE.
- Google's outer hypervisor, maintenance behavior, and live migration remain.
- Nested virtualization does not need to be enabled.
- Bare-metal performance claims do not directly apply inside a GCE VM.

The live run confirmed that GCE's virtual APIC accepted the tested CPU park,
child-start, stop, and resource-return sequences. That result applies to the
tested `n2-standard-16` instance and pinned software revisions; stop/start,
host maintenance, and live migration still need separate testing.

## Important findings from the source

The exact tagged kernel and matching Kerf source were inspected for this
runbook.

### Kernel requirements

`CONFIG_MULTIKERNEL` currently supports x86-64 and depends on `KEXEC_CORE`.
Kerf loads children with the `kexec_file_load` syscall, so `CONFIG_KEXEC_FILE`
is also required in practice.

`CONFIG_MKTTY` supplies the inter-kernel console:

- The child boots with `console=mktty0`.
- The primary kernel exposes `/dev/mktty`.
- `kerf exec NAME --console` can start and attach in one operation.

Kerf accepts an uncompressed ELF `vmlinux` or a compressed `vmlinuz`. For a
`vmlinuz`, Kerf extracts its embedded `vmlinux` before loading it. The extracted
file is unsigned, so that route does not work when Secure Boot lockdown or
forced kexec signature verification is active. Use the build's uncompressed
`vmlinux` and disable Secure Boot for the initial experiment.

### CPU identifiers

Kerf's `--cpus` arguments are **physical APIC IDs**, not Linux logical CPU
numbers. A value such as `--cpus=4-7` is correct only if those are the APIC IDs
reported by `/proc/cpuinfo`.

Never allocate APIC ID 0, the boot CPU, to the Multikernel pool.

### Networking maturity

The inspected kernel contains an AF_VSOCK transport using Multikernel's shared
memory and IPI messaging. It does not provide enough evidence to assume that a
normal Ethernet-style inter-kernel interface is ready for GCE.

Do not assign the GCE VM's only virtual NIC to a child during initial testing.
Doing so could remove networking and metadata access from the primary kernel.
External child networking should later be built through one of these designs:

- AF_VSOCK communication with a proxy in the primary kernel's userspace.
- A verified Multikernel network interface, if implemented in a later release.
- Carefully tested device or virtual-function assignment where supported.

### Storage maturity

Do not assign the GCE boot disk controller to a child. The primary kernel needs
it to remain booted and manageable.

The first child should use a self-contained initramfs. DAXFS can be tested only
after the CPU and memory path succeeds. DAXFS requires `CONFIG_FS_DAX` and is
an out-of-tree module unless deliberately built into a custom kernel tree.

## Proposed GCE laboratory

Recommended first VM:

| Property | Value |
| --- | --- |
| Name | `mklinux-lab` |
| Zone | `asia-southeast1-b` |
| Machine type | `n2-standard-16` |
| Resources | 16 vCPUs, 64 GB RAM |
| Boot disk | 100 GB balanced Persistent Disk |
| OS | Ubuntu 26.04 x86-64 |
| Secure Boot | Disabled initially |
| vTPM | May remain enabled |
| Integrity monitoring | May remain enabled |
| Serial-port logging | Enabled |
| Nested virtualization | Disabled/not required |

Set reusable shell variables before using the commands in this guide. The
project helper prefers an existing `MK_PROJECT` value and otherwise reads the
active `gcloud` configuration:

```bash
export MK_PROJECT="$(./scripts/detect-gcp-project.sh)"
export MK_ZONE="asia-southeast1-b"
export MK_VM="mklinux-lab"
```

Do not create the VM until cloud spending and network exposure have been
approved.

A prospective creation command is:

```bash
gcloud compute instances create "$MK_VM" \
  --project="$MK_PROJECT" \
  --zone="$MK_ZONE" \
  --machine-type=n2-standard-16 \
  --image-project=ubuntu-os-cloud \
  --image-family=ubuntu-2604-lts-amd64 \
  --boot-disk-type=pd-balanced \
  --boot-disk-size=100GB \
  --no-shielded-secure-boot \
  --shielded-vtpm \
  --shielded-integrity-monitoring \
  --metadata=serial-port-enable=true
```

This command assigns an external address by default. Resolve the VPC firewall
issue first, or change the access design to IAP plus suitable outbound package
access.

## Phase 1: prepare the build environment

Connect to the VM and install the normal Linux kernel build dependencies plus
Kerf's build dependencies. The exact Ubuntu package set may evolve, but a
starting set is:

```bash
sudo apt-get update
sudo apt-get install -y \
  bc bison build-essential busybox-static cpio device-tree-compiler dwarves \
  flex git libdw-dev libelf-dev libfdt-dev libncurses-dev libssl-dev musl-tools \
  python3-dev python3-pip python3-venv rsync swig
```

Record the initial system state:

```bash
uname -a
lsblk
lspci -nn
systemctl status google-guest-agent --no-pager
grep -E '^(processor|apicid|physical id|core id)' /proc/cpuinfo
```

Take a GCE disk snapshot before changing the boot kernel if easy rollback is
desired.

## Phase 2: build the primary Multikernel kernel

Clone the exact release:

```bash
git clone --depth=1 --branch v7.0-mk2 \
  https://github.com/multikernel/linux.git
cd linux
```

Start from the running GCE Ubuntu kernel configuration. This retains Google's
guest, storage, network, timing, and console drivers:

```bash
cp "/boot/config-$(uname -r)" .config
make olddefconfig
```

Enable the required Multikernel features. `scripts/config` should be followed
by `olddefconfig` so dependencies are resolved:

```bash
scripts/config --enable KEXEC
scripts/config --enable KEXEC_FILE
scripts/config --enable MULTIKERNEL
scripts/config --enable MKTTY
scripts/config --disable MULTIKERNEL_VSOCKETS
scripts/config --enable DEVTMPFS
scripts/config --enable DEVTMPFS_MOUNT
make olddefconfig
```

The optional Multikernel VSOCK transport in the exact `v7.0-mk2` tag does not
compile against Linux 7.0 because its `stream_allow` callback has the old
signature. It was disabled without patching the pinned source. CPU, memory,
kexec, and MKTTY do not depend on it.

Confirm the GCE-required options survived configuration:

```bash
scripts/config --state KVM_GUEST
scripts/config --state KVM_CLOCK
scripts/config --state VIRTIO_PCI
scripts/config --state SCSI_VIRTIO
scripts/config --state VIRTIO_NET
scripts/config --state PCI_MSI
scripts/config --state KEXEC_FILE
scripts/config --state MULTIKERNEL
scripts/config --state MKTTY
```

Every required item should print `y` or, where safe for an ordinary device
driver, `m`. Multikernel, MKTTY, and the facilities required before the child
root filesystem is available should be built in with `y`.

Build and install:

```bash
make -j"$(nproc)"
sudo make modules_install
sudo make install
sudo update-grub
```

Preserve the stock Ubuntu kernel in GRUB. Do not remove it; it is the recovery
kernel if Multikernel fails to boot.

## Phase 3: boot and validate the primary kernel

Before rebooting, verify that serial console output is enabled and note the
stock GRUB entry. Then reboot:

```bash
sudo systemctl reboot
```

If SSH does not return, inspect GCE serial-port output and select the stock
Ubuntu kernel from GRUB through the interactive serial console.

After a successful boot:

```bash
uname -a
grep -E 'CONFIG_(MULTIKERNEL|MKTTY|KEXEC_FILE)=' \
  "/boot/config-$(uname -r)"
sudo mkdir -p /sys/fs/multikernel
sudo mount -t multikernel none /sys/fs/multikernel
find /sys/fs/multikernel -maxdepth 2 -type f -o -type d
ls -l /dev/mktty
systemctl status google-guest-agent --no-pager
```

Stop here if the guest agent, disk, network, metadata access, or serial console
is not healthy.

## Phase 4: install Kerf v0.2.0

Clone the matching Kerf release:

```bash
cd
git clone --depth=1 --branch v0.2.0 \
  https://github.com/multikernel/kerf.git
cd kerf
```

Build its static helper and install it into a virtual environment. Ubuntu
26.04 uses Python 3.14, while PyPI's `pylibfdt 1.7.2` wrapper does not compile
there. Use Ubuntu's compatible package through a system-site-packages venv:

```bash
make
sudo apt-get install -y python3-libfdt python3-pyudev
python3 -m venv --clear --system-site-packages .venv
.venv/bin/pip install rdtsc==0.2.1
.venv/bin/pip install --no-deps -e .
.venv/bin/kerf --version
```

Kerf operations that mount filesystems, hot-unplug CPUs, allocate memory, or
load kernels require root. Preserve the virtual environment's executable path
when invoking it with `sudo`, for example:

```bash
sudo "$(pwd)/.venv/bin/kerf" --version
```

Do not build or load `lazy_cma` on `v7.0-mk2`.

## Phase 5: map APIC IDs and initialize a pool

Create an explicit logical-CPU-to-APIC-ID map:

```bash
awk '
  /^processor[[:space:]]*:/ { logical=$3 }
  /^apicid[[:space:]]*:/    { print "logical=" logical, "apic=" $3 }
' /proc/cpuinfo
```

Choose four nonzero APIC IDs. Keep the boot CPU and plenty of CPU capacity on
the primary kernel. In the examples below, replace `APIC_LIST` with the actual
comma-separated IDs; do not copy a guessed range.

Preview the change first:

```bash
sudo /path/to/kerf init \
  --cpus=APIC_LIST \
  --memory=8GB \
  --dry-run \
  --verbose
```

Review:

- The selected values are APIC IDs that actually exist.
- APIC ID 0 is not selected.
- At least four CPUs remain with the primary kernel.
- The memory request is on a valid NUMA node.
- No devices are being moved.

Apply the pool only after the dry run is correct:

```bash
sudo /path/to/kerf init \
  --cpus=APIC_LIST \
  --memory=8GB \
  --verbose
```

Create the first child allocation:

```bash
sudo /path/to/kerf create smoke \
  --cpu-count=2 \
  --memory=4GB \
  --verbose

sudo /path/to/kerf show
sudo /path/to/kerf show smoke
```

Do not pass `--devices` during the first experiment.

## Phase 6: make a minimal child initramfs

The first child should not depend on the GCE boot disk, networking, systemd,
Docker, or DAXFS. Build a small initramfs containing a static shell or BusyBox
and an `/init` script that:

1. Mounts `proc` at `/proc`.
2. Mounts `sysfs` at `/sys`.
3. Mounts `devtmpfs` at `/dev`.
4. Prints `uname -a`, `/proc/cpuinfo`, and `/proc/meminfo`.
5. Starts an interactive shell on `/dev/mktty0`, or powers off cleanly after
   recording the results.

Keep this artifact independent of the host filesystem. Its purpose is to test
only CPU startup, memory isolation, initramfs execution, and MKTTY.

Use the uncompressed `vmlinux` produced by the kernel build. This avoids
Kerf's `vmlinuz` extraction and signature-verification complication.

## Phase 7: load and start the child

With the exact kernel build and minimal initramfs paths:

```bash
sudo /path/to/kerf load smoke \
  --kernel=/path/to/linux/vmlinux \
  --initrd=/path/to/smoke-initramfs.cpio.gz \
  --cmdline="rdinit=/init panic=-1" \
  --console=mktty0 \
  --verbose

sudo /path/to/kerf exec smoke --console --verbose
```

Detach from Kerf's console with `Ctrl+]` followed by `.`.

Inside the child, record:

```bash
uname -a
cat /proc/cpuinfo
cat /proc/meminfo
cat /proc/cmdline
mount
```

### Acceptance criteria

The initial GCE experiment succeeds when all of the following are true:

- The primary kernel remains alive and reachable over SSH.
- The child reaches `/init` and produces output over MKTTY.
- The child sees exactly the CPUs assigned by Kerf.
- The child sees approximately the assigned memory and not all VM memory.
- The primary no longer schedules work on the child's CPUs while it is active.
- Killing the child returns its CPUs and memory without rebooting the VM.
- The Google guest agent remains healthy in the primary kernel.

## Phase 8: stop the child and return resources

Use the normal lifecycle first:

```bash
sudo /path/to/kerf kill smoke
sudo /path/to/kerf unload smoke
sudo /path/to/kerf delete smoke
sudo /path/to/kerf init --cpus=none --memory=none --verbose
```

Verify that the primary kernel sees all expected CPUs and memory again:

```bash
lscpu
grep -E 'MemTotal|MemAvailable' /proc/meminfo
sudo /path/to/kerf show
```

If a child or pool operation fails, capture these before rebooting:

```bash
sudo dmesg --ctime
sudo /path/to/kerf show
sudo /path/to/kerf dump --dts
cat /proc/iomem
cat /sys/devices/system/cpu/online
cat /sys/devices/system/cpu/offline
```

Also preserve the GCE serial-port log.

## Recovery

### Primary kernel does not boot

1. Read the GCE serial-port output.
2. Use the interactive serial console to enter GRUB.
3. Select the original Ubuntu kernel.
4. Correct the kernel configuration or installation.
5. If necessary, stop the VM and attach its boot disk to a rescue VM.

### SSH is lost after resource assignment

Use the GCE serial console. This is why no NIC or storage devices are assigned
in the first test and why multiple CPUs remain with the primary kernel.

### Child fails to start

Check, in order:

- `CONFIG_MULTIKERNEL=y`, `CONFIG_MKTTY=y`, and `CONFIG_KEXEC_FILE=y`.
- Secure Boot and kernel lockdown state.
- Whether Kerf was given `vmlinux`, not an unusable signed/compressed path.
- Whether the requested values are physical APIC IDs.
- Primary-kernel `dmesg` for APIC, CPU-hotplug, allocation, and kexec errors.
- The instance state under `/sys/fs/multikernel/instances/`.

### Return the machine to a known state

If Kerf can still operate, kill/delete children and return the entire pool. If
not, reboot into the original Ubuntu kernel. Do not delete the VM or its disk
until logs have been collected.

## Experiments after the smoke test

The complete staged DAXFS functional experiment, including the plan's later
Docker, shared-read, shared-write, restart, exhaustion, and dual-kernel tests,
was executed on 2026-08-28. The performance category was exercised only as a
host-side cached-read microbenchmark; the specifically planned child-initramfs
comparison remains incomplete. See
[`DAXFS-IMPLEMENTATION.md`](DAXFS-IMPLEMENTATION.md) for the result matrix and
[`evidence/daxfs-20260828/`](evidence/daxfs-20260828/) for raw proof.

The result confirms different kernel binaries per Docker-derived workload.
Docker supplies the OCI root filesystem; Kerf/DAXFS and Multikernel launch it
under a selected child kernel. This is distinct from an ordinary Docker
container, which shares the Docker host kernel.

Remaining experiments are:

1. Design and test explicit cross-kernel VFS cache invalidation before using
   DAXFS with multiple writers.
2. Resolve the pinned kernel's AF_VSOCK compile incompatibility, then test
   AF_VSOCK between primary and child kernels.
3. Add a host-side proxy to provide controlled external networking.
4. Test stop/start and GCE host-maintenance behavior.
5. Only then investigate virtual-device assignment.
6. Capture a stopped, validated boot disk as a reusable GCE custom image.
7. Directly compare DAXFS with the existing child-initramfs baseline.
8. Automate reconstruction of the alternate kernel, matching DAXFS module,
   and initramfs before rerunning the dual-kernel target on a fresh VM.

The two-simultaneous-child lifecycle test formerly listed here has already
passed and is recorded in `LEARNINGS.md` and `TASKS.md`.

Do not treat a successful primary-kernel boot as proof that the Multikernel
runtime works. The reusable image should be created only after the child smoke
test and resource-return path both pass.

## Open questions

- Are APIC IDs stable across stop/start and different instances made from the
  same custom image?
- What happens to active child kernels during GCE live migration?
- Does `v7.0-mk2` interact safely with GCE ballooning and memory reporting?
- What is the supported external-network design for children on current code?
- How should monitoring distinguish the primary and child kernels?
- What security boundary does the current implementation actually guarantee
  when all kernels share one outer GCE VM?

These require separate measurement; the completed run does not establish
behavior outside the tested topology and lifecycle.

## References

- [Multikernel Linux `v7.0-mk2` release](https://github.com/multikernel/linux/releases/tag/v7.0-mk2)
- [Multikernel Linux source](https://github.com/multikernel/linux/tree/v7.0-mk2)
- [Multikernel Kconfig](https://github.com/multikernel/linux/blob/v7.0-mk2/kernel/multikernel/Kconfig)
- [MKTTY Kconfig](https://github.com/multikernel/linux/blob/v7.0-mk2/drivers/tty/Kconfig)
- [Kerf](https://github.com/multikernel/kerf)
- [DAXFS](https://github.com/multikernel/daxfs)
- [Official Multikernel getting-started page](https://multikernel.io/getting-started.html)
- [GCE requirements for custom Linux kernels](https://docs.cloud.google.com/compute/docs/images/building-custom-os)
- [GCE guest environment](https://docs.cloud.google.com/compute/docs/images/guest-environment)
- [GCE serial-console troubleshooting](https://docs.cloud.google.com/compute/docs/troubleshooting/troubleshooting-using-serial-console)
- [Shielded VM and Secure Boot](https://docs.cloud.google.com/compute/shielded-vm/docs/shielded-vm)
- [Creating a VM from a custom image](https://docs.cloud.google.com/compute/docs/instances/create-vm-from-custom-image)
