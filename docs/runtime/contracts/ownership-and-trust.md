# Ownership and trust boundary (v1)

Lifecycle journal completion records must match a preceding unique intent's
operation ID, sandbox ID, generation, method, idempotency key and fingerprint.
Orphan/duplicate completions, reused operation IDs, unknown phases and conflicting
embedded sandbox identities fail parsing and are refused before append. Before
storage or backend recovery, the daemon validates snapshot identities and all incomplete intents together.
A stale generation, changed configuration or conflicting recovery owner requires
operator action and does not authorize resource mutation. These checks detect
inconsistent records; the private primary-owned journal remains a trusted input.
A cancellation tombstone together with a pending intent for its request key
also requires operator action before recovery effects. Completion belongs to
the specific operation, not an earlier attempt sharing its key; cancellation
must not leave a retry available for later forward recovery.
Cancellation also binds its selected create intent to the current durable
generation, even when a replacement uses identical configuration. A stale
request key never authorizes teardown of that replacement.


Existing lifecycle, storage, rootfs and network snapshots must explicitly contain
their supported version and required ownership maps. Missing fields do not
inherit defaults for a fresh store. Structured documents reject top-level null;
malformed persisted state fails before reconciliation and is not silently
rewritten as an empty store. Fresh defaults apply only when no state file exists.

Privileged bounded commands resolve basenames using the explicit child PATH,
never the parent's ambient PATH. Every search entry must be absolute; missing,
empty or relative search entries fail before execution. An empty child environment
does not inherit parent secrets. The read-only host checker additionally admits
the trusted `/usr/local/bin` and `/usr/local/sbin` prefixes for installed Kerf.

Host configuration validates every directory and symlink traversed to the
Kerf executable, including intermediate link targets. Directories and the
final file must have the required owner and no group/world write permission;
links must have the required owner. The final file must be regular, nonempty,
single-linked, owner-executable and at most 256 MiB. Managed relative, absolute
and directory links remain supported within trusted paths. Traversal is bounded
and cannot leave the configuration validation boundary. This startup check
alone does not pin the executable inode or validate its interpreter/imports.
The Kerf adapter additionally uses the descriptor-anchored opener at each
launch and appends its executable after existing credential/kernel FDs. A trust
failure cannot be accepted through committed-state observation. Virtualenv
console-script imports remain compatible with descriptor execution; production
interpreter/import dependency trust remains a separate requirement.

Rootfs builders additionally resolve every directory/link through held
descriptors and execute the opened regular file through an inherited descriptor.
System ancestors may be root-owned; other path objects must be root- or
caller-owned and directories/files cannot be group/world writable. Root-owned
sticky directories are allowed only with trusted ownership of each selected
child. Symlinks are read through their held inode, with bounded traversal.
Executable files must be nonempty, single-linked, owner-executable and bounded
to 256 MiB. The executable descriptor is appended after bundle/runtime/storage
descriptors so those numbers stay stable. Managed deployment links and Bash
script self-location remain supported, including a renamed generation. This
pins executable identity against pathname replacement; trusted root/operator
in-place changes and interpreter/support-file provenance remain separate.

Before rootfs/Kerf script launch, the held shebang and all selected interpreter
paths are now validated with bounded recursion and trusted path ownership/modes.
Headers are at most 256 bytes; NUL/CR and truncated headers are rejected. The
supported env form names exactly one command, without assignments/options or
split-string syntax. Its last child PATH must have canonical absolute nonempty
entries, and the selected command passes the same executable trust checks.
Pread preserves artifact offsets. Interpreter trust failures cannot be accepted
through Kerf committed-state observation. Kernel interpreter opens remain
pathname-based beneath trusted directories. ELF loaders and import/support
dependencies are separate from this shebang check.

Interpreters may use root-owned native ELF multicall binaries with multiple
package hardlinks, provided existing ownership/path/mode/size checks and stable
identity/header validation pass. This exception applies only to interpreter
selection. Configured launch artifacts, script interpreters and caller-owned
aliased binaries retain the single-link requirement.

Native builder/Kerf and storage helper selection also checks ELF loader paths.
Only bounded ELF headers/program headers are inspected (at most 128 entries,
loader path at most 4096 bytes); no section table allocation occurs. Duplicate,
malformed or out-of-file loader records fail closed. The loader must be native
ELF selected through trusted directory/link/file ownership and permissions.
Reads preserve descriptor offsets and stable identities. Shared-library
resolution, loader cache, imports and support-file provenance are separate.

Rootfs snapshot inputs are preparation data, never writable overlay state.
The caller's upper becomes the first readonly lower and work is validated but
not supplied to OverlayFS. Index is off and volatile omitted. Real mount inputs
are detached recursive readonly/nodev/nosuid/noexec/noatime clones, leaving
caller mount flags and metadata unchanged. Bind attachment inherits the sealed
clone without a redundant descriptor-target remount. Missing clone/attribute
APIs fail before target mounting. Tested tmpfs/ext4 caller trees preserve every
stat field, xattr and content, including atime; broader layer/feature/fault and
production integration qualification remain required.

Mount setup has whole-request preflight before filesystem access: at most eight
mounts, 64 options per mount, 4096 bytes per option, 128 layers per overlay
including the projected upper, and 256 source views in total. Duplicate overlay
path keys fail with a distinct budget classification. Backend normalization
drops exec/rw/dev/suid before enforcing noexec/readonly/nodev/nosuid. Native clone
setup checks cancellation between calls and refuses target mounting when
cancelled. These are explicit-view limits and cooperative checks; recursive
subtree pressure and in-flight native syscall stalls remain separate.

Rootfs validation/building starts with a fixed system PATH and C locale and
retains only the operator knobs listed in `builderEnvironment` in the
[rootfs backend](../../../runtime/internal/rootfs/backend_linux.go): bootstrap
paths, manifest selection, allowed absolute roots, size/inode limits and free
space reserves. Task paths, identity, port and validation mode come from each
operation. Ambient shell startup, dynamic-loader/Python search variables,
temporary-directory overrides and unrelated credentials do not cross this
boundary. Workload OCI environment belongs only to the guest process. Approved
node executables and those explicit operator knobs remain trusted inputs.

| Resource or action | Owner | Other components |
| --- | --- | --- |
| Registry content, pull, unpack, OCI bundle/snapshot | containerd | Shim passes references; runtime reads validated paths |
| Runtime v2 request and stdio/event mapping | shim | No Kerf or sysfs mutation |
| Allocation policy, journal, generations, cleanup | `mkruntimed` | Sole global lifecycle authority |
| Multikernel pool and child mutation | Kerf adapter | Only daemon invokes it |
| Child kernel, initramfs, `mk-agent` | runtime/operator | Selected only from approved manifest |
| Container process, namespaces, child cgroups | `mk-agent` | Daemon sends authenticated requests |
| OCI root contents | containerd snapshot owner | Agent does not mutate runtime bootstrap |
| CNI allocation, primary namespace endpoint, routes, firewall, counters | `mknetd` | Shim generation-binds and pumps bounded frames; child receives only a TUN |
| Private writable root | exactly one sandbox generation | Primary service retains backing-device ownership; v1 exposes no persistent or writable host volume |
| GCE boot disk, NIC, shared controllers | primary kernel | Never assign to a child |
| Cloud VM, disk, snapshot lifecycle | operator/test harness | Every run records before/after ledger |

## Threat statement

Nested virtualization is not allowed in the target compute instances. KVM or
microVMs inside an instance are therefore unavailable as a containment remedy.
The primary and its children remain one trust domain; separate compute
instances are the deployment boundary for mutually untrusted domains. See the
[deployment and containment constraint](../architecture.md#deployment-and-containment-constraint)
for the required scope of architecture proposals and alternative execution models.

The operator, primary kernel, `mkruntimed`, approved Kerf binary, approved
child kernel/initramfs, and node configuration are trusted. OCI metadata,
bundle filesystem content, protocol bytes, sandbox identifiers, workload
processes, and stale state are untrusted inputs and are validated.

The MVP is restricted to trusted workloads on a single-tenant node.
Multikernel sibling kernels do not have a demonstrated KVM/EPT isolation
boundary. CPU ownership is software coordinated; child memory access limits,
interrupt/MSR/I/O-port behavior, and denial-of-service resistance are not
claimed as hostile-tenant boundaries. Devices use primary ownership and
mediation because a passed-through controller can include the primary boot
disk or NIC. A compromised primary controls every child.

Authentication binds each daemon-agent session to sandbox ID, generation,
protocol major version, endpoint, and a random 256-bit token stored in a
root-only state directory and injected through runtime-owned bootstrap data.
It prevents accidental/stale peers; it does not defend against a compromised
primary or child kernel that can read shared memory.

`mknetd` accepts CNI and shim requests only from the configured UID over a Unix
socket verified with `SO_PEERCRED`. Its durable record binds CNI identity,
network namespace, endpoint generation, sandbox ID, and sandbox generation.
Generationless deletion of a live allocation and cross-generation bind,
counter, reconnect, or delete requests fail closed.

The supported network data plane is IPv4 with primary-mediated outbound access.
Endpoint ingress to primary-local addresses is denied independently from
forwarded source, sibling, and metadata policy. IPv6 ingress is denied at both
primary INPUT and FORWARD. These guards precede endpoint link activation and
remain if link deletion fails; CHECK and startup reconciliation require them.
An older live endpoint missing the guards must be drained and recreated during
upgrade; restarting mknetd alone does not migrate its policy. Existing primary
administrative services must not be exposed by weakening the endpoint guards.

Native storage exports bind the intended nonzero child CID to sandbox and
export generations. State version3 persists this CID; process record version4
and the exact server command line include it. `mkvsock-nbd server IMAGE PORT
IMAGE_ID GENERATION PEER_CID` requires the CID. The accepted address must be an
exact AF_VSOCK address with that CID before any hello bytes are read or returned.
Peer refusal shares the bounded refusal-log budget. Image/size/generation
validation remains required for an admitted peer. Recovery validates every live
export against independently recorded lifecycle ownership before inspecting or
starting any backend; CID changes cannot reuse an existing lease or process.

Version2 state upgrades automatically only when empty or when every export is
validly RELEASED, preserving historical completion evidence without inventing
peer identities. Live or ambiguous legacy exports require operator review and
are retained unchanged. Deployment must drain exports first and preserve the
old state privately: older releases cannot read version3. A rollback must not
replace newer active ownership with an old snapshot. This is trusted transport
peer binding, not protection against a hostile child kernel or shared-memory
access; same-CID workload authority remains a separate review.

Storage recovery reads the generation-private log as bounded evidence. Initial
readiness must match the exact lease at byte0; repeated readiness records are
ambiguous. Pre-acceptance crashes may contain only complete canonical refusal
records (at most16) and one correctly ordered suppression notice. Unknown,
partial or excessive records cause an error before process-record removal.
A canonical acceptance line preserves ClientLost even if its crash tail is
incomplete. Terminal counter evidence must begin with exact readiness and end
with the canonical synced-close line at a record boundary. This does not
independently authenticate a compromised log producer or complete every
post-acceptance/fault semantic; inspection is required for ambiguous historical
logs. [Scoped source proof](../../../evidence/runtime-20261012/g7-storage-client-evidence/README.md)
retains baseline failures and current race checks; [identified root/live qualification](../../../evidence/runtime-20261012/g7-recovery-live/README.md)
adds actual process/bootstrap and restart/cleanup proof. Full recovery fault acceptance remains open.

Graceful storage-close counters additionally require the complete preceding
history to be canonical and ordered: bounded refusals/suppression before single
acceptance, at most one terminal I/O/disconnect diagnostic, then matching refusal
summary and synced close. Nonzero counters require acceptance; byte totals must
fit operation counts and admitted per-request/export sizes without arithmetic
overflow. Invalid history retains the process record and refuses graceful-close
release. Zero-I/O pre-attachment closes remain supported. Historical malformed
or missing-summary logs require inspection. [Close-evidence source proof](../../../evidence/runtime-20261012/g7-storage-close-evidence/README.md)
retains baseline failures; [identified root/cold/live proof](../../../evidence/runtime-20261012/g7-close-history-live/README.md)
adds deployment qualification and a retained unresolved memory allocation failure. Canonical records
still depend on the trusted log producer, not cryptographic sync authenticity.
