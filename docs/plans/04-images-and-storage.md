# Plan 04: OCI images and storage

## Purpose

Turn container image content into deterministic child roots while preserving
the primary's ownership of all cloud storage controllers.

## Staged minimum implementations

### A. Deterministic image input

- Accept an unpacked OCI bundle from containerd.
- Generate and verify a manifest before boot.
- Reject unsupported file types, unsafe paths, device nodes, and inconsistent
  hardlinks unless explicitly handled.
- Include `mk-agent` without mutating the caller's snapshot.

### B. Single-sandbox root

Use one already-proven mechanism first:

- DAXFS for an immutable or single-owner root; or
- one primary-mediated ext4 image exported to one child.

Do not share a writable DAXFS root across children. Do not expose one writable
ext4 image to multiple children.

### C. Container overlays and volumes

- Give each container a private writable layer.
- Support read-only bind inputs before writable host-path volumes.
- Define ownership mapping and propagation rules.
- Add capacity quotas and high-water refusal before live ENOSPC.

## Tests

- Whiteouts, opaque directories, hardlinks, symlinks, sparse files, xattrs,
  permissions, timestamps, and large trees.
- Read-only image rejection of writes.
- Writable layer persistence only where configured.
- Full disk, inode exhaustion, malformed image, wrong UUID, wrong generation,
  stale lock, duplicate attach, server loss during read/write/flush, and
  primary restart.
- Clean child remount-read-only, NBD disconnect, server sync, and offline
  `e2fsck`.
- Snapshot/clone recovery using disposable copies.
- Cross-sandbox attempts to mount another sandbox's export.

## Evidence

Record image digest, manifest digest, backing allocation, export identity,
request/flush counters, mount table, cleanup sequence, and offline filesystem
result.

## Gate G4

Pass when an OCI root is reproducible, writable state has one clear owner,
normal teardown leaves a clean filesystem, failure cases do not cross exports,
and all storage devices remain owned by the primary.
