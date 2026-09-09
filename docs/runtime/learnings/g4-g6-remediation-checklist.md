# G4-G6 remediation and live-evidence checklist

Audit date: 2026-09-02; implementation status refreshed 2026-09-06

This is the implementation and evidence handoff for gates G4, G5, and G6. The
existing GCE runs demonstrate a useful executable MVP, but they do not close
the normative gates. A checked MVP item is not a full-gate pass, and a feature
must not be described as live-proved unless retained output from the instance
shows the assertion or a retained, hashed test harness makes the assertion and
the transcript records its successful exit.

The audit compared the G4-G6 plans, current runtime and guest implementation,
unit tests, live harnesses, evidence manifests, and every retained G4-G6 file.
The current local documentation checks and Go race suite pass. The work below
is what remains after those local passes.

## Current verdict

| Gate | Demonstrated boundary | Why the gate remains open |
| --- | --- | --- |
| G4 | The current tree builds and verifies canonical manifests and deterministic newc roots, rejects observed source mutation and unsafe metadata, produces bounded fully allocated private ext4 images, generation-binds one mediated export, and journals graceful teardown/recovery. Earlier live runs only prove the narrower BusyBox/private-write MVP. | Configured persistence and read-only bind inputs remain incomplete. Exhaustion, corruption, server-loss, host-reset, clone, cross-export, and replacement-instance evidence matrices have not passed on the current revision. |
| G5 | The current tree contains `mknetd`, CNI 1.0 `ADD`/`CHECK`/idempotent `DEL`, generation-bound endpoint state, negotiated MTU/DNS, bounded exchange/counters, restart reconciliation, and exact-address anti-spoof/firewall policy. Earlier live runs only prove static-link networking. | The CNI implementation and complete firewall CHECK have automated coverage but no current-revision live proof. Traffic, MTU/load/fault, restart, spoof/bypass, primary-health, and cleanup evidence matrices remain open. |
| G6 | The current tree implements the core Task v2 lifecycle, faithful versioned guest PIDs, pause/resume/stats, standard OCI process controls, durable task/process/I/O offsets, a supervised shim worker, and generation-bound task reconstruction. Earlier live runs prove only the narrower lifecycle/I/O MVP. | Current-revision forced-shim reconstruction remains live-unproved. Durable event replay, complete cancellation/FIFO/race matrices, Docker restart, packaging upgrade/rollback, and evidence-grade shared and isolated reruns remain incomplete. |

The canonical gate rows in [`../plans/README.md`](../plans/README.md) and
[`../../project/TASKS.md`](../../project/TASKS.md) must remain unchecked until
the corresponding unchecked work in this document is either completed or the
normative plan is revised with an explicit rationale.

## Retained instance runs and evidence quality

### `mklinux-g6-20260831`: executable MVP

- Retained files:
  [`g4-g6-proof-clean-pool.log`](../../../evidence/runtime-20260901/g4-g6-gce/g4-g6-proof-clean-pool.log),
  [`g4-g6-proof-repeat.log`](../../../evidence/runtime-20260901/g4-g6-gce/g4-g6-proof-repeat.log),
  [`g4-g6-environment.log`](../../../evidence/runtime-20260901/g4-g6-gce/g4-g6-environment.log),
  and [`manifest.json`](../../../evidence/runtime-20260901/g4-g6-gce/manifest.json).
- Useful live evidence: two distinct child boot IDs, two static addresses,
  outbound-network pass markers, bidirectional sibling isolation, exec through
  both clients, signal/exit handling, two successful clean-pool runs, installed
  component hashes, and empty final runtime/container/network inventories.
- Evidence limitations: the proof log is concise and does not retain expanded
  commands, child kernel release output, DNS answers, HTTP response details, or
  the exact final cleanup commands. It does not hash the caller snapshots
  before and after the run.

### `mklinux-gates-20260901`: robustness observations

- Retained files: only
  [`README.md`](../../../evidence/runtime-20260901/g4-g6-robustness/README.md)
  and [`manifest.json`](../../../evidence/runtime-20260901/g4-g6-robustness/manifest.json).
- Recorded observations: containerd restart continuity, `mkruntimed` restart
  continuity, injected builder ENOSPC rollback, forced-shim-death reclaim, and
  terminal rejection on the then-current build.
- Evidence limitation: there is no raw box transcript. Every command and
  assertion in the manifest points back to the narrative `README.md`. There is
  no retained boot-ID before/after output, process/service journal, Kerf or pool
  inventory, TUN/iptables inventory, injected error output with surrounding
  state, or final cleanup transcript.
- Classification: these are retained operator observations. They are useful
  leads for a replacement test but are not evidence-contract-quality live
  proof. In particular, they are insufficient by themselves to close the
  checked containerd-restart, shim-reclaim, or ENOSPC claims.

### `mklinux-g4-g6-matrix-20260901`: shared feature matrix

- Retained files are indexed in
  [`g4-g6-feature-matrix/README.md`](../../../evidence/runtime-20260901/g4-g6-feature-matrix/README.md).
- Useful live evidence: a hashed harness completed with exit status 0 and pass
  rows for image inspection, create/start/state/exec/stdio/wait, nonzero exit,
  signals, deletion, two name-reuse cycles, private roots, static networking,
  isolation, `mkruntimed` restart, and cleanup. Terminal and pause/resume were
  correctly recorded as rejected rather than passed.
- Evidence limitations: the retained transcript contains feature-row markers,
  not the expanded commands and observed values. The manifest contains a
  redacted `${HOST_BOOT_ID}` that does not satisfy the current evidence schema.
  The repository was dirty and no diff artifact was retained.

### `mklinux-g4-g6-io-20260901`: guest I/O and PTY rerun

- Retained files are indexed in
  [`g4-g6-io-live/README.md`](../../../evidence/runtime-20260902/g4-g6-io-live/README.md).
- Useful live evidence: the hashed current harness completed with exit status
  0 after asserting foreground stdin, detach/reattach, terminal allocation, and
  initial `91` by `37` size propagation through both clients. The environment
  log records installed hashes and empty final inventories; cloud cleanup is
  retained.
- Evidence limitations: the transcript retains the pass rows but not the
  asserted guest output such as `37 91`. The manifest omits the schema-required
  component `version` fields and uses `${HOST_BOOT_ID}`, which is invalid under
  the current boot-ID pattern. It labels the combined run `gate: G6` and
  `result: pass` even though the G6 gate remains provisional; the result needs
  an explicitly scoped meaning or should be `provisional`. The repository was
  dirty and no diff artifact was retained.

## Rules for closing remediation work

- [ ] A code item has focused automated tests, including failure behavior and
  cleanup where applicable.
- [ ] A live item has raw immutable instance output, a schema-valid manifest,
  an explicit assertion, and the exact retained artifact path.
- [ ] A summary marker is accompanied by the observed values needed to audit
  it; do not retain only `PASS` when boot IDs, hashes, addresses, exit codes,
  mount state, or resource inventories are the actual assertion.
- [ ] Every live run records the clean starting state, repository commit and
  dirty diff, installed component hashes, qualified host report, command exit
  status, failure diagnostics, final host state, and cloud resource cleanup.
- [ ] Expected rejection is labelled `unsupported` or `rejected`, never
  `passed`, and is followed by proof that no resources were allocated or left.
- [ ] Sanitized committed evidence remains schema-valid. Either retain a
  non-identifying valid run identity or revise the evidence schema and contract
  together to represent redacted values explicitly.
- [ ] Each manifest result is scoped to its actual run. A feature-matrix pass
  must not look like closure of the entire gate.

## G4: OCI images and storage

### Implementation still required

- [ ] Generate a deterministic image/root manifest and verify it before any
  sandbox allocation. Record the manifest digest separately from the source
  OCI image digest and generated initramfs digest.
- [ ] Make initramfs generation reproducible, not merely sorted with a
  timestamp-free gzip header. Normalize or deliberately preserve and manifest
  cpio metadata, including runtime-file mtimes, uid/gid, modes, xattrs,
  hardlinks, symlinks, sparse extents, and device policy; prove two builds from
  identical inputs have the same digest.
- [ ] Reject unsafe paths, traversal, escaping symlinks, unsupported file
  types, device nodes, inconsistent hardlinks, malformed metadata, and input
  mutation during the copy/build window.
- [ ] Validate OCI image architecture against the selected child-kernel
  manifest before allocation and record required kernel features.
- [ ] Preserve the caller snapshot as containerd-owned input. Mount it with the
  least privileges needed, handle every unmount failure, and prove the runtime
  cannot write through an absolute or relative `root.path`.
- [ ] Define and enforce single-owner writable-root identity, generation,
  duplicate-attach prevention, and stale-lock handling rather than relying
  only on one private initramfs per current shim.
- [ ] Decide the supported writable-state model. Implement private writable
  layers, read-only bind inputs, persistence/volumes, ownership mapping, and
  propagation semantics, or narrow the G4 plan explicitly if some are outside
  the intended runtime.
- [ ] Add capacity accounting, block/inode quotas, a high-water refusal policy,
  and bounded behavior for host and initramfs ENOSPC.
- [ ] Implement the storage teardown and recovery sequence appropriate to the
  selected persistent backend: quiesce processes, remount read-only, flush,
  disconnect, sync, offline-check, and preserve a diagnosable state on failure.
- [ ] Resolve partial-artifact cleanup. Failed `Create` must remove token,
  initramfs, recovery, mount, and runtime-directory state as well as avoiding a
  Kerf allocation. Root preparation now defensively unmounts even when mount
  reports failure, removes runtime/storage artifacts and its durable record at
  every confirmed-unmounted build/state boundary, propagates cleanup failures,
  and retains only a recoverable record when unmount or durable-record cleanup
  cannot be proven. The shim now resolves an ambiguous daemon
  `CreateSandbox` response through exact-key/config `CancelCreateSandbox`:
  mkruntimed durably stops forward reconciliation, tombstones delayed create
  replay, removes only a create-state storage/backend owner, and confirms that
  cleanup is safe before the shim removes prepared roots. If cancellation is
  temporarily unavailable, the strict existing token is reused so the next
  Create retries the same idempotency identity rather than creating a second
  ambiguity. The full end-to-end injected failure matrix and live no-leak
  inventory remain open. Rootfs durable state now validates its no-follow,
  single-link, caller-owned file; exact record key/request/result; derived
  bundle/runtime/storage paths; unique bundle/port ownership; and forward-only
  phases. Reconciliation rechecks configured path derivation before recursive
  cleanup, and deep-copy tests prevent callers from mutating journal fields by
  alias. Cleanup is descriptor-anchored beneath stable bundle/storage-root
  inodes; focused symlink and post-open rename tests prove a replacement tree
  is not traversed or removed.
  Privileged builder output consumption now uses bounded no-follow opens with
  caller-owner, single-link, mode, and stable-identity checks. Exact storage
  metadata is revalidated against the request, and both initial publication
  and recovery bind the declared quota to the image's size, allocated blocks,
  and digest. Service result files are created exclusively; focused tests
  reject malformed metadata, hardlinks, symlinks, sparse or wrongly sized
  images, and pre-existing publication targets. The live injected failure and
  no-leak matrix remains open. Builder execution now drains output with a
  one-MiB retention ceiling and bounded returned diagnostics; the earlier of
  caller cancellation and a ten-minute default kills its complete process
  group. Focused tests reject overflowing output and prove a background child
  holding the pipe is killed promptly at deadline.

### Automated tests still required

- [x] Manifest generation and digest reproducibility across two builds. The
  deterministic newc builder and changed-input control are exercised by
  `scripts/test-runtime-rootfs-build.py`; disposable-host evidence is still
  required by the replacement-run section below.
- [x] Whiteouts/device nodes, opaque directories, hardlinks, symlinks, sparse
  files, xattrs, modes, uid/gid, timestamps, and large trees have focused
  coverage. The suite proves equivalent sparse/dense bytes and differing
  mtimes produce identical output, extracts and checks links/modes, rejects
  malformed opacity and unsupported xattrs, and exercises FIFO, socket,
  escaping-link, external-hardlink, and device rejection. Device creation is
  permission-gated locally and must execute rather than skip in the privileged
  replacement-host run.
- [x] Relative and absolute OCI root paths, hostile symlinks, concurrent source
  changes, wrong architecture, malformed OCI JSON, and unsupported OCI fields.
  `test-runtime-root-validation.py` exercises canonical relative/allowlisted
  absolute roots plus traversal, non-canonical spelling, files, missing paths,
  and symlink components. `test-runtime-rootfs-build.py` injects mutation after
  a stable file read. `test-runtime-image-validation.py` covers wrong ELF
  architecture, interpreters, and escaping entrypoints, while the 29-case OCI
  suite rejects duplicate/truncated JSON and unsupported behavior fields before
  the builder reaches allocation.
- [ ] Read-only input rejection, private-write isolation, configured
  persistence, and proof that unconfigured writes do not persist.
- [ ] Block and inode exhaustion, high-water refusal, wrong UUID/generation,
  stale lock, duplicate attach, interrupted copy, and builder failure at every
  allocation boundary. The local ext4 builder now accounts for its private
  staging clone, normalizes staged atime/mtime and imported inode ctime, and
  passes repeated byte-identical rebuilds with hardlinks, symlinks, an explicit
  source-atime change, and a changed-content negative control. The broader
  allocation/fault matrix and disposable-host evidence remain open. Storage
  export start now retains `PREPARING` ownership on both pre-mutation and
  post-mutation failure; focused exact-retry and reconciliation tests prove an
  ambiguous live process is stopped and restarted with the same generation
  before `ACTIVE` is published. The durable storage store also rejects forged
  keys/identities/states, invalid release evidence, identity changes, state or
  timestamp regressions, and duplicate live owner/path/port/UUID claims before
  reconciliation; its file is loaded no-follow with inode, link, mode, and
  owner checks. Backend record/log reads now apply the same checks, readiness
  is bound to exact path/image/generation/size/port, and close counters require
  the corresponding ready marker plus a canonical terminal line. Tests reject
  mismatched and hard-linked evidence and prove managed stop signals before its
  timeout rather than waiting for a server that exits only on `SIGTERM`.
  Offline `e2fsck` now shares the bounded process-group runner with rootfs
  builds: a five-minute default, caller cancellation, one-MiB combined-output
  retention, descendant termination, and secret-safe errors are enforced.
  Focused tests cover timeout with a background child, overflow, stderr
  evidence hashing, and a non-clean exit. Live fault evidence remains open.
- [ ] Server loss during read, write, and flush; primary daemon restart;
  primary host reset where durability is claimed; corrupted image; clean and
  dirty recovery; snapshot/clone recovery using disposable copies.
- [ ] Cross-sandbox attempts to mount or address another sandbox's export.

### Replacement instance evidence required

- [ ] Record exact OCI index and selected `linux/amd64` manifest digests,
  containerd snapshot identity, source-root mount table, and before/after
  metadata or Merkle digests proving the caller snapshot was unchanged.
- [ ] Record two initramfs builds from the same input with identical manifests
  and digests, plus a changed-input negative control with a different digest.
- [ ] Prove `/bin/busybox` and any dynamic libraries are from the OCI root while
  `mk-agent`, bootstrap tools, and the transport module are outside it; retain
  hashes and mount/inode provenance from inside the child.
- [ ] Record backing allocation, owner sandbox and generation, quota/high-water
  state, mount table, request/flush counters where relevant, teardown order,
  offline filesystem result, and before/after proof that every cloud storage
  device and allocatable storage controller remained owned by the primary.
- [ ] Retain raw output for every injected failure and an immediate post-failure
  inventory showing no child, mount, TUN, iptables rule, partial artifact, or
  ownership leak.

## G5: primary-mediated networking

### Implementation still required

- [x] Implement the planned primary networking service boundary (`mknetd`) or
  revise the architecture and ownership documents to justify networking inside
  each shim. `mknetd` now owns durable endpoint allocation, Linux resources,
  restart reconciliation, and an authenticated root-only Unix API independently
  of shim lifetime; replacement-instance proof remains below.
- [x] Implement a CNI binary and versioned configuration supporting normal
  `ADD`, `CHECK`, and idempotent `DEL`, including partial-`ADD` rollback and
  stale namespace cleanup. `mk-cni` implements CNI 1.0.0, a durable generation
  cache, strict input, rollback, and stale-generation rejection. It now binds
  the returned endpoint to the exact ADD identity and validates address, MTU,
  DNS, owner, generation, and state before caching; safely identifiable
  post-ADD failures receive a bounded generation-bound DEL. Cache directories
  and files are owner/mode/symlink checked, and reads are no-follow and
  inode-stable. Beneath CNI, mknetd now journals `ALLOCATING` before its first
  namespace/link mutation and reconciles incomplete generations by bounded
  teardown; injected final-state persistence failure proves the durable record
  exists before mutation and is removed only after rollback. Durable endpoints
  receive full semantic/key/path validation before reconciliation, and the
  state file is loaded no-follow with inode-stability checks.
  Endpoint teardown symmetrically journals `DELETING` before external removal;
  an injected backend failure proves restart reconciliation completes deletion
  and removes the retained record. Runtime-owned RELEASE retains its sandbox
  generation through that phase, and a focused failure/retry test proves the
  same process can resume teardown without an mknetd restart.
- [x] Consume the CNI-created primary namespace and endpoint rather than
  requiring Docker `--network none` plus a runtime-private static link as the
  final design. An external CNI caller can create the OCI namespace endpoint;
  `mknetd` binds it to the exact sandbox generation. Standalone `ctr` uses a
  runtime-owned generation-named namespace through the same endpoint contract.
  The shim requests an already-open descriptor and performs no namespace or
  link operations; the previous fixed shim link/rules were removed.
- [x] Authenticate and generation-bind every network endpoint and recovery
  record. Reject stale sandbox identity, address reuse, and cross-generation
  reconnect. Peer credentials, endpoint generations, sandbox generations, and
  monotonic counter reports are checked together.
- [x] Replace fixed MTU/address/DNS assumptions with validated configuration,
  MTU negotiation, collision-free allocation, explicit link state, and
  bounded frame sizes. The configured RFC1918 pool allocates collision-free
  `/30`s and the agent rejects frames above the negotiated MTU.
- [x] Add bounded queues, backpressure, packet/drop/error counters, disconnect
  detection, reconnect policy, and slow/unresponsive guest handling. The
  data plane is single-frame/single-flight, uses a 250 ms exchange deadline,
  reconnects the authenticated agent sequence, and durably reports monotonic
  packet/drop/error counters and link state. Every privileged Linux network
  command in the primary and guest, plus the mknetd egress preflight, now uses
  the shared process-group runner with primary caller cancellation, guest
  server cancellation, a 30-second default, one-MiB combined output retention,
  and 16-KiB error diagnostics. Guest dispatch propagates its server context,
  setup rollback has an independent five-second cleanup bound, and failed link
  or DNS cleanup keeps retry identity. Repeated successful close cannot remove
  the restored DNS file. Focused tests cover a blocked descendant, output
  overflow, combined stdout/stderr, cancellation without mutation, repeated
  close, and failed DNS restoration; live fault and leak evidence remains open
  below.
- [x] Define firewall and network-policy ownership and install rules that
  cannot be bypassed by spoofed source addresses, alternate routes, malformed
  packets, or sibling traffic. Per-generation primary chains enforce source,
  metadata, sibling, return-traffic, egress, and default-drop policy.
- [x] Restore the guest's configured DNS state cleanly on teardown and avoid
  hard-coding a public resolver as the only supported policy. DNS is validated
  node configuration; regular-file, symlink, and absent states have restoration
  tests.

### Automated and live tests still required

- [ ] Explicit child-to-primary, outbound TCP, outbound UDP, DNS query/answer,
  and return-traffic assertions; retain destination and response details.
- [ ] Two sandboxes with overlapping internal names but distinct network
  identity, plus positive allowed routing and negative default isolation.
- [ ] MTU boundaries, fragmentation, checksums, malformed/oversized frames,
  loss, reordering, burst traffic, sustained load, and slow readers.
- [ ] Agent transport disconnect/reconnect, child restart, networking-service
  restart, `mkruntimed` restart, shim death, and primary restart.
- [ ] CNI failure after every partial `ADD` boundary, repeated `CHECK`, repeated
  `DEL`, stale namespace/link/rule cleanup, and name/address reuse.
- [ ] Source spoofing, route injection, metadata-address access policy,
  forwarding-rule bypass, and sibling-link policy bypass.
- [ ] Before/during/after checks for primary SSH, metadata access, guest agent,
  default route, NIC PCI ownership, and boot-disk/NIC controller ownership.

### Replacement instance evidence required

- [ ] Retain CNI stdin/config, command argv, stdout/stderr, exit status, primary
  namespace/link/route/rule state, child interface state, negotiated MTU, and
  packet counters for each `ADD`, `CHECK`, and `DEL`.
- [ ] Retain successful DNS, TCP, and UDP exchanges plus failed bidirectional
  sibling attempts. A bare `network-ok` or failed `ping` marker is insufficient
  for the final gate.
- [ ] Retain pre-run and post-run NIC/controller ancestry and final empty
  namespace, TUN/TAP, route, iptables/nftables, process, and recovery-record
  inventories.

## G6: containerd Runtime v2 shim

### Implementation still required

- [ ] Preserve and reconnect a running task after forced shim death. Safe
  reclaim is a useful fallback but is not the plan's reconnect requirement.
  Focused reconstruction now proves that an exact daemon-owned sandbox and
  network generation restore the recorded live guest PID, authenticated agent
  identity, relay/network ownership, output/wait loop, and subsequent exact
  exit completion. Reconstruction cleanup ownership now begins immediately
  after network-descriptor acquisition; a forced relay-start failure proves
  the descriptor, command, and socket identity are released. Forced-death
  process continuity and live identity evidence remain open.
- [ ] Define ownership transfer for containerd restart, shim restart, daemon
  restart, and shutdown. Reconstruct process state, stdio endpoints, exit
  status, and event delivery without changing the child boot identity. Task
  `Shutdown` now refuses to terminate while any process record remains, then
  atomically seals an empty service against Create, requires the durable event
  journal to flush, and joins event retry before invoking shutdown once. A
  failed flush leaves shutdown retryable. Agent relays are now explicit shim
  ownership: every start uses a dedicated process group, every connection or
  reconstruction failure reaps that group, normal Delete stops it only after
  the final guest Shutdown reply, and a failed socket removal retains its path
  for retry. Focused descendant and socket-cleanup tests pass. The complete
  cross-process restart transfer and live identity transcript remain open.
- [x] Implement faithful guest PID reporting or define a versioned virtual PID
  mapping. `Start`, `State`, `Pids`, `Connect`, exit, and delete now report the
  guest PID under mapping version `multikernel-v1-guest-pid`; the pre-start
  Create response retains the supervisor PID required by the Task v2 launch
  handshake.
- [x] Implement and test Task `Stats`, `Update`, and `Checkpoint`, or revise the
  advertised G6 surface and plan explicitly. `Pause`, `Resume`, and `Stats` are
  implemented with focused tests; pause/resume transact across every live init
  and exec process group, while Stats aggregates their CPU, RSS, and PID counts
  with overflow rejection. Plan 06 now explicitly excludes `Update`
  because Kerf allocation is generation-immutable and excludes `Checkpoint`
  because the selected Multikernel/Kerf contract has no checkpoint primitive;
  both reject before child contact or state mutation, with a focused test.
- [x] Complete lifecycle event publication and ordering, including exactly-once
  or documented replay semantics across restart, exec events, exit/delete
  races, and publication failure. Typed Task events are persisted before
  publication in a private bounded journal and replayed in local sequence after
  worker reconstruction or by a joined periodic worker while the shim remains
  otherwise idle. Each periodic publication attempt is bounded. The explicit
  contract is at-least-once: remote
  acceptance followed by a crash before local acknowledgement can duplicate an
  event. Exit queue state is durable and Delete repairs a missing exit first.
  Focused tests cover ordered replay, queue/ack failure, unsafe journal input,
  and exit-before-delete. Journal loading now additionally enforces caller
  ownership, one link, no symlinked ancestors, and stable identity across the
  bounded read; focused hardlink and ancestor tests cover the added boundary.
  The full event matrix and live transcript remain in their test/evidence rows
  below.
- [ ] Harden FIFO handling for peer disappearance, attach/detach churn, blocked
  writers, slow/unread output, output pressure, `CloseIO` races, and shim
  restart. Bound retained output and goroutine/process lifetime. Output is now
  nonblocking and fetched in atomic-size chunks; offsets advance only after
  complete delivery, while sustained pressure has a logged 30-second bounded
  drop policy. FIFO opens honor cancellation and focused tests cover reattach,
  acknowledged offsets, replay under pressure, bounded drop, and transient
  guest-close failure. Close requested and guest acknowledged are separate
  durable states; pending acknowledgement retries after FIFO EOF and recovery
  until process exit. Pre-start close persists without premature guest contact,
  and stopped close fails without mutation. The full peer/churn/`CloseIO`/
  restart matrix remains open. Stdio paths now fail before Create/Exec mutation
  unless they are absolute canonical, private, caller-owned, single-link FIFOs
  or output files. Start and reconstruction use no-symlink `openat2` opens and
  bind device/inode, mode, owner, link count, and ctime; focused tests reject
  unsafe modes, hardlinks, symlinked ancestors, inode replacement, same-inode
  mode changes, regular-file stdin, and cancelled opens. These changes still
  need live revalidation.
- [ ] Enforce context cancellation and deadlines through rootfs mount,
  initramfs build, daemon calls, child boot, agent connect, stdio, wait, and
  teardown without leaking resources. The shared daemon client now applies
  the earlier of a caller deadline and a 30-second default across dial, write,
  and read, and cancellation actively interrupts an already-connected socket.
  Focused tests cover pre-dial cancellation, blocked request writes, blocked
  response reads, the default timeout, and a successful round trip. Auditing
  also rejects unknown/duplicate response fields and mismatched response
  versions or request IDs. Fault-injecting every remaining boundary is still
  open. Rootfs builder execution now has bounded capture, a finite default,
  caller cancellation, and descendant process-group termination as described
  under G4. Rootfs Prepare/Cleanup/Reconcile reject pre-cancelled calls before
  mutation, mount entry checks cancellation, and storage-image verification
  checks between bounded hash reads. Focused tests prove pre-cancelled service
  calls preserve backend and journal state and that hashing returns the caller
  cancellation. Storage Provision/Release/Reconcile and all backend entry
  points now enforce the same pre-mutation rule; inspection polls during image
  hashing. Focused tests prove cancelled release retains `ACTIVE` ownership,
  no backend call occurs, and cancellation interrupts a multi-chunk inspection.
  The shared bounded runner also closes the offline-check descendant/output
  boundary with focused cancellation and overflow tests. Privileged network
  commands in both the primary and guest now use that runner as well. Focused
  deadline, descendant, output-bound, pre-mutation cancellation, and retryable
  DNS-cleanup tests pass. Kerf lifecycle commands now use the shared runner too:
  caller cancellation and a 30-second default kill the complete process group,
  output retention is capped at one MiB, and failures expose only retained byte
  count and SHA-256 rather than verbose output that may contain the agent token.
  Caller cancellation is checked before commands and sysfs observations and
  cannot be converted to success by an already-matching state. Focused
  descendant-timeout, secret-safe overflow, and canceled-observation tests
  pass. Host-qualification Kerf and guest-agent probes now share the bounded
  runner with a five-second default and 64-KiB combined-output ceiling; focused
  descendant-timeout, overflow, and mixed-output tests pass. Every supported
  mutating Task service RPC now rejects an already-cancelled context before
  local state, durable intent, events, or guest state can change, and rechecks
  cancellation after acquiring its mutation lock. A focused table test covers
  Create, Start, Kill, Exec, Delete, Shutdown, ResizePty, Pause, Resume, and
  CloseIO while proving state and guest-call counts remain unchanged. Shim
  startup and reconstruction also reject pre-cancelled calls before creating
  or inspecting runtime state, with focused coverage. Shim worker launch now
  treats address publication, inherited-socket transfer, process start, and
  PID publication as one cleanup transaction: any later failure kills and
  reaps the worker process group and removes only artifacts created by that
  attempt. A focused post-start PID-publication failure test covers the process
  and artifact boundary. The live cross-service cancellation/leak matrix is
  still open. Supported Task read/wait RPCs also reject pre-cancelled calls
  before locking or guest contact; focused State, Wait, Pids, Connect, and
  Stats coverage proves the process record and guest-call count are unchanged.
  Task entry lock acquisition itself is now cancellation-aware; contended
  read and mutation tests prove cancellation returns while another owner still
  holds the lock and without changing process state. Mandatory cleanup locks
  after an external mutation remain non-cancellable so ownership cannot be
  abandoned halfway through reconciliation. Event queue and flush lock
  acquisition is cancellation-aware as well; focused contention tests prove a
  cancelled publisher neither queues nor acknowledges journal state.
- [ ] Validate containerd namespace, task ID, bundle path, rootfs mounts, OCI
  process, and runtime paths before allocation; protect against symlink/path
  races and hostile mount inputs. Service construction rejects unsafe task,
  namespace, and bundle values; the global sandbox ID now digest-binds the
  complete namespace/task tuple so different namespaces, punctuation, or
  truncated long IDs cannot alias. Exec IDs outside the guest protocol's
  bounded safe alphabet are rejected before guest contact. Rootfs and OCI
  path validation is covered. Stdio validation and descriptor acquisition now
  apply the no-symlink, stable-identity contract described above; the broader
  hostile-input/failure matrix and disposable-host evidence remain open.
  Reconstruction and fallback Cleanup now share a bounded, no-symlink,
  caller-owned single-link recovery-file loader with stable identity and strict
  decoding. It binds sandbox/generation/task/storage/network ownership and
  validates bounded unique process state before external action. Cleanup no
  longer suppresses network, relay, sandbox, or rootfs failures; a failed stop
  prevents deletion and rootfs removal. Normal startup and reconstruction now
  share a no-symlink, caller-owned, single-link, exact-size, stable-identity
  token loader instead of reconstruction bypassing those checks with a path
  read. Focused wrong-owner, malformed-state, symlink, hardlink,
  symlinked-ancestor, partial-network, stop-failure, and rootfs-failure tests
  pass. Fresh connection and reconstruction also refuse to unlink a stale
  relay path unless it is a caller-owned, single-link Unix socket with safe
  mode; focused regular-file, directory, symlink, and missing-path tests prove
  hostile path types remain untouched. Live stale-socket replacement remains
  open. Recovery and event-journal publication is descriptor-anchored beneath
  a no-symlink, caller-owned, non-world-writable parent and uses same-directory
  `openat`/`renameat`; focused normal replacement, symlinked-parent, and unsafe
  parent-mode tests prove publication cannot be redirected.
- [ ] Expand OCI support required by the agreed G6 scope, or keep each omitted
  capability, namespace, mount, hook, rlimit, cgroup/resource, seccomp,
  read-only-root, hostname, and path control fail-closed with focused tests and
  truthful capability reporting.
- [ ] Complete packaging: versioned binaries, explicit containerd and Docker
  configuration fragments, service dependencies, fresh-host installation,
  upgrade/rollback behavior, and no default-runtime mutation. Every Go
  component now has an injected common version/revision and deterministic
  release manifest; Docker configuration is conflict-detecting, validated,
  opt-in, and preserves the default. The containerd v3/v4 import-only CRI
  fragment adds a named handler without owning the main config or default and
  passes structural plus containerd config-dump tests. A verified immutable
  release layout now provides atomic activation, fresh binary installation,
  upgrades, rollback, ownership-safe uninstall, and inactive-release removal
  with end-to-end tests. Automated service/config installation and live
  upgrade/rollback evidence remain open.

### Automated tests still required

- [ ] A fake-daemon Task v2 suite for every method, state transition, duplicate
  request, invalid transition, event, exit code, and cleanup path. Focused
  coverage now rejects exec start before init, exec creation after init exit,
  kill of absent or non-running processes, init deletion with retained execs,
  unsafe exec IDs, and duplicate execs before agent contact; the exhaustive
  method/transition matrix remains open.
- [ ] Event ordering and publication failure for create/start/exec/exit/delete,
  including containerd disconnect and restart. Delete now persists a queued
  marker, requires the ordered journal to flush before rootfs/process record
  removal, and retains retry ownership across guest or broker failure. Focused
  tests inject both boundaries; the exhaustive sequence and live disconnect
  transcript remain open.
- [ ] Context cancellation and deadline expiry at every blocking boundary.
- [ ] FIFO writer/reader disappearance, no initial peer, late attach, repeated
  attach, output backpressure, terminal and non-terminal `CloseIO`, resize
  before start and during exec, invalid resize, and teardown while attached.
  Focused tests now cover retained pre-start size, successful running resize,
  guest-rejected resize rollback, and stopped/invalid requests without state
  mutation. Reconstruction reapplies the durable size before restarting I/O;
  the remaining FIFO/attach matrix and live post-start resize are open.
- [ ] Unsupported OCI configuration before allocation and after each possible
  partial allocation, proving fail-closed cleanup.
- [ ] Two or more concurrent sandboxes under churn with disjoint CPUs, memory,
  generations, roots, agent endpoints, networks, and recovery records.
- [ ] Containerd restart, Docker daemon restart, `mkruntimed` restart, clean shim
  restart, forced shim death with reconnect, and forced shim death with bounded
  fallback reclaim.
- [ ] Init and exec signal delivery, ignored `SIGTERM`, `SIGKILL`, nonzero exit,
  descendant cleanup, wait/delete races, and same-name reuse after every
  failure mode. Pause/resume now signals all applicable init and exec process
  groups transactionally, with deterministic order and bounded reverse-order
  rollback after partial signal failure. Focused tests cover success and the
  partial boundary; the remaining signal/exit/churn and live matrix is open.

### Replacement instance evidence required

- [ ] Re-run the complete shared `ctr`/Docker matrix while retaining expanded
  commands and observed boot IDs, kernel release, image/binary provenance,
  stdout/stderr, stdin/attach output, terminal size, signals, exit statuses,
  names/generations, and cleanup inventories.
- [ ] Change each terminal size after the guest process is confirmed running
  and retain the before/after values. The 2026-09-02 run proves PTY operation
  and initial-size propagation, not a deliberate post-start live resize.
- [ ] Retain a containerd-restart transcript with service PID/boot identity,
  task state, child boot ID, exec before and after, stdio continuity, events,
  and final deletion. Repeat separately for Docker daemon restart.
- [ ] Retain an `mkruntimed`-restart transcript with journal/snapshot/recovery
  state and child identity before and after; do not rely only on a pass marker.
- [ ] Retain forced-shim-death transcripts for both required outcomes: task
  reconstruction/reconnect and bounded safe reclaim when reconnect is
  deliberately made impossible. Include killed PID, service journals, recovery
  record, task/client behavior, child state, and final resources.
- [ ] Retain exact Task v2 event order and timestamps for init and exec
  processes, including nonzero and signaled exits.
- [ ] Prove final return of CPUs, memory, Kerf instances/pool, rootfs mounts,
  initramfs/runtime artifacts, agent/relay/shim processes, FIFOs, TUN links,
  routes/firewall rules, containerd tasks/containers, and Docker containers.

## Cross-cutting evidence and tooling repair

- [x] Extend `make docs-check` or a dedicated evidence target to validate every
  committed `evidence/runtime-*/**/manifest.json`, not only schema fixtures.
- [x] Repair the 2026-09-01 feature-matrix and 2026-09-02 guest-I/O manifests,
  or preserve them as explicitly non-conforming historical manifests with a
  machine-readable explanation. Do not silently rewrite raw transcripts.
- [ ] Add required component versions, valid/redaction-aware host identities,
  exact command output paths, and `repository.diff_output` whenever `dirty` is
  true.
- [ ] Use separate schema-valid manifests or explicitly scoped run identifiers
  for G4, G5, and G6 assertions. A manifest labelled only `G6` must not be the
  sole index for G4/G5 claims.
- [ ] Capture structured `resources-before.json` and `resources-after.json`
  covering instances, disks, snapshots, addresses, firewall rules, and every
  billable or retained resource. Narrative cloud-cleanup text is supplementary,
  not a substitute for the contract ledger. A strict project-wide GCE ledger
  schema and exclusive-create collector now cover these resource classes,
  attachment users, and boot/data-disk auto-delete policy. The collector passed
  against the replacement project; retaining its before/after outputs in the
  final evidence bundle remains open.
- [ ] Make the live harness tee safe expanded commands and assertion values to
  the retained transcript while continuing to redact credentials and tokens.
  Preserve command exit status even when an expected negative test fails. The
  shared matrix now emits delimited observations for host/service identity,
  image digests/platforms, task states, three boot IDs and kernel releases,
  stdio/private-root values, DNS/HTTP output, endpoint addresses, both negative
  sibling exit statuses, daemon PIDs, signal/nonzero exits, stdin/attach/PTY
  output, and initial/final resource counts. The exclusive mode-0600 capture
  runner streams combined output, records redacted argv and UTC boundaries,
  preserves nonzero exit status, and fsyncs the transcript. Equivalent value
  capture in every isolated fault harness remains open.
  `mk-agentctl` now applies a 30-second deadline to complete framed exchanges,
  including malformed/authentication probes, and kills controller-owned relay
  process groups. Focused blocked-write, blocked-read, and descendant-reap
  tests prevent that portion of the live harness from stalling indefinitely;
  current-revision VM execution remains open.
- [ ] Capture relevant `journalctl` output for containerd, Docker,
  `mkruntimed`, shim, relay/network service, guest agent, and kernel/serial logs
  around every restart and fault injection.
- [ ] Record the exact working-tree patch or test-harness artifact used by a
  dirty run so installed binary hashes can be reproduced from source.
- [ ] Add a final evidence audit that checks referenced files exist, hashes the
  retained bundle, verifies assertions point to raw output rather than a
  narrative index, and rejects `result: pass` when required gate assertions are
  absent or failed. `scripts/audit-g4-g6-evidence.py` now enforces separate
  final G4/G5/G6 pass manifests, the versioned required-assertion inventory,
  exact capture envelopes/exit codes/raw assertion markers, consistent source,
  host and component identities, valid before/after GCE ledgers, tested-instance
  and boot-disk deletion, retained-disk preservation, reference containment,
  and a tamper-evident bundle inventory. Applying it to the completed live run
  remains open.

## Full `ctr` and Docker support work breakdown

The remaining work can be summarized into eight delivery workstreams. This is
the practical route from the current executable MVP to a supportable trusted-
node `ctr` and Docker runtime; it does not include Kubernetes, hostile multi-
tenant isolation, performance qualification, or general production release.

1. **Lifecycle and restart correctness:** preserve or reconstruct tasks across
   containerd, Docker, shim, and `mkruntimed` restarts; recover process state,
   stdio, exit status, and events; handle every lifecycle race and deadline.
2. **OCI validation and containment:** reject unsupported input before
   allocation, secure bundle/rootfs paths, validate architecture and kernel
   compatibility, and implement or explicitly exclude the agreed namespaces,
   capabilities, seccomp, mounts, hooks, rlimits, cgroups, hostname, and read-
   only-root surface.
3. **Task v2 completeness:** implement or formally exclude pause/resume,
   stats, update, and checkpoint; provide faithful PID semantics, event
   ordering/replay, and complete init/exec signal, wait, and delete behavior.
4. **Robust stdio and terminals:** bound output retention, implement
   backpressure and slow/absent-peer behavior, survive attach churn, preserve
   I/O across restart, and live-test a deliberate post-start resize.
5. **Deterministic image and storage support:** verify manifests, reproduce
   initramfs output and metadata, prove input snapshot immutability, define
   writable/persistent volume ownership, enforce quotas, and recover cleanly
   from interruption, exhaustion, corruption, and server loss.
6. **CNI networking:** replace the private static-link integration with normal
   `ADD`/`CHECK`/idempotent `DEL`, primary service ownership, negotiated
   configuration, bounded queues and counters, reconnect, policy enforcement,
   load/fault coverage, and complete cleanup.
7. **Control-plane and cleanup hardening:** close the G0-G3 crash windows and
   contract drift needed by G4-G6, propagate cleanup failures, detect orphans,
   persist recovery state, and make cancellation and rollback reliable at every
   allocation boundary.
8. **Packaging and release evidence:** version binaries and configuration,
   validate fresh installation and upgrade/rollback, retain exact source and
   structured resource ledgers, and pass the complete shared and isolated fault
   matrices with schema-valid evidence.

### Rough remaining-work calculation

The checklist currently contains 85 unchecked G4-G6 rows. The upstream G0-G3
checklist contains another 45 unchecked rows, but many are prerequisites or
test/evidence forms of the same work above; adding the two counts would greatly
overstate the number of independent features.

The estimate below assumes an engineer already familiar with Go, containerd
Runtime v2, Linux storage/networking, and this Multikernel/Kerf environment. It
includes implementation, focused automated tests, and the corresponding live
fault run, but excludes Kubernetes, performance work, multi-tenant hardening,
and time waiting for upstream kernel or Kerf changes.

| Workstream | Rough person-weeks remaining |
| --- | ---: |
| Lifecycle, restart, and task reconstruction | 6-10 |
| OCI validation and agreed containment controls | 8-14 |
| Remaining Task v2 methods, events, and PID semantics | 4-7 |
| Stdio, attach, terminal, and backpressure hardening | 4-7 |
| Deterministic roots, writable storage, quotas, and recovery | 10-18 |
| CNI service, policy, reconnect, and network fault matrix | 10-16 |
| Foundational control-plane, rollback, and cleanup hardening | 5-9 |
| Packaging, fresh-host validation, and evidence repair | 4-7 |
| **Base total** | **51-88 person-weeks** |

Allowing roughly 20-25% integration and disposable-host debugging contingency
gives a planning range of approximately **60-110 person-weeks**. In calendar
terms, that is roughly:

- one experienced engineer: **14-25 months**;
- two engineers with storage/network and shim work split: **8-14 months**; or
- three engineers with clear ownership and shared integration support:
  **6-10 months**.

These ranges are intentionally broad. Shim reconnection, storage recovery, CNI
fault handling, or an upstream Multikernel/Kerf defect can dominate the
schedule. Explicitly narrowing supported OCI, persistence, or Task v2 methods
through an approved plan/contract revision would reduce it.

As a rough maturity measure, about **60% of the ordinary happy-path client
feature rows** have a passing or narrowly passing implementation, but only
about **25-35% of full support readiness** is complete after weighting restart,
failure safety, storage, CNI, containment, packaging, and auditable evidence.
The practical remaining fraction is therefore approximately **65-75%**. This
is a planning judgment, not a mechanically derived gate score; unchecked rows
are deliberately not treated as equal-sized units.

## Required replacement-run sequence

Use a fresh disposable qualified instance and stop on the first unexplained
failure. Capture evidence before repair, rebuild, reboot, or deletion.

1. Record cloud resources, source commit/diff, host qualification, boot ID,
   kernel/Kerf versions, installed hashes, services, CPU/memory/pool state,
   mounts, devices, networking, and empty runtime inventories.
2. Run the G4 deterministic-root, snapshot-immutability, ownership, quota,
   failure, persistence, and recovery matrix with raw output.
3. Run the G5 CNI lifecycle, traffic, isolation, MTU/load/fault, reconnect,
   policy, and primary-health matrix with raw output.
4. Run the G6 shared lifecycle and I/O matrix, then isolated containerd,
   Docker, daemon, and shim restart/crash cases with raw output and events.
5. Run every expected rejection from clean state and record both the error and
   the immediate no-allocation/no-leak inventory.
6. Run the full local/race verification suite on the exact source revision
   installed on the instance.
7. Record final host health and empty CPU/memory/pool/storage/network/process/
   container inventories, then delete the instance and auto-delete disk.
8. Validate all manifests and links locally before describing any item as
   closed. Publish a final claim-to-assertion-to-file matrix for G4, G5, and G6.

## Gate closure

- [ ] G4 may close only after every G4 plan requirement has implementation,
  automated coverage, and replacement-instance evidence, or an approved plan
  revision explicitly removes it.
- [ ] G5 may close only after normal CNI operations, isolation, cleanup,
  controller ownership, and the network fault matrix pass with raw evidence.
- [ ] G6 may close only after the agreed Task v2 surface, restart/reconnect,
  event, stdio, cancellation, concurrency, and cleanup matrices pass with raw
  evidence.
- [ ] Reissue the G4-G6 summary and update checked project tasks only after each
  claim maps to a schema-valid assertion and actual retained instance output.
