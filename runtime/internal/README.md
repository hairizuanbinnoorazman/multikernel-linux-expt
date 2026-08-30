# Internal packages

Expected package boundaries:

- `kerf`: exact, testable adapter for the pinned Kerf backend.
- `lifecycle`: sandbox state machine and rollback policy.
- `state`: journal, locks, ownership, and restart reconciliation.
- `storage`: DAXFS and mediated-block implementations behind one contract.
- `network`: primary-owned namespace and packet-transport implementation.

No adapter may release a resource unless the state layer proves that the
runtime owns it.
