# Pinned compatibility patches

- `kerf-v0.2.0-skip-special-files.patch`: handles unsupported special files in
  Docker/OCI-derived DAXFS roots.
- `kerf-v0.2.0-hardlink-inode-count.patch`: fixes hardlink inode accounting for
  the pinned Kerf revision.
- `linux-v7.0-mk2-vsock-build-safety.patch`: updates the pinned Multikernel
  AF_VSOCK callback and adds receive-length validation.

Apply patches only to the exact revisions recorded in the experiment reports.
