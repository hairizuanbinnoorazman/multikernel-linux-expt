# G0 learnings: scope and contracts

## Result

Gate G0 passed on 2026-08-31. The v1 contract set is under
[`../contracts/`](../contracts/README.md), with strict schemas for sandbox
configuration, approved kernel artifacts, and evidence manifests.

Go was selected for `mk-host-check`, `mkruntimed`, and `mk-agent`. Existing C
transport code remains appropriate at the AF_VSOCK boundary; the G3 run later
made that boundary a compatibility requirement rather than merely a migration
convenience.

The frozen MVP uses a runtime-owned kernel/initramfs/agent and a
containerd-owned OCI bundle/rootfs. It is `linux/amd64`, trusted-workload only,
and fail-closed for OCI fields not in the advertised feature set. Sandbox
identity is `(id, random generation)` and every mutation has an idempotency
key. Error, ownership, lifecycle, configuration, authentication, kernel
selection, and cloud evidence rules are explicit.

## License audit

No upstream code was copied into the Go implementation. Linux-derived kernel
and module work remains GPL-2.0; Kerf v0.2.0 is Apache-2.0; containerd and Kata
concepts reviewed for interface design are Apache-2.0. The new C relay carries
GPL-2.0-or-later consistently with the existing transport helpers. A future
release still needs a generated dependency/SBOM audit.

## Key learning

G0 prevented three live findings from leaking into higher layers as accidental
behavior: Kerf can commit despite a nonzero exit, transport direction matters,
and EOF/half-close is not a safe Multikernel AF_VSOCK framing contract.
