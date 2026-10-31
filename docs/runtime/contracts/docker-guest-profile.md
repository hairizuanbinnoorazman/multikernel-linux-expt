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

## Workload identity

The approved rootless profile reserves guest UID 0 for bootstrap/control. Docker
client workloads require UID/GID 1000, supplementary groups limited to 1000 and
no-new-privileges. Both production builder preflight and the agent enforce this
policy; agent create, exec and start cannot select bootstrap identity. Inner
Docker build containers run under the engine's separate rootless mappings.

Non-root capability grants are refused. Explicit empty capability objects,
including Docker's `--cap-drop ALL` projection, are supported through the sealed
credential executor. The trusted static pure-Go agent clears bounding bits before
changing all UID/GID values; every helper thread receives NNP before that change.
The requested environment, cwd and executable are used after dropping identity.
The [bounded host Docker lifecycle proof](../../../evidence/runtime-20261022/g8-host-docker-lifecycle/README.md)
now qualifies runtime selection, offline builds and teardown with private daemon
state. Recovery, concurrency and adversarial release qualification remain open.

Root-owned copies alone cannot isolate arbitrary guest UID 0. This profile's
separate workload identity protects outer bootstrap/control ownership without
granting workload access to root's procfs handles. Read-only sealing and complete
child process authority still need qualification.

## Activation status

Approval validation now includes a fresh scan of the held source filesystem.
The production builder compares its deterministic manifest digest to approval,
permits copied contexts only under `/work` plus the three standard Docker
metadata seeds, and rejects image symlinks/hardlinks at or below those targets.
The builder now prepares independent outer-root copies of the approved image's
`newuidmap`, `newgidmap`, `rootlesskit`, `nsenter`, `ip`, and their ELF
interpreter/library closure. The offline RootlessKit backend still needs the
namespace and IP helpers to configure its isolated loopback interface.
It parses bounded ELF64 x86-64 metadata as data, never executes image binaries
on the primary, and refuses custom loader paths/audit/filter configuration or
unresolved dependencies. All outer code is regular independent files; source
symlinks/hardlinks cannot create mutable-code references. Fixed outer account,
subuid/subgid and files-only NSS records replace image-selected account policy.
The prepared outer root includes fixed UID/GID 1000 private home, temporary and
runtime directories for the outer RootlessKit process, plus a root-owned sticky
`/tmp` (mode `1777`) for its namespace bind-mount staging. RootlessKit uses
the fixed outer `/run/user/1000/rootlesskit` state directory. Trusted boot
preparation must bind this data-only directory at the same inner path so
Docker can access its API after chroot. Helpers/libraries remain separate
regular outer files; sharing the image home or executable trees is not required.
Mapping helpers and libraries require root-owned source metadata. The non-setuid
`rootlesskit` release may retain its approved image build UID; the held source
must match that UID and content hash, and the independent outer copy is created
by root. Group/other-writable inputs remain refused. The closure is limited to
256 files and 128 MiB, with 64 MiB per source file.
Preparation remains in private scratch and is discarded on refusal.

**Approved-profile boot and clean shutdown passed on a direct child.**
The [retained proof](../../../evidence/runtime-20261022/g8-direct-child-boot/README.md)
uses the production builder, real Docker client/server, authenticated Quiesce and
Shutdown, synced NBD export and clean offline filesystem check. Docker builds
remain separate from the later host Docker lifecycle qualification below.
The builder verifies a scan of the copied image against both
approval and the held source before bind materialization, installs the independent
outer helper tree, rehashes bootstrap/module bytes while copying, and writes a
protected outer marker. Init accepts only that fixed marker and fixed modules,
then selects `--docker-guest`. Ordinary manifests emit no marker and keep their
ordinary startup path. No production manifest or service has been changed by
this implementation work.

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

The [real-image outer-helper proof](../../../evidence/runtime-20261021/g8-docker-outer/README.md)
qualifies independent data-only preparation, source mutation refusal and custom
loader refusal against the pinned rootless Docker image. Ownership alone does
not seal these copies against guest UID 0. Production boot must also protect
outer code and privileged procfs identities before accepting workloads; neither
read-only sealing nor complete process authority isolation is qualified by this
preparation proof.

Native relay and storage helpers now establish and read back non-dumpability
before opening endpoints/devices. The storage client proxy inherits that state;
the primary server also retains its post-privilege-drop guard. This protects
same-UID procfs access to these helper processes and does not by itself protect
all guest processes or qualify the complete Docker lifecycle.

The [process protection and identity receipts](../../../evidence/runtime-20261021/g8-guest-process-protection/README.md)
qualify native entry guards under equal-capability conditions, agent admission,
and real-image preflight for UID 1000/UID 0 on the GCE primary kernel. The
retained failed control explains why matching UID without matching capability
sets is not sufficient to demonstrate procfs exposure. Complete child Docker
activation/builds remain unqualified.

The [credential projection proof](../../../evidence/runtime-20261021/g8-docker-capability-projection/README.md)
records Docker's actual empty capability object, real manager chroot execution,
five zero capability sets, NNP and post-drop cwd/environment. It does not qualify
child-engine startup/builds. Before activation, access to native control/storage
transports by workload and daemon descendants must also be enforced and tested;
cross-child CID authentication alone does not demonstrate that process boundary.

The fixed [transport boundary](../../../runtime/internal/transportguard/README.md)
is now installed by workload executors and the Docker bootstrap wrapper. Native
VSOCK creation and io_uring are denied; incompatible ABIs are terminated. The
filter synchronizes all threads and persists across exec. Workloads retain only
stdio and the bootstrap only its verified script at fd3. Control agent/relay/NBD
processes keep their separate transport role. Complete child integration remains
unqualified until boot/build and adversarial live checks pass.

The [installed transport qualification](../../../evidence/runtime-20261021/g8-guest-transport/README.md)
checks real manager chroot workloads, privileged bootstrap execution, inherited
socket removal, exec inheritance and deliberate installation failure on the
restarted GCE primary kernel. It qualifies this process prerequisite; actual
child-engine startup/build and complete gate closure remain open.

The [real foreground engine qualification](../../../evidence/runtime-20261021/g8-docker-foreground/README.md)
uses the production wrapper and supervisor with the pinned Docker image in
private primary-kernel namespaces. Real API readiness, foreground lifetime and
owned-cgroup removal pass; retained failures drove the helper, temporary-directory
and fixed state-directory fixes. This is not direct-child boot/build evidence.
Production boot preparation and copied-root verification still need integration
before complete production qualification can be claimed.

The agent's explicit Docker path now owns fixed filesystem preparation before
service launch. It rejects aliases in runtime/data paths, nonfresh outer state
and a nonprotected/non-tmpfs inner `/run`; enables the service parent controllers;
and creates the state bind and read-only config/cgroup mounts before admitting
clients. Approved artifact installation, module loading and the production boot
opt-in are now wired in the builder/init; their direct-child behavior remains
unqualified until a live run passes.

The [agent-owned preparation proof](../../../evidence/runtime-20261022/g8-docker-preparation/README.md)
qualifies that exact filesystem method with the real engine, including alias
refusal before preparation, EROFS write attempts and shared-state identity on the
GCE primary kernel. Actual child boot/build still requires qualification.

The [profile construction proof](../../../evidence/runtime-20261022/g8-profile-boot/README.md)
qualifies the integrated production builder against the real pinned image and
selected child kernel artifacts. It inspects generated marker/bootstrap/helper/
module bytes and modes, retaining the refusal controls. It does not yet qualify
execution of that generated child boot or Docker builds.

The outer state directory is self-bound before sharing into the image so that
RootlessKit cannot unlink the shared inode at exit. Both mounts are held by
`open_tree` descriptors and transferred to service ownership before launch.
Once the engine scope is empty, cleanup moves each exact mount to a protected
outer directory and unmounts it normally before permitting root read-only remount.
Busy references retain ownership and refuse storage release. The mechanism does
not depend on an image-writable pathname or execute image helpers at shutdown.
The [installed ownership proof](../../../evidence/runtime-20261022/g8-state-mount/README.md)
checks image-side rename, EBUSY refusal, retry and idempotent cleanup with the real
engine on the GCE primary. It retains the preceding deleted-inode failure; this
phase starts no child and performs no Docker build.

The [direct-child boot proof](../../../evidence/runtime-20261022/g8-direct-child-boot/README.md)
subsequently qualifies the generated mediated-root boot and clean shutdown.
Earlier primary-only evidence remains bounded to its original phase. No complete
G7/G8 gate is closed by this boot check.

The [production-profile offline build proof](../../../evidence/runtime-20261022/g8-production-docker-build/README.md)
now passes multi-stage COPY/RUN/COPY --from, a cache-hit repeat, execution of the
built image, failed-build rejection and independent saved-image layer/content
verification on the mediated direct child. Authenticated shutdown, clean offline
filesystem checking and restored resources pass in that same invocation. Host
Docker/Runtime v2 selection, recovery, concurrency and adversarial integration
remain open. This advances the earlier boot-only phase without closing full gates.

## Docker import archive preparation

The approved image root is xattr-free. The original registry export can carry
`security.capability` PAX headers on mapping helpers; Docker import restores
those headers even when an earlier ordinary tar extraction ignored them. The
runtime continues to reject capability and other unsupported xattrs.

Operators can use `scripts/build-runtime-docker-image-archive.py ROOT OUTPUT
--manifest-sha256 APPROVED_DIGEST` to package the exact approved extracted tree
for Docker import. This explicit distribution step scans the tree with the
ordinary strict root validator, verifies its approval digest, and serializes
only approved bytes, owners, modes, links and directories. It never executes
image programs, changes the source or emits xattrs. Changed content, unknown
xattrs and existing outputs are refused. The approved independent outer copies
still install setuid mapping helpers through the supervised bootstrap contract.
The host Docker qualification imports this archive and checks the exported
filesystem against the same approval before attempting child startup.

The [host Docker lifecycle proof](../../../evidence/runtime-20261022/g8-host-docker-lifecycle/README.md)
subsequently passes host runtime selection, the offline build matrix and clean
teardown through private containerd/runtime/Docker state. It retains independent
firewall policy/rule and link/route comparisons and a pre-teardown workload
checkpoint. Recovery, concurrency, adversarial and complete release gates remain
open; historical phase-specific limitations above describe their original runs.

The [graceful runtime-restart proof](../../../evidence/runtime-20261022/g8-host-docker-runtime-recovery/README.md)
subsequently keeps the Docker child live across one private runtime process
restart. Child boot ID, local engine ID, saved-image digest and ownership
generation remain identical; final teardown and raw primary/network comparisons
pass. Crash/power loss, other service restarts, service privilege bounds and
complete G7/G8 acceptance remain open.

## Primary baseline after an instance restart

Capture a fresh primary boot ID, Docker observation, active service PIDs and
protected file hashes before the next child workload. Previous-boot PIDs are
historical observations; a PID value can be reused after boot. Check idle kernel
resources and preserve retained failures before admitting another trial.

`runtime_primary_docker.persistent_unchanged_after_restart(previous, fresh)`
compares schema-2 observations from distinct canonical boot epochs. It requires
the same primary engine and persistent image/container/volume inventory digests.
It rejects malformed observations and changed persistent identities without
echoing private names. Docker may recreate network IDs at boot, so this function
does not qualify network identity parity. Network policy, service health,
protected files and retained data contents need their own checks. Inventory
digests do not verify image filesystem contents.

Use the fresh observation as the workload baseline and call
`runtime_primary_docker.unchanged(fresh, during_or_after)` throughout the workload.
That guard still requires the same boot and exact inventory, including networks.
Check the precise guest-built image's absence from primary Docker separately.
Never use the restart comparison to accept a boot change during a workload.

The [restart controls](../../../evidence/runtime-20261031/g8-docker-restart-comparison-controls/README.md)
and [GCE preflight](../../../evidence/runtime-20261031/g8-supervisor-docker-restart-preflight/README.md)
qualify this distinction, including fresh PIDs and full retained-data hashing.
The subsequent [actual Docker run](../../../evidence/runtime-20261031/g8-live-docker-supervisor-diagnostics/README.md)
keeps the child engine/image boundary and normal cleanup intact. Failed trials
retain their original units and directories. An absent success or after receipt
must remain absent; a separately exported read-only snapshot does not imply that
the failed VM procedure wrote such a file. Repair a guard under a new procedure,
directory and unit, retaining the original refusal.

Direct child kernels require no nested virtualization; the target cloud instance
cannot use nested virtualization as a containment solution. Child Docker uses
its guest-local engine, socket and storage. The primary and child kernels still
share host trust after child-kernel compromise, and full G7/G8 remain open.
