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

## 6. Register the opt-in containerd runtime

Confirm the host's main containerd configuration imports
`/etc/containerd/conf.d/*.toml`, then install only the supplied fragment:

```bash
sudo containerd config dump | sed -n '1,30p'
sudo install -d -m 0755 /etc/containerd/conf.d
sudo install -m 0644 deploy/containerd/20-multikernel-runtime.toml \
  /etc/containerd/conf.d/20-multikernel-runtime.toml
sudo containerd config dump | grep -A16 'runtimes.multikernel'
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
