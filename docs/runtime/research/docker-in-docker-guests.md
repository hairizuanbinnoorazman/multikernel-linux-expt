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

## Disposable host Docker qualification

The production-profile laboratory fixture uses separate Docker/containerd/runtime
state and a private mount namespace. Its own dockerd also needs a private network
namespace: `bridge=none` alone does not protect a pre-existing primary `docker0`.
The [retained bridge failure](../../../evidence/runtime-20261023/g8-primary-bridge-failure/README.md)
records that concrete effect while preserving successful child builds, survivor
continuity and export teardown. `40baa83` isolates only the disposable dockerd's
netlink operations. Containerd, Kerf and mknetd remain in the primary network
namespace so the intended child/primary network policy is still exercised.
The strict raw links/routes/firewall comparison remains required.

The fixture now requires its exact systemd unit name in `MK_TEST_UNIT`, verifies
its own PID and invocation ID against that unit, and requires `PrivateMounts=yes`
and `KillMode=process`. The latter matches the production runtime unit's storage
helper survival contract. A default cgroup-killing test unit can destroy accepted
NBD sessions during failure cleanup and cannot qualify restart adoption.
Normal fixture release uses host Docker's authenticated signal path and a shell
trap; it does not require an exec command to complete while simultaneously
terminating the main process. Docker documents exec lifetime as bounded by the
[container's primary process](https://docs.docker.com/reference/cli/docker/container/exec/).
The separate exec/exit reliability boundary remains open.

These are disposable laboratory fixtures with pinned local artifact directories,
not the fresh-checkout operator workflow required for full G8. Direct children
require no nested virtualization and remain in the primary kernel trust domain.

The subsequent [namespace-isolated qualification](../../../evidence/runtime-20261023/g8-concurrent-netns/README.md)
passes two overlapping production-profile child build matrices, separate engine
and storage observations, sibling removal with survivor continuity, synced/checked
export release and the unchanged strict network comparison. Invocation-bound
source/workload/final records and the raw native-server logs are retained. This
is one bounded successful run; prior failures remain retained and full release
reliability still requires its other cases.

The [private runtime SIGKILL proof](../../../evidence/runtime-20261023/g8-runtime-sigkill/README.md)
subsequently kills the control plane after the first build matrix while keeping
the Docker child live. The replacement listener is checked by Unix peer PID,
not socket-path existence. Child identity and the original archive survive;
a fresh build/cache/run/failure/save matrix succeeds after adoption, followed
by checked export release and primary/resource restoration. The bounded case
has separate exit, recovery and final receipts. Crashes during resource mutation
or active builds, other services, host power loss and pressure remain separate
open cases; full G7/G8 does not close.

The [executing-RUN crash checkpoint](../../../evidence/runtime-20261023/g8-runtime-active-run/README.md)
then moves SIGKILL into a real BuildKit RUN's deliberate 45-second sleep window.
Raw BEGIN-only snapshots before kill, after exit and after adoption bind that
window; END and successful first/fresh build matrices follow. The first saved
archive is created after recovery, so no pre-crash archive preservation is
claimed in this case. Child boot, engine and ownership persist, and teardown
checks pass. Storage write pressure, resource mutations, other crash points
and the complete reliability/security gates remain open.

The [storage-pressure fixture](../../../evidence/runtime-20261023/g8-pressure-io-failure/README.md)
attempts bounded exhaustion of the 2 GiB mediated filesystem, holding ENOSPC while
the primary checks backing geometry and installed state before resuming builds.
Its first run instead encounters I/O errors before exhaustion. The native server
log has a synced close but no request refusal, and a read-only filesystem check
subsequently reports uncorrected errors. The [failed disk quarantine](../../../evidence/runtime-20261023/g8-pressure-quarantine/README.md)
preserves bytes/ledgers and returns host resources. No pressure recovery pass or
speculative request-size diagnosis is claimed.

The native client's initramfs log is copied into the root during startup; late
client failures are not retained there. Diagnosis needs bounded client/kernel
observations collected while the child is live, before failure teardown loses
them. The next remediation must distinguish socket/proxy, NBD, filesystem and
memory failure evidence, then repeat the original pressure requirements. Keep
the fixed image geometry, strict ENOSPC/space restoration checks, post-pressure
build matrix, native counters and offline-check release hold intact.

The [bounded console repeat](../../../evidence/runtime-20261023/g8-pressure-console-failure/README.md)
now retains NBD send/receive errors before journal abort. A
[client-instrumented repeat](../../../evidence/runtime-20261023/g8-pressure-client-timeout/README.md)
subsequently captures an NBD timeout, local-TCP EOF, ioctl return 0, and explicit
proxy SIGTERM before the host workload deadline fails. Its cleanup also fails.
The user's next [external restart](../../../evidence/runtime-20261024/g8-pressure-external-restart/README.md)
returns resources; it does not qualify automatic recovery. Bounded
[server I/O timing controls](../../../evidence/runtime-20261024/g8-server-io-controls/README.md)
now cover payload, backing file, sync and reply phases. The timing-only change
preserves the original request/timeout limits and pressure requirements. Physical
pressure qualification and full G7/G8 remain open; no nested virtualization is used.

The [server-timed repeat](../../../evidence/runtime-20261024/g8-server-io-pressure-failure/README.md)
retains slow WRITE payload receives, subsequent NBD timeout and failed exec/cleanup.
Its [operator recovery](../../../evidence/runtime-20261024/g8-server-io-quarantine/README.md)
restores raw primary network state and resources without rewriting the failed stores.
[Per-phase aggregates](../../../evidence/runtime-20261024/g8-phase-summary-controls/README.md)
now prevent a full detail budget from hiding all later phase maxima. Read-only
queue observation is implemented at bootstrap; it awaits live qualification.
No queue or timeout policy change is applied, and pressure recovery stays open.

The [981bc2a clean-boot run](../../../evidence/runtime-20261024/g8-pressure-recovery-cleanup-failure/README.md)
now proves zero-space ENOSPC and full-capacity recovery with the same engine and
complete build matrix. [Read-only archive inspection](../../../evidence/runtime-20261024/g8-pressure-saved-image/README.md)
verifies the child-built result, input and layer diff IDs. Normal cleanup fails:
the existing strict close parser does not recognize native I/O timing records.
The exact transcript is a failing baseline regression and passes with
[f2d8439's bounded grammar validation](../../../evidence/runtime-20261024/g8-native-close-timing-controls/README.md).
Unknown, duplicate, missing, contradictory or reordered records still refuse release.
[Quarantine proof](../../../evidence/runtime-20261024/g8-pressure-recovery-quarantine/README.md)
returns resources while retaining the failed ledgers and clean recovered disk.
The [same-boot startup failure](../../../evidence/runtime-20261024/g8-warm-start-failure/README.md)
remains unresolved; a [pinned repeatable trial](../../../evidence/runtime-20261024/g8-trial-identity-controls/README.md)
now binds each source revision/archive before module mutation. See the earlier
[cold restoration proof](../../../evidence/runtime-20261024/g8-cold-module-restoration/README.md)
for the no-hot-unload policy. These are direct child-kernel tests with no nested
virtualization. Fixed-runtime live cleanup acceptance and full G7/G8 remain open.

The [fixed-runtime live case](../../../evidence/runtime-20261024/g8-pressure-recovery-qualified/README.md)
and [same-boot repeat](../../../evidence/runtime-20261024/g8-pressure-warm-repeat/README.md)
now qualify bounded zero-space ENOSPC recovery, builds and normal automatic cleanup.
Both release the exact storage generation after clean offline checking, return
CPU/pool/network/child resources and retire VSOCK references without changing
installed service PIDs/files or raw network state. The repeat reuses CPUs/CID with
a fresh engine/filesystem and no transport reload, as confirmed by the primary
registration journal. Previous failed-recovery startup behavior and broader
G7/G8 exit requirements remain unqualified. Cold restoration is separate from
normal cleanup; these direct child-kernel tests use no nested virtualization.
