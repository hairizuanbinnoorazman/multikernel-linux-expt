# Runtime deployment examples

These files contain no project, account, credential, or operator-specific home
directory. Install and adapt them explicitly on a qualified host:

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
sudo install -m 0755 runtime/bin/mk-cni /opt/cni/bin/multikernel
sudo systemctl daemon-reload
sudo systemctl enable --now sys-fs-multikernel.mount mkruntimed.service mknetd.service
```

The shim name follows containerd's Runtime v2 binary convention, so `ctr` can
select `io.containerd.multikernel.v2` directly once the binary is on the daemon
`PATH`; do not change containerd's default runtime. For Docker versions that
require explicit registration, merge the opt-in entry without overwriting any
existing daemon settings:

```bash
work=$(mktemp -d)
candidate=$work/daemon.json
./scripts/merge-runtime-docker-config.py /etc/docker/daemon.json "$candidate"
sudo dockerd --validate --config-file "$candidate"
sudo install -d -m 0755 /etc/docker
if test -e /etc/docker/daemon.json; then
  sudo cp --preserve=mode,ownership,timestamps /etc/docker/daemon.json \
    /etc/docker/daemon.json.pre-multikernel
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
that contract. `mkruntimed` performs snapshot mounting and deterministic root
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
