# Internal packages

Expected package boundaries:

- `boundedexec`: deadline-, process-group-, and output-bounded Linux commands.
- `kerf`: exact, testable adapter for the pinned Kerf backend.
- `lifecycle`: sandbox state machine and rollback policy.
- `state`: journal, locks, ownership, and restart reconciliation.
- `rootfs`: validated bundle mounts, private artifact preparation, and
  forward-only cleanup ownership.
- `storage`: DAXFS and mediated-block implementations behind one contract.
- `network`: primary-owned namespace and packet-transport implementation.

No adapter may release a resource unless the state layer proves that the
runtime owns it.

The daemon client bounds the complete Unix request, not only connection setup.
It applies caller cancellation/deadlines and a 30-second default to writes and
reads, actively wakes blocked socket I/O, and returns the caller's context
error when its deadline is the limiting boundary. Strict response decoding and
exact version/request-ID matching prevent a stale or malformed reply from
being attributed to the operation.

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
Rootfs builder execution is a bounded process-group operation. Output is
drained without retaining more than one MiB, error diagnostics are truncated,
and the earlier of the caller deadline and a ten-minute build bound kills the
entire builder group so descendants cannot retain pipes or partial work.
Prepare, cleanup, reconciliation, mount entry, and recovery hashing reject a
cancelled context before mutation; large storage-image hashing checks the
context between bounded read chunks.
Storage Provision, Release, and Reconcile likewise reject cancellation before
journal transitions or backend calls. Backend inspection, start, observation,
stop, and offline-check entry points reject it before filesystem or process
mutation, and image hashing polls between four-MiB reads.
Rootfs building and offline ext4 checking share `boundedexec`: both always have
a finite timeout, kill the complete process group, continuously drain output,
and retain at most the configured evidence bound.
Privileged primary and guest network commands and the `mknetd` egress preflight
use the same runner with a 30-second default and bounded combined-output
diagnostics.
Kerf lifecycle commands also use it; failure reports retain only output byte
count and digest so verbose Kerf output cannot disclose the agent token.
Read-only host-qualification probes use a smaller 64-KiB retention ceiling and
fail closed when a command times out or overflows it.
