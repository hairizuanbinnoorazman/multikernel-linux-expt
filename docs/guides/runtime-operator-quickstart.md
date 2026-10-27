# Runtime operator quickstart

This guide takes a qualified, disposable Multikernel host from a clean source
checkout to one completed container. It does not provision the GCE VM or build
the primary kernel; use the [GCE laboratory runbook](gce-lab-runbook.md) for
that work.

The runtime is an experimental, opt-in runtime for trusted single-tenant
nodes. Do not make it the default containerd or Docker runtime. Commands that
start services, create child kernels, or modify container configuration should
be run only on an otherwise idle disposable host.

## Preconditions

Before installing anything, confirm all of the following:

- the host is running the pinned Multikernel kernel and has working serial
  recovery;
- Kerf is installed at the pinned revision;
- the proposed child APIC IDs contain whole SMT cores, exclude APIC ID 0, and
  leave at least four CPUs and 8 GiB with the primary;
- `/sys/fs/multikernel` is mounted;
- a dedicated ext4 storage filesystem is mounted at the intended runtime
  storage path; and
- the kernel, initramfs, agent, relay, and transport-module artifacts have a
  validated manifest as described in
  [Kernel and bootstrap artifacts](kernel-and-bootstrap-artifacts.md).

Never assign the primary NIC, boot disk, or their PCI controllers to a child.

## 1. Validate the source tree

Work from the repository root. A release build must come from a clean checkout
so all components receive the real source revision instead of `unknown`.

```bash
make docs-check
make runtime-test
make runtime-build
git status --short
```

Run the race detector separately before producing the release:

```bash
cd runtime
GOCACHE=/tmp/mk-go-cache go test -race ./...
GOCACHE=/tmp/mk-go-cache go vet ./...
cd ..
```

## 2. Qualify the host without changing it

`mk-host-check` is read-only. Supply the exact pool proposed for the runtime so
the report includes Kerf's allocation dry-run:

```bash
runtime/bin/mk-host-check \
  --kerf=/absolute/path/to/kerf \
  --probe-pool-cpus=8,10,12,14,9,11,13,15 \
  --probe-pool-memory=16GB >host-report.json
```

Stop if the command fails or reports unknown critical state, existing child
instances, unsafe device ancestry, insufficient primary resources, or an
invalid CPU topology. Do not turn a failed qualification into a warning.

## 3. Build and identify one release

Choose an explicit version and use the clean checkout's full commit:

```bash
runtime_revision=$(git rev-parse HEAD)
test -z "$(git status --porcelain --untracked-files=normal)"
make runtime-manifest \
  RUNTIME_VERSION=0.1.0-dev \
  RUNTIME_REVISION="$runtime_revision"
```

`runtime-manifest` exclusive-creates `runtime/release-manifest.json`. If that
path already exists, inspect and preserve it or move it out of the way; do not
silently overwrite release evidence.

Install the verified binaries into an immutable release and inspect the active
selection:

```bash
sudo ./scripts/manage-runtime-binaries.py install \
  runtime/release-manifest.json runtime/bin
sudo ./scripts/manage-runtime-binaries.py inspect
```

The installer creates stable command links but does not start services or
change the default container runtime.

## 4. Prepare runtime configuration

Copy `deploy/systemd/runtime.env.example` to a private staging path and replace
every example APIC ID, storage identity, egress interface, and resolver with
values qualified on this host. Do not put credentials in this file.

Create a strict host configuration from the documented
[configuration contract](../runtime/contracts/configuration.md) and
[`host-config.valid.json`](../runtime/contracts/fixtures/host-config.valid.json).
In particular, use absolute paths and make the kernel-manifest directory point
to the validated manifest installed in the previous prerequisite.

Before starting the daemon, validate the dedicated storage mount using the
same identity values placed in `runtime.env`:

```bash
sudo ./scripts/validate-runtime-storage-mount.py \
  --mountpoint /srv/multikernel-storage \
  --device /dev/disk/by-id/YOUR-RUNTIME-DISK \
  --serial YOUR-DISK-SERIAL \
  --bytes YOUR-EXACT-BYTE-SIZE \
  --label YOUR-EXT4-LABEL \
  --uuid YOUR-EXT4-UUID
```

Install the service configuration as an immutable deployment generation:

```bash
sudo ./scripts/manage-runtime-deployment.py install \
  /path/to/runtime.env /path/to/config.json
sudo ./scripts/manage-runtime-deployment.py inspect
```

### After an approved storage grow

The mount guard binds the exact raw-device byte count. After an independently
approved disk/filesystem grow, validate the same device, mount, serial, label and
UUID with the new byte count using the command above. Preserve the retained-image
hash/identity evidence before changing the binding. Then clone the active
immutable deployment with only its size assignment changed, for example for an
approved 30-to-50 GiB grow:

```bash
sudo ./scripts/manage-runtime-deployment.py update-storage-size 32212254720 53687091200
sudo ./scripts/manage-runtime-deployment.py inspect
```

Use the actual old/new byte counts for the host. The command verifies the active
generation and asset protections, preserves its other bytes/modes and old
generation, and atomically selects the clone. Stale expected values, shrinking,
non-sector-aligned sizes and unsafe or mode-drifted generations refuse. It can
run as a standalone script without importing current repository assets. It does
not resize storage or restart services. Inspect the resulting generation. During the authorized idle maintenance window,
run `sudo systemctl daemon-reload` and `sudo systemctl restart mkruntimed.service`
to load the new binding, then verify its active state and mount-guard result.
For a first installation, use the next section instead. Do not edit an immutable
generation in place. The
[qualified GCE recovery proof](../../evidence/runtime-20261027/g8-live-static-restoration/README.md)
retains the old-size refusal and new-size validation with the same storage identity.

## 5. Start the runtime services

The explicit activation step is an opportunity to check paths one final time:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now \
  sys-fs-multikernel.mount mkruntimed.service mknetd.service
systemctl --no-pager --full status mkruntimed mknetd
```

If either service fails, stop here and use
[Runtime recovery and cleanup](runtime-recovery-and-cleanup.md). Do not edit
runtime state files to force startup.

With a qualified daemon/client build supporting `native-resource-identity-v1`,
inspect recorded resource ownership and sampled native export identity through
the protected daemon socket, without invoking recovery or Kerf:

```bash
sudo mkruntimed --config=/etc/mkruntime/config.json --doctor
```

Version-3 JSON reports `consistent`, `degraded` or `inconclusive`; the latter two
exit 1. It correlates explicit storage expectations, lease generations, CID,
complete descriptor fingerprints and exact native export process/backing identity.
Recheck changing observations and intermediate states. An ACTIVE lease with a
missing record or process is degraded; unavailable identity is inconclusive.
Released history is explicitly not probed and is not treated as a live orphan.
Keep sandbox and export generations with findings. These are inspection hints,
never cleanup authorization; follow the recovery guide before any action.

`NATIVE_RESOURCE_IDENTITY_UNSUPPORTED` distinguishes a valid older capability
reply from `NATIVE_RESOURCE_IDENTITY_UNAVAILABLE`. There is no partial legacy
fallback. Early failures retain the initial `boot_id` with `boot_stable: false`.
Matching names cannot prove the kernel incarnation, and matched native identity
cannot prove socket/session health, I/O progress, storage contents or containment.
A missing record can coexist with a live, serving export. The
[negative-case proof](../../evidence/runtime-20261025/g8-live-native-record-loss/README.md)
keeps child Docker working while diagnostics correctly degrade. Do not clear
leases, stop an export or reconstruct a record from a guessed PID based on this
finding; follow the recovery guide. The lab test restores a known held inode
and is not a production repair procedure.
Private-file type validation also refuses an unpaired FIFO promptly;
[the live FIFO proof](../../evidence/runtime-20261025/g8-live-fifo-record/README.md)
returns inconclusive native-identity findings without changing ownership or
interrupting child Docker. This fixes FIFO open rendezvous, not every possible
filesystem/device stall, and does not authorize replacing malformed metadata.
Those limits remain explicit in `unobserved` and the
[native identity contract](../runtime/contracts/native-resource-identity.md).
The [live GCE proof](../../evidence/runtime-20261025/g8-live-native-resource-identity/README.md)
uses a private qualified stack during child Docker ENOSPC/build/cleanup; installed
legacy services remain unchanged. Older
[installed-daemon](../../evidence/runtime-20261025/g8-runtime-doctor/README.md) and
[compatible-daemon](../../evidence/runtime-20261025/g8-compatible-daemon-doctor/README.md)
proofs describe earlier diagnostic versions. Direct children use no nested
virtualization and share the primary's trust domain after child-kernel compromise.

With a qualified client and daemon supporting `storage-ownership-v1`, inspect
recorded storage leases through the same protected socket:

```bash
sudo mkruntimed --config=/etc/mkruntime/config.json --inspect-storage
```

The [bounded metadata contract](../runtime/contracts/storage-ownership.md)
excludes backing paths and credentials. Busy, oversized, unsupported or invalid
views fail rather than return a partial inventory. These are recorded claims;
live-state close counters are not current I/O metrics, and a released record is
not a fresh filesystem check. Follow the recovery guide before any mutation.
The [GCE qualification](../../evidence/runtime-20261025/g8-storage-ownership/README.md)
uses private production daemons; installed legacy services remain unchanged and
do not support this view. [Recorded correlation during live child activity](../../evidence/runtime-20261025/g8-live-resource-correlation/README.md)
now passes. The separately negotiated [version-3 view](../runtime/contracts/native-resource-identity.md)
also samples native process/current-file identity; full socket/session health
remains open. Mismatches are inspection hints, never repair authorization.

## 6. Register the opt-in containerd runtime

Use a separately qualified patched host containerd. The current security
qualification targets upstream **2.2.10** (config version 3); 2.3.7 uses config
version 4. The fragment disables both CRIU and experimental checkpoint restore
through CreateContainer, while retaining the runc default and opt-in Multikernel
handler. Older containerd releases may silently ignore these keys. Confirm both
are false in the **running CRI service's** verbose Status response after restart;
a successful configuration parse alone does not prove enforcement. Keep full
configuration and registry credentials private when collecting that proof.

Run the configuration contract against the exact selected binary:

```bash
containerd_bin=/path/to/qualified/containerd
sudo env MK_CONTAINERD="$containerd_bin" python3 scripts/test-runtime-containerd-config.py
```

The [host containerd security guide](host-containerd-security.md) describes the
qualified service override, private rollback and required live enforcement proof.
Distribution client binaries can remain older than the selected daemon.

Confirm the host's main containerd configuration imports
`/etc/containerd/conf.d/*.toml`, then install only the supplied fragment:

```bash
sudo install -d -m 0755 /etc/containerd/conf.d
sudo install -m 0644 deploy/containerd/20-multikernel-runtime.toml \
  /etc/containerd/conf.d/20-multikernel-runtime.toml
sudo "$containerd_bin" config dump | \
  grep -E 'enable_criu|enable_experimental_restore_via_create|runtimes.multikernel'
```

Do not synthesize a partial main configuration and do not change
`default_runtime_name`. Restart containerd only when the host has no unrelated
tasks. Docker registration is optional and is covered in the
[runtime usage guide](using-the-runtime.md).

## 7. Run the first workload

Pull a pinned test image and select the runtime explicitly:

```bash
sudo ctr images pull docker.io/library/busybox:1.36
sudo ctr run --rm \
  --runtime io.containerd.multikernel.v2 \
  docker.io/library/busybox:1.36 mk-quickstart \
  /bin/sh -c 'uname -r; cat /proc/sys/kernel/random/boot_id; echo runtime-ok'
```

The command should print the child kernel release, a boot ID different from the
primary host, and `runtime-ok`.

## 8. Verify cleanup

After the command exits, check that no test task, child, link, or runtime
firewall rule remains:

```bash
sudo ctr tasks list
sudo ctr containers list
sudo find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 -print
ip -o link show | awk -F': ' '$2 ~ /^mkv[0-9a-f]+$/ {print $2}'
sudo iptables -t nat -S POSTROUTING | grep '172\.31\.' || true
sudo iptables -S | grep '^\(-N\|-A\) MK-' || true
```

Any unexpected output is a failed cleanup, not a successful quickstart.

## Upgrade, rollback, and uninstall

Installing a new release or deployment preserves the prior immutable
generation. List identifiers with each manager's `inspect` command, then roll
back by activating an exact identifier:

```bash
sudo ./scripts/manage-runtime-binaries.py activate RELEASE_IDENTIFIER
sudo ./scripts/manage-runtime-deployment.py activate DEPLOYMENT_IDENTIFIER
sudo systemctl daemon-reload
sudo systemctl restart mkruntimed mknetd
```

`uninstall` and the remove commands are dry runs unless `--apply` is supplied.
An active release or deployment cannot be removed. Review the dry-run JSON
before applying any removal. The detailed ownership and preservation behavior
is documented in [`deploy/README.md`](../../deploy/README.md).
