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

Field names are canonical at the manifest top level and throughout the approval
object, including its artifact objects. Case aliases such as `DOCKER_GUEST`,
`KIND` or `PATH` are refused. They must not shadow a real approval with null or
give the two parsers different interpretations of the same document.

## Activation status

Approval validation now includes a fresh scan of the held source filesystem.
The production builder compares its deterministic manifest digest to approval,
permits copied contexts only under `/work` plus the three standard Docker
metadata seeds, and rejects image symlinks/hardlinks at or below those targets.
The builder now prepares independent outer-root copies of the approved image's
`newuidmap`, `newgidmap`, `rootlesskit`, and their ELF interpreter/library closure.
It parses bounded ELF64 x86-64 metadata as data, never executes image binaries
on the primary, and refuses custom loader paths/audit/filter configuration or
unresolved dependencies. All outer code is regular independent files; source
symlinks/hardlinks cannot create mutable-code references. Fixed outer account,
subuid/subgid and files-only NSS records replace image-selected account policy.
The closure is limited to 256 files and 128 MiB, with 64 MiB per source file.
Preparation remains in private scratch and is discarded on refusal.

**Production activation is not implemented.** After these checks the builder
still refuses activation before source-root copying or storage construction. It must not ignore
this field and launch an ordinary workload in place of the requested Docker
service. Approval metadata alone is not source-image verification or daemon
readiness; the held scan and eventual copied-root verification are separate
requirements.

Activation requires a supervised, fixed bootstrap path, verification that the
held source-root manifest matches approval, protected outer-root installation,
module compatibility, bounded logging, an actual Docker API readiness probe and
engine shutdown before mediated-root quiescence. It must preserve the ordinary
workload's restricted capabilities/NNP and reject arbitrary OCI hooks. The
activation path must also prevent materialized bind inputs or Docker metadata
seeds from replacing approved mapping helpers, bootstrap dependencies or daemon
configuration. User build contexts need a separate, restricted destination;
approving the source image alone does not approve later replacements.
After accepting the workload, privileged supervision must not execute helpers
from its writable OCI root. Guest UID 0 can modify its own files even with zero
capabilities. Shutdown needs held process identities and a fixed control path
outside that root before the mediated filesystem can be remounted read-only.
The existing [diagnostic Docker guest](../research/docker-in-docker-guests.md)
remains the functional reference, not production integration proof.

The [deployment constraint](../architecture.md#deployment-and-containment-constraint)
excludes nested virtualization in the target cloud instances. This contract
prepares the direct-child runtime; it does not establish containment after
arbitrary child-kernel compromise. G7/G8 and production Docker-in-Docker remain
open.

The [scoped approval/refusal receipts](../../../evidence/runtime-20261021/g8-docker-profile/README.md)
record portable checks and validation against installed GCE artifacts. They do
not qualify Docker activation or the whole G8 gate.

The [guest service scope primitive](../../../runtime/internal/guestscope/README.md)
provides finite cgroup limits and owned descendant shutdown. The [engine supervisor](../../../runtime/internal/dockerengine/README.md) now
uses it in the explicit agent startup path, with bounded logs, socket peer
identity, Docker API readiness and stop-before-storage ordering. Protected
bootstrap/module installation, production boot integration and qualification of the
complete Docker lifecycle remain required.

The [GCE source-binding and scope receipts](../../../evidence/runtime-20261021/g8-docker-source-scope/README.md)
qualify held-root binding and copy-input refusal in the builder, plus cgroup
cleanup and memory enforcement on the primary kernel. They do not qualify a
child Docker daemon, build workload, or its complete lifecycle.

The [supervisor qualification](../../../evidence/runtime-20261021/g8-docker-supervisor/README.md)
records synthetic UID-1000 readiness endpoints on the restarted GCE primary
kernel, including foreign-cgroup refusal and cleanup. Actual child Docker
activation/builds remain unqualified.
