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
map into the daemon's global ID space using a readable prefix plus a digest of
the complete namespace/task tuple. The prefix is never an identity boundary;
the digest prevents namespace, punctuation-normalization, and truncation
aliases without making either caller-controlled value a trusted path.
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

Task events use a versioned, private journal beside the bundle. The shim
persists each typed event and monotonically increasing local sequence before
publication, publishes pending entries in sequence, and acknowledges them with
an atomic rewrite. A transient publication error does not roll back an already
committed lifecycle mutation; the next event or reconstructed worker retries
the journal. A joined background worker also retries once per second while the
shim remains alive, with each attempt bounded to five seconds, so a quiet task
does not require a later lifecycle call to recover from a transient containerd
disconnect. Exit state records whether its exit event was durably queued, and
Delete repairs a missing exit entry before it can enqueue the delete event.
Containerd's forwarding API has no transactional event identifier, so the
contract is **at-least-once replay**, not exactly once: a crash after remote
acceptance but before the local acknowledgement may produce a duplicate.
Consumers must treat the Task event tuple and state transition idempotently.
First delivery remains journal-ordered; replayed duplicates must not be
interpreted as new lifecycle transitions.

Task `Shutdown`, including a request with `now=true`, acknowledges without
terminating while any process record remains owned by the shim. Once the
registry is empty, shutdown atomically seals the service against a new Create,
flushes every durable event, joins the event retry worker, and invokes the
server shutdown callback once. A failed final event flush reopens the service
for a later shutdown retry rather than abandoning the journal.

Delete is likewise a durable retryable transition. A failed guest, network,
daemon, rootfs, or event operation retains the process record and clears only
the in-memory in-progress guard. The delete-event queued bit is persisted, and
the event journal must flush before rootfs artifacts or the final process
record are removed. Retrying can therefore resume idempotent external cleanup
without losing the exit-before-delete order.

The shim owns each agent relay that it starts. Relays run in dedicated process
groups; failed connection setup, failed reconstruction, normal task deletion,
and fallback cleanup kill and reap the complete group. Relay-socket removal is
retryable and its path remains owned until removal succeeds. Normal deletion
keeps the relay alive through the final authenticated guest `Shutdown` reply,
then terminates it before releasing the primary network endpoint.

Output FIFOs are opened nonblocking with a guard endpoint so detached clients
may reattach. The shim fetches at most Linux `PIPE_BUF` (4096) bytes per stream
and advances each durable guest-output offset only after the entire chunk is
written. Backpressure therefore replays rather than silently losing a chunk.
If a consumer remains absent or slow for 30 seconds, the shim logs the exact
dropped byte count and advances that stream deliberately so process wait and
cleanup remain bounded. An unconfigured output stream is discarded by
contract. Input and output FIFO opens honor the Task request context.
Every configured stdio path is validated before Create or Exec can allocate or
contact the guest. Paths must be absolute and canonical private, caller-owned,
single-link FIFOs or output files. Validation and each later start/recovery
open use `openat2` with symlink and magic-link traversal disabled, then bind
device, inode, mode, owner, link count, and ctime across the open. Stdin is
restricted to a FIFO. A replacement path or same-inode metadata change fails
closed, and a cancelled request cannot acquire a descriptor.

Terminal resize is a durable intent. A created process retains it for Start;
a running process records it before contacting the guest, rolls the record back
if the guest rejects it, and reapplies the retained size while reconstructing a
live terminal after shim restart. Stopped and paused process resize requests
fail without mutating the record.

Stdin close also separates durable request from guest acknowledgement. A
request made while the process is CREATED is retained without premature guest
contact and is acknowledged by the stdin pump after Start. Running and paused
processes retry transient acknowledgement failures; a stopped process cannot
gain a new close intent, while an already acknowledged request is idempotent.

Task pause and resume cover every running or paused init/exec process group in
a deterministic order. If any signal fails, already transitioned groups are
signaled back in reverse order under a bounded rollback context. Process states
are committed together only after all signals succeed, and persistence or
event failure likewise restores every group and its previous durable state.

Task stats aggregate CPU, resident memory, and PID counts across every running
or paused init and exec process group. Created and stopped processes are not
reported as live consumption, and arithmetic overflow rejects the response
rather than wrapping a task metric.

## Tests

- Unit tests against a fake daemon.
- Containerd shim protocol tests and event ordering.
- `ctr run`, `ctr task exec`, signal, wait, delete, and terminal resize.
- Containerd restart while a task runs.
- Shim crash while a task runs and shim reconnect/recovery.
- Duplicate requests, context cancellation, deadline expiry, and FIFO peer
  disappearance.
- Relay descendants and sockets are absent after connection failure,
  reconstruction failure, normal deletion, and cleanup retry.
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
