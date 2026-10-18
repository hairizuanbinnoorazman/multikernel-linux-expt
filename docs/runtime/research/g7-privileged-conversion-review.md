# G7 privileged conversion review

Reviewer/executor: Codex, 2026-10-13. Source checkpoint:
`b0d9d4d61ea0405c806b90d634df3fdb1bf9960b` (isolated source snapshot).
This report traces the production input boundary and records fixes and outstanding
acceptance work. SEC-04 is **open**; this is not an independent completion review.

The caller supplies OCI metadata, snapshot content, task identifiers, protocol
bytes and stale records. The primary operator, host configuration and installed
approved executables remain trusted. A hostile primary or compromised child
kernel is outside the containment claim; see the
[boundary assessment](g7-security-boundary-assessment.md).

| Input/conversion | Controls and source | Evidence and limits |
| --- | --- | --- |
| OCI process fields | [OCI validator](../../../scripts/validate-runtime-oci.py), [shim process validation](../../../runtime/cmd/containerd-shim-multikernel-v2/tasks_process.go): supported fields, types, byte limits, capability/namespace/security policy; guest argv/environment remain JSON data | OCI contract regressions and `TestValidateExecProcessFailsClosed`; host builder no longer inherits arbitrary ambient environment |
| Task/sandbox identifiers | [Shim identity validation](../../../runtime/cmd/containerd-shim-multikernel-v2/cleanup.go), generation-bound lifecycle records and argument arrays | Hostile identity and namespace/task alias regressions; no shell command assembled from workload argv |
| Bundle/root path | [Root validator](../../../scripts/validate-runtime-root.py), held bundle descriptors in [rootfs service](../../../runtime/internal/rootfs/service.go) | Traversal/noncanonical/symlink rejection, allowed absolute roots, replaced bundle/root fixtures; absolute-root policy is an operator setting |
| Snapshot mount inputs/target | [Rootfs backend](../../../runtime/internal/rootfs/backend_linux.go) pins bind/overlay directories and target identity, enforces readonly/nodev/nosuid/noexec | Synthetic mount-boundary assertions preserve original descriptors under substitution; kernel-backed upper/work metadata preservation still requires qualification |
| Readonly bind content | [Materializer](../../../scripts/materialize-runtime-binds.py) and manifest verification | Source/destination substitution, protected/overlapping destinations, quotas and exact copied manifest regressions; host paths are held/copies are private |
| Image tree/archive | [Rootfs builder](../../../scripts/build-runtime-rootfs.py), [archive verifier](../../../scripts/verify-runtime-rootfs.py) | Escaping links, FIFOs, sockets, devices, external hardlinks, xattrs, changed files, size/inode/capacity bounds; normalized deterministic output and no-replace publication |
| Storage image/build result | [Storage builder](../../../scripts/build-runtime-storage.py), [Go result validation](../../../runtime/internal/rootfs/backend_linux.go) | Fully allocated regular image, fixed quota/UUID/identity, held roots and exclusive publication; collision/ENOSPC tests preserve replacements and caller source |
| Cleanup/publication | [Go safe-file helpers](../../../runtime/internal/safefile/directory_linux.go), [Python publication helpers](../../../scripts/runtime_safe_publish.py) | Identity-conditioned replace/remove, quarantine/recovery, no-follow regular-file opens and collision/cancellation fixtures; ambiguous ownership is retained rather than guessed |
| Kernel/initramfs handoff | [Kerf adapter](../../../runtime/internal/kerf/kerf.go), sealed credential initrd, approved artifact descriptors | Replaced public initramfs and unsafe present artifact rejection; token is not a kernel command-line argument; source/initrd canaries retained |
| Subprocess environment/lookup | [Bounded runner](../../../runtime/internal/boundedexec/boundedexec_linux.go), explicit rootfs builder environment | Pre-fix negative controls reproduce hostile parent PATH selection, nil-environment secret inheritance and startup-hook canary mutation; corrected regressions reject these paths |
| Native arguments/data | [Shared argument parser](../../../tools/native_args.h), [NBD](../../../tools/mkvsock-nbd.c), [relay](../../../tools/mkvsock-relay.c), experiment [probe](../../../tools/mkvsock-probe.c) | Checked decimal ranges, size alignment, identifier/path lengths and reserved field; 80 malformed CLI cases leave a synthetic canary unchanged and no socket. NBD frame/range/identity regressions and UBSan pass |

## Fixes at this checkpoint

`exec.Command` performs basename lookup before applying `Cmd.Env`; supplying a
restricted child PATH therefore did not restrict lookup. The bounded runner now
resolves against the supplied PATH, validates every search entry before lookup,
and refuses relative executable paths or implicit/current-directory searches.
A non-nil empty environment prevents accidental inheritance when callers pass
nil. The host checker explicitly searches trusted system and local install
prefixes for Kerf.

Rootfs validation and build previously inherited all daemon variables. Both now
use system PATH/C locale and an explicit list of operator settings. Per-task
identity, paths, port and validation mode are supplied by the operation. Shell
startup, loader/Python search controls, temp-directory overrides and unrelated
secrets are omitted. The actual validation/build regression children confirm
the absence of those variables, preserved operator knobs and unchanged canaries.
These are defenses against ambient process/configuration influence; OCI
environment was not directly merged into the host builder environment.

Native decimal parsing previously silently accepted signs/suffixes and narrowed
overflowing values. The three helpers now reject malformed numbers before
image/device/socket operations. NBD additionally validates block size, bounded
identifiers and the reserved placeholder. Relay UNIX addresses are initialized
and bounded before connection setup. The common header must accompany sources
in standalone builds. Direct argv execution remains distinct from shell parsing.

No production Go `unsafe` or cgo imports were found in this checkpoint's runtime
source scan. All seven release commands were built with `CGO_ENABLED=0`. This
does not make the kernel, native helpers, dependencies or trusted scripts memory
safe. Native NBD has bounded request buffers/transfers and canonical handshakes;
relay/probe lifetime and transport pressure belong to the remaining failure audit.

## Remaining acceptance work

1. Complete executable selection: the rootfs backend now opens a validated
   executable through held directories/links and launches its exact inode;
   [portable proof](../../../evidence/runtime-20261012/g7-builder-executable-trust/README.md)
   retains generation replacement, original support lookup and mutation-free
   unsafe-parent rejection. Host config
   now validates Kerf directories, every traversed link and the final executable
   type/mode/size at load; [portable proof](../../../evidence/runtime-20261012/g7-kerf-executable-trust/README.md)
   reproduces ten unsafe pre-fix acceptances. Kerf launch now also pins the exact
   executable; [local proof](../../../evidence/runtime-20261012/g7-kerf-launch-pinning/README.md)
   verifies virtualenv imports, inherited descriptor numbers and rejection
   despite matching observed states. The [combined live qualification](../../../evidence/runtime-20261012/g7-pinning-live/README.md)
   passes production Kerf/builder normal compatibility, both-client network/
   storage/idle, restart and eventual zero inventory; it does not audit all
   interpreter/support dependencies or the remaining security acceptance.
   Storage helper/checker execution already
   uses an exact held executable. [Script interpreter checks](../../../evidence/runtime-20261012/g7-script-interpreter-trust/README.md)
   now reject unsafe direct/env interpreter paths locally while preserving
   offsets and virtualenv behavior; integrated compatibility is qualified below.
   [ELF loader path checks](../../../evidence/runtime-20261012/g7-elf-loader-trust/README.md)
   now bound program-header parsing and refuse unsafe native loader selection
   locally for builder/Kerf/storage helpers; this newest step is not deployed.
   Finish shared-library/import/support-file trust without breaking
   virtualenv imports or inherited descriptor numbering.
2. Extend [real-kernel snapshot proof](../../../evidence/runtime-20261012/g7-snapshot-preservation/README.md).
   The deployed pre-fix readonly index=on fixture changes upper-root ctime/xattrs;
   legacy descriptor-target bind remount also fails. New lower-only overlays and
   sealed recursive source clones preserve stat/xattrs/content including atime
   on tmpfs/ext4, with whiteout/opaque and nested-bind tests. This newest code is
   now deployed with integrated compatibility qualified below. Metacopy/redirect/userxattr, other layer/stack and
   fault/pressure cases remain open; do not generalize the bounded fixtures.
3. Complete resource-pressure and native campaign coverage, including approved
   executable replacement, helper subprocess/environment controls, and canaries
   for approved boot artifacts/caller snapshots under accepted/rejected requests.
   Process-group deadlines/output caps do not bound every underlying disk/kernel
   syscall. Preserve this distinction in SEC-04/SEC-05/SEC-09 acceptance.
   [Mount setup budgets](../../../evidence/runtime-20261012/g7-mount-budget/README.md)
   now reject oversized/ambiguous explicit view requests before filesystem
   access, normalize exec away and cooperate with cancellation between native
   calls. Recursive-subtree/kernel pressure and integrated live qualification
   remain open; this is not a complete pressure campaign.
   The [integrated release](../../../evidence/runtime-20261012/g7-snapshot-live/README.md)
   now passes all 19 ctr/Docker feature rows and independent final inventory.
   It includes the interpreter-only root-owned ELF multicall compatibility
   correction, whose first failed release/source remain preserved. Deployment
   compatibility does not close the remaining dependency or pressure review.
4. Obtain independent completion review. Passing normal live compatibility and
   portable boundary fixtures do not discharge the preceding items.


## 2026-10-15 kernel input boundary checkpoint

The [eight-overlay build](../../../evidence/runtime-20261012/g7-kernel-eight-overlay-build/README.md)
retains the exact patched kernel sources/configuration and actual binaries.
The [rb4 live qualification](../../../evidence/runtime-20261012/g7-kernel-rb4-live/README.md)
adds cold host boot,96malformed root/overlay refusals, the identical real
partial-growth failure with full cleanup, and minimal new-kernel guest boot.
All 19 shared ctr/Docker features and independent final cleanup pass on the
same rb4 boot with the original approved guest profile. These trusted-root fixtures do not establish
hostile-child containment or complete SEC-04.

The standard-tree checker validates baseline, overlay and outer-manifest
structure. The named-root instance format receives actual-buffer bounds and
exact four-byte, below-INT_MAX ID checks. This is deliberately narrower than
complete structural or resource-semantic validation. A full-tree pass also does
not establish that an arbitrary property is a valid string or physical range.

Review of the exact rb4 sources identifies the next unfinished conversions:

- `dts.c:mk_dt_parse_single_pci_device` still scans the generic `pci-id`
  property without validating its string length/full consumption and narrows
  parsed fields and vendor/device IDs. The baseline-specific parser's earlier
  checks do not cover this separate function.
- `mk_dt_parse_single_platform_device`, `mk_dt_parse_embedded_devices` and
  `mk_dt_parse_devices` still require property-local string validation. In
  particular, the device-name list uses `strlen` while advancing within its
  supplied length, and generic device types reach `strcmp`.
- `instance_dt.c:mk_restore_ipi_buffer` and `mk_restore_host_instance` check
  property widths but still need semantic physical-address/page-count bounds
  and ownership review before mapping. Valid guest boot proves compatibility,
  not adversarial physical-range rejection.
- Named-root complete structure, later CPU/device resource rollback and
  concurrent publication remain separate acceptance work.

These are source review findings and pending work, not newly executed exploit
results. Keep the corresponding main security tasks open.


The [ninth-overlay source/object checkpoint](../../../evidence/runtime-20261012/g7-kernel-device-properties/README.md)
addresses the preceding generic PCI and device-string findings: property-local
single-string checks, full/range-checked PCI conversion, no platform truncation,
whole-list preflight and bounded device paths. Four real-parser/UBSan groups and
old-source controls plus real GCC object compilation pass. The qualified rb4
source/binaries and running host remain unchanged. Full new-kernel build/boot,
actual device behavior and broader acceptance remain open; this is source
remediation proof, not live exploit rejection or complete SEC-04 closure.


The [tenth-overlay named-root reader checkpoint](../../../evidence/runtime-20261012/g7-kernel-instance-dtb/README.md)
now checks complete tag nesting/root presence, property extents/names and the
reservation map before existing generic/embedded readers. It preserves named
roots and does not weaken standard libfdt empty-root validation. Old bounds-only
code accepts all12malformed trees; new checks and unread-subtree reader controls
pass with real libfdt. The allocation-free helper can run during early restore.
This addresses the prior named-root structural gap at source level. Full new
kernel/host/guest and remaining physical-range/ownership/format semantics remain
open. The standard checker and resource-semantic policy are distinct; these
fixtures do not establish every DT specification rule or hardware authority.


The [eleventh-overlay manifest/IPI checkpoint](../../../evidence/runtime-20261012/g7-kernel-ipi-ranges/README.md)
requires exact scalar widths, nonzero aligned/representable/nonwrapping addresses
and the actual shared-ABI page count before data mapping. It preflights both
ranges/nonoverlap before CPU registration or restore allocation, uses safe CPU
scalar loads and bounds the outer producer-page address before mapping. Actual
GCC objects and an independent kernel ABI probe match263696bytes/65pages;
real-libfdt/UBSan fixtures and four old-reader controls pass within modeled
mapping/allocation/CPU effects. Cumulative new-kernel qualification remains open.
The host ring lies outside assigned guest RAM; these checks do not authenticate
ownership or establish independent physical confinement. Source review also
found early instance IDR/list publication followed by direct frees on some
restore failures. This is recorded for deferred-publication/ownership-correct
cleanup, not presented as solved by the mapping fixtures.


The [twelfth overlay](../../../kernel-patches/g7/0012-defer-boot-instance-publication.patch)
defers boot instance IDR/list publication until preparation completes and uses
private cleanup for partial resource-description copies and borrowed mappings.
[Source/object and modeled fault proof](../../../evidence/runtime-20261012/g7-kernel-restore-publication/README.md)
is retained. CPU-present-mask rollback remains separate and unproved; optional
host channel failures retain the prior warning policy. Full cumulative build and
new host/guest acceptance remain separate from this proof.


The [thirteenth overlay](../../../kernel-patches/g7/0013-stop-boot-on-invalid-resource-handoff.patch)
fixes permissive continuation after an invalid boot handoff or restore failure.
Bad/duplicate handoffs halt during population; the dedicated initcall halts on
restore errors or absent root policy. Required child DTB/positive ID refusal and
child pre-restoration PCI/platform denial accompany the NULL root platform guard.
[Actual-source/object proof](../../../evidence/runtime-20261012/g7-kernel-boot-policy/README.md)
retains simulated terminal halts, old-reader/runtime-error controls and saved GCC
commands. This deliberately halts the affected kernel on boot initialization
failure, including failed host root allocation. Valid no-manifest host behavior
is preserved in focused checks; actual host/guest boot and recovery policy remain
unqualified. CPU-assignment semantic validation and rollback remain open.


The [fourteenth overlay](../../../kernel-patches/g7/0014-commit-validated-boot-cpu-policy-at-publication.patch)
replaces permissive CPU-mask restoration with private policy preparation. It
requires a nonempty resolvable/present assignment, rejects aliases and requires
the boot CPU. The present mask is copied only after complete resource/object
preparation and successful IDR allocation, under the instance mutex alongside
publication. [Exact source/object and modeled fault proof](../../../evidence/runtime-20261012/g7-kernel-cpu-restore/README.md)
checks both cpumask representations and unchanged global mask on all pre-commit
failures. This closes the identified late-allocation present-mask mutation in
that boot path at source level; early topology registration, physical ownership
and actual new-kernel host/guest behavior remain separate.


The [fifteenth overlay](../../../kernel-patches/g7/0015-pci-refuse-domain-aliases-and-failed-bridge-reads.patch)
keeps PCI domain values wide until checking representability and refuses failed
secondary/subordinate bridge reads even when they fill plausible data.
[Exact guard/object proof](../../../evidence/runtime-20261012/g7-kernel-pci-policy/README.md)
retains modeled config failures and old controls. Full callsite search confirms
existing driver hooks; the first partial output was not a complete call graph.
The ACPI/generic platform gates separately lose HID context at generic registration;
that supported-assignment mismatch remains pending. Selector OR semantics are
documented and are not changed by the PCI fix. Actual hardware identity/ownership
and new cumulative live qualification remain separate.


The [sixteenth overlay](../../../evidence/runtime-20261012/g7-kernel-platform-policy/README.md)
preserves firmware name/HID identity for the canonical unnumbered ACPI platform
tuple at generic registration, closing the second-gate HID-only mismatch. Own
companion/name/id matching avoids granting inherited HIDs to derived devices.
This preserves the existing name OR HID contract and resource cleanup. Exact
UBSan/model checks, old control, actual object and cumulative rb8 build pass;
firmware/hardware ownership and live rb8 qualification remain open. The user
restart returned the host to original kernel while retaining the rb7 profile.


The [seventeenth overlay](../../../evidence/runtime-20261012/g7-kernel-overlay-memory/README.md)
validates dynamic memory-overlay byte ranges before lossy page shifts/native
allocation-size conversion. Common add/remove/inverse readers now reject zero,
unaligned, nonrepresentable or inclusive-end-wrapping extents before publishing
outputs. Pool sizes also reject native narrowing. Exact-reader models and actual
GCC compilation pass; qualified rb8 inputs/binaries remain unchanged. This is
source/object proof, not a new boot or physical-ownership guarantee. Follow-up:
the separate dynamic overlay PCI parser still accepts partial strings and
unchecked narrowing, and active memory add allocation/address correspondence
requires review. Earlier generic DTB PCI parsing does not cover this function.


The [eighteenth overlay](../../../evidence/runtime-20261012/g7-kernel-overlay-device/README.md)
addresses the separate dynamic PCI parser: property-local C-string bounds,
complete BDF grammar, range checks before narrowing and no output publication on
refusal. Optional driver text is now checked against the actual payload field
before logging/copying, preserving missing/empty overrides. Extracted complete
operation fixtures show refusal before modeled device effects for all routes;
old PCI/driver controls compile then fail. Actual GCC compilation passes with
qualified rb8 identities unchanged around the isolated compile. Hardware binding,
IPI authorization, physical ownership and full new build/live behavior remain
separate acceptance work. Typed optional flags/other overlay scalar/string
readers and multi-item rollback still require review.


The [nineteenth overlay](../../../evidence/runtime-20261012/g7-kernel-overlay-strings/README.md)
checks property-local C-string extents in metadata and target/name selectors.
Metadata previously copied bytes without establishing a terminator; target/name
checks allowed embedded-NUL suffixes to route to a shorter prefix. A shared
reader now rejects those forms and non-not-found errors. Initial metadata errors
free private state before transaction identity, operations or publication.
Extracted full apply/reader effects and both compiled old controls pass, with
real saved-GCC object proof and unchanged qualified rb9 inputs/binaries. Display
truncation of valid labels remains intentional. Later-fragment atomicity, name
storage capacity, ID/scalar conversions and physical authority remain separate.


### Overlay rollback ownership review — 2026-10-16

The current pinned overlay path has no completion journal. `parse_and_apply`
stops at the first failure, while `parse_and_rollback` re-walks every fragment
and operation in reverse, without recording which ones ran. For example,
instance-create can fail with EEXIST before creating anything, but its inverse
looks up the same name and destroys whichever instance exists then. Repeating
rmdir after an inverse failed can also replay inverses that already succeeded.
These are source findings, not a live destructive experiment. Do not claim
transaction atomicity, safe retry or instance-incarnation ownership from the
normal compatibility matrix. Required remediation includes completion tracking,
exact instance/resource identity, irreversible-operation policy, uncertain
primitive outcomes and a retry cursor; parsing the old DTB alone is insufficient.

The transaction counter also wraps into negative/reused identities even though
apply returns an int ID and the writer interprets negative values as failure.
Patch0022 serializes ID allocation under the existing resource mutex and
saturates at INT_MAX before kernfs/resource effects. This establishes lifetime
ID uniqueness, a prerequisite for completion tracking, without claiming to fix
the rollback findings above. A failed directory allocation consumes an ID and
does not recycle it. The first issued production ID remains2 as before.


Patch0023 changes the candidate's rollback disposition: a preallocated completion
ledger records successful operation boundaries, reverse consumption persists
across lookup failures, and uncertain failed primitives block automatic replay.
A failed EEXIST apply no longer automatically destroys a pre-existing instance;
partial inverse retries cannot replay consumed work. Two unsupported restoration
paths now return EOPNOTSUPP. Focused actual-function/rmdir models and isolated
GCC object pass in the [retained proof](../../../evidence/runtime-20261012/g7-kernel-overlay-progress/README.md).
This is not full atomicity: there is no per-item/primitive outcome proof, exact
instance-incarnation/original-resource ownership check, explicit reconciliation
or safe record disposal API. Exit still frees evidence after rollback failure.
The normal live deployment remains eighteen-overlay rb9; candidate changes19–23
are not boot-qualified. The original source header's atomicity claim is unproved.


Patch0024 closes the candidate's same-name/ID instance replacement route by
assigning nonzero, non-recycled incarnation tokens in the common lifecycle and
both boot/runtime allocators. The ledger captures resource-target identity under
the instance mutex and captures created identity before releasing the creation
lock. Resource inverses reject mismatched/zero recorded identities; created-
instance inverse compares its token while holding the destruction lock. This
uses values rather than held references, avoiding delayed last-ref resource
release. Counter saturation and immutable assignment refuse rather than wrap.
[Source/four-object proof](../../../evidence/runtime-20261012/g7-kernel-overlay-identity/README.md)
passes models, both creator paths, two old controls and actual header dependency
checks. The original atomicity wording is corrected. The layout is now linked in rb10 and passes actual host/two-CPU child boot;
[qualification proof](../../../evidence/runtime-20261012/g7-kernel-rb10-live/README.md)
retains matching profile activation, structural refusals and real pool rollback. Original
CPU/device/memory state, ambiguous primitive outcomes, reconciliation/record
disposal, exit evidence and complete failure/concurrency acceptance remain open.


The accumulated twenty-four-overlay rb10 kernel/modules now fully build/link,
including all changed-layout users and the generation allocator symbol;
[exact source/artifact/terminal proof](../../../evidence/runtime-20261012/g7-kernel-twentyfour-overlay-build/README.md)
verifies feature preservation, eleven prior inputs, four actual rebuilt objects
and final existing-host19zero inventory. This supersedes the source-only linking
limitation at the prior checkpoint. Installation and actual host/child execution now pass. The [full nineteen-feature suite and independent final inventory](../../../evidence/runtime-20261012/g7-kernel-rb10-matrix/README.md)
pass on this same rb10 boot. Security/reconciliation/resource-ownership criteria
remain open.


Candidate0025 removes a deterministic partial-effect route inside CPU/device
operations: both formerly validated an item immediately before moving it, so a
malformed later item could follow a successfully moved prefix. Each now validates
all matching item fields over its private immutable DTBO before the first move,
including undo. The second pass preserves item order/routes/metadata. The
[implementation and actual object proof](../../../evidence/runtime-20261012/g7-kernel-overlay-item-preflight/README.md)
retain144modeled rejections, compiled old-source failure, existing parser/ledger
regressions, exact patch and real GCC object with all17qualifiedrb10inputs and
3binaries unchanged. No new link/live policy acceptance is claimed. Earlier
operations/fragments and backend failures can still have partial effects;
original resource state, per-item outcomes and reconciliation remain required.
The reviewed overlay exit helper has no callers in the retained review source;
changing its cleanup alone would not harden these active per-item operations.


Candidate0026 extends operation-local preflight to memory-add/remove. Complete
lists now validate reg/size, NUMA/type and the pool-add reg prohibition before
allocation, shrink, park teardown or remote moves, in forward and undo paths.
Unsupported pool-growth undo and backend uncertainty remain unchanged.
[Source/object proof](../../../evidence/runtime-20261012/g7-kernel-overlay-memory-preflight/README.md)
retains66modeled malformed cases with zero moves, compiled old-source failure,
five UBSan memory-reader modes, CPU/device and completion/incarnation regressions,
exact patch and actual GCC object. The17qualifiedrb10inputs/3binaries remain
unchanged. Candidates0025/0026 are not linked/boot-qualified. Earlier operations
and backend errors can still leave partial effects; original resource ownership,
per-item outcomes, explicit reconciliation and full acceptance remain required.


Candidate0027 adds transaction-wide input preflight before the first fragment's
effects. A private validation descriptor reuses operation readers without live
resource actions; namespace create parses/frees configuration and refuses CPU-set
allocation failure, namespace remove returns before lookup/destruction. Syntax-only
instance targets preserve create-then-target overlays. Actual state/ownership
checks remain apply-time. [Proof](../../../evidence/runtime-20261012/g7-kernel-overlay-transaction-preflight/README.md)
retains24modeled malformed operation positions, compiled old driver control,
read-only CPU/device/memory/create/target paths, regressions and real kernel object
with17qualifiedrb10inputs/3binaries unchanged. This does not reserve resources,
cache create config or make backend/late allocation failures atomic. The runtime
constructor still needs explicit refusal when config CPU-set allocation fails.
Candidates25–27 remain unlinked/undeployed; ownership/per-item outcomes and
reconciliation/disposal/teardown acceptance remain open.


Candidate0028 supersedes the constructor initializer-failure limitation above.
The shared initializer now returns a required checked result, initializes cleanup
state before CPU-set allocation and returnsENOMEM on failure. Runtime creation,
boot restoration and preflight stop and clean private state; parser entries and
CPU parser refuse missing configuration CPU sets. [Proof](../../../evidence/runtime-20261012/g7-kernel-config-init/README.md)
retains actual whole-tree caller inventory, focused init/parser/annotation and
runtime/boot/preflight failure checks, both compiled old caller failures,
regressions and four actual objects against the modified header.17rb10inputs/
3binaries remain unchanged. This is source/object qualification; candidates25–28
are not linked/boot-qualified. Resource ownership/per-item outcomes, reconciliation/
disposal, teardown retention and full fault acceptance remain required.


Candidate0029 moves CPU source/destination bookkeeping checks and destination
capacity reservation before repark/IPI in active and non-running initiators.
Unexpected post-ACK tracker error returnsEUCLEAN rather than success: add excludes
the CPU from the free pool, remove keeps the old owner until pool tracking works.
[Proof](../../../evidence/runtime-20261012/g7-kernel-cpu-ownership/README.md) retains actual initiators
with modeled allocator/backend/ACK errors, three compiled old controls and real
GCC object against candidate28header.17baselineinputs/3reviewedCPU/IPI/set sources/
3binaries remain unchanged. No broad CPU-transfer mutex was added because instance
creation holds instance_mutex while message sends can look up instances, risking
lock inversion. Capacity assumes serialized writers; per-resource epochs, peer/
replay correlation, concurrent lifecycle changes and uncertain physical results
remain open. Candidates25–29 remain source/object-only pending group qualification.


All29patches now build/link together as rb11 with preserved feature config and
20pinned inputs; six changed-layout/API users have actual rebuilt object/command/
header-dependency records. [Build proof](../../../evidence/runtime-20261012/g7-kernel-twentynine-overlay-build/README.md)
checks all46archive members,20before-input snapshots, six linked guard symbols,
trusted terminal actor/invocation and independent19zero existing-host inventory.
This supersedes the pending-link limitation for candidates25–29; it does not
qualify all backend/resource ownership or boot/hotplug behavior. Recovery/default
preservation and subsequent installation/boot observations are tracked in notes.


[Rb11 live qualification](../../../evidence/runtime-20261012/g7-kernel-rb11-live/README.md) supersedes the prior pending boot/hotplug
status: actual host and matching child boot, physical CPU12 add/remove on a
running child, independent cleanup,96malformed-write refusals,real allocation
rollback and matching runtime-profile activation pass. Five recovery profiles
and original GRUB default remain preserved. Full19feature suite is separate
and pending; broad ownership/concurrency/peer/recovery/G7/G8 criteria stay open.


[Rb11 full feature qualification](../../../evidence/runtime-20261012/g7-kernel-rb11-matrix/README.md) now passes all19shared features
through containerd/Docker on the same cold host boot and matching guest kernels.
The suite terminates success13:53:34UTC; a separate final audit confirms19zero
counters/no instances/no pool. Five recovery profiles and capability/NoNewPrivs
bounds remain verified. This supersedes the pending-feature status above;
whole SEC-04/G7/G8 acceptance and remaining ownership/failure criteria stay open.


Pending IPI responses formerly matched only type/operation/resource. Because
new entries prepend to the list, a late response for a timed-out operation can
complete a newer same-resource request; duplicate responses can overwrite its
first result. [Candidate0030](../../../evidence/runtime-20261012/g7-kernel-request-correlation/README.md)
adds a random boot epoch and nonwrapping request sequence, propagates it through
resource/shutdown deferred work and accepts one exact completion. Result/list
consumption shares the completion lock. Requests use new wire subtypes so
legacy receivers never decode larger layouts as old resource operands. Exact
size/zero-ID guards and shutdown operation/ID-narrowing checks apply.
Actual-function models and three real objects pass; old behavioral controls
fail as expected. This is source/object scope only, not authenticated-origin
or duplicate-request protection. CPU-add uncertain outcomes can still leave
an assignable-pool alias, and unsolicited halt generation/teardown are open.
Complete30patch link/boot qualification remains separate from preserved rb11.


CPU-add previously left a free-pool entry until ACK success: a timeout after
peer online could leave the CPU assignable elsewhere. Slot membership also
followed wake success, while teardown returned CPU records before proving
park-context release and removed region metadata before an arch failure.
[Candidate0031](../../../evidence/runtime-20261012/g7-kernel-cpu-retention/README.md)
records destination ownership/slot intent before effects, keeps post-effect
uncertainty sticky, and gates CPU operations against image/directory release.
Confirmation covers all owned CPUs. Unsafe cleanup retains image references,
region/pool metadata and unexpected last-reference objects in a retention
registry, which blocks pool reconfiguration. Actual function/fault checks and
four compiled behavioral controls pass; six real modified-header objects build.
This does not establish global ownership serialization or complete checked
operator recovery: generic image-free remains void, retention inspection is
not yet bounded and fallback records have no managed release API. Candidates
30–31 still need full link/matching host-child boot and shared-feature proof.


[Candidate0032](../../../evidence/runtime-20261012/g7-kernel-checked-unload/README.md)
addresses the unchecked syscall result left by0031: checked image release returns
errors before unlink, targeted unload preserves those errors under the kexec
operation lock, and bulk unload stops at refusal or returns E2BIG after128
successful releases if images remain. Partial progress is explicit. Retention
inspection prints at most64 rows with truncation, but has no paging/managed
release API. Actual branch/helper/lifecycle models, three compiled predecessor
behavioral failures and four real isolated kernel objects are retained. An
overly broad dependency check failed after three successful object compiles;
the corrected runner reused them unchanged and compiled only the fourth.
Candidates30–32 remain undeployed; full link/boot, live refusal/retry, general
lifecycle/global ownership serialization and reconciliation remain required.


[Candidate0033](../../../evidence/runtime-20261012/g7-kernel-cpu-pool-serialization/README.md)
adds the missing shared ownership lock for common CPU-pool transfer/return,
host accounting and affected snapshots. Host/pool transitions reserve custody
before hotplug, withdraw pool-to-host CPUs from assignability, and retain
backend/tracking/publication failures with bounded root-only inspection.
The mutex never spans arch/CPU-hotplug/IPI waits. CPU-set reserve count overflow
and transfer request aliases are rejected. Focused actual-function pthread/UBSan
checks and two compiled predecessor controls pass; seven actual modified-header
kernel objects build without changing rb11. This accounting lock does not make
park-area availability checking atomic with later free, serialize all slot/
instance lifecycle writers, authenticate peers or provide managed reconciliation.
Candidates30–33 still require full group link/matching host-child qualification.


[Candidate0034](../../../evidence/runtime-20261012/g7-kernel-park-quiescence/README.md)
closes host-park admission with teardown's ownership check and reopens only after
complete setup. Persistent physical CPU pins cover common hotplug helpers,
including remote-removal workers that do not use pool-transition bookkeeping.
Dying CPUs use their preselected park mode; confirmed online retirement clears
pool-play-dead and the selector before unpin under CPU read-side exclusion.
Separate local CPU hotplug serialization protects pin/unpin helper sequencing.
Actual-function models and compiled old teardown/selector controls are retained.
A first three-object build preceded the selector-retirement review fix; a
separate three-object build verifies final source. Neither is deployed.
Remaining general instance/slot lifecycle synchronization, managed recovery,
full candidate link/matched host-child and live fault/native-hotplug proof
remain required; this is not authenticated-peer or hostile-child containment.
