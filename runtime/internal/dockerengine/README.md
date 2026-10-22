# Child Docker engine supervision

`mk-agent --docker-guest` launches one fixed `/docker-bootstrap` from its trusted
outer root, through `/bin/busybox sh` and an inherited descriptor. It refuses
unsafe bootstrap metadata/interpreter lines and uses a fixed environment,
cgroup name, socket path, PID limit (512), and RAM ceiling (512 MiB). No OCI or
RPC field selects a service command. The caller must install approved immutable
outer-root helpers and enable the parent memory/pids controllers before launch.

The bootstrap must remain foreground, exec its rootless launcher, and run the
engine as guest UID 1000. The fixed candidate is `guest/runtime-docker-bootstrap`; it execs the outer
RootlessKit chain, closes script fd3 and keeps logs on stdout/stderr. Boot
preparation must supply the inner `/run/docker-daemon.json`, bind the fixed
outer `/run/user/1000/rootlesskit` state directory at the same inner path, and
prepare mounts before launch. The historical diagnostic bootstrap exits on socket creation and is
incompatible with this lifecycle. The approved-profile builder/init path now installs the outer generation and
selects this mode; direct-child boot/build qualification is still required.

Startup is bounded to 30 seconds. A Unix connection must have UID 1000 and a
live `SO_PEERPIDFD` peer in the owned non-delegated cgroup. Kernels without that
socket option are refused; there is no numeric-PID fallback. HTTP requests to
`/_ping` and `/info` have bounded response headers/bodies and deadlines, refuse
redirects, and require Linux, `vfs`, the fixed rootless data directory and
`name=rootless`. This verifies daemon identity/configuration and readiness; it
cannot establish honesty of a compromised, previously approved engine.

The launcher receives a kernel-created pidfd at clone time. Logging continuously
drains stdout/stderr into a shared 64 KiB tail, without returning daemon output
in RPC errors. Shutdown signals the held pidfd, allows up to two seconds for
normal engine shutdown, then kills and empties the owned cgroup and waits for
launcher/log reaping. It executes no shutdown helper from the writable OCI root.
Cancellation/failure retains cleanup ownership for retry. The agent must stop
the service before filesystem quiescence; a failed stop prevents remount and
NBD disconnect. Unexpected launcher death cancels serving and triggers cleanup;
requested shutdown leaves time for its authenticated reply.

The opt-in installed tests simulate Docker's readiness protocol as UID 1000.
They exercise peer identity, bounded logs, detached descendants, failed readiness,
canceled/idempotent release, foreign-peer refusal and unexpected death. They are
not real Docker or child-kernel build qualification. Enable only on the testing
instance with `MK_TEST_GUEST_SCOPE_PARENT`, using a staged test binary accessible
to UID 1000. Tests create fresh PID-specific `mk-engine-*` cgroups and never
adopt or repair existing groups.

The [profile contract](../../../docs/runtime/contracts/docker-guest-profile.md)
and [cloud constraint](../../../docs/runtime/architecture.md#deployment-and-containment-constraint)
apply: nested virtualization is unavailable, and process controls do not prove
containment after arbitrary child-kernel compromise. G7/G8 remain open.

The explicit agent path selects `NewDockerGuestManager`, reserving UID 0 for
bootstrap and requiring workload UID/GID 1000, only group 1000 and NNP. Create,
exec and start all enforce identity. Explicit empty non-root capability objects are supported through the sealed
credential executor. Non-empty grants remain refused; complete host Docker/child
integration still requires qualification. See the [scoped process/admission proof](../../../evidence/runtime-20261021/g8-guest-process-protection/README.md).

`TestInstalledRealForegroundBootstrap` uses an explicitly staged pinned real
image in a private primary mount namespace. It exercises the production wrapper,
real API readiness, foreground lifetime and cgroup removal. This opt-in test does
not itself qualify child boot or Docker builds.

The outer helper closure includes `nsenter` and `ip`: RootlessKit configures
loopback even for `--net=none`. Source aliases (including a BusyBox applet) are
flattened to independent files under their required invocation names.

The [real foreground proof](../../../evidence/runtime-20261021/g8-docker-foreground/README.md)
records the pinned Docker engine passing this installed test on the restarted
GCE primary kernel. It retains the failed controls that identified missing outer
helpers, temporary-directory permissions and shared API state. Production boot
integration and actual direct-child builds remain open.

`PrepareFilesystem` now performs the fixed pre-admission filesystem setup used
by `mk-agent --docker-guest`: no-symlink path checks, fresh private API state,
protected inner tmpfs, memory/pids controller enablement, private mount
propagation, read-only inner cgroups/config and fixed data-directory ownership.
The installed real-engine test calls this same function and checks alias refusal,
read-only write attempts and shared state identity before launching the engine.
The builder/init still must install approved outer inputs and load the approved
modules. Preparation failure serves no clients; failed children require teardown,
not adoption or in-place retry.

Prepared filesystems self-bind the outer state directory to prevent RootlessKit
from deleting the shared inode on exit. Both the source and inner bind transfer
owned `open_tree` descriptors into the service. After scope population reaches
zero and the launcher is reaped, Stop moves each captured mount into a root-owned
private cleanup directory, closes its descriptor and normally unmounts it before
storage quiescence. Cleanup follows the owned
mount even if the image-side path was renamed. A busy reference refuses release
and retains retry ownership; no lazy detach or image-selected helper is used.
The installed real-engine test checks rename, busy-reference refusal, retry and
idempotence. The [retained direct-child proof](../../../evidence/runtime-20261022/g8-direct-child-boot/README.md)
subsequently passes real Docker startup, root read-only remount, NBD disconnect
and offline filesystem checking with this production preparation path. Actual
Docker builds and host Docker integration remain separate qualifications.
