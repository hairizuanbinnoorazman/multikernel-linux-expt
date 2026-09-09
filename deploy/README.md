# Runtime deployment examples

These files contain no project, account, credential, or operator-specific home
directory. Build with an explicit release identity and create the deterministic
component manifest before installation:

```bash
revision=$(git rev-parse HEAD)
test -z "$(git status --porcelain --untracked-files=normal)"
make runtime-manifest RUNTIME_VERSION=0.1.0-dev RUNTIME_REVISION="$revision"
```

`runtime-manifest` fails rather than replacing an existing manifest. Each
binary supports `--version`; the generator requires the same exact 40-hex
revision from every regular, executable, non-symlink component before hashing
it. Preserve this manifest alongside the installed hashes in evidence.

Install the verified binaries into an immutable, versioned release directory
and atomically select it:

```bash
sudo ./scripts/manage-runtime-binaries.py install \
  runtime/release-manifest.json runtime/bin
sudo ./scripts/manage-runtime-binaries.py inspect
```

The installer verifies every component identity and hash both before and after
copying. It refuses unrelated files at its command paths, creates stable links
without replacing operator-owned paths, and switches
`/usr/local/lib/multikernel/current` atomically. `mk-agent` is retained in the
release's `bin` directory for deterministic initramfs construction; it is not a
host command. Installing a newer manifest performs an upgrade while preserving
the prior immutable release. Roll back by selecting its exact identifier from
`inspect`, for example:

```bash
sudo ./scripts/manage-runtime-binaries.py activate \
  0.1.0-dev-0123456789abcdef0123456789abcdef01234567
```

`uninstall` and `remove-release` are dry-run/preservation oriented: uninstall
requires `--apply`, removes only managed command links and the active selector,
and preserves all releases. An active release cannot be removed. After
activating another release or uninstalling, remove an exact verified inactive
release with `remove-release IDENTIFIER --apply`.

Create an adapted `runtime.env` and strict host configuration outside the
repository, then install the complete service/config set as an immutable
generation and atomically activate it:

```bash
sudo ./scripts/manage-runtime-deployment.py install \
  /path/to/runtime.env /path/to/config.json
sudo ./scripts/manage-runtime-deployment.py inspect
sudo systemctl daemon-reload
sudo systemctl enable --now sys-fs-multikernel.mount mkruntimed.service mknetd.service
```

The installer validates both operator inputs, hashes every deployed unit,
fragment, rootfs builder/helper, and guest bootstrap script, refuses unrelated
destination paths, and switches all stable links through
`/etc/multikernel/current`. Installing a changed configuration or support tool creates
a new immutable generation; `activate DEPLOYMENT` rolls back atomically.
`uninstall` is a dry run unless passed `--apply` and preserves generations;
an active generation cannot be removed with `remove-deployment`. The reported
activation commands remain explicit because starting privileged services is a
separate operator action and must follow host qualification.

The equivalent manual layout, useful for auditing rather than installation,
is:

```bash
sudo install -d -m 0755 /etc/multikernel
sudo install -m 0600 deploy/systemd/runtime.env.example \
  /etc/multikernel/runtime.env
sudo install -m 0644 deploy/systemd/sys-fs-multikernel.mount \
  deploy/systemd/mkruntimed.service deploy/systemd/mknetd.service \
  /etc/systemd/system/
sudo install -d -m 0755 /etc/cni/net.d /opt/cni/bin
sudo install -m 0644 deploy/cni/10-multikernel.conf \
  /etc/cni/net.d/10-multikernel.conf
sudo systemctl daemon-reload
sudo systemctl enable --now sys-fs-multikernel.mount mkruntimed.service mknetd.service
```

The shim name follows containerd's Runtime v2 binary convention, so `ctr` can
select `io.containerd.multikernel.v2` directly once the binary is on the daemon
`PATH`; do not change containerd's default runtime. CRI callers use the named
`multikernel` handler from
[`containerd/20-multikernel-runtime.toml`](containerd/20-multikernel-runtime.toml).
That import-only fragment contains neither a config version nor
`default_runtime_name`. Install it only after verifying the active main config
imports `/etc/containerd/conf.d/*.toml`:

```bash
sudo containerd config dump | sed -n '1,30p'
sudo install -d -m 0755 /etc/containerd/conf.d
sudo install -m 0644 deploy/containerd/20-multikernel-runtime.toml \
  /etc/containerd/conf.d/20-multikernel-runtime.toml
sudo containerd config dump | grep -A16 'runtimes.multikernel'
```

If the host has no main config, first stage `containerd config default` as the
candidate main config and confirm its `imports` entry; do not synthesize a
partial main config or replace an existing one. Restart only on an idle
disposable host, then verify CRI still reports `runc` as its default and the
new handler separately. Remove only this fragment to roll back.

For Docker versions that
require explicit registration, merge the opt-in entry without overwriting any
existing daemon settings:

```bash
work=$(mktemp -d)
candidate=$work/daemon.json
./scripts/merge-runtime-docker-config.py /etc/docker/daemon.json "$candidate"
sudo dockerd --validate --config-file "$candidate"
sudo install -d -m 0755 /etc/docker
if test -e /etc/docker/daemon.json; then
  sudo cp --no-clobber --preserve=mode,ownership,timestamps \
    /etc/docker/daemon.json /etc/docker/daemon.json.pre-multikernel
fi
sudo install -m 0644 "$candidate" /etc/docker/.daemon.json.multikernel-candidate
sudo mv /etc/docker/.daemon.json.multikernel-candidate /etc/docker/daemon.json
sudo systemctl reload docker
sudo docker info --format '{{json .Runtimes}}'
rm -r "$work"
```

[`docker/runtime.fragment.json`](docker/runtime.fragment.json) shows the exact
entry. The merge tool preserves `default-runtime` byte-for-value at the JSON
value level and rejects conflicting entries. Roll back by running the same
tool with `--remove`, validating the candidate, installing it atomically, and
reloading Docker; if a pre-install copy was retained, restoring that exact file
is the stronger rollback. Do not restart containerd or Docker until their
current task inventories are empty or the restart-continuity matrix is the
explicit purpose of the disposable-host run.

Install a strict `/etc/mkruntime/config.json` from the documented host-config
contract, mount the qualified `mk-mediated-storage` filesystem at
`/srv/multikernel-storage`, and install the rootfs builder, NBD helper, guest
bootstrap scripts, kernel/module manifest, and agent artifacts referenced by
that contract. The deployment manager installs the versioned rootfs builder
and its validation/build helpers plus both guest init scripts. The
host/kernel-specific NBD helper, NBD module, transport module, kernel manifest,
and artifacts referenced by that manifest remain separately provisioned and
must pass bootstrap validation. `mkruntimed` performs snapshot mounting and deterministic root
construction; the shim only submits a bounded preparation request and never
mounts the caller's snapshot itself.

Install `runtime/bin/mknetd` as `/usr/local/sbin/mknetd`, and adapt
`MKNETWORK_EGRESS` and `MKNETWORK_DNS` to the qualified host before starting
the service. Containerd or another orchestrator may invoke CNI `ADD`, `CHECK`,
and `DEL`; `mknetd` binds that exact endpoint to the sandbox generation. For a
standalone `ctr` task without a CNI-created namespace, `mknetd` creates and
owns a generation-named namespace through the same endpoint contract. The
unprivileged shim only requests `PROVISION`, receives the TUN descriptor with
`ATTACH`, and requests `RELEASE` over the authenticated daemon socket.

Set `MK_PROJECT` in the operator environment for GCE commands. Authentication
comes from `gcloud` or the VM's attached identity and is never stored here.
