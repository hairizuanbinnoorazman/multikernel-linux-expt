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
Containerd remains responsible for pulling images, applying OCI layers, and
preparing snapshot/rootfs mounts; the shim passes those inputs and `config.json`
through the runtime contracts instead of building a guest OS or boot image.

`Pause`, `Resume`, and `Stats` are supported extensions to this minimum
surface. Task `Update` and `Checkpoint` are deliberately not advertised by
this gate and return `UNIMPLEMENTED` before contacting the child or mutating
state. CPU and memory ownership is fixed for a sandbox generation at Create;
changing it in place would violate Kerf's allocation contract. The selected
Multikernel/Kerf interface also has no checkpoint/restore primitive from which
an OCI checkpoint with defensible storage, network, and process identity could
be built. Those methods require a future versioned contract rather than a
partial compatibility claim.

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
- Pull a stock `linux/amd64` BusyBox image with containerd, then use the
  Multikernel Runtime v2 shim to run `/bin/echo`, `/bin/sh`, and a nonzero-exit
  command from that image.
- Prove the BusyBox executable and libraries come from the OCI root while the
  reported kernel release comes from the selected runtime kernel manifest.

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

After G6 passes with standalone containerd, register the same shim as an
alternative Docker Engine runtime and repeat create, run, exec, signal, wait,
remove, daemon-restart, and cleanup checks. Docker compatibility is a follow-up
acceptance layer; it does not introduce an ISO or a second child OS build.
