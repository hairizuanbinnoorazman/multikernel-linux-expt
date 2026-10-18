# G7 kernel pool hardening overlays

Pinned base: Multikernel Linux3bdd35b64413da0b4e089ce931bfc2e8b031cbf7.
These patches are source candidates; do not infer running-kernel qualification
from a userspace fixture or object compilation.

`0001-baseline-roll-back-partial-memory-growth.patch` records physical bases
of successful requests in one baseline grow attempt. If a later request fails,
it rolls back only those newly acquired chunks in reverse order. It keeps
existing chunks and successful grants unchanged. Failed shrink continues other
rollback and returnsEUCLEAN with a bounded diagnostic; a busy/live chunk is
preserved instead of being freed behind an owner. Tracking allocation failure
occurs before any pool growth. Request count uses the existingMAX_NUMNODES bound.
This covers failure within memory growth; CPU/device/host-park failures later
in baseline initialization and grow's internal architecture-callback rollback
remain separate audit work.

Apply to a private checkout of the pinned source, retaining existing reviewed
transport overlays. Run:

```sh
patch -p1 < /path/to/0001-baseline-roll-back-partial-memory-growth.patch
python3 /path/to/scripts/test-kernel-pool-rollback.py kernel/multikernel/baseline.c
```

The fixture extracts the exact grow function and injects resource failures;
it proves transaction decisions with mocked allocation/shrink, not real pages,
locking or boot. Full kernel build/boot, allocation/failure/concurrency and
release qualification are required before installation claims.

The observed16GiB allocation failure is a separate reliability issue. The pinned
allocator scans physically contiguous ranges; enough total free memory does not
prove a suitable range exists. Splitting a pool into smaller pieces is not a
drop-in fix because individual child memory regions still require contiguous
allocations. This rollback patch does not establish fragmentation resistance.

`0002-pool-require-resource-registration.patch` registers a newly allocated
range in the resource tree before adding it to the allocator and pool list.
Registration failure aborts and frees the unpublished pages; allocator-add
failure removes the resource before freeing. Architecture-callback rollback
failure now reportsEUCLEAN and retains the tracked/live chunk rather than
silently returning the original setup error. This does not solve allocator
fragmentation or complete later CPU/device initialization rollback.

Run `python3 scripts/test-kernel-pool-publication.py /path/to/patched/mem.c`
for exact-function ordering/ownership failure injection. It mocks pages,
resource insertion and allocator primitives; real kernel build/boot/fault and
concurrency qualification remain necessary.

`0003-baseline-reserve-dtb-before-resource-commit.patch` applies after0001.
It reserves a private copy of the retained baseline DTB before clearing or
moving resources. A DTB allocation failure leaves the prior tree and resources
untouched. Later parse/resource failures free the private copy; success replaces
the old tree without another allocation after pool/CPU/device changes.
This closes the late-allocation ownership failure, not general CPU/device or
host-park rollback. Its source/object checkpoint precedes cumulative full-build
qualification.

Run `python3 scripts/test-kernel-baseline-dtb-commit.py /path/to/baseline.c`.
The exact-function fixture retains the previous failure, allocation-before-
mutation checks, private-tree cleanup across seven errors and exact successful
replacement. Its CPU/device/pool primitives are mocked. A complete build/boot
and real failure qualification are still required for this third overlay.

`0004-baseline-bound-dtb-and-device-properties.patch` applies after0003.
It rejects short DTB buffers before header reads in the baseline entry point
and root kernfs writer, and refuses nonzero write offsets. Device type, PCI ID
and optional platform name must be single nonempty NUL-terminated properties;
malformed present properties fail rather than becoming C strings or aliases.
PCI addresses must consume the whole string with bounded component widths,
legal slot/function ranges and no signs, whitespace or prefixes. Vendor/device
IDs must fit16 bits before narrowing. Valid short or padded hexadecimal BDF
components and optional platform names remain supported.

Run `python3 scripts/test-kernel-baseline-inputs.py /path/to/baseline.c
/path/to/kernfs.c /path/to/scripts/dtc/libfdt` on one line after all five overlays.
For0004 alone, use `--section headers` and `--section devices` separately.
The fixture extracts
the actual device parser/root writer and compiles the pinned real libfdt.
It checks malformed properties/address ranges, valid PCI/platform inputs and
short buffers against a protected page. Kernel allocation/list/locking effects
are mocked. The inaccessible-page reproduction is a userspace boundary test,
not a claim that the same write crashes the deployed kernfs buffer allocator.
Full DTB structural validation and other DTB/overlay readers remain separate
work. Partial platform metadata staging is addressed below. This overlay needs
cumulative kernel qualification.

`0005-baseline-stage-platform-metadata.patch` applies after0004. Device parsing
now writes platform metadata into a private list, with cleanup on every later
initializer error and ownership transfer only at successful commit. Malformed
subnode iteration is an error. Existing metadata is no longer cleared before
input parsing and CPU validation. Resource-stage failures can still discard old
metadata after validation, and general memory/CPU/device rollback remains open.

The real-libfdt input fixture additionally checks a valid platform request
followed by a malformed PCI request: new metadata stays private and old metadata
is unchanged. Run the DTB ownership fixture with `--staging` for exact initializer
parse/validation/resource error cleanup and success-transfer checks. Parser APIs
are adapted for historical negative controls; resources/list/lock effects remain
mocked. All five overlays now have a successful full kernel build with actual
artifacts retained in the cumulative evidence bundle. The five-patch cold boot
passed, but actual rollback failed because host park allocations remained busy,
leading to the sixth correction below. Broader concurrency/fault qualification
remains open.

`0006-baseline-release-owned-host-park-on-rollback.patch` applies after0005.
The real five-patch kernel revealed that the first grow creates host parking
infrastructure inside its new chunk, so a later grow failure left that chunk
busy even with no children or parked CPUs. The baseline returnedEUCLEAN and
retained64MiB rather than freeing live infrastructure. Before rollback of a
previously empty pool,0006 calls the existing host-park teardown helper, which
independently refuses while pool CPUs depend on it. Pre-existing pools keep
their park; generic busy-chunk checks still preserve other owners. This is a
fix for that recorded failure, not a reason to bypass EBUSY or global ownership.

Run the partial-growth fixture with `--host-park` to model architecture-owned
pages, pre-existing ownership and quiescence refusal. Previous code fails the
newly owned park case; the new source and real kernel object pass. The full
six-overlay build, cold boot and identical actual failure rerun now pass;
[retained live proof](../../evidence/runtime-20261012/g7-kernel-rb3-live/README.md)
shows host park release before exact chunk removal and no remaining pool.
Normal compatibility and broader rollback/concurrency acceptance remain separate.

`0007-baseline-validate-complete-device-trees.patch` applies after0006.
Both the root writer and direct baseline initializer now call `fdt_check_full`
with the supplied buffer length before parsing resources. The existing minimum
header, exact-size and serialized resource checks remain. Kernel libfdt already
declared this API but omitted its implementation; the overlay adds its build
wrapper, linked object and source-update list entry.

The checker is taken from the kernel's pinned DTC revision
[a26ef6400bd845beb6e7f369809ca1c42d856904](https://github.com/dgibson/dtc/blob/a26ef6400bd845beb6e7f369809ca1c42d856904/libfdt/fdt_check.c),
with the legal trailing-NOP allowance present in later upstream code. Licensing
is preserved. The real-parser fixture now checks packed/padded valid trees,
legal NOP padding, nine header-valid malformed trees and a truncated supplied
buffer. Previous writer and unmodified checker negative controls are retained.
Kernel resource effects remain mocked. Real GCC baseline/kernfs/checker objects
and a relocatable symbol-resolution link pass. The cumulative eight-overlay build,
[host boot and actual malformed refusals](../../evidence/runtime-20261012/g7-kernel-rb4-live/README.md)
now pass. General rollback and broader DTB reader qualification remain open.

`0008-dtb-bound-overlay-and-manifest-readers.patch` applies after0007.
Overlay writes reject short buffers before allocation and use the full-tree
checker before transaction application. Boot manifests reject unsupported
lengths before mapping and validate their complete outer tree. Manifest restore
and CPU-registration readers recheck the outer page before reading properties.

Flat instance DTBs currently use named roots. They retain that format through
`mk_dt_check_blob_bounds`, which checks minimum header and actual buffer bounds
before header/property access without imposing the standard empty-root rule.
Generic parsing, property-size inspection, stored base trees and embedded
instance readers use this helper. Instance IDs must be exactly four bytes and
below INT_MAX, preserving a representable exclusive end for the downstream
`idr_alloc(id, id + 1)` call. Invalid IDs leave outputs unchanged.

Run `scripts/test-kernel-dtb-entry-bounds.py` with the patched
`kernel/multikernel` and pinned `scripts/dtc/libfdt` directories. It extracts
actual reader functions, uses real libfdt, and mocks mapping, allocation and
application. Six focused groups pass; six old-reader controls fail, including
a protected-page userspace short-reader fault and the ID upper limit. Four real
kernel objects and the combined seven-object relocatable link pass. Full named-
root structural validation and broader resource/string semantics remain open.
The cumulative build, host boot, actual root/overlay refusals and minimal new
guest early-boot qualification now pass; the linked live proof records scope.


`0009-dtb-validate-generic-device-properties.patch` applies after0008.
The generic embedded/named-device parser now validates property-local single
strings, exact PCI consumption/ranges before narrowing, and complete platform
identities before allocation. It preflights every device-name list element and
its generated-path bound before base-tree lookup or allocation; empty lists
remain valid, while empty elements, aliases/path components and truncated names
are rejected. Malformed iteration and unsupported device types fail closed.
Optional absent platform fields and embedded non-device nodes remain supported.

Run `scripts/test-kernel-device-properties.py` with patched dts.c, pinned libfdt
and the actual include/linux/multikernel.h as its three arguments. Four real-
parser/UBSan groups pass and four old-source controls fail refusal assertions.
Kernel allocations/lists/base ownership are modeled and scanf is userspace libc.
[Real GCC object and source proof](../../evidence/runtime-20261012/g7-kernel-device-properties/README.md)
retain the actual compiler command and successful terminal, unchanged qualified
rb4 sources/binaries and independent clean-host inventory. The [cumulative ten-overlay build](../../evidence/runtime-20261012/g7-kernel-ten-overlay-build/README.md)
now passes with actual binaries retained; [new host and full new guest-profile compatibility](../../evidence/runtime-20261012/g7-kernel-rb5-live/README.md) also pass. Named-root complete structure,
remaining resource semantics and hardware/concurrency acceptance remain open.


`0010-dtb-validate-complete-instance-trees.patch` applies after0009.
The existing bounds helper now walks the complete structure/reservation map with
bounded libfdt readers, preserving named roots without weakening the standard
checker. It requires a single balanced root and final end, validates every
property name/value extent and rejects properties outside nodes. The walk is
allocation-free for early restore/CPU-registration use. Resource ranges/ownership
and broader format semantics remain separate policy.

[Named-root proof](../../evidence/runtime-20261012/g7-kernel-instance-dtb/README.md)
retains real-libfdt/UBSan positive and12malformed cases, guarded short/provided
buffers, old acceptance/refusal controls and reader integration. An unread
malformed subtree now blocks property/identity output. Generic device parser
regressions also pass. Run the new instance-DTB structure fixture with patched
dts.c and pinned libfdt; the entry-reader fixture now includes integration.
The cumulative ten-overlay kernel build passes with actual retained binaries;
[new host/full guest-profile compatibility](../../evidence/runtime-20261012/g7-kernel-rb5-live/README.md) also passes. Parser fixtures alone make no install/boot claim.


`0011-manifest-bound-shared-ipi-mappings.patch` applies after0010.
Both shared-ring readers now require exact scalar widths, nonzero aligned and
representable addresses, a nonwrapping extent and exactly the producer's page-
rounded ABI size before mapping. Both nodes are preflighted for validity and
nonoverlap before CPU registration or restore allocation. Optional absent nodes
retain compatibility; malformed present nodes fail. Safe scalar CPU loads and
signed lengths avoid alignment and conversion hazards. The outer manifest also
requires a nonzero producer-page-aligned base before initial mapping.

[Source/actual-object proof](../../evidence/runtime-20261012/g7-kernel-ipi-ranges/README.md)
retains five real-libfdt/production-layout groups in native64bit and modeled32bit
physical-address widths, four old-reader controls, actual GCC instance/manifest
objects and a separate kernel ABI probe confirming263696bytes/65pages. All new
code UBSan checks pass. Legacy numeric controls disable alignment UBSan/old sign
warnings; physical mapping/allocation/CPU effects are modeled. No new full kernel
build/install/boot is implied. The qualified rb5 host/profile remain unchanged.
Physical ownership, hardware-address policy and restore publication/error cleanup
remain open; source review found published instances freed by some later errors.


`0012-defer-boot-instance-publication.patch` applies after0011. Boot allocation
now stays private until CPU/DTB/device/IPI preparation succeeds. Locked IDR/list
publication and root assignment follow preparation; failures release private
copies and unmap borrowed rings rather than returning resources to root. The
original root rejects incomplete CPU metadata. [Focused failure/source/object
proof](../../evidence/runtime-20261012/g7-kernel-restore-publication/README.md)
covers every allocation ordinal, publication/ID collisions, CPU metadata and
mapping failures. CPU-mask rollback and physical-range authority remain open.

The [cumulative twelve-overlay kernel/module build](../../evidence/runtime-20261012/g7-kernel-twelve-overlay-build/README.md)
passes as rb6. New host/guest installation and acceptance remain unqualified.


`0013-stop-boot-on-invalid-resource-handoff.patch` applies after0012. Invalid or
duplicate boot handoffs halt immediately; restoration requires the DTB and a
positive child ID. An early initcall guard halts on restore failure/absent root.
Child PCI/platform probing is denied until root policy exists, including a NULL
root guard for platform devices. Normal no-manifest hosts keep ID0 behavior.
[Source/object proof](../../evidence/runtime-20261012/g7-kernel-boot-policy/README.md)
retains real-parser and extracted-code checks, old failure controls and actual
GCC objects. Deliberate boot halt, full new build/boot and CPU assignment semantics
require further qualification. Optional host-channel warning policy is unchanged.


`0014-commit-validated-boot-cpu-policy-at-publication.patch` applies after0013.
It prepares an exact child present-mask policy privately, refusing missing/empty
sets, unknown/out-of-range/absent or aliased CPU mappings and a missing boot CPU.
The mask is committed under the instance mutex only after all fallible instance
preparation and IDR allocation succeed. Cleanup leaves the original mask intact
on every pre-publication failure. Normal host and optional host stubs preserve
their present mask. [CPU policy proof](../../evidence/runtime-20261012/g7-kernel-cpu-restore/README.md)
retains both cpumask representation checks, actual restore/commit fault checks
and real GCC objects. Early CPU enumeration effects, physical ownership and
actual new host/guest behavior remain separate acceptance requirements.

The [cumulative fourteen-overlay rb7 kernel/module build](../../evidence/runtime-20261012/g7-kernel-cpu-restore/README.md)
passes. Valid host/guest boot and deliberate boot halt/recovery remain unqualified.

The [actual fourteen-overlay rb7 host/both guests](../../evidence/runtime-20261012/g7-kernel-rb7-matrix/README.md)
now pass all19shared ctr/Docker features and independent final19zero/no-pool
inventory. Matching profile/artifact/source identities and original/rb5 recovery
assets are retained. Deliberate invalid-boot halt/recovery, early topology effects
and broader security acceptance remain separate open requirements.


`0015-pci-refuse-domain-aliases-and-failed-bridge-reads.patch` applies after0014.
It refuses PCI domains outside the DTB u16 representation before comparison and
requires successful bridge routing reads. Failed reads cannot authorize a
bridge through plausible or all-ones output bytes. [Source/object proof](../../evidence/runtime-20261012/g7-kernel-pci-policy/README.md)
retains actual extracted guard/UBSan cases, old-source controls and actual kernel
GCC object compilation. New cumulative build/live hardware qualification remains
open; the qualified rb7 tree/binaries and clean host are unchanged.


`0016-platform-preserve-canonical-acpi-policy-identity.patch` applies after 0015.
Canonical unnumbered ACPI platform devices carry the same firmware name/HID into
the generic registration policy gate. Derived devices retain explicit name checks.
Name OR HID semantics, pre-resource refusal and existing cleanup are preserved.
[Exact source/object and full sixteen-overlay build proof](../../evidence/runtime-20261012/g7-kernel-platform-policy/README.md)
retains modeled ACPI/non-ACPI checks and the compiled old negative control.
The original build completed before the user's restart and was recovered from
its journal. Current host is original; selected guest profile is rb7. Rb8 is
built but not installed or live-qualified. Earlier rb7 live proof is historical.


`0017-overlay-validate-memory-ranges-before-page-conversion.patch` applies after
0016. Dynamic overlay memory ranges reject zero/unaligned, narrowing and
inclusive-end overflow before PFN conversion or pool/native calls, including
inverse rollback paths. Size-only pool growth rejects native-size narrowing.
[Source/object proof](../../evidence/runtime-20261012/g7-kernel-overlay-memory/README.md)
retains exact-reader UBSan/native-width models, an old-reader control and actual
GCC object compilation with all qualified rb8 identities unchanged. This patch
is not installed/booted; physical ownership and new live qualification remain open.


`0018-overlay-bound-pci-and-driver-properties.patch` applies after 0017.
The dynamic overlay PCI reader requires one bounded complete BDF string and
checks all fields before narrowing. Optional driver strings must terminate
within the actual payload capacity before logs or device transfer; missing and
empty overrides remain supported. [Source/object proof](../../evidence/runtime-20261012/g7-kernel-overlay-device/README.md)
retains both compiled old controls, complete operation/mutation fixtures and
actual GCC object compilation. Hardware binding, whole-transaction atomicity,
physical ownership and new cumulative live qualification remain open.


The [complete eighteen-overlay rb9 kernel/module build](../../evidence/runtime-20261012/g7-kernel-eighteen-overlay-build/README.md)
now passes with all 31 actual retained proof/artifact members verified, unchanged
feature configuration and thirteen unchanged prior inputs. This is build proof;
matching host/guest behavior, hardware ownership and broader G7 remain separate.


[Actual eighteen-overlay rb9 cold host/new child](../../evidence/runtime-20261012/g7-kernel-rb9-live/README.md)
now confirms host/child release, two-CPU commit,96structural refusals, real pool
rollback, matching activated guest profile, unchanged daemon bounds/identities
and independent pre-matrix19zero inventory. Full feature matrix is a separate
live job; semantic hardware policy, physical ownership and broader G7 stay open.


`0019-overlay-bound-metadata-and-selector-strings.patch` applies after 0018.
Metadata, target routing and instance-name readers require one property-local
terminated string, preventing embedded-NUL prefix aliases and unbounded copies.
Malformed initial metadata returns before transaction ID/effects/publication and
frees private allocations. Optional absent/empty and long bounded display labels
retain compatibility. [Source/object proof](../../evidence/runtime-20261012/g7-kernel-overlay-strings/README.md)
retains actual apply/reader models, both old controls and real GCC compilation.
The qualified rb9 inputs/binaries stay unchanged; this patch is not deployed.
Whole later-fragment atomicity and remaining scalar/ownership work stay open.


The [actual rb9 host/both guests and all19shared features](../../evidence/runtime-20261012/g7-kernel-rb9-matrix/README.md)
now pass, including independent final19zero/no-pool inventory, actual identities
and three preserved recovery profiles. This qualifies the eighteen-overlay normal
compatibility checkpoint. Source0019 remains isolated source/object proof; broader
security/hardware/ownership and deliberate invalid-boot recovery remain open.


Patch0020 rejects malformed optional scalar/NUMA properties, unsafe instance IDs
and allocator bounds, and uses unaligned-safe CPU cell readers. [Source/two-object
proof](../../evidence/runtime-20261012/g7-kernel-overlay-scalars/README.md) retains six focused groups,
seven compiled old controls, regressions, actual GCC artifacts and a fresh clean
host audit. Patches0019/0020 remain undeployed; eighteen-overlay rb9 is the last
fully qualified deployment. Whole transaction atomicity and G7/G8 remain open.


Patch0021 allocates the complete inactive overlay transaction directory/attributes
before resource effects, preventing lost tracking on allocation failure.
[Source/object proof](../../evidence/runtime-20261012/g7-kernel-overlay-publication/README.md) retains six
modeled allocation failures, old-source failure, regressions and actual isolated
GCC compilation. Patches0019–0021 remain undeployed; whole rollback remains open.


Patch0022 serializes overlay transaction ID allocation and saturates at INT_MAX
before sysfs/resource effects, preserving positive unique lifetime identities.
[Boundary/old-control and actual object proof](../../evidence/runtime-20261012/g7-kernel-overlay-id-lifetime/README.md)
passes. Patches0019–0022 remain undeployed. Full rollback completion/ownership
tracking and G7/G8 acceptance remain open.


Patch0023 preallocates an overlay operation-completion ledger, consumes only
completed operations in reverse and refuses automatic replay after uncertain
resource failures. Failed rmdir retains its record; unsupported pool-growth and
instance-removal inverses now return errors. [Proof](../../evidence/runtime-20261012/g7-kernel-overlay-progress/README.md)
retains actual traversal/rmdir/unsupported-function models, two compiled old
controls, regressions and real isolated GCC object. Sources0019–0023 remain
undeployed. Exact target incarnation/ownership, reconciliation/teardown and whole
atomicity remain open; normal rb9 qualification covers eighteen overlays.


Patch0024 gives boot/runtime instances unique immutable incarnation tokens and
checks them before resource or created-instance inverses. Reused names/IDs no
longer authorize those inverses. [Proof](../../evidence/runtime-20261012/g7-kernel-overlay-identity/README.md)
retains generation/concurrency/exhaustion models, actual allocation/rollback
fixtures, two compiled old controls and four kernel objects compiled against the
new header with actual dependency proof. Sources0019–0024 remain undeployed;
changed instance layout requires a full rebuild before any kernel/module use.
Original resource ownership, reconciliation/teardown and whole G7/G8 remain open.


All twenty-four overlays now [build/link as rb10](../../evidence/runtime-20261012/g7-kernel-twentyfour-overlay-build/README.md).
Exact37member archive,17inputs/3binaries, four actual rebuilt objects, linked
generation/FDT guards and final existing-host19zero/no-pool inventory verify.
Feature config differs only in LOCALVERSION; eleven prior inputs stay unchanged.
User-authorized [root expansion](../../evidence/runtime-20261012/g7-root-storage-expansion/README.md) gives
193GiB root/~106GiBfree; separate/bootremains69MiBfree. New kernel is not installed
or booted. Preserve recovery assets before installation and qualify actual new
host/child/full matrix. Broader G7/G8 acceptance remains open.


The current deployed checkpoint supersedes the installation status above:
[rb10 host/child qualification](../../evidence/runtime-20261012/g7-kernel-rb10-live/README.md)
and [all19shared features/final audit](../../evidence/runtime-20261012/g7-kernel-rb10-matrix/README.md)
pass with all24overlays. Recovery artifacts/default are preserved; root200GB
and verified obsolete-boot retirement provided installation space.

Patch0025 validates complete CPU/device item lists before operation-local
resource moves, forward and undo. [Proof](../../evidence/runtime-20261012/g7-kernel-overlay-item-preflight/README.md)
retains144modeled malformed cases, compiled old-source control, actual reader/
ledger regressions and isolated kernel GCC object with17qualifiedrb10inputs and
3binaries unchanged. This source candidate is not deployed or boot-qualified;
whole-overlay/backend atomicity, ownership/reconciliation and G7/G8 remain open.


Patch0026 validates complete memory-add/remove item lists before operation-local
allocation/shrink/park teardown/remote moves, forward and undo. [Proof](../../evidence/runtime-20261012/g7-kernel-overlay-memory-preflight/README.md)
retains66modeled malformed cases, old control, five memory-reader width/page
models, existing CPU/device/ledger regressions and actual isolated kernel object
with17qualifiedrb10inputs/3binaries unchanged. Candidates0025/0026 remain
undeployed; whole-overlay atomicity/resource ownership/reconciliation remain open.


Patch0027 preflights all fragments/operations before applying the first one, using
a private validation descriptor and the existing readers. Create configuration
parsing/failure, namespace removal and future-instance target syntax stay free
of live identity/resource actions during preflight. [Proof](../../evidence/runtime-20261012/g7-kernel-overlay-transaction-preflight/README.md)
retains24modeled malformed positions, read-only path/old control/regression checks
and actual kernel object;17rb10inputs/3binaries stay unchanged. Candidates25–27
remain undeployed. Resource atomicity/ownership/outcomes/reconciliation remain open.


Patch0028 makes config initialization return a required checked result and refuse
CPU-set allocation failure. Runtime, boot and preflight callers stop and clean up;
parser entry/CPU guards prohibit silently skipped validation. [Proof](../../evidence/runtime-20261012/g7-kernel-config-init/README.md)
retains init/parser/annotation/runtime/boot/preflight failure checks, old controls,
regressions, complete caller inventory and four actual modified-header objects.
17qualifiedrb10inputs/3binaries remain unchanged. Candidates25–28 remain undeployed;
cumulative link/boot and remaining ownership/reconciliation acceptance stay open.


Patch0029 validates CPU source/destination records and reserves destination
tracking before context/repark/IPI changes, active and non-running. Unexpected
post-ACK bookkeeping errors require reconciliation and never pretend success.
[Proof](../../evidence/runtime-20261012/g7-kernel-cpu-ownership/README.md) retains focused initiator/
old-control/UBSan checks and actual kernel object with candidate28header plus
17baseline/3reviewedinput/3binary preservation. Candidates25–29 are not deployed;
concurrency, peer identity, original ownership/per-item outcomes and live CPU
hotplug/cumulative acceptance remain open.


All29patches now [build/link as rb11](../../evidence/runtime-20261012/g7-kernel-twentynine-overlay-build/README.md),
with46exact archive members,20inputs/3binaries,6actual rebuilt objects and20before
snapshots verified. Features differ only inLOCALVERSION; rb10artifactdir/archive
and independent19zero old-host inventory are retained. This supersedes the prior
unlinked status of candidates25–29. New host/child/CPU hotplug acceptance is a
separate qualification step; broad resource/peer/concurrency/G7/G8 criteria remain.


[Rb11 live qualification](../../evidence/runtime-20261012/g7-kernel-rb11-live/README.md) supersedes the prior pending boot/hotplug
status: actual host and matching child boot, physical CPU12 add/remove on a
running child, independent cleanup,96malformed-write refusals,real allocation
rollback and matching runtime-profile activation pass. Five recovery profiles
and original GRUB default remain preserved. Full19feature suite is separate
and pending; broad ownership/concurrency/peer/recovery/G7/G8 criteria stay open.


[Rb11 full feature qualification](../../evidence/runtime-20261012/g7-kernel-rb11-matrix/README.md) now passes all19shared features
through containerd/Docker on the same cold host boot and matching guest kernels.
The suite terminates success13:53:34UTC; a separate final audit confirms19zero
counters/no instances/no pool. Five recovery profiles and capability/NoNewPrivs
bounds remain verified. This supersedes the pending-feature status above;
whole SEC-04/G7/G8 acceptance and remaining ownership/failure criteria stay open.


`0030-messaging-correlate-request-ack-and-complete-once.patch` addresses late
ACKs satisfying newer same-resource requests and duplicate ACK result overwrite.
A random boot epoch/nonwrapping sequence is registered, copied through six
resource workers and shutdown, and matched exactly; consumption shares the ACK
lock and completion is accepted once. Exact layouts/nonzero request IDs and a
new wire subtype namespace prevent older decoders misreading enlarged payloads.
Matched host/child kernels are required. CPU-add registers before first repark.
[Source/three-object proof](../../evidence/runtime-20261012/g7-kernel-request-correlation/README.md)
passes modeled actual-function/concurrency/old-control checks and isolated real
messaging/hotplug/core compilation against the new header. Qualified rb11 stays
unchanged; candidate30 is not linked/deployed. Peer authentication, repeated
request suppression, per-resource/uncertain ownership and lifecycle/recovery
remain open; random epochs are probabilistic boot separation, not durable IDs.


`0031-cpu-transfers-retain-ownership-and-unsafe-release.patch` follows0030.
Active CPU-add transfers accounting before repark/send; post-effect errors keep
the CPU out of the free pool, with sticky uncertainty rather than speculative
inverse. Atomic per-instance CPU/release gates exclude in-flight CPU changes
from image/directory release. Slot membership precedes hot-add/secondary/boot
wakes, survives timeout, and confirmation covers every owned CPU. Arch release
and CPU return precede memory/metadata destruction. Unsafe image/last-reference
release retains backing memory and ownership records; read-only transfer and
retention inspection is added. [Source/six-object proof](../../evidence/runtime-20261012/g7-kernel-cpu-retention/README.md)
passes focused actual-function/old-control/UBSan checks and actual messaging/
hotplug/core/spawn/kernfs/kexec_core compilation with the new header. Qualified
rb11 remains unchanged. Global pool writers/general lifecycle serialization,
checked unload error propagation, bounded retention inspection/managed recovery,
peer identity and full group link/boot remain open.


`0032-kexec-propagate-unload-refusals-and-bound-retention.patch` follows0031.
Checked image free returns gate/park errors before registry removal. Targeted
unload propagates the result under the kexec lock; bulk unload stops on refusal
and uses a128-release budget with explicit E2BIG retry rather than silently
truncating at ten images. Retained-instance reporting caps rows at64 and marks
truncation. [Source/four-object proof](../../evidence/runtime-20261012/g7-kernel-checked-unload/README.md)
retains passing actual-function models, three compiled failing predecessor
controls and real core/kexec_core/kexec/kexec_file objects. An initial dependency
check failure and the resumed fourth-object compile are both retained; completed
objects were reused unchanged. Candidates30–32 remain undeployed. Full group
link/boot, live refusal/retry, global serialization and managed recovery remain open.

`0033-cpu-pool-serialize-claims-and-retain-transitions.patch` follows0032.
A shared ownership mutex serializes common CPU-pool transfers/returns, host
accounting and guarded array snapshots. Host/pool transitions claim ownership
before hotplug, retain uncertain/failed publication outcomes and expose bounded
root-only state. No ownership mutex spans physical hotplug/park/IPI waits.
Reserve count overflow and transfer request aliases are rejected.
[Source/seven-object proof](../../evidence/runtime-20261012/g7-kernel-cpu-pool-serialization/README.md)
retains focused pthread/actual-function/UBSan checks, compiled predecessor
controls and seven real modified-header objects. General lifecycle/slot-array
synchronization, atomic park-area check/free, managed reconciliation and complete
group link/matching host-child qualification remain open.

`0034-park-quiesce-admission-and-pin-physical-users.patch` follows0033.
Park teardown commits admission closure with the empty ownership check; successful
complete setup reopens it. Physical removal helpers select/pin host-park usage
before CPU removal, including deferred worker routes. Online recovery clears the
pool flag and per-CPU selector before unpinning under CPU read-side exclusion.
Pins survive unsafe outcomes and block teardown; reporting stays bounded. A
separate local physical-hotplug mutex protects helper pin/unpin sequencing.
[Source/three-object proof](../../evidence/runtime-20261012/g7-kernel-park-quiescence/README.md)
retains actual-function/failure models and two compiled controls. A reviewed
intermediate build and revised final build are distinguished. Full group link/
matching host-child qualification, general instance/slot lifecycle serialization,
managed reconciliation and live fault/native-hotplug proof remain open.


Patches30–34 now have a [complete cumulative34 kernel/modules build](../../evidence/runtime-20261012/g7-kernel-thirtyfour-overlay-build/README.md).
The15 final source paths match frozen predecessors, only LOCALVERSION changes,
and12 rebuilt objects/ten new linked symbols are verified. Qualified rb11 and
26original inputs are preserved. Installation/matching boot and live shared-feature
qualification remain separate; this build does not close SEC-04/G7/G8.


`0035-root-cpu-inventory-sync-after-secondary-startup.patch` follows0034.
Actual rb12 host boot passed, but the [normal16GiB/eight-CPU baseline failed](../../evidence/runtime-20261012/g7-kernel-rb12-baseline-refusal/README.md)
before child boot: early root restoration recorded onlyCPU0 before SMP startup,
so the strict ownership check refused all requested secondary CPUs. The
[source/model correction](../../evidence/runtime-20261012/g7-kernel-root-cpu-sync/README.md)
adds late initialization of currently online physical ownership, preserves early
manifest policy/strict checks and refuses incomplete boot on tracking failure.
The predecessor absence control fails the strict claim;16CPU/sparse/failure models
pass. A distinct rb13 build and normal-pool live qualification are required.
The successful cumulative34 build does not supersede this observed live failure.
