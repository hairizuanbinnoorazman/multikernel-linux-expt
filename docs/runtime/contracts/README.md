# Runtime contract set (v1)

This directory freezes the contracts required by gate G0. Version 1 targets a
trusted, single-tenant `linux/amd64` GCE node running Multikernel Linux
`v7.0-mk2` and Kerf `v0.2.0`.

The normative documents are:

- [`lifecycle.md`](lifecycle.md): identity, state transitions, API operations,
  idempotency, and error classes;
- [`ownership-and-trust.md`](ownership-and-trust.md): component ownership and
  the security boundary;
- [`kernel-and-image-policy.md`](kernel-and-image-policy.md): runtime boot
  artifacts, OCI inputs, compatibility, and feature rejection;
- [`configuration.md`](configuration.md): host and sandbox configuration;
- [`protocol-v1.md`](protocol-v1.md): daemon and agent wire envelopes; and
- [`evidence.md`](evidence.md): evidence manifest and cloud resource ledgers.

Machine-readable JSON Schemas live in [`schemas/`](schemas/). Version 1 is
strict: unknown fields are rejected unless a schema explicitly permits them.
Breaking changes require a new protocol or schema major version.

## Frozen MVP boundary

G0 through G3 provide host qualification, resource lifecycle, and one managed
OCI process in one child. Storage, networking, containerd, and Kubernetes are
G4 through G7 and are not implied by a G3 result. The child agent may use a
test-owned root directory for G3; production root delivery remains a G4
contract.

No open decision changes this boundary. AF_VSOCK transport details, storage
selection, networking, and the containerd shim can evolve behind the v1
ownership and lifecycle contracts.
