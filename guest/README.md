# Child guest artifacts

- `init`: minimal CPU, memory, initramfs, and MKTTY bring-up.
- `daxfs-*` and `docker-proof.sh`: DAXFS and Docker-derived root proofs.
- `ext4-bootstrap-init`: bounded rejection of a missing direct ext4 root.
- `mediated-*`: AF_VSOCK transport and primary-mediated ext4 roots.
- `security-authority-probe.sh`: diagnostic workload for G7 privilege and
  synthetic child-bootstrap canary checks. Never included in production images.
- `buildkit-bootstrap.sh`, `buildkit-build`, and `buildkit-probe.sh`: experimental
  approved builder-guest preparation, `mk-build` client, and bounded Dockerfile
  verification. See [guest image builds](../docs/runtime/research/guest-image-builds.md).
- `docker-bootstrap.sh` and `docker-probe.sh`: experimental dedicated rootless
  Docker guest and ordinary nested build/run verification, with no user privileged
  flags. See [Docker-in-Docker guests](../docs/runtime/research/docker-in-docker-guests.md).

See the [experiment asset map](../docs/experiments/ASSET-MAP.md) for the
matching builders, tests, reports, and evidence.

- `docker-online-bootstrap.sh` prepares the experimental rootless Docker daemon
  with slirp4netns and guest-local bridge/NAT modules for outbound networking.
- `docker-online-probe.sh` pulls an official Python base from Docker Hub,
  installs packages from PyPI, builds/runs the small online Python example,
  and records image/package provenance and authority observations.
- `docker-online-nsenter` handles RootlessKit's ip-only namespace invocation
  using absolute iproute2, avoiding BusyBox's minimal ip applet substitution.
