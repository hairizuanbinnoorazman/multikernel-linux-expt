# Plan 03: Child agent and OCI process lifecycle

## Purpose

Move from “boot a root filesystem” to “manage container processes correctly.”

## Minimum implementation

Build a static `mk-agent` into a minimal child initramfs. It must:

- establish a versioned, authenticated AF_VSOCK session;
- report agent and kernel capabilities;
- mount `proc`, `sysfs`, `devtmpfs`, cgroup v2, and the sandbox root;
- create mount, PID, IPC, UTS, and user namespaces as configured;
- apply OCI user, groups, environment, working directory, capabilities,
  rlimits, and cgroups;
- implement `CreateProcess`, `StartProcess`, `ExecProcess`, `SignalProcess`,
  `WaitProcess`, and `DeleteProcess`;
- stream stdout and stderr independently; and
- initiate clean filesystem quiescence and poweroff.

The initramfs is runtime-owned bootstrap infrastructure, not the container
userspace. After it verifies and mounts the sandbox root, `mk-agent` must apply
the caller's OCI `config.json` to processes whose executables and libraries
come from that root. It must not expect the OCI image to contain a kernel,
bootloader, systemd, or a particular distribution layout.

Do not initially implement every OCI option. Reject unsupported fields
explicitly instead of silently weakening them.

## Tests

- PID 1 exits 0, exits nonzero, crashes, and ignores `SIGTERM`.
- Correct argv without shell interpretation.
- Environment, cwd, UID/GID, supplementary groups, umask, and rlimits.
- Read-only and masked paths.
- Capability add/drop and `noNewPrivileges`.
- cgroup CPU, memory, and PID limits within the child's assigned resources.
- Interactive terminal resize and non-terminal split stdout/stderr.
- Concurrent `exec`, signal delivery, exit-code preservation, and wait races.
- Agent disconnect and reconnect policy.
- Protocol replay, stale generation, oversized frame, malformed message, and
  unauthenticated peer rejection.
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
this gate. The minimum implementation and test lists above remain normative;
in particular `ExecProcess`, independent bounded stdio streaming, configured
namespaces/capabilities/rlimits/cgroups, tested signal semantics, and verified
quiescence must land before G3 is checked in the master plan. The narrower
frozen G3 OCI policy describes which inputs the provisional implementation is
allowed to accept or must reject; it does not waive this plan's exit criteria.
