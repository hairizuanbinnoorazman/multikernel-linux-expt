# Storage adapters

The storage service binds a prepared primary-owned ext4 image to one exact
sandbox generation. Durable state prevents duplicate owner, path, UUID, or
port attachment. Export generation is independently random, restart
reconciliation never adopts a conflicting live generation, and release must
complete backend quiescence plus an offline filesystem check before state is
marked `RELEASED`.

Export creation journals `PREPARING` before starting the server. A failed or
ambiguous start retains that owner record. Reconciliation—or an exact repeated
provision request—stops an exact live-but-ambiguous process, re-inspects the
image, and restarts it with the same export generation before publishing
`ACTIVE`. It never adopts a process merely because it is present, and it never
acts on a conflicting generation.

The durable store treats its file as untrusted input. It opens state no-follow
and verifies the pre-open inode is the private, caller-owned, single-link inode
actually read. Every key, owner/generation, prepared-image identity, lifecycle
state, UTC timestamp, and offline-check digest is validated before recovery.
Non-released records must be unique by sandbox owner, path, port, and filesystem
UUID, and writes can only advance `PREPARING` → `ACTIVE` → `QUIESCING` →
`RELEASED` without changing immutable identity.

Backend process records and logs are also bounded, private, caller-owned,
single-link files opened no-follow with stable inode/metadata checks. A server
is ready only when its marker exactly repeats the lease path, image ID, export
generation, size, and port. Graceful-close counters are accepted only after
that exact ready marker and only as the canonical terminal log line. Managed
teardown first checks for an already-reaped child, otherwise revalidates its
PID/start-time/argv identity, sends `SIGTERM` immediately, and waits within the
configured bound.

If the daemon restarts in `QUIESCING`, reconciliation either stops the exact
still-running export or adopts its generation-specific graceful-close counter
record, then runs the offline check and finalizes release. An absent server
without that close record, a conflicting generation, or a failed check remains
durably `QUIESCING` for diagnosis instead of being guessed clean.

If an `ACTIVE` server disappears before any client was accepted, reconciliation
may restart the same generation: no child session existed to lose. Once the
generation-bound log records `MKNBD_SERVER_CLIENT_ACCEPTED`, server death is
terminal for that NBD session. Reconciliation retains the exact process record
and lease and fails closed instead of starting an orphan server that the child
cannot reconnect to. Only a canonical terminal `MKNBD_SERVER_CLOSED synced=1`
record converts accepted-client absence into a clean close.

The backend boundary is intentionally injectable. The Linux backend uses the
qualified primary-mediated NBD transport; service tests exercise inspection,
start, observation, stop, and offline-check failure boundaries without
pretending those mocks are G4 evidence.
