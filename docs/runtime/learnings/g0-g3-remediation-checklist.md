# G0-G3 remediation and revalidation checklist

Audit date: 2026-08-31

This is the handoff checklist for the next implementation session. The original
G0-G3 run remains useful evidence for narrow milestones, but the gates must not
be treated as fully closed until the unchecked items below are either completed
or the normative plan/contract is explicitly revised with a rationale.

The audit compared the original gate plans, frozen v1 contracts, implementation,
unit tests, live scripts, retained GCE evidence, and the earlier DAXFS/direct-
ext4/mediated-ext4 learnings. It found no reversal of the earlier empirical
storage, Kerf, transport, or isolation findings. The open work is primarily
contract enforcement, untested behavior, over-broad pass wording, and incomplete
evidence.

Follow-up audit: 2026-08-31, against repository commit `e99a6bf`. All four
learning documents were compared again with the current implementation, tests,
and retained evidence. The complete local verification command was rerun and
passed. This review split mixed items so their completed portions can be closed,
corrected two over-broad unit-test classifications in the learnings, and added
explicit work for secret-safe errors and the plan-required G3 containment
features.

Synchronization audit: 2026-09-02. The remediation findings were folded back
into learnings `00` through `03` and compared with the later G4-G6
implementation. Later work added exec, stdin/attach, PTYs, resize, process-group
signals, incremental output reads, and a real running-process shutdown check.
Those additions do not close G3: focused failure coverage, bounded retention,
containment controls, reconnect, structured errors, and agent-driven
quiescence/poweroff remain open. The audit also found contract drift: the agent
method schema and kernel/image policy lag the new methods, while the G6 OCI
adapter can discard unsupported fields instead of failing closed.

## Rules for closing an item

- [x] Do not close a code item without a focused automated test. (Applied in
  this session; schema and decoder closures have fixtures/tests.)
- [x] Do not close a live item without indexed raw evidence and an assertion in
  that gate's manifest.
- [x] Do not weaken a frozen requirement merely to preserve a historical pass;
  if scope changes, revise the plan and contract together and record why.
- [x] Keep the earlier safety invariants: never assign a shared boot-disk/NIC
  controller, never share a writable DAXFS root, give writable storage exactly
  one sandbox owner, observe exact Kerf state before retrying, and prohibit
  direct Go AF_VSOCK on the pinned transport.

## P0: reconcile what “passed” means

- [x] Mark the existing G0-G3 results as `partial`/`provisional`, or amend the
  normative gate definitions so that they precisely describe the narrower
  milestones that were actually demonstrated.
- [x] Resolve the internal G3 scope conflict: the plan's “minimum
  implementation” requires namespaces, capabilities, rlimits, cgroups,
  `ExecProcess`, and clean quiescence, while the frozen G3 policy permits many
  of those features to fail closed.
- [x] Change the G3 learning's “tested subset” wording to distinguish live
  tested, unit tested, implemented but untested, rejected, and not implemented.
- [x] Correct learning claims that exceeded the focused tests: G1's current
  suite does not directly test duplicate/offline APIC rejection, and G3's
  authentication test does not exercise wrong identity fields.
- [ ] Reissue the final G0-G3 summary only after every retained pass claim maps
  to an explicit test and evidence assertion.

## G0: contracts and schemas

### Missing or under-specified artifacts

- [x] Add the missing strict host-configuration schema, including state/socket
  paths, Kerf/sysfs/manifest paths, primary CPU and memory reservations,
  forbidden APIC IDs, timeout, and maximum frame size.
- [x] Add machine-readable daemon and agent protocol schemas, or explicitly
  revise the G0 requirement that calls for a versioned protocol schema.
- [x] Extend the kernel-manifest schema with the promised Multikernel and Kerf
  compatibility/version pins.
- [x] Tighten the evidence schema's nested repository, component, host, command,
  assertion, limitation, and cleanup objects; require command exit status and
  output path where applicable.
- [x] Make the evidence schema usable by later gates rather than limiting the
  `gate` enum to G0-G3.
- [x] Add the documented label-key bounds/pattern to the sandbox schema.
- [x] Define whether duplicate JSON object names are invalid at schema parsing
  time, wire parsing time, or both, and provide one shared strict decoder.
- [ ] Reconcile post-G3 protocol evolution with frozen v1. The current agent
  implements state, stdin/output, terminal resize, and network methods absent
  from the agent schema, and the kernel/image policy still describes terminal
  I/O as rejected. Revise/version schema, policy, fixtures, and implementation
  together rather than documenting the mismatch as conformance.
- [ ] Restore end-to-end fail-closed OCI handling or explicitly revise the
  contract. The G6 initramfs builder and exec translation currently discard
  unsupported caller fields before the agent can reject them.

### Contract validation

- [x] Add schema fixtures covering valid documents, unknown fields, duplicate
  fields, overflow, unsafe/relative paths, malformed digests, missing version
  pins, and invalid labels.
- [x] Run schema validation from the repository verification target; the current
  documentation checker only verifies file presence and local links.
- [x] Reverify that the pinned Multikernel, Kerf, and other source revisions are
  obtainable, and record the check without silently moving any pin.
- [x] Verify that every normative contract statement has either an implementing
  gate or an explicit future-gate annotation.
- [x] Retain the license conclusion, but schedule the already-deferred generated
  dependency/SBOM audit for release rather than presenting it as completed.

## G1: host qualification and isolation

### Qualification implementation

- [ ] Report and assess contiguous-allocation readiness rather than only total
  primary memory.
- [ ] Report existing pool state, assigned devices, unknown/stale resources, and
  `/proc/kimage` state, not only instance directory names.
- [ ] Resolve boot-disk and NIC sysfs ancestry to their allocatable PCI function
  and emit explicit forbidden/shared-controller findings.
- [ ] Reject a requested boot-disk/NIC/shared controller before mutation and
  connect that policy to `mkruntimed`, not only to a reporting command.
- [ ] Fail closed when the Kerf version, Secure Boot state, lockdown state,
  kexec readiness, or required recovery signal is unknown or incompatible.
- [ ] Check serial-console/recovery readiness explicitly.
- [ ] Count online CPUs for primary headroom; reject duplicate logical/APIC
  mappings and report offline CPUs accurately.
- [ ] Define and enforce the SMT-sibling allocation policy.
- [ ] Preserve zero-valued physical/core/NUMA topology IDs in JSON rather than
  losing them through `omitempty`, and test NUMA mapping where available.
- [ ] Validate requested APIC IDs against the live report in the actual daemon
  allocation path, including online status and retained primary headroom.

### Isolation investigation and tests

- [ ] Complete and retain the pinned-kernel source audit for memory mapping,
  interrupts, DMA, MSRs, I/O ports, and denial-of-service behavior.
- [ ] Publish a per-resource matrix classifying isolation as hardware-enforced,
  software-coordinated, accidental, unverified, or prohibited.
- [ ] Add fixtures for missing/wrong Kerf, unknown Secure Boot/lockdown, offline
  CPUs, duplicate APIC IDs, SMT splits, existing pool, assigned device, stale
  resource, shared boot controller, and shared NIC controller.
- [ ] Retain live proof of the child's assigned CPU view and approximate memory
  view.
- [ ] Retain the full G1 evidence set required by the plan: report, Kerf state,
  device tree, APIC map, primary and child logs, boot ID, GCE configuration, and
  final resource return.

## G2: recoverable control plane

### Recovery correctness

- [ ] Reconcile incomplete journal intents even when a crash occurred before the
  sandbox was committed to `state.json`. Current reconciliation iterates only
  snapshotted sandboxes and can miss an externally created instance in this
  window.
- [ ] Add crash injection at every operation boundary: before/after intent
  fsync, external mutation, observation, snapshot commit/rename, and completion
  record for create, load, start, stop, and delete.
- [ ] Detect unknown backend instances/pools that are absent from durable state
  and return `OPERATOR_ACTION`; do not limit reconciliation to known IDs.
- [ ] Define a canonical mapping between Kerf's `created`/`loaded`/`active`
  statuses and the richer runtime states. In particular, prove that a daemon
  restart after a clean stop does not misclassify runtime `STOPPED` versus Kerf
  `loaded` as unexplained disagreement.
- [ ] Decide and implement resume-versus-operator-action behavior for each
  incomplete transition rather than marking all disagreement generically.
- [ ] Check and propagate failures while recording error completion and error
  state; several store errors are currently ignored on the failure path.
- [ ] Fsync the state directory after atomic snapshot rename if durability across
  host failure is claimed.
- [ ] Implement known tombstones, or revise the lifecycle contract that says a
  known absent delete can succeed.

### API and contract enforcement

- [ ] Implement `WatchEvents` and its versioned event semantics.
- [ ] Load a strict root-owned host configuration rather than relying only on
  command-line flags; validate owner, mode, and safe parent directories.
- [ ] Resolve the requested approved kernel manifest and verify artifact type,
  ownership, architecture, release, hashes, required config, modules, protocol,
  and OCI features before allocation. Remove the current fixed arbitrary
  `--kernel`/`--initrd` bypass from the production path.
- [ ] Validate bundle path, manifest name, label keys/values, idempotency-key
  length/printability, request IDs, live CPU eligibility/headroom, and memory
  availability before mutation.
- [ ] Require every sandbox CPU to be a member of the configured Kerf pool and
  account for the pool's usable memory/slack before invoking Kerf.
- [x] Reject duplicate JSON fields in daemon envelopes and method bodies through
  the shared strict decoder; its top-level and nested duplicate behavior has a
  focused automated test.
- [ ] Add focused daemon wire tests for unknown fields, trailing values, frame
  bounds, and numeric overflow; these paths are not covered merely by the shared
  decoder test.
- [ ] Return `BACKEND_TIMEOUT` for actual timeouts rather than mapping every
  backend error to `BACKEND_FAILURE`.
- [ ] Populate stable operation IDs on mutation errors as required by the error
  contract.
- [ ] Sanitize daemon and agent errors and add focused tests proving that
  backend output, host paths, tokens, and other secrets cannot cross either API.
- [ ] Make intermediate `STOPPING` and `RELEASING` state semantics observable,
  or revise the documented state machine.
- [ ] Verify socket owner/group, safe socket parent, peer authorization, journal
  permissions, artifact permissions, and configurable maximum frame size.

### Missing tests and live evidence

- [ ] Expand fake-Kerf coverage to every nonzero exit, timeout, malformed or
  truncated observation, and committed-then-failed operation—not only create.
- [x] Test exact duplicate `CreateSandbox` replay with the same idempotency key.
- [ ] Expand deterministic duplicate coverage to start, stop, and delete, and
  test terminal-state retries with new valid idempotency keys.
- [ ] Test two disjoint live sandboxes through the daemon.
- [ ] Test client/shim disappearance while a child continues running.
- [ ] Test child/backend failure during load, boot/start, and stop.
- [ ] Re-run daemon recovery with state in a durable non-`/tmp` location and
  retain the journal/snapshot. If host-reset durability is claimed, test a host
  reset as a separate case from daemon `SIGKILL`.

## G3: agent and OCI process lifecycle

### Process semantics

- [ ] Add focused `ExecProcess` lifecycle, invalid-spec, duplicate, failure,
  cleanup, concurrency, and signal/wait tests. Exec was implemented and
  exercised through both G6 clients after the historical G3 run, but it does
  not yet satisfy the G3 automated-test matrix.
- [ ] Replace wait-time in-memory stdout/stderr accumulation with bounded,
  independent streaming and backpressure. The current implementation does not
  satisfy the plan's streaming requirement and can create oversized replies.
- [ ] Define signal scope and signal the container process group when required;
  test delivery rather than only advertising `signals`.
- [ ] Test non-root UID/GID and non-empty supplementary groups in the live child.
- [ ] Test exact argv without shell interpretation, environment, non-default
  cwd, process state transitions, and delete/wait races.
- [ ] Test PID 1 exit 0, nonzero exit, crash, ignored `SIGTERM`, forced kill, and
  complete descendant cleanup.
- [x] Make `WaitProcess` reject an unstarted process instead of waiting forever.
- [ ] Bound process count, output, and retained stopped-process state.

### OCI and capability truthfulness

- [x] Maintain the required feature matrix with separate `live-tested`,
  `unit-tested`, `implemented-unproven`, `rejected`, and `not-tested` states.
- [ ] Implement and test the plan-required namespaces, capability application,
  rlimits, and cgroups, or revise the plan and frozen contract together with a
  recorded rationale. Fail-closed rejection in the provisional OCI subset does
  not satisfy this G3 exit criterion.
- [ ] Report actual agent and child-kernel capabilities; the current
  `Capabilities` response reports only protocol and OCI feature names.
- [x] Do not advertise UID/GID, supplementary groups, or signals as tested until
  their nontrivial live cases pass.
- [ ] Validate supported OCI version and executable/image architecture before
  process start.
- [x] Test fail-closed rejection of unsupported mounts.
- [ ] Add fail-closed tests for every other declared unsupported field: hooks,
  capabilities, namespaces, resources/cgroups, seccomp, masked/read-only paths,
  read-only root, `noNewPrivileges`, rlimits, and hostname. Terminal mode is now
  implemented; retain its positive and invalid-size tests separately. Also test
  rejection before and after the G6 adapter so unsupported fields cannot be
  silently stripped.
- [ ] Decide whether annotations and empty-but-present unsupported objects are
  accepted, ignored, or rejected, then test the chosen semantics.
- [ ] Securely resolve the bundle/root path without caller-controlled symlink or
  chroot escape assumptions before containerd integration.

### Agent protocol and shutdown

- [ ] Add focused tests for the implemented `Shutdown` behavior: it rejects
  while a managed process remains live and reports `quiesced` only after the
  manager is no longer running a process. Define and test lifecycle/race and
  deterministic stop/kill behavior rather than relying on code inspection.
- [ ] Ensure shutdown explicitly ends the agent session and powers off the child
  without depending on the controller closing the connection as an implicit
  command.
- [ ] Integrate the earlier mediated-ext4 clean sequence when storage lands:
  remount read-only, flush, disconnect NBD, sync server, then power off.
- [ ] Generate a fresh random 256-bit token and random generation for every live
  run; the retained G3 script uses fixed fixtures.
- [x] Reject duplicate fields and trailing JSON values in agent envelopes and
  method bodies.
- [ ] On an oversized or truncated frame, close or fully drain the connection so
  the stream cannot become desynchronized; add malformed-frame tests.
- [ ] Define whether replies require authentication/integrity protection and
  update the protocol contract and implementation consistently.
- [ ] Return the frozen structured error codes from the agent API rather than
  unclassified error strings.
- [ ] Test wrong sandbox ID, stale generation, wrong endpoint, wrong protocol,
  invalid MAC, replay, out-of-order/concurrent sequences, oversized frames, and
  malformed messages in the live transport path where safe.
- [ ] Implement and test the declared disconnect/reconnect policy.
- [ ] Keep the patched C relay as the only allowed pinned-transport boundary and
  add a qualification check that prevents use when the module/hash/direction is
  incompatible.

## Evidence repair and final revalidation

- [ ] Create a separate schema-valid manifest for each gate rather than one
  combined manifest labelled only `G3`.
- [ ] Add `resources-before.json` and `resources-after.json` for every cloud run,
  including retained resources and operator acknowledgment.
- [ ] Record exact command lines or safe command identifiers, exit statuses,
  output paths, repository commit/dirty state, component hashes, assertions,
  limitations, and cleanup in each manifest.
- [ ] Retain raw G2 journal/snapshot and meaningful command output; the current
  71-byte pass-marker log is insufficient for the evidence contract.
- [x] Retain raw evidence for the failed direct-Go AF_VSOCK attempts and two
  primary resets, or downgrade those statements to unretained observations.
- [ ] Ensure redaction is performed during capture and validate that manifests,
  logs, console output, process listings, and kernel command-line evidence do
  not expose tokens or credentials.
- [x] Run the complete local suite, schema tests, race tests, shell syntax checks,
  documentation link checks, and static analysis from one recorded command.
- [ ] Run the corrected G1, G2, and G3 live matrix on a disposable qualified GCE
  host and prove final CPU, memory, pool, instance, disk, address, and guest-agent
  state.
- [x] Update `00` through `03` learnings with the currently demonstrated
  boundary, remediation findings, later implementation changes, and retained
  evidence limitations. This documentation synchronization does not close the
  audit or any gate; replacement evidence and the unchecked implementation/test
  work above remain required.

## Known-consistent findings to preserve

These are not remediation items. They are constraints established by the
earlier experiments and confirmed, not contradicted, by G0-G3:

- Kerf v0.2.0 may commit create and then exit nonzero; accept this only after
  observing the exact requested instance.
- GCE boot and workload disks may share one allocatable controller; physical
  controller handoff is prohibited on the tested N2 and C3 topologies.
- Writable DAXFS is not coherent across sibling kernels; use read-only sharing
  or exactly one writer/owner.
- DAXFS allocation-lifetime retention is not host-reboot or host-failure
  durability.
- The pinned Multikernel AF_VSOCK module requires the recorded patch, direct Go
  endpoints are prohibited, direction matters, and application messages require
  explicit bounded framing rather than EOF/half-close delimiters.
- Multikernel provides a trusted-workload resource boundary, not demonstrated
  hostile-kernel KVM/EPT-grade isolation.
