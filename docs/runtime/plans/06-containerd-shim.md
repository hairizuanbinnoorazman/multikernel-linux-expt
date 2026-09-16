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
surface. Each per-process `StatsProcess` observation is an idempotent bounded
reconnect transaction; a lost reply repeats the same process identity, while
an authenticated rejection remains terminal and no partial aggregate is
returned. Task `Update` and `Checkpoint` are deliberately not advertised by
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
Exec creation rollback is a bounded ownership transaction. If recovery or the
exec-added event cannot be queued after guest creation, the shim spends at most
five seconds confirming `DeleteProcess`; authenticated `NOT_FOUND` is an
idempotent success. It removes the in-memory owner only after that confirmation,
persists the resulting registry in either case, and returns every guest or
recovery cleanup error. A failed guest deletion therefore remains explicitly
owned rather than becoming an untracked process.
Before `ExecProcess`, the CREATED owner is now durable. An ambiguous guest
reply is reconciled for five seconds across relay reconnect: exact CREATED
continues, while absence or any invalid/unavailable state enters the bounded
rollback above. If rollback cannot confirm deletion, the durable owner remains
available to Task Delete. Reconstruction drops a confirmed pre-mutation
absence, rejects any wrong/live identity, and replays `TaskExecAdded` for an
exact CREATED exec under the journal's at-least-once contract.
Normal Task Delete applies the same authenticated boundary: `NOT_FOUND`
confirms that the guest process is already absent and permits exact delete-event
publication and local owner removal, while every other guest error retains the
owner for retry. Transport loss is retried through the owned relay within a
single bounded call, so a deletion whose first reply was lost is completed only
after the retry authenticates `NOT_FOUND`.
Init Start reconciles its already durable CREATED owner with the authenticated
agent before mutation. Only authenticated `NOT_FOUND` permits `CreateProcess`;
an exact ID with `CREATED`, zero PID, and zero exit is reused after an earlier
stdio/Start failure or shim recovery. Transport failure or any other identity
or state cannot be interpreted as absence and cannot issue a duplicate create.
Because `CreateProcess` itself is not replay-safe, a transport error from that
call triggers an independent five-second state reconciliation across relay
reconnect instead of another create. Only the exact CREATED identity completes
the original call; authenticated create rejection or absent, invalid, or
unavailable reconciliation remains an error with the durable local owner intact.
Once `StartProcess` succeeds, invalid PID observation, recovery failure, or
start-event failure cannot revert the process to CREATED. The shim retains the
RUNNING owner and its output/wait monitors, then uses a cancellation-independent
five-second `SIGKILL` transaction. Signal failure is returned; authenticated
`NOT_FOUND` confirms absence, and any later exact exit follows the same durable
completion path. A guest PID is accepted only when it is positive and exactly
representable by Task v2's `uint32` field; Start and reconstruction share that
check, so an oversized signed agent value cannot be truncated into a different
durable or externally reported identity. Start additionally requires the exact
requested agent process ID and a validated `RUNNING` or already-`STOPPED` state
before it publishes success; reconstruction accepts only that exact running
observation or the separately validated stopped-completion path.
An errored `StartProcess` reply is ambiguous because the authenticated response
may have been lost after mutation. A cancellation-independent, five-second
state observation reconnects after transport loss and distinguishes exact
`CREATED` (safe retry) from exact
`RUNNING` or `STOPPED` (the start applied and succeeds). An unavailable or
malformed observation retains unverified RUNNING ownership, starts its monitors,
persists that uncertainty, and attempts bounded termination. A process that
exits before the first state observation is therefore not misclassified as an
invalid Start.
Completion requires a `WaitProcess` reply for the requested agent process,
`STOPPED` state, the already established guest PID (or a valid PID when Start
could not verify one), and an agent-derived exit status in `0..255`. A semantic
mismatch is retried like transport loss and cannot close Task wait, mutate the
durable exit, or publish an exit event.

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

Init deletion uses a two-phase guest shutdown. Guest `CloseNetwork` is itself
bounded and reconnect-retryable so a lost successful reply cannot strand
teardown. After process and network closure, the shim obtains an authenticated,
reconnect-retryable `Quiesce` acknowledgement that proves the process registry
is empty, storage is quiet, and later guest mutations are sealed. It then sends
terminal `Shutdown`.
Cancellation or an authenticated rejection still retains retry ownership; a
transport loss after confirmed quiescence is safe to complete locally because
the terminal request cannot bypass storage quiescence or admit new work.
Initial guest `ConfigureNetwork` uses the same bounded reconnect transaction;
the agent accepts only an exact replay of its completed configuration, making a
lost successful reply safe without admitting changed network identity.
Live and reconstructed terminal resize uses that bounded reconnect transaction
too. `ResizeProcess` is an exact set operation, so replay carries the same
process ID, width, and height; a lost successful reply can no longer make the
shim roll its durable terminal-size intent back behind the guest's actual size.
Authenticated guest rejection is not replayed and still restores the prior
durable intent.

The shim owns each agent relay that it starts. Relays run in dedicated process
groups; failed connection setup, failed reconstruction, normal task deletion,
and fallback cleanup kill and reap the complete group. Relay-socket removal is
retryable and its path remains owned until removal succeeds. Normal deletion
keeps the relay alive through authenticated guest quiescence and the terminal
`Shutdown` attempt, then terminates it before releasing the primary network
endpoint.

Shim recovery state is untrusted input after restart. Both reconstruction and
fallback `Cleanup` open a bounded private caller-owned single-link regular file
with `openat2` symlink/magic-link rejection and stable identity checks. Strict
decoding binds the sandbox, generation, task/storage owner, optional network
owner, process identities and states, stdio paths, and terminal/stdin invariants
to the current containerd namespace/task tuple. Fallback cleanup rejects invalid
state before external mutation, reports every teardown failure, stops before
sandbox deletion when stop fails, and removes the rootfs only after sandbox
deletion succeeds.
Reconstruction observes each recorded live process with the bounded reconnect
transaction. Both `StateProcess` and the stopped-state `WaitProcess` follow-up
are idempotent reads, so transport loss reconnects and repeats the same process
identity; authenticated rejection remains terminal and cannot fabricate a
recovered process or exit.

Output FIFOs are opened nonblocking with a guard endpoint so detached clients
may reattach. The shim fetches at most Linux `PIPE_BUF` (4096) bytes per stream
and advances each durable guest-output offset only after the entire chunk is
written. Backpressure therefore replays rather than silently losing a chunk.
Before delivery, each agent reply must contain no more than the requested 4096
bytes per stream, an exact non-overflowing next offset equal to request plus
returned length, and a known `RUNNING` or `STOPPED` state. A malformed reply
cannot write to a destination or change durable offsets.
After complete delivery, both candidate offsets are published in one recovery
update. Publication failure restores both prior offsets and retries the same
bounded reply; this is explicit at-least-once output rather than a silent
in-memory acknowledgement. An unchanged empty reply does not rewrite recovery.
If a consumer remains absent or slow for 30 seconds, the shim logs the exact
dropped byte count and advances that stream deliberately so process wait and
cleanup remain bounded. An unconfigured output stream is discarded by
contract. Input and output FIFO opens honor the Task request context.
Every configured stdio path is validated before Create or Exec can allocate or
contact the guest. Paths must be absolute and canonical private, caller-owned,
single-link FIFOs or output files. Validation and each later start/recovery
open use `openat2` with symlink and magic-link traversal disabled, then bind
device, inode, mode, owner, link count, and ctime across the open. Stdin is
restricted to a FIFO. Recovery format v2 persists each stream's immutable
device, inode, owner, group, mode, and link-count identity captured before the
Create or Exec mutation. Start and reconstruction require that exact identity;
a replacement path, same-inode metadata change, or legacy recovery record that
cannot prove the original stream fails closed. A cancelled request cannot
acquire a descriptor.

Background output and terminal-wait reads are idempotent reconnect boundaries.
Each operation has one 30-second budget covering the initial authenticated
call, relay reconnect attempts, and replay with the same acknowledged output
offsets. An authenticated structured agent rejection is terminal and is never
replayed within that exchange; only transport/protocol failures enter relay
reconnect. An exchange rejection or exhausted transport budget leaves the
process state and exit channel unchanged, logs the uncertainty, and begins
another bounded observation epoch after 100 milliseconds.
The monitor is owned for the lifetime of the recorded running task; only an
authenticated `WaitProcess` result may transition it to stopped and publish an
exit status. Transport loss therefore cannot fabricate an exit.
Even after that result, the prior visible state and open Task wait channel are
retained until the exact stopped state, exit code, and timestamp are durable in
recovery. Publication failure is retried at 100-millisecond intervals; only a
successful recovery update permits exit-event queuing and Task completion.
After durable exit-event queuing, the recovery `exit_event_queued` flag is a
separate acknowledgement. If that update fails, the in-memory flag returns to
false to match the last durable process record; Delete or reconstruction then
repairs the event through the documented at-least-once path.
The agent removes each connection's cancellation callback when that session
ends normally; active server cancellation still closes a blocked connection.
Repeated relay reconnects therefore do not retain a goroutine and connection
reference for every completed session.
The primary daemon applies the same rule to its listener and concurrent request
handlers. Service cancellation or accept-loop return closes incomplete request
sockets rather than leaving handler goroutines blocked on peer EOF.
The daemon admits at most 128 handlers by default, caps configuration at 1,024,
closes excess connections without a goroutine, and joins admitted handlers
before returning.
Daemon replies are capped at the protocol's one-MiB response bound; oversized
or unencodable bodies become a bounded INTERNAL response with the original
request ID. Agent reply envelopes require exactly one body or structured error,
and expected typed bodies are strict-decoded before the caller can act on them.

Terminal resize is a durable intent. A created process retains it for Start;
a running process records it before contacting the guest, rolls the record back
if the guest rejects it, and reapplies the retained size while reconstructing a
live terminal after shim restart. Stopped and paused process resize requests
fail without mutating the record.

Stdin close also separates durable request from guest acknowledgement. A
request made while the process is CREATED is retained without premature guest
contact and is acknowledged by the stdin pump after Start. Running and paused
processes retry transient acknowledgement failures through the relay within
the earlier of the caller deadline and the 30-second I/O bound; a stopped
process cannot gain a new close intent, while an already acknowledged request
is idempotent.
Before forwarding each at-most-32-KiB stdin chunk, recovery format v2 persists
the bytes and current acknowledged offset. `WriteProcess` uses the advertised
`stdin-offset-v1` contract: the guest accepts only its exact next offset and
idempotently acknowledges the immediately preceding offset when its length and
SHA-256 are identical. The shim can therefore reconnect and replay a lost reply
without duplicating input, then clears the pending bytes only after the next
offset is durably recorded. A partial local write retains its accepted prefix,
and an exact replay writes only the remaining suffix before acknowledgement.
A failure to publish intent precedes guest mutation;
a failure to persist acknowledgement restores the same replay tuple.
The nonblocking stdin pump treats an empty attached FIFO (`EAGAIN`) as a
temporary no-data condition, just like an unattached FIFO EOF. It remains
available for later bytes from that writer until process teardown or an
explicit stdin-close request completes. A close request permits the one read
already in flight to become durable and reach the guest, then closes guest
stdin before the pump accepts another chunk; a continuously writing peer
therefore cannot postpone `CloseIO` indefinitely.

Task pause and resume cover every running or paused init/exec process group in
a deterministic order. If any signal fails, already transitioned groups are
signaled back in reverse order under a bounded rollback context. Process states
are committed together only after all signals succeed. Persistence or event
failure rolls every group and in-memory state back; the recovery rollback is
then published explicitly even when the failed transition write had an
ambiguous outcome. Any signal or compensating recovery failure is returned
rather than suppressing a potentially mismatched durable state.

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
- Recovery files reject unknown fields, wrong owners/generations, unsafe file
  identities, partial network identity, duplicate processes, and invalid state;
  fallback cleanup propagates ordered stop, delete, and rootfs failures.
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
