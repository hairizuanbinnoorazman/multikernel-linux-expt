# Approved Docker guest profile

The optional `docker_guest` object in an operator-owned
[kernel manifest schema](schemas/kernel-manifest-v1.schema.json) records approval
of a dedicated Docker guest image and its trusted bootstrap inputs. It is not
an OCI hook, image label, workload environment variable or default runtime mode.
Old manifests without this object keep their existing behavior. Older parsers
reject the new field rather than ignoring an approval they cannot implement.

The currently recognized kind is `docker-rootless-offline-v1`. It requires:

- `rootfs_manifest_sha256`: a canonical lowercase SHA256 of the deterministic
  source-root manifest emitted by `build-runtime-rootfs.py --manifest-only`.
  This identifies materialized content, ownership, modes, symlinks and hardlinks;
  it is not an image tag or registry manifest digest.
- `bootstrap`: a protected, hash-approved artifact using exactly
  `#!/bin/busybox sh` as its interpreter line. The bootstrap executes only in
  the child; validation never executes it on the primary.
- `modules`: exactly `llc`, `stp`, `bridge` and `veth`, each a protected,
  hash-approved artifact with the corresponding module name and the selected
  child kernel's release. A hash-approved module of a different name or release
  is still refused.

Unknown fields, unrecognized kinds, null approval, missing/extra modules,
noncanonical digests, unsafe artifact permissions, substitution and interpreter
changes are refused. Both the daemon's Go resolver and the builder's Python
validator enforce approval; the daemon binds the complete manifest digest into
its existing prepared-root contract. The Python test-only `--skip-modinfo` flag
is not used by production builder validation.

## Activation status

This increment establishes approval validation. **Production activation is not
implemented.** The production builder explicitly refuses any `docker_guest`
approval before source-root copying or storage construction. It must not ignore
this field and launch an ordinary workload in place of the requested Docker
service. Approval alone is not source-image verification or daemon readiness.

Activation requires a supervised, fixed bootstrap path, verification that the
held source-root manifest matches approval, protected outer-root installation,
module compatibility, bounded logging, an actual Docker API readiness probe and
engine shutdown before mediated-root quiescence. It must preserve the ordinary
workload's restricted capabilities/NNP and reject arbitrary OCI hooks. The
existing [diagnostic Docker guest](../research/docker-in-docker-guests.md)
remains the functional reference, not production integration proof.

The [deployment constraint](../architecture.md#deployment-and-containment-constraint)
excludes nested virtualization in the target cloud instances. This contract
prepares the direct-child runtime; it does not establish containment after
arbitrary child-kernel compromise. G7/G8 and production Docker-in-Docker remain
open.
