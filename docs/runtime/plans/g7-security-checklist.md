# G7 STRIDE security review and comparison tasks

Status: assessment in progress, 2026-10-12. SEC-01 and SEC-11 have completed
source assessments with an architectural limitation; SEC-02 has source and
portable, baseline live, and six-case remediation coverage but remains open for
production qualification and remaining authority review. See the
[boundary assessment](../research/g7-security-boundary-assessment.md). Checked
assessment tasks do not signify that the security property passed. Use the
[STRIDE threat model](../research/stride-threat-model.md),
[G7 plan](07-security-and-reliability.md), and
[G1 audit](../research/g1-pinned-isolation-audit.md) together.

## Execution and evidence rules

Assign an owner before starting each task. Retain exact source/binary/kernel
hashes, configuration, initial attacker privileges, assertions, observed access,
failures, and final ownership/resource inventory. Label portable tests and
privileged tests separately. Run hostile-child, crash, reset, and exhaustion
experiments only on disposable hosts with synthetic data and a recovery path;
cloud runs require before/after cost and cleanup ledgers.

Do not run security probes as an implicit consequence of writing this checklist.
For active testing, first define bounded fixtures and specific stop conditions.
A negative result means the tested route failed under that configuration, not
that escape is impossible. Findings need reproduction, impact, remediation or
explicit disposition, and a verification rerun.

## Current execution priority — 2026-10-18

Owner/executor: Codex. The user prioritizes completing these STRIDE tasks with
retained proof. Main accounting is two checked assessments and eleven open
tasks; kernel patch counts and compatibility percentages are not G7 completion
percentages. Acceptance criteria below remain the checkoff requirements.

The rb13 normal-pool baseline passed with retained proof; its broader
qualification remains open. Prioritize SEC-12's complete dependency and
artifact-trust review, followed by SEC-08's secret/audit trace and SEC-03's
remaining authorization cases. Record findings, implement required fixes and
retain exact-source verification against each task's acceptance criteria.
Continue SEC-02/SEC-10 authority review, SEC-05 fuzzing, SEC-07 mediation and
SEC-09 fault/soak coverage; produce the SEC-13 comparison and completion review.
Critical findings can change the order, with the reason recorded in the
[implementation notes](../../../evidence/runtime-20261012/g7-implementation-notes.md).
Do not replace a whole-task checkoff with another scoped kernel milestone.

## First priority: evaluate the escape hypothesis

- [x] **SEC-01 — Freeze the claim and attacker tiers (TM-09/TM-12).**
  Owner/source reviewer: Codex. Outcome: source assessment completed;
  hostile-child containment is an architectural limitation. Independent review
  remains pending in the completion review.
  Re-audit direct boot, page-table control, E820, shared memory/IPI, CPU handoff,
  APIC/MSR/I/O, and DMA behavior at the deployed kernel revision. Separate
  process compromise, child root, and child-kernel execution. Deliver a reviewed
  claim matrix identifying software policy versus independently enforced
  barriers. Decision criterion: retain trusted-single-tenant scope wherever
  hostile-child barriers are missing; do not claim superiority to gVisor.

- [ ] **SEC-02 — Inventory actual workload authority (TM-05).**
  Owner/source reviewer: Codex. Source review and agent race suite complete;
  baseline inventory and six-case remediation rerun complete. Command-line
  credential exposure and the tested bootstrap canary route are fixed and
  verified for four workload profiles; unsafe capability and disabled-NNP
  requests fail closed. Mediated-root/Docker qualification, shared-procfs/root
  signal review, and independent review remain open. See the
  [remediation evidence](../../../evidence/runtime-20261012/g7-authority-remediation/README.md).
  The [builder-guest prototype](../research/guest-image-builds.md) additionally
  verifies offline image builds with automatic trusted namespace preparation,
  zero-capability/NNP client execution and retained canary/credential checks.
  Review its mapping helpers, local socket authorization, service lifecycle,
  nested namespace authority and production integration before qualifying it.
  The [Docker-in-Docker follow-up](../research/docker-in-docker-guests.md)
  verifies ordinary Docker build/run/save commands with a rootless guest-local
  daemon, client caps zero, and nested NNP/seccomp. Offline prototype evidence
  does not close production networking/storage or independent authority review.
  The [online Python follow-up](../research/docker-online-python-guest.md)
  additionally verifies Docker Hub pulls, HTTPS PyPI package installation,
  runtime HTTPS and wheel hashes in a static diagnostic networking profile.
  Full production CNI/storage and independent authority review remain open.
  Inspect [agent execution](../../../runtime/agent/oci_exec_linux.go),
  [OCI validation](../../../runtime/agent/manager.go), root policy, and bootstrap.
  In non-root and supported root fixtures, enumerate effective capabilities,
  mounts, accessible devices, `/proc`, `/sys`, agent endpoints, module-loading
  interfaces, token/command-line exposure, and inherited file descriptors.
  Attempt bounded reads of synthetic credentials and writes to runtime-owned
  canaries. Acceptance: every exposed authority is documented; unintended
  credential/descriptor exposure is fixed; unsupported OCI controls fail closed.
  Seccomp and namespace/resource projection are currently rejected rather than
  enforced by the agent, so proposed confinement must account for that limit.

- [ ] **SEC-03 — Audit authentication and operation authorization (TM-01).**
  Review [daemon RPC](../../../runtime/internal/daemon/server.go),
  [network RPC](../../../runtime/internal/network/rpc.go),
  [agent server](../../../runtime/agent/server.go), and shim reconnection.
  Test wrong UID/token/version/endpoint, reused sandbox names, stale generation,
  unauthenticated data paths, and reconnect after restart. Include authenticated
  peers requesting unauthorized operations. Acceptance: requests fail before
  mutation, returned descriptors belong to the expected generation, and no
  rejected request leaks credentials or changes another sandbox.
  Scoped proof: [eight agent rejection cases and native handshake checks](../../../evidence/runtime-20261012/g7-auth-handshake/README.md)
  pass locally with exact sources and mutation assertions. Full authorization
  coverage and live native rejection/export survival remain open.

## Repository and protocol review

- [ ] **SEC-04 — Review privileged input conversion (TM-02).**
  [Rb13 normal-pool baseline](../../../evidence/runtime-20261012/g7-kernel-rb13-normal-pool/README.md)
  now passes the unchanged16GiB/eight-CPU pool, matching2CPU guest boot and
  independent pre/post19zero/no-pool cleanup after candidate0035. Full rb13
  CPU-hotplug/unload/refusal/rollback/profile/matrix acceptance remains open.
  [Actual rb12 baseline refusal](../../../evidence/runtime-20261012/g7-kernel-rb12-baseline-refusal/README.md)
  prevents child/workload qualification despite successful full link/host boot.
  [Candidate0035 post-SMP inventory](../../../evidence/runtime-20261012/g7-kernel-root-cpu-sync/README.md)
  corrects missing secondary ownership without weakening checks; focused models
  pass. Fixed full build and normal-pool matching-kernel qualification remain open.
  [Complete cumulative34 build](../../../evidence/runtime-20261012/g7-kernel-thirtyfour-overlay-build/README.md)
  links request correlation, checked unload, ownership and park-lifetime changes.
  Twelve rebuilt objects/ten linked symbols and preserved26before-inputs pass;
  matching host/child and full feature qualification remain separate.
  [Candidate0033 CPU-pool claims and transitions](../../../evidence/runtime-20261012/g7-kernel-cpu-pool-serialization/README.md)
  serializes shared accounting and reserves host/pool moves before effects,
  retaining unsafe outcomes with bounded inspection. Focused actual-function
  concurrency models and seven real objects pass. General lifecycle, atomic
  park-area teardown, managed reconciliation and full group live proof remain open.
  [Candidate0032 checked unload and bounded retention](../../../evidence/runtime-20261012/g7-kernel-checked-unload/README.md)
  propagates release refusal before registry removal, reports bulk partial
  progress/budget exhaustion, and bounds retained records. Actual-function
  models, three compiled old controls and four real objects pass; full group
  link/boot, live failure/retry and managed recovery remain open.
  [Candidate0031 CPU ownership/release retention](../../../evidence/runtime-20261012/g7-kernel-cpu-retention/README.md)
  claims ownership before park/IPI effects, records slot intent before wakes,
  gates CPU/release operations and retains unsafe image/instance resources.
  Actual-function fault models, four compiled old controls and six real objects
  pass. Full link/boot, global serialization and checked recovery remain open.
  [Candidate0030 request/ACK correlation](../../../evidence/runtime-20261012/g7-kernel-request-correlation/README.md)
  adds nonwrapping request identities, one completion, copied deferred-worker
  identities and strict versioned layouts across resource/shutdown operations.
  Actual-function models, compiled old controls and three real objects pass;
  complete link/boot and transport peer/physical-outcome acceptance remain open.
  [Rb11 live qualification](../../../evidence/runtime-20261012/g7-kernel-rb11-live/README.md)
  passes host/child boot, active CPU add/remove and cleanup, 96 malformed-write
  refusals, real allocation rollback and matching profile activation. These
  scoped milestones are complete. The [full rb11 feature suite](../../../evidence/runtime-20261012/g7-kernel-rb11-matrix/README.md)
  passes all 19 features through both frontends and an independent final
  nineteen-counter zero-resource audit. This does not close SEC-04.
  [Complete29patch rb11 kernel/modules build](../../../evidence/runtime-20261012/g7-kernel-twentynine-overlay-build/README.md)
  now passes cumulative link, preserved feature config, six rebuilt objects and
  protected20beforeinputs. Independent existing-host19zero inventory and prior
  rb10artifacts remain verified. Cold host/child/CPU-hotplug qualification is
  tracked separately; this does not check off SEC-04.
  [Candidate CPU initiator ownership/capacity](../../../evidence/runtime-20261012/g7-kernel-cpu-ownership/README.md)
  checks records and reserves destination tracking before repark/IPI; unexpected
  tracker failure reports reconciliation instead of success/free-pool alias.
  Focused/old-control and actual kernel object proof pass; per-resource identity,
  peer/concurrency/physical ownership and live qualification remain open.
  [Candidate checked config initialization](../../../evidence/runtime-20261012/g7-kernel-config-init/README.md)
  refuses required CPU-set allocation failure across runtime/boot/preflight
  callers, with parser-entry guards and checked return annotation. Focused
  failure/old-control and four actual modified-header objects pass; broader
  ownership, fault acceptance and link/boot qualification remain open.
  [Candidate transaction-wide input preflight](../../../evidence/runtime-20261012/g7-kernel-overlay-transaction-preflight/README.md)
  validates all fragments/operations before effects, including read-only create
  config parsing and syntax-only future targets. Focused/model/old-control and
  actual object proof pass; backend failures, ownership and boot qualification
  remain open.
  [Candidate memory item-list preflight](../../../evidence/runtime-20261012/g7-kernel-overlay-memory-preflight/README.md)
  validates complete memory-add/remove lists before effects, forward and undo.
  66modeled malformed cases, compiled old control, reader/ledger regressions and
  actual isolated kernel object pass; whole-overlay/physical ownership and
  backend atomicity remain open.
  [Candidate CPU/device item-list preflight](../../../evidence/runtime-20261012/g7-kernel-overlay-item-preflight/README.md)
  validates an entire operation before moves in forward/undo paths;144modeled
  malformed cases, old-source control, regressions and isolated real kernel
  object pass. This candidate is not linked/boot-qualified; ownership and
  backend failure atomicity remain open.
  [Eighteen-overlay rb9 cold host/child qualification](../../../evidence/runtime-20261012/g7-kernel-rb9-live/README.md)
  now retains actual matching kernel/profile, CPU commit, structural refusals,
  real rollback and independent pre-matrix zero inventory. The
  [full nineteen-feature matrix and independent final inventory](../../../evidence/runtime-20261012/g7-kernel-rb9-matrix/README.md)
  also pass. [Scalar/ID and allocator conversion bounds](../../../evidence/runtime-20261012/g7-kernel-overlay-scalars/README.md)
  pass focused/old-control and actual two-object checks. [Preallocated overlay
  tracking](../../../evidence/runtime-20261012/g7-kernel-overlay-publication/README.md)
  prevents transaction-file allocation failures after resource changes, with
  six modeled failures and actual object proof. [Transaction ID lifetime](../../../evidence/runtime-20261012/g7-kernel-overlay-id-lifetime/README.md)
  now refuses exhaustion before publication/resource effects; boundary/old-source
  and actual object checks pass. [Completion-ledger rollback](../../../evidence/runtime-20261012/g7-kernel-overlay-progress/README.md)
  consumes completed inverses once and refuses uncertain primitive replay;
  focused rmdir/fault and real object proof pass. [Instance-incarnation checks](../../../evidence/runtime-20261012/g7-kernel-overlay-identity/README.md)
  now bind instance inverses to shared non-recycled generation tokens, with
  boot/runtime allocation checks and four real objects. The
  [complete twenty-four-overlay rb10 kernel/modules](../../../evidence/runtime-20261012/g7-kernel-twentyfour-overlay-build/README.md)
  now build/link with the modified layout and preserved feature config.
  [Actual rb10 host/child qualification](../../../evidence/runtime-20261012/g7-kernel-rb10-live/README.md)
  passes cold boot, 96 structural refusals, real partial-growth rollback and
  matching profile activation. All24 overlays are deployed; [all nineteen shared features and independent final inventory](../../../evidence/runtime-20261012/g7-kernel-rb10-matrix/README.md)
  pass. Physical ownership and complete conversion
  acceptance remain open. This does not check off SEC-04.
  [Manifest/IPI range and producer-page bounds](../../../evidence/runtime-20261012/g7-kernel-ipi-ranges/README.md)
  require exact field widths, page-rounded shared ABI, alignment/type/extent
  bounds and disjoint declarations before early CPU/restore effects. Real
  parser/old controls and actual GCC objects/ABI measurement pass; mapping/
  allocation/CPU effects are modeled. The [twelve-overlay full kernel build](../../../evidence/runtime-20261012/g7-kernel-twelve-overlay-build/README.md)
  passes. [Deferred publication/private cleanup](../../../evidence/runtime-20261012/g7-kernel-restore-publication/README.md)
  fixes published-object error frees with focused fault/object proof. Physical
  ownership remains open. [Boot CPU policy preparation and
  publication commit](../../../evidence/runtime-20261012/g7-kernel-cpu-restore/README.md)
  now refuse missing/unknown/absent/aliased assignments or an excluded boot CPU
  and preserve the present mask on every pre-publication failure. Both mask
  representation fixtures and actual GCC objects pass. Early CPU registration
  effects remain open. The [fourteen-overlay rb7 host/both guests](../../../evidence/runtime-20261012/g7-kernel-rb7-matrix/README.md)
  now pass all19shared ctr/Docker features and an independent final19zero/no-pool
  inventory with exact installed identities/capability bounds retained.
  [Boot failure handling](../../../evidence/runtime-20261012/g7-kernel-boot-policy/README.md)
  now halts invalid/duplicate handoffs or failed/absent restoration, requires a
  positive child ID/DTB and denies child probing before root restoration. Actual
  GCC objects/focused checks and cumulative build/valid host/guest boot pass;
  deliberate invalid-boot panic/recovery behavior remains unqualified.
  [Complete named-root tree validation](../../../evidence/runtime-20261012/g7-kernel-instance-dtb/README.md)
  adds an allocation-free structural/reservation/property walk before existing
  reader access. Valid roots/NOPs and12malformed old-source controls plus unread
  subtree/output-preservation integration pass. The [cumulative ten-overlay build](../../../evidence/runtime-20261012/g7-kernel-ten-overlay-build/README.md)
  passes with actual binaries/source/config/journal verified locally. The
  [rb5 host/minimal guest boot,96live refusals and rollback](../../../evidence/runtime-20261012/g7-kernel-rb5-live/README.md)
  also pass; its rb5 guest profile was active at that checkpoint after all19shared
  ctr/Docker features and independent final cleanup pass with actualrb5host/
  guests. Device/resource semantics and broader acceptance remain open.
  [Ninth-overlay generic device-property hardening](../../../evidence/runtime-20261012/g7-kernel-device-properties/README.md)
  validates property-local strings, PCI ranges before narrowing, exact platform
  field capacity and the complete device-name list before lookup/allocation.
  Four real-parser/UBSan groups, old-source refusal controls and the actual
  kernel object pass; qualified rb4 sources/binaries remain unchanged. Cumulative full build passes in the linked ten-overlay proof; new host/guest
  and actual kernel device behavior plus broader acceptance remain open.
  [Seventh/eighth kernel overlays](../../../evidence/runtime-20261012/g7-kernel-dtb-entry-bounds/README.md)
  add complete baseline/overlay/outer-manifest checks, actual-buffer bounds for
  named instance trees and exact ID conversion before allocator-end overflow.
  Real parser/refusal controls and kernel object/symbol checks pass. The
  [cumulative build](../../../evidence/runtime-20261012/g7-kernel-eight-overlay-build/README.md)
  and [new host/minimal guest boot, 96 live refusals and actual rollback](../../../evidence/runtime-20261012/g7-kernel-rb4-live/README.md)
  pass. All 19 shared ctr/Docker features and independent final cleanup pass. Named-root full structure,
  remaining resource/string semantics and broader acceptance remain open.

  [Kernel baseline input bounds](../../../evidence/runtime-20261012/g7-kernel-baseline-inputs/README.md)
  add DTB length/offset guards, bounded single-string properties and PCI
  full-consumption/range checks. Real-libfdt fixtures and isolated real kernel
  objects pass. Later cumulative build/boot and standard baseline full-tree
  qualification pass in the linked eight-overlay proof; generic device parsing,
  remaining resource semantics and other reader acceptance remain open.
  The [conversion trace/review](../research/g7-privileged-conversion-review.md)
  records source boundaries, retained controls and remaining acceptance work.
  Executable lookup now uses the child PATH, nil environments do not inherit
  parent secrets, root builders retain only explicit operator settings, and
  native helpers reject malformed arguments before host operations. Local
  canary regressions and identified live compatibility/restart qualification
  pass; see [proof](../../../evidence/runtime-20261012/g7-exec-environment/README.md).
  [Kerf configuration path trust](../../../evidence/runtime-20261012/g7-kerf-executable-trust/README.md)
  now rejects writable parents and untrusted intermediate redirects while
  preserving managed links; portable regressions and full race/vet/build pass.
  [Builder path/inode pinning](../../../evidence/runtime-20261012/g7-builder-executable-trust/README.md)
  additionally passes held-generation replacement and unsafe-parent canary
  regressions locally. [Kerf launch-time pinning](../../../evidence/runtime-20261012/g7-kerf-launch-pinning/README.md)
  now preserves virtualenv imports and credential/kernel FD numbers and refuses
  unsafe selection even with matching observed state. [Combined live qualification](../../../evidence/runtime-20261012/g7-pinning-live/README.md)
  passes production Kerf/builder compatibility, both-client network/storage/idle,
  restart and independent zero inventory. [Script interpreter trust](../../../evidence/runtime-20261012/g7-script-interpreter-trust/README.md)
  now rejects unsafe direct/env paths before mutation in local regressions.
  Fresh live qualification, ELF-loader/import/support trust and
  remaining snapshot feature/fault coverage remain open. [Snapshot hardening proof](../../../evidence/runtime-20261012/g7-snapshot-preservation/README.md)
  now retains an actual pre-fix upper metadata mutation and passing tmpfs/ext4
  lower-only overlay/recursive readonly-bind preservation, including atime.
  Production upgrade and full acceptance remain open.
  [Mount setup budgets](../../../evidence/runtime-20261012/g7-mount-budget/README.md)
  cap overlay layers/aggregate source views before lookup and reject duplicate
  path keys, with typed refusals, noexec normalization and cooperative native
  setup cancellation. Kernel/subtree pressure coverage remains open.
  The [identified integrated release](../../../evidence/runtime-20261012/g7-snapshot-live/README.md)
  passes all 19 ctr/Docker compatibility rows, restart and independent zero
  inventory. Dependency, broader snapshot/pressure/security and independent
  completion acceptance remain open.
  [ELF loader path trust](../../../evidence/runtime-20261012/g7-elf-loader-trust/README.md)
  now has bounded parser/real patched-shell refusal proof and storage-helper
  integration locally. Fresh deployment and library/import/support provenance
  still require qualification.
  [Kerf Python startup paths](../../../evidence/runtime-20261012/g7-python-startup/README.md)
  exclude implicit script/CWD search and user-site processing on Python >=3.11,
  with local shadow-module negative control and preserved virtualenv/FDs.
  Version enforcement, editable/global dependencies and fresh deployment remain open.
  [Builder support generation trust](../../../evidence/runtime-20261012/g7-builder-support-trust/README.md)
  preflights ten known helper/import files and their directories locally before
  helper execution, with unsafe helper/import negative controls. Python/library
  dependency isolation and deployment qualification remain open.
  [GCE root support qualification](../../../evidence/runtime-20261012/g7-builder-support-live/README.md)
  additionally proves foreign-owner refusal and real-host OCI regressions at
  fourteen verified uploaded source inputs. Candidate installation/full
  integration and dependency qualification remain open.
  [Isolated builder Python sources](../../../evidence/runtime-20261012/g7-builder-python-isolation/README.md)
  now exclude implicit sibling/site and support bytecode routes locally while
  preserving real helper publication. Standard-library/shared-library trust,
  fresh combined installation and independent acceptance remain open.
  [Integrated loader/Kerf/builder qualification](../../../evidence/runtime-20261012/g7-isolation-live/README.md)
  now deploys these pending controls, passes all nineteen shared ctr/Docker
  feature rows/restart and proves actual process/artifact/support identities
  plus independent zero inventory/no pool. Full security criteria, dependency
  inventory/least privilege and independent acceptance remain open.
  Trace OCI fields through shim rootfs preparation, root/image builders,
  [safe-file helpers](../../../runtime/internal/safefile/),
  [rootfs service](../../../runtime/internal/rootfs/),
  [Kerf adapter](../../../runtime/internal/kerf/), and subprocess execution.
  Test traversal, symlink substitution/races, special files, hostile IDs,
  mount options, environment/PATH influence, and argument injection.
  Acceptance: caller input cannot mutate host canaries, approved boot artifacts,
  or caller snapshots, and subprocesses use approved executables and bounded
  arguments/time/resources. Review unsafe code, cgo, and native helpers too.

- [ ] **SEC-05 — Add a targeted fuzzing campaign (TM-03).**
  Portable daemon JSON and lifecycle-journal campaigns now have bounded inputs,
  retained corpus, execution budgets, exact source and passing outputs; see
  [parser campaign proof](../../../evidence/runtime-20261012/g7-state-parsers/README.md).
  Review also fixed inherited defaults for missing persisted fields across
  lifecycle/network/storage/rootfs snapshots and rejects top-level null structs.
  Native, OCI, network-specific and remaining state parser campaigns remain open.
  Owner/implementation: Codex. Native NBD handshake matching now requires
  canonical reserved/status fields and identity padding. A pure request validator
  rejects unsupported commands/flags, malformed flush/disconnect fields and
  unsafe ranges before payload receipt or disk I/O. Portable boundary checks and
  an undefined-behavior-sanitized run pass; see the
  [scoped parser evidence](../../../evidence/runtime-20261012/g7-nbd-parser/README.md).
  Accepted-client request headers/payloads and replies now use independent
  absolute monotonic transfer deadlines while preserving indefinite idle time.
  Socketpair idle/trickle/partial-payload/unread-reply checks pass outside the
  restricted sandbox. The native/Go fuzz campaigns and live storage compatibility
  qualification remain open.
  Cover [protocol JSON/framing](../../../runtime/protocol/), OCI conversion,
  state decoding, network RPC/frame parsing, and
  [native NBD transport](../../../tools/mkvsock-nbd.c).
  Seed with valid contracts and existing boundary fixtures; mutate truncation,
  duplicates, lengths, offsets, integer limits, sequencing, and disconnects.
  Use a suitable native harness/sanitizers for C and Go fuzz targets for Go.
  Acceptance: no unresolved high-severity finding, panic, out-of-bounds access,
  unchecked allocation, unauthorized mutation, or unbounded wait in the tested
  corpus. Record budgets, coverage, harness limitations, and regression seeds.

- [ ] **SEC-06 — Audit lifecycle and recovery authorization (TM-04).**
  [Candidate0034 park admission/pin hardening](../../../evidence/runtime-20261012/g7-kernel-park-quiescence/README.md)
  closes admission with the teardown check, pins physical park users before
  removal, retires selectors before verified-online unpin, and keeps failed
  setup closed. Focused source/fault models and three real objects pass; full
  group boot, general lifecycle and managed reconciliation remain open.
  [Overlay completion ledger and uncertain rollback refusal](../../../evidence/runtime-20261012/g7-kernel-overlay-progress/README.md)
  retains completed-operation progress and failed rmdir evidence, avoiding
  inverses after failed apply and replay after partially effective inverses.
  [Instance-incarnation source/object proof](../../../evidence/runtime-20261012/g7-kernel-overlay-identity/README.md)
  now refuses same-name/ID replacement before inverses. Original resource
  ownership, reconciliation API,
  subsystem-exit evidence and full atomicity remain open; source/object only.
  [Version-aware loading and five-patch cold boot passed](../../../evidence/runtime-20261012/g7-kernel-candidate-boot/README.md).
  The subsequent [actual partial-growth test failed](../../../evidence/runtime-20261012/g7-kernel-host-park-rollback/README.md):
  a new 64 MiB chunk retained host park allocations after the second request failed.
  Patch0006 releases newly owned quiescent host park infrastructure before shrink;
  source/object and six-patch full build pass. [Corrected boot and identical live fault
  cleanup now pass](../../../evidence/runtime-20261012/g7-kernel-rb3-live/README.md); unchanged 16 GiB compatibility also passes all19shared features and the independent zero/no-pool audit. Zero ordinary counters
  alone do not satisfy the final no-pool requirement.
  [Platform metadata staging](../../../evidence/runtime-20261012/g7-kernel-device-staging/README.md)
  keeps partial device lists private, preserves previous metadata through input
  validation and frees staging on initializer errors. Source/real-parser and
  object checks pass. The [first two kernel overlays built successfully](../../../evidence/runtime-20261012/g7-kernel-two-patch-build/README.md);
  the [all-five cumulative build also passes](../../../evidence/runtime-20261012/g7-kernel-cumulative-build/README.md).
  Corrected six-patch cold boot and exact partial-growth rollback pass; broader fault/runtime qualification remains open.
  [Baseline DTB commit ownership](../../../evidence/runtime-20261012/g7-kernel-baseline-dtb/README.md)
  now reserves the retained tree before resource changes and commits it without
  a late allocation. Exact-function allocation/error/replacement checks and real
  kernel object compilation and cumulative full build/cold boot pass; actual DTB
  allocation/commit failure qualification remains open. General later CPU/device rollback is not closed.
  [Kernel partial pool-growth rollback](../../../evidence/runtime-20261012/g7-kernel-pool-rollback/README.md)
  fixes an error path that retained earlier allocations when a later request
  failed; exact-function failure injection and real kernel object compile pass.
  It preserves existing/busy chunks and reports incomplete rollback. The scoped
  six-patch full build/cold boot and exact partial-growth fault pass; locking,
  concurrency, remaining faults and allocation reliability remain open.
  [Complete close-history qualification](../../../evidence/runtime-20261012/g7-close-history-live/README.md)
  adds ordered diagnostic/summary/counter validation, root46 cases and identified
  same-candidate cold/live restart/zero inventory. Initial16GiB kernelENOMEM is
  retained; reboot restored compatibility but allocation reliability and full
  durable-write/resource/ownership/fault acceptance remain open.
  [Storage client-evidence qualification](../../../evidence/runtime-20261012/g7-recovery-live/README.md)
  now proves canonical refusal/ambiguity handling in root17+4 cases and identified
  fresh-release artifact/bootstrap/both-client restart/cleanup compatibility.
  A missing-guest-input package failure and health repair are retained. This
  remains a scoped pass; durable-write/resource crash and full ownership/fault
  acceptance below are open.
  Owner/implementation: Codex. Journal parsing now binds completions to their
  original operation identity and rejects conflicting embedded identities.
  Recovery preflights snapshot and incomplete-intent ownership before storage
  or backend mutation. Ten journal corruption cases and four recovery ownership
  cases pass; rejected recovery preserves snapshots and makes no backend calls.
  Full runtime race/vet/build checks pass; see
  [scoped proof](../../../evidence/runtime-20261012/g7-recovery-identity/README.md).
  Live qualification and the complete recovery matrix remain open.
  Review state, lifecycle, storage/rootfs/network journals, shim recovery, and
  generation reuse. Inject crashes before/after durable writes and resource
  mutations, concurrent create/delete, replay, corrupt records, and changed live
  ownership. Acceptance: stale state never authorizes another generation's
  teardown; ambiguity is reported; recovery is bounded and repeatable;
  resources reconcile or remain explicitly accounted for.

- [ ] **SEC-07 — Verify storage and network mediation (TM-06).**
  Owner/implementation: Codex. The endpoint now installs a primary `INPUT`
  deny rule before link activation; forwarded policy alone did not protect
  primary-local addresses. IPv6 ingress to primary/forwarded networks is denied
  explicitly for the IPv4-only data plane. CHECK requires these guards and
  teardown retains them when link removal fails. Portable command/rollback
  regressions and disposable-stock-host packet positive/negative controls pass;
  CHECK rejects the missing guard and links/rules/cloud resources are reclaimed.
  See the [scoped evidence](../../../evidence/runtime-20261012/g7-network-primary-guard/README.md).
  Live Multikernel qualification, IPv6 packet probes, and historical isolation
  harness adaptation remain open; this does not close SEC-07.
  Extend existing cross-sandbox storage and network-isolation suites with wrong
  export identity, range overflow, read/write-policy violations, descriptor
  reuse, reconnect, malformed packet, and forged source-address fixtures.
  Inspect primary/metadata-service reachability and required policy separately
  from sibling reachability. Acceptance: only configured paths are reachable,
  no primary device/controller is assigned, and host/sibling canaries and
  firewall/export ownership remain intact through restart and cleanup.

- [ ] **SEC-08 — Verify audit records and secret handling (TM-07).**
  Owner/implementation: Codex. [Primary credential-file hardening](../../../evidence/runtime-20261012/g7-primary-credential-file/README.md)
  replaces the lab client's plaintext argument with a protected token file,
  disables client dumpability and removes token values from harness helper
  arguments. The protected identity reader rejects FIFOs without blocking.
  [Private bootstrap archive publication](../../../evidence/runtime-20261012/g7-private-bootstrap-archive/README.md)
  fixes world-readable token-bearing archives and output-target overwrites
  in both fixture-capable builders, with old controls, real UID65534 read
  attempts and no-replace/producer-failure checks. Exact source proof is retained.
  [Create-operation identity](../../../evidence/runtime-20261012/g7-create-operation-identity/README.md)
  removes the raw twelve-hex-digit token prefix from the daemon request/journal
  key. Exact old Create code fails the neutral regression; corrected actual
  Create/cancel, stable/domain-separated vectors and full race/vet pass.
  Production activation of the earlier staged candidate is withheld until
  this follow-up is included. The remaining trace and audit criteria stay open.
  Exact process/wire and unsafe-file proof is retained; full bootstrap/state/
  error/diagnostic tracing, bounded audit records and whole SEC-08 remain open.
  Scoped implementation checkoff (whole task remains open):

  - [x] Remove primary client/redaction-helper token values from command lines, with old/new process and MAC proof.
  - [x] Secure token-bearing fixture archives and preserve existing output targets, with root/unauthorized-reader proof.
  - [x] Stop copying credential bytes into create-operation metadata, retaining stable retry/cancel identity and legacy cancellation behavior.
  - [ ] Finish production bootstrap/state/error/stdout/evidence/diagnostic secret tracing.
  - [ ] Qualify complete audit identity/outcome records and bounded retention/pressure behavior.

  Trace tokens through bootstrap, command lines, state, errors, stdout/stderr,
  evidence collectors, and diagnostic archives. Use synthetic distinct secrets
  so redaction can be asserted without publishing real credentials.
  Acceptance: unauthorized readers cannot obtain secrets; records identify
  component/operation/sandbox/generation/outcome; bounded logs preserve useful
  evidence under pressure. Coordinate diagnostic implementation with G8.

- [ ] **SEC-09 — Execute the G7 failure and soak matrix (TM-08).**
  [Native peer-log/resource bounds](../../../evidence/runtime-20261012/g7-storage-resource-bounds/README.md)
  retain256 refused hellos then healthy-client I/O with sub1KiB logs/canonical
  terminal evidence in a loopback real-server fixture, production musl/root
  ceiling enforcement and actual VSOCK helper profiles/adoption/zero inventory.
  Other components, negative VSOCK pressure, fairness and full failure/soak
  criteria remain open.
  Owner/implementation: Codex. Primary daemon and mknetd request reads and
  response writes now have independent bounded deadlines, preventing stalled
  peers from retaining handler slots indefinitely. Focused race regressions
  cover incomplete frames, unread replies, and operation execution beyond the
  frame budget. The sequential child agent now bounds initial authentication,
  active request transfers and complete replies while preserving authenticated
  idle sessions and operation execution. Agent/full runtime race suites and vet
  pass; see [agent evidence](../../../evidence/runtime-20261012/g7-agent-frame-deadlines/README.md).
  This is a hardening slice; the full matrix remains open.
  The [identified ctr/Docker compatibility matrix](../../../evidence/runtime-20261012/g7-live-compatibility/README.md)
  passed with daemon restart continuity and zero final runtime inventory.
  This provides compatibility evidence; the failure and soak scope below remains open.
  Test shim, daemon, agent, child kernel, storage/network server and connection,
  containerd, primary userspace restart, and GCE stop/start/reset, idle and under
  I/O. Add parallel churn, exhaustion, slow readers, log pressure, clock changes,
  and bounded sustained load. Acceptance: record detection time, workload/data
  effect, reconciliation, and final inventory for every case; no silent leaks
  or cross-generation release; primary management stays within specified
  availability bounds. Existing G6 restart tests are inputs, not this full pass.

- [ ] **SEC-10 — Establish least-privilege requirements (TM-10).**
  [Native request syscall/descriptor restriction](../../../evidence/runtime-20261012/g7-storage-syscall-filter/README.md)
  now closes inherited extras and denies new path/socket/process/host-control
  operations before readiness; root musl refusal/I/O and live actual seccomp/FD
  profiles pass with both-client idle/restart adoption and zero final inventory.
  Retained descriptor/UID0/log/image/peer/metadata rights and broad daemon
  authority remain; this does not close complete component/security acceptance.
  [Native storage request-stage capability removal](../../../evidence/runtime-20261012/g7-storage-zero-cap/README.md)
  now proves zero permitted/effective/inheritable/ambient masks before accept,
  root exec without regain, held-image operations and actual helper adoption
  with both-client storage/network/idle/restart/zero inventory. UID0/root IPC,
  full component/security acceptance remain open. [Cold-start proof](../../../evidence/runtime-20261012/g7-cold-start/README.md)
  qualifies one reboot, ordered transport registration and both-client restart/zero inventory.
  [Runtime privilege contract](../contracts/primary-runtime-privileges.md) now
  bounds the main service to eight capabilities and separates module setup into
  an ordered one-capability oneshot. [Warm-host qualification](../../../evidence/runtime-20261012/g7-runtime-capabilities/README.md)
  passes all nineteen shared feature rows, inherited storage-helper masks,
  actual identities and zero/no-pool inventory. Additional helper separation,
  complete privilege inventory and independent acceptance remain open.
  [Broker privilege contract](../contracts/primary-network-privileges.md) now
  bounds the managed broker to four capabilities, empty ambient capabilities
  and NoNewPrivileges. [Live proof](../../../evidence/runtime-20261012/g7-mknetd-capabilities/README.md)
  retains exact masks, both-client network/storage/idle/live broker restart and
  independent zero inventory/no pool. Broad SYS_ADMIN/SYS_PTRACE/root ownership,
  main runtime/helper inventory/reductions and whole SEC acceptance remain open.
  Scoped hardening: both service templates set NoNewPrivileges=yes; live process
  flags, both-client network/storage compatibility, restart and zero final
  runtime inventory pass in the [reviewed live bundle](../../../evidence/runtime-20261012/g7-reviewed-live/README.md).
  Full root capabilities remain; complete inventory and authority reductions
  remain open. The native rejection reply also no longer discloses the export
  identity; its exact overlay and regression proof are retained in that bundle.
  Inventory UIDs, capabilities, sysfs/device/filesystem access, subprocesses,
  descriptors, and network access for every daemon and helper. Review
  [systemd units](../../../deploy/systemd/) and actual deployed confinement.
  Acceptance: every retained privilege has a concrete need; propose reductions
  and test compatible hardening with restart/recovery on a qualified host.
  G7 records the boundaries; full operational service hardening remains G8.

## Controlled comparison and architectural decision

- [x] **SEC-11 — Evaluate post-compromise containment (TM-09/TM-12).**
  Owner/source reviewer: Codex. Outcome: pinned source establishes the missing
  barrier; no live primary-memory exploit is required or claimed. Continue the
  existing trusted-child scope; any expanded isolation claim needs a new design.
  First review SEC-01; if the source audit establishes a missing barrier, record
  it without requiring an exploit demonstration. If a live confirmation is
  useful, design a bounded approved child-kernel fixture that accesses only
  explicitly reserved synthetic primary/sibling memory canaries. Preserve
  management access and abort on unexpected access or host instability. Do not
  scan arbitrary host memory or publish real data. Distinguish a fixture that
  assumes kernel compromise from a demonstration that a workload can achieve it.
  Acceptance: retain observed reads/writes and exact assumptions; confirmed
  unauthorized access refutes that containment claim and triggers an architecture
  decision about hardware-backed isolation or the continued trusted-child scope.

- [ ] **SEC-12 — Review dependencies and artifact trust (TM-11).**
  [Patched runtime deployment](../../../evidence/runtime-20261012/g7-sec12-deployment-live/README.md)
  activates clean revision0c80131 built with Go1.26.9, with actual running
  daemon/broker hashes and retained rollback. All nineteen ctr/Docker rows,
  independent zero-resource audits and four live pre-allocation refusals pass.
  Failed harness history and exact seven-binary scans are retained there.
  Three CRI advisory IDs remain; scanner package/symbol matches are explicit.
  Google-agent vendor primitives remain adverse; full dependency findings,
  licensing/provenance and host/child ABI assessment remain open.
  [Bootstrap component parity and literal trailer](../../../evidence/runtime-20261012/g7-bootstrap-component-parity/README.md)
  closes CLI base-archive component admission with held bounded reads, matching
  daemon policy. Both parsers refuse a normalized trailer alias. Portable controls
  reproduce old component/trailer acceptance; corrected actual-profile root
  qualification passes with strict parent/modinfo policy. An initial private
  fixture under unsafe `/var/tmp` was refused and retained, then moved under a
  protected test configuration directory without weakening policy. Complete
  dependency/provenance/ABI and deployment qualification remain open.
  [Base/generated archive component and selected-profile binding](../../../evidence/runtime-20261012/g7-initramfs-component-binding/README.md)
  now enforces current agent/relay/transport consistency in base admission,
  current transport/NBD in generated boot and exact manifest identity for the
  prepared root. Create rejects before allocation/state/journal mutation;
  Load/recovery verify before storage provisioning/backend load. Actual installed
  base/root synthetic lifecycle checks and pre/post nineteen-zero inventories
  pass. Full race/vet/seven builds pass; old resolver accepts all three wrong
  embedded components. Dependency/provenance/ABI and production compatibility
  qualification remain open; whole SEC-12 is not checked off.
  [Bootstrap CLI release parity and bounded ELF parsing](../../../evidence/runtime-20261012/g7-bootstrap-kernel-binding/README.md)
  reject the actual wrong-release kernel through both admission paths and
  refuse malformed unused symbol names; root/source/control/build proof retained.
  The base archive has matching agent/relay/transport bytes; NBD belongs to
  the separately built mediated bootstrap. Newer proof above implements
  base/derived semantic checks and profile binding. The newer deployment proof
  above activates these guards; this earlier proof describes its own baseline.
  [Daemon embedded-kernel release guard](../../../evidence/runtime-20261012/g7-kernel-embedded-release/README.md)
  now rejects a hash-approved real rb13 kernel under the rb11 profile; old
  resolver accepts it. Exact root/source/old-control/build proof is retained.
  Initramfs semantic binding and host/child ABI remain.
  [Google agent review](../../../evidence/runtime-20261012/g7-google-agent-dependency-review/README.md)
  retains exact binaries/source/vendors and adverse scans. Refreshed official
  indexes offer no newer package candidates; both High rules remain open for
  backport/reachability review and supported update or qualified rebuild.
  [OpenSSL remediation and activation proof](../../../evidence/runtime-20261012/g7-openssl-security-remediation/README.md)
  upgrades four packages to3.5.5-1ubuntu3.7 and verifies the exact CVE-2026-84782
  rule changes true→false after reboot, unchanged runtime/default hashes and
  zero final idle counts. Other material findings and full acceptance remain.
  [Official Ubuntu/Python dependency inventory](../../../evidence/runtime-20261012/g7-dependency-oval-inventory/README.md)
  records553 matched vulnerability definitions, including three High-priority
  rules, and15 matched Python package/version pairs. Many rules indicate
  possible affected packages; backport/fixed-version and reachability review
  remains required. The custom kernel is not covered by distro package checks.
  [Module identity hardening and root proof](../../../evidence/runtime-20261012/g7-module-artifact-identity/README.md)
  now refuse wrong transport/support releases despite approved hashes, before
  backend/pool/state/journal mutation. Actual signed modules and installed
  profile pass; old CLI accepts the wrong support release and new CLI refuses.
  Kernel/initramfs byte mismatches also fail the lifecycle pre-allocation
  fixture. Initramfs semantic identity, full ABI/provenance, remaining
  dependency review and production deployment acceptance remain open.
  Owner/executor: Codex. [Go dependency remediation](../../../evidence/runtime-20261012/g7-go-dependency-remediation/README.md)
  requiresGo1.26.9 and patched containerd1.7.36/gRPC1.83.2/xsys0.47.0.
  Full race/vet/seven builds pass; exact before/after scans retain57→3 advisory
  IDs, with remaining CRI entries triaged against absent compiled imports and
  actual Ubuntu security backports. The newer deployment proof above
  activates these changes; this earlier bundle retains the local-build baseline.
  Python/C/kernel/deployment inventory triage, provenance/license gaps and
  complete artifact trust acceptance beyond the bounded wrong-artifact
  refusals remain open.
  Inventory Go, Python, C, kernel/module, Kerf, CNI, and deployment dependencies;
  record versions, licenses, known vulnerabilities, and reachable-code triage.
  Verify hashes and ownership at install and boot, including a mismatched
  kernel/module/initramfs fixture. Acceptance: unresolved material vulnerabilities
  and provenance gaps are explicit; wrong artifacts fail before allocation.
  Feed inventory/provenance automation into G9 rather than declaring G9 complete.

- [ ] **SEC-13 — Produce the pinned gVisor comparison report.**
  Pin gVisor release, platform, host kernel, privileges, filesystem/network modes,
  and resource policy. Match workload/root privileges and permitted external
  access; document unavoidable differences. Compare reachable interfaces,
  memory-safety exposure, independent containment after kernel/Sentry compromise,
  helper compromise, sibling access, and availability. Use equivalent synthetic
  assets and matched adversary starting conditions; do not compare a compromised
  child kernel only against an uncompromised Sentry. Report compatibility and
  performance separately from security. Acceptance: every claim has source or
  reproducible evidence and explicit scope; publish adverse results and avoid
  a general superiority claim from finite negative tests or exploit counts.

## Completion review

- [ ] Assign reviewers, map every TM row to findings/tests/evidence, and record
  residual risks with explicit rationale and acceptance ownership.
- [ ] Publish the trusted-single-tenant security statement, including the
  consequences of child-kernel compromise and the comparison's limitations.
- [ ] Review the full G7 exit criteria independently. Keep G7 unchecked until
  fuzzing, failure-matrix, privilege-boundary, and resource-leak requirements
  pass; a completed threat-model document alone does not close the gate.
