# Plan 06: containerd Runtime v2 shim

## Purpose

Make the proven daemon and agent path consumable through standard containerd
interfaces.

## Minimum implementation

Implement `containerd-shim-multikernel-v2` with the smallest required Task
service surface:

- `Create`, `Start`, `State`, `Wait`, `Kill`, and `Delete`;
- `Exec`, `ResizePty`, and process deletion;
- stdio FIFO handling;
- task exit and lifecycle event publication; and
- `Shutdown` with safe shim/daemon ownership transfer.

The shim must remain unprivileged except for access to the daemon socket. It
must never execute Kerf directly. Containerd namespace and sandbox identifiers
must map deterministically to internal IDs without becoming trusted paths.

## Tests

- Unit tests against a fake daemon.
- Containerd shim protocol tests and event ordering.
- `ctr run`, `ctr task exec`, signal, wait, delete, and terminal resize.
- Containerd restart while a task runs.
- Shim crash while a task runs and shim reconnect/recovery.
- Duplicate requests, context cancellation, deadline expiry, and FIFO peer
  disappearance.
- Unsupported OCI configuration fails before resource allocation where
  possible and always cleans up if allocation already occurred.
- Two concurrent sandboxes using disjoint child resources.

## Packaging

- Versioned binary names and configuration.
- A generated containerd configuration fragment.
- Separate developer and privileged-integration test targets.
- No automatic host installation in the default build.

## Gate G6: MVP

Pass when an unmodified OCI bundle can be launched through `ctr`, inspected,
executed into, signaled, waited on, and deleted; containerd or shim restart is
recoverable; and final CPU, memory, storage, network, and process cleanup is
proven.
