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

## Repository and protocol review

- [ ] **SEC-04 — Review privileged input conversion (TM-02).**
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
  Review state, lifecycle, storage/rootfs/network journals, shim recovery, and
  generation reuse. Inject crashes before/after durable writes and resource
  mutations, concurrent create/delete, replay, corrupt records, and changed live
  ownership. Acceptance: stale state never authorizes another generation's
  teardown; ambiguity is reported; recovery is bounded and repeatable;
  resources reconcile or remain explicitly accounted for.

- [ ] **SEC-07 — Verify storage and network mediation (TM-06).**
  Extend existing cross-sandbox storage and network-isolation suites with wrong
  export identity, range overflow, read/write-policy violations, descriptor
  reuse, reconnect, malformed packet, and forged source-address fixtures.
  Inspect primary/metadata-service reachability and required policy separately
  from sibling reachability. Acceptance: only configured paths are reachable,
  no primary device/controller is assigned, and host/sibling canaries and
  firewall/export ownership remain intact through restart and cleanup.

- [ ] **SEC-08 — Verify audit records and secret handling (TM-07).**
  Trace tokens through bootstrap, command lines, state, errors, stdout/stderr,
  evidence collectors, and diagnostic archives. Use synthetic distinct secrets
  so redaction can be asserted without publishing real credentials.
  Acceptance: unauthorized readers cannot obtain secrets; records identify
  component/operation/sandbox/generation/outcome; bounded logs preserve useful
  evidence under pressure. Coordinate diagnostic implementation with G8.

- [ ] **SEC-09 — Execute the G7 failure and soak matrix (TM-08).**
  Test shim, daemon, agent, child kernel, storage/network server and connection,
  containerd, primary userspace restart, and GCE stop/start/reset, idle and under
  I/O. Add parallel churn, exhaustion, slow readers, log pressure, clock changes,
  and bounded sustained load. Acceptance: record detection time, workload/data
  effect, reconciliation, and final inventory for every case; no silent leaks
  or cross-generation release; primary management stays within specified
  availability bounds. Existing G6 restart tests are inputs, not this full pass.

- [ ] **SEC-10 — Establish least-privilege requirements (TM-10).**
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
