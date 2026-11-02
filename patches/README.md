# Pinned source patches

- `kerf/`: compatibility patches for the pinned Kerf revision.
- `linux/`: compatibility patches and ordered hardening overlays for
  Multikernel Linux.

Apply patches only to the exact revisions recorded in the experiment reports.

## Kerf compatibility

Apply in this order:

1. [Skip special files](kerf/kerf-v0.2.0-skip-special-files.patch): handles unsupported special files in
  Docker/OCI-derived DAXFS roots.
2. [Hardlink inode count](kerf/kerf-v0.2.0-hardlink-inode-count.patch): fixes hardlink inode accounting for
  the pinned Kerf revision.

## Multikernel Linux

- [VSOCK build safety](linux/linux-v7.0-mk2-vsock-build-safety.patch): updates the pinned Multikernel
  AF_VSOCK callback and adds receive-length validation.
- [G7 kernel pool hardening](linux/g7/README.md): 35 overlays, applied in
  numeric filename order to the pinned base with reviewed transport overlays
  retained. The series README records dependencies and qualification evidence.

Historical evidence retains the paths used when it was captured:
`kernel-patches/g7/` now lives at `patches/linux/g7/`, and the former top-level
compatibility patches now live under `patches/kerf/` or `patches/linux/`.
