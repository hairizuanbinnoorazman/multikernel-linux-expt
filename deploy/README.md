# Runtime deployment examples

These files contain no project, account, credential, or operator-specific home
directory. Install and adapt them explicitly on a qualified host:

```bash
sudo install -d -m 0755 /etc/multikernel
sudo install -m 0600 deploy/systemd/runtime.env.example \
  /etc/multikernel/runtime.env
sudo install -m 0644 deploy/systemd/sys-fs-multikernel.mount \
  deploy/systemd/mkruntimed.service /etc/systemd/system/
sudo install -d -m 0755 /etc/docker
sudo install -m 0644 deploy/docker/daemon.json /etc/docker/daemon.json
sudo systemctl daemon-reload
sudo systemctl enable --now sys-fs-multikernel.mount mkruntimed.service
sudo systemctl restart containerd docker
```

Set `MK_PROJECT` in the operator environment for GCE commands. Authentication
comes from `gcloud` or the VM's attached identity and is never stored here.
