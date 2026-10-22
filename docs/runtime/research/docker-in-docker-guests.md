# Docker-in-Docker in a multikernel guest

The [deployment constraint](../architecture.md#deployment-and-containment-constraint)
excludes nested virtualization inside the target cloud instances. This design
therefore uses direct Multikernel children, not KVM/microVM guests inside the
instance. The host engine manages outer workloads; inner builds must use only
the child-local Docker engine and storage. The primary and children remain one
trust domain after arbitrary child-kernel compromise; successful inner builds
and workload restrictions do not prove that stronger containment boundary.

An experimental approved Docker guest starts its own rootless Docker Engine.
The client uses the normal local socket, so user commands are ordinary Docker:

```sh
docker build -t my-image .
docker run --rm my-image
```

No `--privileged`, capability flags, security overrides, `DOCKER_HOST` export,
or primary Docker socket is required by the user. Namespace, storage and daemon
configuration are prepared automatically by guest bootstrap. This is a dedicated
guest-image prototype, not yet a default feature of the containerd/Docker shim.

The [production profile contract](../contracts/docker-guest-profile.md) reserves
UID 0 for bootstrap and requires workload UID/GID 1000. Its supervised activation
and child build integration are still being qualified.

## What runs where

The diagnostic prototype described here runs its outer workload client as guest UID 0 with zero effective/bounding
capabilities and mandatory no-new-privileges. The guest-local Docker daemon and
its containers run in a RootlessKit user namespace: namespace root maps to guest
UID/GID 1000, with subordinate IDs starting at 100000. Bootstrap creates this
namespace before chroot and enables NNP before executing `dockerd`.

The Docker socket is `/var/run/docker.sock` inside this dedicated guest. It is
mode 0666 for the restricted client and does not refer to the primary daemon.
There is no TCP API listener. Any process that can access the local socket can
control this guest-local engine; the guest is for one workload/trust domain.

The approved image includes:

- Pinned `docker:29-dind-rootless` filesystem and standard mapping helpers. The
  exported initramfs restores root-owned setuid helper permissions during trusted
  image preparation, before workload startup; NNP prevents workload exec gains.
- A root mount point retaining restricted devices/procfs/sysfs/run and read-only
  guest cgroup2 discovery. The profile does not delegate writable cgroups.
- Trusted guest-bootstrap loading of pinned `llc`, `stp`, `bridge` and `veth`
  modules. Workloads receive no module-loading capability or memory/I/O devices.
- Docker's VFS storage driver and classic image store, avoiding an overlay/FUSE
  dependency for this diagnostic profile.
- A RootlessKit network namespace and a normal Docker bridge inside it. This
  fixture is offline: forwarding, masquerading and iptables are disabled, and
  RootlessKit uses its `none` network backend.

The daemon config and namespace setup are platform defaults, not user build
flags. Internal setup still exists; a separate kernel does not automatically
configure a usable Docker service. Rootless Docker's upstream requirements are
documented in [Docker's rootless guide](https://docs.docker.com/engine/security/rootless/)
and [Docker-in-Docker tips](https://docs.docker.com/engine/security/rootless/tips/).

## Verification

The [evidence bundle](../../../evidence/runtime-20261012/g7-dind/README.md)
records Docker Engine 29.9.0 on the pinned multikernel kernel, with the unchanged
production agent and private credential handoff. The bounded matrix checks:

- Direct daemon startup under the ordinary restricted root workload, preserving
  its actual outcome rather than assuming that UID 0 grants administrative caps.
- Automatically prepared rootless daemon, `docker info`, standard multi-stage
  `docker build` with `COPY`, `RUN`, and `COPY --from`, and a cache-hit repeat.
- Default `docker run --rm` of the built image and the expected generated output.
- Nested-container UID mapping, NNP and default seccomp observations.
- `docker save`, independent archive/config/layer digest and file-content checks,
  and a deliberately failed build that must not be reported as successful.
- Client privileges, command-line credential visibility, the exact bootstrap
  canary route, restricted devices, image/container cleanup and full cloud cleanup.

The [live harness](../../../scripts/test-runtime-dind-live.sh) uses the shared
builder diagnostic and [image constructor](../../../scripts/build-docker-probe-initramfs.sh).
The [offline verifier](../../../scripts/verify-g7-dind-evidence.py) checks
source/artifact identity, Docker save contents, observed authority and cleanup.

## Remaining scope

This proves offline Docker-in-Docker functionality on an initramfs-backed guest.
Registry pulls, package downloads, network egress, port publishing, mediated
persistent storage, cache durability, restart, concurrency and resource-limit
enforcement need separate qualification. Rootless cgroup limits are not established
by a read-only cgroup mount. Stock-image startup without this approved bootstrap
profile is not qualified by the prepared-daemon result.

The subsequent [online Python prototype](docker-online-python-guest.md)
qualifies a bounded Docker Hub pull, HTTPS PyPI installation and runtime HTTPS
request in a dedicated online diagnostic profile. The remaining production
network/storage/lifecycle limits above still apply.

The [approved Docker profile contract](../contracts/docker-guest-profile.md) now
validates operator approval and implements a supervised bootstrap path. The
[production builder proof](../../../evidence/runtime-20261022/g8-profile-boot/README.md)
qualifies construction, and the
[owned mount proof](../../../evidence/runtime-20261022/g8-state-mount/README.md)
qualifies real-engine state cleanup in private primary namespaces. The
[direct-child boot proof](../../../evidence/runtime-20261022/g8-direct-child-boot/README.md)
now qualifies production mediated-root startup and clean shutdown. The
[production-profile build proof](../../../evidence/runtime-20261022/g8-production-docker-build/README.md)
subsequently passes offline multi-stage builds, a cache repeat, built-image run,
failed-build rejection and independently verified saved-image contents on that
mediated child. Host Docker integration still requires qualification.

Production integration requires an explicit approved Docker profile, immutable
bootstrap inputs, daemon/socket lifecycle and authorization review, and the usual
storage/network compatibility matrix. No generic privileged startup hook was
added to production `mk-agent-init`, and its workload capability/NNP policy is
unchanged. The [G7 architectural limitation](g7-security-boundary-assessment.md)
after guest-kernel compromise remains; this run does not establish containment
of a malicious guest kernel or superiority over gVisor.

The [host Docker lifecycle checkpoint](../../../evidence/runtime-20261022/g8-host-docker-lifecycle/README.md)
now qualifies host Docker selecting the current runtime and running this offline
build workload in a mediated direct child, followed by full disposable cleanup.
It uses independent daemon state, explicit supported OCI settings and the
approved xattr-free import archive. Recovery, concurrency and adversarial release
qualification remain open. The raw network snapshots and invocation-bound
workload/final receipts are retained; this does not close complete G7/G8 gates.

The [graceful runtime-restart proof](../../../evidence/runtime-20261022/g8-host-docker-runtime-recovery/README.md)
subsequently keeps the Docker child live across one private runtime process
restart. Child boot ID, local engine ID, saved-image digest and ownership
generation remain identical; final teardown and raw primary/network comparisons
pass. Crash/power loss, other service restarts, service privilege bounds and
complete G7/G8 acceptance remain open.
