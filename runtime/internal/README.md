# Internal packages

Expected package boundaries:

- `kerf`: exact, testable adapter for the pinned Kerf backend.
- `lifecycle`: sandbox state machine and rollback policy.
- `state`: journal, locks, ownership, and restart reconciliation.
- `rootfs`: validated bundle mounts, private artifact preparation, and
  forward-only cleanup ownership.
- `storage`: DAXFS and mediated-block implementations behind one contract.
- `network`: primary-owned namespace and packet-transport implementation.

No adapter may release a resource unless the state layer proves that the
runtime owns it.

Rootfs durable records are treated as untrusted across restart. Their files,
keys, request identities, phase-specific results, and exact bundle/runtime/
storage path derivations are validated before reconciliation. Store reads and
writes return deep copies so callers cannot mutate journal ownership through
aliased mount, storage, or JSON fields.
Recursive cleanup opens the canonical bundle and configured storage root as
stable descriptor-backed `os.Root` handles and removes only relative owned
names. A symlink or rename after validation therefore cannot redirect cleanup
into a replacement tree.
Privileged rootfs-builder outputs are also treated as untrusted. Publication
opens bounded caller-owned, single-link regular files with `O_NOFOLLOW`, checks
stable identity across reads, validates exact storage metadata, and binds the
declared quota to the image's size, allocated blocks, and digest. Runtime
result files are created exclusively so an existing file or symlink cannot be
overwritten.
