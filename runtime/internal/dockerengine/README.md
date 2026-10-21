# Child Docker engine supervision

`mk-agent --docker-guest` launches one fixed `/docker-bootstrap` from its trusted
outer root, through `/bin/busybox sh` and an inherited descriptor. It refuses
unsafe bootstrap metadata/interpreter lines and uses a fixed environment,
cgroup name, socket path, PID limit (512), and RAM ceiling (512 MiB). No OCI or
RPC field selects a service command. The caller must install approved immutable
outer-root helpers and enable the parent memory/pids controllers before launch.

The bootstrap must remain foreground, exec its rootless launcher, and run the
engine as guest UID 1000. The historical diagnostic bootstrap exits on socket
creation and is incompatible with this lifecycle. Production builder activation
still refuses until immutable helper preparation, copied-image verification and
boot integration are implemented and qualified together.

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
