# Storage adapters

The storage service binds a prepared primary-owned ext4 image to one exact
sandbox generation. Durable state prevents duplicate owner, path, UUID, or
port attachment. Export generation is independently random, restart
reconciliation never adopts a conflicting live generation, and release must
complete backend quiescence plus an offline filesystem check before state is
marked `RELEASED`.

The backend boundary is intentionally injectable. The Linux backend uses the
qualified primary-mediated NBD transport; service tests exercise inspection,
start, observation, stop, and offline-check failure boundaries without
pretending those mocks are G4 evidence.
