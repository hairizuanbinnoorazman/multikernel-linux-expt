# Plan 04: OCI images and storage

## Purpose

Turn container image content into deterministic child roots while preserving
the primary's ownership of all cloud storage controllers.

## Staged minimum implementations

### A. Deterministic image input

- Accept the OCI bundle and snapshot/rootfs mounts prepared by containerd.
- Leave registry access, image pulling, content verification, layer unpacking,
  and snapshot ownership with containerd; do not implement a second image
  acquisition path in the Multikernel runtime.
- Generate and verify a manifest before boot.
- Reject unsupported file types, unsafe paths, device nodes, and inconsistent
  hardlinks unless explicitly handled.
- Keep `mk-agent`, transport modules, and bootstrap utilities in the
  runtime-owned initramfs without mutating the caller's snapshot.
- Verify image and child-kernel architecture compatibility before allocating
  the sandbox. Record any kernel feature assumptions required by the bundle.

### B. Single-sandbox root

Use one already-proven mechanism first:

- DAXFS for an immutable or single-owner root; or
- one primary-mediated ext4 image exported to one child.

Do not share a writable DAXFS root across children. Do not expose one writable
ext4 image to multiple children.

The ext4 builder copies an admitted source into a private staging tree with
archive semantics, accounts for that staging allocation in its high-water
check, and normalizes imported atime/mtime there. Because POSIX cannot assign
ctime, the builder normalizes imported inode ctime in the completed image,
then runs an offline filesystem check before publishing the image and identity
record. Metadata normalization must never be applied to the caller-owned root.
Export allocation journals its exact owner and generation as `PREPARING`
before server creation. An ambiguous start retains that journal; recovery
stops any exact live partial server, revalidates the image, and restarts with
the same generation. Only a start that returns success may publish `ACTIVE`.
The storage journal is loaded as untrusted input: its file identity and owner,
record keys, immutable export identities, state-specific completion fields,
UTC timestamps, live-owner uniqueness, and forward-only transitions are all
validated before reconciliation may inspect or signal a backend process.
The root-preparation journal similarly binds `rootfs` and `.multikernel`
directories to the canonical bundle and its storage directory to the configured
storage root plus task identity. Recovery validates these derivations again
immediately before recursive cleanup and removes relative names through
inode-stable, descriptor-anchored roots; phases advance only from `MOUNTING`
to `MOUNTED` to `PREPARED`.
Backend process records and logs must be private bounded files, bound to the
exact export lease, and opened no-follow. Readiness must repeat the lease's
path/image/generation/size/port; close counters are valid only after that exact
marker and as the terminal canonical line.
The rootfs service must distrust privileged builder outputs too: required
artifacts are no-follow, bounded, caller-owned single-link regular files whose
identity remains stable while read. Storage metadata must match the requested
path and port and the supported identity/quota contract; the image itself must
have the declared size, full allocation, and digest before publication and on
recovery. Service-authored result files use exclusive creation so pre-existing
files and symlinks cannot redirect or replace publication.
The builder and all of its descendants execute in a dedicated process group
under the earlier of the request deadline and a ten-minute default. Timeout or
cancellation kills that group. Combined output is continuously drained but at
most one MiB is retained, overflow fails the build, and any returned diagnostic
is separately truncated so a hostile builder cannot exhaust memory or the
daemon response frame.
Prepare, cleanup, and reconciliation must reject a pre-cancelled operation
before filesystem or journal mutation. Mount entry and artifact verification
do the same, and storage-image hashing observes cancellation between one-MiB
chunks rather than monopolizing the rootfs service through a complete image
scan.
Storage Provision, Release, and Reconcile must similarly reject cancellation
before an ownership transition. Each backend operation checks before external
inspection or process mutation; in particular, Stop cannot signal an export
after its request is already cancelled, and image inspection polls between
four-MiB hash reads.
The offline checker uses the same process-group runner as the rootfs builder,
with a five-minute default and one-MiB combined stdout/stderr evidence bound.
Timeout or cancellation kills descendants; overflow and every non-clean exit
fail closed. Only successful bounded output is hashed into the release record,
and failure responses do not disclose checker diagnostics.

### C. Container overlays and volumes

- Give each container a private writable layer.
- Support read-only bind inputs before writable host-path volumes.
- Define ownership mapping and propagation rules.
- Add capacity quotas and high-water refusal before live ENOSPC.

The read-only bind-input v1 subset admits at most eight non-overlapping host
directories. Each mount must be read-only, select exactly one of `bind` or
`rbind`, and may use private plus standard restrictive/atime flags; writable,
shared, slave, and unknown options are rejected. The adapter normalizes every
accepted form to the stricter guest `bind,ro,nodev,nosuid,noexec` policy. A
destination may not overlap `/dev`, `/proc`, `/run`, or `/sys`. The primary
manifests each source before and after an archive-semantic copy into the
container's private ext4 staging root, requires the copied manifest to match,
and retains the normalized manifest and digest as a separately verified
artifact. Host source paths are never mounted or revealed in the child.
Numeric UID/GID, modes, links, and admitted content are preserved. Propagation
is deliberately `none`: this is a point-in-time materialized input, not a live
host bind. Before the first process, the guest bind-mounts the materialized
destination onto itself and remounts it `ro,nodev,nosuid,noexec`. Existing
destinations, nested inputs, source symlinks, mutation, writable options, and
shared/slave or unsupported propagation options fail closed. Writable host-path volumes and configured
persistence remain a later storage format and ownership contract.

## Tests

- Whiteouts, opaque directories, hardlinks, symlinks, sparse files, xattrs,
  permissions, timestamps, and large trees.
- Byte-identical ext4 rebuilds after source atime changes and across different
  staging creation times, plus a changed-content negative control.
- Preservation of a containerd-unpacked BusyBox root without treating it as a
  bootable ISO, disk installer, or source of the child kernel.
- Read-only image rejection of writes.
- Writable layer persistence only where configured.
- Full disk, inode exhaustion, malformed image, wrong UUID, wrong generation,
  stale lock, duplicate attach, server loss during read/write/flush, and
  primary restart.
- Backend start failure both before mutation and after an exact server becomes
  live; exact retry and daemon reconciliation must retain the same owner and
  export generation.
- Forged, symlinked, hard-linked, oversized, mismatched, or changing process
  records/logs; wrong readiness identity; nonterminal close evidence; PID reuse;
  and immediate bounded signaling of a managed server.
- Forged, symlinked, hard-linked, sparse, incorrectly sized, permissively
  writable, mismatched, or changing rootfs-builder outputs, plus refusal to
  overwrite existing result files.
- Builder output overflow and a cancelled or timed-out builder with a live
  descendant holding its output pipe.
- Offline-check timeout with a descendant holding the pipe, output overflow,
  combined stdout/stderr hashing, and secret-safe non-clean failure.
- Clean child remount-read-only, NBD disconnect, server sync, and offline
  `e2fsck`.
- Snapshot/clone recovery using disposable copies.
- Cross-sandbox attempts to mount another sandbox's export.

## Evidence

Record image digest, manifest digest, backing allocation, export identity,
request/flush counters, mount table, cleanup sequence, and offline filesystem
result.

## Gate G4

Pass when containerd-prepared OCI content becomes a reproducible child root,
writable state has one clear owner, runtime bootstrap files remain outside the
OCI snapshot, normal teardown leaves a clean filesystem, failure cases do not
cross exports, and all storage devices remain owned by the primary.
