# Rootfs reconciliation requires unambiguous ownership

Daemon startup supplies the rootfs reconciler with a lifecycle ownership snapshot
after lifecycle reconciliation succeeds. Its keys are canonical absolute backing
paths and its values are immutable preparation digests, not hashes recalculated
from guest-modified writable images. Invalid path/digest input refuses the batch.

Before any rootfs backend action or artifact removal, preflight every record
against that snapshot. A claim beneath the mount target, runtime directory or
storage directory is compatible only with the exact `root.ext4` path of a
PREPARED record carrying the same preparation digest. A different digest,
partial MOUNTING/MOUNTED record, or other claimed file in a cleanup tree is a
conflict, not evidence that the preparation is orphaned. Refuse without backend
calls or journal/artifact changes, including when the public bundle is absent.
Later conflicts also prevent earlier orphan cleanup in the same batch.

An exact owner keeps the existing metadata-only verification path: guest writes
do not trigger a full-image digest comparison. A genuinely unclaimed preparation
keeps the existing guarded orphan cleanup path. Canonical sibling paths do not
adopt an orphan or block its cleanup. Claim lookup uses sorted path-prefix ranges
and component boundaries rather than a quadratic cross-product or loose prefix.

This is a refusal policy for the internal authoritative snapshot. It is not an
operator repair command or permission to supply an empty snapshot to release an
uncertain owner. Conflicting durable records require inspection before retry.
The policy does not discover every filesystem alias or prove storage/network
health, actual child-Docker recovery or full G7/G8 completion. Direct child
kernels use no nested virtualization and share primary trust after child-kernel
compromise.

The [baseline comparison and GCE proof](../../../evidence/runtime-20261109/g8-rootfs-reconcile-claims/README.md)
reproduce the unsafe baseline in five negative groups and qualify eleven native
groups/fifteen subcases from committed source. Rootfs/daemon/lifecycle race and
required checks pass. The GCE receipt also verifies private preservation and
own-unit/cgroup cleanup. These synthetic fixtures qualify refusal and existing
positive cleanup paths, not actual child recovery or arbitrary crash behavior.
