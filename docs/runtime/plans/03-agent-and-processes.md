# Plan 03: Child agent and OCI process lifecycle

## Purpose

Move from “boot a root filesystem” to “manage container processes correctly.”

## Minimum implementation

Build a static `mk-agent` into a minimal child initramfs. It must:

- establish a versioned, authenticated AF_VSOCK session;
- report agent and kernel capabilities;
- mount `proc`, `sysfs`, `devtmpfs`, cgroup v2, and the sandbox root;
- apply the G3 OCI subset: user, supplementary groups, environment, working
  directory, terminal mode, and process-group signals;
- reject namespaces, capabilities, rlimits, cgroups/resources, mounts,
  seccomp, hooks, path masks, read-only roots, and `noNewPrivileges` before
  allocation;
- implement `CreateProcess`, `StartProcess`, `ExecProcess`, `SignalProcess`,
  `WaitProcess`, and `DeleteProcess`;
- stream stdout and stderr independently through bounded reads, with bounded
  producer backpressure and explicit truncation if a reader stalls; and
- initiate clean filesystem quiescence and poweroff.

The initramfs is runtime-owned bootstrap infrastructure, not the container
userspace. After it verifies and mounts the sandbox root, `mk-agent` must apply
the caller's OCI `config.json` to processes whose executables and libraries
come from that root. It must not expect the OCI image to contain a kernel,
bootloader, systemd, or a particular distribution layout.

G3 validates the direct-bundle agent, transport, lifecycle, and fail-closed OCI
boundary. Namespace/capability/rlimit/cgroup application is a G6 production-
container requirement, not a G3 exit requirement. This split is intentional:
silently accepting those fields would be unsafe, while requiring the entire
containerd OCI surface here makes the direct-agent gate duplicate G6. Every
unsupported field remains a hard error before allocation.

## Tests

- PID 1 exits 0, exits nonzero, crashes, and ignores `SIGTERM`.
- Correct argv without shell interpretation.
- Environment, cwd, UID/GID, and supplementary groups.
- Fail-closed read-only/masked paths, capabilities, `noNewPrivileges`, rlimits,
  namespaces, and cgroup resources.
- Interactive terminal resize and non-terminal split stdout/stderr.
- Concurrent `exec`, signal delivery, exit-code preservation, and wait races.
- Agent disconnect and reconnect policy.
- Protocol replay, stale generation, oversized frame, malformed message, and
  unauthenticated peer rejection.
- Controller request-write and response-read deadlines, plus complete
  process-group cleanup for controller-owned reconnect relays.
- A minimal BusyBox OCI bundle whose `/bin/busybox` comes from the bundle root,
  while `uname` proves the separately selected child kernel is running.
- Wrong image architecture and unsupported required kernel/OCI features fail
  before the configured process starts.

## Compatibility record

Maintain an OCI feature matrix with `supported`, `rejected`, and `not tested`
states. The MVP may omit checkpoint/restore, seccomp notify, device injection,
and advanced hooks, but it must report the omission.

## Gate G3

Pass when a directly supplied OCI bundle runs as a managed process in one
child, lifecycle and stdio semantics are deterministic, unsupported OCI fields
fail closed, and orderly agent shutdown returns control to the daemon.

The 2026-08-31 run is a provisional direct-bundle milestone, not closure of
this gate. Later work implemented `ExecProcess`, stdin/attach, PTYs, resize,
process-group signals, bounded streaming, reconnect, actual capability
reporting, and agent-driven shutdown. G3 closes only after their focused and
live matrices pass. Namespace/capability/rlimit/cgroup application remains
mandatory for G6 production container support; G3 proves that those fields are
rejected rather than discarded. The policy and protocol schemas must stay
synchronized with the implemented method and terminal evolution.
`mk-agentctl` applies a 30-second deadline to each complete framed exchange,
including negative authentication/framing probes, and starts owned reconnect
relays in dedicated process groups so teardown kills and reaps descendants.
