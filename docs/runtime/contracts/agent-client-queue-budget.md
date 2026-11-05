# Agent client caller budget while queued

Source `9b5bf7dd` makes admission to the shared agent stream respect the caller's
context for CallContext, ReconnectContext and ReconnectPeerContext. A caller waiting
behind an unrelated frame can now return Canceled/DeadlineExceeded while the owner
continues. It does not send a frame, dial, consume a sequence, close/poison the active
connection, or change the selected relay policy. A cancellation racing acquisition
returns the token before proceeding.

Each client owns one lazily initialized capacity-one channel. It creates no
per-waiter worker or polling timer. Successful callers still serialize whole framed
requests/replies and verified reconnect/policy replacement. Existing MAC/sequence,
checked-peer PID/UID/GID, explicit reconnect, stream-poisoning and idempotent signal
rules remain intact. Canceling a queue waiter does not revoke an earlier owner's
already accepted request. It is not signal-result or cleanup authority.

Client.Timeout keeps its existing wire-phase meaning. Supply a bounded caller
context to cover queue admission as well. No fairness, node admission, bounded
caller count or complete operation-readiness guarantee is added.

[Local exact-parent red/green proof](../../../evidence/runtime-20261105/g8-agent-client-gate-controls/README.md)
uses real Unix sockets with an active held owner. Six call/reconnect cancellation/
deadline cases preserve that owner and have zero wire/dial/sequence/policy side
effects. Thirty-two concurrent authenticated calls preserve framing, sequence and
per-caller reply binding. Full runtime race and required checks pass.

This defect was found during retained-output/pending-signal investigation. Its
causal role in the original GCE Docker timeout is unproven. It does not recover
the retained child/export or pass Docker cleanup/full G7/G8. No nested virtualization
is used; direct child kernels share host trust after compromise.
