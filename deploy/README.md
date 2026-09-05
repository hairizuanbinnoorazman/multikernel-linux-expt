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
sudo install -d -m 0755 /etc/docker
sudo install -m 0644 deploy/docker/daemon.json /etc/docker/daemon.json
sudo systemctl daemon-reload
sudo systemctl enable --now sys-fs-multikernel.mount mkruntimed.service mknetd.service
sudo systemctl restart containerd docker
```

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
