# G4-G6 remediation and live-evidence checklist

Audit date: 2026-09-02; implementation status refreshed 2026-09-18

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
| G4 | The current tree builds and verifies canonical manifests and deterministic newc roots, rejects observed source mutation and unsafe metadata, produces bounded fully allocated private ext4 images, generation-binds one mediated export, journals graceful teardown/recovery, and now has privileged live ctr/Docker proof for ephemeral private writable roots, non-persistence after delete/name reuse, materialized read-only directory/file binds with no host write-through, and fail-closed generic writable-volume/propagation rejection. | Persistent and writable host volumes are explicitly outside v1 rather than partially supported. Exhaustion, corruption, server-loss, host-reset, clone, cross-export, and replacement-instance evidence matrices have not passed on the current revision. |
| G5 | The current tree contains `mknetd`, CNI 1.0 `ADD`/`CHECK`/idempotent `DEL`, generation-bound endpoint state, negotiated MTU/DNS, bounded exchange/counters, restart reconciliation, and exact-address anti-spoof/firewall policy. Current live evidence proves distinct `/30` identities, DNS/HTTP egress, bidirectional sibling rejection, daemon continuity, and clean teardown for shared ctr/Docker workloads. | UDP, MTU/fragmentation, sustained load/backpressure, injected faults/reconnect, spoof/bypass, and comprehensive primary-health evidence matrices remain open. |
| G6 | The current tree implements the core Task v2 lifecycle, faithful versioned guest PIDs, pause/resume/stats, standard OCI process controls, durable task/process/I/O offsets, a supervised shim worker, and generation-bound task reconstruction. Current retained evidence proves the shared ctr/Docker lifecycle/I/O matrix, exact post-start resize, daemon/containerd/Docker/clean-and-forced-shim restart continuity, durable ordered Task-event replay, the exhaustive 17-method fake-daemon matrix, concurrent churn, sequential pool reuse, and graceful final pool release. | The live cross-service cancellation matrix, deferred stale-relay workload observation, complete restart/transport fault scope, final evidence manifests/audit, and remaining isolated fault reruns remain incomplete. |

The canonical gate rows in [`../plans/README.md`](../plans/README.md) and
[`../../project/TASKS.md`](../../project/TASKS.md) must remain unchecked until
the corresponding unchecked work in this document is either completed or the
normative plan is revised with an explicit rationale.

## Continuation checkpoint: 2026-09-17

The current branch contains committed implementation progress through
`0bcbe0f`, including replay-safe guest signals, primary-side materialization of
bounded read-only directory and regular-file bind inputs, an authenticated
guest capability handshake, standard Docker/`ctr` read-only bind option
normalization, and bind-destination hiding semantics. The corresponding
focused fault tests, 100-iteration race-detector groups, full
`go test -race -count=1 ./...`, `go vet ./...`, and `scripts/check-docs.sh`
passed locally before this checkpoint. These are local implementation results,
not replacement-instance evidence.

Disposable instance `mklinux-g4-g6-final-20260905` in
`asia-southeast1-b` was observed `TERMINATED` and restarted on 2026-09-17. GCE
then reported it `RUNNING`, with start timestamp
`2026-09-17T05:31:55.506-07:00`, internal address `10.148.0.56`, and ephemeral
external address `34.126.167.148`. This is an operator observation only: no new
evidence bundle was created, and the replacement instance has not yet been
qualified against or populated with the current source revision.

Source transfer and new retained infrastructure evidence remain deliberately
paused pending the exact authorization:
`Approved: upload the source-only archive and retain the described infrastructure evidence.`
Until that authorization is supplied, local remediation may continue, but no
current-revision live claim may be marked proved.

## Incremental audit findings: 2026-09-18

These findings were recorded before the corresponding implementation work so
that an interrupted remediation pass does not lose them:

- GCE again reported `mklinux-g4-g6-final-20260905` `RUNNING` on 2026-09-18,
  with the same start timestamp and external address recorded above. No restart
  was required. This is a control-plane observation, not software-execution
  evidence, and no source or evidence artifact was transferred.
- Rootfs replay and reconciliation verify the prepared initramfs through a
  held, identity-matched runtime-directory descriptor. The subsequent load
  formerly reopened `.multikernel/initramfs.path` and `token` through the public
  pathname and supplied the initramfs pathname to Kerf. The rootfs service now
  revalidates the complete prepared result and returns the exact journal-bound
  runtime-directory and initramfs descriptors to lifecycle. Kerf validates
  bounded artifacts relative to that held directory, requires the canonical
  `initramfs.path`, reads the token from the same inode, and inherits the exact
  initramfs as fd 3. A fake-Kerf test replaces the public runtime directory
  before `Load`, reads the original bytes and token, and preserves the
  substitute. Rootfs-to-lifecycle descriptor ownership and close behavior are
  tested separately. The backend does not verify and then reopen the archive:
  it hashes the exact open file against both durable build-result digests,
  rewinds it, and returns that same descriptor for inheritance.
- The approved kernel-manifest resolver formerly verified artifact digests and
  returned path strings. Manifest, config, and artifact reads now use bounded
  no-follow opens with stable-identity checks; the resolver retains the exact
  digest-verified kernel and approved initramfs descriptors through the
  lifecycle call, and Kerf inherits the kernel after the generated initramfs.
  A replacement test swaps the public kernel pathname after resolution and
  proves both the resolver and fake Kerf retain the original inode while the
  substitute remains untouched. The combined kernel/rootfs/lifecycle/Kerf
  replacement group passed 100 race-detector repetitions locally on
  2026-09-18.
- Identity-conditional recursive removal no longer relies on a check followed
  by `RemoveAll` of the public name. Cleanup uses `renameat2(RENAME_NOREPLACE)`
  to move the child into a deterministic identity-derived quarantine, verifies
  both the opened and named quarantined inode, and removes contents only below
  that held descriptor. A replacement injected between inspection and rename
  is detected and restored intact rather than deleted. A quarantine left by a
  crash between rename and removal is recognized by its recorded identity and
  resumed. Focused replacement and recovery cases passed 100 race-detector
  repetitions locally on 2026-09-18.
- The shared private-file helper now returns the exact inode read or published
  and supports identity-conditioned, no-replace quarantine removal. Storage
  process logs and records retain those identities across cleanup, so they no
  longer use unconditional `unlinkat` after validation. CNI carries its exact
  cache inode across the mknetd `DEL`, rejects even a same-content inode
  replacement, and preserves both original and substitute. Focused safe-file,
  storage, and CNI groups passed 100 race-detector repetitions. The CNI group
  also proves two repeated `CHECK`s, idempotent repeated `DEL`, and same-name
  reuse with a new generation. Quarantine names bind a hash of the logical
  filename plus device/inode; a bounded recovery read rediscovers exactly one
  matching residue. A simulated restart containing only the quarantined CNI
  cache reissues the exact generation-bound `DEL` and clears the residue in the
  same 100-repetition group. Storage process-record reads likewise rediscover
  an identity-bound quarantine after simulated restart in 100 repetitions.
- The shared Unix-socket owner used by mknetd, mkruntimed, the guest agent, and
  shim relay paths now applies the same exact-inode quarantine rule to stale
  replacement and shutdown cleanup. Its privilege-independent removal-time
  replacement algorithm passed 100 race-detector repetitions. The equivalent
  real pathname-socket case remains skipped because this local sandbox denies
  pathname listener creation; it remains mandatory without a skip on the
  disposable host.
- A continuation audit found two socket-lifecycle callers outside that shared
  cleanup path. `mknetd` created its socket parent with pathname-recursive
  `MkdirAll`, which could follow a substituted ancestor and could not express
  the rule that existing ancestors retain their modes. Its startup now walks
  from `/` with held directory descriptors and `O_NOFOLLOW`, creates only
  missing components, preserves existing ancestor modes, and requires the
  final parent to be caller-owned and not group/other-writable. `mk-agentctl`
  reconnect and final relay teardown also used pathname removal; it now
  captures each relay socket before dialing, dials through the held parent
  descriptor while checking that exact identity before and after connection,
  and delegates removal to the identity-conditioned quarantine owner. The
  parent creation, invalid-root rejection, privilege-independent removal, and
  affected command-package groups passed 100 race-detector repetitions. The
  real captured-socket dial/replacement case remains skipped under the local
  pathname-listener restriction and is still a required disposable-host case;
  it was not counted as local proof. After this change, the full repository
  race suite, `go vet ./...`, documentation/schema/evidence/deployment checks,
  and `git diff --check` all passed locally on 2026-09-18.
- The next raw-path audit found that shim startup's
  `removeStaleRelaySocket` remained a separate `Lstat`-then-`os.Remove` path,
  outside the shared socket owner. A same-name replacement after validation
  could therefore be unlinked even though running-relay cleanup was
  identity-bound. The same audit found raw pathname cleanup for partially
  written shim address/PID files and the supervisor worker PID marker; those
  are recorded as a distinct follow-up boundary rather than being implied
  closed by the socket fix. Stale startup cleanup now treats an initially
  missing path as a no-op and otherwise captures and removes only the exact
  safe socket through the shared owner. Unsafe non-socket rejection and the
  privilege-independent removal algorithm passed 100 race-detector
  repetitions. The real safe-stale pathname-socket test is skipped locally and
  remains a disposable-host case. Containerd's atomic address/PID publications
  are now captured as exact caller-owned, single-link 0644 regular-file inodes
  beneath a held no-symlink, non-writable working-directory descriptor.
  Partial-launch rollback quarantines only those captured identities. Focused
  tests prove normal address removal after a later PID failure and preservation
  of both an original and a same-name replacement. The supervisor now opens
  its working directory without following symlinks or changing its safe mode,
  removes a bounded private crash residue by exact identity, exclusively
  publishes each new worker PID, and removes that captured inode after the
  worker exits. It refuses group/other-writable working directories and
  publication collisions. The safe-directory/capture group and the combined
  launch/replacement/supervisor/stale-relay group each passed 100 race-detector
  repetitions. The subsequent full repository race suite, `go vet ./...`,
  documentation/schema/evidence/deployment checks, and `git diff --check` all
  passed locally on 2026-09-18. The real pathname-socket skip remains excluded
  from this proof as stated above.
- Guest DNS setup and teardown still inspect, remove, write, and restore
  `/etc/resolv.conf` by public pathname. This is not protected merely by being
  inside the guest: a running OCI process can replace that name while the
  agent retains network ownership, after which `CloseNetwork` can delete the
  process's replacement. Initial setup has the same inspect/remove window for
  the supported regular-file and symlink forms. The required closure is a held
  no-symlink parent descriptor, exact original and managed identities,
  identity-conditioned removal, and exclusive descriptor-relative restore.
  The agent now opens the caller-owned, non-writable parent without following
  symlinks, reads a regular original or symlink target from the same stable
  identity it records, removes only that identity, and exclusively publishes
  the exact-mode managed resolver. Teardown removes only the managed inode and
  exclusively restores the original bytes/mode, symlink target, or absence.
  A same-mode process replacement is preserved and reported; returning the
  exact managed inode permits a later retry to restore the original. If setup
  fails after mutation and immediate restoration is blocked, the
  held descriptor and pending state remain owned by `Manager` for a later
  `CloseNetwork` retry rather than being discarded with the setup error.
  Public regular/symlink helper coverage and regular/symlink/absent plus replacement
  DNS coverage each passed 100 race-detector repetitions. The subsequent full
  repository race suite, `go vet ./...`, documentation/schema/evidence/
  deployment checks, and `git diff --check` passed locally on 2026-09-18.
  Disposable-host validation remains pending.
- The allocation mutex still opened `MK_SHIM_LOCK` with pathname
  `O_CREATE|O_RDWR`, followed symlinks, accepted an unvalidated object, and
  blocked in `flock` without observing caller cancellation. Replacement of the
  public lock name can also split cooperating shims across different inodes.
  The planned closure is to validate the configured location, open its
  caller-owned non-writable parent without symlinks, and acquire a cancellable
  exclusive lock on that held directory descriptor itself. This finding is
  recorded before implementation. Allocation now normalizes the configured
  location, opens that exact parent component-by-component with no symlinks,
  preserves its safe mode, and uses nonblocking `flock` retries governed by the
  caller context. The filename is never opened, so a symlink there cannot
  redirect or split the lock. A focused test proves bounded cancellation under
  real contention, unchanged symlink-target bytes, reacquisition after close,
  and rejection of a group-writable parent in 100 race-detector repetitions.
  The subsequent full repository race suite, `go vet ./...`, documentation/
  schema/evidence/deployment checks, and `git diff --check` passed locally on
  2026-09-18. Disposable-host validation remains pending.
- Service construction still validates the bundle with `EvalSymlinks` plus
  `Stat` and then retains only its pathname. Later OCI/network reads, recovery
  state, token/event publication, and the rootfs daemon reopen that public
  name. Their individual no-follow checks safely bind the directory they open,
  but do not prove it is the bundle inode originally assigned to this shim. A
  caller-owned same-mode replacement can therefore cross the service-to-RPC or
  shim-to-daemon handoff. Restart adds a second boundary because the supervisor
  sets `cmd.Dir` from the public working-directory pathname. This finding is
  recorded before implementation; closure requires a service-lifetime bundle
  descriptor/identity, identity propagation to rootfs, durable restart
  binding, and replacement tests at each handoff.

- The first focused implementation run on 2026-09-18 passed
  `internal/safefile` and `internal/rootfs`, including the new rootfs bundle
  identity handoff rejection. The shim package did not pass: existing unit
  fixtures generally inherit the environment's `0775` temporary-directory
  mode, while the new held-bundle opener deliberately rejects
  group/other-writable directories. The resulting persistence failures cascade
  into monitor timeouts. This is recorded before fixture repair and is not
  counted as a passing shim result or complete bundle-identity closure.

- After changing only bundle fixtures to the existing private-directory test
  helper, `go test -count=1 ./cmd/containerd-shim-multikernel-v2` passed in
  2.737 seconds. This confirms that the earlier cascade was caused by obsolete
  fixture permissions. Adversarial replacement, supervisor restart, repeated
  race, full-repository, and disposable-host evidence are still pending.

- The focused safefile/rootfs/shim group then passed together once. New
  adversarial tests observed that conditional recovery-file exchange rolls
  back without deleting either a raced public substitute or the displaced
  original; a worker launched after public bundle replacement has the held
  bundle inode as its actual cwd and receives the same device/inode/UID
  handoff; incomplete or mismatched supervisor handoff is rejected; and a
  persisted bundle mismatch is rejected before any daemon RPC. Repeated race
  execution, descriptor-lifetime cleanup, durable binding across a completely
  new supervisor, and disposable-host proof remain open.

- Successful Task shutdown now releases the held bundle descriptor through a
  one-shot close, before invoking the shim shutdown callback. The focused
  shutdown test passed and observed the closed-file error on a subsequent
  descriptor operation. This closes the in-process descriptor-lifetime leak;
  it does not solve authoritative identity discovery by a brand-new
  supervisor.

- Authoritative new-supervisor binding is now carried in the daemon's
  journaled `SandboxConfig` as bundle device/inode/UID. Lifecycle validation
  rejects an absent identity, allocation copies it from the held descriptor,
  and recovery compares the daemon-returned value before token loading or any
  network, relay, or agent reconnect. The protocol/rootfs/lifecycle/daemon/shim
  package group passed once. A focused mismatch test observed exactly one
  `ListSandboxes` RPC and no resource reconnect. Schema validation, repeated
  race execution, full-repository checks, and disposable-host proof remain
  pending.

- Descriptor-relative token create/reuse, event-journal bundle replacement,
  recovery runtime-directory replacement, and two-descriptor shutdown cleanup
  passed 100 race-detector iterations as a combined focused group. This closes
  the locally identified bundle-descendant handoff cases; current-revision
  disposable-host validation remains unauthorized and pending.

- Final semantic review found a rootfs-to-shim window between creation of
  `.multikernel` and the shim's first held open. `PrepareRootfs` now returns the
  exact runtime-directory identity on first preparation and replay, and the
  shim verifies its held child descriptor before token creation. The focused
  rootfs and shim packages pass, including a prepared-directory replacement
  rejection. Repeated race and final full-tree verification are pending.

- Rootfs prepare/replay runtime-identity return and shim prepared-directory
  replacement rejection each passed 100 race-detector iterations.

- On the final current tree, the complete repository race suite,
  `go vet ./...`, `scripts/check-docs.sh`, G2 shell syntax, and
  `git diff --check` all passed on 2026-09-18. This establishes the local
  bundle-identity checkpoint. It does not replace the still-unapproved
  disposable-instance execution and evidence collection.

- Post-checkpoint audit after `52ea8a3` found that the containerd
  delete/reclaim `Cleanup` path still read `.multikernel/sandbox.json` through
  a relative pathname and trusted that local record before destructive
  daemon/network cleanup. This finding is recorded before implementation. The
  path must reuse held bundle/runtime descriptors and match daemon-journaled
  bundle identity before stopping ownership.

- Fallback cleanup now loads recovery through the held `.multikernel`
  descriptor, requires its bundle identity to match the service handoff, and
  lists daemon sandboxes to confirm the same ID, generation, and journaled
  bundle identity before network, relay, sandbox, or rootfs cleanup. The
  focused fallback suite passes: existing stop/rootfs failure propagation is
  preserved, daemon mismatch performs only `ListSandboxes`, and public bundle
  replacement performs zero daemon calls. Repeated race and full-tree checks
  remain pending.

- The complete fallback-cleanup group passed 100 race-detector iterations
  after descriptor and daemon identity binding.

- The subsequent full repository race suite, `go vet ./...`, documentation/
  schema/evidence/deployment checks, and `git diff --check` all passed locally
  on 2026-09-18. Disposable-host validation remains pending authorization.

- The next fallback audit confirmed that containerd's `ReadAddress` plus
  `RemoveSocket` path ultimately performs raw `os.Remove` on the socket named
  by the bundle file. A same-owner address-file replacement could redirect
  cleanup to another shim socket. This is recorded before implementation;
  cleanup must compare the held address bytes to this task's deterministic
  containerd address and remove only a captured socket inode.

- Fallback socket cleanup now reads `address` through the held bundle
  descriptor, parses a unique nonempty containerd `-address` invocation value,
  recomputes the namespace/task-specific socket, requires byte-for-byte
  equality, and hands the canonical path to the shared exact-inode socket
  owner. Redirected content never reaches socket capture. Missing recovery can
  still remove this authenticated startup socket without a daemon call. The
  combined focused address/flag/fallback suite passes once; repeated race and
  full-tree verification remain pending.

- The deterministic address parsing, redirected-address rejection,
  exact-socket owner handoff, no-recovery cleanup, and authenticated fallback
  group passed 100 race-detector iterations.

- The subsequent full repository race suite, `go vet ./...`, documentation/
  schema/evidence/deployment checks, and `git diff --check` passed locally on
  2026-09-18. The local socket-ownership checkpoint is complete; live proof is
  still pending the explicit source/evidence authorization.

- The adjacent `StartShim` collision path still handled `EADDRINUSE` by
  unconditionally removing the deterministic socket and binding a new one.
  That can disrupt a live shim, and the raw remove can delete a same-name
  replacement installed after the failed bind. Containerd's grouping-aware
  manager first preserves a connectable socket, but its pathname probe/remove
  sequence does not close the replacement race. This finding is recorded
  before implementation. Closure requires capturing the exact socket inode,
  probing that captured owner, preserving a live listener, and removing only
  the captured stale inode before one bounded rebind attempt.

- Startup now uses the shared held-parent listener in exclusive mode rather
  than containerd's pathname listener/remover. On collision it captures one
  exact socket identity, dials through that held owner, returns the existing
  address without removal when live, and conditionally quarantines only that
  inode when stale before one rebind. Failure rollback closes the
  identity-owning listener and no longer follows it with raw `os.Remove`.
  Focused shim tests pass for live, stale, and replaced-owner outcomes; focused
  Unix-socket tests pass for exclusive collision preservation, non-removing
  owner release, and exact cleanup. The pathname-listener cases are permitted
  locally in this run and did not skip. Repeated race and full-tree checks are
  pending.

- The live/stale/replaced startup collision group and the exclusive-bind,
  captured-owner release, replacement cleanup, and privilege-independent
  quarantine group each passed 100 race-detector repetitions. The real
  pathname listener cases again ran without a skip. Full-tree verification is
  pending.

- The subsequent complete repository race suite, `go vet ./...`, the full
  documentation/schema/evidence/deployment validation chain, and
  `git diff --check` passed on 2026-09-18. The documentation chain again
  covered 7 schemas/22 cases and 17 explicitly classified historical evidence
  manifests. Its separate Python socket-rejection subcase remained skipped by
  that sandbox, while the new Go pathname-socket cases above executed. The
  local startup-socket checkpoint is verified; current-source disposable-host
  proof remains pending the explicit transfer/evidence authorization.

- Final semantic review tightened the collision probe before checkpointing:
  it now observes caller cancellation, and only `ECONNREFUSED` authorizes a
  conditional stale-inode removal. Cancellation or any other probe failure
  releases capture authority without deleting the endpoint. The new cancelled
  probe case and the focused shim/socket packages pass once; repeated race and
  full-tree results above must be rerun for this refinement.

- The refined focused groups passed 100 race-detector repetitions. This run
  includes an integrated real-path case that reuses a live captured listener,
  then replaces a closed stale listener through exact conditional removal and
  exclusive rebind; it did not skip. Cancellation remained non-destructive.
  Final full-tree verification is pending.

- On the refined tree, the complete repository race suite, `go vet ./...`,
  documentation/schema/evidence/deployment validation, and
  `git diff --check` all passed on 2026-09-18. The independent Python
  socket-rejection subcase retains its documented sandbox skip; the integrated
  Go collision case did not skip. This supersedes the pre-refinement full-tree
  result and completes the local startup-socket checkpoint.

- A final cancellation-boundary test now cancels after the captured dial
  reports refusal and proves removal is not attempted. Context is also checked
  before the first bind and before rebind. Both focused groups passed another
  100 race-detector repetitions; the full-tree result immediately above must
  be rerun once more for these boundary checks.

- The final post-boundary complete repository race suite, `go vet ./...`,
  documentation/schema/evidence/deployment validation, and
  `git diff --check` all passed on 2026-09-18. This is the authoritative local
  result for the startup-socket checkpoint. Live current-source execution is
  still not claimed without the required authorization.

- The next startup audit found a residual publication window: containerd's
  atomic pathname writers installed `address` and `shim.pid`, after which the
  shim separately reopened and captured the public names. A same-owner
  replacement between those operations could become rollback's recorded
  object, and an existing entry was overwritten before any ownership proof.
  This finding is recorded before implementation. Both files must be
  exclusively published relative to an already-held safe parent and return the
  exact created identity as part of that one operation.

- `address` and `shim.pid` now use the shared descriptor-relative exclusive
  regular-file publisher. A pre-existing entry is preserved and rejects the
  launch, while successful publication returns the exact inode used by
  rollback without a reopen window. Focused shim tests pass for exclusive
  collision, exact replacement preservation, post-start PID failure cleanup,
  and the prior address replacement case. Repeated race and full-tree checks
  are pending.

- The exclusive publication, raced replacement, and partial-launch cleanup
  group passed 100 race-detector repetitions. Full-tree verification remains
  pending.

- The subsequent complete repository race suite, `go vet ./...`, the full
  documentation/schema/evidence/deployment checks, and `git diff --check`
  passed on 2026-09-18. The local exclusive launch-metadata checkpoint is
  complete; current-source disposable-host proof remains unauthorized.

- Despite descriptor-bound worker restarts, the initial supervisor command
  still inherited `cmd.Dir` from the public `Getwd` pathname constructed by
  `newCommand`. A same-owner bundle replacement between service validation and
  `exec` could therefore start the supervisor in the substitute before any
  worker identity handoff. This residual initial-handoff finding is recorded
  before implementation; the supervisor command must chdir through the
  service-lifetime held bundle descriptor.

- The initial supervisor command now sets `cmd.Dir` to the held bundle's
  `/proc/self/fd` path; `newCommand` no longer snapshots the public cwd name.
  Supervisor startup also opens the inherited current directory before
  deriving any display path. A focused replacement test moved the original
  bundle, installed a same-mode substitute, and observed the supervisor marker
  only in the held original. The existing worker-restart identity and
  pre-cancelled startup cases pass with it. Repeated race and full-tree checks
  are pending.

- The initial-supervisor replacement, descriptor-bound worker restart, and
  pre-cancelled startup group passed 100 race-detector repetitions in 105.230
  seconds. Full-tree verification remains pending.

- The subsequent complete repository race suite, `go vet ./...`, the full
  documentation/schema/evidence/deployment checks, and `git diff --check`
  passed on 2026-09-18. The local initial-supervisor bundle-handoff checkpoint
  is complete; current-source disposable-host proof remains unauthorized.

- Collision reuse currently proves only that the deterministic socket accepts
  a connection. It does not bind that listener to this held bundle's published
  `address`/`shim.pid`, the expected supervisor executable/cwd/process group,
  or its namespace/task/containerd-address invocation. A same-owner listener
  at the deterministic name can therefore be returned as this task's shim.
  This finding is recorded before implementation. Reuse must fail closed unless
  launch metadata and an anchored `/proc/<pid>` supervisor identity all match.

- Live collision reuse now additionally requires the held bundle's exact
  `address` and numeric `shim.pid`, then opens and retains `/proc/<pid>` while
  checking the supervisor cwd against the held bundle, executable inode against
  `/proc/self/exe`, live process-group leadership, and exact namespace/task/
  containerd-address command flags. Focused tests accept the exact supervisor
  and reject a different cwd, executable, process group, task ID, or containerd
  address. The earlier live/stale/replaced socket cases pass alongside it.
  Repeated race and full-tree checks remain pending.

- The authenticated existing-supervisor and live/stale/replaced socket group
  passed 100 race-detector repetitions. Full-tree verification remains
  pending.

- The subsequent complete repository race suite, `go vet ./...`, the full
  documentation/schema/evidence/deployment checks, and `git diff --check`
  passed on 2026-09-18. The local authenticated collision-reuse checkpoint is
  complete; current-source disposable-host proof remains unauthorized.

- `python3 scripts/check-runtime-schemas.py` passed after the contract change:
  7 schemas and 22 fixture cases. The schema result proves structural contract
  consistency only; it is not runtime or disposable-host evidence.

- The conditional exchange tests, rootfs handoff replacement test, and shim
  held-bundle/supervisor/recovery/shutdown identity group each passed 100
  race-detector iterations. The shim group took 104.120 seconds because every
  supervisor case launches a race-instrumented worker subprocess. This is
  repeated local execution evidence; the full repository suite and authorized
  disposable-host run remain pending.

- `GOCACHE=/tmp/mklinux-gocache go test -race -count=1 ./...` passed across
  the complete Go runtime repository on 2026-09-18 after the bundle changes.
  Vet, documentation/evidence checks, diff review, and disposable-host proof
  remain pending at this point.

- A post-suite caller audit found that the direct G2 `CreateSandbox` harness
  still emitted the pre-identity JSON shape. It now captures each prepared
  bundle with GNU `stat` and sends that exact device/inode/UID. Lifecycle tests
  explicitly reject a missing identity, and the shim plan now names recovery
  format v3 rather than stale v2. These compatibility edits were recorded
  before their verification rerun.

- The compatibility rerun passed: `bash -n scripts/test-runtime-g2.sh`, the
  complete `scripts/check-docs.sh` chain, and 100 race-detector iterations of
  lifecycle input validation. The docs chain again covered 7 schemas/22 cases,
  17 classified historical evidence manifests, OCI/bind/bootstrap/image,
  release/binary/deployment/resource-ledger/containerd configuration, and the
  final G4-G6 evidence-audit tests.

- Diff review found two remaining descendants of the bundle boundary before
  checkpointing: the event journal still reopened the public bundle path, and
  `.multikernel` plus first token creation were not held across operations. A
  new child-directory helper also needed a nil-safe `Stat` failure path. The
  event journal is now read/replaced/removed relative to the held bundle; the
  exact `.multikernel` descriptor is retained once opened, token creation uses
  exclusive descriptor-relative publication, and shutdown closes both held
  directories. The shim package passes once. Replacement tests observed no
  event journal in a substitute bundle, no recovery file in a substitute
  runtime directory, and the original recovery remaining at PID 7 rather than
  the rejected PID 8 update. Repeated race and final full-suite checks remain
  pending.

Privileged disposable-host execution remains subject to the exact authorization
phrase above.

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

- [x] Generate a deterministic image/root manifest and verify it before any
  sandbox allocation. Record the manifest digest separately from the source
  OCI image digest and generated initramfs digest.
  A 2026-09-19 descriptor audit found that `build-runtime-rootfs.py` begins
  scanning with `root.resolve(strict=True)`. When rootfs passes an inherited
  `/proc/self/fd/...` input, this resolves it back to a caller-visible pathname
  and discards the held-directory boundary before manifesting or archiving.
  The same pattern exists at the ext4 copy boundary. This finding is recorded
  before implementation; each builder must open the supplied directory once
  with no-follow semantics and traverse only a new held-fd path.
  The first 2026-09-19 focused rootfs run exposed an important `/proc` detail:
  18 cases ran, with 10 failures and 3 errors, because no-follow metadata on
  `/proc/self/fd/N` classified the synthetic root as a symlink and prevented
  child traversal. The source descriptor was held correctly, but root metadata
  must be read through that descriptor while no-follow remains mandatory for
  every child. The ext4 suite did not run after this fail-fast result.
  After correcting that distinction, all 18 focused rootfs cases passed on
  2026-09-19. The new pathname-replacement case changed the public root after
  open and observed the original bytes through the held descriptor while the
  replacement remained unchanged.
  All 9 focused ext4-builder cases also passed on 2026-09-19. Its adversarial
  case replaced the public source after open, then `debugfs` read the original
  `one\n` from the emitted image while the replacement retained
  `replacement\n`; the source fd was explicitly inherited by `cp`.
  A subsequent 2026-09-19 validator audit found two remaining losses before
  implementation: `validate-runtime-root.py` resolves the inherited bundle and
  prints a public path back to its parent, while `validate-runtime-image.py`
  resolves its supplied root before executable inspection. The former must
  preserve a verified inherited-fd anchor; the latter must hold its own
  no-follow root descriptor for the entire validation.
  On 2026-09-19, all 7 focused root-path validation cases passed, including an
  inherited descriptor whose public bundle was replaced while the exact
  `/proc/self/fd/N/rootfs` anchor remained in the result. The image validator
  also passed its ELF, interpreter, wrong-architecture, escape, and held-root
  race cases; the race hashed the original executable and preserved the
  wrong-architecture replacement.
  A 2026-09-19 repetition campaign then passed 100 consecutive public-path
  replacement runs for each of the rootfs scan, real ext4 source copy,
  inherited-bundle validator, and image-entrypoint validator boundaries.
  The first 2026-09-19 full `go test -race -count=1 ./...` invocation did not
  execute tests because Go selected the sandbox's read-only default build
  cache. This environmental setup failure is recorded before rerun and is not
  evidence of a passing or failing runtime test; the gate must use the
  established writable `/tmp/mklinux-gocache`.
  With that cache configured, `go test -race -count=1 ./...` passed across all
  runtime packages on 2026-09-19.
  `go vet ./...` also passed with the writable cache on 2026-09-19.
  `bash scripts/check-docs.sh` passed on 2026-09-19, including 18 rootfs cases,
  9 storage cases, 7 root-path cases, the image held-root race, 55 OCI semantic
  cases, schema/evidence audits, deployment lifecycle, and resource-ledger
  checks.
  The final ordering/evidence audit confirms `prepareRootfs` completes both
  source scans, deterministic storage identity, storage build, generated
  initramfs, and independent initramfs verification before the shim invokes
  `CreateSandbox`. Its retained build result keeps `source_scan_before`,
  `source_scan_after`, image validation, storage identity, generated-bootstrap,
  and verified-bootstrap objects separately. Exact-source caller-snapshot
  evidence `33e1d92e…` independently binds OCI index/amd64 manifest/config/
  layer/diff ID to source-root manifest `65714ed0…` (and Docker active-root
  manifest `e9573a41…`) from an all-zero starting state. Current exact-source
  repro evidence `b32b030e…` separately records generated initramfs and
  manifest digests `35771180…`/`416833e3…`. This satisfies deterministic
  manifest generation, verification-before-allocation, and digest separation.
- [x] Make initramfs generation reproducible, not merely sorted with a
  timestamp-free gzip header. Normalize or deliberately preserve and manifest
  cpio metadata, including runtime-file mtimes, uid/gid, modes, xattrs,
  hardlinks, symlinks, sparse extents, and device policy; prove two builds from
  identical inputs have the same digest.
  A 2026-09-18 publication audit found that the deterministic builder's
  `atomic_write` still used replacing rename semantics. It could overwrite a
  pre-existing archive or manifest and could leave a newly published manifest
  if archive publication subsequently collided or failed. This is recorded
  before implementation; builder outputs must use no-replace publication and
  identity-conditional rollback as one artifact transaction.
  The builder now publishes completed temp inodes with no-replace hard links,
  verifies the installed identity, and uses `renameat2(RENAME_NOREPLACE)`
  quarantine for exact rollback. It publishes the archive first and removes
  only that inode if manifest publication fails. Focused tests preserve
  pre-existing archive and manifest bytes in every collision order, observe no
  orphan peer, and preserve a same-name replacement during rollback. The full
  17-case rootfs-builder suite passes once; repeated execution, docs gates, and
  disposable-host proof remain pending.
  The no-replace transaction and replacement-preserving rollback cases then
  passed 100 repetitions. Full repository and documentation verification
  remain pending.
  Final semantic review moved mode assignment and identity capture onto the
  still-open temp descriptor and made temp-name cleanup identity-conditional
  too. The complete 17-case suite and the two adversarial cases for another
  100 repetitions pass after this refinement.
  The subsequent complete repository race suite, `go vet ./...`, the full
  documentation/schema/evidence/deployment chain, and `git diff --check`
  passed on 2026-09-18. The local no-replace builder-publication checkpoint is
  complete. Exact-commit zero-skip disposable-host execution now proves
  reproducibility and no-replace/rollback publication on ext4; the evidence is
  recorded in the running closure notes below.
- [x] Reject unsafe paths, traversal, escaping symlinks, unsupported file
  types, device nodes, inconsistent hardlinks, malformed metadata, and input
  mutation during the copy/build window.
- [x] Validate OCI image architecture against the selected child-kernel
  manifest before allocation and record required kernel features.
  A 2026-09-19 entrypoint-resolution audit found that holding the top-level
  OCI root is insufficient while `resolve_in_root` still performs child
  `lstat`, `readlink`, and open operations by pathname. A child component can
  be replaced between those calls and redirect traversal outside the root.
  This is recorded before implementation; the kernel must enforce in-root
  resolution and ELF/shebang reads must use the resulting exact descriptor.
  The first descriptor-walker focused run stopped before the new race because
  the test fixture still contained the preceding `/escape` case; the resolver
  correctly rejected it. This harness setup failure is recorded before rerun,
  and the fixture must be reset to `/bin/program` for the child race.
  After resetting it, the focused image suite passed on 2026-09-19. The new
  race replaces `bin` with a symlink to a wrong-architecture executable after
  the original directory is opened; validation hashes the original amd64 file
  through the held child descriptor and leaves the replacement untouched.
  The complete image-validation suite, including held-root and held-child
  replacements, then passed 100 consecutive repetitions on 2026-09-19.
  The subsequent `go test -race -count=1 ./...`, `go vet ./...`, and full
  `bash scripts/check-docs.sh` gate all passed on 2026-09-19; the documentation
  output explicitly includes the held root/child image races.
- [x] Preserve the caller snapshot as containerd-owned input. Mount it with the
  least privileges needed, handle every unmount failure, and prove the runtime
  cannot write through an absolute or relative `root.path`.
  A 2026-09-19 outer-source audit found that `root.path` validation, both
  manifests, image validation, and the archive copy still acquire separate
  root descriptors. A same-content root replacement can therefore change the
  copied inode without changing before/after manifests. This is recorded
  before implementation. The root validator must return its observed
  device/inode, and the outer shell must open one matching directory fd and
  reuse that inherited descriptor for every downstream source consumer.
  A local shell probe confirmed that Bash retains a directory fd across child
  commands and that `/proc/self/fd/N` exposes the held inode and contents.
  The validator now returns its observed path/device/inode as JSON; the shell
  opens that path once, rejects an `fstat` mismatch, and passes the exact fd
  path to image validation, both source manifests, and `cp`. Focused
  2026-09-19 suites passed with 7 root-path cases, 19 rootfs cases, 10 real
  ext4 cases, and the image suite. The shell-handshake case accepts the exact
  identity and rejects a `rootfs` replacement installed after validation.
  The root identity handshake, exact-fd rootfs scan, real ext4 copy, and image
  validation boundaries then passed 100 consecutive repetitions each on
  2026-09-19.
  The subsequent `go test -race -count=1 ./...`, `go vet ./...`, and full
  `bash scripts/check-docs.sh` gate passed on 2026-09-19, including the expanded
  19-case rootfs and 10-case storage suites and all evidence/deployment audits.
  A subsequent 2026-09-19 transaction review found that the post-copy source
  manifest is first built safely as `.after` but then published with replacing
  `mv`. A same-name final artifact can therefore bypass the inner builder's
  no-replace guarantee. This is recorded before implementation; the post-copy
  scan must publish directly to the final no-replace name before comparison.
  The pre-copy manifest now exists only beneath the private `mktemp` workspace;
  the post-copy scan publishes directly to the retained final name, and the
  shell compares them without replacing `mv` or public temporary cleanup.
  On 2026-09-19, shell syntax, the 55-case OCI/transaction suite, and the
  focused rootfs no-replace collision/rollback case passed.
  The subsequent `go test -race -count=1 ./...`, `go vet ./...`, and full
  `bash scripts/check-docs.sh` gate all passed on 2026-09-19.
  Exact-commit disposable-host qualification now closes the remaining live
  clauses. VM race tests prove caller mount sanitization and every mount/build/
  unmount rollback phase, while real ctr-relative and Docker-absolute tasks
  mutate private guest roots without changing their exact caller sources.
  The retained OCI/snapshot/mount/Merkle transcript and independent post-reboot
  zero-resource audit are recorded in the running closure notes below.
- [x] Define and enforce single-owner writable-root identity, generation,
  duplicate-attach prevention, and stale-lock handling rather than relying
  only on one private initramfs per current shim.
- [x] Decide the supported writable-state model. Implement private writable
  layers, read-only bind inputs, persistence/volumes, ownership mapping, and
  propagation semantics, or narrow the G4 plan explicitly if some are outside
  the intended runtime. Read-only bind-input v1 now accepts directories and
  private single-link regular files expressed as standard read-only `bind` or
  `rbind` mounts, including
  Docker's `rbind,rprivate,ro` form, and normalizes them to the stricter guest
  `bind,ro,nodev,nosuid,noexec` policy. The primary rejects writable, shared,
  slave, unknown, protected, overlapping, symlinked, special, or type-mismatched
  destinations. Existing type-matched real objects are replaced only in the
  private staging copy to reproduce bind-mount hiding semantics. The primary
  manifests each source before/after archive-semantic materialization, compares
  the copy, and retains a daemon-verified digest-bearing manifest. Numeric
  UID/GID and admitted metadata are preserved, propagation is explicitly
  absent, original host paths are removed from the guest projection, and the
  agent self-binds the materialized directory or file read-only before process
  creation. Focused tests cover copy identity, destination replacement/type
  rejection, symlinks, mutation, malformed provenance, and guest fail-closed
  parsing. Writable host volumes, configured persistence, and privileged live
  read-only enforcement remain open.
  A 2026-09-19 bind-materialization audit found that source validation and the
  pre-copy manifest close before `cp` reopens the public pathname, and the
  post-copy manifest opens it yet again. A same-content directory replacement
  could therefore change the copied inode without changing either manifest.
  The private target root is likewise retained only by pathname. This is
  recorded before implementation; source and target must be opened
  component-by-component without following symlinks and held across admission,
  copy, and verification.
  The first 2026-09-19 focused run then failed the regular-file bind case:
  `cp --archive` preserved the `/proc/self/fd/N` magic link itself, and copied
  target validation rejected that symlink. Directory sources were unaffected.
  This is recorded before correction; regular sources must dereference only
  the fd magic-link boundary while directory-tree symlinks remain preserved.
  The next focused run rejected the symlinked source as intended but failed its
  diagnostic assertion because the component walker exposed raw `ELOOP` text.
  This is recorded before correction; the no-follow rejection remains and the
  established stable diagnostic must be restored.
  After regular-fd dereference and diagnostic correction, the focused bind
  suite passed on 2026-09-19. A source replacement immediately before `cp`
  still copied `original\n` from the held inode and preserved the substitute;
  a target-root replacement before destination creation wrote only into the
  held original root and preserved the marked replacement.
  The complete bind-materialization suite, including both descriptor races,
  then passed 100 consecutive repetitions on 2026-09-19.
  The subsequent `go test -race -count=1 ./...`, `go vet ./...`, and
  `bash scripts/check-docs.sh` all passed on 2026-09-19; the documentation gate
  explicitly reports held source/target races alongside the complete builder,
  validator, evidence, deployment, and resource-ledger suites.
- [x] Add capacity accounting, block/inode quotas, a high-water refusal policy,
  and bounded behavior for host and initramfs ENOSPC.
  A 2026-09-18 storage-builder audit found that the ext4 image and metadata
  still use replacing renames after an `exists()` precheck, while failure
  cleanup unconditionally unlinks both public names. A raced pre-existing or
  substituted artifact can therefore be overwritten or deleted. External
  `mke2fs`, `debugfs`, and `e2fsck` also reopen the random temp pathname rather
  than inheriting the exact allocated image descriptor. This finding is
  recorded before implementation; image construction, publication, metadata,
  and rollback need descriptor-bound/no-replace identity semantics.
  Ext4 construction now keeps the allocated image descriptor open and passes
  inherited `/proc/self/fd` references to `mke2fs`, `debugfs`, and `e2fsck`;
  the debugfs command stream is anonymous and rewound before inheritance.
  Image and metadata use the shared no-replace publisher, and failure removes
  only identities created by that attempt. Focused races preserve a substituted
  image or metadata file and roll back the paired owned image when metadata
  collides. The 17 rootfs cases, 7 storage cases, and deployment lifecycle test
  pass once; the helper is now an immutable deployed asset. Repeated and full
  checks remain pending.
  The real ext4 raced-image/raced-metadata transaction then passed 100
  repetitions. Full repository and documentation verification remain pending.
  The subsequent complete repository race suite, `go vet ./...`, Python
  compilation, the full documentation/schema/evidence/deployment chain, and
  `git diff --check` passed on 2026-09-18. The local descriptor-bound ext4 and
  exact-publication checkpoint is complete; the G4 row remains open for its
  broader ENOSPC matrix and disposable-host evidence.
  A follow-up staging audit found that the private clone still flowed through
  its random public pathname and final `shutil.rmtree(staging)`. A same-name
  directory replacement could redirect copy/normalization/import or be
  recursively deleted during cleanup even though the image descriptor itself
  was safe. This is recorded before implementation; all staging consumers and
  exact cleanup must share one held staging-directory inode.
  The storage builder now opens the newly created staging directory once,
  creates its root relative to that descriptor, and uses `/proc/self/fd` plus
  descriptor inheritance for copy, normalization, inode accounting, and
  `mke2fs` import. Cleanup first moves only the exact public staging inode into
  a no-replace quarantine. A focused replacement test moves the original,
  installs a marked substitute, proves construction continues only from the
  held original, then requires cleanup to fail closed, roll back the published
  image/metadata, and preserve the substitute. The 8-case suite and this real
  ext4 boundary for 100 repetitions pass after the final transaction review.
  Full verification remains pending.
  The subsequent complete repository race suite, `go vet ./...`, the full
  17-case rootfs/8-case storage and documentation/schema/evidence/deployment
  chain, and `git diff --check` passed on 2026-09-18. This completes the local
  staging-identity checkpoint; privileged interruption and ENOSPC evidence
  remain open.
  Commit `97bcae5` adds host free-inode accounting and reserve policy beside
  existing byte high water, fixes exact-temp cleanup for write/fsync/publish
  ENOSPC, and passes deterministic failure injection at every storage-builder
  allocation/publication boundary plus archive/manifest rollback. Exact-source
  disposable-host evidence `g4-capacity-enospc-live-pass.log` then proves byte
  and inode high-water refusal and real kernel block/inode ENOSPC for both
  storage and initramfs builders on constrained filesystems. Each case leaves
  no public or private attempt artifact; bracketing audits have all 19 runtime
  counters zero and four healthy services. The retained mode-0600 transcript
  is 82,525 bytes/SHA-256 `f216d797…`, wrapper exit 0, with an empty credential
  scan. This closes the capacity/accounting row without closing the broader
  interrupted-copy and corruption fault matrix.
- [x] Implement the storage teardown and recovery sequence appropriate to the
  selected persistent backend: quiesce processes, remount read-only, flush,
  disconnect, sync, offline-check, and preserve a diagnosable state on failure.
  A 2026-09-19 storage-backend audit found that `Inspect` validates a public
  image pathname, `Start` later passes that public name to `mkvsock-nbd`, and
  `OfflineCheck` reopens it again. Parent-component replacement can redirect
  each boundary, and a replacement between inspection and start can become the
  exported writable disk. This is recorded before implementation. Inspection,
  server fd 3 plus restart authentication, and offline `e2fsck` must all bind
  exact descriptors opened beneath a held no-symlink parent directory.
  The first 2026-09-19 focused compile after the durable-identity conversion
  found 8 storage-backend test call sites still expecting the old error-only
  `Inspect` result, so storage tests did not run; `safefile` and lifecycle tests
  passed. This compatibility failure is recorded before updating fixtures to
  assert the returned image identity.
  After signature correction, 6 storage cases failed before their assertions
  because local `t.TempDir()` parents were mode `0775`; the new no-symlink
  opener correctly requires a caller-owned, non-group/other-writable image
  directory. `safefile` and lifecycle remained green. This is recorded before
  aligning the ext4 fixture with production's private `0700` storage root.
  That correction left one fixture failure: the command-start cleanup backend
  retained default required UID 0 and rejected the non-root test image before
  creating its runtime directory. This pre-start rejection is recorded before
  assigning the caller UID so each artifact subtest reaches its intended
  cleanup boundary.
  Storage state is now version 2 and retains the inspected image device/inode;
  only empty version-1 state upgrades, while active legacy or identity-less
  records are rejected. Inspection hashes an exact private fd, server start
  revalidates and inherits that inode as fd 3, recovery authenticates the
  process's fd 3, and offline `e2fsck` inherits another exact fd. Public parent
  identity is rechecked after each operation. The race-enabled storage,
  `safefile`, and lifecycle packages passed on 2026-09-19, including real ext4
  parent replacement at inspect/start/offline-check, process-record inode
  mismatch, and reconciliation refusal before restart.
  The real-ext4 inspect/start/offline-check parent-replacement group then
  passed 100 race-detector repetitions in 63.852 seconds on 2026-09-19.
  The subsequent authoritative full repository gate passed on 2026-09-19:
  `go test -race -count=1 ./...` passed every package, including storage in
  3.582 seconds, and `go vet ./...` completed with no diagnostics. The full
  `bash scripts/check-docs.sh` chain then passed, covering links, schemas, 17
  current evidence manifests, 55 OCI semantic cases, bind/rootfs/image race
  suites, release/deployment lifecycle, resource-ledger, command-capture,
  containerd, and final-evidence audits; `git diff --check` was also clean.
  A post-gate ownership review then found a remaining lock-transfer defect:
  `Start` releases the inspection `flock` before `mkvsock-nbd` opens and locks
  fd 3, leaving a second-owner/content-mutation window, while `OfflineCheck`
  does not lock the image at all. This is recorded before correction. Server
  launch must retain one open-file-description lock across exec, and the
  complete offline check must hold its own exclusive lock.
  The server now duplicates inherited fd 3 instead of reopening it, preserving
  the locked open file description from validation through serving; offline
  checking locks its exact fd for the complete command. The production C
  server passed `-Wall -Wextra -Werror` syntax compilation, and race-enabled
  storage, `safefile`, and lifecycle packages passed. Direct contention tests
  prove the lock is unavailable while either server or checker owns it and is
  available again after exact server stop or checker exit. The combined
  server handoff/recovery and offline-check locking/bounds group then passed
  100 race-detector repetitions in 58.265 seconds on 2026-09-19. Full-tree
  verification remains pending for this correction.
  The next full Go race suite passed, but its combined gate harness named
  `tools/mkvsock-nbd.c` while running from `runtime/`; that nonexistent path
  made C compilation fail before `go vet` was attempted. This command-path
  failure is recorded before rerunning the skipped checks from the correct
  location.
  The corrected production C compilation and `go vet ./...` then passed with
  no diagnostics. Together with the immediately preceding all-package race
  pass (storage: 3.571 seconds), the full code gate is complete; documentation
  and repository-integrity verification remain pending. The subsequent full
  `bash scripts/check-docs.sh` chain passed across links, schemas, evidence,
  OCI, bind/rootfs/image races, release/deployment, resource-ledger,
  command-capture, containerd, and final-evidence audits; `git diff --check`
  was clean. This completes the local exact-image identity and continuous-lock
  checkpoint. Disposable-host proof remains unauthorized.
  The next recovery audit found that process authentication still opens
  `/proc/<pid>/stat`, `cmdline`, and `fd/3` independently and teardown then
  signals the numeric PID/process group. Exit plus PID reuse between those
  operations can cross identities. This is recorded before implementation;
  recovery must retain one pidfd for exact liveness/signaling and one held
  `/proc/<pid>` directory for all metadata inspection.
  The first pidfd-backed implementation passed the race-enabled storage suite,
  including daemon-restart adoption and exact stop. Its review found an
  adjacent executable-identity gap: launch still resolves the server binary by
  pathname and recovery trusts argv without authenticating `/proc/<pid>/exe`.
  This is recorded before extending the durable process record and exact
  recovery check to the executable device/inode as well.
  The first executable-binding run then stopped at three fixture assumptions:
  two compiled fake-server parents inherited mode `0775` and were correctly
  rejected as group-writable, while the missing-binary cleanup case now fails
  before runtime-directory creation but still assumed that directory existed.
  `safefile` passed; these fixture-boundary failures are recorded before
  aligning the tests with the new pre-launch trust boundary.
  After fixing the parents, the compiler's output was observed as mode `0775`
  and was correctly rejected too; production installs the helper as `0755`, so
  the compiled fixtures must explicitly reproduce that deployed mode.
  With fixture modes aligned, race-enabled storage and `safefile` suites pass.
  Launch now executes a held server fd, process-record version 3 binds both
  image and executable inodes, recovery reads stat/cmdline/fd 3/exe through one
  held proc directory, and teardown signals only the retained pidfd.
  Adversarial executable-replacement and repeated verification remain pending.
  The first adversarial executable-replacement test did not compile because
  its new `bytes.Equal` assertion omitted the `bytes` import; no test executed.
  This harness error is recorded before adding the import and rerunning it.
  After restoring the import, the race-enabled executable-replacement,
  recovered-pidfd stop, managed stop, and startup-cleanup group passed. The
  held original reaches the post-exec identity check, the unsafe substitute is
  preserved, launch fails closed, and no runtime artifact remains. Repeated
  and full-tree verification remain pending.
  A follow-up recovery review found that the boolean matcher still collapses
  “process absent” and “same recorded process conflicts with executable/image
  metadata.” `Observe` would remove the record in both cases and could abandon
  a live server after executable-path replacement. This is recorded before
  separating proven absence/PID reuse from a diagnosable live conflict.
  The split now passes focused race testing. A forged executable inode returns
  a specific conflict and preserves the process record byte-for-byte; after
  restoring it, replacement of the public binary pathname does not break
  adoption because `/proc/<pid>/exe` still matches the durably recorded
  launched inode, and exact pidfd stop succeeds. Repeated verification remains
  pending. The combined executable-substitution and recovered-process identity
  group then passed 100 race-detector repetitions in 55.082 seconds on
  2026-09-19.
  The expanded race-enabled storage, `safefile`, and lifecycle gate then passed.
  Direct trusted-executable tests retain the original inode after pathname
  replacement and reject non-executable, group-writable, and symlink inputs.
  The subsequent all-package race suite passed (storage: 3.792 seconds), and
  `go vet ./...` completed without diagnostics. Documentation and
  repository-integrity verification remain pending for this checkpoint. The
  full `bash scripts/check-docs.sh` chain then passed across links, schemas,
  evidence, 55 OCI cases, bind/rootfs/image races, release/deployment,
  resource-ledger, command-capture, containerd, and final-evidence audits;
  `git diff --check` was clean. This completes the local exact-process recovery
  checkpoint; disposable-host proof remains unauthorized.
  Pre-commit error-path review then found that `pidfd_open` and signal-0
  failures other than `ESRCH` were treated as absence, so descriptor exhaustion
  or permission/kernel errors could allow stale-record removal. This is
  recorded before correction: only `ESRCH` may prove absence; every other
  pidfd/proc failure must preserve state and surface an error.
  After restricting absence to `ESRCH`, race-enabled storage, `safefile`, and
  lifecycle suites passed. Other pidfd/proc errors now preserve the process
  record and return a diagnostic failure. The final all-package race run then
  passed (storage: 3.775 seconds), and `go vet ./...` was clean. The final
  documentation chain then passed across links, schemas, evidence, OCI,
  bind/rootfs/image races, release/deployment, resource-ledger,
  command-capture, containerd, and final-evidence audits; `git diff --check`
  was clean. The local pidfd/executable checkpoint is complete.
  The next offline-check audit found that the image is descriptor-bound but
  `CheckBinary` is still executed by public pathname; a substituted checker
  could return success and falsely certify a bad filesystem. This is recorded
  before implementation. The deployed checker contract is a regular `0755`,
  single-link executable and must use the same held-executable boundary.
  `OfflineCheck` now revalidates the exact image name, executes a held checker
  as fd 4 while the image remains fd 3 and exclusively locked, and rejects and
  preserves a public checker substitute after execution. The focused
  race-enabled parent-replacement and complete bounded-check matrix passed.
  The complete offline-check matrix then passed 100 race-detector repetitions
  in 27.721 seconds on 2026-09-19, covering checker substitution, exclusive
  locking, timeout/descendant cleanup, output bounds, evidence hashing, and
  secret-safe failure. Full-tree verification remains pending.
  The first full race run then found one environment-specific fixture failure:
  the integrated graceful-stop case used `/usr/sbin/e2fsck`, exposed here as
  UID 65534 while the test runs as UID 1000, so the new caller-owned check
  rejected it; all other packages passed and `go vet` was skipped. This is
  recorded before copying the real checker bytes into the fixture's private
  caller-owned directory and rerunning the full gate.
  The integrated graceful-stop/offline-check case now passes with an exact
  byte copy of the real `e2fsck` in its private caller-owned fixture directory,
  retaining real checker behavior while matching the production trust
  contract. The corrected all-package race suite then passed (storage: 3.780
  seconds), and `go vet ./...` completed without diagnostics. Documentation
  and repository-integrity verification remain pending. The full documentation
  chain then passed across links, schemas, evidence, OCI, bind/rootfs/image
  races, release/deployment, resource-ledger, command-capture, containerd, and
  final-evidence audits; `git diff --check` was clean. This completes the local
  exact-checker checkpoint; disposable-host proof remains unauthorized.
  The next guest-teardown audit found that the agent syncs, remounts `/`
  read-only, syncs again, and powers off, but never issues `NBD_DISCONNECT`;
  the helper supports it, yet the runtime shutdown path neither calls it nor
  emits ordered disconnect evidence. This is recorded before implementation.
  Disconnect must occur after the authenticated Shutdown reply is delivered
  and immediately before poweroff, using a no-follow, exact `/dev/nbd0`
  block-device check and fail-closed behavior.
  The first shutdown test compile found that this platform's `Stat_t.Rdev` is
  `uint64`, while two fixture assignments used `int64`; mk-agent tests did not
  run, and the independent agent suite passed. This harness type error is
  recorded before correcting the fixture assignments.
  After correction, race-enabled mk-agent and agent suites passed. Tests prove
  exact `sync -> remount-ro -> sync -> NBD disconnect -> poweroff` ordering,
  no-follow `/dev/nbd0` block major/minor validation, ordered PASS evidence,
  and that disconnect failure emits FAIL evidence and prevents poweroff.
  Header verification, repetition, and full-tree gates remain pending.
  The first ioctl header probe failed to link because `<linux/nbd.h>` exposes
  `NBD_DISCONNECT` via `_IO` without including the userspace
  `<sys/ioctl.h>` definition; no probe binary ran. This harness failure is
  recorded before rerunning with the header pair used by the production C
  helper.
  The corrected host-header probe reported `NBD_DISCONNECT = 0xab08`, exactly
  matching the Go implementation. The expanded device/order/failure group
  then passed 100
  race-detector repetitions in 1.021 seconds on 2026-09-19; remount failure
  stops before disconnect/poweroff, and disconnect failure stops before
  poweroff. The complete repository race suite then passed, including
  mk-agent, and `go vet ./...` completed without diagnostics. Documentation
  and repository-integrity verification remain pending. The full documentation
  chain then passed across links, schemas, evidence, OCI, bind/rootfs/image
  races, release/deployment, resource-ledger, command-capture, containerd, and
  final-evidence audits; `git diff --check` was clean. A static deployment-form
  mk-agent build remains to be checked before this local checkpoint closes.
  `CGO_ENABLED=0 go build -trimpath ./cmd/mk-agent` then produced a statically
  linked x86-64 ELF, and its version entrypoint ran successfully. The local
  ordered-disconnect checkpoint is complete; live child/primary proof remains
  unauthorized.
  Final diagnostic review changed the generic failure prefix from `poweroff:`
  to `shutdown:` because a disconnect failure deliberately prevents poweroff;
  retained console evidence now names the failed phase accurately.
  The complete repository race suite and `go vet ./...` passed again after
  that correction. The final documentation/evidence chain and
  `git diff --check` also passed. Local implementation and verification are
  complete; privileged live ordering evidence remains open.
  The following server-sync audit found that production calls final
  `fdatasync` before its close marker, but the marker contains only counters
  and fake servers can emit an indistinguishable line without syncing. This
  evidence-contract gap is recorded before implementation. The canonical close
  record must include `synced=1` emitted only after successful final
  `fdatasync`; recovery must reject every legacy or forged unsynced line.
  The production C helper now emits that field only after final `fdatasync` and
  passes warning-clean compilation. Focused graceful-stop/recovery and
  canonical parser tests pass; `synced=0` and legacy records without the field
  are rejected. The graceful-stop, pidfd-recovery, sync-proof parser, and
  managed-stop group then passed 100 race-detector repetitions in 59.239
  seconds on 2026-09-19. The production C helper, all-package race suite
  (storage: 3.910 seconds), and `go vet ./...` then passed. Documentation and
  repository-integrity verification remain pending. On 2026-09-20, the full
  documentation chain passed across links, schemas, evidence, OCI,
  bind/rootfs/image races, release/deployment, resource-ledger,
  command-capture, containerd, and final-evidence audits; `git diff --check`
  was clean. The local explicit server-sync evidence checkpoint is complete;
  live child/primary proof remains open.
  Exact candidate replay now supplies that proof. Retained mode-0600
  `g4-storage-teardown-live-second.log` is 30,921 bytes/SHA-256 `8e7d93e0…`,
  wrapper exit 0, and has an empty credential scan. Its private console records
  `MK_STORAGE_ROOT_QUIESCE_PASS stages=sync,remount-ro,sync` at line 7 before
  `MK_STORAGE_NBD_DISCONNECT_PASS` at line 9. The exact generation then emits
  canonical terminal `MKNBD_SERVER_CLOSED synced=1` with 281 reads/10,452,992
  bytes, 49 writes/692,224 bytes, and 9 flushes; durable state repeats those
  counters, is `RELEASED`, and records clean offline-check digest `bf24cda1…`.
  Exact process record and backing image are absent. The qualifier returns the
  idle pool and keeps all four services healthy. Independent mode-0600 audit
  `g4-storage-teardown-final-resource-audit-pass.log` is 14,362 bytes/SHA-256
  `49f4e2e2…`, wrapper exit 0, empty credential scan, candidate daemon exact,
  no pool/child, and all 19 resource counters zero. Failure preservation,
  remount/disconnect failure stopping, unsynced/forged-record rejection,
  QUIESCING recovery, exact pidfd/image/checker identities, and offline-check
  bounds remain covered by the focused race tests above. The row is closed.
- [x] Resolve partial-artifact cleanup. Failed `Create` must remove token,
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
  phases. Each record also captures the bundle and configured storage-root
  device/inode/owner before mutation. Replay, service restart, cleanup, and
  reconciliation reject a whole-directory substitution before any backend
  action. Reconciliation rechecks configured path derivation before recursive
  cleanup, and deep-copy tests prevent callers from mutating journal fields by
  alias. Cleanup is descriptor-anchored beneath the identity-bound
  bundle/storage-root inodes; focused pre-open replacement, symlink, and
  post-open rename tests prove a replacement tree is not traversed or removed.
  The identity-bearing disk format is now version 4 and additionally records
  the rootfs mountpoint, runtime directory, and per-task storage-directory
  identities before `MOUNTING` publication. Empty version-1 through version-3
  stores upgrade atomically, while active legacy records that cannot prove
  these identities are rejected rather than guessed. Runtime and storage
  directories are created exclusively and passed to the bounded builder as
  inherited descriptors; build input/output uses `/proc/self/fd` while storage
  metadata and `initramfs.path` retain canonical logical paths. Focused tests
  replace both names during build, prove writes remain on the originals,
  preserve substitute bytes, refuse `PREPARED`, and retain recoverable
  ownership. Prepared replay/reconciliation similarly open the exact recorded
  directories, verify every artifact through held descriptors, and recheck the
  names afterward; a 100-repetition replacement test proves verification reads
  the originals but refuses a concurrently substituted name. Recovery now
  requires the archive, generated/source/read-only-bind manifests, storage
  metadata/image, exact durable build result, and canonical `initramfs.path`.
  Archive and manifest digests must match both generated and independently
  verified build-result sections; focused mutation cases reject each formerly
  omitted artifact. Cleanup also rejects either substitution before backend
  mutation.
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
  Current evidence audit keeps this row open. Mode-0600 VM race transcript
  `g6-oci-postvalidation-rollback-vm-race-pass.log` proves the nine-stage shim
  rollback matrix (prepare rootfs, handoff, token, create/load sandbox,
  network, namespace holder, recovery, and create-event persistence), exact
  ambiguous-create cancellation, tombstoning, and every lifecycle cancellation
  boundary. `g6-oci-preallocation-live-pass.log` proves a real unsupported OCI
  rejection before pool/child/storage/network allocation and subsequent full
  zero inventory. Neither is a live injected post-allocation Create failure at
  every stage. Therefore they substantiate implementation and preallocation
  rejection but do not satisfy the row's explicitly required end-to-end live
  injected failure/no-leak matrix. No closure is claimed.
  The next live boundary is safe to stage: repository adapter fixed paths
  `/opt/mkruntime/bin/kerf-real`, `/opt/mkruntime/bin/kerf-fault-wrapper`, and
  `/run/mkruntime-kerf-fault` are all absent after restart. Active support
  deployment is `b4d185c6…` and host config still selects pinned Kerf directly.
  Injecting a one-shot `load` failure will occur only after real pool, child,
  storage, initramfs, token, and durable Create state exist; successful
  synchronous rollback plus immediate inventory would therefore close the
  missing live post-allocation class. Staging/execution remains pending.
  Focused qualifier `test-runtime-partial-artifact-live.sh` is now implemented
  at SHA-256 `4df7634a…`. It requires a fully clean host, installs a temporary
  managed support generation selecting the repository's one-shot Kerf fault
  adapter, injects `load` failure after real Create preparation/allocation,
  records immediate 20-field inventory, requires a new durable `RELEASED`
  storage record with offline check, and requires pool/child/artifact/network/
  shim convergence. It then atomically reactivates the original support
  generation, removes only the test generation/fixed adapter paths, runs the
  nine-stage shim, six rootfs cleanup/diagnosability, and four lifecycle
  cancellation cases under `-race`, and repeats clean inventory. Its EXIT trap
  restores the original generation even on failure. Bash syntax, ShellCheck
  (when installed), and diff checks pass. No live result is claimed yet.
  Commit `83b1164` freezes only this 206-line qualifier; ledgers and evidence
  remain excluded. Remote transfer/hash verification and execution remain
  pending.
  The first immutable attempt is retained as
  `g4-partial-artifact-live-first-manager-mode-fail.log`, mode 0600, 5,103
  bytes, SHA-256 `6a725b23…`, exit 1. It fails before mutation because the
  qualifier required executable mode on `manage-runtime-deployment.py`, which
  is intentionally mode 0644 and invoked with `python3`. The EXIT trap observes
  no staged deployment/adapter and removes its empty scratch paths. This is a
  qualifier permission-assumption failure and supplies no rollback result.
  The preflight is corrected to require a regular file. The corrected qualifier
  passes Bash syntax, optional ShellCheck, and diff checks and has SHA-256
  `5131c04c97fc9847880f9f384f6bd80f9ddd8d6aa27870a13ddfcef438ff32a2`;
  isolated corrective commit `3ce774e` changes only the preflight predicate.
  The upload at `/var/tmp/test-runtime-partial-artifact-live-3ce774e.sh`
  rehashes exactly to `5131c04c…` on restarted boot
  `95e59482-e5c4-4b4c-a520-b96b1943b088`; the exact source root is present and
  `mkruntimed`, `mknetd`, `containerd`, and Docker are active. Live execution
  is pending.
  A second immutable attempt is retained as
  `g4-partial-artifact-live-second-root-invocation-fail.log`, mode 0600, 4,907
  bytes, SHA-256 `e2db439c9c9868c69ecddf0ef21c1e2bdaa30f6a2b29dde979a2bab588ab58dd`,
  exit 1. The wrapper mistakenly invoked the qualifier through `sudo`; its
  ordinary-user guard rejected this before mutation. The trap again records
  `restore_required=0`, no staged deployment, and removal only of empty
  scratch paths. This is an invocation failure and supplies no rollback
  result; the exact committed script must run as the SSH user.
  A third immutable attempt is retained as
  `g4-partial-artifact-live-third-source-access-fail.log`, mode 0600, 4,801
  bytes, SHA-256 `a247ed1cd4f5120020eeeccceb0a81b66796740ef50d7797dde017a30b75089e`,
  exit 1. Direct ordinary-user invocation passes the UID guard but cannot
  traverse the exact extraction because `/var/tmp/mksrc-e396511-build` is
  `root:root` mode 0700 (its `runtime` child and manager are mode 0775). This
  also stops before mutation with `restore_required=0`; it is source staging,
  not rollback, evidence.
  A complete permission audit finds only the extraction root nontraversable
  and no non-world-readable source file. Only that directory is changed from
  0700 to 0755; ordinary-user manager/runtime access then passes and the
  uploaded qualifier still rehashes to `5131c04c…`.
  A fourth immutable attempt is retained as
  `g4-partial-artifact-live-fourth-socket-readiness-fail.log`, mode 0600,
  30,100 bytes, SHA-256
  `e9907b682a71e38d77faf2e280e6cffc0a47831ae9b3fe171f86c6f47cd594b8`,
  exit 1. It proves clean 20-field pre-inventory and stages temporary managed
  deployment `0e1cbc45…`, but the active-only restart wait returns before
  `/run/mkruntimed.sock` exists. `ctr` consequently fails at preallocation
  connection and leaves the one-shot control unconsumed, so this is not the
  intended post-allocation result. The trap restores `b4d185c6…`, direct Kerf,
  removes all test paths/deployment, and the subsequent audit finds the socket
  ready, all four services active, and zero `mkruntimed` restarts. The
  qualifier must wait for both service-active and socket-ready after each
  managed restart.
  That bounded readiness helper is now applied to fault activation, normal
  restoration, and trap restoration. Bash syntax, optional ShellCheck, and
  diff checks pass at corrected SHA-256
  `a64faea1db586fcc31a369abd5f9e31dfa95cc95f25c1101f73624fb38666e56`;
  isolated commit `cf5d0d8` contains only this readiness correction. Transfer
  at commit-specific path rehashes exactly to `a64faea1…`; immediately before
  retry, original deployment `b4d185c6…`, direct Kerf, socket readiness, and
  absence of every fixed test path are reconfirmed.
  The fifth immutable attempt is the first substantive post-allocation result
  and is retained as
  `g4-partial-artifact-live-fifth-postallocation-leak-fail.log`, mode 0600,
  50,706 bytes, SHA-256
  `bba1b94c161c0b2c7a7de35d24c43396fdc288840b2938b28c940e64fc75ec2c`,
  exit 1. The one-shot `load` control is consumed and `ctr` returns the intended
  `BACKEND_FAILURE`. Immediate inventory proves zero child, mount, runtime,
  bundle, FIFO, network, container, rootfs-record, live-export, NBD, and relay
  residue, but finds two transient shim processes and—critically—Kerf's empty
  memory pool remains configured. The row stays open. Trap restoration removes
  the shims/test deployment and restarting the original daemon releases the
  pool; all services return active and export history advances 190 to 191.
  Code inspection identifies the durable cause: after `CreateSandbox` succeeds
  and `LoadSandbox` (or any later Create stage) fails, shim rollback calls
  ordinary `DeleteSandbox`; that path intentionally retains an idle pool.
  `CancelCreateSandbox` already authenticates the original Create journal,
  accepts `CREATED`, deletes storage/backend state, aborts the Create, and
  releases the zero-owner first pool. Post-Create rollback must use that
  cancellation owner, while allowing containerd's shim disconnect a bounded
  convergence interval before the final process assertion.
  The implementation now carries the exact sandbox config/idempotency key into
  deferred rollback and uses `CancelCreateSandbox` for every attempted
  lifecycle allocation; ordinary `DeleteSandbox` is no longer expected in any
  of the nine post-validation failure stages. Successful cancellation alone
  authorizes rootfs cleanup and clears shim sandbox state. The live qualifier
  preserves its immediate inventory and adds a bounded 30-second shim-process
  convergence wait before the strict final clean assertion. Gofmt, Bash
  syntax, optional ShellCheck, and repository diff checks pass. Focused race
  tests for the nine-stage shim matrix, ambiguous cancellation, and four
  lifecycle pool/cancellation cases pass locally; the initial cache-denied
  invocation is environmental and the rerun uses task-specific `/tmp` caches.
  The updated qualifier SHA-256 is
  `7ca683d2f8442ebd0582bb56a3ce0783ce6d6522aafb46d8fbab53a442184cd6`.
  Full race-enabled shim and lifecycle package suites also pass locally
  (`9.559s` and `1.851s` respectively). Commit and disposable-host rebuild/
  execution remain pending; no closure is claimed.
  Commit `a32e4dd` freezes exactly the runtime rollback, matrix expectation,
  and qualifier changes (21 insertions/12 deletions across three files). Its
  exact tracked-source archive is 4,812,800 bytes with SHA-256
  `d4d7e48cf64555a2d71c92d0995a55e8ef04c5e16ab302dddd2cd0cb465dc988`;
  authoritative revision is `a32e4dde49077d4a0b0ee6cdecf8ae362e7741f6`.
  Ledgers/evidence remain outside the commit. Guest transfer/build are pending.
  The guest independently matches archive SHA-256 `d4d7e48c…`, byte size
  4,812,800, and verifies unique build path `/var/tmp/mksrc-a32e4dd-build`
  absent before extraction. Kerf is clean with no configured pool. Extraction
  and build remain pending.
  Immutable build evidence `g4-partial-artifact-build-a32e4dd.log` is mode
  0600, 5,429 bytes, SHA-256 `6d5981e98749c397cd61a2d81f1440486a585c84cacb71d1539eafa32e110dcd`,
  exit 0, with empty credential scan. It independently rechecks the archive,
  extracts root-owned source, builds the complete static release stamped
  `a32e4dde49077d4a0b0ee6cdecf8ae362e7741f6`, and publishes manifest SHA
  `aed3b718…`. Exact mkruntimed/shim/mknetd/agent hashes are `5dde0c70…`,
  `2ff97961…`, `d99acb5e…`, and `87b33ee3…`. Installation/execution remain
  unclaimed.
  The first activation capture installs/selects
  `0.1.0-dev-a32e4dde49077d4a0b0ee6cdecf8ae362e7741f6` and restarts both runtime
  daemons; all four services report active with zero restarts. It then exits 1
  at an immediate socket assertion before identity checks, repeating the
  already-localized systemd-active/socket-ready race. Retained
  `g4-partial-artifact-activate-a32e4dd-socket-race-fail.log` is mode 0600,
  2,188 bytes, SHA-256 `5ac4512f4c5b8f721d9084108376b40d21144cddda5e09d67933d5c89944719a`.
  Selection/restart are real mutations; coherent running identities remain
  unclaimed pending a bounded replacement capture.
  The bounded replacement confirms socket readiness, four active/zero-restart
  services, selector and public shim link at `a32e4dd`, then exits 127 because
  it assumes nonexistent `/usr/local/bin/mkruntimed`. Retained
  `g4-partial-artifact-activate-a32e4dd-public-path-fail.log` is mode 0600,
  2,328 bytes, SHA-256 `3ffe88ac1a3cd8ece3c3299ec2411817fcb7d7089e6ff029f18eec4f59cc7660`.
  Read-only resolution finds systemd uses `/usr/local/sbin/mkruntimed`, PID
  9967 resolves to the candidate immutable release, and `/proc/9967/exe`
  reports exact revision `a32e4dd…`. Hash/Kerf capture still remains.
  Corrected activation evidence
  `g4-partial-artifact-activate-a32e4dd-pass.log` is mode 0600, 3,956 bytes,
  SHA-256 `4efa82b1035764a260580ec442493c86000f354defd5c5d3c2c3c74ca56b42bd`,
  exit 0, with empty credential scan. It proves socket readiness, all four
  services active with zero restarts, immutable selector and shim link at
  `a32e4dd…`, exact running/public mkruntimed SHA `5dde0c70…`, exact shim SHA
  `2ff97961…`, both version reports at the full revision, and no Kerf pool or
  instance. Candidate live replay is now authorized but not yet claimed.
  As with the preceding root-owned extraction, only the new source-root mode
  is changed 0700 to 0755 for ordinary-user traversal. Its embedded qualifier
  rehashes exactly to `7ca683d2…`; manager/wrapper access passes and all fixed
  adapter/control paths are absent immediately before replay.
  Repaired immutable evidence `g4-partial-artifact-live-pass.log` is mode
  0600, 94,007 bytes, SHA-256
  `4e66461fede39dd34ac58565b49d4912c9f658c4acf3bc352f87269bf9120f04`,
  exit 0, with empty credential scan and terminal
  `G4_PARTIAL_ARTIFACT_LIVE_PASS`. Provenance binds restarted boot
  `95e59482…`, candidate selector/mkruntimed `a32e4dd…`/`5dde0c70…`, original
  support generation `b4d185c6…`, and qualifier `7ca683d2…`. The consumed
  one-shot `load` fault returns `BACKEND_FAILURE`. Its immediate snapshot has
  zero child/mount/artifact/FIFO/network/container/rootfs/storage/NBD/relay
  residue (only the containerd shim supervisor/worker are disconnecting); 20
  quarter-second samples converge those processes to zero. The strict clean
  checkpoint then proves all 20 counters zero plus no pool/instance. Storage
  history advances exactly 191 to 192 with new `RELEASED` record, zero I/O
  counters, and offline-check SHA `5648b3d2…`. The VM race matrix passes all
  nine shim stages, six rootfs cleanup/diagnosability cases, and four lifecycle
  cancellation/pool cases, followed by another all-zero/no-pool checkpoint
  and four active zero-restart services. Independent resource audit remains
  pending, so the row is not yet closed.
  Independent audit script SHA `bc5e2f7d…` re-verifies the result in
  `g4-partial-artifact-final-resource-audit-pass.log`, mode 0600, 14,185
  bytes, SHA-256 `545ad9ea7b748a861ba1c2ba2fde8d690e49b38b7f879913dc1cdcda25d2d284`,
  exit 0, empty credential scan. Its independent candidate provenance binds
  boot `95e59482…`, selector `a32e4dd…`, running mkruntimed SHA `5dde0c70…`,
  proves no Kerf pool/instance, all 19 audit counters zero, and all four
  services active/running with zero restarts. Together with the live injected
  post-allocation pass and the focused failure matrices, this closes the row.
  The post-closure full documentation/runtime gate and repository diff check
  pass. Generated bytecode is removed; checklist totals are now 49 closed and
  36 open.

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
- [x] Read-only input rejection, private-write isolation, configured
  persistence, and proof that unconfigured writes do not persist. V1 now
  explicitly has no configured persistence or writable-volume API: such bind
  and propagation requests fail before allocation. Local tests cover bind
  admission/materialization, numeric ownership, and UID/GID-mapping rejection;
  retained live evidence covers ctr/Docker read-only guest write rejection and
  no host write-through, private guest writes without caller-source mutation,
  same-name delete/recreate non-persistence, and zero-live-state rejection of
  generic writable binds and shared propagation.
- [x] Block and inode exhaustion, high-water refusal, wrong UUID/generation,
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
  timeout rather than waiting for a server that exits only on `SIGTERM`. The
  backend now holds and identity-binds the private runtime directory and uses
  its descriptor for process-record publication, record/log reads, readiness,
  restart observation, stop, and removal. Publication is no-replace; unsafe
  stale logs and record collisions preserve prior artifacts; and preownership
  process-start failures reap the child and remove their log. Focused tests
  prove a whole-directory replacement is rejected and untouched, hostile stale
  paths are not followed, and a missing server binary leaves no artifacts.
  Offline `e2fsck` now shares the bounded process-group runner with rootfs
  builds: a five-minute default, caller cancellation, one-MiB combined-output
  retention, descendant termination, and secret-safe errors are enforced.
  Focused tests cover timeout with a background child, overflow, stderr
  evidence hashing, and a non-clean exit. The ext4 builder now rejects a
  negative or unreasonably large free-space reserve so the high-water policy
  cannot be disabled through arithmetic, and a real 256-file/128-inode build
  proves inode exhaustion fails boundedly with no image, metadata, or staging
  residue. A separate fully allocated 63-MiB source into the minimum 64-MiB
  ext4 quota proves block exhaustion follows the same no-residue failure path.
  Rootfs and storage ownership stores now create and walk their state
  directories component-by-component without following symlinks, bind the
  opened directory device/inode for the lifetime of the store, read private
  state relative to that descriptor, and publish synchronized replacements
  with `openat`/`renameat`. Focused post-open directory-replacement tests prove
  a substituted pathname is rejected without receiving durable state.
  Live fault evidence remains open.
  A 2026-09-19 outer-transaction audit found that
  `build-runtime-container-initramfs.sh` still unconditionally removes its
  public output names from the failure trap. That can erase a same-name
  replacement after an inner no-replace builder succeeds and a later step
  fails. This is recorded before implementation; the trap must either remove
  only exact identities it owns or defer to the backend's descriptor-anchored
  directory cleanup.
  The outer shell now removes only its private `mktemp` workspace. Production
  failure cleanup remains with the service, which had already recorded and
  quarantines the exact private runtime/storage directory identities. A
  focused 2026-09-19 early-failure run pre-populated all 8 former public
  cleanup targets and proved every replacement byte remained unchanged while
  all 55 OCI semantic cases and existing namespace/file-identity boundaries
  passed.
  After the outer-cleanup change, `bash -n`, the focused OCI/cleanup suite,
  `git diff --check`, `go test -race -count=1 ./...`, and `go vet ./...` all
  passed on 2026-09-19; the full documentation gate follows separately.
  `bash scripts/check-docs.sh` then passed on 2026-09-19 with the outer-cleanup
  boundary explicitly included, plus the complete builder, validator,
  evidence, deployment, and resource-ledger suites.
  Exact-source evidence now completes the matrix. The mode-0600
  `g4-storage-fault-matrix-live-pass.log` (86,309 bytes, SHA-256
  `1eca1db8…`) explicitly passes a real SIGKILL-interrupted staging copy,
  pristine/current ext4 wrong-UUID rejection, quota/clean-state inspection,
  generation-bound single ownership, conflicting-generation refusal, every
  injected builder allocation/publication boundary, and real block/inode
  exhaustion plus byte/inode high-water refusal. Earlier mode-0600
  `g4-single-owner-live-pass.log` (`5790e421…`) supplies durable stale-
  generation/duplicate-claim rejection, live duplicate-lock contention, and
  exact release. Bracketing audits are all-zero, services remain healthy, and
  credential scans are empty. This row closes without implying the separate
  server-loss, corruption, and recovery matrix below.
- [x] Server loss during read, write, and flush; primary daemon restart;
  primary host reset where durability is claimed; corrupted image; clean and
  dirty recovery; snapshot/clone recovery using disposable copies.
  Initial next-row audit keeps this open. Existing exact live evidence proves
  mkruntimed restart with generation/PID/start-time/argv/image/binary-bound NBD
  helper survival/adoption, subsequent graceful close with nonzero read/write/
  flush counters, recovery of an exact already-exited helper from canonical
  `READY` plus `CLOSED synced=1`, offline `e2fsck`, and eventual clean deletion.
  Focused storage tests already reject wrong UUID, dirty ext4 state, pristine
  digest change, image inode replacement, unauthenticated/malformed close
  evidence, and QUIESCING recovery without graceful-close proof. They also
  restart a missing exact `ACTIVE` export and reject a conflicting generation.
  None of this injects server death specifically while a read, write, or flush
  is outstanding; performs a primary-host reset; or proves clean/dirty and
  corruption handling on disposable image clones. V1 explicitly declares the
  private writable root non-persistent, so no workload-data durability across
  host reset is claimed; reset evidence must instead prove fail-closed durable
  ownership/reconciliation and resource cleanup. A focused disposable-copy
  qualifier and an explicit accepted-client server-loss policy are still
  required before closure.
  Code audit then found that startup reconciliation restarted every absent
  `ACTIVE` helper, including one whose exact log proved a child had already
  connected. Since the child NBD session cannot attach to that replacement,
  this could misreport an orphan listener as recovery. The backend now reports
  `ClientLost` only when exact generation-bound `READY` precedes
  `MKNBD_SERVER_CLIENT_ACCEPTED` without a canonical synced close. It retains
  the dead helper's exact record instead of deleting it. Reconciliation refuses
  automatic restart for that state, while still restarting the same generation
  when loss occurred before client acceptance. The storage contract documents
  this boundary. Focused race tests prove accepted-client death is retained,
  Stop refuses it without graceful-close evidence, reconciliation performs no
  replacement Start, and durable `ACTIVE` ownership is unchanged. The complete
  storage package passes under `-race` in 3.854s. An initial gofmt command used
  repository-relative paths from inside `runtime` and stopped on `lstat`
  before testing; the corrected command supplies the authoritative pass.
  The backend test additionally removes the exact process record and proves
  the retained generation-bound READY→CLIENT_ACCEPTED log alone still reports
  `ClientLost` rather than safe absence. Focused three-case race execution
  passes in 1.022s. The complete runtime tree then passes under `go test -race
  -count=1 ./...` and `go vet ./...`; storage completes in 3.781s within that
  full run. Repository-wide qualification remains next.
  Exact live and clone evidence now closes the row. Disposable clone matrix
  `g4-storage-disposable-clone-matrix-f9971d8-pass.log` (mode 0600/2,277
  bytes/SHA `96b861aa…`) proves byte-identical clean clone acceptance with new
  inode identity, dirty-clone pristine rejection plus bounded offline e2fsck
  diagnosis, explicit repair followed by pristine acceptance, corrupt
  superblock rejection, and unchanged source digest/identity. Live accepted-
  client qualifier `g4-storage-accepted-client-loss-31a2318-redacted.log`
  (mode 0600/74,239 bytes/SHA `ab11a88c…`) binds simultaneous continuous
  reads and one-block fsynced writes to an accepted generation, observes
  substantial server I/O, kills the exact helper with no terminal close,
  retains the exact record/image/durable ACTIVE generation, and proves primary
  daemon restart refuses replacement. Its raw credential-bearing temporary was
  removed after a mechanically redacted, empty-scan derivative was verified.
  The initial host-reset control then proves old code unsafely reopened the
  same image (`c49da816…` plus exact-process proof `a43ea4ff…`), preventing a
  false-positive pass. Commits `ebbb2db…`/`ca7d7d0…` require exact READY-only
  pre-acceptance evidence and remove verbose credential-bearing qualification
  output. Final candidate reset audit
  `g4-storage-host-reset-fix-post-reset-pass-v3.log` (mode 0600/15,346
  bytes/SHA `d895049b…`) proves the selected daemon fails before rootfs
  reconciliation on every attempt, creates no runtime record/log or helper,
  preserves the exact durable ACTIVE state SHA/image inode, and leaves zero ctr
  tasks and zero Kerf pool/instances. V1 still makes no workload-data
  persistence claim: the proven reset behavior is durable fail-closed ownership
  with all ephemeral compute resources returned.

- To prepare subsequent qualification without erasing the diagnosed state,
  exact lifecycle state/journal (`1e493170…`/`05c36775…`), rootfs state
  (`e359f144…`), storage state (`c05b754d…`), mknetd state (`1b40028d…`), and
  2-GiB root image (`437f2a7c…`) are hashed then moved intact into four unique
  root-only mode-0700 quarantine trees bound to boot `f93f21b6…`. Only orphan
  ctr container metadata is removed; new private empty directories are created
  and all four services become active/running with zero restarts. This is an
  administrative recoverable reset, not product cleanup evidence. Mode-0600
  log is 3,532 bytes/SHA `31397961…`, exit 0, empty credential scan.

- Independent exact-source post-reset audit binds selected candidate
  `ca7d7d01…`, live daemon SHA `a4a91006…`, and boot `f93f21b6…`; all 19
  resource counters are zero, Kerf has no pool/instances, and all four services
  are active/running with zero restarts. Terminal marker
  `G6_FINAL_RESOURCE_RETURN_PASS` exits 0. Mode-0600 evidence is 16,301
  bytes/SHA `3aeb7b79…`, empty credential scan. The VM is clean for the next
  storage-isolation qualifier, while all fault inputs remain quarantined.
- [x] Cross-sandbox attempts to mount or address another sandbox's export.
  Qualification is in progress on restarted disposable boot `f93f21b6…` from
  the independently clean, exact `ca7d7d01…` candidate state. Source review
  establishes the distinction from the already-passed sibling IP test: the
  primary NBD server binds its Multikernel AF_VSOCK listener to `CID_ANY`, then
  authenticates a fixed-size hello over image size, image ID, and the random
  export generation before exposing any block request. It accepts only one
  stream and the intended child already occupies that stream. This is useful
  design evidence but is not yet a pass: the row remains open until two live
  sandboxes reciprocally attempt the peer port with wrong and exact identities,
  fail to create or mount `/dev/nbd1`, leave both intended `/dev/nbd0` roots
  healthy, preserve both durable export records/images, and return all primary
  resources under a retained live transcript.
  Restart preflight confirms boot `f93f21b6-21f1-4fc0-aebc-806347d6b43d`,
  exact selected release `0.1.0-dev-ca7d7d01…`, live daemon SHA
  `a4a91006…`, four active services, zero kernel instances, and zero ctr tasks.
  The first ad-hoc preflight exits nonzero only because it assumed
  `/var/lib/mkruntimed/storage/state.json` existed; the independently cleaned
  host has not created that file. This is a harness-only observation, not a
  product failure, and the live qualifier must define missing fresh state as
  zero exports.
  The first retained qualifier run is an environment-capacity failure, not an
  isolation result. It binds the same exact boot/release/daemon, installed and
  injected helper SHA `95e886d6…`, qualifier SHA `66e71807…`, an all-zero
  initial inventory, and passing hello-identity/timeout focused tests. The very
  first child root build then fails closed at the storage high-water check:
  free 2,684,846,080 bytes is below required 3,233,472,512 bytes. No child or
  cross-export attempt runs, so the row remains open. Retained mode-0600 log is
  1,389 bytes/SHA `b3a871b6…`, exit 1, with an empty expanded credential scan.
  Post-failure inspection proves cleanup returned zero kernel instances, ctr
  tasks/containers, and storage/relay helpers; only the expected empty 36-byte
  rootfs state file exists. Capacity is isolated to the 20-GB
  `/srv/multikernel-storage` disk (17,182,076,928 used; 2,684,850,176
  available), while the 100-GB boot disk has 51,274,813,440 bytes free. Four
  retained 2-GiB forensic quarantine images and the prior dual-root experiment
  images account for the data-disk pressure. They will not be deleted; the
  disposable data disk will be expanded before retry.
  GCE identifies `/dev/sdb` exactly as nonboot persistent disk
  `mk-mediated-storage-20260830`; it is a whole-device ext4 filesystem mounted
  `rw,nosuid,nodev,relatime`. The disk is expanded in place from 20 to 30 GB and
  `resize2fs /dev/sdb` grows the mounted filesystem from 20,957,446,144 to
  31,526,436,864 bytes. Used bytes remain exactly 17,182,076,928 while
  available bytes rise from 2,684,850,176 to 12,802,879,488, proving retained
  evidence was preserved rather than deleted. The unchanged qualifier can now
  exercise two concurrent 2-GiB roots.
  The capacity-fixed second run is another harness-only failure. It again
  proves exact provenance, a zero-resource baseline, and both focused C passes;
  it allocates both tasks, but the qualifier equates containerd `RUNNING` with
  guest workload readiness and immediately reads `/tmp/storage-isolation-marker`.
  The marker is not yet present, so it exits before extracting export identity
  or making any peer attempt. Its trap returns zero instances, ctr tasks and
  containers, helpers, rootfs records, and live exports. Retained mode-0600 log
  is 1,291 bytes/SHA `ac8fc9ea…`, exit 1, empty credential scan. A bounded
  per-guest marker wait is required; this is not isolation evidence.
  Readiness-fixed run three reaches two real ready guests and binds distinct
  durable owners at ports 4061/4062. It retains both full 2-GiB image hashes,
  storage/rootfs state hashes, server PIDs/I/O counters, and only SHA-256
  digests of the two bearer export generations. Before its first peer attempt,
  the harness calls `chmod` on a not-yet-created private attack log and exits.
  Thus it provides useful two-owner setup evidence but no isolation result.
  Its trap is independently verified to return zero instances, ctr tasks and
  containers, helpers, rootfs records, and live exports. Mode-0600 transcript
  is 2,254 bytes/SHA `a048dccf…`, exit 1, empty credential scan. The next
  revision must create each private log before setting its mode.
  Private-log-fixed run four reaches the same two-owner state, but all four
  nominal peer attempts exit immediately with status 126, duration 0, and an
  identical 32-byte output SHA `848f2037…`. Although `/dev/nbd1` remains zero
  sectors and each mount fails, those facts follow from failure to execute the
  injected helper and are not transport-isolation evidence. The harness later
  exits without its integrity/final/pass observations. Retained mode-0600 log
  is 3,555 bytes/SHA `9f13ac7a…`, exit 1, empty credential scan. The attempt
  classifier must reject exit 126/unknown failures and the injected helper's
  guest mode/exec path must be diagnosed before retry.
  Source inspection supplies the exact design explanation: the only admitted
  read-only bind option vector is `bind,ro,nodev,nosuid,noexec`, and retained
  metadata explicitly labels the guest policy
  `bind-remount-ro-nodev-nosuid-noexec`. Executing an injected bind payload is
  therefore prohibited, not a viable test technique. This path is paused; a
  later retry must package the helper into a purpose-built OCI test image and
  must not weaken the production noexec policy.
  Independent post-run inspection confirms the paused fourth run returned zero
  kernel instances, ctr tasks, and live exports.
  The resumed qualifier preserves that policy. VM inspection proves the exact
  deployed helper is a statically linked x86-64 ELF, while BusyBox 1.36 is
  already present in both local Docker and containerd stores. The qualifier
  now builds locally with `--pull=false --network=none`, copying the helper
  into `/usr/local/bin` of a purpose-built OCI image, imports the Docker archive
  into containerd, proves the helper's SHA inside both guests, and removes both
  image references during cleanup. Peer attempts execute the in-image path;
  the read-only bind and its `noexec` policy are untouched. It also hashes its
  executing path rather than assuming the source tree copy. Local Bash syntax
  and diff checks pass; ShellCheck is unavailable. Mode/size/SHA-256 are
  `0755`, 14,029 bytes, and
  `5efcdf0dd95445939d0100e69b36ac3d6a3672164f66b393e7a184dc570079cf`.
  This is implementation evidence only until the exact qualifier runs live.
  Guest transfer `/tmp/test-runtime-cross-sandbox-storage-live-5efcdf0d.sh`
  independently matches mode `0755`, size 14,029, full SHA-256
  `5efcdf0dd95445939d0100e69b36ac3d6a3672164f66b393e7a184dc570079cf`,
  and syntax marker `GUEST_STORAGE_ISOLATION_VERIFY_PASS`. No isolation result
  is inferred from transfer verification.
  Fifth retained run stops during local image construction before provenance,
  child creation, or any peer attempt because the VM's legacy Docker builder
  does not implement Dockerfile `COPY --chmod`. The private transcript
  `g4-cross-sandbox-storage-live-fifth.log` is mode `0600`, 921 bytes, SHA-256
  `af6b9cb63405562c93ef601d09fdd751ad0f3d04a9ed5df7ec1e7a9f4ed45ab2`,
  exit 1, credential-pattern clean. Cleanup removes the scratch context and
  any provisional image reference. The Dockerfile now uses portable `COPY`
  followed by `RUN chmod 0755`; this changes only test-image construction and
  the isolation row remains open. Corrected mode/size/SHA-256 are `0755`,
  14,058 bytes, and
  `5c47fe19f64ad0fb8f953beac059108b107d5000378091d91ce9364e1d18b278`;
  local syntax and diff checks pass.
  Guest transfer `/tmp/test-runtime-cross-sandbox-storage-live-5c47fe19.sh`
  matches mode `0755`, size 14,058, and full SHA-256
  `5c47fe19f64ad0fb8f953beac059108b107d5000378091d91ce9364e1d18b278`;
  guest syntax ends `GUEST_STORAGE_ISOLATION_PORTABLE_VERIFY_PASS`.
  Sixth run supplies the first real reciprocal transport evidence but remains
  a harness failure. Exact image construction/import and helper SHA
  `95e886d6…` pass; distinct live owners occupy ports 4061/4062 with distinct
  image and generation digests. All four A→B/B→A wrong/exact attempts execute
  the in-image helper, fail after 15–16 seconds as
  `occupied-listener-no-response`, leave `/dev/nbd1` at zero sectors, and fail
  the peer mount. The run then exits before its integrity marker because it
  requires online whole-image hashes and server I/O counters to remain exact,
  even though its own guest commands create a mount directory and read/write
  the legitimate root. Retained mode-0600 transcript is 4,356 bytes, SHA-256
  `04437a467eb2efe2931b712d0a8625aa8441ed87dd837d0d53ba43ed9f674b58`,
  exit 1, credential-pattern clean. The trap removes all tasks, containers,
  instances, helpers, rootfs/live-export records, and both image references.
  Two immediate strict audits (mode 0600/406 bytes, SHAs `795e316b…` and
  `7186710e…`) exit 1 before observations because the empty 16-GiB Kerf pool
  remains configured; direct inspection confirms allocation zero and no
  instance. The corrected qualifier instead precreates mountpoints, binds an
  owner-specific canary in each legitimate root, requires canary/durable-state/
  process-record/PID identity unchanged, records rather than equates expected
  live image/I/O movement, and restarts idle mkruntimed to release the pool
  before its final audit. The row remains open pending that exact retry.
  Controlled idle release changes mkruntimed PID 1446→11547, leaves it active
  with zero restarts, and returns Kerf to `No memory pool configured`; retained
  mode-0600 evidence is 1,143 bytes, SHA-256
  `1d936a8647a619bf41ea3a20f477f576c21334af683a278a66d5a1d8fdc514bf`,
  exit 0, credential-pattern clean. The following strict audit passes clean
  Kerf, all 19 counters zero, and four healthy services: mode `0600`, 1,909
  bytes, SHA-256
  `d2d3d9882e79824299c3ef5da778069a959f19a94914b1c4dbfd32b529816896`,
  exit 0, credential-pattern clean. The canary/idle-release qualifier passes
  local syntax and diff checks and is now mode `0755`, 15,376 bytes, SHA-256
  `78b5e81e3d30a287f2d3efae456c98b5546f3b92e9ffab81ad6d6965050d405b`.
  Guest upload `/tmp/test-runtime-cross-sandbox-storage-live-78b5e81e.sh`
  independently matches mode `0755`, size 15,376, full SHA-256
  `78b5e81e3d30a287f2d3efae456c98b5546f3b92e9ffab81ad6d6965050d405b`,
  and syntax marker `GUEST_STORAGE_ISOLATION_CANARY_VERIFY_PASS`.
  Seventh run closes the row as `g4-cross-sandbox-storage-live-seventh.log`:
  mode `0600`, 5,934 bytes, SHA-256
  `44f12b5a6cf1f1aade556b9f76055a491a1b7c538056d4772f5957bba9f285e9`,
  exit 0, credential-pattern clean, terminal
  `G4_CROSS_SANDBOX_STORAGE_ISOLATION_PASS`. The purpose-built Docker and
  containerd image IDs match; both guests prove in-image helper hash
  `95e886d6…`. Distinct owners at ports 4061/4062 have distinct image and
  generation digests. All four reciprocal wrong/exact attempts execute for 15
  seconds, fail as `occupied-listener-no-response`, leave `/dev/nbd1` at zero
  sectors, and cannot mount it. Both owner-specific canaries, durable
  storage/rootfs state, process-record hashes, and server PIDs remain exact;
  legitimate own-root I/O advances while both health files remain readable.
  Normal task cleanup plus idle mkruntimed restart changes PID 11547→14106 and
  returns instances/tasks/rootfs/live exports/helpers to zero with no Kerf
  pool. Independent `g4-cross-sandbox-storage-final-resource-audit.log` is mode
  `0600`, 1,909 bytes, SHA-256
  `e6f499cbb9b4fd9b172acf2da55bf35f657a2e0ab3cbec95c1ded9c1a40b110d`,
  exit 0, credential-pattern clean; it confirms clean Kerf, all 19 counters
  zero, and four active zero-restart services.
  Commit `f35de77b4ef159d0f4951032b41f005c27aa4a49` (`test: qualify
  cross-sandbox storage isolation`) checkpoints the exact qualifier and both
  learning records. The checklist now has 59 closed and 26 open rows.

### Replacement instance evidence required

- [x] Record exact OCI index and selected `linux/amd64` manifest digests,
  containerd snapshot identity, source-root mount table, and before/after
  metadata or Merkle digests proving the caller snapshot was unchanged.
  Exact `fe9a2de` evidence binds index `73aaf090…` to amd64 manifest
  `b7f3d86d…`, config `b116e155…`, layer `034d6572…`, diff ID and committed
  snapshot `97e4ece8…`; it records both the read-only committed ext4 projection
  and the exact Docker overlay projection. Independent normalized scans prove
  byte-identical 442-entry committed manifests at `65714ed0…`; Docker's exact
  452-entry active caller root remains `e9573a41…` after private guest writes.
- [x] Record two initramfs builds from the same input with identical manifests
  and digests, plus a changed-input negative control with a different digest.
  Exact-current `0672fa1` evidence records independently built and verified
  archive/manifest pairs that match at `35771180…`/`416833e3…`, then a
  changed-content control that diverges at `3bd844ae…`/`e1408c2a…`. The
  20-case builder suite and qualifier exit 0. Retained private
  `g4-initramfs-repro-current-live-pass.log` is mode 0600/9,171 bytes/SHA-256
  `b32b030e…`; independent all-zero audit
  `g4-initramfs-current-final-resource-audit-pass.log` is mode 0600/15,094
  bytes/SHA-256 `314d0b63…`. Both wrappers exit 0 and their joint credential
  scan is empty.
- [x] Prove `/bin/busybox` and any dynamic libraries are from the OCI root while
  `mk-agent`, bootstrap tools, and the transport module are outside it; retain
  hashes and mount/inode provenance from inside the child.
  Source mapping now fixes the intended provenance before live qualification.
  The root builder installs trusted BusyBox, `mk-agent`, relay, and
  `mk-agent-init` at the outer ext4 root, copies the caller-owned OCI snapshot
  only below `/bundle/rootfs`, and rewrites OCI `root.path` to `rootfs`.
  Bootstrap initramfs separately contains BusyBox, `mkvsock-nbd`, NBD and
  transport modules, and `runtime-mediated-init`; it mounts the ext4 image and
  `switch_root`s to the trusted outer `/init`. `mk-agent` then launches OCI
  processes with chroot `/bundle/rootfs`. Therefore a normal task's
  `/bin/busybox` and interpreter/libraries must resolve inside the OCI subtree,
  while `/mk-agent`, `/mkvsock-relay`, outer `/init`, and bootstrap-only module
  bytes must not be part of that chroot. This is design evidence only; the row
  stays open for child-observed hashes, device/inode/mount identity, and exact
  comparison to retained OCI/bootstrap manifests.
  On the current candidate host, approved `gce-mk2.json` supplies exact trusted
  SHA-256 identities: agent `b57bae66…`, relay `293ff1ea…`, initramfs
  `2dec85b8…`, kernel `5cdf26d0…`, and transport module `bef1b888…`. The selected
  runtime release directory contains host binaries only; child outer-root
  provenance must therefore compare observed bytes to the approved manifest
  and exact builder inputs, not merely to similarly named release files.
  The live deployed builder resolves to immutable deployment
  `b4d185c6…` and selects
  `/usr/local/libexec/multikernel/guest/mk-agent-init`; that file and the exact
  candidate-source copy both hash to `edc9284c…`. The provenance qualifier can
  therefore compare outer `/init` to a concrete deployed build input.
  The first retained provenance run is an explicit false positive despite its
  terminal `PASS` marker. Its `cleanup` function executes `set +e` in the
  parent shell, permanently disabling later assertion exits. Primary reads of
  root-owned containerd bundle/runtime files fail with `EACCES`; the workload
  also correctly receives `EACCES` for `/proc/1/root/...`; empty captured
  values then compare equal and disabled assertions allow completion. No
  provenance claim is accepted. Renamed mode-0600 transcript
  `g4-root-provenance-live-first-false-pass.log` is 3,436 bytes/SHA
  `2da0a7b4…`, wrapper exit 0, empty credential scan. Useful product evidence
  is limited to outer proc-root denial and a genuine all-zero final inventory.
  The harness must isolate cleanup options, use privileged primary reads,
  require nonempty values, and assert proc-root denial as intended isolation.
  Corrected run two restores fail-closed assertions and exits 1 without a pass
  marker. It proves exact clean baseline, boot/release/daemon/manifest and
  qualifier SHA `70c51fa…`, plus on-disk equality for all approved trusted
  artifacts, then exits before any live-image or child-provenance observation.
  Redirection hides the failing command, so no provenance claim is made.
  Independent inspection confirms zero instances, tasks, containers, helpers,
  rootfs records, and live exports. Mode-0600 transcript is 1,890 bytes/SHA
  `5cf0b81a…`, exit 1, empty credential scan. The next harness revision must
  print its failing line/status and let `debugfs` create private dump targets
  rather than precreating files it may refuse to overwrite.
  Diagnostic run three remains fail closed and repeats only the exact baseline
  and approved-artifact observations before exit 1. Its new EXIT hook emits
  `line=1`, which is not actionable and does not identify the failed command;
  there is again no live-image/child observation or pass marker. Mode-0600 log
  is 1,945 bytes/SHA `16581e66…`, empty credential scan. The next action is a
  credential-free shell trace of this same harness to locate the failure, not a
  claim or unchecked retry.
  Retained trace localizes the failure to privileged `test -f` of
  `<containerd-bundle>/rootfs/bin/busybox`: the snapshot is mounted only in the
  shim's private mount namespace, so even primary root correctly sees no file
  at that pathname. The trace is mode 0600/13,166 bytes/SHA `f43d49d0…`, exit
  1, empty credential scan, and confirms cleanup. The builder's SHA-bound
  `initramfs.source-manifest.json` already records the held snapshot entry
  `bin/busybox` with type, mode, ownership, size, and content SHA; that is the
  correct OCI comparison source. The qualifier must parse it rather than cross
  the shim namespace.
  Manifest-based run four supplies strong partial product evidence but exits 1
  at its mount-string assertion. Read-only inspection of the exact live ext4
  image records outer agent inode 59/hash `b57bae66…`, relay `293ff1ea…`, init
  `edc9284c…`, OCI BusyBox inode 21/hash `f060103f…`, and OCI-root inode 19.
  From inside the workload, `/` is device/inode `11008:19`, BusyBox is
  `11008:21`, its hash is the same `f060103f…`, outer proc-root traversal exits
  1, `/sys/class/block/nbd0/dev` is `43:0`, size is 4,194,304 sectors, and both
  `mk_transport` and `nbd` are live. The assumed `/proc/mounts` root pattern is
  absent, so `root_mount` is empty and line 226 correctly fails. Mode-0600 log
  is 4,855 bytes/SHA `6260575e…`, exit 1, empty credential scan. Final proof
  must compare child `st_dev` to `makedev(43,0)` and retain the actual nbd0
  mount line instead of assuming its mountpoint spelling. Early ERR messages at
  lines 19/25/26 are diagnostic noise from expected cleanup misses and must be
  suppressed by disabling the ERR trap inside cleanup.
  Device-bound run five repeats all exact hash and inode matches and removes
  cleanup diagnostic noise, but still exits 1 at line 237 because the child
  mount table does not name its mounted source `/dev/nbd0`. Mode-0600 log is
  4,693 bytes/SHA `35a2827e…`, empty credential scan. This pathname is not the
  ownership proof: child root `st_dev=11008`, while the observed block device
  is major/minor `43:0`, whose Linux `makedev` value is exactly 11008. The final
  qualifier will retain `/proc/mounts` verbatim for audit and use that exact
  device-number equality, not a false source-name assumption.
  Mount-table run six retains the full child table: devtmpfs, proc, read-only
  sysfs, private `/run`, and expected protected proc/sys projections. It has no
  ext4 row because procfs reports mounts visible beneath the chroot and omits
  the containing ext4 mount. Line 216's remaining `grep ' ext4 '` therefore
  exits 1 before the already-implemented `st_dev == makedev(nbd0)` assertion.
  Mode-0600 log is 5,807 bytes/SHA `f7f5b5d4…`, empty credential scan. The full
  table remains required evidence; only the contradicted ext4-row assertion is
  removed, while exact device/inode binding remains mandatory.
  Final qualifier SHA `1212ad47…` passes on exact boot `f93f21b6…`, selected
  release `ca7d7d01…`, daemon `a4a91006…`, and approved manifest `d4230978…`.
  The held-source manifest's `bin/busybox` record (mode 0755, uid/gid 0,
  1,013,320 bytes, SHA `f060103f…`) matches both live ext4 inode 21 and the
  child's `11008:21`; OCI-root inode 19 matches child `/` at `11008:19`.
  Device `11008` equals `makedev(43,0)` for the observed 4,194,304-sector nbd0.
  The exact BusyBox has no ELF interpreter or `DT_NEEDED` entries, so there are
  no dynamic libraries to attribute. Live outer image hashes match approved
  agent `b57bae66…`, relay `293ff1ea…`, and deployed init `edc9284c…`; from the
  workload all trusted paths are absent and outer proc-root traversal exits 1.
  `mk_transport` and `nbd` are live, and the complete visible child mount table
  is retained. Normal teardown returns instances, tasks/containers, records,
  exports, helpers, links, and firewall rules to zero with four services still
  healthy. Terminal `G4_ROOT_PROVENANCE_LIVE_PASS` exits 0. Mode-0600 evidence
  is 6,827 bytes/SHA `ad9a358e…`, empty credential scan. This closes the row.
  Validated harness plus focused hello-identity negatives are preserved in
  isolated test commit `940e21e749f0…` (`test: qualify runtime root
  provenance`). The live runtime binaries remain exact `ca7d7d01…`; the
  transcript binds the separately uploaded qualifier by SHA `1212ad47…`.
- [x] Record backing allocation, owner sandbox and generation, quota/high-water
  state, mount table, request/flush counters where relevant, teardown order,
  offline filesystem result, and before/after proof that every cloud storage
  device and allocatable storage controller remained owned by the primary.
  Audit checkpoint: retained exact-source evidence already supplies the
  generation-bound owner (`g4-single-owner-live-pass.log`), nonzero read/write/
  flush counters, guest quiesce-before-NBD-disconnect ordering and offline
  `e2fsck` (`g4-storage-teardown-live`/single-owner evidence), plus byte/inode
  quota and high-water behavior (`g4-capacity-enospc-live-pass.log`). This does
  not yet close the composite claim. No single retained run binds the live
  image's allocated size/quota and child mount table to a before/after inventory
  demonstrating that every real cloud block device and every allocatable
  storage controller remained in the primary. A focused replacement-instance
  qualifier must collect those missing observations without attaching a cloud
  disk or controller to the child.
  Read-only topology inspection on restarted boot `f93f21b6…` confirms the
  concrete assertion set. `/dev/sda` (100 GiB boot, serial
  `persistent-disk-0`) and `/dev/sdb` (30 GiB mediated store, serial
  `mk-mediated-storage-20260830`) are SCSI LUNs `0:0:1:0` and `0:0:2:0`; both
  resolve through the one primary Virtio-SCSI PCI function `0000:00:03.0`
  (`1af4:1004`). The store remains mounted only at
  `/srv/multikernel-storage`. Kerf reports its CPU/memory pool available and no
  instances; `/proc/kimage` is empty. Therefore the live qualifier must compare
  canonical serial/HCTL/sysfs/PCI-controller inventories before, during, and
  after the child, require both primary mounts throughout, require no physical
  `sd*` device/controller in the child, and retain the child's virtual NBD and
  mount table separately.
  The focused qualifier is now implemented as
  `test-runtime-storage-primary-ownership-live.sh`; local Bash syntax and diff
  checks pass (`shellcheck` is unavailable). Its exact SHA-256 is
  `09607c71402d…`. Transfer to boot `f93f21b6…` verifies the complete
  11,324-byte script at the same digest and VM-side Bash syntax passes. This is
  transfer provenance only; no live result is claimed before execution.
  First execution is retained as mode 0600/51,893 bytes/SHA-256
  `4bd181c60c3c…`, exits 1, and has an empty expanded credential scan. It
  successfully records identical primary disk/controller/mount inventories
  before, during, and after; a fully allocated 2-GiB image and equal quota;
  262,144-inode limit; distinct owner/export generations; 10.65 GB and
  1,966,045 inodes remaining; a child with only loop/NBD devices, no `sd*` or
  storage-class PCI function, and its complete mount table; ordered quiesce at
  console line 7 before NBD disconnect at line 9; and released counters
  (276 reads/10,461,184 bytes, 50 writes/827,392 bytes, 9 flushes) plus clean
  offline-check SHA `b3b2f0ec…`. It fails only at the final idle-daemon restart.
  Diagnosis shows a genuine replacement-host configuration drift introduced by
  the earlier non-destructive 20→30-GiB store expansion: the already-running
  daemon tolerated the enlarged filesystem, but its systemd preflight still
  expects the old byte size and now rejects restart with `runtime storage byte
  size mismatch`. The auto-restart loop reached 18; Kerf has no pool/instances,
  both durable live counts and ctr inventories are zero. The run is not a pass,
  and the expected-size deployment configuration must be corrected before a
  clean retry.
  The first immutable-deployment correction attempt stops the already-failing
  service, verifies exact 30-GiB device and candidate environment SHA
  `7f296d38…`, then fails closed because the uploaded input is owned by the
  ordinary SSH user; the deployment manager correctly rejects it as unsafe.
  No deployment is installed or activated. Private log
  `g4-storage-size-config-correction.log` is mode 0600/1,990 bytes/SHA-256
  `a5f6114e…`, exits 1, and has an empty credential scan. The retry must first
  make the exact input root-owned mode 0600, as required by the manager.
  The managed retry succeeds and is retained privately at mode 0600/3,511
  bytes/SHA-256 `83ab9edc…`, exit 0, with an empty credential scan. Exact
  candidate source installs and atomically selects immutable support deployment
  `6182145c5cef…` with `MKRUNTIME_STORAGE_BYTES=32212254720`. The preflight now
  accepts the actual disk; mkruntimed is active with zero restarts, the selected
  runtime remains `ca7d7d01…` with daemon SHA `a4a91006…`, Kerf has no pool or
  instances, and both ctr inventories are empty. The VM is healthy for a clean
  rerun; the failed transcript remains evidence of the caught drift.
  The unchanged qualifier's second run is retained mode 0600/53,092 bytes,
  SHA-256 `fafcd762…`, exit 1, with an empty credential scan. It repeats every
  allocation, owner, quota/high-water, mount, counter, offline-check and
  before/during/after physical-ownership observation, and the corrected daemon
  restart now succeeds active with zero restarts. The sole failure is the final
  harness assumption that clean Kerf must print a configured
  `Pool Allocated: 0`; after idle restart it instead correctly prints
  `No memory pool configured`. Independent inspection confirms that clean
  state, no instances, empty ctr inventories, and zero durable live counts.
  The retry will accept either authenticated clean representation while still
  requiring zero instances and all services healthy.
  The narrow clean-pool correction passes local and VM Bash syntax plus diff
  checks. Revised 11,488-byte qualifier SHA-256 is `fa46fb36364a…`; the remote
  mode-0700 copy matches exactly. No other assertion changed, and no pass is
  claimed before the third execution.
  Exact third execution exits 0 with
  `G4_STORAGE_PRIMARY_OWNERSHIP_LIVE_PASS`. Private evidence is mode 0600/
  54,812 bytes/SHA-256 `028e0733…`, and the expanded credential scan is empty.
  On boot `f93f21b6…`, runtime `ca7d7d01…`, daemon `a4a91006…`, and qualifier
  `fa46fb36…`, the primary's before/during/after inventories are byte-identical:
  100-GiB `persistent-disk-0` at `0:0:1:0` mounted as `/`, 30-GiB
  `mk-mediated-storage-20260830` at `0:0:2:0` mounted at the storage root, and
  shared Virtio-SCSI controller `0000:00:03.0`. The child has no `sd*` and no
  storage-class PCI function; only zero-sized loops, 2-GiB nbd0, zero-sized
  nbd1, and its complete protected mount table are present. The live image is
  2,147,483,648 bytes with 2,147,487,744 allocated bytes, equal 2-GiB quota,
  262,144 inode limit, 10,655,387,648 free bytes and 1,966,045 free inodes over
  the 1-GiB/1,024 production reserves. Owner generation `aa2738aa…` and export
  generation `40685c89…` remain identical through release. Console line 7
  quiesces sync/remount-ro/sync before line 9 disconnects nbd0; release records
  280 reads/10,461,184 bytes, 49 writes/823,296 bytes, 9 flushes, and offline
  clean SHA `b3b2f0ec…`. After idle restart Kerf is unconfigured with no
  instances and four services active with zero restarts.
  Independent exact-source audit SHA `7010e3da…` then exits 0 with
  `G6_FINAL_RESOURCE_RETURN_PASS`: all 19 child/mount/artifact/FIFO/network/
  ctr/Docker/rootfs/endpoint/process counters are zero. Its private mode-0600
  transcript is 16,505 bytes/SHA-256 `b2f411ea…`, credential-clean, and binds
  the same boot/release/daemon. The composite evidence row is closed.
  The validated qualifier is preserved in isolated commit `9c6c40f`
  (`test: qualify primary storage ownership`). The post-closure repository gate
  passes documentation/links, 7 schemas/22 cases, 17 current evidence
  manifests, 97 OCI cases, 20 initramfs cases, 12 storage cases, and all bind,
  bootstrap, mount, image, release, deployment, ledger, capture, containerd and
  final-audit checks. Five generated bytecode files were removed,
  `git diff --check` is clean, and checklist totals are 52 closed/33 open.
- [x] Retain raw output for every injected failure and an immediate post-failure
  inventory showing no child, mount, TUN, iptables rule, partial artifact, or
  ownership leak.
  Evidence audit closes this row using the retained exact-source VM runs rather
  than a duplicate injection. `g4-partial-artifact-live-pass.log` is private
  mode 0600/94,007 bytes/SHA-256 `4e66461f…`, wrapper exit 0, credential-clean.
  It retains the real post-allocation one-shot `load` rejection
  (`BACKEND_FAILURE`) and the immediately following 20-field inventory: zero
  children, runtime/NBD mounts, storage/bundle artifacts, FIFOs, links, routes,
  NAT/filter rules, default/moby/Docker objects, rootfs records, live exports,
  endpoints, NBD helpers, and relays. The two disconnecting shim processes are
  explicitly observed, sampled every 250 ms, and converge to zero before the
  strict all-zero checkpoint; the new generation is durably `RELEASED` with a
  clean offline check, proving no ownership leak. The same raw transcript names
  and passes all nine post-validation Create rollback stages, six rootfs
  failure/diagnosability cases, and four lifecycle cancellation/pool cases,
  then repeats the all-zero checkpoint. Independent private audit
  `g4-partial-artifact-final-resource-audit-pass.log` is mode 0600/14,185
  bytes/SHA `545ad9ea…`, exit 0, credential-clean, and verifies all 19 resource
  counters zero with no Kerf pool/instance and four healthy services.
  Builder/allocation injection output is separately retained in private
  `g4-storage-fault-matrix-live-pass.log` (86,309 bytes/SHA `1eca1db8…`, exit
  0, credential-clean): interrupted copy, wrong UUID, every storage/initramfs
  allocation/publication ENOSPC boundary, real block/inode ENOSPC, and both
  high-water refusals each require no public/private partial output. Its
  bracketing audits pass all-zero, and independent post-fault audit
  `g4-storage-post-fault-final-resource-audit.log` is mode 0600/16,301 bytes/
  SHA `3aeb7b79…`, exit 0. These raw per-case results plus immediate and
  independent inventories substantiate every G4 injected failure class and
  close the row.

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
  post-ADD failures receive a bounded generation-bound DEL. CHECK now requires
  mknetd to return that same complete endpoint identity and rejects a missing
  or mismatched response rather than treating transport success as proof.
  Shim PROVISION/recovery/REPORT/RELEASE apply the same complete durable
  endpoint validator. Mismatched workload or generation is never retained,
  invalid ATTACH closes its descriptor, and malformed report/release generation
  cannot panic or reach mknetd; the boundary matrix passes 100 race repetitions.
  Guest `CloseNetwork` now has an independent five-second deadline; timeout is
  returned while local TUN cleanup continues, with 100 focused race-detector
  repetitions proving the descriptor is closed after a blocked guest call.
  Cache directory
  creation, private bounded reads, exclusive publication, and deletion are now
  relative to a component-walked no-symlink directory descriptor. The
  descriptor stays open across CHECK/DEL daemon contact, and DEL refuses to
  remove a cache whose generation record changed in flight. Beneath CNI,
  mknetd now journals `ALLOCATING` before its first
  namespace/link mutation and reconciles incomplete generations by bounded
  teardown; injected final-state persistence failure proves the durable record
  exists before mutation and is removed only after rollback. Durable endpoints
  receive full semantic/key/path validation before reconciliation, and the
  state file is loaded no-follow with inode-stability checks.
  Endpoint teardown symmetrically journals `DELETING` before external removal;
  an injected backend failure proves restart reconciliation completes deletion
  and removes the retained record. Runtime-owned RELEASE retains its sandbox
  generation through that phase, and a focused failure/retry test proves the
  same process can resume teardown without an mknetd restart. The mknetd store
  now shares the descriptor-anchored directory walk, private bounded read, and
  synchronized `openat`/`renameat` publication used by the storage stores; a
  post-open directory replacement is rejected without mutating the substitute.
  mknetd, mkruntimed, and the guest mk-agent listener now share a descriptor-anchored Unix
  socket guard: the parent is opened without symlinks and must be caller-owned
  and non-writable by group/other; only an exact-mode, caller-owned,
  single-link stale socket is removed; binding resolves through the held parent
  descriptor; and shutdown unlinks only the captured socket inode. Go's
  automatic Unix-listener unlink is explicitly disabled so it cannot bypass
  the identity check. Real pathname-socket tests cover safe stale replacement,
  live connection, normal cleanup, and hostile replacement preservation. The
  guest init creates a private `/run/multikernel-agent` parent instead of
  placing the agent control socket in shared `/tmp`.
  ATTACH descriptor receipt now marks every received right close-on-exec and
  closes it on truncated, malformed, misbound, error, or wrong-count replies;
  the server accepts only a complete payload-and-rights send. A real
  Unix-socket/pipe test checks that a rejected truncated reply leaves no hidden
  reader descriptor, but both real SCM_RIGHTS tests are skipped by the current
  local sandbox and remain mandatory without a skip on the disposable host.
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
  the restored DNS file. Successful configuration now retains its complete
  identity and accepts only an exact replay; the shim reconnects after an
  injected lost reply, while changed identity fails without mutation. Failed
  close retains the replay identity and a successful retry clears it. Both
  matrices pass 100 race-detector repetitions. Shim state/counter report
  failures now enter a joined
  one-entry latest-state worker: two-second calls retry every 100 milliseconds,
  and a newer state supersedes stale pending state. Injected
  `DISCONNECTED`-to-`READY` coalescing, two failures, current counters, and a
  later `DEGRADED` report pass 100 race-detector repetitions. Focused tests
  cover a blocked descendant, output
  overflow, combined stdout/stderr, cancellation without mutation, repeated
  close, and failed DNS restoration; live fault and leak evidence remains open
  below.
  Service cancellation now closes both the mknetd listener and every accepted
  incomplete request; accept-loop return cancels the derived handler context.
  Focused blocked-peer and shared callback tests pass 100 race-detector
  repetitions.
  The exact serve loop also enforces 128 handlers by default, a hard maximum of
  1,024, closes over-budget peers without a goroutine, and joins admitted
  handlers before return. Saturation/cancellation passes 100 race-detector
  repetitions; the guarded pathname listener remains a disposable-host case.
- [x] Define firewall and network-policy ownership and install rules that
  cannot be bypassed by spoofed source addresses, alternate routes, malformed
  packets, or sibling traffic. Per-generation primary chains enforce source,
  metadata, sibling, return-traffic, egress, and default-drop policy.
- [x] Restore the guest's configured DNS state cleanly on teardown and avoid
  hard-coding a public resolver as the only supported policy. DNS is validated
  node configuration; regular-file, symlink, and absent states have restoration
  tests.

### Automated and live tests still required

- [x] Explicit child-to-primary, outbound TCP, outbound UDP, DNS query/answer,
  and return-traffic assertions; retain destination and response details.
  Current evidence audit keeps this row open. The older shared matrix proves a
  configured child address and successful hostname HTTP, but its marker-only
  output deliberately discards the DNS answer and HTTP response. It has no
  explicit primary listener exchange and no separate UDP response. Focused
  network tests validate configuration/counters but cannot replace live packet
  evidence. A new exact-source qualifier must retain: the endpoint identity;
  a tokenized TCP and UDP request/reply between child and a primary-owned
  listener; the configured resolver plus a DNS question and returned A/AAAA
  details; an external TCP destination/status/body digest; and before/after
  counters and zero-resource cleanup.
  A focused qualifier now exists as `test-runtime-network-flows-live.sh`. It
  creates tokenized one-shot primary TCP/UDP listeners, requires exact replies
  inside the child, retains both primary peer/payload JSON records, records the
  configured endpoint and DNS answer, retains external HTTP status/body size/
  digest, compares endpoint identity/counters, deletes the workload, returns
  the idle pool, and invokes the independent 19-counter audit. Local Bash
  syntax and diff checks pass; `shellcheck` is unavailable. The executable is
  9,414 bytes/SHA-256 `25fa8a71dc60…`. The file was uploaded to the running
  disposable VM as `/tmp/test-runtime-network-flows-live-25fa8a71.sh` and the
  guest independently reports mode `0755`, size 9,414 bytes, and the complete
  matching SHA-256
  `25fa8a71dc60c7c9ae78221e1ae116fc2ad07ae372ba352fe9617552e4a9bc98`;
  guest-side `bash -n` also passes. The first live attempt is retained as
  `g5-network-flows-live-first.log`, mode `0600`, 33,436 bytes, SHA-256
  `c8cf853e522147b57cf72ef2c7b4d7a2fd900381e6e105b7d3827821d21cf2d5`,
  exit 1, with no credential-pattern matches. It establishes a clean start,
  exact boot/release/daemon/qualifier provenance, endpoint `172.31.0.2/30`,
  tokenized TCP and UDP request/reply between that endpoint and primary
  `10.148.0.58:18080/18081`, resolver `169.254.169.254`, two returned IPv4 and
  two returned IPv6 answers for `example.com`, and HTTP destination
  `172.66.147.243:80`, status 200, 577-byte response/SHA-256
  `25ddf2c883e0d1958ea971d279a7e4f0fd446724ee3db7db19dadabd4a62e484`.
  Endpoint identity remained byte-identical with zero errors, and normal
  release reached zero rootfs/export/endpoint counts. The attempt failed only
  after restarting `mkruntimed`, at the embedded final-resource audit before it
  emitted an observation; the audit readiness contract is under diagnosis, so
  this is substantial packet evidence but not a passing qualification.
  A separately retained traced audit 62 seconds later,
  `g5-network-flows-first-post-failure-audit.log`, passes: mode `0600`, 16,298
  bytes, SHA-256
  `38f90c26b62d2d7d97ebdd5af18e652f8ca6eebae85de5477badaac80094e370`,
  exit 0, credential-pattern clean, exact release/daemon/boot provenance, Kerf
  without a pool or instances, all 19 inventory counters zero, and all four
  services active with zero restarts. The daemon journal contains only the
  deliberate stop/start and successful mount validation. This isolates the
  first failure to a readiness race: systemd became active before the immediate
  Kerf-backed audit was ready, rather than a packet-path or leaked-resource
  failure. The qualifier now waits up to 60 seconds for both clean Kerf
  assertions, retains that response, and only then invokes the audit. The
  revised executable passes local Bash syntax and diff checks and is 9,807
  bytes/SHA-256
  `cfbd71542d84d364d3c302279216635ff37b0fd1ba880fca83d2944360d98940`;
  the guest reports the uploaded
  `/tmp/test-runtime-network-flows-live-cfbd7154.sh` as mode `0755`, size 9,807
  bytes, the same complete SHA-256, and guest-side syntax-valid. This revision
  was executed as the retained `g5-network-flows-live-second.log`, mode `0600`,
  36,258 bytes, SHA-256
  `85bd5da562521fe66ad807ef1bfa182ae680d5ce1abe6e8a16cfee2a8e298af5`,
  exit 1, credential-pattern clean. It independently repeats every TCP, UDP,
  DNS, HTTP, stable-identity, zero-error, and zero-count assertion and obtains
  the clean Kerf response immediately after restart, but the following full
  audit still exits before its first observation. A traced audit started 21
  seconds later passes as
  `g5-network-flows-second-immediate-audit.log`, mode `0600`, 16,298 bytes,
  SHA-256
  `6372ebaff55876204cddddb20e0321c39f93e94a2f5e67ae6483a126af8f9e1f`,
  exit 0 and credential-pattern clean, with all 19 counters zero and all four
  services active/zero-restart. Kerf readiness alone is therefore insufficient
  to define complete post-restart settling; the qualifier must wait for the
  complete exact-source audit contract, retaining only its eventual pass. It
  now polls that entire audit for up to 60 seconds, requires the terminal pass,
  and emits the successful audit output. Local syntax/diff checks pass; the
  resulting 9,991-byte executable has SHA-256
  `80e3327e3333fd71f9fe668534280e3021b723d63fa31ce34305cbf1492ea30c`
  and was uploaded as `/tmp/test-runtime-network-flows-live-80e3327e.sh`.
  Guest verification reports mode `0755`, size 9,991 bytes, the matching full
  digest, and passing guest-side syntax. The third run passes and is retained as
  `g5-network-flows-live-third.log`, mode `0600`, 42,937 bytes, SHA-256
  `a380aaf75e60ac3c508acd32cff52e0de8ff37ab9ec20be5ec2ff95aff8c0f67`,
  exit 0, credential-pattern clean, with terminal
  `G5_NETWORK_FLOWS_LIVE_PASS`. It proves exact ca7/boot/binary/qualifier
  provenance; endpoint `172.31.0.2/30`, gateway `172.31.0.1`, MTU 1400 and
  resolver `169.254.169.254`; tokenized TCP and UDP request/reply with primary
  `10.148.0.58`; two A and two AAAA answers; external HTTP 200 from
  `104.20.23.154:80` with 577-byte body/SHA-256 `25ddf2c883e0…`; unchanged
  endpoint identity and zero errors; normal-release zero counts; clean Kerf;
  and an eventually passing exact-source audit after three expected transient
  failures, with all 19 counters zero and all four services active/zero-restart.
  The separately invoked
  `g5-network-flows-final-resource-audit.log` passes independently: mode
  `0600`, 16,298 bytes, SHA-256
  `40dc7f4ad2ad5e87abbe83011b3f605c965ba0d9ed247cc9073c83ab867d39a1`,
  exit 0, credential-pattern clean, exact ca7/boot/daemon provenance, Kerf with
  no pool/instances, all 19 counters zero, and all services active with zero
  restarts. The focused flow row is therefore closed. The post-closure local
  gate also passes: documentation/links; 7 schemas/22 fixtures; all 17 current
  manifests; 97 OCI cases; read-only bind, bootstrap, 20 initramfs, 12 storage,
  mount, 7 architecture, 6 release-manifest, lifecycle, deployment, GCE ledger,
  capture, containerd-config, and final-evidence-audit checks; Bash syntax and
  `git diff --check` are clean.
- [x] Two sandboxes with overlapping internal names but distinct network
  identity, plus positive allowed routing and negative default isolation.
  Evidence audit keeps this row open. The retained shared matrix proves two
  distinct `/30` identities and bidirectional sibling `ping` rejection, but its
  workloads use different container IDs and never assert an identical internal
  hostname. Its outbound marker also does not retain a tokenized positive route
  response for each sandbox. The new single-sandbox flow qualifier cannot fill
  either two-sandbox gap. A focused current-source qualifier must start two
  simultaneous children with the same hostname, retain distinct endpoint and
  child-reported identities, prove a tokenized allowed exchange from each to a
  primary listener, prove bidirectional sibling TCP/UDP or ICMP rejection with
  exit/output details, and finish with the exact 19-counter audit.
  `test-runtime-network-isolation-live.sh` now implements that focused case
  with two simultaneous ctr sandboxes, explicit shared hostname, durable and
  child-reported address correlation, two tokenized replies from one
  primary-owned listener, bidirectional sibling ICMP rejection with raw output,
  normal deletion, and bounded exact-source final-audit readiness. Local Bash
  syntax and diff checks pass. The executable is mode `0755`, 9,547 bytes,
  SHA-256
  `b49960be4e120b4f4ae3e2cd10c512396577491a08016e1e4b4c03c3cfedc173`;
  the guest independently reports the uploaded
  `/tmp/test-runtime-network-isolation-live-b49960be.sh` as mode `0755`, size
  9,547 bytes, the same complete digest, and syntax-valid. No live result is yet
  claimed by transfer alone. The first live run passes as
  `g5-network-isolation-live-first.log`: mode `0600`, 44,193 bytes, SHA-256
  `de553bbb59a635efb71a5684572f7383b6a9b7a95224eaa2159e4435a929f02b`,
  exit 0, credential-pattern clean, terminal
  `G5_NETWORK_ISOLATION_LIVE_PASS`. It retains exact ca7/boot/daemon/qualifier
  provenance; the same `shared-internal-name` inside both children; distinct
  `172.31.0.2/30` and `172.31.0.6/30` child/state identities, gateways,
  generations, and sandbox generations; exact `route-token-a/b` replies from
  primary `10.148.0.58:18082`, observed there with matching child peer IPs;
  bidirectional sibling ping exit 1 with one sent/zero received/100% loss; zero
  post-release rootfs/export/endpoint counts; and an eventual exact-source
  19-counter-zero audit after three transient retries. A separately invoked
  audit also passes as
  `g5-network-isolation-final-resource-audit.log`: mode `0600`, 16,298 bytes,
  SHA-256
  `a8bf3216fd63af84f9324529fe92fc05a34483f5dce30828987e4e973382d9c7`,
  exit 0, credential-pattern clean, same exact release/boot/daemon, clean Kerf,
  all 19 counters zero, and four active zero-restart services. The overlapping-
  name/distinct-identity/positive-route/default-isolation row is closed. Its
  post-closure repository gate passes documentation/links, all schemas/current
  manifests, 97 OCI cases, bind/bootstrap/initramfs/storage/mount/architecture/
  release/lifecycle/deployment/ledger/capture/containerd/final-audit checks,
  qualifier syntax, and `git diff --check`.
- [x] MTU boundaries, fragmentation, checksums, malformed/oversized frames,
  loss, reordering, burst traffic, sustained load, and slow readers. A
  socketpair-backed shim pump suite now proves exact-MTU bidirectional
  forwarding, oversized primary ingress and guest egress drops, exact counter
  increments, disconnected-packet loss accounting, and retry-until-success
  reconnect after two injected failures. Fragmentation, checksum, ordering,
  load, and slow-reader coverage remain open.
  Current-source audit confirms the precise test gap. The shim's pump is
  deliberately packet-opaque and single-flight, but its only focused test sends
  one exact-MTU packet, one oversized packet in each direction, and two packets
  around a disconnect. There is no ordered multi-packet sequence, fragment/
  checksum/malformed-byte preservation set, burst or sustained loop, or
  receiver-backpressure assertion. Add focused socketpair tests for these pump
  invariants before designing the narrower live kernel-stack MTU/load run; do
  not infer these clauses from implementation structure alone.
  The first added source-test attempt passes representative first/last IPv4
  fragment plus checksum and truncated-frame byte preservation, and a 256-
  packet ordered burst with exact payloads/counters. Its slow-reader subcase
  fails before traffic because this local sandbox rejects `SO_RCVBUF` with
  `EPERM`; the historical exact-MTU/oversize/disconnect subcases still pass.
  This is a harness portability failure, not a slow-reader result. Remove the
  unnecessary buffer-size mutation and fill the default nonblocking receive
  queue instead before retrying. That correction passes: both the historical
  pump test and new `TestNetworkPumpPacketIntegrityOrderingLoadAndSlowReader`
  complete in 0.513 seconds. The latter proves three representative opaque
  packet shapes unchanged, 256 ordered 64-byte packets unchanged with exact
  counters/no drops, and a default queue filled until a nonblocking TX drop,
  followed by drain and an exact recovery marker with zero fatal errors.
  The combined old/new pump selection also passes 20 race-detector repetitions
  in 10.905 seconds. Exact-source VM execution and a real kernel-stack MTU/load
  qualifier remain pending. The repository gate remains green across docs,
  schemas/manifests, 97 OCI cases, and all focused runtime suites; the diff is
  clean. Commit `e4d7c9c0c7b66f5a5b8cb8e574c7ca80a26e87a6` freezes this
  source coverage. Its deterministic gzip archive is 1,330,103 bytes/SHA-256
  `3ba8b51764f5fc458a98059d8324cc214d5a362a0b6abf0157f7688304c28ee6`;
  guest verification matches that size/digest, extraction into unique
  `/var/tmp/mksrc-e4d7c9c` yields 655 regular files, and the relevant guest
  `main_test.go` matches the local SHA-256 `0a545d38811f…`. Guest execution is
  retained in `g5-network-pump-stress-source-live.log`: mode `0600`, 31,692
  bytes, SHA-256
  `9a8f6f401ff2f4c1be95f1c29685108a041c2961cb67bea871c3023719583d30`,
  exit 0, credential-pattern clean. All seven named subcases pass in each of 20
  race-detector repetitions (10.552 seconds): exact MTU, both oversize
  directions, disconnect/loss/reconnect, fragment/checksum/malformed opacity,
  256-packet ordering/load, and slow-reader drop/recovery. This closes the
  source pump portion only; real child/primary negotiated-MTU and load evidence
  remains required for the composite row.
  `test-runtime-network-stress-live.sh` now implements that live slice: a real
  runtime child must complete exact 1,400-byte IPv4 ICMP, 3,000-byte fragmented
  ICMP reassembly, a 256-packet/10ms-interval zero-loss/no-duplicate burst, and
  a one-MiB zero stream through a primary TCP server that deliberately waits
  one second before reading and verifies byte count/SHA-256. It also requires
  stable endpoint identity, increased RX/TX counters with unchanged drops/
  errors, normal release, and the exact 19-counter audit. Local Bash syntax and
  diff checks pass; the mode-0755, 9,551-byte qualifier hashes to
  `336e15ecd63c32cd1bd8984b5f81ebd89e81119449b31d566b5eef1554a137ca`.
  The guest reports uploaded
  `/tmp/test-runtime-network-stress-live-336e15ec.sh` with the same mode, size,
  full digest, and passing guest-side syntax. Its first run passes as
  `g5-network-stress-live-first.log`: mode `0600`, 100,518 bytes, SHA-256
  `31d18fcaae2ec20c2d539ae40022abfa6ee16aa6a7f7a905a19b6dc6cbf18e2e`,
  exit 0, credential-pattern clean, terminal `G5_NETWORK_STRESS_LIVE_PASS`.
  Exact 1,400-byte IPv4 traffic completes 5/5 with zero loss; 3,000-byte ICMP
  completes 3/3 after fragmentation/reassembly; the 256-packet 10ms burst has
  256/256, zero loss, and no duplicate marker; and the one-second delayed
  primary reader receives exactly 1,048,576 zero bytes from `172.31.0.2` with
  SHA-256 `30e14955ebf1…` and returns the same exact result to the child. Durable
  counters advance from RX/TX 0/0 to 768/1,024 with stable identity and zero
  drops/errors. Normal release reaches zero counts, and the 19-counter audit
  passes after three transient retries. Independent
  `g5-network-stress-final-resource-audit.log` also passes: mode `0600`, 15,686
  bytes, SHA-256
  `3f2c735794049ca274154de4305f69776e388121dd703b79771ff410ec11f27c`,
  exit 0, credential-pattern clean, exact release/boot/daemon, clean Kerf, all
  19 counters zero, and four active zero-restart services. Combined with the
  exact-source VM race suite for opaque checksum/malformed/oversized handling,
  disconnect loss/reconnect, ordering, burst and backpressure recovery, the
  composite MTU/fragment/load/slow-reader row is closed. The post-closure
  repository gate passes docs/links, schemas/current manifests, 97 OCI cases,
  all focused runtime suites, qualifier syntax, and `git diff --check`.
- [ ] Agent transport disconnect/reconnect, child restart, networking-service
  restart, `mkruntimed` restart, shim death, and primary restart. Focused pump
  coverage now proves exchange disconnect detection and authenticated reconnect
  retry before later traffic succeeds; the cross-process restart matrix remains
  open.
- Restart-row evidence audit (recorded before a new live attempt): the retained
  `g6-mkruntimed-restart-continuity-v2-pass.log` proves the daemon PID changes
  while the task, namespace-holder PID, child boot ID, recovery record, stream,
  exec, and events remain usable; `g6-forced-shim-reconnect-fifo-pass.log`
  proves worker/holder replacement, unchanged child boot identity, continuous
  attach, post-fault exec, and normal teardown. Those are valid evidence for
  the `mkruntimed` and shim-death members, but neither transcript exercises a
  network exchange across the fault. The packet-pump source suite proves an
  injected transport disconnect and authenticated reconnect in-process only.
  No retained focused transcript proves live packet continuity across an
  `mknetd` restart, relay/agent transport-process death, child replacement, or
  restart of the primary-side network service. The composite row therefore
  remains open specifically for those live boundaries plus packet assertions
  around the already-proved daemon/shim boundaries; host reboot evidence is
  not being substituted because earlier reboot observations exposed separate
  volatile-bundle/orphan reconciliation failures.
- Disposable-instance restart-row preflight: GCE reports
  `mklinux-g4-g6-final-20260905` `RUNNING` with start timestamp
  `2026-10-05T17:58:35.542-07:00`; host boot ID remains
  `f93f21b6-21f1-4fc0-aebc-806347d6b43d`. `mkruntimed`, `mknetd`, containerd,
  and Docker are active. Default/moby task inventories, Docker containers,
  Multikernel children, and mknetd's durable endpoint map are empty, and no
  anchored shim/relay/NBD workload process is live. The restarted VM is
  therefore available and clean for a new focused qualifier; this preflight is
  availability/cleanliness evidence only, not restart-row behavioral proof.
- New focused implementation `scripts/test-runtime-network-restart-live.sh`
  creates a real child, performs a token-exact child-to-primary TCP exchange
  before faults, and repeats it after `mknetd` restart, `mkruntimed` restart,
  and forced shim-worker reconstruction. It requires unchanged child boot and
  endpoint ownership/generations across those continuity boundaries, changed
  daemon or worker/holder PIDs as applicable, then deletes and recreates the
  same task name and requires new child boot, sandbox generation, and endpoint
  generation before another exchange. Every generation is normally removed
  and the final 19-counter resource audit is mandatory. Local Bash syntax,
  ShellCheck when installed, and diff checking pass; the mode-0755,
  10,568-byte script hashes to
  `c4a3bb672fd5166ff5b7e5c4630ebfb3af079bf77162d68f3cae40ceb64d89ec`.
  This harness intentionally does not claim direct relay-process replacement
  or primary-host reboot; those remain separate boundaries after this run.
- Guest upload verification independently reports the exact full SHA-256
  `c4a3bb672fd5166ff5b7e5c4630ebfb3af079bf77162d68f3cae40ceb64d89ec`
  and 10,568-byte size for
  `/tmp/test-runtime-network-restart-live-c4a3bb67.sh`; guest-side `bash -n`
  passes with `GUEST_UPLOAD_VERIFY_PASS`. The `stat` presentation contains a
  harmless quoting artifact (`\755 ...'`), so executable mode is established
  by the preceding successful `chmod 0755` and subsequent direct execution,
  not by normalizing that displayed string. No live behavior is claimed yet.
- First live attempt is retained, not overwritten, as
  `g5-network-restart-live-first.log`: mode `0600`, 11,639 bytes, SHA-256
  `a438da83021fe6af92b40012ebe936dbd5e79f106512a792f1402135898c4b3c`,
  exit 1, credential-pattern clean. Clean preflight and exact provenance pass;
  task creation reaches `RUNNING` with one rootfs, live export, and endpoint.
  The first boot-ID exec then fails before every restart fault with
  `INVALID_ARGUMENT: invalid agent request` because the harness supplied
  `cat` rather than the runtime-required absolute `/bin/cat`. Its trap stops
  and removes the task. All five boot-ID exec paths are corrected; Bash,
  ShellCheck when present, and diff checks pass. The corrected mode-0755,
  10,593-byte script hashes to
  `4f18aa1dea413825a9119691e546195ddb3c09e68eaea414dbe51e2035b2edf5`.
- The immediate external audit is also retained as
  `g5-network-restart-first-post-failure-audit.log`: mode `0600`, 406 bytes,
  SHA-256
  `391a4928b529361fbf9aad3f55c59ef3a07c4a43ed50e881923d508186ab6631`,
  exit 1, credential-pattern clean, with no behavioral output because the
  auditor rejects the still-configured idle Kerf pool at preflight. A separate
  inventory confirms this exact classification: Kerf has all 16 GiB available
  and no instance; ctr tasks/containers, children, rootfs records, and network
  endpoints are zero; all four services have result `success` and zero
  restarts. The storage file contains 24 historical export records, not 24
  live exports. An idle mkruntimed restart and second audit are required before
  retry; no restart member is claimed from this attempt.
- That recovery is now captured in
  `g5-network-restart-first-recovered-audit.log`: mode `0600`, 2,266 bytes,
  SHA-256
  `0cd9d87e2bceeb14ec12f59057e2170ad6175ac123ebf6b0ff0003d3225e536f`,
  exit 0, credential-pattern clean. The deliberate idle mkruntimed restart
  changes PID 44448→46406, releases the empty pool, and the independent audit
  ends `G6_FINAL_RESOURCE_RETURN_PASS`: clean Kerf, every one of 19 counters
  zero, and all four services active with zero restart count. The corrected
  qualifier may now retry from a proven clean state.
- Corrected guest upload `/tmp/test-runtime-network-restart-live-4f18aa1d.sh`
  independently reports exact mode `0755`, size 10,593, and SHA-256
  `4f18aa1dea413825a9119691e546195ddb3c09e68eaea414dbe51e2035b2edf5`;
  guest syntax passes with `GUEST_CORRECTED_VERIFY_PASS`. This establishes the
  retry's script identity only.
- Corrected behavioral attempt is preserved as
  `g5-network-restart-live-second.log`: mode `0600`, 94,013 bytes, SHA-256
  `0681e7151e11b6e1b80d28a2bc1f311d5081450544f84cf78d23fedae508245b`,
  exit 1, credential-pattern clean. Unlike the first attempt, all intended
  behavior before the final audit passes. Five exact primary TCP observations
  see peer `172.31.0.2` and return the distinct baseline/post-fault tokens.
  `mknetd` changes PID 8326→47224 and mkruntimed 46406→47323; shim recovery
  changes worker/holder 46897/47096→47426/47450 under unchanged supervisor
  46892. Across all three, child boot `fb326a9b…`, endpoint `172.31.0.2/30`,
  sandbox generation `a904f85c…`, endpoint generation `afe9d4df…`, and zero
  drop/error counters remain stable. Normal deletion reaches zero rootfs/live
  export/endpoint counts. Same-name replacement then changes child boot to
  `560753c9…`, sandbox generation to `2f731ec8…`, and endpoint generation to
  `26889f19…`, completes its exact packet exchange, and again cleans to zero.
  The attempt exits only because the strict final auditor is called while the
  now-idle pool is still configured; 60 retries correctly refuse that state.
  Therefore these four behavioral members are substantiated but the composite
  row remains open and the run is not a closing pass.
- The qualifier now performs an explicit idle mkruntimed restart after zero
  workload counts and before the strict final audit, recording both daemon
  PIDs. Syntax, ShellCheck when present, and diff checks pass. Revised mode,
  size, and SHA-256 are `0755`, 10,994 bytes, and
  `f2f782a9b8c397e72dfc7e544bd45c64e05fa29e493cfdc6d9789e2ded38a855`.
- Independent recovery after the second attempt passes in
  `g5-network-restart-second-recovered-audit.log`: mode `0600`, 2,267 bytes,
  SHA-256
  `4d32cdbf0934c1b8240cafe6b183618722af2dbc2fd4da7ee2680d71e84a273d`,
  exit 0, credential-pattern clean. Idle mkruntimed PID changes 47323→48889;
  clean Kerf, all 19 counters zero, and four active zero-restart services end
  in `G6_FINAL_RESOURCE_RETURN_PASS`. The final corrected retry again starts
  from independently proved empty state.
- Final guest upload `/tmp/test-runtime-network-restart-live-f2f782a9.sh`
  independently matches mode `0755`, size 10,994, full SHA-256
  `f2f782a9b8c397e72dfc7e544bd45c64e05fa29e493cfdc6d9789e2ded38a855`,
  and guest syntax (`GUEST_FINAL_VERIFY_PASS`). Live execution remains separate.
- Final corrected run passes as `g5-network-restart-live-third.log`: mode
  `0600`, 76,778 bytes, SHA-256
  `6bdd7e54a685385ef9612e9f714c9252992166c54c1b6abf73797b8261976d66`,
  exit 0, credential-pattern clean, terminal
  `G5_NETWORK_RESTART_LIVE_PASS`. Five token-exact child→primary TCP exchanges
  are independently observed from peer `172.31.0.2`: baseline, after mknetd
  PID 47224→49723, after mkruntimed PID 48889→49823, after shim worker/holder
  49394/49594→49925/49949 beneath unchanged supervisor 49389, and after
  same-name child replacement. The first three fault boundaries preserve child
  boot `1a6dc772…`, endpoint `172.31.0.2/30`, sandbox generation `b07bd996…`,
  endpoint generation `7484d028…`, and zero drops/errors. Replacement changes
  boot to `2a3e36c7…`, sandbox generation to `e31dd74d…`, and endpoint
  generation to `1b5e9d2b…`; both generations clean to zero ownership counts.
  The embedded idle release changes mkruntimed 49823→50524, after which the
  strict auditor passes clean Kerf, all 19 counters zero, and four active
  zero-restart services. This closes live packet proof for mknetd restart,
  mkruntimed restart, shim-worker death/reconstruction, and child replacement.
  The composite row remains open only for direct agent-transport-process
  disconnect/replacement and primary-host restart/recovery semantics.
- Separately invoked `g5-network-restart-final-resource-audit.log` also passes:
  mode `0600`, 1,910 bytes, SHA-256
  `f4e9c07596293bad835fd2a198efe0010709fee6fb92fbcee7700d49cebf42f3`,
  exit 0, credential-pattern clean, exact release/boot/daemon provenance,
  clean Kerf, all 19 counters zero, and four active zero-restart services. This
  independently confirms the passing harness left no tracked resource residue.
- Follow-on evidence hardening is recorded before execution: the passing shim
  reconstruction necessarily replaces its worker-owned relay, but the prior
  transcript did not name those relay PIDs. The qualifier now resolves exactly
  one `mkvsock-relay`/`mk-agent-relay` direct child of each worker, requires the
  old relay to be dead and the replacement PID distinct before the post-fault
  packet exchange, and records both. It also records and requires distinct
  primary TCP listener PIDs for each of the five exchanges. Local Bash syntax,
  ShellCheck when present, and diff checks pass; revised mode/size/SHA-256 are
  `0755`, 11,867 bytes, and
  `2e998c178597319ea058044ee3ac23dcfa136d8b8b3e7722b3f220c88dbe96d6`.
  This is implementation only until the exact revision passes live.
- Guest transfer of `/tmp/test-runtime-network-restart-live-2e998c17.sh`
  independently matches mode `0755`, size 11,867, full SHA-256
  `2e998c178597319ea058044ee3ac23dcfa136d8b8b3e7722b3f220c88dbe96d6`,
  and guest syntax (`GUEST_RELAY_VERIFY_PASS`). Behavioral proof remains next.
- Relay-identity replay is retained as
  `g5-network-restart-live-relay.log`: mode `0600`, 9,884 bytes, SHA-256
  `40ed0cbd71c02eecdddc5630dccbc65f62c8bb8d833b4ff0d228d034782d0b62`,
  exit 1, credential-pattern clean. It passes clean preflight and exact
  provenance but the initial child Create fails before relay discovery or any
  injected fault with `BACKEND_FAILURE`; therefore it supplies no new restart
  claim and does not supersede the passing `third` transcript. The bounded
  containerd journal shows connection to the task shim at 12:37:00Z, followed
  at 12:38:23Z by a disconnected-shim cleanup whose fallback cannot execute
  `/usr/local/bin/containerd-shim-multikernel-v2` because that public pathname
  is absent. Direct post-failure inventory is nevertheless empty: Kerf has no
  pool or instance; tasks, containers, children, rootfs records, live exports,
  and endpoints are zero (28 storage entries are released history). Per the
  instruction to skip the latest failed attempt, no relay replay is attempted
  again now; the exact public-shim-path issue remains recorded for later
  ownership/provenance work while qualification moves to another open row.
- User-restarted-instance baseline: GCE reports `RUNNING`, start timestamp
  `2026-10-06T17:59:57.528-07:00`; guest boot ID is now
  `cc677283-6e80-4bef-a34d-aac8af61566a`, so no pre-restart live identity is
  carried forward. Exact installed selector `0.1.0-dev-ca7d7d0…` and source
  tree `/var/tmp/mksrc-e4d7c9c` survive. mkruntimed/mknetd/containerd/Docker
  are active/running at PIDs 1446/1226/1468/1521 with result `success` and
  zero restarts. Both containerd namespaces and Docker are empty; Kerf has no
  pool/instance; rootfs/endpoints/live exports are zero (28 released export
  histories remain). Crucially, the public shim path is present again as the
  expected symlink to the selected release, whose 14,034,425-byte target is
  executable. This post-restart observation narrows the prior absent-path
  failure to the old boot but does not explain or retroactively pass it. The
  new boot is a clean base for the CNI row.
- CNI replacement-instance preflight finds the managed executable at
  `/opt/cni/bin/multikernel`, a symlink to selected-release `mk-cni`; both hash
  to `d8dbaa8018554117a8eacd271d8eb8fb91a0b257ba594fa33b75de221bd201d5`
  and the target is mode 0755/4,791,080 bytes. No
  `/etc/cni/net.d/10-multikernel.conf` is installed, and the CNI cache,
  named-netns, `mkv*`/`mkhost*`, MK-chain, and 172.31 NAT inventories are empty.
  The focused qualifier must therefore supply and retain its own exact config
  and private cache directory rather than claiming CRI wiring; executable and
  mknetd API qualification can proceed without mutating host configuration.
- Audit of the still-open composite fault row narrows its missing proof. The
  retained single-owner live run already executes stale/conflicting generation
  and duplicate owner/path/port/UUID claims, demonstrates real second-attach
  lock contention, and proves exact lock release. The capacity run now covers
  block/inode exhaustion, both high-water dimensions, and deterministic plus
  real builder allocation failures. Source tests reject ext4 wrong UUID and
  conflicting live export generations, but the wrong-UUID backend cases were
  not selected by the earlier VM pattern. The copy-boundary injection returns
  a generic failure rather than an actual signaled copy process. Therefore the
  row stays open specifically for an actual signal-interrupted staging copy
  with cleanup and exact-source VM execution of wrong-UUID plus the completed
  fault matrix.
- The storage builder now accepts an explicit copy executable (production
  default remains `/bin/cp`) so a focused test can exercise a real signal
  boundary without a production failpoint. The helper writes a partial staged
  file and kills itself with `SIGKILL`; the builder observes child status `-9`
  and removes the exact staging directory with no image or metadata. All 12
  storage-builder cases pass locally and the verbose test name is selected by
  the updated live driver. The driver's VM race pattern also selects pristine
  and current-image wrong-UUID rejection plus conflicting-generation and
  single-owner service cases. Its first local Go invocation reached no tests
  because the sandbox's default Go cache is read-only; this infrastructure
  failure is recorded before retry with a writable isolated cache.
- With `GOCACHE` isolated under `/tmp`, all four selected storage tests pass
  under `-race`: ext4 pristine/current wrong-UUID and quota/clean-state
  inspection, generation-bound single ownership, and retained-preparation
  refusal of a conflicting live generation. This is local source evidence;
  exact-source VM execution and a full gate remain before the composite row
  can close.
- The full repository gate passes with the expanded 12-case storage suite;
  four generated bytecode files are removed and the diff check is clean. The
  interrupted-copy and VM identity selection changes are isolated as commit
  `0672fa1`. Its exact 4,792,320-byte archive hashes to `56140248…`; the live
  driver, real-ENOSPC helper, and verbose storage suite hash to `6495ebc2…`,
  `c9b04fca…`, and `8a92c62f…`. Running ledgers/evidence remain excluded.
  Upload and fresh VM replay remain pending.
- Fresh VM extraction of `0672fa1` matches the 4,792,320-byte `56140248…`
  archive and all three `6495ebc2…`/`c9b04fca…`/`8a92c62f…` script hashes,
  contains 648 regular files, and passes guest Bash syntax. Boot remains
  `c5537cb9…` with all four services active. This is transfer provenance only;
  no expanded fault result is claimed yet.
- Exact `0672fa1` expanded qualification exits 0. Its verbose 12-case builder
  suite explicitly passes `test_signaled_staging_copy_is_bounded...`; the VM
  race run explicitly passes pristine/current ext4 wrong-UUID inspection,
  quota/clean-state identity, generation-bound single ownership, and retained
  conflicting-generation refusal. It also repeats all four real constrained-
  filesystem ENOSPC cases, high-water cases, and bracketing all-zero audits.
  Remote private transcript is mode 0600, 86,309 bytes, SHA-256 `1eca1db8…`,
  wrapper exit 0, with an empty credential scan. Local retention and exact
  post-copy verification remain before closing the composite row.
- Local retention exactly matches mode 0600/86,309 bytes/SHA-256
  `1eca1db8…`; its credential scan is empty and all named SIGKILL, wrong-UUID,
  conflicting-generation, ownership, constrained-filesystem, audit, suite,
  qualifier, and exit markers are present. Combined with the retained
  `5790e421…` single-owner/lock transcript, every clause of the composite
  allocation/fault row is now evidenced and that row is closed. The distinct
  server-loss/corruption/recovery row remains open. Full gates and totals
  follow.
- The post-closure full gate passes documentation/links, 7 schemas/22 cases,
  17 current evidence manifests, 97 OCI cases, 20 initramfs cases, the expanded
  12-case storage suite, publisher/bind/bootstrap/mount/image fixtures, and all
  release/deployment/ledger/capture/containerd/final-audit checks. The known
  local socket `EPERM` remains explicit and is covered by VM execution. The
  diff is clean, four generated bytecode files are removed, and totals are now
  45 closed / 40 open.
- Audit of the unchecked replacement-instance initramfs evidence row finds
  exact retained historical proof: `g4-initramfs-repro-live-pass.log` is mode
  0600/9,148 bytes/SHA-256 `5a1d5ae6…` and records two byte-identical verified
  archive/manifest pairs at `2532e1b5…`/`bf6f3e04…`, while its changed-input
  control diverges at `5850d990…`/`fd059b19…`. Its credential scan is empty.
  Independent audit `c5748c81…` is mode 0600/12,601 bytes, all 19 counters
  zero, no pool/child, four healthy services, and also credential-clean. This
  exactly satisfies the row at commit `7725e17`; because current builder commit
  `0672fa1` adds inode-high-water and ENOSPC cleanup behavior, the row remains
  open until an exact-current replay removes revision doubt.
- Exact-current replay removes that doubt. `g4-initramfs-repro-current-live-
  pass.log` records equal independent outputs at `35771180…`/`416833e3…`,
  divergent changed control `3bd844ae…`/`e1408c2a…`, all 20 tests, qualifier
  pass, and wrapper exit 0. Its local mode/size/hash are 0600/9,171/
  `b32b030e…`. Independent audit is 0600/15,094/`314d0b63…`, all 19 counters
  zero with four healthy services and exit 0. Joint credential scan is empty;
  the replacement-instance initramfs reproducibility evidence row is closed.
  Gates and totals follow.
- The post-closure full gate passes all documentation, schema, 17 current
  evidence-manifest, 97 OCI, 20 initramfs, 12 storage, publisher, bind,
  bootstrap, mount/image, release/deployment, ledger/capture, containerd, and
  final-audit checks. The diff is clean, four generated bytecode files are
  removed, and checklist totals are now 46 closed / 39 open. The broad server-
  loss/host-reset/corruption/recovery row remains explicitly unproved.
- The deterministic-manifest row audit confirms the current shim calls
  `PrepareRootfs` before `CreateSandbox`, and the builder returns only after
  source-before/source-after manifest equality, storage identity/build, and
  generated-versus-verified initramfs digest equality. Retained live evidence
  `33e1d92e…` separately records OCI content and source-root manifest digests;
  exact-current `b32b030e…` separately records generated archive/manifest
  digests. The implementation row is closed; full gates and totals follow.
- [x] CNI failure after every partial `ADD` boundary, repeated `CHECK`, repeated
  `DEL`, stale namespace/link/rule cleanup, and name/address reuse. CNI stdin
  now rejects a valid JSON prefix followed by bytes beyond its one-MiB limit;
  cache creation rejects a symlinked ancestor before creating redirected
  directories; and no-replace publication makes an exact generation replay
  idempotent while refusing to overwrite a conflicting generation. Focused
  tests prove all three boundaries; the remaining repeated/fault matrix stays
  open. The exact-source race matrix plus installed-plugin run below now close
  the repeated/fault matrix.
- New focused implementation `scripts/test-runtime-cni-faults-live.sh`
  supplies a private exact CNI 1.0 config/cache without altering host CRI
  configuration. It runs seven exact-source rollback/reconcile/reuse tests 20
  times under `-race`; performs a real `ADD` against a missing named namespace
  so veth creation succeeds and the following move fails/rolls back; then runs
  a successful external-netns `ADD`, two `CHECK`s, namespace removal before
  `DEL`, repeated idempotent `DEL`, and same-name reuse. Reuse must retain the
  released address but change the generation. It captures every config/env/
  argv/stdout/stderr/status plus durable endpoint/cache, primary and namespace
  link/address/route, iptables/NAT, and final resource state. Local Bash syntax,
  ShellCheck when present, and diff checks pass; mode/size/SHA-256 are `0755`,
  8,369 bytes, and
  `4b6ce3783cdc947ffd3e515a4fcead1a3a61ce34efc630f8252db838d7ce894f`.
  Live execution remains required before the row changes state.
- Guest upload `/tmp/test-runtime-cni-faults-live-4b6ce378.sh` independently
  matches mode `0755`, size 8,369, and full SHA-256
  `4b6ce3783cdc947ffd3e515a4fcead1a3a61ce34efc630f8252db838d7ce894f`;
  guest syntax ends `GUEST_CNI_VERIFY_PASS`. This is transfer proof only.
- First CNI attempt is preserved as `g5-cni-faults-live-first.log`: mode
  `0600`, 30,713 bytes, SHA-256
  `204da70e89c8c406dde6c4a75e07e18e3495b5df944252619a0ec6eca1299d54`,
  exit 1, credential-pattern clean. It proves exact guest provenance, private
  mode-0600 config identity, and 20 race-detector repetitions of all seven
  named source cases; all pass, including every Linux ADD command rollback,
  durable allocation/final-persist failure, allocating/deleting restart
  reconciliation, repeated CHECK/DEL/name reuse, and cache-quarantine recovery.
  It stops before the first live CNI call because Bash expands `label` inside
  the same `local` declaration under `set -u`; the trap reaches the same bug.
  Exact inspection finds only scratch directory
  `/var/tmp/mk-cni-faults.SCB9OC`, its 142-byte mode-0600 config, and an empty
  root-owned mode-0700 cache. Endpoints, test namespaces, `mkv*` links, MK
  rules, and 172.31 NAT rules are zero. That exact validated scratch directory
  is removed and absence confirmed. The declaration is split; local Bash,
  ShellCheck when present, and diff checks pass. Corrected mode/size/SHA-256
  are `0755`, 8,376 bytes, and
  `6d3372be3f421d28a1e919aa1b359d7a169526f6a00f2828207f11bfafb5b289`.
  Source fault coverage is now substantiated, but no live CNI behavior is
  claimed from this attempt.
- Corrected upload `/tmp/test-runtime-cni-faults-live-6d3372be.sh`
  independently matches mode `0755`, size 8,376, full SHA-256
  `6d3372be3f421d28a1e919aa1b359d7a169526f6a00f2828207f11bfafb5b289`,
  and guest syntax (`GUEST_CNI_CORRECTED_VERIFY_PASS`). Live retry follows.
- Second CNI attempt `g5-cni-faults-live-second.log` is retained at mode
  `0600`, 42,436 bytes, SHA-256
  `13f2b6e0f8c38175c1a8f91829d95fdedd026f60db6fefec908b557546f07276`,
  exit 1, credential-pattern clean. All seven exact-source cases again pass 20
  race-detector repetitions. The installed plugin reaches the real partial-ADD
  boundary: host-veth creation succeeds, the peer move into the deliberately
  absent namespace fails with CNI code 100, and rollback leaves endpoints,
  cache, links, routes, chains, NAT rules, and test namespaces all zero. The
  next real ADD succeeds with `172.31.0.2/30`, gateway `172.31.0.1`, and DNS
  `169.254.169.254`; the harness then fails because persisted JSON omits the
  false `managed_namespace` field while its parser requires that key. Its trap
  issues both DELs and removes the namespace and exact scratch tree.
- Independent post-failure audit `g5-cni-faults-second-post-failure-audit.log`
  passes clean Kerf, all 19 counters zero, and four active zero-restart
  services: mode `0600`, 1,907 bytes, SHA-256
  `3008c6557765e177357da9f113d95ca5d7d1e73ed7cc3bfe56d0c52395319701`,
  exit 0, credential-pattern clean. The evidence parser now interprets an
  omitted `managed_namespace` as false, matching the schema's `omitempty`
  encoding. Local syntax and diff checking pass; ShellCheck is unavailable.
  The corrected qualifier is mode `0755`, 8,386 bytes, SHA-256
  `7eda9ea25bf25bfb3a1098328efe96055ecebf9b4d752f71bf1479cd8a9d5721`.
  Product behavior is unchanged; the complete live matrix remains pending.
- Guest upload `/tmp/test-runtime-cni-faults-live-7eda9ea2.sh` independently
  matches mode `0755`, size 8,386, and full SHA-256
  `7eda9ea25bf25bfb3a1098328efe96055ecebf9b4d752f71bf1479cd8a9d5721`;
  guest syntax ends `GUEST_CNI_FINAL_VERIFY_PASS`. This is transfer proof only.
- `g5-cni-faults-live-third.log` is a preflight invocation error: the caller
  omitted required `SOURCE_ROOT`, so the script exits before scratch creation
  or any product operation. The retained mode-`0600`, 492-byte transcript has
  SHA-256 `9f3ffb468c21ebf9c2a80db60d49723e2ba780e29a2cba0b10bcea7f8aad0732`,
  exit 1, and an empty credential-pattern scan. It supplies no product claim;
  the corrected invocation passes `/var/tmp/mksrc-e4d7c9c` explicitly.
- Correct invocation passes as `g5-cni-faults-live-fourth.log`: mode `0600`,
  105,747 bytes, SHA-256
  `ef37e21c88e3b902d920b76290e45c06fbfc73b04d2534d4aee680dd6a6edccf`,
  exit 0, credential-pattern clean, terminal `G5_CNI_FAULTS_LIVE_PASS`.
  All seven exact-source cases pass 20 race-detector repetitions, including
  every Linux ADD command boundary and allocating/deleting reconciliation. The
  installed plugin's missing-netns ADD returns CNI code 100 and zeroes all
  seven focused inventories. Generation `826bedc1…` then reaches READY at MTU
  1400 and address `172.31.0.2/30`; two CHECKs return empty stdout/stderr and
  exit 0. After the namespace is deliberately removed, DEL and repeated DEL
  both return empty stdout/stderr and exit 0, and all focused inventories are
  zero. Same-name reuse obtains the same address in distinct READY generation
  `ee11cff6…`, its CHECK/DEL pass, and cleanup is again all zero. The embedded
  strict audit reports clean Kerf, all 19 counters zero, and four active
  zero-restart services. This live execution plus exact-source boundary tests
  closes the CNI partial-failure/repetition/stale-cleanup/reuse row.
- Separately invoked `g5-cni-faults-final-resource-audit.log` independently
  confirms the pass did not hide trap residue: mode `0600`, 1,907 bytes,
  SHA-256 `9998bad4799aec59f4fb1c8ec0e0fe138f8020fa3596a7d2ec53d138640a60bc`,
  exit 0, credential-pattern clean. It reports the same exact selector/boot,
  clean Kerf, all 19 counters zero, and four active zero-restart services.
- Commit `561880f2e7b2a7972596a2f9f76edfae81dc06ee` (`test: qualify G5 CNI
  fault recovery`) checkpoints the CNI qualifier, relay-identity evidence
  hardening, and both continuously updated learning records. At this checkpoint
  the checklist has 58 closed and 27 open rows.
- [x] Source spoofing, route injection, metadata-address access policy,
  forwarding-rule bypass, and sibling-link policy bypass.
  The already-passing two-sandbox qualifier now targets these remaining live
  boundaries without changing product policy. For both distinct children it
  records per-generation iptables counters before/after; rejects HTTP to the
  metadata address while leaving the already-proved DNS exception intact;
  installs explicit sibling `/32` routes through each legitimate gateway and
  requires reciprocal ping failure; adds distinct unallocated source `/32`s
  and attempts source-bound TCP tokens to a primary listener, requiring both
  client failure and a five-second no-accept observation. The original
  same-hostname/distinct-identity, legitimate token routes, default sibling
  rejection, normal cleanup, and strict audit remain unchanged. Local syntax
  and diff checks pass; ShellCheck is unavailable. Revised mode/size/SHA-256
  are `0755`, 13,331 bytes, and
  `069e6d6c4b9aff5d806cb28a9437f3439c3f670a1400d75346a39c3f9211a917`.
  This is implementation evidence only; the row remains open until the exact
  file runs on the disposable VM.
  Guest upload `/tmp/test-runtime-network-isolation-live-069e6d6c.sh`
  independently matches mode `0755`, size 13,331, full SHA-256
  `069e6d6c4b9aff5d806cb28a9437f3439c3f670a1400d75346a39c3f9211a917`,
  and syntax marker `GUEST_NETWORK_POLICY_VERIFY_PASS`.
  First policy replay is retained as
  `g5-network-policy-bypass-live-first.log`: mode `0600`, 45,630 bytes,
  SHA-256 `15b95d337b94a473e10ac97f6d1f869f8e76e2ec53e621634f9ca38901c23c93`,
  exit 2, credential-pattern clean. Exact provenance, distinct identities,
  positive primary routes, default sibling rejection, and both metadata HTTP
  rejections pass. The first explicit route mutation is rejected inside the
  default OCI process with `RTNETLINK ... Operation not permitted`, before a
  bypass packet is sent; this is a real least-privilege barrier but not proof of
  the primary firewall under a privileged workload. Trap cleanup converges.
  Post-failure audit plus CLI capability inspection is mode `0600`, 2,452
  bytes, SHA-256
  `15e5b96a0157abafd38c6f026714f1d9e80391d71cc22abe847eac5f5bd44331`,
  exit 0, credential-pattern clean: clean Kerf, all 19 counters zero, four
  healthy services, and `ctr run` explicitly supports `--cap-add`. The retry
  grants only `CAP_NET_ADMIN` to both disposable test workloads so mutations
  reach the primary policy; it does not alter host or product configuration.
  The capability-scoped qualifier passes local syntax/diff checks and is mode
  `0755`, 13,379 bytes, SHA-256
  `7104931bc239b3b9707f9e1b69a566548fd70427087161e94ef98d57f16a2b44`.
  Guest upload `/tmp/test-runtime-network-isolation-live-7104931b.sh` matches
  mode `0755`, size 13,379, full SHA-256
  `7104931bc239b3b9707f9e1b69a566548fd70427087161e94ef98d57f16a2b44`,
  and syntax marker `GUEST_NETWORK_POLICY_CAP_VERIFY_PASS`.
  Capability-scoped second run passes as
  `g5-network-policy-bypass-live-second.log`: mode `0600`, 77,112 bytes,
  SHA-256 `4df3969bbd7128b8700349ddc40635757a3dbfb12fdcca991c289f61c62812f4`,
  exit 0, credential-pattern clean, terminal
  `G5_NETWORK_ISOLATION_LIVE_PASS`. Both privileged guests successfully install
  explicit sibling `/32` routes and distinct `198.18.0.1/32`/`.2/32` source
  aliases. Reciprocal routed pings still lose 100%; each chain's sibling-drop
  counter advances from one to two packets. Both metadata HTTP requests fail
  with connection refused and each metadata REJECT counter advances 0→1. Both
  source-bound TCP attempts exit 1 and the primary listener records
  `{"accepted": false}`. Their primary anti-spoof chain counters remain zero,
  showing those spoofed flows were rejected before that host-chain rule rather
  than attributing an unobserved hit to it; exact-source tests separately prove
  the `/32` rule installation/CHECK contract. Legitimate token routes from both
  assigned sources still pass first. Cleanup reaches zero endpoints/roots/live
  exports and the embedded strict audit passes all 19 counters after bounded
  pool release. Independent `g5-network-policy-final-resource-audit.log` is
  mode `0600`, 1,909 bytes, SHA-256
  `6673daf0806a70e4722e2ffb4e5472ac30fef0ce3dea316d5d895df03b2c8c3b`,
  exit 0, credential-pattern clean, again proving clean Kerf, all counters
  zero, and four active zero-restart services. This closes the policy-bypass
  row without claiming an anti-spoof counter hit that was not observed.
  Commit `cd3e5c9ef6ff26229cdff122d1e4c8a15248dfef` (`test: qualify G5
  policy bypass rejection`) checkpoints the expanded qualifier and evidence
  narrative. The checklist now has 60 closed and 25 open rows.
- [x] Before/during/after checks for primary SSH, metadata access, guest agent,
  default route, NIC PCI ownership, and boot-disk/NIC controller ownership.
  Current-boot preflight identifies the exact stable objects the three-phase
  check must bind: active `ssh.service` and `google-guest-agent.service`; two
  port-22 listeners; default route `via 10.148.0.1 dev ens4 ... src
  10.148.0.58`; NIC `ens4` at PCI `0000:00:04.0/virtio1` bound to
  `virtio_net`; root `/dev/sda1`, base disk `sda`, beneath PCI
  `0000:00:03.0/virtio0/...` bound to the SCSI `sd` driver. A primary metadata
  request returns HTTP 200, 19 bytes, and `Metadata-Flavor: Google`. The ad-hoc
  probe's shell-escaped SHA extraction fails and is not claimed; the qualifier
  will use direct file hashing and compare one normalized snapshot before,
  while two children are active, and after cleanup.
  The qualifier now implements that normalized snapshot with direct metadata
  body SHA-256, response status/flavor/size, the live SSH session plus listener
  count, guest-agent PID/executable, exact default route, NIC device/driver,
  and root-source/base-disk device/driver. It requires byte-identical snapshots
  before task creation, after all privileged policy attempts with two children
  live, and after normal cleanup plus strict audit. Local syntax/diff checks
  pass; mode/size/SHA-256 are `0755`, 15,560 bytes, and
  `7cecd2cc5879ebfe4c41ad9c81cf0eadef9628522d19aefa28cd3d3e273f7520`.
  Live execution remains required.
  Guest upload `/tmp/test-runtime-network-isolation-live-7cecd2cc.sh`
  independently matches mode `0755`, size 15,560, full SHA-256
  `7cecd2cc5879ebfe4c41ad9c81cf0eadef9628522d19aefa28cd3d3e273f7520`,
  and syntax marker `GUEST_PRIMARY_HEALTH_VERIFY_PASS`.
  First three-phase attempt `g5-primary-health-live-first.log` is a
  pre-observation harness failure: mode `0600`, 13,524 bytes, SHA-256
  `1be67cf4d477fe5b09e8c4df0d8932e50dd4bfe3b707a90c2a82feaa58e4e27d`,
  exit 1, credential-pattern clean. Exact provenance and zero-resource
  preflight pass; Bash then expands `phase` inside its own `local` declaration
  under `set -u`, before metadata access or task creation. The trap confirms
  both task identities absent. Splitting the declaration fixes only the
  qualifier; no product claim follows from this attempt.
  Corrected mode/size/SHA-256 are `0755`, 15,575 bytes, and
  `4e08b01af8f49af0cfb74610bfb3725a95ad4f8972b9bb3681f46fc5a9834923`;
  local syntax and diff checks pass.
  Guest upload `/tmp/test-runtime-network-isolation-live-4e08b01a.sh` matches
  mode `0755`, size 15,575, that full digest, and syntax marker
  `GUEST_PRIMARY_HEALTH_CORRECTED_VERIFY_PASS`. Corrected run passes as
  `g5-primary-health-live-second.log`: mode `0600`, 103,165 bytes, SHA-256
  `47db93520a48e0c71098b6f13342872289b73632e5980df05e8ee6be8939129d`,
  exit 0, credential-pattern clean. Before, during two active privileged
  children after every policy attempt, and after cleanup, the normalized
  snapshot is byte-identical: current SSH connection/two listeners, guest-agent
  PID 1079, primary metadata 200/Google/19-byte body SHA `2887acc3…`, exact
  default route, NIC PCI/virtio path and driver, and root-disk controller path
  and driver. Independent final audit is mode `0600`, 1,909 bytes, SHA-256
  `425dd620b036de24dcb5086bad0ea79b70a4ae68231c0a41e34bdec25864d638`,
  exit 0, credential-pattern clean, with all 19 counters zero. This closes the
  three-phase host-health/controller-ownership row.

### Replacement instance evidence required

- [x] Retain CNI stdin/config, command argv, stdout/stderr, exit status, primary
  namespace/link/route/rule state, child interface state, negotiated MTU, and
  packet counters for each `ADD`, `CHECK`, and `DEL`.
- [x] Retain successful DNS, TCP, and UDP exchanges plus failed bidirectional
  sibling attempts. A bare `network-ok` or failed `ping` marker is insufficient
  for the final gate.
  Retained flow transcript `a380aaf7…` contains tokenized TCP and UDP
  request/reply pairs observed at primary `10.148.0.58`, the exact configured
  resolver and returned A/AAAA answers, plus external HTTP details. Retained
  isolation transcript `de553bbb…` contains both child identities and complete
  A→B/B→A ping output with exit 1, one transmitted, zero received, and 100%
  loss. The newer policy transcript `4df3969b…` independently repeats
  legitimate token routes and both default and injected-route sibling failures.
  These are response-bearing/raw observations, not summary markers, and close
  this evidence row.
- [x] Retain pre-run and post-run NIC/controller ancestry and final empty
  namespace, TUN/TAP, route, iptables/nftables, process, and recovery-record
  inventories.
  The passing transcript retains pre/during/post ancestry. The first dedicated
  final-network wrapper is a quoting failure before observation (mode `0600`,
  1,985 bytes, SHA `37e58a96…`, exit 1) and is not evidence. Shell-safe retry
  `g5-primary-health-final-network-inventory-second.log` passes at mode `0600`,
  2,303 bytes, SHA-256
  `4d88750064f63370f1897bff1c4dc22790a6333af54c26501be7a22ad59cd67b`,
  exit 0, credential-pattern clean: namespaces, managed/TUN/TAP links, routes,
  iptables filter/NAT and nftables matches, shim/relay/NBD processes, endpoints,
  and rootfs records are all zero; NIC/root controller identities remain exact.
  Commit `dadb104f2df04878d4d675ce51276ba9eb7bfca0` (`test: qualify G5
  primary host continuity`) checkpoints the qualifier and both evidence
  narratives. The checklist now has 63 closed and 22 open rows.

## G6: containerd Runtime v2 shim

### Implementation still required

- [x] Preserve and reconnect a running task after forced shim death. Safe
  reclaim is a useful fallback but is not the plan's reconnect requirement.
  Focused reconstruction now proves that an exact daemon-owned sandbox and
  network generation restore the recorded live guest PID, authenticated agent
  identity, relay/network ownership, output/wait loop, and subsequent exact
  exit completion. An injected first `StateProcess` reply loss now requires one
  relay reconnect and restores the exact PID in 100 race-detector repetitions;
  reconstruction state and stopped-state wait reads use the same bounded
  idempotent path. Reconstruction cleanup ownership begins immediately after
  network-descriptor acquisition; a forced relay-start failure proves the
  descriptor, command, and socket identity are released. Forced-death process
  continuity is proven by exit-0 transcript
  `g6-forced-shim-reconnect-fifo-pass.log` (`fe6eee0d…`): supervisor remains
  stable, worker and holder are replaced, Task/child/recovery/network identity
  remains exact, post-fault exec works, and the same attach stream contains all
  pre/post stdout/stderr before ordered exit/delete and all-zero release.
- [x] Define ownership transfer for containerd restart, shim restart, daemon
  restart, and shutdown. Reconstruct process state, stdio endpoints, exit
  status, and event delivery without changing the child boot identity. Task
  `Shutdown` now refuses to terminate while any process record remains, then
  atomically seals an empty service against Create, requires the durable event
  journal to flush, and joins event retry before invoking shutdown once. A
  failed flush leaves shutdown retryable. Agent relays are now explicit shim
  ownership: every start uses a dedicated process group, every connection or
  reconstruction failure reaps that group, normal Delete stops it only after
  authenticated `Quiesce` and the terminal guest Shutdown attempt, and a failed
  socket removal retains its path for retry. `Quiesce` is idempotent across
  reconnect, runs storage quiescence once, and seals later mutation. Injected
  network-close reply loss, quiescence-reply loss, terminal-reply loss, invalid
  acknowledgement, authenticated rejection, and post-quiescence cancellation
  pass paired 100-iteration race-detector matrices.
  The mkruntimed lifecycle snapshot and append journal now use the
  same descriptor-anchored, private, caller-owned state directory contract as
  the G4/G5 ownership stores. Snapshot and journal reads are bounded and
  strict; journal creation is exclusive and directory-synchronized; entries
  are individually bounded with contiguous sequences; append exhaustion is
  explicit; and a replaced directory or journal pathname fails before a
  substitute is written. Snapshot persistence failures now restore the
  corresponding in-memory ownership mutation. Focused malformed-input,
  capacity, hardlink/mode, and post-open replacement tests pass. Focused
  descendant and socket-cleanup tests pass. The mkruntimed listener uses the
  same descriptor-anchored exact-inode socket lifecycle as mknetd and retains
  its allowed-peer UID ownership check. The complete
  Cross-process transfer is now directly proven by focused containerd
  (`afa9187e…`), mkruntimed (`e99f610e…`), forced-shim reconnect (`fe6eee0d…`),
  and clean shutdown/recreation (`754a05f6…`) transcripts: durable process,
  child, stdio, event, recovery, and final ownership identities are retained at
  each applicable boundary.
- [x] Implement faithful guest PID reporting or define a versioned virtual PID
  mapping. `Start`, `State`, `Pids`, `Connect`, exit, and delete now report the
  guest PID under mapping version `multikernel-v1-guest-pid`; the pre-start
  Create response retains the supervisor PID required by the Task v2 launch
  handshake.
- [x] Implement and test Task `Stats`, `Update`, and `Checkpoint`, or revise the
  advertised G6 surface and plan explicitly. `Pause`, `Resume`, and `Stats` are
  implemented with focused tests; pause/resume transact across every live init
  and exec process group, while Stats aggregates their CPU, RSS, and PID counts
  with overflow rejection. Stats now reconnects and replays the same
  idempotent per-process observation after an injected lost reply; the exact
  aggregate and one reconnect pass 100 race-detector repetitions. Plan 06 now
  explicitly excludes `Update`
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
- [x] Harden FIFO handling for peer disappearance, attach/detach churn, blocked
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
  mode changes, regular-file stdin, and cancelled opens. Recovery format v3
  now persists the immutable device/inode/owner/group/mode/link identity
  captured before Create or Exec mutation, and Start/reconstruction refuse a
  same-type replacement instead of trusting the recovered pathname. Legacy
  v1 records without that proof fail closed. Twenty race-detector repetitions
  cover FIFO and regular-output substitution without modifying the replacement.
  A real FIFO test covers no initial stdin peer, an attached writer idle long
  enough for the nonblocking reader to return `EAGAIN`, a subsequent writer,
  exact guest-call bytes, and prompt pump termination when its descriptor is
  torn down. The pump now treats that empty attached state as temporary rather
  than silently abandoning later input; 100 race-detector repetitions pass.
  `CloseIO` also bounds the opposite case: after its durable request, the pump
  forwards at most the read already in flight, acknowledges guest closure, and
  performs no subsequent read even if the peer continuously supplies input.
  The controlled cutoff test passes 100 race-detector repetitions.
  Output polling and final Wait now treat only
  transport/protocol failures as reconnectable, replay their idempotent reads
  with unchanged acknowledged offsets, and share one 30-second overall budget;
  a structured authenticated agent rejection terminates that exchange without
  reconnect or replay. Twenty
  race-detector repetitions cover successful output/Wait reconnect, an initial
  reconnect failure, non-replayed remote rejection, and deadline exhaustion.
  Live `ResizePty` and reconstruction now apply the same bounded reconnect
  rule to the exact process-ID/width/height set operation. An injected lost
  successful reply reconnects once, replays the same request, and retains the
  new durable dimensions; guest rejection retains the prior dimensions. The
  focused resize matrix passes 100 race-detector repetitions.
  Exhausting one bounded background epoch no longer fabricates exit 255: the
  recorded process remains running and its wait channel remains open while the
  monitor retries after 100 milliseconds. Only an authenticated `WaitProcess`
  result transitions it to stopped. A sustained-outage/recovery test observes
  the later exact exit 37 and passes 100 race-detector repetitions.
  Each output reply is also rejected before delivery or durable mutation if a
  stream exceeds the requested 4096-byte chunk, either next offset is not the
  exact non-overflowing request-plus-length value, or state is unknown. Focused
  malformed-reply cases pass 100 race-detector repetitions.
  Complete delivery now advances both offsets in one durable recovery update;
  publication failure restores both old values and retries the same chunk under
  an explicit at-least-once contract. Injected failure requests offsets 0, 0,
  then 4 after repair and reaches exact exit 11 in 100 race-detector
  repetitions. Empty unchanged polls do not rewrite recovery state.
  Observed exit completion is also ordered after durable stopped state, exact
  code, and timestamp. Missing recovery storage retains the prior running state
  and open Task wait channel; after repair, disk records `STOPPED` and exit 37
  before completion. The injected boundary passes 100 race-detector repetitions.
  If the later exit-event flag cannot be persisted, memory rolls it back to
  false to match that durable stopped record, so Delete/reconstruction repairs
  via at-least-once replay. The post-publication failure passes 100
  race-detector repetitions with exact exit 37 retained.
  Exec creation rollback after recovery/event failure now uses a five-second
  cancellation-independent guest delete, treats authenticated `NOT_FOUND` as
  confirmed absence, retains ownership on other delete failures, republishes
  the resulting registry, and returns all cleanup errors. Success, absence, and
  failure cases align memory and disk in 100 race-detector repetitions. The
  CREATED exec owner is now durable before `ExecProcess`; reply-loss
  reconciliation accepts exact CREATED, removes confirmed absence, and retains
  ownership when state and deletion remain uncertain. Reconstruction applies
  the same exact identity/state boundary and at-least-once exec-added event
  repair. Both three/five-boundary matrices pass 100 race-detector repetitions.
  Agent connection cancellation no longer leaves a goroutine and connection
  reference behind after each normal disconnect. Focused tests prove active
  cancellation still unblocks the session and completed sessions unregister
  their callback; 100 race-detector repetitions pass.
  Stdin now uses the advertised `stdin-offset-v1` contract: recovery v3
  persists each bounded pending chunk before guest mutation, the agent accepts
  only the exact next offset, and an identical most-recent offset/length/SHA-256
  replay is acknowledged without a duplicate write. The shim clears pending
  bytes only after durably recording the returned offset. A local partial write
  retains its accepted prefix and an exact replay resumes with only its
  unaccepted suffix before advancing the acknowledged offset. Focused agent,
  protocol, and shim tests cover ordered writes, changed/gapped rejection,
  lost-response reconnect (including post-exit acknowledgement), publication
  failure before guest contact, acknowledgement persistence failure, bounded
  recovery input, partial local-write continuation, and offset-free v1
  compatibility.
  `CloseProcessStdin` acknowledgement also reconnects within the earlier Task
  caller deadline and shared 30-second I/O bound; focused injection proves the
  requested state survives transport loss and the retry becomes acknowledged.
  The aggregate is now revalidated by the zero-skip disposable-VM race suite
  `6fb8ffff…`, the shared live attach/resize matrix `8ee2f800…`, and forced
  worker-death FIFO continuity transcript `fe6eee0d…`.
- [ ] Enforce context cancellation and deadlines through rootfs mount,
  initramfs build, daemon calls, child boot, agent connect, stdio, wait, and
  teardown without leaking resources. The shared daemon client now applies
  the earlier of a caller deadline and a 30-second default across dial, write,
  and read, and cancellation actively interrupts an already-connected socket.
  Focused tests cover pre-dial cancellation, blocked request writes, blocked
  response reads, the default timeout, and a successful round trip. Auditing
  also rejects unknown/duplicate envelope and typed-body fields, mismatched
  response versions or request IDs, missing typed bodies, and a valid response
  prefix followed by data beyond the complete one-MiB bound. The mknetd client
  applies the same exact overflow rejection; ATTACH additionally requires a
  complete newline-terminated descriptor response and rejects truncation.
  Fault-injecting every remaining boundary is still
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
  Wait's post-exit state snapshot now reacquires the task lock through the
  same cancellation-aware path; a focused completed-process contention test
  proves cancellation cannot strand the waiter behind concurrent teardown.
  Shared protocol writes complete across injected short-success transport
  wrappers and fail on zero progress. Focused agent framing covers both
  directions; daemon and mknetd tests cover their request/response paths.
  Daemon and mknetd service return also cancel derived request handlers and
  close incomplete accepted peers; focused blocked-request tests pass 100
  race-detector repetitions.
  Both accept loops have a bounded 128-handler default and 1,024 hard maximum,
  close over-budget connections, and join admitted handlers. Exact serve-loop
  saturation/cancellation tests pass 100 race-detector repetitions.
  Daemon replies now enforce the one-MiB client bound and replace oversized or
  unencodable bodies with a bounded request-ID-bound INTERNAL error. Agent
  replies require exactly one body/error and strict typed-body decoding;
  unknown/duplicate fields, missing body, and body-plus-error all fail. The
  adversarial response groups pass 100 race-detector repetitions.
- A source-controlled live qualifier now targets the remaining cross-service
  evidence boundary without modifying the installed generation. New
  `runtime-cancellation-blocker.sh` passes validation/non-target Kerf calls to
  the exact real executable, but blocks the rootfs build or Kerf `load` with a
  deliberately signal-resistant descendant. New
  `test-runtime-cancellation-live.sh` runs `mkruntimed` temporarily under a
  collected systemd unit with the blocker selected, cancels the real ctr Create,
  requires both blocker processes to be reaped, and requires the strict
  all-zero audit after each rootfs-build and child-boot boundary. It then
  cancels live Task Wait and attach calls, requires the same running task PID,
  proves a post-cancel exec, deletes normally, and repeats the strict audit.
  Its trap stops the transient unit, restores the official service, and removes
  qualification assets on every exit. Bash syntax, available ShellCheck, and
  diff hygiene pass; blocker/qualifier are mode 0755, 829/6,708 bytes, SHA-256
  `60ee299811c457db22923f47e1cddc0971dbc0f81766d6f4408ad2c16569488d` /
  `524f8d5bcb6ccb264127878d5ec3f7cf4e2de90fa03e03afb79e7681447115bc`.
  This is harness design evidence only; the live cancellation row remains open
  until an exact-source VM run and independent cleanup audit pass.
- Commit `af132d8b75a8a08244ac39a45718a6087180cd6a` freezes the cancellation
  qualifier and blocker. Its clean 6,696,960-byte source archive
  `/tmp/mksrc-af132d8.tar` hashes to
  `4e91114549ad13ceb079558f9b1228d8e65a18c9c9659fa18281c4829fb9201d`.
  This binds the pending transfer; no live result is yet claimed.
- Guest preflight transcript `20261008-g6-cancellation-source-preflight.log`
  is mode 0600, 17,196 bytes, SHA-256
  `fda143bdfb4761e32076881facc101dd8fcbe1ddd05d4420a6977835e0b1ea71`,
  and exit 0. The VM independently matches archive SHA-256 `4e911145…`,
  extracts 667 files/6,145,091 bytes into fresh `/var/tmp/mksrc-af132d8`,
  matches blocker/qualifier hashes `60ee2998…`/`524f8d5b…`, and passes guest
  Bash syntax. Its strict starting audit reports no pool/instances, all 19
  counters zero, and unchanged active zero-restart services. Transfer and clean
  baseline are proved; cancellation behavior is not yet claimed.
- First exact-source cancellation run is retained as
  `20261008-g6-cancellation-live-first.log`, mode 0600/73,262 bytes/SHA-256
  `1f59d9f4cd187d40c270c5d1cb6253075c16e18d3f4ee7d7524cc0f44d0e82c5`,
  exit 1 and credential-pattern clean. It reaches the rootfs-build blocker and
  cancels the real ctr Create, but blocker PID 79086 remains visible for the
  complete 60-second assertion window. The trap restores the official daemon
  and removes qualification paths, but this is not a pass.
- The first independent audit
  `20261008-g6-cancellation-first-failure-audit.log` is mode 0600/9,129
  bytes/SHA-256
  `94c5f2d274a2051ac513cff25525eb7e6882fc3857d890c0e2a47d9bb7c6e4cd`.
  Its strict audit actually fails with `shim_processes=2` despite every other
  counter, Kerf instance, and pool being zero; later diagnostic commands mask
  the wrapper exit to 0, so it is explicitly classified failed. Corrected
  diagnostic `20261008-g6-cancellation-first-leak-diagnostic-corrected.log`
  is mode 0600/8,435 bytes/SHA-256
  `17e1300502db4c606d7a58316d31fb6925663c8e4e6df3a61324b0dcddf25f9d`,
  exit 0. It proves live sleeping supervisor/worker PIDs 79062/79067 for exact
  task `mk-cancellation-live`, not zombies: parentage 1→79062→79067, exact
  installed binary/argv, 4/7 threads. Containerd records Delete deadline
  exceeded, shim disconnect, then fallback delete failure because its bundle
  working directory was already absent. The earlier diagnostic transcript is
  retained separately as a mode-0600/823-byte/SHA `38ce2b34…` harness failure
  caused by nested awk expansion. Product/harness remediation and cleanup are
  required before rerun; the cancellation row remains open.
- Source diagnosis identifies the leak in the proxied supervisor: after starting
  `bridge.serveWorker` asynchronously it blocked synchronously in `cmd.Wait`, so
  a containerd EOF could not be consumed and the idle worker had no reason to
  exit. The correction races worker completion against proxy completion; a
  proxy-first result closes the private connection, kills/reaps a still-live
  worker, removes its identity-bound PID file, and exits without treating the
  deliberate reap as a recoverable worker crash. A process-level regression
  holds an actual worker and closes the containerd side, then requires bounded
  supervisor exit, absent worker PID, and absent PID file.
- The first local focused test command never compiled because the managed
  default Go cache is read-only. Its private-cache retry compiled the fix but
  the new process test hit the known local abstract-Unix-socket `EPERM`; its
  timeout then produced a test-only directory-close race. The test now probes
  abstract-listener capability and skips before spawning any goroutine when it
  is unavailable. This is not pass evidence; private-cache validation and a
  zero-skip VM run remain required.
- Cleanup transcript `20261008-g6-cancellation-first-leak-cleanup.log` is a
  retained mode-0600/1,227-byte/SHA `77457ff5…` harness failure: leading `-id`
  was parsed as a grep option, so no signal was sent. Corrected
  `20261008-g6-cancellation-first-leak-cleanup-corrected.log` is mode 0600,
  14,853 bytes, SHA-256
  `2f9ad273ccdd793df98e06241a2695c6695a39e3cb03e39a8fee4ed3e8a743e4`,
  exit 0. It first binds both PIDs to exact `-id mk-cancellation-live` argvs,
  sends SIGTERM only to supervisor 79062, observes 79062/79067 disappear, and
  then proves no pool/instances, all 19 counters zero, official mkruntimed PID
  79465 at the exact installed hash, and four active zero-restart services.
- After the abstract-listener preflight correction, the private-cache focused
  race matrix passes 20 repetitions in 24.042 seconds. The new process test is
  cleanly skipped only under the local abstract-socket restriction; bridge
  replay/replacement and signaled-worker restart tests pass. This validates the
  correction locally without substituting for the required zero-skip VM run.
- The supervisor correction now gives a disconnected worker up to 30 seconds
  after closing private TTRPC to return normally, preserving Create's context-
  cancellation rollback; only a worker that exceeds that bound is SIGKILLed
  and reaped. The regression injects 100 ms for its intentionally non-serving
  worker. The corrected four-test focused matrix passes 20 race repetitions in
  24.067 seconds; the complete shim package passes under `-race` in 9.985
  seconds, followed by package vet, Bash syntax, and diff hygiene. These local
  checks validate the implementation but do not prove the installed live path;
  an exact candidate build/activation and zero-skip replay remain required.
- Commit `385f019905d0309065f065288117df6ecf9914fe` freezes the diagnosed
  supervisor fix, regression, failed-run evidence, and exact cleanup evidence.
  Clean archive `/tmp/mksrc-385f019.tar` is 6,850,560 bytes with SHA-256
  `99bb1941082a855e8cf895141d3d314ecb677b7c68b96479a1dc615d914386c0`.
  Candidate transfer/build/activation remain unclaimed.
- First `385f019` VM build gate is retained as
  `20261008-g6-cancellation-build-385f019.log`, mode 0600/14,177 bytes/SHA-256
  `df4b0ec6d6f081b89b014959f3f3ff65300c840c8847b86fb56187e0ed1cd16a`,
  exit 1, with zero skips/race reports and one top-level failure. Nineteen
  process-level disconnect repetitions pass, but one reads the marker after
  file creation and before PID bytes are written, producing `Atoi("")`. The
  gate stops before any build or install, so the active release remains
  unchanged. The regression now waits for a nonempty complete PID record.
- Commit `cacec0db6fef21a5721bcdef558dc30703867d89` freezes that test-only
  correction and failed gate transcript. Its clean 6,860,800-byte archive
  `/tmp/mksrc-cacec0d.tar` hashes to
  `b16af3c929a1fe4d34163521fa055799e65fa6e31a155eae5e9ed720ec4b7f79`.
  The corrected local focused matrix passes 20 repetitions in 24.059 seconds;
  transfer and VM retry remain unclaimed.
- Strengthened VM gate/build transcript
  `20261008-g6-cancellation-build-cacec0d.log` is mode 0600/63,147 bytes,
  SHA-256
  `1e16427487eab78a2021d7dce4ef1f09bd0cb9e23b8e4c157335f17476e7be8e`,
  exit 1. Its product gate passes 100 race-detector repetitions in 128.518
  seconds: 400 passing groups, zero failures/skips/race reports. All seven
  revision-stamped binaries build and `make runtime-manifest` creates the
  manifest. A redundant second exclusive-create generator then correctly
  refuses the existing manifest with `EEXIST`; install is never invoked and
  the active release is unchanged. This is a build-command harness failure,
  not an installed candidate or live pass.
- Corrected activation transcript
  `20261008-g6-cancellation-activate-cacec0d.log` is mode 0600/17,873 bytes,
  SHA-256
  `91853bab6940d074e2c2c6c104fe9ef24e05d835b6557d8bc482d59f2f729315`,
  exit 0, and passes the credential-value scan. It installs and selects exact
  release `0.1.0-dev-cacec0db6fef21a5721bcdef558dc30703867d89`; the installed
  shim and source shim both hash to
  `3d1b1b9ff843e7e9ac4f65ffb3a10a415165b4c707db223961c52fa1f866a3ed`.
  On boot `0404118a-b8a8-4187-89fa-2e45529963c2`, restarted idle
  mkruntimed/mknetd and unchanged containerd/Docker are active/running with
  zero restarts. The strict post-activation audit finds no Kerf pool or
  instances and all 19 tracked resource counters zero. This proves exact
  candidate activation and a clean baseline only; the cancellation row stays
  open until the live qualifier and independent post-run audit pass.
- Exact-source live transcript `20261008-g6-cancellation-live-cacec0d.log` is
  retained mode 0600/73,286 bytes, SHA-256
  `2bfba8a852054ebd6884068c7c2a5e798fb0740baf58a22eec938e07d811e25b`,
  exit 1, with a clean credential-value scan. It proves the clean exact-release
  preflight and reaches the rootfs builder blocker (parent 16712, deliberately
  TERM-ignoring child 16715). Canceling the actual `ctr run` returns nonzero
  status 124, but neither blocker PID disappears within 60 seconds, so the
  qualifier correctly stops before the child-boot and Task Wait/Attach cases.
  This is a remaining live cancellation failure, not pass evidence.
- Immediate read-only diagnostic
  `20261008-g6-cancellation-cacec0d-failure-diagnostic.log` is mode
  0600/16,423 bytes, SHA-256
  `503e92dc176e797cdfb988f60f76fafa2a4370eb16afec2336073015de07b9c9`,
  exit 0, with a clean credential-value scan. By collection time the trap's
  transient-service stop had removed both blocker PIDs. Containerd records a
  Delete deadline at 13:28:56, shim disconnect at 13:29:01, and failed fallback
  delete due to the already-removed bundle CWD. The restored official daemon
  is exact candidate PID 17091; all four services are active/running with zero
  restarts, no Kerf pool/instances exist, and all 19 resource counters are
  zero. Thus cleanup after harness restoration is proved, but cancellation
  before service teardown is not; the row remains open.
- Root cause is the mkruntimed daemon wire contract, not the already-correct
  bounded command runner: the client half-closed its write side to delimit JSON,
  so the server consumed EOF before dispatch and had no later socket signal
  from which to derive caller cancellation. The client now newline-frames the
  request without half-closing; the server bounds and parses that frame, keeps
  accepting the former EOF-delimited form, and cancels the per-request dispatch
  context when the framed caller disconnects. This propagates through
  Rootfs.Prepare/Kerf into `boundedexec`, which already SIGKILLs the complete
  private process group on context cancellation.
- Focused end-to-end tests cover both direct framed disconnect and actual
  `daemon.Client` context cancellation reaching a blocked server dispatch.
  Both pass 100 race-detector repetitions. The complete daemon, rootfs,
  lifecycle, mkruntimed, and shim package race suites pass, followed by vet and
  diff hygiene. These local results validate the correction but do not close
  the live row; an exact candidate build, activation, and full live replay are
  still required.
  The retained transcript `20261008-g6-daemon-disconnect-local-race.log` is
  mode 0600/1,369 bytes, SHA-256
  `d190575d88cc589f014864fc6d34d2a5b2a82e6eec4cee4b4d96ef78300e478e`,
  exit 0, with a clean credential-value scan.
- Commit `4fed167` (`runtime: propagate daemon client cancellation`) freezes
  the daemon protocol correction, regressions, exact failed-live diagnostics,
  and contemporaneous findings. Its tracked-source archive
  `/tmp/mksrc-4fed167.tar` is 7,055,360 bytes with SHA-256
  `db5110f342fb9ea2fe1ff93443193f766576e975cb72251832d05e629eb042d4`.
  Transfer transcript `20261008-g6-cancellation-transfer-4fed167.log` is mode
  0600/353 bytes/SHA-256
  `9b6206270181e435fee9f3d1703f72b3be642e84eb64fe1826528485868b9f0b`,
  exit 0. Guest preflight
  `20261008-g6-cancellation-source-preflight-4fed167.log` is mode 0600/1,422
  bytes/SHA-256
  `94cb2dea0892b7706f3145041b023101bc1b8c524f4537febf40d809337765de`,
  exit 0. Both pass the credential-value scan; the guest independently matches
  the archive digest, hashes the two daemon sources and unchanged qualifier
  scripts, passes Bash syntax, and emits the preflight marker. Build and
  activation remain unclaimed.
- First `4fed167` VM build-command transcript
  `20261008-g6-cancellation-build-4fed167.log` is retained mode 0600/1,935
  bytes/SHA-256
  `e98dc7c00e94185d30ad52984208af9da15f4079768bd58482d4ab7420e2acb0`,
  exit 1, with a clean credential-value scan. Nested shell quoting expands the
  archive-check awk `$1` under `set -u`; the gate stops before tests, build,
  install, or activation. No product result is inferred.
- Corrected build-gate transcript
  `20261008-g6-cancellation-build-4fed167-corrected.log` is retained mode
  0600/30,687 bytes, SHA-256
  `97b90135a67a350e8f217301907d9a383ae15756fc51e39f58b0f7ea30f426ee`,
  exit 1, with a clean credential-value scan. Both new daemon disconnect tests
  pass all 100 race repetitions. The shim disconnect regression passes 19 of
  20 repetitions; one observes a successfully opened but still-empty marker
  and falls through to `t.Fatal(nil)` at line 6322. The earlier marker fix only
  gated parsing and did not continue the poll on this state. The gate stops
  before full-package tests, build, install, or activation; no product failure
  is inferred. Empty successful reads must explicitly continue polling.
- Commit `a17f10a` (`test: keep polling empty supervisor pid marker`) freezes
  that test-only correction and the two failed `4fed167` gate transcripts.
  Local syntax/diff checks and the full shim race suite pass; the focused test
  capability-skips locally because abstract Unix sockets are unavailable, so
  only a zero-skip VM stress run can qualify it. Exact tracked-source archive
  `/tmp/mksrc-a17f10a.tar` is 7,096,320 bytes/SHA-256
  `6f50c87f610b9fb391b978f2d1e5edbb5f06d4c69bd55c14ff762966dfff3986`.
  Transfer `20261008-g6-cancellation-transfer-a17f10a.log` is mode 0600/353
  bytes/SHA-256
  `45bd46608336032720450fdc1d4f59dc614648dadd091cb3242a4c2d557ecc50`,
  exit 0. Guest preflight
  `20261008-g6-cancellation-source-preflight-a17f10a.log` is mode 0600/1,429
  bytes/SHA-256
  `064a88b29cfbcd28b96b7330b5279a30e4ae73d06a14f3aea0164bbfb3e7edde`,
  exit 0. Both credential scans are clean; exact archive/source/qualifier
  hashes and Bash syntax pass. Build and activation remain unclaimed.
- First `a17f10a` VM gate `20261008-g6-cancellation-build-a17f10a.log`
  is retained mode 0600/45,499 bytes, SHA-256
  `3dc84228caca4cc0f1c6ac715e371f118e2022e9f4394a1648274ffa5680cd8d`,
  exit 1, with a clean credential-value scan. The two daemon regressions pass
  200 total invocations (100 each) and the process-level shim disconnect/reap
  regression passes all 100 zero-skip repetitions. The subsequent full shim
  package alone fails because the long configured `GOTMPDIR` makes the relay
  test's Unix socket pathname exceed the kernel limit (`bind: invalid
  argument`). Vet/build/install never run. This validates the marker correction
  under the VM authority but remains a build-harness failure; rerun uses a
  short dedicated temporary directory.
- Short-path rerun `20261008-g6-cancellation-build-a17f10a-corrected.log` is
  retained mode 0600/4,778 bytes, SHA-256
  `745ed0b0fa9196782e8f9c09325781945b4a3a883985b4629f064dd46cd5da30`,
  exit 1, with a clean credential-value scan. The path-length failure is gone,
  but the newly created short GOTMPDIR has permissive mode bits and the relay
  safety test correctly rejects its group/other-writable parent. The other four
  packages pass race; shim/build/install remain failed/not run. The next rerun
  explicitly installs the short cache/temp directories at mode 0700.
- Mode-0700 rerun `20261008-g6-cancellation-build-a17f10a-final.log` is
  retained mode 0600/4,968 bytes, SHA-256
  `510539617da92c3393163af7fafd42cd9d09b5c94baf67bf0d7c79cf929ce165`,
  exit 1, with a clean credential-value scan. Although the immediate GOTMPDIR
  is 0700, the generated relay socket remains under world-writable ancestor
  `/tmp`; `stopRelay` correctly rejects that ancestry. Four other packages
  pass race, and build/install again do not run. The authoritative rerun must
  use a short caller-owned tree beneath a non-writable ancestor.
- Caller-owned runtime-tree rerun
  `20261008-g6-cancellation-build-a17f10a-pass.log` is retained despite its
  provisional name: mode 0600/5,100 bytes, SHA-256
  `57468f3689871a4fe1539dabb287b1ed917a630331397c41c2c9694ed42b815f`,
  exit 1, clean credential-value scan. It proves UID 1001 owns mode-0700
  `/run/user/1001/t`, yet the relay fixture's generated immediate socket parent
  is still rejected by `unixsocket.Capture`. Four other packages pass race;
  vet/build/install do not run. Further environment guessing is stopped; the
  exact directory identity seen inside the fixture must be diagnosed.
- Syscall transcript `20261008-g6-relay-fixture-parent-diagnostic.log` is mode
  0600/3,346 bytes, SHA-256
  `21eb09b8365f5a203c7abbc810d059573668019101f1a90e720a1e35f91f5a5d`,
  expected exit 1, with a clean credential-value scan. It proves the exact
  generated socket parent is caller-owned but mode 0775: Go's `t.TempDir()`
  inherited the guest user's 0002 umask. `openat2` binds the intended parent
  without symlink traversal and `fstat` correctly rejects its write bits. The
  fixture now chmods its private directory to 0700 before binding; production
  validation is unchanged.
- Commit `2935595` (`test: secure relay fixture socket directory`) freezes that
  test-only correction and its complete VM diagnostic trail. The full local
  shim race suite and vet pass. Exact tracked-source archive
  `/tmp/mksrc-2935595.tar` is 7,168,000 bytes/SHA-256
  `26454162c43e4eacbd8018b1846551cf6f119299db57c868ab98a943f27b4f24`;
  Transfer `20261008-g6-cancellation-transfer-2935595.log` is mode 0600/353
  bytes/SHA-256
  `3dec17f936d66c59dffff1eff158e6ffc4893f2ef4f64c44e4e55fe3033a9e15`,
  exit 0. Guest preflight
  `20261008-g6-cancellation-source-preflight-2935595.log` is mode 0600/1,429
  bytes/SHA-256
  `7afe19a6fca26248d6146740a0654c5fa0d4fe92164893157fb1cc367ddded3d`,
  exit 0. Both credential scans are clean; archive/source/qualifier hashes and
  Bash syntax pass. Build and activation remain unclaimed.
- Exact `2935595` VM gate `20261008-g6-cancellation-build-2935595.log` is
  retained mode 0600/6,193 bytes, SHA-256
  `9c42a4bba2087bd0b0762027d824a14d8d01803c5e34718f09aa2df8f6141114`,
  exit 2, with a clean credential-value scan. Both daemon tests and the shim
  disconnect test pass 100 race repetitions each; all five complete package
  race suites and vet pass. All seven binaries build, but strict manifest
  validation rejects the abbreviated revision `2935595` because it requires
  lowercase 40-hex provenance. No manifest/install/activation is claimed; the
  build stage must be rerun with full revision
  `293559519a09e574265686b1cd59598556f6eca7`.
- Corrected full-revision build
  `20261008-g6-cancellation-build-2935595-corrected.log` is mode 0600/6,072
  bytes, SHA-256
  `24037397d193293951c78c6e9fd94967d35eebfdb3ea7023f1005cac4166f768`,
  exit 0, with a clean credential-value scan and explicit pass marker. It
  starts with no manifest, rebuilds all seven static binaries stamped with
  exact full revision `293559519a09e574265686b1cd59598556f6eca7`, creates
  and validates the strict manifest, executes every binary's matching version
  output, and records all component/manifest hashes. Combined with the
  immediately preceding 100-repeat/full-race/vet transcript, the exact guest
  build gate is complete. Install/activation and live replay remain open.
- [ ] Validate containerd namespace, task ID, bundle path, rootfs mounts, OCI
  process, and runtime paths before allocation; protect against symlink/path
  races and hostile mount inputs. Service construction rejects unsafe task,
  namespace, and bundle values; the global sandbox ID now digest-binds the
  complete namespace/task tuple so different namespaces, punctuation, or
  truncated long IDs cannot alias. Exec IDs outside the guest protocol's
  bounded safe alphabet are rejected before guest contact. Rootfs and OCI
  path validation is covered. Every supported Task RPC now rejects nil input
  and a task ID different from the per-shim identity before lock acquisition,
  guest contact, events, or state mutation; a full method-table test covers
  both cases. Stdio validation and descriptor acquisition now
  apply the no-symlink, stable-identity contract described above. The broader
  hostile-input/path matrix now passes on the disposable host as recorded
  below; complete live stale-relay workload proof remains open. The
  shim now applies the rootfs service's complete mount contract before sandbox
  allocation, including bounded count/source/options, nil entries, canonical
  no-symlink source paths, supported option syntax, and duplicate rejection;
  focused adapter and service tests cover the shared boundary. The privileged
  backend now reopens the identity-bound mountpoint, every bind source, and
  every overlay lower/upper/work directory with no-symlink `openat2`,
  substitutes held `/proc/self/fd` references, and retains every descriptor
  until the mount call returns. Focused hostile replacement tests reject a
  pre-open mountpoint substitution, rename all caller-visible paths inside the
  mount boundary, prove each reference still resolves to the original distinct
  inode, and prove all descriptors close afterward. Pre-syscall rejection is
  explicitly classified so rollback removes private artifacts without
  unmounting the substituted target; uncertain syscall failure still performs
  defensive unmount. Builder-time artifact-path replacement is
  descriptor-anchored as described above, as is prepared-artifact verification.
  Removal-time public-name cleanup is now quarantined and identity-conditional
  as recorded in the 2026-09-18 checkpoint. Exact-source disposable-host
  qualification now enumerates 59 shim/rootfs/safefile/Unix-socket/storage
  path cases and passes each for 100 race repetitions with zero skips, failures,
  or race reports (`e9b9a241…`), followed by all-zero embedded and independent
  audits (`b972d661…`).
  The shim's separate network-namespace projection now reads `config.json`
  through a one-MiB, caller-owned, single-link, stable-identity `openat2`
  boundary as well; hardlink, symlinked-ancestor, and oversized-valid-prefix
  tests prevent this secondary parser from weakening the OCI input contract.
  Reconstruction and fallback Cleanup now share a bounded, no-symlink,
  caller-owned single-link recovery-file loader with stable identity and strict
  decoding. It binds sandbox/generation/task/storage/network ownership and
  validates bounded unique process state before external action. Cleanup no
  longer suppresses network, relay, sandbox, or rootfs failures; a failed stop
  prevents deletion and rootfs removal. Normal startup and reconstruction now
  share a no-symlink, caller-owned, single-link, exact-size, stable-identity
  token loader instead of reconstruction bypassing those checks with a path
  read. Initially absent tokens are created exclusively relative to the same
  validated parent descriptor, and failure cleanup unlinks only the inode that
  attempt created. Focused wrong-owner, malformed-state, symlink, hardlink,
  symlinked-ancestor, partial-network, stop-failure, and rootfs-failure tests
  pass. Fresh connection and reconstruction also refuse to unlink a stale
  relay path unless it is a caller-owned, single-link Unix socket with safe
  mode; focused regular-file, directory, symlink, and missing-path tests prove
  hostile path types remain untouched. The relay helper no longer performs
  its own unchecked pre-bind or post-accept `unlink`; pathname cleanup belongs
  solely to the validating shim. The shim must capture the published relay
  socket through a held no-symlink parent descriptor before agent dialing and
  teardown removes only that exact inode; a focused replacement test proves a
  substituted socket is preserved. Live stale-socket replacement remains open.
  Recovery and event-journal publication is descriptor-anchored beneath
  a no-symlink, caller-owned, non-world-writable parent and uses same-directory
  `openat`/`renameat`; focused normal replacement, symlinked-parent, and unsafe
  parent-mode tests prove publication cannot be redirected. The event journal
  additionally retains the exact loaded or newly published inode and refuses
  both a later atomic rewrite and final acknowledgement unlink if the name was
  replaced; the pending event remains available for at-least-once replay and a
  focused test proves the substitute bytes are untouched. An empty
  symlinked-parent test separately proves token creation cannot be redirected.
- [x] Expand OCI support required by the agreed G6 scope, or keep each omitted
  capability, namespace, mount, hook, rlimit, cgroup/resource, seccomp,
  read-only-root, hostname, and path control fail-closed with focused tests and
  truthful capability reporting. The adapter and guest now also share bounded
  process-shape semantics: argv/environment byte and count ceilings, NUL and
  duplicate-environment rejection, canonical bounded cwd, and unique bounded
  supplementary groups all fail before allocation or guest process mutation.
  Focused adapter and guest tests cover these hostile shapes.
  The supported mount subset now additionally includes at most eight
  non-overlapping materialized read-only directory or private single-link
  regular-file inputs with fixed `nodev,nosuid,noexec` policy. Writable,
  shared/slave or unsupported propagation, protected-path, noncanonical,
  conflicting, and unsanitized bind forms remain fail-closed;
  the agent advertises this additive subset as `readonly-bind-inputs-v1`.
  Initial connection and reconstruction now perform an authenticated bounded
  capability handshake and require unique complete protocol/OCI feature sets,
  including stdin/signal replay, two-phase shutdown, process controls, root
  policy, standard mounts, and read-only bind inputs. Mixed-version peers fail
  before guest network configuration or process reconstruction. Focused success,
  missing-feature, wrong-version, duplicate, and missing-identity cases plus
  reconstruction pass 100 race-detector repetitions.
  Both boundaries also open `config.json` without following symlink or magic
  link ancestors, require a private caller-owned single-link regular file,
  cap the complete input at one MiB, and reject identity changes across the
  read. Oversized-valid-prefix and hardlink tests prove the added boundary.
  Restarted-VM exact-source qualification adds 97 semantic cases across every
  named category plus namespace, file-identity, and outer-cleanup boundaries.
  Installed-service qualification rejects unsupported AppArmor explicitly at
  `validate OCI bundle before allocation`, observes no Kerf or durable/runtime/
  network allocation immediately, reaps the two transient shim wrappers,
  runs a supported positive control, and returns every one of the 19 counters
  to zero. Transcript hashes are `019608e7…` for the source matrix,
  `379a03bc…` for live behavior, and `9e600572…` for the independent final
  audit; all exit 0 and are credential-pattern clean.
- [x] Complete packaging: versioned binaries, explicit containerd and Docker
  configuration fragments, service dependencies, fresh-host installation,
  upgrade/rollback behavior, and no default-runtime mutation. Every Go
  component now has an injected common version/revision and deterministic
  release manifest; Docker configuration is conflict-detecting, validated,
  opt-in, and preserves the default. The containerd v3/v4 import-only CRI
  fragment adds a named handler without owning the main config or default and
  passes structural plus containerd config-dump tests. A verified immutable
  release layout now provides atomic activation, fresh binary installation,
  upgrades, rollback, ownership-safe uninstall, and inactive-release removal
  with end-to-end tests. Privileged live qualification now proves service
  activation, exact old-generation rollback workload, exact candidate forward
  restoration workload, per-generation identity/readiness/default assertions,
  and complete resource return. Service and configuration files now
  have a separate immutable generation manager: it validates operator runtime
  environment and host config input, hashes the fixed systemd/CNI/containerd
  assets and the complete rootfs builder/helper/guest-init support set, refuses
  unrelated paths or unsafe directory ancestry, switches all
  managed links atomically through one selector, supports exact rollback, and
  preserves generations on ownership-safe uninstall. End-to-end alternate-root
  tests cover fresh install, upgrade, rollback, dry-run/uninstall, inactive
  removal, collisions, invalid input, and symlinked installation ancestry.

### Automated tests still required

- [x] A fake-daemon Task v2 suite for every method, state transition, duplicate
  request, invalid transition, event, exit code, and cleanup path. Focused
  coverage now rejects exec start before init, exec creation after init exit,
  kill of absent or non-running processes, init deletion with retained execs,
  unsafe exec IDs, and duplicate execs before agent contact. The exact-source
  disposable-VM aggregate now runs all 87 selected tests across all 17 Task RPC
  entries for 20 race-detector repetitions. Lifecycle 20, process/I/O 33,
  control/read 14, and delete/event/recovery 20 all report zero skips and race
  reports. Transcript `20261008-g6-task-v2-exhaustive-matrix.log` is mode
  0600/810,938 bytes/SHA-256
  `d2ab3c84ca4ee92abc5faf5015dc1eb343b88893bd082ff26d7b38a8e6b59b2d`,
  exits 0, is credential-pattern clean, and contains the aggregate pass marker.
  Its strict pre/post audits show no Kerf pool or instances, all 19 resource
  counters zero, and unchanged active zero-restart services.
- [x] Event ordering and publication failure for create/start/exec/exit/delete,
  including containerd disconnect and restart. Delete persists a queued marker,
  requires the ordered journal to flush before rootfs/process record removal,
  and retains retry ownership across guest or broker failure. The complete
  six-topic test queues sequences 1..6 through six broker failures, reconstructs
  and replays exact order, and injects pre-publication persistence failure for
  each topic with zero publication/exact rollback. The updated disposable-VM
  race suite passes with 334 run entries, 141 passing groups, and zero skips in
  `g6-task-v2-vm-race-event-matrix.log` (mode 0600/55,565 bytes/SHA-256
  `e95e63d1223953ccfdc8dbbd1305db1ce5500db1d16410c6eedf9a724f0b9f8b`).
  Live containerd transcript `afa9187e…` independently retains create/start and
  post-restart exec/exit/delete events; focused retry/ack/delete repair tests
  retain the documented at-least-once boundary.
- FIFO aggregate audit found two literal missing cases and added terminal versus
  non-terminal `CloseIO` plus teardown with attached stdin/stdout peers. The
  first 20x race run failed because the test could teardown before its newly
  launched pump captured `p.stdinReader`, racing at the pump's initial read.
  This is not product evidence: the corrected test must first send and observe
  a byte to prove the pump is active before exercising attached-peer teardown.
- Corrected FIFO additions pass 20 race-detector repetitions in 21.541 s.
  Terminal and non-terminal running execs both persist/acknowledge `CloseIO`,
  contact the guest exactly once, and make repeat close idempotent. Attached-
  peer teardown first proves stdin delivery, then requires the pump to exit,
  all process I/O handles to clear, the still-open stdin writer to lose its
  reader, and the still-open stdout reader to observe EOF. Full local/VM suite
  reruns remain pending.
- Full local FIFO-matrix suite transcript
  `g6-task-v2-unit-race-fifo-matrix.log` is mode 0600/56,274 bytes/SHA-256
  `ea8e826e6fd054bd9f5f6c65aa713e319e2b0a04f6728cbc6ed6f3e47f2f8b77`.
  It exits 0 under the race detector in 9.567 s with 338 run entries, 141
  passing groups, and only the two known local pathname-socket skips. The VM
  zero-skip rerun remains pending.
- [x] Context cancellation and deadline expiry at every blocking boundary.
  Direct tests map rootfs/storage mutation and hash loops, bounded initramfs
  builder descendants, daemon dial/write/read/default timeout, Kerf child boot
  commands, agent connect/call, stdio open/pumps, Task locks/wait, and teardown
  and network cleanup. The exact source runs repository-wide under `-race` on
  ext4 with a short socket-safe `TMPDIR`: 968 run entries, 477 passing groups,
  all 22 tested packages, zero skips/failures. Transcript
  `runtime-all-packages-vm-race-cancellation-pass.log` is mode 0600/144,078
  bytes/SHA-256
  `94f077c3b6837ef90f8416202f0779c4bd3bba0957c3b9b5b1368f737fc4edb0`.
  This closes the automated row; the separately stated live cross-service
  cancellation/leak matrix in the implementation row remains open.
- [x] FIFO writer/reader disappearance, no initial peer, late attach, repeated
  attach, output backpressure, terminal and non-terminal `CloseIO`, resize
  before start and during exec, invalid resize, and teardown while attached.
  Focused tests cover retained pre-start size, successful running resize,
  guest-rejected resize rollback, stopped/invalid requests without mutation,
  terminal/non-terminal idempotent CloseIO, and teardown with attached peers.
  Reconstruction reapplies durable size before restarting I/O. Exact updated
  source SHA-256 `10101b2d…` passes the disposable-VM race suite with 338 run
  entries, 143 passing groups, zero skips, and zero failures in 9.335 s;
  `g6-task-v2-vm-race-fifo-matrix.log` is mode 0600/56,087 bytes/SHA-256
  `6fb8ffff1e22a32ed288fc6c42922bebe4e75f8d24f1f5812074a3cb26b00a8f`.
  Live matrix `8ee2f800…` proves ctr/Docker attach and exact 37x91 post-start
  resize, while forced-shim transcript `fe6eee0d…` proves FIFO continuity across
  worker death/reconstruction.
- [x] Unsupported OCI configuration before allocation and after each possible
  partial allocation, proving fail-closed cleanup.
  Current audit confirms this remains a real implementation gap: Exec process
  shape is validated before mutation, and rootfs/network paths are validated
  before allocation, but the complete init OCI contract is loaded and rejected
  by the guest during Start, after Create may have allocated rootfs, storage,
  sandbox and network ownership. Existing lifecycle crash/cancel tests prove
  generic rollback, not unsupported-OCI rejection at each partial boundary.
  Do not close this row until a shared host/guest OCI validator rejects the
  complete supported subset before allocation and deterministic injected tests
  prove cleanup for every later validation/application boundary.
  Pre-allocation implementation checkpoint: `Create` now calls a new
  identity-bound `ValidateRootfs` RPC before `allocate`; the Linux backend runs
  the canonical descriptor-bound builder with `MK_VALIDATE_ONLY=1`, and the
  builder exits immediately after `validate-runtime-oci.py`, before requiring
  task/storage identity or creating artifacts. Bash syntax passes. The first
  compile run is intentionally not evidence: rootfs fake backends lack the new
  method and one shim daemon fixture rejects the new call. These fixtures and
  focused no-mutation/order tests remain required before qualification.
  The shim pre-allocation rejection test already passes 20 race repetitions.
  The first rootfs focused invocation stopped at a test-only type mismatch
  (`*os.Root` versus the backend's held `*os.File`), before executing behavior;
  it is not evidence. The fixture now reopens `.` descriptor-relative through
  the verified root, matching production handoff semantics.
  After that correction, the rootfs identity/no-artifact and backend held-
  descriptor validation tests plus the shim reject-before-allocation test each
  pass 20 race-detector repetitions. The focused runs establish call ordering
  and local non-mutation; full suites, deployment of the changed daemon/shim/
  builder, and disposable-VM behavioral qualification remain unclaimed.
  Canonical Python validation passes all 95 semantic OCI cases plus namespace,
  file-identity, and outer-cleanup boundaries; builder Bash syntax passes. A
  first full-Go command was issued from the repository root and stopped at Go
  module discovery, so it ran no tests and is not evidence; the identical race
  suite must run from `runtime/`.
  Corrected module-root `go test -race -count=1 ./...` passes every tested
  runtime package, including changed shim (9.537 s), daemon (1.191 s), and
  rootfs (1.717 s); storage passes in 3.777 s. This is the local integration
  baseline only; exact source hashing and VM execution remain pending.
  After the operator restart, the disposable VM has new boot
  `f1d650a2-373b-4fe7-9a96-382e51332172`, kernel `7.0.0-mk2-gce-lab`, and
  active/zero-restart mkruntimed/containerd/Docker/mknetd PIDs
  1467/1485/1528/1236. ctr and Docker running inventories are empty. Selector
  remains prior control `1edd368f…`; the changed candidate is not yet deployed.
  Commit `11a65f0` freezes the pre-allocation validation implementation and
  focused tests. Key SHA-256 values are builder `aae6496c…`, shim source
  `0107ddd4…`, rootfs backend `0bb75277…`, and daemon server `5c4ca54b…`.
  Documentation/evidence remain deliberately separate from this code commit.
  Exact `git archive` transfer is SHA-256 `bf8a16d3…` (1,043,133 bytes); VM
  extraction at `/var/tmp/mksrc-11a65f0` contains 637 files/4,163,480 bytes.
  Remote builder/shim/rootfs/daemon hashes match the four local values exactly.
  This binds subsequent VM tests to commit `11a65f0`.
  Exact-commit VM transcript `g6-oci-preallocation-vm-race-pass.log` is mode
  0600/146,655 bytes/SHA-256
  `f751bdea06bc530852016e748119b0872b67544a7bcb83d725a8519b4c0723fa`.
  It exits 0 with canonical 95-case OCI validation, 971 Go run entries, 480
  passing groups, all 22 tested packages, zero skips/failures. The three new
  ordering/identity/no-artifact tests are present and pass. This proves the
  exact source in isolation; installed-service behavior remains pending.
  The first root-owned build command changed only the verified temporary tree's
  ownership, then stopped before compilation because its ordinary outer shell
  could no longer enter the mode-0700 directory. No manifest, installation, or
  service state is claimed. The corrected command must perform `cd` and build
  within one privileged shell.
  Corrected privileged build succeeds for full revision
  `11a65f08f07b6bb88a6eb3088301582a37f55005`. Release manifest SHA-256 is
  `e22e76a6…`; mkruntimed/shim/mknetd/agent are `0e1c87c3…`, `326a8274…`,
  `2dbe69d2…`, and `c7e61607…`. The built daemon prints the exact version and
  revision. Installation, support activation, and execution remain unclaimed.
  Immutable managers install/select binary release
  `0.1.0-dev-11a65f08f07b6bb88a6eb3088301582a37f55005` and matching support
  generation `b4d185c68b567c8d44882b34978cd18ac29a67d22f264cc9b4d719971a84371f`.
  All managed binary/config/service/support links inspect valid. This proves
  atomic selection only; service reload/restart and execution remain pending.
  Empty-host reload/restart activates exact candidate: mkruntimed PID 15863
  resolves into release `11a65f08…` and hashes `0e1c87c3…`; public version
  reports the full revision. Shim hash is `326a8274…`; active builder hash is
  `aae6496c…` beneath support generation `b4d185c6…`. mkruntimed, mknetd,
  containerd and Docker are active/running with `NRestarts=0`. Workload and
  unsupported-OCI behavior remain unclaimed.
  Post-qualification VM checkpoint retains boot `d9cdfa98…` and the same four
  active/running, zero-restart services at PIDs mkruntimed 9410, containerd
  1480, Docker 1519, and mknetd 1227. ctr and Docker running-task counts are
  zero. Exact `/proc/*/comm` enumeration reports zero multikernel shims,
  `nbdkit`, and `mk-agent-relay` helpers. An initial `pgrep -f` helper count was
  discarded because it matched the audit command itself; it is not evidence.
  The pre-allocation half is now live-qualified on exact `11a65f08…`.
  `g6-oci-preallocation-live-pass.log` is mode 0600/129,202 bytes/SHA-256
  `c409237a4041960a6cd0acfb506ba89387a450fe0ba0f6ff8672a4b2da7c7cd3`.
  It records seven observation blocks and exit 0: exact daemon/shim/builder
  identities; initial zero state; canonical AppArmor rejection explicitly
  labeled `validate OCI bundle before allocation`; immediate absence of pool,
  children, mounts, artifacts, network, records, and helpers; bounded shim
  reaping to the full 19-category zero inventory; positive-control output
  `MK_OCI_SUPPORTED_PASS`; and the same final zero inventory after reusable-
  pool release. All four services remain active/running with `NRestarts=0`.
  A credential-pattern scan finds zero matches. This substantiates rejection
  before allocation. Exact-commit VM matrix evidence below supplies the
  remaining deterministic partial-ownership cleanup proof.
- [x] Two or more concurrent sandboxes under churn with disjoint CPUs, memory,
  generations, roots, agent endpoints, networks, and recovery records.
  `g6-concurrent-churn-pass.log` is mode 0600/96,522 bytes/SHA-256
  `09a0c135eeb56c2ac9b1bec45038754e615c444faf66862f689fc612124ec543`,
  exit 0. Its safe durable projection proves every disjoint identity, 12+12
  parallel execs, simultaneous pause/resume, distinct child boots, and audited
  retained/released cleanup on the exact final candidate.
- [x] Containerd restart, Docker daemon restart, `mkruntimed` restart, clean shim
  restart, forced shim death with reconnect, and forced shim death with bounded
  fallback reclaim.
  Exact exit-0 transcripts hash respectively to `afa9187e…`, `fa381592…`,
  `e99f610e…`, `754a05f6…`, `fe6eee0d…`, and `e833dcd1…`; each includes its
  scoped behavior plus cleanup rather than relying only on a summary marker.
- [x] Init and exec signal delivery, ignored `SIGTERM`, `SIGKILL`, nonzero exit,
  descendant cleanup, wait/delete races, and same-name reuse after every
  failure mode. Pause/resume now signals all applicable init and exec process
  groups transactionally, with deterministic order and bounded reverse-order
  rollback after partial signal failure. Event-failure rollback also returns a
  labeled recovery rewrite failure rather than hiding a disk/memory mismatch;
  injected directory replacement covers Pause and Resume for 100 race-detector
  repetitions. A failed initial transition write also reverse-signals and
  republishes the prior recovery state as a new inode; both directions pass
  100 race-detector repetitions. Focused tests cover success and these partial
  boundaries. After guest Start succeeds, invalid PID, recovery, or start-event
  failure retains RUNNING ownership and monitors while a cancellation-independent
  five-second `SIGKILL` is attempted. Signal errors are returned and
  authenticated `NOT_FOUND` confirms cleanup. Injected recovery/signal failure
  retains PID 41 and later records exact exit 9 in 100 race-detector
  repetitions. Task Kill now persists a generation-scoped signal operation ID before guest
  contact. The advertised `signal-operation-id-v1` guest ledger returns exact
  replay without signaling twice, including after process exit, and rejects
  cross-target or changed-signal reuse. It refuses before mutation rather than
  evicting an unresolved result when its 4,096-entry bound is full, while
  idempotent `AcknowledgeSignal` retires durably observed results. Lost-reply
  Kill performs two calls, one reconnect, and one applied signal through its
  durable intent/result/retirement phases; reconstruction resumes the recorded
  phase, and cleanup/pause/resume use the same primitive. The focused matrices
  pass 100 race-detector repetitions.
  Start and reconstruction also reject positive agent PIDs above
  Task v2's `uint32` range instead of truncating them. An injected oversized
  PID retains durable unverified ownership without publishing a start event,
  stays monitored through cleanup, and passes 100 race-detector repetitions.
  Init Start reconciles its durable CREATED owner before mutation: only
  authenticated `NOT_FOUND` creates, while exact CREATED state is reused after
  partial failure/recovery. Wrong identity, live state, fabricated PID, and
  transport failure cause no duplicate create across 100 race repetitions.
  A lost successful `CreateProcess` reply is also reconciled without replaying
  the non-idempotent mutation: an independent state observation reconnects and
  accepts only exact CREATED identity. The injected broken-transport case makes
  one create and one reconnect in 100 race-detector repetitions; authenticated
  create rejection remains terminal.
  Ambiguous `StartProcess` failure is reconciled under an independent
  five-second bound with relay reconnect: exact CREATED remains retryable,
  exact RUNNING or rapid STOPPED succeeds, and unavailable state retains
  monitored, durable ownership plus bounded cleanup. The five-boundary matrix,
  including one lost state reply and one reconnect, passes 100 race repetitions.
  Start/reconstruction require the exact agent process identity and `RUNNING`
  or validated completion state rather than trusting PID alone. Wrong identity
  and `CREATED` observations after a successful Start retain unverified
  ownership and monitoring with no start event across 100 race-detector
  repetitions; an exact rapid `STOPPED` observation is accepted.
  Wait/reconstruction completion now requires the requested agent ID,
  `STOPPED`, the established PID, and exit status `0..255`. Wrong ID/state/PID,
  negative exit, and exit 256 remain RUNNING with no exit event until an exact
  reply arrives; the five-boundary matrix passes 100 race-detector repetitions.
  Normal Task Delete treats authenticated guest `NOT_FOUND` as confirmed
  idempotent absence, returns the exact PID/exit status, publishes the delete
  event, and removes local ownership. Arbitrary guest failure still retains
  retry ownership. A lost successful deletion reply reconnects and requires
  authenticated `NOT_FOUND` before continuing; the three-boundary matrix passes
  100 race-detector repetitions.
  Live signal transcript `g6-signal-lifecycle-pass.log` (SHA-256 `6647cc0e…`)
  proves ignored TERM, exec/init KILL 137, descendant cleanup, attached wait/
  delete completion, and same-name reuse after signal. The final-candidate
  shared matrix (`8ee2f800…`) proves repeated nonzero 17 plus same-name reuse.
  The descendant process-group signal test now waits for the terminal marker
  value instead of treating its earlier ready value as a terminal failure,
  eliminating a false negative while preserving the two-second bound.

### Replacement instance evidence required

- [x] Re-run the complete shared `ctr`/Docker matrix while retaining expanded
  commands and observed boot IDs, kernel release, image/binary provenance,
  stdout/stderr, stdin/attach output, terminal size, signals, exit statuses,
  names/generations, and cleanup inventories.
  Final-candidate transcript `g6-shared-matrix-final-candidate-pass.log` is
  mode 0600/315,263 bytes/SHA-256
  `8ee2f800c96a7b49f62875f73b1b26f04d99c828570eeb4b40ed956471a10746`,
  exit 0. It retains 19 feature passes, every expanded observation, both exact
  live resize values, five true scoped assertions, and released all-zero final
  inventory under exact candidate `1edd368f…`.
- [x] Change each terminal size after the guest process is confirmed running
  and retain the before/after values. `g6-shared-matrix-pool-retained-pass.log`
  records exact `ready:24 80` then `resized:37 91` for ctr and Docker under the
  frozen `3015518c…` matrix/`fe7059cf…` helper, and closes exit 0.
- [x] Retain a containerd-restart transcript with service PID/boot identity,
  task state, child boot ID, exec before and after, stdio continuity, events,
  and final deletion. Repeat separately for Docker daemon restart. Exact
  `dbc4630c…`/`ab0cea4e…` focused harnesses produce mode-0600 exit-0
  transcripts `g6-containerd-restart-continuity-pass.log` and
  `g6-docker-restart-continuity.log`, hashing to `afa9187e…`/`fa381592…`.
  Both retain changed service PID, unchanged host and child boot identities,
  pre/post exec and init stdout/stderr, Task events, normal deletion,
  retained-pool cleanup, and final released all-zero inventory.
- [x] Retain an `mkruntimed`-restart transcript with journal/snapshot/recovery
  state and child identity before and after; do not rely only on a pass marker.
  Corrected capture `g6-mkruntimed-restart-continuity-v2-pass.log` is mode
  0600/136,321 bytes/SHA-256 `e99f610e008679cda2dbabf1f615d8fcc09d0ce9d07654d19d79871005bfd6f7`,
  exit 0 at `2026-10-01T12:10:42.454130Z`. It records daemon `65694→66493`,
  unchanged host boot/containerd PID, unchanged child/task/recovery/network
  identities, byte-identical durable journal/snapshot/rootfs/storage state,
  post-restart exec, continuous stdout/stderr, timestamped Task events, and
  retained then released clean inventories. An independent audit binds final
  PID 67287 to the exact candidate binary and confirms no pool/instances or
  shim/keeper processes plus four healthy zero-restart services.
- [x] Retain forced-shim-death transcripts for both required outcomes: task
  reconstruction/reconnect and bounded safe reclaim when reconnect is
  deliberately made impossible. Include killed PID, service journals, recovery
  record, task/client behavior, child state, and final resources. Reconnect
  transcript `g6-forced-shim-reconnect-fifo-pass.log` is mode 0600/179,780
  bytes/SHA-256 `fe6eee0d3711afc79c7153afbaba5f91efb478db139d812537018a9d383d6a18`,
  exit 0 at `2026-10-01T11:57:02.480703Z`; bounded-reclaim transcript
  `g6-forced-shim-reclaim.log` is mode 0600/47,705 bytes/SHA-256
  `e833dcd1baa2127f66f73a1a01ddbe2c9eed8bc8ec846fe3e5d887bbcff28f5a`,
  exit 0. Both contain scoped pass markers and audited cleanup.
- [x] Retain exact Task v2 event order and timestamps for init and exec
  processes, including nonzero and signaled exits. Focused transcript
  `g6-task-events-live-pass.log` is mode 0600/68,210 bytes/SHA-256
  `43cf0ba4422247b04c282be475e9fc2fcd3b0c42107e642a85a248161d1f3506`,
  exit 0. It retains all 12 nanosecond timestamps in exact order, exec exit 17,
  exec/init SIGKILL exits 137 in both client results and events, and final
  released all-zero cleanup under the exact candidate.
- [x] Prove final return of CPUs, memory, Kerf instances/pool, rootfs mounts,
  initramfs/runtime artifacts, agent/relay/shim processes, FIFOs, TUN links,
  routes/firewall rules, containerd tasks/containers, and Docker containers.
  `g6-final-resource-return-pass.log` is mode 0600/12,896 bytes/SHA-256
  `3bc958626d131aa44389eaa9f6fdc3fb694200487890a825aa01fc9e8b9d7ff5`,
  exit 0. Kerf reports no pool/instances, all 19 explicit residual counters are
  zero, exact candidate identity is retained, and all services are healthy.

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
- [x] Capture structured `resources-before.json` and `resources-after.json`
  covering instances, disks, snapshots, addresses, firewall rules, and every
  billable or retained resource. Narrative cloud-cleanup text is supplementary,
  not a substitute for the contract ledger. A strict project-wide GCE ledger
  schema and exclusive-create collector now cover these resource classes,
  attachment users, and boot/data-disk auto-delete policy. The collector passed
  against the replacement project. The retained mode-0600 before/after ledgers
  hash to `fc6eb570…`/`0f5f3508…`; after deleting only `captured_at`, their
  normalized contents are byte-identical.
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

## Live continuation checkpoint: 2026-09-20

The operator explicitly authorized transferring the current source to the
disposable VM and running the live qualification suite. This authorizes source
transfer and execution on the disposable instance; it does not reproduce the
separately documented exact sentence authorizing retention of the described
infrastructure evidence. The run will therefore record its findings in these
working learnings as they occur, while permanent raw-evidence publication and
any resulting live-closure claims remain conditional on that distinct
retention approval.

GCE control-plane inspection then found
`mklinux-g4-g6-final-20260905` already `RUNNING`; restart or recreation was not
required. It reported the expected `n2-standard-16` machine type, labels
`disposable=true` and `purpose=multikernel-g4-g6-final`, an auto-delete 100-GB
boot disk, internal address `10.148.0.56`, ephemeral external address
`136.85.103.235`, and last start timestamp
`2026-09-19T20:36:29.072-07:00`. This is a control-plane observation, not yet
guest-execution evidence.

The first guest readiness probe succeeded over GCE SSH. The instance reported
hostname `mklinux-g4-g6-final-20260905`, kernel
`7.0.0-mk2-gce-lab`, boot ID
`3b4d5c5d-9436-4bab-9c17-609a4dcd181d`, an active
`google-guest-agent`, and a present `/sys/fs/multikernel`. The source revision
selected for transfer is committed revision
`2051d0131428b3e75ab177b3f3e63bf4b41ad6ea`; the archive is to be produced by
`git archive`, so `.git`, the pre-existing untracked
`evidence/runtime-20260907/`, and these uncommitted running notes cannot enter
the guest payload.

The first transfer orchestration attempt uploaded the archive but stopped
before extraction: its local SHA-256 was
`6cae2c16495742dd9f7d9f8b8f579e5a83e199bd0f2431de2bf05d89b1b9972a`,
whereas the remote command embedded a stale expected value and an incorrectly
escaped `awk` field. The remote equality test consequently failed with an
empty computed value. No qualification result is inferred from this operator
error, and no source directory was created by that command.

The corrected remote check printed and matched the same SHA-256, unpacked the
archive once into
`/home/hairizuan-tw/mklinux-src-2051d013-20260920`, confirmed that `.git` and
`evidence/runtime-20260907/` were absent, and found the expected
`runtime/go.mod`. The current committed source is therefore present on the
guest with a locally and remotely matched archive digest.

Running the transferred revision's `scripts/verify-host.sh` passed: the custom
release was exact, CPUs `0-15` were online, Multikernel sysfs was mounted, and
`ens4` retained the primary address. The subsequent deployment inspection
found a fresh runtime state: no active or installed managed binary release, no
active or installed managed deployment generation, and none of their stable
command/config links. `mkruntimed`, `mknetd`, `containerd`, and Docker all
reported `inactive`. Because `systemctl is-active` returned nonzero under
`set -e`, that command ended before its later idle-inventory and binary-hash
steps. The host is qualified, but it must be provisioned before a runtime suite
can execute.

The first prerequisite-inventory command produced no inventory: its
`dpkg-query` format contained the shell-like token `${binary:Package}`, which
the remote shell evaluated under `set -u` and rejected as an unbound variable.
This is another operator-command quoting defect, not a missing-package or
runtime result; the read-only command made no guest change.

The corrected prerequisite inventory established that this boot image is bare
beyond the qualified kernel: `docker.io`, `containerd`, `golang-go`, and
`socat` are not installed; `/opt/mkruntime`, `/etc/mkruntime`,
`/etc/multikernel`, `/etc/containerd`, and `/etc/docker` are absent. Required
core tools including BusyBox, cpio, e2fsprogs, GCC, iproute2, iptables, jq,
make, Python, rsync, and util-linux are installed. The retained 20-GB disk is
attached at `/dev/sdb` through
`/dev/disk/by-id/google-mk-mediated-storage-20260830`, has ext4 label
`mk-mediated-host`, and is not mounted. Provisioning therefore must install
the missing packages and runtime/Kerf artifacts, mount the mediated disk, and
install configuration/services before live qualification.

Kernel/artifact inspection found the matching host kernel image and config in
`/boot`, plus
`/lib/modules/7.0.0-mk2-gce-lab/kernel/drivers/block/nbd.ko`; neither NBD nor
`mk_transport` was loaded. The home directory retains pinned Kerf and Linux
source trees and `~/multikernel-artifacts`, so those inputs can be inspected
rather than rebuilt blindly. The attached disk reports the exact serial
`mk-mediated-storage-20260830`, size `21474836480`, ext4 label
`mk-mediated-host`, UUID `507c0523-8e58-4ae3-9524-3b7513aad344`, and clean
filesystem state. This supports an identity-checked mount of the existing
filesystem; the first-format helper must not be used because a valid filesystem
already exists.

Artifact provenance inspection matched both required upstream pins: Kerf is
commit `8b72b3e9b266f8d32e707e2c1743ad7afc50b1ec` and reports version `0.2.0`;
the Linux tree is commit
`3bdd35b64413da0b4e089ce931bfc2e8b031cbf7`. The retained artifact directory
contains only the earlier child initramfs, SHA-256
`487127ea26e4ce4a23cb91bdaaf8bb3229c2aa3146e8d0235785252baa3261ef`.
It does not contain a ready transport module, relay, mediated-NBD helper, or
current agent. Built `vmlinux` candidates exist in the pinned Linux tree, but
the missing runtime-specific artifacts must be rebuilt and validated before
deployment.

Package provisioning completed successfully from Ubuntu Resolute repositories.
Observed versions are containerd `2.2.2-0ubuntu1.1`, Docker
`29.1.3-0ubuntu4.1`, Go `1.26.0`, socat `1.8.1.1-1ubuntu0.1`, and musl-tools
`1.2.5-3build1`. Package installation enabled containerd and Docker units, but
the Multikernel runtime has not yet been activated.

The exact-source artifact build completed. `make runtime-manifest` built all
seven static x86-64 Go components with version `0.1.0-dev` and revision
`2051d0131428b3e75ab177b3f3e63bf4b41ad6ea`; the generated manifest SHA-256
is `579ad1e9b69a12aa1cd0415f7ed40f4869410553f0b4c4284e221f6316374825`.
The current C sources compiled warning-clean and static: `mkvsock-nbd` SHA-256
`a0259098bba0a4319737f2ca4fca5c8ea39e42c8261c6dd0edadcacaa10f0ca3`
and `mkvsock-relay` SHA-256
`293ff1eaa209d16103c7caaa8e8f9a24702e58d979453fad5495c503f76aaf98`.
The pinned Linux tree built module `mk_transport`, SHA-256
`bef1b888e7c66705f0d652b1437a239835584f2fb376ad6bfea2a77ecaa1c3d2`,
with module name `mk_transport` and exact vermagic
`7.0.0-mk2-gce-lab SMP preempt mod_unload modversions`. The component
manifest additionally recorded hashes
`43ee9154…` (shim), `ecc85bd6…` (agent), `9198e5fa…` (agentctl),
`1a76a8f4…` (CNI), `320074fc…` (host check), `cb32e8b9…` (mknetd), and
`02af8cff…` (mkruntimed). No privileged runtime installation had occurred at
this checkpoint.

Source remediation now makes the deployment and loader contracts compatible
without weakening the ownership boundary. The host-config loader accepts a
caller-owned regular file or generation symlink, resolves one canonical
snapshot, validates both the public and resolved parent chains, then reads a
bounded exact regular target with owner/mode and pre-open/post-read identity
checks. A focused generation-link test passes, and a link into a writable
target parent is rejected. The hostconfig package passed once and 100
race-detector repetitions (`1.772s` for the repeated run), and package vet
passed. Full-repository validation and disposable-host redeployment remain
pending.

Commit `f5d8da448f23710e3777ed011afaad003881877a` freezes the explicit
device-policy opt-out checkpoint. Its tracked-source-only archive
`/tmp/mklinux-f5d8da4.tar.gz` hashes to
`164a1b9d6895a34253fd1b5a6b7107ae748ce62dd9eff523b7dd0e19114586eb`;
the untracked historical evidence tree remains excluded and untouched. Guest
transfer and exact live qualification remain pending.

Guest-side hashing reproduces archive `164a1b9d…`, but the VM has rebooted
since the preceding activation: current boot ID is
`1151712d-c18e-4e15-9c6d-2d945dcd5bb0`, replacing `e76edca2…`. The uploaded
archive is present, while `/tmp/audit-runtime-clean.sh` was lost across reboot,
so the combined command stops there and establishes no clean-runtime claim.
The syntax-checked audit script must be re-uploaded and current service/
resource state re-established before build or activation.

The restored audit proves the new boot is clean across both containerd
namespaces, Docker, children, runtime storage/state, host network, and helper
processes. All five services are active and exact `298d3de…` selectors/hashes
survived reboot. mknetd PID 1231 has zero restarts; mkruntimed PID 1505 has one
restart. Current-boot journal evidence attributes it to the first 12:29:11 UTC
qualification seeing the Google guest agent as `unknown`; systemd retried at
12:29:26 and startup passed. This service interval cannot inherit pre-reboot
PID continuity, but the host is a valid empty build precondition.

The exact root-owned `f5d8da4…` build completes: release manifest
`d56a0d15…`, shim `1e25d1e2…`, mkruntimed `6d3bc10e…`, mknetd `4daf0150…`,
agent `9534c5b3…`, and initramfs `ac25694d…`. Privileged/pipefail listing
confirms agent, transport module, and relay membership. Nothing is selected or
activated yet.

Managed selection installs binary release `0.1.0-dev-f5d8da4…` and support
deployment `b8176d6c…`; deployed/source OCI validator hashes match at
`31bd16b0…`. Current-boot daemon PIDs/restart counts remain 1231/0 and 1505/1,
so running processes still belong to the previous revision. Manifest staging
and coordinated restart remain pending.

Root-only revision artifacts `9534c5b3…`/`ac25694d…` are staged, and strict
bootstrap validation passes for candidate manifest `bfb05789…` with all pinned
kernel/module/relay/config/feature identities intact. The candidate remains
inactive pending an immediate clean-host audit.

The immediate audit is fully empty across workloads, children, storage,
durable active state, network resources, and runtime helper processes. Current
boot daemon identities remain 1231/0 and 1505/1. Coordinated activation can
proceed without displacing work.

Coordinated f5 activation succeeds on current boot `1151712d…`, preserving the
298 manifest for rollback. Active kernel/release/shim hashes are
`bfb05789…`/`d56a0d15…`/`1e25d1e2…`; all version reports and strict bootstrap
validation agree on f5. Mount, mknetd, mkruntimed, containerd, and Docker are
active; PIDs 9314/9333/9343/9378 have zero restarts/status 0 in the new
interval. Exact basic workload proof is next.

The exact f5 runner hashes to `7fb6cc6e…`. ctr reaches `RUNNING`; Docker clears
the resource contract and then fails closed on
`linux.cgroupsPath must be absolute, canonical, and bounded`, exits 125, and
runs the EXIT trap. The basic suite remains incomplete. Cleanup and Docker's
exact cgroupsPath form must be observed before deciding whether it is inert
host placement metadata or requires child enforcement.

Independent cleanup is again complete across every workload/resource/process
inventory. f5 daemon PIDs 9314/9333 remain active and unrestarted. The only
new boundary is the requested cgroupsPath representation.

The runc-backed request uses systemd cgroup syntax
`system.slice:docker:<64-lowercase-hex-container-id>`. This field selects a
host cgroup location; the runtime already validates then omits absolute
cgroupsPath from the child projection, and the accepted Docker resource
contract is explicitly unrestricted. A bounded exact Docker-systemd grammar
is therefore equivalent host placement metadata. Validation may accept only
that grammar in addition to the existing canonical absolute path; malformed,
wrong-slice/prefix, uppercase, short, or extra-component forms remain rejected.

The narrow Docker systemd cgroupsPath grammar is implemented. Focused OCI
validation passes 78 semantic cases, including wrong slice, short ID, and
uppercase ID rejection; Python compilation, both shell syntax checks, and
`git diff --check` pass. Full gates and exact live revision remain pending.

The complete local gate passes: full Go race suite, vet, all documentation/
schema/evidence/deployment checks with 78 OCI cases, and diff check. Only the
classified local socket `EPERM` subcase is skipped. The cgroupsPath checkpoint
is ready to freeze for live proof.

Commit `92531eb6453f72783c63ba3047b8b5c2666fde9b` freezes the bounded
Docker-systemd cgroupsPath checkpoint. Its source-only archive
`/tmp/mklinux-92531eb.tar.gz` hashes to
`c6d542b9021dcbef97e0bf6b434bb0763ab6a429a9e5447c5ae46253fc2104f7`;
the untracked historical evidence tree remains excluded. Guest verification
is pending.

Guest SHA-256 matches `c6d542b9…` on unchanged boot `1151712d…`. Every
workload, child, storage/state, network, and helper-process inventory is zero;
f5 daemon PIDs 9314/9333 remain active with zero restarts. Build preconditions
pass.

Exact root-owned build hashes are `95382a6f…` (release), `c5c3bb28…` (shim),
`2ccb1418…` (mkruntimed), `902b4a23…` (mknetd), `8c89b5a3…` (agent), and
`3ed53121…` (initramfs). Privileged/pipefail inspection confirms agent,
transport module, and relay membership. Selection/activation remain pending.

Managed selection installs release `0.1.0-dev-92531eb…` and deployment
`844df6d2…`; deployed/source validator SHA-256 agrees at `a6f2b969…`. f5
daemon PIDs remain 9314/9333 with zero restarts, proving no service process has
changed yet. Revision manifest staging remains pending.

Root-owned revision artifacts are now staged at the immutable `92531eb…`
artifact path. Strict bootstrap validation passes for candidate manifest
`807d177001c5fb1c7925644b214353e5c7e0dbfd1b75d8f5e69d5090f3b8d24f`;
its pinned agent and initramfs reproduce build hashes `8c89b5a3…` and
`3ed53121…`. The candidate is not active and the f5 processes have not been
restarted. An immediate clean-state audit remains required before coordinated
activation.

The immediate pre-activation audit is empty: default/moby tasks and
containers, Docker objects, children, storage files, active lifecycle
sandboxes, mknetd endpoints, rootfs records, `mkv*` links, NAT/filter rules,
and exact runtime helper processes all report zero. Existing f5 mknetd and
mkruntimed remain active at PIDs 9314/9333 with zero restarts and successful
status. The staged revision may therefore be activated without displacing a
live workload.

The first coordinated activation attempt stopped before starting the new
revision because the orchestration script assumed an `opt-mkruntime.mount`
unit that this host does not define. Its error trap restored the f5 manifest;
active, rollback, and preserved manifests all hash to `bfb05789…` on unchanged
boot `1151712d…`. The restart side effect produced healthy mknetd,
mkruntimed, containerd, and Docker PIDs 15366/15385/15392/15427, each with
zero restarts/status 0. A post-rollback audit is fully empty across every
workload, child, storage/state, network, firewall, and helper-process
inventory. The candidate pathname was consumed by the attempted move, so it
must be regenerated; no `92531eb…` runtime behavior is claimed from this
orchestration failure.

The first candidate-regeneration retry also stops safely before writing a
candidate: its idempotency check verifies the already staged agent hash, then
cannot read the root-only mode-0600 build initramfs without `sudo`. This is a
staging-script privilege error only; the source-side hash comparison must be
privileged before retry.

With both root-only hashes read under `sudo`, idempotent restaging verifies the
existing agent and initramfs against their exact build outputs, recreates the
candidate, and again passes strict validation at manifest hash `807d1770…`.
No artifact was overwritten. A corrected activation must omit the nonexistent
mount unit and retain the same rollback guarantees.

The second activation reached a healthy exact `92531eb…` service set, but its
final diagnostic invoked nonexistent `/usr/local/bin/mkruntimed` rather than
the service's `/usr/local/sbin/mkruntimed`. That false post-check restored the
f5 manifest while already-active `92531eb…` processes remained running,
temporarily breaking manifest/process coherence. Inspection identified the
actual service paths and exact `92531eb…` shim identity. Rollback handling is
corrected to stop and restart all services after restoring a manifest, so a
future diagnostic failure cannot repeat that mismatch.

After regenerating the same strictly validated candidate, coordinated
activation succeeds on unchanged boot `1151712d…`. Active manifest hash is
`807d1770…`; f5 rollback/preserved copies remain `bfb05789…`. mknetd,
mkruntimed, containerd, and Docker are active at PIDs
18265/18283/18294/18328 with zero restarts/status 0. The shim, mkruntimed, and
mknetd each report exact revision
`92531eb6453f72783c63ba3047b8b5c2666fde9b`. This proves coherent activation,
not workload behavior.

The post-activation pre-workload audit is again fully empty across both
containerd namespaces, Docker, children, storage/state, host networking,
firewall rules, and helper processes. Exact-revision daemons remain PIDs
18265/18283 with zero restarts. The basic live suite now has a clean starting
boundary.

The exact basic runner hashes to `7fb6cc6e…`. On boot `1151712d…`, its ctr
workload reaches `RUNNING`, and Docker passes the newly accepted bounded
systemd cgroupsPath. Docker's next fail-closed rejection is
`linux.maskedPaths contains a duplicate or unsupported path`; it exits before
workload start and the runner invokes cleanup. The suite is not a pass. An
independent cleanup audit and exact observation of Docker's masked-path list
must precede any policy change.

The independent post-failure audit is completely empty across containerd,
Docker, child, storage/state, host network, firewall, and runtime-process
inventories. Exact-revision daemon PIDs 18265/18283 remain active with zero
restarts. The failed Docker create therefore leaves no survivor; only the
maskedPaths compatibility boundary remains under investigation.

The self-cleaning runc-backed probe shows Docker's exact maskedPaths list has
12 unique entries: `/proc/acpi`, `/proc/asound`, `/proc/interrupts`,
`/proc/kcore`, `/proc/keys`, `/proc/latency_stats`, `/proc/sched_debug`,
`/proc/scsi`, `/proc/timer_list`, `/proc/timer_stats`,
`/sys/devices/virtual/powercap`, and `/sys/firmware`. readonlyPaths is the
five-entry list `/proc/bus`, `/proc/fs`, `/proc/irq`, `/proc/sys`, and
`/proc/sysrq-trigger`. The error's “duplicate or unsupported” branch is thus
the unsupported-set case, not actual duplication. These path restrictions are
security policy and cannot be discarded merely for Docker compatibility; the
current exact allowlist and the child's mount visibility must be compared
before implementation.

An exact Multikernel-child probe shows `/proc/interrupts` exists as a
readable mode-0444 proc regular file and returns 1091 bytes. Therefore the
Docker request is meaningful isolation: it must be accepted into the agent's
bounded root policy and actually masked, not removed during host projection.
The current allowlist differs from Docker's unique list by this one standard
path only; the other 11 masked paths and all five read-only paths already
match exactly.

The minimal agent correction adds only `/proc/interrupts` to the existing
bounded masked-path allowlist; application remains the existing read-only
`/dev/null` bind mask for files. A regression accepts Docker's exact ordered
12-masked/5-read-only lists and rejects duplicate and unsupported variants.
The focused agent race suite, focused vet, and `git diff --check` pass. Full
repository gates remain required before an immutable live revision.

Repository-wide Go race tests and vet pass. The first combined full-gate
command then stops before documentation checks because it invokes
`scripts/check-docs.sh` from the `runtime/` subdirectory, where that path does
not exist. This is a local orchestration/path error, not a checker failure;
the documentation gate must be rerun from the repository root.

The corrected root-level gate passes completely: documentation/link/schema/
evidence/deployment checks, the 78-case OCI boundary suite, all supporting
runtime suites, and final evidence audit succeed; `git diff --check` is clean.
The expected local socket-rejection `EPERM` subcase remains the sole
classified skip. Generated Python cache was removed. Together with the
already passing repository-wide race and vet runs, the one-path correction is
ready for an immutable source checkpoint and exact live rebuild.

Commit `9bd7e94e598bb9ba439d2cb51c7911402e972e51` freezes the bounded
`/proc/interrupts` policy correction. Its source-only archive
`/tmp/mklinux-9bd7e94.tar.gz` hashes to
`49a337ef044641f05c5a1a839fdad8166af83e0f4afe2ca7b72eb53a068946a7`;
Git metadata and the pre-existing untracked evidence tree are excluded.
Guest transfer, verification, build, activation, and workload proof remain
separate unclaimed boundaries.

The active strict manifest confirms the reusable bootstrap inputs are the
root-owned transport module `/opt/mkruntime/artifacts/mk_transport.ko` at
`bef1b888…` and relay `/opt/mkruntime/bin/mkvsock-relay` at `293ff1ea…`.
They remain covered by successful strict manifest validation; the new build
will resolve these exact pinned paths rather than infer artifacts from a prior
revision directory.

Guest hashing independently reproduces archive digest `49a337ef…`; extraction
is entirely root-owned and contains neither `.git` nor the excluded historical
evidence tree. Strict validation reconfirms the active shared bootstrap inputs.
The exact build succeeds with release manifest `5028dae7…`, shim `31fe5c7a…`,
mkruntimed `baf0833e…`, mknetd `f77e3215…`, agent `0cbfdf78…`, and
initramfs `5ea168b0…`. Privileged pipefail listing proves the initramfs contains
`mk-agent`, `mk_transport.ko`, and `mkvsock-relay`. No release, deployment, or
kernel manifest has yet been selected for this revision.

Managed installation selects immutable binary release `0.1.0-dev-9bd7e94…`;
all command links validate. Installing the exact-source support assets with
the existing qualified environment/config deterministically reuses deployment
`844df6d2…`, and every managed support link validates. The following source/
deployed validator comparison stops because the ordinary SSH user cannot
traverse the root-only extraction, so its service inspection does not run.
This is a diagnostic privilege error after selection, not activation; hashes
must be repeated under `sudo` and running process identities rechecked.

Privileged comparison proves exact-source and deployed OCI validators both
hash to `a6f2b969…`. mknetd/mkruntimed remain the pre-selection PIDs
18265/18283 with zero restarts, and `/proc/18283/exe --version` identifies
`92531eb…`. Thus selection has not changed running code. Revision artifacts
and a strict candidate manifest remain mandatory before restart.

Root-owned revision artifacts are staged at exact build hashes `0cbfdf78…`
and `5ea168b0…`. Strict bootstrap validation passes for candidate manifest
`effb90a2c222fd2675f14b0f9565c2606e820bb870ff5721664e20d6d93582e5`.
The active manifest and running processes remain `92531eb…`; a fresh clean
audit must precede coordinated activation.

The immediate pre-activation audit is fully empty across containerd/Docker,
children, storage and durable state, host networking/firewall rules, and
helper processes. The still-running `92531eb…` daemon PIDs 18265/18283 remain
healthy and unrestarted. Activation can proceed without displacing a workload.

Coordinated activation succeeds on unchanged boot `1151712d…`. Active
manifest hash is `effb90a2…`; the preserved `92531eb…` and rollback manifests
remain `807d1770…`. mknetd, mkruntimed, containerd, and Docker are active at
PIDs 26044/26063/26073/26109 with zero restarts/status 0, and the shim,
mkruntimed, and mknetd all report exact revision `9bd7e94…`. This establishes
coherent activation only; workload behavior remains to be executed.

Post-activation inventory remains zero across every workload, child,
storage/state, networking/firewall, and helper-process category. The new
daemon PIDs 26044/26063 remain active with zero restarts. This is the clean
starting boundary for the exact-revision basic runner.

The first new-revision suite invocation does not enter the runner: the
ordinary qualification user cannot traverse the intentionally mode-0700
root-owned source extraction. `cd` returns permission denied before the hash
or script executes, so no runtime behavior is inferred and no cleanup is
needed. The exact runner must be installed into a user-readable temporary path
under privilege, hash-compared to source, then executed non-root.

The copied runner matches exact source hash `7fb6cc6e…` and executes under UID
1001. ctr again reaches `RUNNING`, but Docker still fails with
`linux.maskedPaths contains a duplicate or unsupported path`, then invokes
cleanup. Thus the runc bundle's 12-entry list was not sufficient to explain
the request reaching the Multikernel agent; the one-path correction is not a
live fix and the suite is not a pass. The exact Multikernel-bound list must be
captured after independent cleanup, without broadening policy speculatively.

The independent failure audit is again completely empty, with exact-revision
daemons 26044/26063 active and unrestarted. The failed create remains
fail-clean; diagnostic capture can proceed from a known empty host.

Source inspection distinguishes the two otherwise similar diagnostics. The
live message says “duplicate or unsupported,” exactly the host Python
validator's wording; the agent says “unsupported or duplicate.” The
`9bd7e94…` change updated only the agent allowlist, while the independently
bounded host `SAFE_MASKED_PATHS` still omits `/proc/interrupts`. The request
was therefore rejected before guest projection and never exercised the new
agent path. Both enforcement layers must share the same exact bounded set,
with tests at the host boundary as well.

The host validator now adds exactly `/proc/interrupts` to its existing bounded
set, matching the agent. A host regression accepts Docker's complete exact
12-masked/5-read-only policy and rejects a duplicate. Python compilation, the
focused fail-closed suite (now 80 semantic cases), and diff checking pass.
Complete repository gates remain required before a replacement immutable
revision.

The full local gate passes with the synchronized policy: repository-wide Go
race tests and vet, documentation/link/schema/evidence/deployment checks, the
expanded 80-case OCI suite, all supporting runtime suites, final evidence
audit, and diff checking. The expected local socket `EPERM` subcase remains
the only classified skip; generated Python cache is removed. The host-layer
correction is ready for a new immutable checkpoint and exact live rebuild.

Commit `d0c33cc3e7b28fae45056fe58c88a3773384037d` freezes the synchronized
host/guest policy. Its source-only archive `/tmp/mklinux-d0c33cc.tar.gz`
hashes to
`67dd2201d9e1583011552943e1df797da14998ba081685622500a1929ada4806`;
Git metadata and the pre-existing evidence tree are excluded. All guest
boundaries must be repeated for this exact revision.

The guest independently matches archive digest `67dd2201…`, verifies a wholly
root-owned extraction and exclusions, and revalidates the manifest-pinned
module/relay. Exact build hashes are `fb85f3ce…` (release), `610a9b0e…`
(shim), `d8d66197…` (mkruntimed), `2dc31c1f…` (mknetd), `f3a6c2c3…`
(agent), and `666e30c3…` (initramfs). Privileged listing confirms agent,
module, and relay membership. Selection and activation remain unclaimed.

Managed selection installs binary release `d0c33cc…` and changed support
deployment `dd3ce8cc2f440a3898a0885f0377d4216666cd0a5e08606f95ccba5682e65dbb`;
all links validate. Exact-source and deployed validator hashes match at
`70a98070…`. Running daemon PIDs remain 26044/26063 with zero restarts, and
direct executable identity is still `9bd7e94…`; selection has not yet changed
running code.

Staged artifacts reproduce build hashes `f3a6c2c3…`/`666e30c3…`; candidate
manifest `831208794a0aa3c7da4eb6fb77c245fb82b59c3ea753825a2a5872267983d7f9`
passes strict validation. The immediate audit is fully empty and predecessor
daemons remain stable at PIDs 26044/26063. Coordinated activation may proceed.

Activation succeeds on unchanged boot `1151712d…`. Manifest `83120879…` is
active; predecessor `9bd7e94…` remains preserved/rollback at `effb90a2…`.
mknetd, mkruntimed, containerd, and Docker PIDs 31537/31557/31568/31603 are
active with zero restarts/status 0, and all runtime binary identities agree on
exact `d0c33cc…`. Workload proof remains separate.

The post-activation audit is fully empty across all tracked resource classes;
new daemon PIDs 31537/31557 remain stable and unrestarted. The exact runner
has a clean starting boundary.

The exact copied runner again hashes to `7fb6cc6e…`. ctr reaches `RUNNING`,
and Docker now clears both host and agent masked/read-only path validation,
proving the synchronized correction live. Its next fail-closed rejection is
`mounts[4].source must be an absolute canonical bounded path`; Docker does not
start and cleanup runs. The suite is not a pass. Cleanup and Docker's exact
indexed mount request must be observed before changing the mount contract.

The independent audit is completely empty and exact daemons 31537/31557
remain active with zero restarts. The mount rejection is fail-clean.

The self-cleaning runc bundle identifies index 4 exactly as destination
`/sys/fs/cgroup`, type/source `cgroup`, options
`ro,nosuid,noexec,nodev`. Indices 0–3 and 5–6 are standard proc/dev/sys/mqueue/
shm mounts; indices 7–9 are Docker-managed `/etc/{resolv.conf,hostname,hosts}`
binds and remain separate policy boundaries. The cgroup entry must be compared
with the dedicated child's own kernel/cgroup view before deciding whether the
host validator may consume it as a fixed default.

Inside an exact Multikernel child, `/sys/fs/cgroup` is only a mode-0555
directory and `/proc/mounts` contains no cgroup or cgroup2 filesystem. The
host hierarchy is therefore not exposed across the dedicated-kernel boundary.
Accepting only Docker's exact read-only default and omitting host projection
preserves a stricter child view; writable, reordered-value, wrong-type/source,
or option-modified forms must remain rejected.

The first focused regression run fails in its expectation code, not the
validator: validation accepts the exact cgroup default and correctly omits it
from guest projection, while the test oracle's hardcoded consumed-default set
does not yet include `/sys/fs/cgroup`. The oracle must be synchronized before
the result can be assessed.

After synchronizing the oracle, the focused fail-closed suite passes 82
semantic cases. It accepts only the exact read-only Docker cgroup mount,
consumes it from guest projection, and rejects the writable variant; Python
compilation and diff checking also pass. Full gates remain pending.

The complete local gate passes: repository-wide Go race tests and vet, all
documentation/schema/evidence/deployment and supporting-runtime checks, the
82-case OCI suite, final evidence audit, and diff hygiene. Only the classified
local socket `EPERM` skip remains; generated cache is removed. The exact
cgroup-default correction is ready to freeze and rebuild.

Commit `0a6668e9a9699ec8373321c347dc778a86038e7d` freezes the exact read-only
cgroup-mount compatibility contract. Its source-only archive
`/tmp/mklinux-0a6668e.tar.gz` hashes to
`2ae8239c36bc0306f6d74947a5d5d57ff0dc288028b7e9ee57c23f0b2f431066`;
repository metadata and the pre-existing evidence tree are excluded. Exact
guest transfer, rebuild, changed support deployment, activation, and workload
rerun remain pending.

On resumption, authoritative VM state still matches the checkpoint: boot
`1151712d…`, exact `d0c33cc…` binary identities, stable daemon PIDs
31537/31557 with zero restarts, and zero counts across every audited workload,
child, storage/state, network/firewall, and helper-process category. This is
the clean pre-build boundary for `0a6668e…`.

The guest independently matches archive digest `2ae8239c…`, verifies the
root-owned extraction/exclusions and approved module/relay hashes, and builds
exact release/shim/mkruntimed/mknetd/agent/initramfs identities
`964260e0…`/`2b8cbc9b…`/`a5a7d9b2…`/`d257448c…`/`b2acc4c5…`/
`e9f41b5d…`. Privileged listing confirms all required bootstrap members.
Selection and activation remain unclaimed.

Managed selection installs release `0a6668e…` and support deployment
`7a2e5a67f9dcede3aae6402ff1b1d0db436d2cdbabeef1c7405b8f6cb7a12d1b`;
all links validate and source/deployed validator hashes match at `74dc8f63…`.
Running daemon PIDs remain 31537/31557 with zero restarts and direct identity
`d0c33cc…`, proving selection has not activated the new code.

Staged agent/initramfs reproduce `b2acc4c5…`/`e9f41b5d…`; candidate manifest
`747eb832dad812b7d5ffb1cdb75c81c2721a5a1ac721f4d205022da47bbd7376`
passes strict validation. The immediate pre-activation resource audit is fully
empty and predecessor daemons remain stable. Coordinated activation may
proceed without displacing a workload.

Coordinated activation succeeds on unchanged boot `1151712d…`. Active
manifest is `747eb832…`; predecessor `d0c33cc…` remains rollback at
`83120879…`. mknetd/mkruntimed/containerd/Docker PIDs
39910/39929/39940/39977 are active with zero restarts/status 0, and all runtime
binary reports agree on exact `0a6668e…`. Workload behavior remains separate.

The immediate post-activation audit remains zero across every tracked
resource/process category, with new daemon PIDs 39910/39929 stable and
unrestarted. The exact workload runner starts from a clean boundary.

The exact runner (`7fb6cc6e…`) proves ctr `RUNNING` and Docker clears the new
cgroup-mount contract. Its next rejection is
`OCI mount '/dev/shm' differs from the enforced default contract`. Live runc
evidence already shows Docker's otherwise identical default uses
`size=67108864`, while the validator pins `size=65536k`; both denote exactly
64 MiB. Cleanup runs and the suite is not a pass. Compatibility may admit
only these two exact equivalent spellings, retaining rejection of every other
size or option change.

The independent post-failure audit is completely empty and exact daemons
39910/39929 remain active with zero restarts. The rejection is fail-clean.

A self-cleaning runc probe with explicit Docker `--read-only` preserves the
same default mounts and changes each managed `/etc/{resolv.conf,hostname,hosts}`
bind to exact options `rbind,rro,rprivate`; root.readonly is also explicit.
`rro` is recursively read-only and can be normalized to the existing guest
`bind,ro,nodev,noexec,nosuid` materialization without weakening policy.
Qualification will request `--read-only`, and validation may accept exactly
one of `ro` or `rro`; absence or conflicting markers remains fatal.

The first focused run passes all 86 semantic cases but diff hygiene catches a
single trailing space in the new fixture. This is a formatting failure, not a
behavioral pass of the complete gate; it is corrected before rerun.

After correction, the focused suite passes 86 cases. The complete repository
gate also passes: all Go race tests, vet, documentation/schema/evidence/
deployment and supporting-runtime checks, final evidence audit, runner syntax,
and diff hygiene. The classified local socket `EPERM` skip is unchanged and
generated cache is removed. The combined exact-shm/read-only-bind checkpoint
is ready to freeze.

Commit `1dd73eb091325a7983931c4362fcddbeb350fa4c` freezes the exact 64-MiB shm
alias, recursive read-only bind contract, and explicit Docker `--read-only`
qualification. Source archive `/tmp/mklinux-1dd73eb.tar.gz` hashes to
`338429c30522efcf6606f8b1b2f1dacad76f4be530ae581a18242846b1657ced` and
excludes Git metadata and the historical evidence tree. Exact guest proof
remains pending.

Guest verification matches archive `338429c3…`, confirms root-owned extraction,
exclusions, and approved shared bootstrap inputs, then builds exact release/
shim/mkruntimed/mknetd/agent/initramfs hashes `c4000e65…`/`350e55ca…`/
`3eff3264…`/`376d383b…`/`851e3bd4…`/`1c765dee…`. Required initramfs
membership is proven. Selection and activation remain separate.

Managed selection installs release `1dd73eb…` and support deployment
`bc0517cce94b24246a9de13a1f8a800f42a8e6f3f62b188d4780851fb2daa6f7`;
source/deployed validator hashes agree at `830353a5…`. Candidate manifest
`4cb7ad221981d0bc2b54137aed99059aa5245e2833627c7e6435079e6e43c3f9`
strictly validates with exact staged hashes. The immediate resource audit is
fully empty and predecessor daemons remain stable; activation may proceed.

Activation succeeds on unchanged boot `1151712d…`. Active/rollback manifest
hashes are `4cb7ad22…`/`747eb832…`; mknetd/mkruntimed/containerd/Docker PIDs
45468/45488/45499/45534 are healthy with zero restarts, and all runtime binary
identities report exact `1dd73eb…`. Workload evidence remains separate.

The exact runner hashes to `15e12e19…`, passes preflight, and proves ctr
`RUNNING`. Docker clears every previously discovered OCI validation boundary,
builds/launches far enough to wait on runtime integration, then fails:
`bind-mount /proc/0/ns/net -> /var/run/docker/netns/...: no such file or
directory`. Cleanup runs. This indicates the shim reports task PID 0 where
Docker/containerd requires a host-visible network-namespace owner; it is a
new runtime API boundary, not an OCI-policy rejection. The suite is not a
pass, and cleanup plus PID/netns provenance must be established before repair.

The independent audit disproves cleanup: default namespace still contains one
task and container; one child, storage image, active sandbox, mknetd endpoint,
rootfs record, NAT rule, eight filter rules, two exact shim processes, and one
relay remain. Docker/moby is empty and daemons are stable. This is a material
cleanup failure, so no further workload may run until exact survivor IDs and
journals are captured and only the known qualification resources are removed.

Exact survivor capture identifies default task/container `mk-proof-ctr` (task
PID 162, `STOPPED`), sandbox `mk-mk-proof-ctr-592a31dcfc440cd2`, generation
`4edb8056…`, its matching endpoint/rootfs/storage state, and no moby object.
The lifecycle journal contains prior stop/delete completions but the current
generation remains `RUNNING`, while Docker's create call had only just
terminated. This supports a runner cleanup race. A bounded retry targets only
`mk-proof-ctr` after termination.

The bounded retry does not clean. `ctr tasks rm -f mk-proof-ctr` returns
`failed precondition`; container removal then reaches deletion but fails at
`close guest network: INTERNAL: agent operation failed`. Every survivor count
remains unchanged. This reproduces the earlier open guest-network cleanup bug
on exact `1dd73eb…`; code-level delete ordering and agent CloseTUN behavior
must be diagnosed before any forced recovery.

The complete local gate then passes: full Go race suite, `go vet ./...`, the
entire documentation/schema/evidence/deployment chain (with 72 OCI cases), and
`git diff --check`. The known unprivileged socket-rejection `EPERM` subcase is
the only classified skip. The sysctl/seccomp checkpoint is ready to commit;
exact-revision deployment and workload behavior are not yet proven.

Commit `298d3debe832696e18c749dd1bbed4d03e64433b` freezes the exact-map
sysctl/seccomp checkpoint. Its tracked-source-only archive
`/tmp/mklinux-298d3de.tar.gz` hashes to
`9520d18ef58d88f0a82b48560ae87a9a5b0ae1fb988c069e59af432e766be2b5`;
the pre-existing untracked evidence tree remains excluded and untouched.
Guest transfer and verification remain pending.

Guest-side hashing reproduces `9520d18e…` on unchanged boot `e76edca2…`.
The immediate pre-build audit is fully empty across both containerd
namespaces, Docker, children, runtime storage/state, host network resources,
and runtime helper processes; mknetd/mkruntimed PIDs 11328/11347 remain active
with zero restarts. Exact root-owned extraction/build can proceed.

The root-owned exact build completes. Hashes are release manifest
`f302816e…`, shim `33128b4c…`, mkruntimed `13513537…`, mknetd `db1a3dd1…`,
agent `5aa29a55…`, and initramfs `7b79a2a7…`; privileged/pipefail inspection
confirms the bootstrap contains agent, transport module, and relay. No new
release, support deployment, or manifest is selected yet.

Managed installation now selects binary release `0.1.0-dev-298d3de…` and
support deployment `6508b98f…`; every managed link validates and deployed OCI
validator hash `65f04894…` matches exact source. Running daemon PIDs remain
11328/11347 with zero restarts, so they still execute the preceding revision.
Revision artifacts and strict manifest validation remain mandatory before
coordinated restart.

Revision-specific agent/initramfs hashes `5aa29a55…`/`7b79a2a7…` are staged
root-only, and strict bootstrap validation passes for candidate manifest
`50c74397…`. It retains the pinned kernel/module/relay and exact compatibility,
config, and feature declaration. The candidate is not active; an immediate
clean-host preflight remains required.

The immediate activation audit again reports zero across all workload,
Docker, child, storage, active durable-state, host-network, and runtime-helper
inventories; daemon PIDs 11328/11347 remain stable with zero restarts.
Coordinated activation can proceed on unchanged boot without displacing work.

Coordinated activation succeeds on unchanged boot `e76edca2…`, with the a72
manifest preserved for rollback. Active hashes are `50c74397…` (kernel
manifest), `f302816e…` (release), and `33128b4c…` (shim); selectors resolve
exact `298d3de…` and deployment `6508b98f…`. All five units are active;
mknetd/mkruntimed/containerd/Docker PIDs are 18614/18633/18643/18676 with zero
restarts/status 0. Version reports and strict bootstrap validation agree.
Exact basic workload proof is next.

The exact `298d3de…` runner hashes to `ac1d54a7…`. ctr reaches `RUNNING`, and
Docker now clears AppArmor, OOM, seccomp, and sysctl validation. It next fails
closed with `only exact default resource contracts are supported`, exits 125,
and invokes the EXIT trap. The basic suite is still not a pass. Independent
cleanup plus live inspection of Docker's exact `linux.resources` object are
required before changing the resource contract.

The independent post-failure audit is fully empty across tasks, containers,
Docker, children, storage, active durable state, network resources, and helper
processes. mknetd/mkruntimed PIDs 18614/18633 remain active with zero restarts.
The next boundary is therefore isolated to Docker's resource projection, not
cleanup.

The runc-backed bundle shows Docker resources are not the existing containerd
default: `devices` contains an ordered deny/allow rule program (including an
explicit deny for character 10:229 and duplicated standard-device rules), and
`blockIO` is an empty object. The child mounts the full kernel devtmpfs into
the container root, so merely accepting and stripping Docker's restrictive
device program would weaken policy. Before validator work, qualification must
explicitly opt out of device-cgroup restriction and the resulting OCI rule
program must be observed to end in effective allow-all; otherwise Docker
support requires implementing device policy in the child.

The explicit `--device-cgroup-rule 'a *:* rwm'` probe appends an OCI rule
`{allow:true,type:"a",major:-1,minor:-1,access:"rwm"}` after Docker's complete
default program. Because device rules are ordered, that final all-device rule
makes the requested policy unrestricted and therefore equivalent to the
child's devtmpfs exposure; empty `blockIO` is inert. Validation can safely
accept and strip only this exact observed full object. The restrictive object
without the terminal allow-all, reordered/modified rules, and nonempty blockIO
must remain rejected, and qualification must supply the opt-out explicitly.

Exact Docker unrestricted-resource validation and the explicit qualification
flag are implemented across all Docker paths. The focused suite passes 75
semantic cases, including rejection of the same program without its terminal
allow-all, a modified terminal rule, and nonempty blockIO; Python compilation,
both shell syntax checks, and `git diff --check` pass. Full gates and an exact
live revision remain pending.

The complete local gate passes again: full Go race suite, vet, all
documentation/schema/evidence/deployment checks with 75 OCI cases, and diff
check. Only the already classified local socket `EPERM` subcase is skipped.
The unrestricted-device checkpoint is ready to freeze; live proof remains
pending.

The `1124739` coordinated activation stopped at deployment installation before
service restart. Binary release and atomically staged agent/initramfs/manifest
updates succeeded, but the new deployment manager rejected the active older
generation with `installed deployment manifest file set mismatch` because that
generation predates the newly managed validator asset. The services remain
stopped and no workload ran. This exposes an upgrade-compatibility defect in
the immutable deployment manager: it assumes every historical generation has
the current asset set. Verification and activation must understand the exact
file set recorded by a known historical generation without creating links to
assets it does not contain.

The deployment manager now recognizes only two explicit generation schemas:
the current complete file set and the immediately preceding set without the
new storage validator. It recomputes the deployment identity from sorted
manifest names and hashes, verifies every recorded file, creates links only for
assets present in the selected generation, and removes/restores the optional
link transactionally during rollback. A focused lifecycle test constructs an
identity-correct historical generation, activates it without a dangling new
link, upgrades to the complete generation, and rolls back; it passes together
with the storage validator and diff checks. Broad gates remain pending.

That full deployment lifecycle passed 100 consecutive repetitions. The
subsequent complete Go race/vet and documentation, schema, evidence, OCI,
bind/bootstrap/rootfs/storage/image, release/deployment/ledger/capture,
containerd, and final-audit chain passed; `git diff --check` was clean. The
expected local socket permission skip remains assigned to the guest. Commit,
exact rebuild, and recovery of the stopped live services remain pending.

The deployment-evolution fix was committed as `3ce80c8`; its uploaded archive
hash `e4a1cd26741188df2c8df64ddff31ec1dc947c8f168850e9fd8b617b7b19daeb`
matched on the guest. The exact build produced release manifest `bfa37246…`,
shim `eb418b63…`, mkruntimed `7d74471f…`, mknetd `a498f963…`, agent
`6e0a59af…`, and gzip-valid initramfs `0d61926c…`. These identities precede
the recovery deployment; services remain stopped at this checkpoint.

The recovery deployment passed the historical-generation transition. Exact
release `0.1.0-dev-3ce80c89e10f8aefcf70c625d232b6705b1f490d` and storage-aware
generation `01c685c211346c6d8eec3930e8bee84f04348a4461b35938063e1cf4952d97d0`
became active; bootstrap and installed hashes matched, and both services were
active after five seconds on the qualified mount. This proves live systemd
parsing and the positive pre-start validator path. The deliberate unmounted
negative service-start test remains next.

The managed negative/positive service test passed. After an empty-inventory
check, mkruntimed was stopped and the disk unmounted. `systemctl start
mkruntimed` returned 1, its pre-start journal recorded `runtime storage mount
rejected: runtime storage path is not one distinct mountpoint`, and no child
appeared. After resetting the failed unit, the same by-id disk was remounted,
the installed validator returned `RUNTIME_STORAGE_MOUNT_VALID`, and mkruntimed
held stable PID `10351` for five seconds. This closes the live reboot-fallback
service boundary; the basic workload suite remains pending.

The post-storage-remediation basic retry still failed before ctr task creation
with `build child root: exit status 1` and an empty stderr suffix. The stable
host boot ID remained `f9d00c5f…`, so this is no longer attributable to reboot
or missing storage, and the earlier OCI resource rejection text did not recur.
The suite exited 1 and cleanup ran; no workload pass is claimed. A one-shot
builder-stage trace is required to identify the silent `set -e` boundary.

The one-shot trace proved every OCI, bootstrap, image, source-copy, mediated
ext4 build, initramfs build, and independent verification stage succeeded. The
failure is the final equality assertion: `jq -e --slurpfile ...` is invoked
without `-n` and receives no stdin, so jq exits 4 without evaluating the two
loaded result documents. The managed builder link was restored immediately and
the diagnostic container removed. The assertion must use `jq -n -e`, with a
regression check that the final independent build/verify comparison cannot
silently lose null input.

The final comparison now uses `jq -n -e`; the OCI/builder contract test
requires that exact null-input form so removing it fails locally before live
qualification. Builder shell syntax, the 62-case OCI suite, and diff checks
pass once. The root-only trace files were deleted after verifying the restored
managed link, empty inventories, qualified mount, and active services.
Repeated and broad gates remain pending.

The OCI/builder suite passed 100 repetitions with the final null-input proof
assertion. The complete Go race/vet and documentation, schema, evidence,
OCI/bind/bootstrap/rootfs/storage/image, release/deployment/ledger/capture,
containerd, and final-audit chain then passed, as did `git diff --check`.
Exact commit, guest rebuild, and live workload retry remain pending.

The final-proof correction was committed as `2864cff`. The guest verified its
source archive hash `4ff24454f4051ccefe0d09aa8a0780f74a3e705ba2cc768623466268a224e531`
and built full-revision outputs: release manifest `ce2426bc…`, shim
`1de0faae…`, mkruntimed `6d8e5fca…`, mknetd `3c77289d…`, agent `793bcaf9…`,
and gzip-valid initramfs `8125169d…`. Deployment and live retry remain
separate pending steps.

The exact correction is active as release
`0.1.0-dev-2864cff996498f32df0ae1ce5c6bd698fddcff06` and deployment
`5967012e86593a26b1aae8e184e335e459969c5af2d4fa0ad948c60c5ab6f896`.
Empty inventories and the qualified mount preceded the switch; bootstrap
validation passed, and both services remained active with stable mkruntimed PID
`12242` for five seconds. The basic workload retry is next and not yet claimed.

The `2864cff` retry still returned a blank-stderr builder exit 1 before ctr
task creation. Because the former jq site would exit 4 and is now corrected,
this is a distinct later boundary after the independent comparison rather than
a recurrence of that bug. The boot ID remained stable and cleanup ran; no live
pass is claimed. A second one-shot stage trace is required beyond the final
comparison.

The second trace completed every builder command, including the corrected
comparison and final result JSON. Unlike the earlier failures, the diagnostic
advanced to a containerd task in `CREATED` state and a live Multikernel child
`mk-mk-builder-trace-diag2-fc2448f635970bae`; the initiating SSH command
returned no transcript while task start remained unresolved. This is positive
builder evidence and a new child-launch/agent-readiness boundary. The live
objects are intentionally left in place until their state, process, and service
logs are captured; cleanup has not yet been claimed.

State capture found the host pool initialized, two CPUs and 3 GB assigned, the
kernel image loaded, the NBD server alive, and no network link yet. Normal
`ctr tasks rm -f` then failed with `stop sandbox: FAILED_PRECONDITION: invalid
state LOADED`; the task, child, and loaded image remain for diagnosis. This is
a cleanup state-machine defect: cancellation or transport loss between load
and start must be able to unload/delete a `LOADED` sandbox. No clean-state
claim is made.

Code-level diagnosis narrows that cleanup defect to the public lifecycle
transition. `DeleteSandbox` already supports non-running `LOADED` sandboxes by
releasing storage and asking the Kerf backend to unload/delete them, and intent
recovery already accepts physical `LOADED` as successful completion of a stop.
Only the public stop entry point rejects durable `LOADED`. The correction must
therefore treat `LOADED` as already stopped, persist `STOPPED` without issuing
an invalid `kerf kill`, and retain the real backend stop for `RUNNING`.

The lifecycle correction and regression now pass 100 race-detector
repetitions. The test proves the durable result is `STOPPED`, the physical
backend remains safely `LOADED`, no backend stop/kill is invoked, and the
ordinary delete path subsequently removes the sandbox. Full repository gates
and exact-source disposable-host recovery remain pending.

The subsequent complete local gate passed: full Go race suite, `go vet ./...`,
documentation and local-link checks, all 7 schemas/22 cases, all 17 classified
historical evidence manifests, the 62-case OCI suite, bind/bootstrap/rootfs/
storage/mount/root/image suites, release/binary/deployment/ledger/capture/
containerd checks, final-evidence audit, and `git diff --check`. The expected
locally permission-gated socket subcase remains reserved for the disposable
host. Exact-source deployment and recovery of the preserved live task remain
pending.

The remediation checkpoint is commit
`04da537332ce018c173ae2351679be5a09b20d81`. Its source-only archive hashes to
`479287d56ba6bafd95319a8ca61d7b16df0b365ca6bf7e0385c7658bbf961ab4`.
The disposable instance is still `RUNNING`; archive transfer, guest hash
verification, rebuild, deployment, and preserved-task recovery are separate
pending boundaries.

The guest independently verified that exact archive hash, extracted it into a
new private source directory, and confirmed the lifecycle source is present.
No artifact identity or runtime behavior is inferred from transfer success;
the exact-source rebuild is the next boundary.

The exact guest rebuild completed for all seven revision-stamped components.
The release manifest is `122bf1a…`; representative identities are shim
`f809e51a…`, mkruntimed `bae3b11e…`, mknetd `0892a2fd…`, and agent
`6600d82b…`. Direct version output reports the full `04da537…` revision for
both daemon and shim. Installation, service restart, and preserved-task
recovery remain pending and are not implied by the build.

The first activation precheck confirmed the same boot ID, active daemon/
network/containerd services, the preserved task in `CREATED`, and the qualified
`/dev/sdb` ext4 mount. It then stopped before installation because the
standalone storage validator was invoked with a nonexistent `--environment`
convenience flag instead of its six explicit identity arguments. No release or
service was changed by that attempt; installation remains pending.

The verified release then installed and became the active immutable release
`0.1.0-dev-04da537332ce018c173ae2351679be5a09b20d81`. Only mkruntimed was
restarted; after five seconds it and mknetd were active, mkruntimed PID `14571`
reported the exact revision, and the boot ID was unchanged. The preserved task
has not yet been deleted, so this is activation evidence rather than recovery
evidence.

The first preserved-task retry did not reach the corrected transition. The old
shim attempted its recorded daemon endpoint `/run/mkruntimed.sock`, but after
the daemon restart that pathname did not exist, so ctr returned `UNAVAILABLE`
before stop/delete. The task and child must still be treated as live. This is a
restart/recovery endpoint-continuity finding; socket paths and daemon journal
must be inspected before another cleanup attempt.

Journal inspection corrects the earlier five-second activation observation:
mkruntimed is not stable. Roughly 18 seconds into each start, rootfs reconcile
fails closed with `prepared build result differs from journal`; systemd then
restarts it, creating misleading brief `active` windows and periods with no
daemon socket. The storage prestart check passes on every attempt. The
preserved task remains `CREATED` and its child remains present. Qualification
must fix this prepared-rootfs upgrade/reconcile mismatch and use a health
window longer than the observed failure latency.

Source inspection identifies the mismatch mechanism. The builder writes its
JSON result bytes verbatim to `build-result.json`, while the durable rootfs
store embeds the same `json.RawMessage` inside `json.MarshalIndent` output.
After restart, unmarshalling yields the indented embedded bytes; reconcile then
uses raw byte equality against the original compact artifact. The JSON value is
unchanged, but whitespace makes every prepared record fail restart validation.
The comparison must remove JSON whitespace from both valid documents before
requiring byte equality, preserving key order and numeric/string lexemes rather
than weakening the check to an unordered decoded-object comparison.

The whitespace-stable verification correction now passes 100 rootfs race-
detector repetitions. The integration regression takes a real builder result,
indents it as durable-state persistence does, and proves all prepared artifact
and digest checks still succeed; the existing mutation cases continue to
reject changed artifacts. Full gates and exact-source redeployment remain
pending.

The subsequent full local gate passed again: repository Go race and vet,
documentation/link/schema/evidence validation, all boundary simulation suites,
release/deployment/ledger/capture/containerd tests, final evidence audit, and
`git diff --check`. The generated Python cache was removed. Commit, exact
archive rebuild, and live daemon recovery remain pending.

The reconcile correction is committed as
`89aada60b430386b0e5ec51c322006e480e5488e`. Its source-only archive hashes to
`2d1e93c18d5e92588d9db829ae76b369677cf575783f6ac3aa1f5513ded2cf76`.
Transfer, guest verification, exact rebuild, activation, and a health window
beyond the prior 18-second failure remain separate unclaimed steps.

The guest matched the full `89aada6` archive hash and rebuilt all seven
components. The release manifest is `db262432…`; shim is `66dc651c…`,
mkruntimed `6f2f2348…`, mknetd `a5ab9e91…`, and agent `31369e71…`.
Mkruntimed reports the exact full revision. Activation and durable health are
still unclaimed.

Release `0.1.0-dev-89aada60b430386b0e5ec51c322006e480e5488e` is now active.
After an explicit restart, mkruntimed retained PID `15958`, its daemon socket,
and exact version for 35 seconds—well beyond the former approximately
18-second failure—and the boot ID remained unchanged. The journal's preceding
failure belongs to the old binary; the new start has not restarted. Preserved-
task cleanup is the next boundary.

The preserved task cleanup then succeeded through normal `ctr tasks rm -f`.
The immediate audit found no ctr task, no Multikernel child, and zero durable
rootfs records. Its process-name search was over-broad and matched the daemon
and audit shell because their command lines mention `mkvsock-nbd`; the audit
then stopped when it assumed the lifecycle journal was
`/var/lib/mkruntimed/state.json`, which is not the configured/default path.
Thus task/child/rootfs reclamation is established, but exact NBD, lifecycle,
storage, pool, service, and container-metadata checks still require a corrected
audit before claiming fully clean state.

The next audit command also stopped early because `find` was given a missing
`/run/mkruntimed` operand under `pipefail`. Before stopping, it revealed one
generation-bound NBD log still present in `/run/mkstorage`. This may be a
runtime-log cleanup leak; it must be correlated with the storage journal and
exact server process inventory rather than silently ignored.

The corrected full audit confirms: task inventory empty, no child, no exact
`mkvsock-nbd` process, zero rootfs records/artifacts, all CPUs online, stable
mkruntimed PID `15958`, active daemon/network/containerd services, and unchanged
boot ID. However, storage state still contains one export and its runtime log
remains. Container metadata also remains by design until resource cleanup is
resolved. Therefore loaded-task cleanup is materially improved but not yet
fully leak-free.

Non-secret state inspection resolves that residual: the export is deliberately
retained in terminal `RELEASED` state with a release timestamp, zero counters,
a successful offline-check result, and exact terminal
`MKNBD_SERVER_CLOSED synced=1` evidence. There is no NBD process, prepared
image, child, or rootfs record. The state entry and generation log are durable
release/audit evidence used for idempotency and restart proof, not a live
resource leak. Container metadata can now be removed and the clean inventory
rechecked.

Container metadata removal and the final inventory passed. Ctr task/container,
Multikernel child, exact NBD process, rootfs record/artifact, and runtime-link
inventories are empty; all CPUs are online; daemon/network/containerd remain
active with stable PID `15958`; boot ID is unchanged. Kerf additionally reports
no memory pool, no instances, and an empty `/proc/kimage`. The preserved
post-load/pre-run failure is therefore recovered without live resource leaks.
The ordinary live qualification suite must now be rerun from the exact source.

The first suite invocation did not start: the archive's script is not directly
executable and this sudo policy ignores `-E`, so sudo returned permission
denied before the script body ran. No workload or runtime state was changed.
The retry must invoke the exact archived script explicitly with `bash` and rely
on its documented configuration rather than wholesale environment retention.

Running the script through root-owned `sudo bash` also stopped before workload
creation because the suite intentionally requires an ordinary sudo-capable
operator. Its entry cleanup found nothing to remove. The correct invocation is
ordinary-user `bash` on the exact archived file; the script itself applies
sudo only to bounded privileged operations.

Before the corrected ordinary-user invocation could start, GCE reported that
`mklinux-g4-g6-final-20260905` no longer exists. The suite did not execute and
no new workload evidence was produced. The previous exact-source recovery
evidence remains recorded, but continuing live qualification now requires a
new disposable instance and fresh qualification/provisioning rather than
assuming the deleted host's state.

Cloud inventory confirms there are currently no instances in the qualification
zone. The non-auto-delete 20 GB `mk-mediated-storage-20260830` disk remains
`READY` and unattached, and the qualified 100 GB source snapshot
`mklinux-lab-pre-daxfs-20260828-2030` remains `READY`. The recorded predecessor
used `n2-standard-16`, a 100 GB pd-balanced auto-delete boot disk restored from
that snapshot, the retained disk as a non-boot/non-auto-delete attachment,
Secure Boot disabled, and disposable/purpose labels. Recreation will reproduce
that shape rather than inventing a different host.

Recreation succeeded. GCE created the 100 GB pd-balanced boot disk from the
qualified snapshot, then created `mklinux-g4-g6-final-20260905` as
`n2-standard-16` with internal address `10.148.0.57`, ephemeral external
address `34.87.128.162`, the persistent storage attachment, and status
`RUNNING`. This is control-plane evidence only; guest kernel, boot identity,
disk identity, cleanliness, and service qualification remain pending.

Fresh guest baseline now confirms hostname, new boot ID
`e22e4b51-1038-4254-89f8-fc91d57a74a9`, qualified kernel
`7.0.0-mk2-gce-lab`, x86-64, 16 CPUs, approximately 64 GiB RAM, and an active
Google guest agent. `/sys/fs/multikernel` and its module are present with no
children. The retained whole `/dev/sdb` is exact 20 GiB ext4 with label
`mk-mediated-host`, UUID `507c0523-8e58-4ae3-9524-3b7513aad344`, and serial
`mk-mediated-storage-20260830`; it is deliberately not yet mounted. Runtime,
Kerf, container engines, topology, and clean-state qualification remain.

The first prerequisite inventory repeated an earlier quoting pitfall:
`${Status}` and `${Version}` in dpkg's format were expanded by the remote shell
under `set -u`, so its `missing` package lines are invalid evidence. The rest
of the read-only command did establish inactive containerd/Docker/runtime
services, empty visible workload inventories, retained `~/src/kerf` and
`~/src/linux`, and only the historical child initramfs in the artifact
directory. Package inventory must be rerun with shell-safe dpkg arguments.

The corrected installed/missing decision is reliable: containerd, Docker, Go,
and socat are absent; the remaining listed build/runtime prerequisites are
installed. Version formatting still printed literal `${Version}` and must not
be used as version evidence. Kerf is clean at exact revision
`8b72b3e9b266f8d32e707e2c1743ad7afc50b1ec` and reports 0.2.0; Linux is clean
at `3bdd35b64413da0b4e089ce931bfc2e8b031cbf7`. CPU topology is one socket,
cores 0-7 with sibling CPUs 0-7/8-15, matching the pool design.

Default dpkg output then supplied unambiguous installed versions, including
musl-tools 1.2.5, BusyBox 1.37, cpio 2.15, e2fsprogs 1.47.2, GCC 15.2,
iproute2 6.19, iptables 1.8.11, jq 1.8.1, make 4.4.1, Python 3.14.3, rsync
3.4.1, and util-linux 2.41.3. Only the four absent packages now require
installation.

Missing package installation succeeded from Ubuntu Resolute repositories:
containerd `2.2.2-0ubuntu1.1`, Docker `29.1.3-0ubuntu4.1`, Go 1.26, and socat
`1.8.1.1-1ubuntu0.1`. Containerd and Docker are active as package-installed
defaults. No Multikernel integration or workload claim follows from package
activation; inventories and defaults must be checked before configuration.

The recreated guest independently matched the exact `89aada6` archive SHA-256
`2d1e93c18d5e92588d9db829ae76b369677cf575783f6ac3aa1f5513ded2cf76`,
extracted it into a new private directory without `.git`, and found the
expected Go module. That revision's `verify-host.sh` then exited successfully.
This closes source transfer and basic host qualification only; build and
deployment remain pending.

The recreated guest's exact build is reproducible: release manifest
`db262432…`, shim `66dc651c…`, mkruntimed `6f2f2348…`, mknetd `a5ab9e91…`,
and agent `31369e71…` match the earlier `89aada6` guest build byte-for-byte.
Warning-clean static C builds also reproduce NBD helper `a0259098…` and relay
`293ff1ea…`; all three guest executables are static x86-64 ELF. The transport
module, kernel/bootstrap manifest, storage mount, and deployment remain open.

The pinned transport build reproduced module `bef1b888…`; `modinfo` reports
name `mk_transport` and exact `7.0.0-mk2-gce-lab` vermagic. The child kernel is
static x86-64 `vmlinux` hash `5cdf26d0…`; running-kernel image and config hashes
were also captured. This closes the transport/kernel build boundary, not the
root-owned staging or bootstrap-validation boundary.

The exact agent bootstrap initramfs built successfully and is gzip-valid with
hash `09314674…`. Archive inspection confirms the required `init`, `mk-agent`,
`mk_transport.ko`, and `mkvsock-relay` members. It has not yet been installed
or approved by a root-owned manifest.

With the disk unmounted, read-only `e2fsck -fn` completed all five passes and
reported a valid filesystem. The exact UUID fstab entry was installed, and the
by-id disk now mounts at `/srv/multikernel-storage` as rw ext4 with
`nosuid,nodev`. Retained `child-a`/`child-b` historical images remain outside
the runtime subtree; the existing `runtime` directory showed no entries in the
bounded inventory. Runtime storage validation will be repeated after service
configuration.

Pinned Kerf 0.2.0 is installed non-editably in root-owned
`/opt/mkruntime/kerf-venv`; dependency imports and the CLI version check pass,
and the trust-boundary directories/executable are root:root 0755. The generated
host config, storage-aware runtime environment, and exact artifact manifest are
valid JSON/text with hashes `82270b0c…`, `8d6184b8…`, and `afdbcc23…`, and were
transferred to the guest. Remote hash verification and privileged artifact/
deployment installation remain pending.

Remote configuration hashes matched before mutation. Root-owned kernel,
initramfs, module, agent, relay, and NBD helper installations exactly re-match
their build hashes. Immutable runtime release
`0.1.0-dev-89aada60b430386b0e5ec51c322006e480e5488e` is the sole active release
with all six managed host/CNI links. The root-owned exact-source extraction
contains the deployment inputs, and bootstrap validation accepts manifest
`afdbcc23…`, exact release/config pins, all artifact hashes, and the ten claimed
OCI features. Managed deployment and service activation remain separate.

Deployment generation
`5967012e86593a26b1aae8e184e335e459969c5af2d4fa0ad948c60c5ab6f896`
installed as the sole active generation. Inspection confirms every systemd,
environment/config, CNI/containerd, builder, validator, guest-init, and helper
link is managed. The installed storage validator returned
`RUNTIME_STORAGE_MOUNT_VALID` for the exact disk identity. Container-engine
integration and service activation remain deliberately separate.

Pre-integration engine audit passed: ctr tasks/containers and Docker containers
are empty, no explicit containerd main config or Docker daemon config exists,
containerd's default v3 dump includes `/etc/containerd/conf.d/*.toml` and keeps
`default_runtime_name = 'runc'`, and Docker reports only runc with runc as
default. The managed Multikernel fragment is installed but not yet effective
until a complete main config is staged and the idle daemon restarted.

The first integration command stopped before installation because its fixed-
string grep searched for literal `\x27` bytes around `runc`. The staged full
candidate and resolved dump are present and do show the import, Multikernel
runtime stanza, and exact `default_runtime_name = 'runc'`; neither main config
nor Docker config was installed, and both services remain active. The retry
must validate the actual quote characters without the broken escape.

Corrected integration passed. The complete containerd main config was installed
only after empty-inventory validation; post-restart effective config contains
the Multikernel handler and exact runc default. Docker's merged config passed
`dockerd --validate`, reloaded, and now advertises
`io.containerd.multikernel.v2`, `io.containerd.runc.v2`, and `runc` while
retaining runc as default. All engine inventories remain empty. Runtime service
activation is next.

Managed service activation passed a 35-second stability window. Mkruntimed PID
`12120` and mknetd PID `12095` remained unchanged, both binaries report exact
revision `89aada6…`, the daemon socket exists, installed storage validation
passes, child inventory is empty, and boot ID remains
`e22e4b51-1038-4254-89f8-fc91d57a74a9`. This exceeds the earlier restart-
reconcile failure latency. The ordinary-user live workload suite is now the
next boundary.

The exact ordinary-user suite reached its first real ctr task creation but
failed closed at `build child root: exit status 1` with no diagnostic suffix.
All service/mount/shim/TUN/empty-child preconditions passed, BusyBox was pulled,
and the boot/kernel identities were printed before failure. The cleanup trap
ran. This reproduces the silent builder boundary on a genuinely fresh host,
despite the earlier jq correction; no G4/G5/G6 pass is claimed. Immediate
state/leak inspection and a fresh exact-stage trace are required.

The first post-failure audit confirms empty ctr task/container and child
inventories plus zero rootfs records. It stopped when the storage state file
was absent—which is expected if preparation never reached export creation—so
remaining runtime-directory, service, and journal checks must be rerun without
assuming that optional file exists.

The tolerant audit completes fail-clean proof: storage state is absent, runtime
storage and runtime-log directories contain no artifacts, all four services
remain active with unchanged runtime PIDs `12120`/`12095`, and the daemon
journal contains no crash/restart or additional error. A temporary root-only
wrapper will now redirect builder xtrace to a root-only file while preserving
stdout, with the managed builder link restored immediately after one suite
attempt. Raw trace contents will not be published because runtime inputs may
contain ephemeral authentication material.

The one-shot trace restored the exact managed link and exposed two distinct
facts. The builder itself completed every stage through the corrected
`jq -n -e` comparison and final JSON publication. Task creation then failed at
approved kernel-manifest validation. Source inspection localizes that failure:
the daemon requires `config-<kernel_release>` beside the manifest kernel and
checks every advertised required-config line, while provisioning installed the
kernel but omitted its config companion. The standalone bootstrap validator
does not perform this config-file check, so its earlier pass was insufficient.
Installing the already hashed `/boot/config-7.0.0-mk2-gce-lab` beside vmlinux
is required before retry; no source-policy weakening is appropriate.

The exact kernel config companion is now root-owned beside vmlinux, re-matches
hash `f7a61b04…`, and contains all three required lines exactly. The managed
builder link was verified restored, private trace/output files were deleted,
and all engine inventories remain empty. The next ordinary suite retry will
test daemon manifest approval and the unwrapped builder path together.

The unwrapped retry again failed at the silent builder exit even though the
kernel companion is now complete. This contrasts with the traced wrapper,
which ran the versioned target by its resolved path and completed. The wrapper
therefore changed path semantics and is not sufficient evidence that the
normal public managed path works. After confirming fail-clean state, the next
diagnostic must keep the public symlink unchanged and add only a temporary ERR
trap to the exact versioned script, then restore and hash-verify the immutable
file immediately.

The precision ERR trap identified the exact command: `test ! -L "$artifact"`
in the bootstrap-artifact loop. When invoked through the production managed
builder link, `script_dir` is the public link directory, so both managed guest
init paths are themselves symlinks and correctly rejected. The resolved-path
wrapper instead selected regular files inside the immutable generation. The
builder must derive its generation directory from Bash's already-open script
descriptor (`/proc/$$/fd/255`), not re-resolve the mutable public activation
path and not relax the no-symlink artifact rule. The diagnostic asset was
restored to its exact source hash and deployment inspection passed.

The descriptor-pinned builder correction now passes shell syntax and the full
62-case OCI/builder suite 100 consecutive times. The regression both requires
the production builder to derive `script_dir` from its open fd 255 and executes
a real generation-file/public-symlink probe through both the shebang and
explicit `bash` paths, proving each resolves the immutable generation. The
strict no-symlink bootstrap-artifact check remains unchanged. Full repository
gates are next.

The subsequent full local gate passed: repository-wide Go race tests and vet,
documentation/link/schema/evidence checks, all OCI/bind/bootstrap/rootfs/
storage/mount/root/image suites, release/binary/deployment/ledger/capture/
containerd checks, final evidence audit, and `git diff --check`. The expected
locally permission-gated socket subcase remains assigned to the live guest.
Generated Python cache was removed. Commit and exact-source live redeployment
remain pending.

The generation-pinning correction is committed as
`21f772c393ca32be28502ce20730343cdb51ea81`. Its source-only archive hashes to
`624c29c459ae40e08d7a016a4ac0f7ace4cada8929bbfce8974eb0fe9b24cffe`.
Guest transfer, independent verification, full revision-stamped rebuild,
deployment-generation activation, and workload rerun remain unclaimed.

The recreated guest independently matched the full archive hash and rebuilt
the revision-stamped release. Identities are release manifest `4df065cd…`, shim
`ca72d720…`, mkruntimed `676131be…`, mknetd `9cf6f6de…`, agent `d244ef6c…`, and
gzip-valid dependent initramfs `63c6ae5d…`. These precede activation; no live
behavior is inferred yet.

Exact release `0.1.0-dev-21f772c393ca32be28502ce20730343cdb51ea81`,
manifest `6bebfc81…`, and deployment generation `ca7bc745…` are active.
Bootstrap validation passed after activation, and mkruntimed/mknetd PIDs
`14981`/`14964` remained stable for 35 seconds; the daemon reports the exact
revision. Both the predecessor and new immutable deployment are retained for
rollback. The ordinary live suite is the next boundary.

The `21f772c` suite passed the formerly failing builder and approved-manifest
boundaries and progressed through child launch. It then failed with
`CNI TUN is absent: stat /sys/class/net/mktun0: no such file or directory`.
The cleanup trap ran. This is a new network attachment/readiness boundary,
positive evidence for generation pinning but not a workload pass. Immediate
host/child/network inventories and mknetd/shim journals must establish whether
cleanup is complete and why the expected TUN was not visible.

The immediate audit is clean: containerd has no task or container, Multikernel
has no child, no `mkv*` link or named network namespace remains, mknetd has no
endpoint state, rootfs records are empty, and storage has no active state.
mknetd's pre-attach `CHECK` had succeeded by running `ip` inside the endpoint
namespace, including a link check for `mktun0`. `OpenTUN` then entered that
same namespace but tested `/sys/class/net/mktun0`; sysfs belongs to the host
mount namespace and therefore does not provide a reliable namespace-local
interface lookup after network-only `setns`. This explains the apparently
contradictory observations and identifies the check, rather than CNI creation,
as the failed boundary. A namespace-local socket ioctl will replace the sysfs
lookup before the next live run.

A bounded live probe directly reproduced the distinction. In a temporary
namespace, `ip -o link show dev mktun0` succeeded both through `ip netns exec`
and through network-only `nsenter`, while that same `nsenter` reported
`/sys/class/net/mktun0` absent. The trap removed the namespace and the final
inventory confirmed it absent. This converts the diagnosis from inference to
observation without leaving guest state.

The replacement primitive was then exercised on the guest before deployment:
an `AF_UNIX` datagram descriptor plus `SIOCGIFFLAGS`, created after entering a
second temporary namespace, successfully returned `mktun0`. The namespace was
again absent after cleanup. Locally, the focused network race test and vet pass
with this implementation; using `AF_UNIX` also avoids the local sandbox's
denial of an otherwise unnecessary IPv4 socket.

The same containerd journal records a separate compatibility warning:
containerd 2.2.2 probes the runtime binary with `-info`, while the shim rejects
that flag. The workload path still launches the shim, so this did not cause the
TUN failure, but it remains a current-host integration defect and must be
closed before claiming complete qualification.

The shim now handles exactly one `-info` argument before the vendored 1.7 flag
parser, following containerd 2.2's documented binary protobuf contract. It
bounds stdin to one MiB, rejects malformed option `Any` messages, echoes valid
options in `RuntimeInfo`, and reports the revision-stamped runtime identity.
It deliberately advertises no OCI feature document until each such field is
independently substantiated. Focused race tests cover empty/exact-option input,
malformed and oversized rejection; network and shim race tests plus focused vet
and `git diff --check` pass.

The subsequent complete local gate passed on 2026-09-26: repository-wide Go
race tests and vet, documentation/link/schema/evidence checks, all OCI, bind,
bootstrap, rootfs, storage, mount, image, release, deployment, ledger,
containerd, and final-evidence suites, followed by `git diff --check`. The
known locally permission-gated socket subcase remains assigned to the guest.
An immutable commit and exact-source guest deployment are the next boundaries.

The two corrections and their contemporaneous findings are immutable at
`cc56f9a238bbab800fdd0c8c76c091d1ba327eab`. Its source-only archive hashes
to `0b963bb9a3c200b50612831ef7e681a00600c0cc90edb20fb1f21ce7e3969b0e` and
contains neither Git metadata nor the pre-existing untracked evidence tree.
Guest transfer, independent digest verification, rebuild, activation, and live
qualification remain separate unclaimed boundaries.

The guest independently matched the archive digest, extracted it into private
root-owned `/root/mklinux-src-cc56f9a-20260926`, verified that every extracted
object is root-owned and that excluded trees remain absent, then completed the
full revision-stamped build. Exact identities are release manifest
`ee93424a…`, shim `8bd0608d…`, mkruntimed `d2dbc17b…`, mknetd `562dc51e…`, and
mk-agent `f7342e65…`. Installation and activation remain unclaimed.

Binary installation then activated exact release
`0.1.0-dev-cc56f9a238bbab800fdd0c8c76c091d1ba327eab`; manager inspection found
all command links managed, and containerd 2.2.2's own `ctr plugins
inspect-runtime` decoded the repaired response with the exact version/revision,
null options, and deliberately null feature/annotation fields. The following
combined deployment/restart command did not reach the guest because a nested
local quote terminated the `--command` argument; the local shell reported a
tail fragment as an unknown command. Therefore no deployment or restart claim
is made from that attempt.

The safely quoted retry reused the identical active deployment generation
`ca7bc745…` because its support inputs were unchanged, and inspection verified
every managed link. Only after empty containerd and Docker inventories were
confirmed, mkruntimed, mknetd, and containerd were restarted. Those services
and Docker were active immediately and five seconds later; both runtime
daemons report exact `cc56f9a…` revisions. Containerd still selects `runc` by
default, registers Multikernel separately, decodes its runtime info after the
restart, and has zero new `failed to load runtime info` journal records. Live
workload qualification is the next boundary.

The exact ordinary-user suite passed preflight, builder, child launch, and the
formerly failing TUN lookup, then stopped at the next boundary:
`fork/exec /usr/local/libexec/multikernel/mkvsock-relay: no such file or
directory`. The suite cleanup trap ran. This is positive live proof for the
network-namespace correction, but not a workload pass; resource cleanup and
the missing host relay artifact must be audited before retry.

The audit is clean: no containerd/Docker workload, child, host `mkv*` link,
named namespace, endpoint state, rootfs record, or active storage state remains;
all four services are active. The relay was not absent from the qualified host:
root-owned static `/opt/mkruntime/bin/mkvsock-relay` hashes to `293ff1ea…`,
exactly matching the approved kernel manifest's relay path and digest. The shim
instead hard-coded an unrelated `/usr/local/libexec/multikernel/...` default.
This is a path-contract defect. The default must select the manifest-pinned
`/opt` artifact before another immutable build; copying a second untracked
binary would conceal rather than fix the mismatch.

The shim default now names `/opt/mkruntime/bin/mkvsock-relay`; `MK_RELAY`
remains an explicit override. A focused regression proves both the approved
default and override, and the complete shim race suite, focused vet, and clean
diff check pass. Full repository qualification remains pending before commit.

The subsequent full local gate passed on 2026-09-26: every Go package under
the race detector, repository-wide vet, the complete documentation/schema/
evidence and runtime boundary matrix, final evidence audit, and diff checking.
The expected local socket-permission skip remains delegated to the privileged
guest. Commit and exact live redeployment remain separate.

The relay-path correction is immutable at
`f89dd00b804bdac4ac839baa8736d036f17aca65`. Its source-only archive hashes
to `77d8fc3231275cd7418934dfc15c8222b6f7d9daab1e6c88773faa7483c2075a` and
again excludes Git metadata and the untouched untracked evidence tree. Guest
verification, rebuild, activation, and live retry remain unclaimed.

The guest independently matched the new archive digest, extracted an all-root-
owned private tree with excluded metadata still absent, and completed the full
revision-stamped build. Exact hashes are release manifest `ca4c3c08…`, shim
`45d7f740…`, mkruntimed `2fcd2b3a…`, mknetd `58fd7513…`, and agent
`db46bac6…`. These are build identities only; installation is next.

The binary manager activated exact release `0.1.0-dev-f89dd00…` on an empty
host and verified every link. Both restarted daemons and containerd/Docker
were active immediately and after five seconds, all installed runtime binaries
report `f89dd00…`, native runtime inspection succeeds, and the approved relay
hash still exactly matches the kernel manifest. systemd warned that the linked
unit sources had changed on disk and requested `daemon-reload`; that reload
must be completed before the live rerun even though service health is stable.

The requested `daemon-reload` completed; all four services remained active
immediately and three seconds later, with runtime daemon PIDs `22235` and
`22217`. The exact live suite can now run without an unresolved activation
warning.

The exact `f89dd00` suite no longer fails to execute the relay. It progresses
through child, network, and relay setup, then after the agent-connection
interval the shim closes and ctr reports `ttrpc: closed`; the cleanup trap
runs. This proves the approved-relay path correction live, but exposes the
next agent-readiness or shim-liveness boundary. Cause and cleanup completeness
remain unclaimed until resource inventories and journals are collected.

The journal provides an exact cause: while retrying a not-yet-published relay
socket, `captureRelaySocket` converted `(*unixsocket.Path)(nil)` plus
`os.ErrNotExist` into a non-nil interface and stored it. After the child halted
before agent readiness, timeout cleanup called `Remove` on that typed nil and
panicked at `main.go:1985`; containerd then could not complete dead-shim delete
within five seconds. Unlike earlier failures, cleanup is incomplete: the child,
veth, named namespace, relay process, and storage image remain even though both
client inventories are empty. The fix must assign relay ownership only after a
successful capture in both fresh and recovery paths, then explicitly reconcile
this preserved failure before rerunning.

The preserved recovery record binds sandbox `mk-mk-proof-ctr-592a31dcfc440cd2`,
generation `0ba4a966…`, network generation `66d25609…`, task/storage digest
`0a6676ec…`, and bundle inode. mkruntimed still reports it `RUNNING` with active
storage, while relay PID `22955` has the exact approved executable and matching
generation-qualified command line. This identity evidence is sufficient for a
targeted relay termination followed by the shim's authenticated recovery
cleanup; broad namespace/link deletion will not be used.

Exact relay identity revalidation passed and PID `22955` terminated. The first
manual cleanup invocation then failed before mutation because it ran from the
ordinary user's home directory; the shim correctly rejected that mode-0750,
UID-1001 directory as its root caller's bundle identity. The authenticated
retry must execute with the preserved bundle as cwd, matching containerd's
normal invocation contract. No resource-cleanup success is inferred yet.

Running from the correct bundle exposed a second recovery defect before cleanup:
the recovery dial assigned a failed `(*agent.Client)(nil)` directly into the
`agentClient` interface, and its error defer called `Close` on that typed nil.
More fundamentally, containerd's `delete` action should not attempt full guest
recovery before calling the already authenticated `Cleanup` method; an
unreachable guest otherwise makes dead-shim cleanup unreachable. Recovery now
assigns the client only after a successful dial, and exact terminal `delete`
invocations skip event replay/full recovery so `Cleanup` can use held durable
identity. Focused regressions cover typed-nil relay publication and strict
delete-action parsing; the full shim race suite and focused vet pass. The
preserved guest is intentionally not retried until an immutable build exists.

The full 2026-09-26 local gate now passes for these cleanup corrections: all Go
packages under race, repository-wide vet, every documentation/schema/evidence
and runtime boundary suite, final evidence audit, and diff checking. The known
local socket-permission skip remains assigned to the guest. Commit, exact
rebuild, and preserved-failure cleanup remain next.

The cleanup corrections are immutable at
`0214f9727baecc94e46c675666477832afdc3cb4`. Their source-only archive hashes
to `27481fec9baae7e83db1fb47845356467d3e73fb546f9ed29d02b0fdfd1ab4f7` and
excludes repository metadata and the untouched evidence tree. The live failure
remains preserved pending exact guest verification/build/activation.

Guest digest, exclusion, and all-root-owned extraction checks passed, followed
by the exact full build. The release manifest is `6c7fc927…`; shim
`6a19fffc…`, mkruntimed `b6ae56bb…`, mknetd `bffc2dca…`, and agent
`9356724f…` are the revision-stamped outputs. Activation and cleanup behavior
remain unclaimed.

The exact release activated and cleanup-only `delete` ran without either
typed-nil panic. It exceeded the former five-second boundary and removed the
child, veth, and namespace. During this operation mkruntimed began a restart
loop: rootfs startup recovery reported `bundle may not contain symlinks`.
Subsequent source inspection corrects the initial `work`-symlink inference:
the containerd bundle had already been removed, so `validateRequest`'s
`EvalSymlinks` call failed on an absent bundle and collapsed that error into
the misleading generic symlink message. `NewService` therefore cannot reach
reconciliation of the still identity-bound storage artifact after its owner
bundle disappears. The rootfs/storage artifact remains, so cleanup is partial
rather than clean.
The long SSH wrapper was locally interrupted only after process inspection
showed no remote cleanup process; no successful delete response is claimed.
The failing daemon must be stopped to halt the loop before durable-state
inspection.

The apparent reboot was then audited from GCE state, both boot journals, and
the serial console. GCE kept the instance continuously `RUNNING` with its
original September 25 start timestamp, but the guest changed from boot
`e22e4b51…` to `ce405359…`. The previous journal ends without an orderly
shutdown or panic. Serial output is more precise: cleanup's SSH session began
at 02:13:18 UTC, the managed veth/netns disappeared at 02:13:18.99, and UEFI
began a fresh boot at 02:13:19.13. No GCE stop/start, orderly shutdown, or
kernel panic separates those events. The reset is therefore attributable to
the live child-stop cleanup boundary, not routine VM recreation or cloud
maintenance; its exact stop primitive requires correction before another
cleanup retry.

Durable-state inspection then found lifecycle sequence 23 as an incomplete
`StopSandbox` intent, the sandbox snapshot in `STOPPING`, its exact export
generation still `ACTIVE`, and the post-reset Kerf instance absent. This means
rootfs recovery cannot independently discard an apparently owned image.
Recovery must first treat an absent exact backend during an interrupted stop
as irrevocable sandbox loss, quiesce and offline-check that generation, and
commit lifecycle `ABSENT`; only the resulting empty owner map may authorize
identity-bound rootfs storage removal.

The first implementation checkpoint passes race-enabled Kerf, lifecycle, and
rootfs suites. Kerf now observes the exact sysfs state before stopping:
`LOADED` is an idempotent success, only `RUNNING` executes non-force `kill`, and
other states fail closed. Inspection of pinned Kerf 0.2.0 confirms that
`--force` deliberately accepts `loaded` and issues the force-halt reboot
command; the adapter no longer invokes that unsafe path. Non-force Kerf accepts
only `active`, so a concurrent transition to `loaded` is rejected before its
syscall and accepted by the adapter's post-observation. An interrupted stop
with an absent backend now
performs `storage-stop` followed by `offline-check` and commits `ABSENT`.
Rootfs startup structurally validates durable requests without requiring their
expired live paths; reconciliation preserves an exact owner, but once unowned
it removes only the recorded storage directory by root/artifact identities and
does not invoke the pathname mount backend.

The subsequent full local gate passed on 2026-09-26: every Go package under
the race detector, repository-wide vet, all documentation and local links,
seven schemas with 22 cases, all 17 classified historical evidence manifests,
the 62-case OCI boundary suite, bind/bootstrap/rootfs/storage/image suites,
release and deployment lifecycle, resource-ledger and capture checks,
containerd configuration, final evidence audit, and `git diff --check`. The
known workstation-only socket-permission subcase remains assigned to the
privileged guest. Exact commit, guest build, durable recovery replay, and a
fresh live workload remain pending.

After removing force-halt entirely, the focused Kerf stop/transition group
passed 100 race-detector repetitions. The complete race suite, vet, full
documentation/evidence gate, and diff check then passed again; no local claim
depends on the earlier force-kill wording.

The verified recovery change is immutable at
`d0bff83845442883e8ebe9ef1e3b477c94defc83`. Its source-only archive hashes
to `1306898b584fa450937da448ad32e000e38dac750bbeda5927ac08a4965c5391`;
archive inspection found neither repository metadata nor the untouched
`evidence/runtime-20260907` tree. Guest digest verification, exact build,
activation, and recovery replay remain unclaimed.

The guest independently matched archive SHA-256 `1306898b…`, extracted it
under private root ownership, and confirmed that `.git` and the local untracked
evidence tree were absent. The exact full-revision build passed. Its release
manifest is `4fb83d19…`; shim `80bf3a28…`, mkruntimed `b1a0f516…`, mknetd
`a2dbf46d…`, and agent `60f711ec…` all carry revision `d0bff83…`. Activation
and preserved-state recovery remain unclaimed.

With mkruntimed inactive and containerd, Docker, and Multikernel instance
inventories empty, the binary manager installed and activated exact release
`0.1.0-dev-d0bff83845442883e8ebe9ef1e3b477c94defc83`. Inspection reports all
six command/CNI links managed, and both mkruntimed and the shim print the exact
revision. Preserved-state recovery has not yet been started or claimed.

The first exact-daemon recovery start advanced beyond rootfs construction but
failed in lifecycle's storage reconciliation after roughly six seconds:
`restart durable storage export: storage image digest differs from prepared
identity`. Systemd began a restart loop; it was stopped after four retries.
Lifecycle remains sequence 23/`STOPPING`, rootfs remains `PREPARED`, and the
recorded storage directory and image remain present. No recovery mutation or
service-health success is claimed. This exposes a separate contract error:
the prepared-image digest is being required after a writable guest session,
although ext4 runtime writes necessarily change whole-image bytes. Recovery
must authenticate the durable inode/generation and validate the filesystem at
the proper quiesced boundary rather than compare it to its pre-run digest.

The storage contract is now phase-specific. Provisioning and interrupted
`PREPARING` recovery still require the pristine digest. Restart of an `ACTIVE`
export instead requires the same recorded device/inode and stable pathname,
full size/allocation, caller ownership, clean ext4 state, UUID, inode capacity,
and block capacity; it does not compare mutable bytes with the pristine hash.
A changed-byte image passes only current inspection, the pristine inspection
still rejects its digest, and a wrong UUID remains rejected. This boundary,
active restart selection, and inode-replacement rejection passed 100
race-detector repetitions. Full-tree verification remains pending.

The complete repository race suite, vet, documentation/schema/evidence and
runtime-boundary gate, deployment/final audits, and diff check then passed.
Exact commit and live recovery replay remain next.

The phase-specific storage fix is immutable at
`265196dac56713758c1c0a8a24e5a4f21d40784e`. Its source-only archive SHA-256
is `39a9ca5d5682b17a758faeb5cb0e2610560d6e656926b7284d11f7e1dfe61406`;
archive inspection again excludes repository metadata and the untouched local
evidence tree. Guest verification, build, activation, and replay are unclaimed.

The guest matched archive digest `39a9ca5d…`, verified all-root ownership and
both exclusions, and completed the exact build. Release manifest is
`b939e8da…`; shim `cce5baa0…`, mkruntimed `d9f57820…`, mknetd `67ec4807…`, and
agent `84164eb0…` carry revision `265196d…`. Activation and replay remain open.

With the daemon stopped and all external inventories still empty, the binary
manager activated exact release `0.1.0-dev-265196d…`; daemon and shim version
output match the full revision. Preserved-state replay is the next unclaimed
operation.

The first orchestration attempt to start and inspect that replay did not reach
GCE: the local cloud-command approval layer rejected it for account usage
limits before execution. No guest mutation or runtime result is inferred from
that failed tool call. The operator subsequently renewed explicit permission
to transfer source and run the live qualification suite, so replay may be
retried with separately visible checkpoints.

The renewed replay reached the disposable VM on 2026-09-26. Starting the exact
`265196d…` service returned durable initial health after a 15-second observation:
`mkruntimed` was `active`, main PID `35224`, `NRestarts=0`, `Result=success`, and
`ExecMainStatus=0`. The journal records the start at 14:13:26 local time and a
successful runtime-storage mount validation; the module-load unit reported only
that the already-loaded transport module existed. This proves stable process
startup, but not yet completion of the interrupted sequence-23 stop recovery;
the durable state and all external inventories are the next checkpoint.

Direct durable-state inspection then confirms that recovery completed the
interrupted cleanup. The guest boot ID remained
`ce405359-2f14-4655-9120-277e292af6da`, daemon PID `35224` was unchanged across
five seconds, and lifecycle `sandboxes` is empty. Existing journal sequence 23
now has `cleanup-stop-0ba4a966…` recorded as `ABSENT`; recovery completed that
intent rather than allocating a new sequence. Exact export generation
`33a8740c…` is `RELEASED` at 14:13:26 with
`e2fsck-clean-sha256:a37a7dad…`, and the rootfs record map is empty. Host-side
residue and external inventories remain a separate, still-unclaimed check.

The first residue pass found the exact task storage directory absent,
containerd task and container inventories empty, Docker's container inventory
empty, no named network namespace, and no link matching the interrupted task.
Only the expected `mkruntimed` process was displayed. This boot has no
`/sys/kernel/multikernel/instances` directory, so that observation is recorded
literally rather than described as an empty directory. The attempted
`pgrep mk-storage-server` check was inconclusive because Linux process names
are limited to 15 characters; a full-command-line process check must replace
it before storage-server absence is claimed.

The full-command-line retry matched only `mkruntimed` because its configuration
arguments contain the `mkvsock-nbd` server pathname; no separate child was
displayed and nothing listened on port 4061. `mk_transport` is loaded, daemon
health remains active/running with PID `35224`, zero restarts, and no journal
warnings since startup. Sysfs discovery also corrects the earlier probe: this
host exposes Multikernel beneath `/sys/fs/multikernel`, not
`/sys/kernel/multikernel`. Exact executable-name inspection and enumeration of
that real sysfs root remain before the residue checkpoint is closed.

The final residue probe closes that checkpoint. Both exact `comm` matching and
`/proc/*/exe` inspection found no `mkvsock-nbd` process. The correct
`/sys/fs/multikernel` tree contains its control files plus empty `instances`
and `overlays` directories; there is no child instance. Combined with the
absent task directory, empty container inventories, absent listener and network
objects, this directly substantiates complete recovery of the interrupted
sequence-23 cleanup on the unchanged boot.

Fresh-suite artifact preflight found a deliberate remaining boundary: the
active kernel manifest still pins agent `d244ef6c…` and initramfs `63c6ae5d…`,
whereas the exact `265196d…` guest build produced agent `84164eb0…`. The exact
source archive is still present at its verified `39a9ca5d…` digest and its
private root-owned extraction remains available. No workload is run against
this mixed revision; the exact agent, dependent initramfs, and private manifest
must be rebuilt and activated coherently first.

The first candidate-staging command built a gzip-valid exact-agent initramfs
at digest `c3d2b9f0…`, then stopped before manifest creation or validation. A
nested quoting error exposed awk's `$1` to the remote shell under `set -u`,
which reported an unbound variable. The release-specific staged agent and
initramfs exist, but the active manifest and running daemon were not changed;
candidate validation must resume with shell-safe digest extraction.

The digest-extraction retry also stopped before writing the candidate manifest.
It over-escaped jq's `$ap` named variable, so the remote `set -u` shell tried
to expand `ap` and rejected it as unbound. The active manifest and service
again remained untouched; the next retry uses exactly one remote-shell escape
for each jq variable.

Candidate validation then passed without changing active state. The staged
release-specific artifacts are exact agent `84164eb0…`, gzip-valid initramfs
`c3d2b9f0…`, and candidate manifest `fce23175…`; the bootstrap validator
resolved the pinned kernel, relay, transport module, compatibility, and feature
set successfully. The active manifest remained `6bebfc81…`, while daemon PID
`35224` stayed active with zero restarts. Coordinated activation remains next.

Coherent activation then passed on an empty host. Containerd, Docker, and child
inventories were checked empty before the change; the candidate was copied and
validated in the target directory before the daemon stopped. The final
same-filesystem manifest replacement validates at `fce23175…` and selects exact
agent `84164eb0…` plus initramfs `c3d2b9f0…`. `mkruntimed` restarted as PID
`37028` and remained active/running after ten seconds with zero restarts and
successful status. Boot ID `ce405359…` did not change. The qualified-host
preflight and fresh workload suite remain separate next checkpoints.

The exact-revision read-only host preflight passed. `mk-host-check`,
`mkruntimed`, and the shim all report full revision `265196d…`; mkruntimed,
mknetd, containerd, and Docker are active. The report has `qualified=true`,
Kerf 0.2.0, all 16 CPUs online, the requested APIC 8-15/16 GiB dry-run
`ready`, Secure Boot disabled, lockdown inactive, and no configured pool,
instance, stale resource, or finding. This qualifies the empty host; the basic
ctr/Docker workload suite is still unclaimed.

The exact basic-suite runner hash was `3eaa058b…` and passed all service,
mount, device, empty-child, and image preconditions. Its first `ctr run`
connected containerd to the Multikernel shim at 14:22:54, but returned
`timed out connecting to child agent` after the shim disconnected at 14:24:39.
The suite cleanup trap ran. A concurrent inventory then showed no ctr task or
container and no Multikernel child; daemon PID `37028` remained active with
zero restarts. This is a fail-clean agent-readiness failure, not a basic-suite
pass. Durable, shim, kernel, and serial evidence must be collected before
assigning cause.

The next evidence narrows the failure without yet naming its guest error.
Generation `4067a17f…` durably reached CREATED, LOADED, and RUNNING; the kernel
created instance 40, assigned CPUs 8 and 10 plus 3 GiB, loaded the exact
kernel/initramfs, and marked the child active at 14:23:52. The child halted at
14:24:00, 39 seconds before the shim disconnected and well before the client
reported its agent timeout. Cleanup completed through sequence 33: live
sandboxes, rootfs records, active exports, bundle paths, and runtime storage
paths are all empty. The fault is therefore early guest bootstrap/readiness,
not allocation or cleanup; the console or relay error remains to be recovered.

The first console-capture repeat did not reproduce that child boundary. It
never created an instance: after preflight, the shim reported
`UNAVAILABLE: read unix @->/proc/self/fd/7/mkruntimed.sock: i/o timeout`, and
the console poll truthfully recorded `CONSOLE_ATTACH_MISSED`. This is a distinct
post-cleanup daemon-availability finding, not guest-console evidence and not a
second agent timeout. Daemon/socket/process state must be inspected before any
restart or further replay.

Direct inspection does not support calling the daemon dead or wedged. PID
`37028` is active/running with zero restarts, 13 tasks, low current memory, a
live listening `mkruntimed.sock`, no worker subprocess, and no service-journal
error. Durable state is clean at sequence 36. Peak service memory reached
2.5 GiB and accumulated CPU time increased to 1m51s, consistent with the
second create continuing bounded image construction until caller cancellation
and then rolling back. Sequences 34-36 and their result must distinguish a
client deadline from a server failure.

The journal supplies that distinction. Sequences 35-36 contain only the
create intent/completion for generation `4b2416e0…`; the durable result is
`ABORTED: create canceled by runtime shim`, with no load or start. Thus the
socket timeout was the caller deadline expiring during create, followed by a
successful server-side rollback. It is not listener failure. Build timing and
the shim's bounded request timeout must be understood before a console repeat.

Exact timestamps reveal a timeout-contract defect. The create intent began at
14:27:49.892 and durably committed CREATED at 14:28:19.286, a 29.394-second
backend operation. The shim's daemon client imposes a 30-second deadline, so
the successful response lost the delivery race and the subsequent cancellation
released the export. This is not merely an unlucky run: rootfs preparation is
explicitly permitted up to ten minutes while using the same 30-second client.
Long rootfs preparation and lifecycle create therefore need distinct bounded
deadlines that encompass their documented server-side bounds; ordinary daemon
operations must retain their shorter failure detection.

The timeout correction is now focused and tested. `PrepareRootfs` uses an
11-minute copied client, covering its ten-minute builder cap; `CreateSandbox`
uses a 61-minute copied client, covering the host configuration's one-hour
backend maximum. The base client and every ordinary call retain 30 seconds,
and the request context can still cancel sooner. The first focused build caught
that the service stores an injectable `daemon.Caller`; the final helper retimes
only a concrete production client and preserves test fakes. The complete shim
package passed once under the race detector. Full repository gates remain.

The broad local gate then passed: all Go packages under the race detector,
repository-wide vet, documentation and links, 7 schemas/22 cases, all 17
classified historical evidence manifests, the 62-case OCI suite and namespace/
identity/cleanup boundaries, bind/bootstrap/rootfs/storage/image suites,
release/deployment/ledger/capture/containerd checks, final evidence audit, and
`git diff --check`. Only the known locally permission-gated socket subcase was
skipped for the privileged guest. Generated `scripts/__pycache__` was removed;
the pre-existing untracked `evidence/runtime-20260907/` remains untouched.

Commit `cf88beb1c7d61769316fecf7bd33c1a188a5b4a6` freezes the timeout-contract
correction and the preceding live findings. Its source-only archive SHA-256 is
`01044ee9885ec5b91aa6a600f28ab6b95ee90434fc3953777d6215d665bfdf8c`;
archive inspection contains neither Git metadata nor the untouched untracked
evidence tree. Guest digest verification, exact shim build, activation, and
live replay remain unclaimed.

The guest matched archive digest `01044ee9…`, extracted it into a private
root-owned tree, and rechecked the `.git` and untracked-evidence exclusions.
The first build wrapper then stopped before compilation because it attempted to
preserve a `runtime/release-manifest.json` that this clean archive does not
contain. No installed component changed. The exact build can proceed directly
with explicit revision `cf88beb…`.

The exact guest build then succeeded. Release manifest is `05b86f57…`; shim
`bf79230d…`, daemon `c3de5ae2…`, mknetd `fe31d09d…`, and agent `056402d6…`
all report full revision `cf88beb…`. This corrects an earlier planning
assumption: unchanged agent source still produces a new binary because the
revision stamp changed, so coherent qualification does require rebuilding its
dependent initramfs and private kernel manifest before activation.

The coherent `cf88beb…` artifact candidate passed independent bootstrap
validation. Exact agent is `056402d6…`, gzip-valid initramfs `52ce2b0a…`, and
private manifest `a3656ae8…`; the pinned kernel, transport module, relay,
compatibility, and declared feature set all resolve. Installation and active
service identity remain unclaimed.

Coordinated empty-host activation passed. Immutable release
`0.1.0-dev-cf88beb…` is active, the final manifest validates at `a3656ae8…`,
and installed shim hash is `bf79230d…`. After ten seconds mkruntimed PID
`40581` and mknetd PID `40562` were active with zero restarts and successful
status; containerd and Docker remained active. Daemon, network daemon, and shim
all report full revision `cf88beb…`, while boot ID `ce405359…` is unchanged.
Workload behavior remains a separate checkpoint.

The exact `cf88beb…` live run proved the longer daemon deadlines and captured
the actual guest failure. Runner `3eaa058b…` created generation `6cba5f75…`,
the console attached while it was active, the exact kernel booted with two CPUs
and 3 GiB, NBD attached, ext4 mounted, and
`MK_STORAGE_BOOTSTRAP_READY` printed. Then `/init` line 5 reported
`mountpoint: not found`; its fallback `mount` of devtmpfs on the already busy
`/dev` failed, `set -e` exited PID 1 with 255, and the child panicked at 7.835s.
The client consequently reported the expected agent timeout and cleanup ran.
The root cause is bootstrap construction: it installs `/bin/busybox` but does
not provide a `mountpoint` applet link while `runtime-mediated-init` invokes the
bare command. The bootstrap must use explicit BusyBox applets (with archive
tests) rather than suppress the mount failure.

The agent init now detects inherited mounts by reading `/proc/mounts` through
the controlled `/bin/busybox grep` applet and invokes `/bin/busybox mount` only
when a fixed mountpoint is absent. It no longer depends on the optional
`mountpoint` applet and preserves the mediated bootstrap's moved `/dev`,
`/proc`, and `/sys`. Regression checks forbid reintroducing `mountpoint`, bind
the detection and devtmpfs behavior to explicit BusyBox paths, and confirm the
runtime storage builder installs this exact init as `/init`. Shell syntax, the
62-case OCI/builder suite, and `git diff --check` pass; broad gates remain.

The subsequent full local gate passed: every Go package under the race
detector, repository-wide vet, the complete documentation/link/schema/evidence
chain, OCI/bind/bootstrap/rootfs/storage/image/release/deployment/ledger/
capture/containerd checks, final evidence audit, and diff checking. The known
local socket-permission skip remains assigned to the privileged guest.
Generated Python cache was removed and the user-owned untracked evidence tree
was not changed.

Commit `a209f7cb99863a0902afeff19806dbfe2d28d967` freezes the guest-init fix
and its direct console evidence. The source-only archive hashes to
`5e3408f772fe1f4f7b2e0f4058bdff0f393685f9b114c070fc2ab9979a42103a`
and excludes the untracked evidence tree. Because `mk-agent-init` is both
embedded in the runtime storage image and owned by the managed deployment,
qualification requires exact binaries, revision-stamped agent/initramfs/
manifest, and a new deployment generation before replay.

The guest independently matched archive `5e3408f7…`, rechecked root ownership
and source-only exclusions, and completed the exact build. Release manifest is
`18471bfc…`; shim `b24a4acd…`, daemon `4ec33750…`, mknetd `086bdbfe…`, and
agent `01c69ee3…` all carry revision `a209f7c…`. These are build identities
only; artifact/deployment staging and activation remain.

The complete exact stack is now active on an empty host: binary release
`0.1.0-dev-a209f7c…`, deployment `a8a777e3…`, and kernel manifest
`6c0f897e…`. All managed deployment links verify, and installed agent init
`edc9284c…` exactly matches source with no `mountpoint` dependency. After ten
seconds all four services were active; mkruntimed PID `42862` and mknetd PID
`42843` had zero restarts and successful status, every component reported the
full revision, and boot ID `ce405359…` was unchanged. The workload rerun remains
unclaimed.

The `a209f7c…` rerun clears the prior panic but is not yet a pass. The child
remained active and ctr reached task state CREATED, then `ctr run` returned
`INTERNAL: agent operation failed`. The harness cleanup started, but a
concurrent inventory still found the child active and ctr task/container
present while both daemons remained healthy. The console collector is still
attached, so cleanup is incomplete at this checkpoint. Live guest markers and
the exact agent error must be collected before intervening.

The live console proves the mount fix itself: storage mounted, bootstrap became
ready, and `MK_AGENT_START` appeared without a panic. The first host-to-agent
operation then failed; cleanup closed the NBD export at guest uptime 24.598s,
after which ext4 logged expected disconnect/write errors. Containerd's two
delete attempts identify the retained-cleanup boundary as
`close guest network: INTERNAL: agent operation failed`. The exact shim,
relay, child, and recovery files remain for authenticated diagnosis; they must
not be broadly killed before the recovery record and agent protocol are
inspected.

The console collector was terminated only after its PID and full command line
were revalidated; the diagnostic wrapper then returned `RUN_RC=1`. Retained
recovery identity is generation `c247ea40…`, network generation `a2dfb03f…`,
storage generation `99c68361…`, bundle inode 6530, worker PID 43122, relay PID
43722, and relay-socket inode 6615. Lifecycle still records RUNNING. The
authenticated dead-worker cleanup path can avoid the failing guest RPC, but
the exact shim process group/executable/cwd must be verified before it is used.

Supervisor/worker/relay identity checks passed, and only those exact processes
were terminated. Containerd invoked cleanup-only delete, removed the task and
child, and no target process remains. Dead-shim cleanup nevertheless returned
`stop recovered network: persist network counters: STALE_COUNTER: network
counters cannot decrease`; container metadata remains. The recovery snapshot
held zero counters while mknetd had newer live counters, so reconstruction
attempted to report zero as READY. Endpoint/rootfs/storage state must be
inspected before deciding whether this is only a cleanup-reporting defect.

The complete audit shows that it is a reporting/reconstruction defect rather
than leaked runtime ownership. At lifecycle sequence 55 there are no sandboxes,
rootfs records, active exports, mknetd endpoints, namespaces, links, runtime
firewall rules, or storage directories. Only containerd's taskless container
metadata and bundle remain because its dead-shim command returned nonzero.
Normal container metadata removal is now safe; counter recovery still needs a
source correction before another live run.

Normal `ctr containers rm` cleared the remaining metadata and left all four
services active, but the failed task bundle directory remained because the
earlier dead-shim cleanup returned nonzero. It will not be recursively deleted:
after revalidating bundle inode 6530 and absent runtime owners, the exact tree
will be moved to a named `/tmp` quarantine so its diagnostics remain
recoverable and the deterministic task path is clean.

The exact source inode and empty-owner preconditions were revalidated, and the
bundle contents now reside at
`/tmp/mk-proof-ctr-a209f7c-c247ea40.bundle`; the deterministic runtime path is
absent. Correction: `/tmp` is a different filesystem, so GNU `mv` copied then
removed the original tree and the quarantined directory has inode 4252, not
6530. The diagnostic content is recoverable, but the original directory inode
was not preserved.

The complete local gate then passed: `go test -race -count=1 ./...`, full
`go vet ./...`, documentation structure/links, 7 schemas with 22 cases, all 17
classified historical evidence manifests, the 55-case OCI boundary suite,
bind materialization, bootstrap, 19 rootfs-build tests, 10 storage-build tests,
7 image-architecture tests, 3 release tests, binary/deployment lifecycle,
resource ledger, capture, containerd configuration, final evidence audit, and
`git diff --check`. The already documented local socket-permission subcase
remained skipped and still requires the privileged VM run.

The validated loader/deployment compatibility checkpoint was committed as
`0792c9b` (`runtime: load managed host config safely`). A new source-only
`git archive` of that exact commit hashes to
`263ea86901721f40fbcf946512596694ac43207a454a1c0a748a8a3a35fa7662`
and was transferred to the disposable guest. The pre-existing untracked local
evidence directory remained untouched.

The guest verified and unpacked the `0792c9b` archive, then rebuilt all
revision-stamped components. The new release manifest hashes to `1529d73b…`;
key component hashes are `b84ffb3a…` for mkruntimed, `2a832f17…` for the shim,
`41571385…` for mknetd, and `730826b0…` for mk-agent. The regenerated current-
agent initramfs passed `gzip -t` and hashes to `d6af7590…`. The independently
rebuilt C helper hashes and copied exact transport-module hash remained
`a0259098…`, `293ff1ea…`, and `bef1b888…` as expected. A coherent artifact and
manifest upgrade remains necessary before restarting mkruntimed.

The coordinated upgrade stopped the failing service, activated immutable
release `0.1.0-dev-0792c9bc…`, atomically replaced the installed agent,
initramfs, and matching mode-0600 kernel manifest, and passed bootstrap
validation against manifest SHA-256 `5791ed6c…`. Installed mkruntimed, agent,
and initramfs hashes matched. The command then stopped before service start
because its ordinary-user `sha256sum` could not read the intentionally private
kernel manifest. This is an evidence-command privilege error, not an artifact
validation failure; mkruntimed remains stopped until the corrected privileged
hash and health check run.

The corrected privileged manifest hash matched `5791ed6c…`, and mkruntimed
then started successfully from revision `0792c9bc…`. Its main PID `15246`
remained unchanged across the three- and five-second health observations, and
the service remained `active`. It created private `/var/lib/mkruntime` and
`/srv/multikernel-storage/runtime` directories plus an empty private journal;
no child instance existed. The collected journal window necessarily includes
the earlier restart-loop failures, followed by the final 04:01:58 start with no
new host-config error. This is the first live proof that the deployment-shaped
config link is accepted by the corrected runtime.

The corrected revision's `mk-host-check` then returned `qualified=true` with
kernel `7.0.0-mk2-gce-lab`, Kerf `0.2.0`, CPUs `0-15`, 67,416,371,200 bytes of
memory, a ready contiguous 16-GB pool dry-run, active guest agent, and no
findings, stale resources, or instances. `mkruntimed`, `mknetd`, containerd,
Docker, and the Google guest agent were all active; ctr and Docker inventories
were empty. Installed shim/runtime/network/CNI versions and hashes matched
revision `0792c9bc…`. This is the clean qualified boundary immediately before
the live workload suites.

The basic current-revision live proof failed on its first ctr task creation,
before Docker workload creation. Image pulls completed, but the runtime
returned `FAILED_PRECONDITION: build child root: exit status 1: OCI
configuration rejected: [Errno 20] Not a directory: 'self'`. The script's
cleanup trap then ran. No G4/G5/G6 pass marker was emitted, so this is a live
implementation failure, not evidence of gate completion. Immediate leak/state
inspection and diagnosis of the descriptor-backed `/proc/self/fd` validation
path are required before retry.

Immediate post-failure inspection proved fail-clean behavior. All four runtime
services remained active; Multikernel child inventory, ctr containers/tasks,
Docker containers, runtime network links, MK firewall/NAT rules, durable
runtime journal entries, prepared rootfs files, and mknetd endpoint records
were empty. Only the expected private empty state directories remained. The
failure can therefore be investigated without first reclaiming a leaked guest
or endpoint.

Diagnosis found that `validate-runtime-oci.py` intentionally walked normal
absolute paths component-by-component with no-follow semantics, but did not
recognize the builder's deliberate inherited path
`/proc/self/fd/3/config.json`; it therefore rejected procfs component `self`.
The validator now admits only the exact
`/proc/self/fd/<number>/config.json` form, verifies the inherited descriptor is
a caller-owned directory not writable by group/other, and opens only
`config.json` relative to it with `O_NOFOLLOW` before applying the existing
bounded, caller-owned, single-link, stable-file checks. A subprocess test passes
a real inherited bundle descriptor and the focused 55-case OCI suite passes.
Full gates and live redeployment remain pending.

The complete local gate passed after this fix: full Go race suite, full vet,
documentation/evidence/schema checks, the OCI/bind/bootstrap matrices, all
rootfs/storage/image/release tests, binary/deployment/ledger/capture/containerd
checks, final evidence audit, and `git diff --check`. The expected locally
permission-gated socket subcase remains reserved for the disposable host.

The inherited-OCI-config remediation was committed as `7c94ab1`. Its
source-only archive hashes to
`fbeb4eb9fc46ff7b8860f29ee65687c0b4fc7dbb58a6fdca712dbbe3c924c8a4`
and was transferred to the guest. The next retry will rebuild every
revision-stamped binary and dependent agent initramfs, rather than mixing this
support-script revision with the preceding `0792c9b` release.

The guest's exact `7c94ab1` build completed. The release manifest hashes to
`631f4d29…`; key binaries are `7cdd068d…` (mkruntimed), `a34a5027…` (shim),
`72717e2f…` (mknetd), and `8ba5738b…` (agent). The corresponding initramfs is
gzip-valid and hashes to `47129dfc…`. Helper and transport identities remained
unchanged. These identities must now be installed with a matching private
kernel manifest and a deployment generation containing the corrected
validator.

The coordinated `7c94ab1` upgrade then passed its installation boundary. The
uploaded source archive (`fbeb4eb9…`) and private manifest (`f43ccc23…`) were
rechecked before mutation, and the candidate agent and initramfs re-matched
`8ba5738b…` and `47129dfc…`. With both empty runtime services stopped, the
binary manager atomically selected release
`0.1.0-dev-7c94ab1448dbabce2df59d2e1a20099b77b01802`; a fresh root-owned
extraction installed deployment generation
`c4c1677859455084c197a3a8c37e7cc4f0700b02d77739399dd57f1b037b0636`.
The agent, initramfs, and mode-0600 private manifest were staged and renamed
into place, the installed bootstrap validator accepted the complete manifest,
and both `mknetd` and `mkruntimed` remained active after a three-second health
check. Installed hashes exactly matched the candidates. This establishes a
coherent corrected host boundary; the basic live workload suite is the next
checkpoint and is not yet claimed.

The post-upgrade service PID was stable at `17732` across a five-second check,
and `mknetd`, `mkruntimed`, containerd, and Docker were active. An initially
unparameterized `mk-host-check` correctly reported the Kerf allocation probe as
unperformed; merely loading `runtime.env` does not supply this separate
read-only probe. Re-running with the pinned Kerf executable and the intended
APIC IDs 8-15 plus 16 GB returned `qualified=true`, `contiguous_allocation:
ready`, no configured pool or instances, and no findings. This distinction is
recorded so the unparameterized diagnostic is not mistaken for a host
regression.

The first basic workload retry reached the corrected inherited-descriptor path
but failed closed at the next OCI compatibility boundary before creating the
ctr task: `OCI configuration rejected: only the default deny-all device
resource contract is supported`. The suite exited 1 and its cleanup trap ran.
No basic live pass is claimed. The exact containerd-generated device cgroup
shape must now be compared with the validator's supported contract before any
source change.

The post-failure audit found no ctr tasks/containers, named Docker container,
Multikernel child, or `mkv*` link, so this rejection was fail-clean. A
containerd metadata-only diagnostic then exposed the exact standard device
resource list: deny all, followed by `rwm` allows for character devices 1:3,
1:8, 1:7, 5:0, 1:5, 1:9, 5:1, 136:any, and 5:2. The validator currently
accepts only the shorter deny-all-only form even though device resources are
deliberately omitted from the dedicated-child projection. Remediation should
admit only these two exact ordered default contracts and continue rejecting
custom device access.

The validator now accepts only the prior deny-all-only list or the exact
ordered containerd default list observed above. It does not normalize a set:
missing, reordered, duplicated, block-device, or otherwise custom rules remain
fatal, and all resource policy remains absent from the child projection. The
focused suite now has 59 semantic cases and passed once with explicit negative
coverage for missing, reordered, and custom rules. Repeated and full gates are
pending.

The expanded OCI suite then passed 100 consecutive repetitions. The subsequent
complete Go race suite, `go vet ./...`, documentation/link/schema/evidence
chain, all privileged-boundary simulation suites, release/deployment/ledger
tests, containerd config tests, final-evidence audit, and `git diff --check`
also passed. The locally permission-gated socket rejection remains reserved for
the disposable host. Exact-source commit, rebuild, redeployment, and live retry
remain pending.

The storage-mount remediation was committed as `1124739`. Its source-only
archive (`08e22eb1006de8c913184afa1aea5486a0b0c8ba9f04c88d6dd8929c2476ec13`)
and adapted non-secret runtime environment (`8d6184b8…`) were verified on the
guest. The full-revision build produced release manifest `fde70509…`, shim
`35ab7058…`, mkruntimed `1ad3d4be…`, mknetd `89091fb9…`, agent `f819e3cb…`,
and gzip-valid initramfs `7aae45ac…`. Managed activation and live tests remain
pending.

The device-default remediation was committed as `c409ad4` (`runtime: accept
exact containerd device defaults`). Its source-only archive hashes to
`53c972911b03c16174e97c2e1250fb88a3afc51b32a1778feb34d2aca4a50b86`;
the guest verified that digest and built binaries stamped with full revision
`c409ad4fe2efef9e5b7e6c2fe98a3a367414ce16`. The release manifest is
`ba5f2541…`, shim `43a38b7f…`, mkruntimed `0db79260…`, mknetd `9340d38e…`,
agent `6ac23778…`, and the gzip-valid dependent initramfs `6d382231…`.
Coherent installation and another live retry remain pending.

The coherent `c409ad4` upgrade passed. Empty ctr/task/Docker/child inventories
were checked first; all four candidate hashes were rechecked before services
stopped. Release
`0.1.0-dev-c409ad4fe2efef9e5b7e6c2fe98a3a367414ce16` and deployment
`d2c096937c91f7f06f1ca569e07f12ede2b34a3eb068f272cd10f2f5fb7accef`
became active, the agent/initramfs/manifest were atomically replaced, bootstrap
validation passed, and both managed services remained active after five
seconds. Installed hashes matched the candidates exactly. The next checkpoint
is the basic live suite; it is not yet claimed.

The exact `c409ad4` live retry still failed closed at the same resources check,
now with the updated `only exact default device resource contracts are
supported` message. Therefore the device list alone was insufficient to
characterize the actual resources object; another key or representation differs
in the task bundle. The suite exited 1 and no pass is claimed. Full resources
metadata must be inspected before revising the validator again.

A one-shot wrapper recorded only the live bundle's resources object and then
restored the managed builder link to its exact deployment target. The live
bundle contains the already recognized device list plus `cpu: {shares: 1024}`;
container metadata had omitted that default CPU object. CPU share 1024 is the
Linux default, so dropping precisely this value at the dedicated-child boundary
preserves default behavior. Any other share, quota, period, cpuset, or extra
resource field must remain rejected. The diagnostic task was removed and the
stable managed link was verified restored.

Validation now accepts resource objects containing an exact supported default
device list and, optionally, exactly `cpu: {shares: 1024}`. Resource keys are
otherwise closed. The focused suite expanded to 62 cases: the live ctr default
passes, while shares 512 and a quota added beside shares 1024 fail, as do the
existing device mutations. The focused run and `git diff --check` pass; broad
verification remains pending.

The 62-case suite passed 100 consecutive repetitions. Full Go race, vet,
documentation/schema/evidence, OCI/bind/bootstrap, rootfs/storage/image,
release/deployment/ledger/capture/containerd, final-evidence, and diff gates
then passed; only the expected locally permission-gated socket subcase was
skipped for the disposable host. The exact two diagnostic files were removed
after verifying the managed builder link and empty ctr/task inventories.
Commit, exact rebuild, redeployment, and live retry remain pending.

The refined resource-contract change was committed as `3598056`. Its uploaded
source archive hashes to `a4acedb41b51a243a842d8c0caa4ff000e3b61fdba83b4417e226bee82ab3352`
and the guest reverified it before extraction. The full-revision build produced
release manifest `a60f72a7…`, shim `04daf0b8…`, mkruntimed `40b42a65…`, mknetd
`d1f2ff6f…`, agent `da6d8e95…`, and gzip-valid dependent initramfs
`4f58a729…`. Installation and the live retry remain pending.

The exact `3598056` deployment passed the empty-inventory and hash prechecks,
then activated release
`0.1.0-dev-3598056becf7beb698dbdb3388c2e3268b44efdf` and support generation
`5df65a4b65c5b5bdcc9174098887d8e151998c9fc0f37a21f65b45670b6f7f0d`.
Bootstrap validation passed, installed agent/initramfs/manifest hashes matched,
and both services remained active after five seconds. The basic live retry is
the next unclaimed checkpoint.

The exact `3598056` basic retry advanced beyond the resource-contract message
but still failed before ctr task creation: `build child root: exit status 1`
with no forwarded builder stderr. The observed host boot ID was unexpectedly
`f9d00c5f-6376-4db5-94b7-e66e055748ac`, different from the earlier
`3b4d5c5d-9436-4bab-9c17-609a4dcd181d`; this reboot boundary must be audited
before interpreting the new failure. Exit status was 1 and no live pass is
claimed.

The deployment manager reused the prior verified runtime environment and host
configuration as immutable inputs, installed and selected deployment
`c9e5b67094f8f4aba5783ea5fcaac57489a62ec9da77507f7d0eb220717a24ce`,
and reports every managed link valid. The deployed OCI validator exactly
matches source at SHA-256 `5303e914…`. Service PIDs/restart counts remain
unchanged, so daemon reload/restart and the revision-specific kernel manifest
are still pending; this is selected filesystem state, not yet a coherent
running release.

Pre-staging identity checks show the active kernel manifest is a root-owned
mode-0600 regular file (not a symlink) in a root-owned mode-0755 directory;
the artifact parent is likewise root-owned mode 0755. A new revision directory
and same-directory manifest candidate can therefore be created and validated
without following an unmanaged selector.

Revision-specific agent `0ee464e9…` (mode 0755) and initramfs `a78778e2…`
(mode 0600) are now staged in root-only artifact directory `a72c5cb…`.
Strict bootstrap validation passes for the root-owned candidate manifest
`8a8322a4…`, resolving the exact kernel, module, relay, compatibility pins,
required config, and OCI feature set. The candidate has not replaced active
`gce-mk2.json`; a fresh process/resource preflight is required immediately
before coordinated activation.

The immediate activation preflight passes on unchanged boot
`e76edca2-d8d6-4800-9df8-f9ee9dccca8f`: default/moby task and container
inventories, Docker objects, children, runtime storage, active lifecycle/
network/rootfs state, runtime links/rules, and shim/relay/storage-server
processes are all empty. Coordinated stop, atomic manifest replacement,
daemon reload, and service start may now proceed without displacing work.

Coordinated activation succeeds on the same boot. The previous `444428a…`
manifest is retained as a root-owned rollback file; active manifest is
`8a8322a4…`, release manifest `55e4ea88…`, shim `f32e4a1c…`, binary selector
`a72c5cb…`, and support deployment selector `c9e5b670…`. The mount, mknetd,
mkruntimed, containerd, and Docker are active; new PIDs are 11328, 11347,
11357, and 11389 with zero restarts/status 0. Shim, daemon, and mknetd all
report exact revision `a72c5cb…`, and strict active bootstrap validation
passes. Exact-revision workload proof is now the next boundary.

The exact archived basic runner hashes to `808df3e0…`. On exact `a72c5cb…`,
the ctr task again reaches `RUNNING`, and Docker now clears the AppArmor/OOM
validation boundary with explicit `--network none --security-opt
apparmor=unconfined`. Docker then fails closed at the next concrete OCI
boundary: `unsupported linux field(s): seccomp, sysctl`, exiting 125 before
its task starts; the EXIT trap runs. This is not a suite pass. Independent
cleanup audit and exact live values/semantics of those two fields are required
before any remediation decision.

The first post-failure inventory command is invalid: nested quoting exposes
the jq `| length` filters to the remote shell, which interprets them as
pipelines/path fragments and aborts. It establishes no cleanup fact and makes
no intended mutation. The audit must be rerun from a transferred, syntax-
checked script rather than another dense inline command.

The syntax-checked audit proves cleanup is fully empty: default/moby tasks and
containers, Docker objects, child instances, runtime storage, active
lifecycle/network/rootfs records, `mkv*` links, Multikernel firewall rules,
and shim/relay/storage-server processes are all zero. mknetd PID 11328 and
mkruntimed PID 11347 remain active with zero restarts/status 0. The failed
Docker start therefore leaves no resource, but `seccomp`/`sysctl` remain an
open compatibility and security-design boundary.

Short-lived runc-backed Docker bundle inspection shows the exact request.
With explicit AppArmor unconfined but Docker's default seccomp, `linux.seccomp`
is a substantive deny-by-default profile (errno action plus a large allowlist),
so accepting and stripping it would silently remove a security control. Adding
explicit `--security-opt seccomp=unconfined` makes the field absent. In both
cases Docker still requests two sysctls:
`net.ipv4.ip_unprivileged_port_start="0"` and
`net.ipv4.ping_group_range="0 2147483647"`. Those change kernel behavior and
cannot be declared inert without comparing them to the child kernel and/or
implementing application. Both probe containers are removed. The immediate
next evidence is the exact clean-child default for these sysctls.

The exact-runtime child probe reports pristine kernel defaults
`ip_unprivileged_port_start=1024` and `ping_group_range="1 0"`; its normal
`ctr run --rm` cleanup then returns every audited inventory to zero while
daemon PIDs/restart counts remain stable. Docker's emitted values are therefore
not inert. A potentially truthful integration is to make seccomp unconfined
and both child-default sysctl values explicit in qualification, then accept and
strip only that exact map. Docker's generated OCI representation of those
explicit defaults must be observed before implementation.

The narrowed runc probe confirms Docker preserves explicit child-default
settings exactly as string map
`{"net.ipv4.ip_unprivileged_port_start":"1024",
"net.ipv4.ping_group_range":"1 0"}`, while explicit seccomp unconfined leaves
`linux.seccomp` absent. This permits a narrow fail-closed compatibility rule:
qualification explicitly requests all three opt-outs/defaults; validation may
accept and remove only that complete exact sysctl map, because the pristine
child has already proven identical behavior. Missing, extra, permissive,
malformed, or differently typed maps and every seccomp object remain rejected.

The implementation now admits only the complete exact child-default sysctl
map and omits it from the guest projection. Qualification explicitly supplies
unconfined seccomp and both child-default sysctls on every Docker create/run,
including the embedded PTY command. Focused validation passes 72 semantic
cases, covering Docker's permissive map plus partial, extra, typed, and
non-object rejections; Python compilation, both shell syntax checks, and
`git diff --check` also pass. Full local gates and an immutable revision remain
pending.

The reboot audit established an orderly stop at 04:34:44 UTC (including clean
unmount of the storage filesystem), followed by a new boot at 09:17:27 UTC;
there is no kernel-crash signature. The current host requalifies with guest
agent active, allocation ready, no pool/instances/stale resources, and no
findings. However `/dev/sdb` retained the correct serial, ext4 label, and UUID
but was not remounted at `/srv/multikernel-storage`; that pathname resolved to
the root disk and mkruntimed had created an empty `runtime` directory there.
This explains the changed failure boundary and exposes a reboot-safety gap:
the service and harness can start without the approved storage filesystem.
They must fail closed on the expected mounted filesystem identity before any
rootfs construction.

A new deployment-managed storage preflight now binds the mount to a canonical
root-owned by-id link, whole block device, exact byte size, udev short serial,
label and UUID symlinks, a single read-write ext4 mount record, mount/device
numbers, and separation from the root filesystem. The runtime environment
gains strict storage identity fields and mkruntimed executes this validator
before startup; unsafe path/token/size/UUID values are rejected by the
deployment manager. Focused parser, missing-input, service-wiring, and complete
deployment lifecycle tests pass. On the live unmounted host, the updated
validator reached the intended boundary and returned
`runtime storage path is not one distinct mountpoint` with status 1.

With inventories empty and mkruntimed stopped, the live disk's size, serial,
label, UUID, and unmounted state were rechecked. It was mounted through the
approved by-id path with `nodev,nosuid`; the new validator returned
`RUNTIME_STORAGE_MOUNT_VALID`, `findmnt` reported `/dev/sdb` as read-write ext4
at the exact path, and mkruntimed remained active after five seconds. This is
the qualified mounted-storage boundary for continued testing. Persistent boot
ordering remains documented through an exact-UUID fstab entry and the managed
pre-start validation, but that new deployment generation is not yet installed.

The new storage validator passed 100 repetitions, deployment lifecycle tests
passed, and all three live harnesses passed `bash -n`. A local
`systemd-analyze verify` attempt then stopped the aggregate command because the
developer workstation does not have the production `/usr/local/sbin/mkruntimed`
or `mknetd` paths installed. This is an environment limitation rather than a
unit parse error; the immutable deployment test remains the local wiring check,
and live systemd activation will be the authoritative unit check. The full
race/vet/docs chain had not run yet at this checkpoint.

The subsequent complete Go race suite, vet, documentation/link/schema/evidence
chain, 62-case OCI suite, bind/bootstrap/rootfs/storage/image checks, new mount
validator, release/deployment/ledger/capture/containerd checks, final-evidence
audit, and `git diff --check` all passed. The expected locally
permission-gated socket subcase remains for the disposable host. Exact commit,
managed deployment, negative service-start proof, and live workload retry
remain pending.

A fallback child initramfs was then rebuilt from the current agent, current
relay, exact transport module, current `guest/mk-agent-init`, and BusyBox; it
passed `gzip -t` and hashes to
`f8ce18d4cc018dd61611890a2188f8d70b3af9db6239b7dd5d309b4f8f6aae0a`.
The pinned `vmlinux` is a static x86-64 ELF with SHA-256
`5cdf26d0d34bfc8ab3d298d99f8a1e189aa6e2dba9be1f4cb2968078548a3c10`,
and `/boot/config-7.0.0-mk2-gce-lab` hashes to
`f7a61b040e35d4579d3526122d5803e46a20da2d9b527c7c9c043177b6d9c458`.
The agent, relay, and module hashes re-matched the exact-source build. These are
the inputs selected for the strict kernel manifest.

The generated runtime environment, strict host config, and kernel manifest
passed local JSON checks. Their SHA-256 values are `ea011183…`, `82270b0c…`,
and `36ebbc7f…`, respectively. Installing the environment/config through the
current deployment manager into an isolated fake root succeeded as deployment
generation `227c1fca75470cc7f19298c789b7eb97750ed782266feaf328c40f4c32ede975`,
and `inspect` confirmed every expected stable link. This preflight did not
modify the disposable host's privileged installation.

The three validated configuration inputs were transferred to the guest, and
remote SHA-256 checks exactly matched their local values before any privileged
copy. This closes the configuration-transfer integrity boundary; it does not
yet establish that installation or service activation succeeds.

The existing storage filesystem was mounted only after rechecking its by-id
target, byte size, serial, type, label, and UUID. It mounted from `/dev/sdb` at
`/srv/multikernel-storage`; its existing top-level content is limited to
historical `child-a`, `child-b`, and `lost+found`, with no runtime subtree yet.
Pinned Kerf was installed into a root-owned venv and reports `0.2.0`. The
kernel, config, current initramfs, current agent/relay, transport module, and
NBD helper were copied root-owned to their planned paths, and every installed
SHA-256 re-matched its build input. The current bootstrap validator accepted
manifest SHA-256 `36ebbc7f…`, the exact kernel release, required config, full
listed OCI feature set, and all artifact identities. Managed binary/deployment
generation installation and service activation remained pending at this
checkpoint.

The binary manager installed and activated immutable release
`0.1.0-dev-2051d0131428b3e75ab177b3f3e63bf4b41ad6ea`; inspection found its
single release and all six host/CNI command links managed, and version/hash
checks reached the exact installed bytes. The subsequent deployment-manager
call failed closed before creating a deployment because the archive had been
extracted under the ordinary user's ownership while the installer ran as root;
it specifically rejected `deploy/systemd/sys-fs-multikernel.mount` as an
unsafe deployment input. No service was activated. Deployment must be retried
from a root-owned extraction of the same digest-verified source archive.

The archive digest was rechecked, then the same source archive was extracted
into private root-owned `/root/mklinux-src-2051d013-20260920`. From that trust
boundary the deployment manager installed and activated generation
`227c1fca75470cc7f19298c789b7eb97750ed782266feaf328c40f4c32ede975`.
Inspection confirmed every systemd, runtime environment, host config, CNI,
containerd fragment, and runtime support-tool link is managed. The root-owned
copy also contains neither `.git` nor the pre-existing untracked evidence
directory. Containerd/Docker integration and service activation remained
separate subsequent steps.

Before daemon integration, containerd and Docker were active with empty task
and container inventories and Multikernel sysfs had no child instances.
Containerd had no explicit main config, but `containerd config dump` reported
config version 3 and the default import `/etc/containerd/conf.d/*.toml`.
Docker had no `daemon.json` and reported `runc` as its default and only
configured runtime. This is the clean point at which the Multikernel fragment
and opt-in Docker runtime may be activated without displacing a workload or
changing the default runtime.

The first integration command stopped at its pre-restart check: despite the
default dump advertising the import glob, with no explicit
`/etc/containerd/config.toml` the effective dump did not contain the installed
`multikernel` runtime stanza. The required grep returned nonzero, so containerd
was not restarted and Docker configuration was not changed. A complete staged
default main config must be validated with the import fragment before it is
installed, exactly as the deployment guide requires.

The first attempt to stage that main config did not execute remotely: a nested
quote in a grep pattern made the local command parser report an unexpected
end-of-file. It created no candidate and changed no guest configuration. The
retry must use shell-safe, quote-free structural checks.

The corrected containerd integration passed. A complete default version-3
candidate explicitly contained the import glob; resolving that candidate
showed the `multikernel` runtime with type
`io.containerd.multikernel.v2` and preserved
`default_runtime_name = 'runc'`. Because the main config was absent and the
task inventory was empty, the candidate was installed and containerd
restarted. The post-restart effective dump retained the same Multikernel
stanza and runc default, and the final task inventory remained empty.

Docker integration also passed its fail-closed path. The merge tool generated
the candidate from the absent prior config, `dockerd --validate` returned
`configuration OK`, the candidate was installed only after confirming an empty
container inventory, and Docker reloaded successfully. Post-reload `docker
info` advertises `io.containerd.multikernel.v2` while preserving `runc` as the
default; the container inventory remains empty.

Initial managed-service activation exposed a deployment/runtime contract bug.
The mount unit and `mknetd` started, and `mk_transport` loaded, but
`mkruntimed` logged `host configuration must be a regular file with mode 0640
or stricter` and exited with status 2. The deployment manager publishes
`/etc/mkruntime/config.json` as a stable symlink, while the runtime's strict
host-config loader rejects that pathname form. The immediate aggregate
`is-active` check observed systemd's restart window and is not durable-health
proof. No `/var/lib/mkruntime` or storage `runtime` subtree was created; only
the empty mknetd state directory appeared. This must be fixed in source and
redeployed before any live matrix is attempted.

## 2026-09-26 live-continuation checkpoint

Source inspection confirms the cleanup-only `Cleanup` path reconstructs the
persisted network endpoint without reconstructing a guest client, network
pump, or TUN descriptor. `stopNetwork` nevertheless issued a final `REPORT`
with fresh zero counters, explaining the observed `STALE_COUNTER` response
from mknetd. The remediation now conditions that final report on ownership of
one of those live counter-producing resources; endpoint release remains a
separate mandatory operation.

The original guest `ConfigureNetwork` failure still has no safe stage detail:
the agent deliberately redacts arbitrary manager errors to avoid disclosing
paths or workload data. The remediation adds a fixed whitelist of setup stages
(`tun-open`, `tun-request`, `tun-create`, `address`, `link`, `route`, `dns`)
while retaining `INTERNAL` and suppressing the underlying error text. This is
diagnostic instrumentation, not evidence that any particular stage failed;
that conclusion requires an exact-revision live rerun.

Focused race-detector tests now pass for both affected packages. They prove
that cleanup-only recovery makes no mknetd `REPORT`, a service holding a live
TUN owner still makes its final `REPORT`, bounded guest close still releases
the local descriptor, and a staged network error exposes only the whitelisted
stage while suppressing an injected private path and secret token.

The complete local gate now passes: `go test -race -count=1 ./...`,
`go vet ./...`, and `bash scripts/check-docs.sh`, including all 62 OCI
semantic cases, read-only-bind races, bootstrap/storage/image validation,
release/deployment lifecycle tests, resource-ledger and evidence audits. The
known unprivileged socket-rejection subcase was skipped with `EPERM`; it is not
a regression in these changes.

Commit `f34b6bbce490b6715d5185f989039bbb155951de` freezes the locally qualified
cleanup and diagnostic changes. Its source-only archive is
`/tmp/mklinux-f34b6bb.tar.gz`, SHA-256
`25ecbc772d003bb6e3db9dac116647394237ceef7b19d7822f549bd085a3cf3a`.
The untracked historical evidence tree was excluded from the archive and
remains untouched.

The disposable VM already reports `RUNNING`; no restart or recreation was
needed. The pre-transfer audit retains boot ID
`ce405359-2f14-4655-9120-277e292af6da` and kernel
`7.0.0-mk2-gce-lab`. The Multikernel mount, mknetd, mkruntimed, and containerd
are active; mknetd PID 42843 and mkruntimed PID 42862 each have zero restarts.
Containerd task/container inventories are empty, `/run/multikernel` has no
listed residue, and `/var/lib/mkruntime` contains only its state and journal.

The uploaded archive independently re-matched SHA-256 `25ecbc77…`, was
extracted into a new private root-owned tree with neither `.git` nor non-root
entries, and built successfully. Exact build hashes are release manifest
`90869f47…`, shim `ac971043…`, daemon `e2e65944…`, mknetd `0fcd9b75…`, and
agent `a5edf911…`; every binary is stamped with revision `f34b6bb…`. These are
build identities only, before activation.

The first remote binary-manager install exposed a command-level staging error:
`install` activates the new release, and the supplied version already included
the revision. It therefore created/selected the exact-source but incorrectly
named release `0.1.0-dev-f34b6bb…-f34b6bb…`. The matching agent/initramfs
staging hashes are `a5edf911…` and `79b312a2…`. No workload ran and services
were not restarted in this interval. A fresh extraction/build with version
`0.1.0-dev` is required before qualification; none of these first-attempt
artifacts will be used as evidence of the corrected release.

The clean rebuild corrected the version contract. Active managed release is
now `0.1.0-dev-f34b6bbce490b6715d5185f989039bbb155951de`; shim, daemon, and
mknetd report version `0.1.0-dev` and exact revision `f34b6bb…`. Corrected
release manifest is `d581bc41…`, shim `dcb023c4…`, daemon `b9d751ca…`, mknetd
`c64f1fa4…`, agent `8cdb40db…`, and gzip-valid initramfs `9b0f2bcd…`. Strict
bootstrap validation accepts candidate kernel manifest `1650f945…` with the
existing approved kernel, relay, and transport. Services have not yet been
restarted onto these bytes and no workload claim is made.

Coordinated activation passed its empty-host preconditions. Candidate manifest
`1650f945…` was installed by same-filesystem rename while mknetd/mkruntimed
were stopped, then both restarted. After ten seconds the mount, mknetd,
mkruntimed, and containerd were active; mknetd PID 47177 and mkruntimed PID
47196 have zero restarts and exit status zero. Installed manifest, agent, and
initramfs re-match `1650f945…`, `8cdb40db…`, and `9b0f2bcd…`; all three
runtime executables report exact revision `f34b6bb…`. Boot ID remains
`ce405359…`, and containerd task/container inventories remain empty.

The exact basic-suite runner (`3eaa058b…`) passed every host precondition and
entered its first `ctr run`, but exact revision `f34b6bb…` again returned
`INTERNAL: agent operation failed`; the EXIT cleanup then ran. Because the new
whitelisted `ConfigureNetwork` stages did not appear, this result disproves the
working assumption that the surfaced failure necessarily comes from one of
those staged setup operations. Logs, durable lifecycle state, guest console,
and post-cleanup inventories must be captured before another change.

The retained failure audit localizes the first error beyond guest network
configuration. Durable lifecycle sequence 62 records generation
`efe83a023e8eef5d5ae280b0c4be7fc6` as `RUNNING`; mknetd endpoint generation
`185af25c9ca0c75ee21eeacacff2d57c` is `READY` with RX/TX counters 2/1 and no
drops/errors. Thus `ConfigureNetwork` completed and the pump exchanged packets.
The exact shim supervisor/worker are PIDs 47443/47448 from release
`0.1.0-dev-f34b6bb…`, and relay PID 47657 is live. The subsequent EXIT cleanup
failed separately at `CloseNetwork`, leaving the ctr task `CREATED`, child and
storage generation `c7698cdf…` active, and the endpoint/link/firewall rules
retained. This state is intentionally preserved pending console capture.

Before the serial query could recover that retained child console, the VM
rebooted (the cause is not established by this evidence). The new host boot
began at 2026-09-27 02:12 UTC. mknetd then failed closed with an exact durable
consistency error: endpoint generation `185af25c…` still existed in state, but
host link `mkv185af25c9ca` no longer existed after reboot. Consequently the
pre-reboot guest console and live-process state cannot be claimed as retained
evidence. Durable lifecycle/network records remain available and must be
reconciled through owned recovery/cleanup, not manually erased.

New-boot audit records boot ID `a0798b60-d6ff-4490-a176-ff09081f63a9` and no
Kerf pool, child, shim, relay, host link, or network namespace. Durable state,
however, remains lifecycle sequence 62 (`RUNNING`/storage `ACTIVE`), the
mknetd endpoint, its root image, and container metadata. Both services enter
restart loops: mknetd refuses its missing link, while mkruntimed reports
`rootfs reconcile: owned rootfs bundle is absent` because the containerd
bundle lived under reboot-volatile `/run`. This exposes a distinct reboot
reconciliation gap: fail-closed validation detects the mismatch but provides
no owned convergence path even though backend absence is authoritative. No
durable file has been manually changed.

Inspection of the retained cleanup path found why `CloseNetwork` itself failed:
the agent closed its non-persistent TUN descriptor before invoking `ip link
delete`. Descriptor close removes that TUN, so the command can report a missing
link and make otherwise completed cleanup fail. The fix deletes the named link
while its descriptor is still open, closes only after successful deletion, and
retains both on deletion failure for retry. Because live counters prove network
setup completed, new secret-safe stages now cover the next `CreateProcess`
boundaries (`bundle-load` and `root-policy`) without exposing underlying paths
or mount errors.

Focused race-detector tests pass for the agent and shim packages. They prove
the named link is deleted while the TUN descriptor is still open, deletion
failure retains retry state, descriptor-close failure preserves replay
identity, missing bundles receive only the fixed `bundle-load` stage, and an
injected root-policy path/token is absent from the wire reply.

The full local gate passes: repository-wide race tests, `go vet`, and the
complete documentation/schema/evidence/deployment chain, including all 62 OCI
cases and final evidence audit. The known sandbox `EPERM` socket-rejection
subcase is the only skip. Generated Python cache will be removed before commit.

Commit `bbd49c1d88a5e03e5350f80aa6719510518122ee` freezes the fully qualified
teardown ordering and process-stage diagnostics. Its tracked source-only
archive `/tmp/mklinux-bbd49c1.tar.gz` hashes to
`3d93538e1d1a2934a01a7191158ca4f9f8147b39b76b7a33aaf7fc2765effc6e`;
the untracked historical evidence tree remains excluded and untouched.

The reset guest independently matched archive `3d93538e…`, extracted a new
all-root-owned source tree without `.git`, and built exact `bbd49c1…` with the
correct `0.1.0-dev` version. Build hashes are release manifest `8092999c…`,
shim `6782283c…`, mkruntimed `e13d5164…`, mknetd `126782bd…`, and agent
`9571b4f2…`. These are pre-activation identities only.

On empty inventories, the binary manager selected exact release
`0.1.0-dev-bbd49c1d88a5e03e5350f80aa6719510518122ee`. The managed shim,
mkruntimed, and mknetd links report that revision. Matching agent remains
`9571b4f2…`; the newly built gzip-valid dependent initramfs is `7f08fca1…`.
The running services and kernel manifest have not yet been restarted/swapped,
so workload qualification remains unclaimed.

Strict bootstrap validation accepted candidate manifest `4ffc3661…`.
Empty-host activation used a same-filesystem rename while services were
stopped. Ten seconds later the mount, mknetd, mkruntimed, containerd, and
Docker were active; mknetd PID 12565 and mkruntimed PID 12584 have zero
restarts/status 0. Installed manifest and every executable identity match
`bbd49c1…`, task/container inventories remain empty, and boot ID remains
`a0798b60…`. The exact workload rerun is the next checkpoint.

The exact `bbd49c1…` run of unchanged runner `3eaa058b…` again failed its first
`ctr run` with generic `INTERNAL: agent operation failed`, then executed EXIT
cleanup. Neither `bundle-load` nor `root-policy` appeared, so `CreateProcess`
is no longer the supported localization; the next likely boundary is
`StartProcess` (including executable validation or `exec`). Cleanup outcome
and durable state must be audited before changing diagnostics again.

The `bbd49c1…` EXIT cleanup is a live pass for the two teardown fixes. After
shim disconnect/dead-shim cleanup, lifecycle sequence 9 has no sandboxes,
mknetd has no endpoints, and containerd task/container, Kerf child, runtime
storage, shim, and relay inventories are empty. Both daemons remain their
original healthy PIDs 12565/12584 with zero restarts. No `CloseNetwork` or
`STALE_COUNTER` error appears. This proves link-before-descriptor teardown and
cleanup-only counter suppression on the disposable host, while the workload
start failure remains open.

Because network and `CreateProcess` stages are now excluded, the next
instrumentation is restricted to operational `StartProcess` boundaries:
executable validation, OCI constraint encoding, terminal allocation, stdio
pipe setup, and final exec. Each maps to a fixed `INTERNAL` message; raw
executable paths, kernel errors, and workload data remain redacted.

Focused agent/shim race tests pass. They directly verify executable-stage
classification and prove an injected exec path/token is omitted from the wire
reply; all earlier cleanup and diagnostic tests continue to pass.

The complete race, vet, documentation/schema/evidence/deployment, 62-case OCI,
and final-audit gates pass; only the known local socket `EPERM` subcase skips.

Commit `f7c6f475c5c0dc0c35c07e68f51b74f409e211a6` freezes the qualified
StartProcess staging. Its tracked source archive
`/tmp/mklinux-f7c6f47.tar.gz` hashes to
`c851cd2541549d272da4c34cab7632933c40b08edbe2c6a84b342e89d9ddf4c4`;
the historical evidence tree remains untouched and excluded.

The guest re-matched archive `c851cd25…` and built exact `f7c6f47…` from a
new root-owned extraction. Build hashes are release manifest `525cbadb…`, shim
`2d252204…`, mkruntimed `da058880…`, mknetd `5c080d9e…`, and agent
`a727b7ab…`. Activation remains pending.

On empty inventories, managed release `0.1.0-dev-f7c6f47…` became current.
The exact agent/initramfs hash to `a727b7ab…`/`a9b85660…`, and strict bootstrap
validation accepts candidate manifest `11113950…`. Running services have not
yet been restarted onto this set.

Atomic empty-host activation passed. After ten seconds all five services are
active; mknetd PID 14500 and mkruntimed PID 14520 have zero restarts/status 0,
executables report exact `f7c6f47…`, installed manifest re-matches
`11113950…`, and boot ID remains `a0798b60…`.

The exact `f7c6f47…` live run localizes the remaining failure: unchanged runner
`3eaa058b…` reaches `ctr run`, then receives
`INTERNAL: guest process start failed at exec`. This proves executable
validation, constraint encoding, and stdio setup completed; the error is the
final child `exec` boundary. EXIT cleanup ran. No raw path or kernel error was
exposed, as designed. Post-cleanup inventories and the constraint-helper/chroot
launch design require inspection before remediation.

Source inspection explains the exec-stage failure. Any process with the normal
OCI constraint contract was re-executed as `/mk-agent` while Go simultaneously
chrooted it into the container root; the trusted agent exists only outside
that root, so final exec cannot resolve the path. Guest init already binds the
trusted procfs at the container's `/proc`, and the constraint executor's own
tests use `/proc/self/exe`. The launch path now uses that kernel-provided handle
to the current agent across chroot, avoiding both an untrusted workload-root
copy and the nonexistent `/mk-agent` path. This source localization still
requires exact live confirmation.

Focused agent/shim race tests pass, including a regression guard that rejects
return to a workload-root `/mk-agent` dependency and pins the constrained
launcher to `/proc/self/exe`. The existing constraint-application re-exec test
also remains green.

Full race, vet, and documentation/evidence gates pass; the known local socket
`EPERM` subcase remains the only skip.

Commit `1e31080a1d3099fec0a0f2f9fa790eff94719779` freezes the chroot-safe
constraint re-exec fix. Its tracked source archive
`/tmp/mklinux-1e31080.tar.gz` hashes to
`9b0b9ac2339ca466923524c394233b5a6e850332df7e7b9e2b6f376c53ae8d5d`;
the untracked historical evidence tree is excluded.

Guest archive verification and exact `1e31080…` build passed. Hashes are
release manifest `e695cecb…`, shim `db39bcec…`, mkruntimed `4d3b4609…`, mknetd
`095e3931…`, and agent `eb3a2dc1…`; activation remains pending.

Managed release `0.1.0-dev-1e31080…` is selected on empty inventories.
Matching agent/initramfs are `eb3a2dc1…`/`1cd089b1…`, and strict validation
accepts candidate manifest `cefc96b8…`. Service/manifest activation is still
separate.

Coordinated activation passes: after ten seconds all five services are active;
mknetd PID 16385 and mkruntimed PID 16404 have zero restarts/status 0, exact
revision `1e31080…` and manifest `cefc96b8…` are installed, and boot ID remains
`a0798b60…`.

The exact `1e31080…` run proves the constraint-helper fix: the first ctr task
reaches `RUNNING`, clearing the prior exec-stage failure. The next boundary is
Docker task creation, which fails before its task starts with
`invalid argument: inspect stdout: too many levels of symbolic links`.
This comes from host stdio-path inspection, not the guest. The suite then ran
cleanup. Thus ctr start is newly proven, but the shared basic suite is not a
pass; Docker's actual stdio pathname ancestry must be captured and handled
without weakening the identity/race boundary.

The immediate post-failure audit on boot `a0798b60…` finds no lifecycle or
mknetd state file, empty containerd task/container inventories, and no shim,
relay, or runtime-root process. mknetd PID 16385 and mkruntimed PID 16404 are
still active with zero restarts, so this EXIT cleanup is clean. Host path
inspection also establishes the compatibility boundary: `/var/run` is the
root-owned four-byte symlink to `/run`, while Docker's root is
`/var/lib/docker`. This supports—but does not by itself expose the exact
request string—the diagnosis that the strict no-symlink walk rejects Docker's
traditional `/var/run/...` spelling.

The same journal contains a separate cleanup diagnostic: after the Docker shim
disconnected, containerd's configured
`/usr/local/bin/containerd-shim-multikernel-v2 ... delete` exec returned
`no such file or directory`. A follow-up inode check finds the managed link
and exact `1e31080…` target present, so removal is not established; this is a
transient exec/path-resolution failure requiring reproduction. Runtime-owned
resources were already empty, so it is not evidence of a leaked workload.

The stdio compatibility fix is intentionally not general symlink traversal.
Only an exact `/var/run/` prefix is eligible; the shim verifies `/var/run` is
a caller-owned, single-link symlink with unchanged identity and exact `/run`
target, rewrites to `/run/...`, and then applies the existing no-symlink,
no-magic-link, ownership, mode, link-count, and inode checks. Create and Exec
persist that normalized path, so recovery and later opens cannot return to the
alias. Focused race tests pass for the accepted exact alias, an unrelated
prefix, an unexpected alias target, ordinary hostile symlink ancestry,
replacement, cancellation, and pre-mutation rejection.

The subsequent complete repository race suite passes, including the shim,
agent, lifecycle, network, rootfs, and storage packages; `go vet ./...` also
passes without diagnostics. Documentation and exact-revision live gates remain
pending.

The complete documentation/schema/evidence/OCI/bind/bootstrap/storage/image/
release/deployment/resource-ledger/containerd/final-audit gate then passes;
`git diff --check` is clean. The only skip is the already classified local
socket-rejection `EPERM` subcase. Exact-revision live proof remains pending.

Commit `444428ae88e0575e1a6a4a915ea9c83c001b6fc5` freezes the narrow
stdio-alias remediation and all findings above. Its tracked-source-only archive
`/tmp/mklinux-444428a.tar.gz` hashes to
`ed539d24dd58c92300472042795d862403defe20d35b427c98dfeed33026efda`;
the untracked historical evidence tree is excluded. Guest verification/build
and activation remain pending.

The guest receives the same `ed539d24…` archive on an empty host. The first
root-only extraction command then stops before compilation because its nested
`awk` hash expression is expanded as an unset shell parameter. This is a
harness quoting failure, not a build or runtime result; the unique extraction
is retained and will be verified without `awk` before use.

The quoting-free retry verifies the archive and builds all seven binaries plus
the exact release manifest for `444428a…`. Initramfs assembly then stops at an
`install` input with `No such file or directory`; therefore no coherent
candidate exists and nothing is activated. The agent, transport-module, relay,
and init-script inputs must be checked individually before retrying.

Input inspection localizes the missing file to the optional
`/usr/local/libexec/multikernel/mkvsock-relay`; agent, init, BusyBox, and
`mk_transport.ko` exist. Exact successful build hashes already include release
manifest `8a9d5f15…`, shim `e176c997…`, mkruntimed `199529cb…`, mknetd
`72db893c…`, and agent `52e81fa1…`. A combined current-initramfs inspection
command then fails at shell parse time due to nested quotes and changes
nothing; it must be split before deciding whether the optional relay belongs.

The split inspection resolves the correct manifest-bound inputs. The active
initramfs contains `mk-agent`, `mk_transport.ko`, and `mkvsock-relay`; the
relay lives at `/opt/mkruntime/bin/mkvsock-relay` with hash `293ff1ea…`, and
the module artifact hashes to `bef1b888…` (matching its deployed libexec copy).
Thus the failed assembly used the wrong relay pathname; the coherent rebuild
must use these active-manifest inputs.

The corrected initramfs build passes and hashes to `cb1d9771…`; listing the
archive confirms all three expected files are present. The build set is now
coherent, but managed-release install, candidate manifest creation, strict
validation, and service activation remain separate pending steps.

`manage-runtime-binaries.py install` both installs and selects immutable
release `0.1.0-dev-444428a…` (`installed_and_active`); this is earlier than the
intended install-only checkpoint. No service was restarted, so running daemon
processes still belong to the preceding set. Exact agent/initramfs manifest
validation and coordinated service activation remain mandatory before calling
the host coherent.

Release-specific artifact staging verifies and copies exact agent `52e81fa1…`
and initramfs `cb1d9771…`, then stops before candidate JSON is written because
the jq named variables are again over-escaped and remote `set -u` expands
`ap`. The active manifest and services remain unchanged; retry is restricted
to manifest construction/validation with one remote escape.

The corrected candidate construction and strict bootstrap validation pass.
Candidate manifest `9e9c1908…` resolves exact agent `52e81fa1…`, initramfs
`cb1d9771…`, pinned kernel/module/relay, compatibility, configuration, and OCI
feature set. Active manifest remains `cefc96b8…`; daemon PIDs 16385/16404
remain healthy and unrestarted. Coordinated activation is still pending.

The activation preflight finds empty default/moby task and container
inventories, no Docker containers, child instances, runtime links, durable
lifecycle/network state, or storage-server process on unchanged boot
`a0798b60…`. Its shim check is inconclusive because `pgrep -x` warns that the
executable name exceeds Linux's 15-character comm limit; `/proc/*/exe`
inspection must replace that subcheck before activation.

Exact `/proc/*/exe` inspection then finds no shim, relay, or storage-server
process, closing the preflight gap. Coordinated activation succeeds on the
same boot: all five units are active; mknetd PID 20606 and mkruntimed PID 20628
have zero restarts/status 0; shim, mknetd, and mkruntimed report exact
`444428a…`. Active kernel manifest, release manifest, and shim hash to
`9e9c1908…`, `8a9d5f15…`, and `e176c997…`. Workload proof remains pending.

The unchanged basic runner (`3eaa058b…`) on exact `444428a…` clears the stdio
alias failure: ctr again reaches `RUNNING`, and Docker advances through shim
task creation into child-root OCI validation. It then fails closed on
`unsupported process field(s): apparmorProfile, oomScoreAdj`, exits 125, and
runs the EXIT cleanup trap. Thus the alias fix is live-proven, but the basic
suite is still not a pass; cleanup and the concrete values/security contract
of these two Docker fields must be established next.

The failed-run cleanup audit is fully empty across default/moby task and
container inventories, Docker objects, child instances, runtime storage,
lifecycle/network state files, host links/rules, and shim/relay/server
processes. Daemon PIDs 20606/20628 remain active with zero restarts. The
containerd dead-shim `ENOENT` also reproduces while the managed runtime link is
installed; it remains a separate transient cleanup-command diagnostic, not a
surviving runtime resource.

A short-lived default-runc Docker container exposes the exact generated OCI
values from its live containerd bundle: `apparmorProfile` is
`"docker-default"` and `oomScoreAdj` is integer zero. The same process object
contains only those additions plus already-supported args, capabilities, cwd,
env, and user. Zero OOM adjustment is inert; the nonempty AppArmor profile is
not and must not be silently discarded. Child-kernel enforcement availability
must be observed before defining compatibility.

The exact child-kernel probe reports AppArmor enabled (`Y`) and exposes
`/proc/self/attr/{current,exec}`, but the securityfs profile interface is
unavailable and the workload is `unconfined`. Its cleanup sequence then races:
immediate task deletion after SIGKILL sees the task still `running` and fails
precondition. This does not establish a leak, but the exact probe must be
reconciled and removed before further qualification; profile availability also
requires an actual attr-exec attempt rather than inference from the module bit.

The follow-up inventory shows the probe converged to `STOPPED` while its exact
task, container, and child still await deletion. The first explicit cleanup
command executes no mutation because another over-escaped `awk` field trips
remote `set -u`; cleanup can proceed directly from the already observed
stopped state without parsing.

Direct deletion then reaches runtime teardown but fails at
`close guest network: INTERNAL: agent operation failed`. This is a real
cleanup-path result, not a harness error. No cleanup-success claim follows;
lifecycle, network, child, retained console, and task state must be audited
before deciding whether retry is safe.

The audit proves cleanup is incomplete: stopped task/container, exact child
instance, MK firewall chain/NAT rule, and runtime storage/rootfs records remain.
It also finds daemon PIDs now 1236/1466 rather than the earlier activation
PIDs, with mkruntimed at one restart: its first later start failed host
qualification because the Google guest agent was not yet confirmed, then the
unit restarted successfully. This is a separate host/service interval and
invalidates carrying the earlier PID-stability assertion forward; boot ID and
exact durable ownership must be re-established before teardown retry.

Boot audit confirms an intervening VM reboot: current boot is `e76edca2…`,
started 07:26 UTC, replacing activation boot `a0798b60…`. The AppArmor probe
and its exact `444428a…` supervisor/worker (PIDs 1976/1981) were created after
this reboot, so their resources are current-boot, not stale pre-reboot residue.
Current durable stores are `/var/lib/mkruntime/state.json`,
`/var/lib/mknetd/state.json`, and mkruntimed storage/rootfs state; the older
quarantine remains separate. Record contents must bind the pending cleanup
before retry.

Durable records bind one current generation `372e816c…`: lifecycle sequence
45 still says `RUNNING`; mknetd endpoint `f5152c21…` is `READY` with exact
5/5 RX/TX and zero drops/errors; storage export `e6a18eb0…` is `ACTIVE`; and
the rootfs record remains `PREPARED`. These identities match the retained
child and exact source manifest `9e9c1908…`. The failed close did not advance
host teardown, so shim recovery/log evidence is required before a bounded
retry of the same idempotent delete.

Shim recovery confirms init exit 137 and preserves generation/network/storage
ownership; its last persisted counters are 2/1, while mknetd reached 5/5 during
the failed close attempt. Supervisor and worker remain live with their held
bundle/runtime descriptors and agent sockets. The public API deliberately
redacts the guest command error, so a bounded filtered console capture is the
next evidence step.

The bounded filtered console attempt yields no guest log; Kerf reports
`Kernel image not loaded for instance` for the retained entry. This is
inconclusive about `CloseNetwork`. Because the exact shim and durable
identities remain intact and guest close is designed for retry, one bounded
repeat of the same task deletion is the next safe action.

That bounded retry fails at the identical
`close guest network: INTERNAL: agent operation failed` boundary. Repeating
the same request again is not evidence-producing. The stopped task's exact
supervisor and pinned recovery remain live, so the next recoverable route is
the designed shim-worker restart/recovery path—not manual deletion of durable
state.

Killing exact worker 1981 does not produce a replacement marker. Follow-up
shows both worker and supervisor 1976 exited, the worker PID file disappeared,
and containerd logged shim disconnect plus dead-shim cleanup. Its task
inventory is now empty while container metadata remains. Cleanup-only command
completion and resource inventories must be observed before removing metadata
or claiming recovery.

Cleanup-only recovery completes: lifecycle advances to sequence 49 with no
sandboxes, mknetd has no endpoints, and child, MK links/rules, shim, relay,
and storage-server process inventories are empty. Only inert containerd
container metadata remains. Storage/rootfs state and directories still require
verification before normal metadata removal closes this probe.

The first combined storage/rootfs verification command has malformed jq
escaping and stops before `ctr containers rm`; no mutation occurs. Read-only
state projection and metadata removal must be split.

The split projection shows the probe export `RELEASED`, no rootfs records, and
an empty runtime storage directory. Normal container metadata removal then
passes with empty task/container/child inventories. This closes recovery of
the probe without manual durable-state deletion.

The child image has no AppArmor policy loader/profile payload, while live
processes are unconfined; accepting `docker-default` would therefore discard a
requested security control. The fail-closed integration direction is explicit
Docker `apparmor=unconfined`, plus exact acceptance/removal of only integer
`oomScoreAdj=0`. The generated OCI shape of that explicit request still needs
live observation before implementation.

Live inspection of a short-lived Docker container with explicit
`--security-opt apparmor=unconfined` shows the OCI fields remain present as
exact string `"unconfined"` and integer `oomScoreAdj=0`. Therefore validation
must accept and strip only that pair, continue rejecting `docker-default` and
all other/malformed values, and qualification must make both AppArmor and
Docker-network opt-outs explicit.

The compatibility implementation now accepts only the exact AppArmor string
`unconfined` and an exact Python integer `oomScoreAdj=0`, removes those two
host-only compatibility fields from the child projection, and keeps
`docker-default`, nonzero, boolean, floating-point, and malformed values
fail-closed. The qualification entry points now explicitly pass both
`--network none` and `--security-opt apparmor=unconfined` on every Docker
create/run path. On 2026-09-27 the focused validator completed all 67 semantic
cases plus namespace, file-identity, and outer-cleanup boundaries; Python
compilation, both shell syntax checks, and `git diff --check` also passed.
This is local evidence only; the exact revision has not yet been committed or
activated on the disposable host.

The complete local gate then passed on 2026-09-27: `go test -race -count=1
./...`, `go vet ./...`, the full documentation/schema/evidence/deployment
chain, and `git diff --check`. The Docker invocation audit also confirms every
direct qualification `docker create`/`docker run` path carries the explicit
network and AppArmor options, including piped stdin, detached attach, PTY, and
live-resize cases. Commit, archive, deployment, and live execution remain the
next evidence boundary.

The verified checkpoint was committed as
`a72c5cb97c899bbfb567137ddfa48bf42165a887` (`runtime: support explicit
unconfined Docker OCI`). Its source-only qualification archive is
`/tmp/mklinux-a72c5cb.tar.gz`, SHA-256
`bbc2eb49b0aaa4fe1a4846fd500613f569441276a99d860c0bd9aa9e1d880a6c`.
Only the pre-existing untracked `evidence/runtime-20260907/` tree remains
outside the commit; it was not staged or altered. Guest transfer, digest
verification, build, activation, and live execution are still pending.

The disposable GCE resource still exists and is `RUNNING`: project
`new-demo-project-462517`, zone `asia-southeast1-b`, instance
`mklinux-g4-g6-final-20260905`, resource ID `8436995220542526424`, with
`lastStartTimestamp=2026-09-27T00:26:24.270-07:00`. No recreation or restart is
needed at this boundary. The exact archive has not yet been transferred.

Transfer succeeded and the guest independently reproduced archive SHA-256
`bbc2eb49b0aaa4fe1a4846fd500613f569441276a99d860c0bd9aa9e1d880a6c`
(992193 bytes). The current guest boot is
`e76edca2-d8d6-4800-9df8-f9ee9dccca8f`, kernel
`7.0.0-mk2-gce-lab`; mkruntimed, mknetd, and containerd are active and the
containerd task/container inventories are empty. The combined inventory probe
then stopped because `/usr/local/bin/kerf` is not the installed Kerf path, so
child and mknetd inventories are not yet established by this command and must
be re-run using discovered command paths before deployment.

The corrected preflight proves the host is quiescent: zero child instances,
Docker objects, `mkv*` links, Multikernel NAT/filter rules, containerd tasks or
containers, active lifecycle sandboxes, mknetd endpoints, rootfs records, and
runtime-storage files. Lifecycle sequence 49 retains historical results only;
all five storage records are `RELEASED` with offline checks, not active
exports. Service inspection records mknetd PID 1236 with zero restarts,
containerd PID 1377 with zero restarts, and mkruntimed PID 1466 with one
successful automatic restart after the already documented boot-time guest
agent ordering failure. The discovered Kerf executable is
`/opt/mkruntime/kerf-venv/bin/kerf`; it has no `list` subcommand, so direct
sysfs and durable-state inventories are the applicable evidence here.

Root-owned extraction of exact `a72c5cb…` source and the complete seven-binary
revision-stamped build succeeded. Child initramfs assembly then stopped at
`install: No such file or directory` because the command assumed the transport
module lived inside the prior revision's agent/initramfs directory. No binary
release, deployment, kernel manifest, or service was activated. As with the
earlier bootstrap build, each active manifest-bound input must be resolved and
verified individually before a narrow assembly retry; this failure is not a
runtime result.

Active manifest inspection confirms agent and prior initramfs hashes
`52e81fa1…` and `cb1d9771…`, relay path `/opt/mkruntime/bin/mkvsock-relay`
with hash `293ff1ea…`, and manifest hash `9e9c1908…`. The first inspection loop
incorrectly selected the whole `transport.module` object rather than its
`.path` member and stopped at a synthetic path `{`; module/kernel hashes and
the absence of a partial candidate still require a corrected direct check.

The corrected direct check resolves root-owned module
`/opt/mkruntime/artifacts/mk_transport.ko` (`bef1b888…`), kernel
`/opt/mkruntime/artifacts/vmlinux` (`5cdf26d0…`), and relay (`293ff1ea…`),
and proves the failed attempt left no candidate initramfs. These match the
active manifest. A retry limited to initramfs assembly can now use the exact
module and relay paths without changing active state.

The isolated assembly retry succeeds. Exact build hashes are release manifest
`55e4ea88…`, shim `f32e4a1c…`, mkruntimed `497ca3f1…`, mknetd `327dcb51…`,
agent `0ee464e9…`, and root-owned mode-0600 initramfs `a78778e2…` (5637967
bytes). The subsequent non-root archive-list pipeline cannot read that mode-
0600 file; because the remote shell lacked `pipefail`, the trailing `sort`
masked the gzip permission error. Artifact content membership is therefore
not yet proven and must be rechecked under sudo with pipeline failure
propagation. Nothing has been installed or activated.

The first sudo/pipefail membership retry exits 1 with no matches because its
pattern assumes `./` prefixes. It proves neither corruption nor membership;
the unfiltered archive path spelling must be observed before another exact
assertion.

The privileged unfiltered cpio listing succeeds and contains exact members
`mk-agent`, `mk_transport.ko`, and `mkvsock-relay` together with `init`,
BusyBox, and the fixture bundle. The candidate bootstrap is therefore complete
at hash `a78778e2…`; managed binary/support installation and manifest staging
remain separate operations.

The managed binary installer verifies, installs, and selects immutable release
`0.1.0-dev-a72c5cb97c899bbfb567137ddfa48bf42165a887`; all managed links remain
valid and shim/mkruntimed/mknetd links report that exact revision. As expected,
service PIDs remain 1466/1236 with restart counts 1/0, so the running daemon
processes are still the preceding release. Support deployment, manifest
validation, and coordinated restart remain mandatory before coherence can be
claimed.

The `f7c6f47…` failed-start cleanup is independently clean before upgrade:
containerd, lifecycle, mknetd, child, and runtime-storage inventories are all
empty; daemon PIDs 14500/14520 remain active with zero restarts.

For environment reset only, after proving no task process, child, or host
network resource survived the reboot, the exact stale records were moved—not
deleted—into root-only quarantine. Lifecycle state/journal hashes are
`3aafca1d…`/`3469d527…`, mknetd state is `baadf93f…`, and the root image
remains under storage quarantine with its previously recorded hash. Orphaned
container metadata was removed. mknetd then started cleanly as PID 6958 with
zero restarts; mkruntimed still failed after three attempts, proving another
durable rootfs record exists outside `/var/lib/mkruntime`. The reset is not yet
complete and is not runtime cleanup evidence.

The remaining stores were `/var/lib/mkruntimed/storage` and `rootfs`, with
state hashes `f7d4e4b1…` and `231c3776…`; both were added to the same root-only
quarantine before recreating empty service directories. The disposable host is
now clean: mknetd PID 6958 and mkruntimed PID 7683 are active with zero
restarts/status 0, containerd task/container inventories, Kerf child inventory,
and runtime storage directory are empty. All stale data remains recoverable in
the named quarantine. This reset establishes a test precondition only.
### 2026-09-28 — live Docker PID and cleanup failure traced to separate shim contracts

- Source tracing after the live `bind-mount /proc/0/ns/net` failure confirms that init `Start` overwrites the task PID with the child-guest PID and `Connect` returns that same guest-only value (`Version: multikernel-v1-guest-pid`). A guest PID is not a host `/proc/<pid>/ns/net` owner, so it cannot satisfy Docker's host-side network-namespace lookup; returning the shim PID would also be incorrect because the shim remains in the host namespace. The runtime needs a lifecycle-owned host PID resident in the endpoint namespace while retaining the guest PID separately for agent reconciliation.
- The leaked stopped task is a second, independent defect. `Delete` returns immediately when guest `CloseNetwork` fails, before agent/relay closure, mknetd `RELEASE`, sandbox stop/delete, or rootfs cleanup. For a dedicated disposable child, guest-network teardown failure must be collected while irreversible host ownership cleanup continues; otherwise a recoverable guest-side error strands every host-side resource, exactly as the live audit showed.
### 2026-09-28 — disposable VM resumed in a changed recovery state

- GCE reports `mklinux-g4-g6-final-20260905` as `RUNNING` (instance ID `8436995220542526424`, last start `2026-09-27T17:28:55.578-07:00`), so no restart was necessary.
- The prior `/tmp/audit-runtime-clean.sh` helper is no longer present. A bounded read-only check found `mkruntimed` and `mknetd` both `activating`, while `containerd` and Docker are `active`. The former `mk-proof-ctr` task is absent from `ctr -n default tasks list`, but the `mk-proof-ctr` container metadata remains. This differs from the previously recorded stopped-task-plus-live-child state and must be treated as recovery evidence, not silently cleaned or overwritten.
### 2026-09-28 — reboot exposes durable/orphan reconciliation gap

- Boot ID is now `4bd0f8ce-5c42-48d7-a19d-0379a97949ec`. The reboot removed ephemeral `/run` ownership (containerd task bundle, runtime-managed netns, and `mkv*` link) while durable daemon journals still claim the leaked generation.
- `mknetd` is in an unbounded restart loop (`NRestarts` observed at 215 and rising) because durable endpoint `c315962acb7a9a27638ddb5b448a66d1` says `READY` but host link `mkvc315962acb7` is absent. `mkruntimed` is likewise restart-looping because sandbox `4edb8056f26c66fcc11e94ce812771b5` remains journaled `RUNNING`/backend `ABSENT`, while its owned rootfs bundle under the former `/run/containerd/.../mk-proof-ctr` path is absent. Both fail closed, but neither converges to a serviceable state after reboot.
- The durable mkruntimed record explicitly marks `OPERATOR_ACTION` with `journal=RUNNING backend=ABSENT incomplete=false`; mknetd retains the managed namespace `/run/netns/mk-c315962acb7a`, counters `rx=4 tx=4`, and endpoint ownership even though the reboot removed its ephemeral host objects. This is now direct live evidence of a G4/G6 reboot-orphan reconciliation gap, separate from the shim deletion short-circuit.
- `/opt/mkruntime/current` and `/opt/mkruntime/releases` are absent after reboot even though the system-installed binaries and units remain. The next qualification environment should therefore be reprovisioned/redeployed from the exact source revision rather than treating this host as a clean continuation.
### 2026-09-28 — host namespace PID and non-short-circuit deletion pass broad Go gates

- The shim now starts a lifecycle-owned helper by entering the exact endpoint namespace with `/usr/bin/nsenter` *before* execing the Go holder. This avoids the Linux thread-scoped `setns(2)` trap: a post-runtime Go call could change a non-leader thread while `/proc/<pid>/ns/net` continued to expose the leader's old namespace. Startup waits for helper readiness and compares the target and `/proc/<holder>/ns/net` object identities before publishing the PID.
- Create, init Start/State/Pids/events/Delete, and Connect expose the verified host holder PID; the child-guest PID remains separately retained in `process.pid` for authenticated agent reconciliation. Holder startup is part of create/recovery, and holder teardown precedes endpoint release in rollback and normal deletion.
- Init deletion now treats guest network/quiesce/agent-close errors as diagnostic notes, continues holder/relay/endpoint/sandbox cleanup, and succeeds when all host ownership is removed. Host cleanup failures remain retryable errors. A focused regression injects `CloseNetwork` failure and proves `REPORT`, `RELEASE`, sandbox stop/delete, holder removal, endpoint clearing, and process removal still occur with the original holder PID in the response.
- `GOCACHE=/tmp/mklinux-gocache go test -race ./...`, `go vet ./...`, and `git diff --check` pass on the modified tree. Exact guest build and live Docker qualification remain pending.
### 2026-09-28 — exact namespace-holder revision selected

- The source/test correction is committed as `34b1f9cb2ce2bb87241a40449fff97f3ef07b3ba` (`runtime: expose managed network namespace pid`). Only the shim implementation and its tests are in that commit; these continuously updated findings and the pre-existing untracked evidence tree remain outside it. This is the exact revision selected for the next guest build and live qualification.
### 2026-09-28 — broken disposable host replaced from qualified snapshot

- The restart-looping disposable instance was deleted; its auto-delete boot disk was removed while non-auto-delete `mk-mediated-storage-20260830` was preserved and detached `READY`. A new 100 GB pd-balanced boot disk was restored from qualified snapshot `mklinux-lab-pre-daxfs-20260828-2030` and reached `READY`.
- Replacement `mklinux-g4-g6-final-20260905` was created as `n2-standard-16` with the restored auto-delete boot disk, retained 20 GB non-auto-delete storage disk, Secure Boot disabled, vTPM/integrity monitoring enabled, serial console metadata, and the prior disposable/purpose labels. GCE reports `RUNNING`, internal IP `10.148.0.58`, external IP `136.85.39.91`. Guest identity and cleanliness remain to be independently qualified.
### 2026-09-28 — recreated guest and retained disk qualified without discarding prior artifacts

- Guest baseline: hostname `mklinux-g4-g6-final-20260905`, boot ID `83ef4350-79ea-475f-b73c-5d856040ad5d`, kernel `7.0.0-mk2-gce-lab`, x86-64, 16 CPUs, `65836300 kB` memory, active Google guest agent, present Multikernel sysfs with no children, and inactive runtime/container services before package installation.
- The retained whole disk remains exact 20 GiB ext4, label `mk-mediated-host`, UUID `507c0523-8e58-4ae3-9524-3b7513aad344`, serial `mk-mediated-storage-20260830`; read-only `e2fsck -fn` passed all five passes. Pinned Kerf/Linux trees remain clean at `8b72b3e9b266f8d32e707e2c1743ad7afc50b1ec` and `3bdd35b64413da0b4e089ce931bfc2e8b031cbf7`.
- Required packages were installed from Ubuntu Resolute: containerd `2.2.2-0ubuntu1.1`, Docker `29.1.3-0ubuntu4.1`, Go 1.26, and socat `1.8.1.1-1ubuntu0.1` (plus dependencies).
- Prior runtime disk content was not deleted. The prior `runtime` tree was atomically renamed to `reboot-quarantine-20260928-83ef4350`, alongside the earlier `reboot-quarantine-20260927-a0798b60`; a new empty mode-0750 `runtime` directory was created. The exact UUID fstab entry was installed and the disk is mounted `rw,nosuid,nodev` at `/srv/multikernel-storage`.
### 2026-09-28 — exact `34b1f9c` guest build succeeds

- The tracked-source archive SHA-256 is `ba10dcd557c1bdd79ff22ab574d07ca3d3027589dd3b7f864e2bb75ce20c8d8f` (1,010,217 bytes). The guest independently verified it, extracted it without `.git` into `/var/tmp/mklinux-build-34b1f9cb2ce2bb87241a40449fff97f3ef07b3ba`, and passed `scripts/verify-host.sh`.
- Pinned transport module rebuilt warning-clean with exact `7.0.0-mk2-gce-lab` vermagic and SHA-256 `bef1b888e7c66705f0d652b1437a239835584f2fb376ad6bfea2a77ecaa1c3d2`.
- Exact revision-stamped build hashes: release manifest `0d64cd97e4bac2287ff227eb957fc0d785dd38aa36614750b8cb1604a4584ab7`, shim `8910812c42efa7c0786f8712cbda06f785ac6c7084c81e7cc6946e5a05e5c219`, mkruntimed `b3be88f14e6e3011c7d25adb6060a0085e7a18469d38d3a7b78782e225862281`, mknetd `65130b154c104842fa715231f5a11810e095a9cbe1af338ac8a18ad92b32c7db`, and agent `718f6bfcf9d7e699448855fef21972ed62d00d9fa70dbfdbd1c6b1bc466f1391`.
- Static helpers reproduce NBD `a0259098bba0a4319737f2ca4fca5c8ea39e42c8261c6dd0edadcacaa10f0ca3` and relay `293ff1eaa209d16103c7caaa8e8f9a24702e58d979453fad5495c503f76aaf98`. The new agent initramfs hashes to `9c9ad269fd27fda7ac9a8244ee079010771a2546faaa3c34c89ff460ee1dbe9f`. Installation, manifest validation, and service activation remain separate.
### 2026-09-28 — bootstrap validates; support deployment rejects wrong source ownership

- Managed binary release `0.1.0-dev-34b1f9cb2ce2bb87241a40449fff97f3ef07b3ba` installed and selected successfully; all managed links are valid and shim/mkruntimed/mknetd report the exact revision.
- Root-owned kernel/bootstrap artifacts validate strictly. Candidate `gce-mk2.json` hashes to `a072a17f1fc12843e7df0b3d774222682439bc9ede1c59c74ec3fffd1500442f` and binds the exact kernel, initramfs, agent, relay, module, compatibility pins, transport direction, required config, and OCI feature set.
- `manage-runtime-deployment.py install` then refused the support deployment because the source extraction was owned by the ordinary build user (`unsafe deployment input: .../deploy/systemd/sys-fs-multikernel.mount`). This is expected fail-closed ownership enforcement. No support generation or service was activated; retry requires a separate root-owned extraction of the already verified archive.
### 2026-09-28 — exact deployment activation succeeds on clean replacement host

- Root-owned extraction of archive `ba10dcd…` installed immutable support generation `f895d655e5828c9d93588e363f2f752d92155f21a8c95cfa6367f33b44b69562`; all managed config/unit/CNI/containerd/libexec links inspect valid.
- Pre-activation inventory found empty default/moby tasks and containers, zero Docker objects, absent lifecycle/network journals, and an empty new runtime-storage tree. A first non-root `find` stopped on the intended mode-0750 storage directory and a stale assumption about a pre-mounted sysfs path; corrected privileged inspection established the clean state.
- Docker candidate validation passed, the runtime registration was installed atomically, the exact support generation was activated, and idle containerd was restarted. `sys-fs-multikernel.mount`, mkruntimed, mknetd, containerd, and Docker are all active. PIDs/restarts/status: mkruntimed `11452/0/0`, mknetd `11432/0/0`, containerd `11468/0/0`, Docker `2386/0/0`.
- Docker reports runtime `io.containerd.multikernel.v2`; the qualified storage validator prints `RUNTIME_STORAGE_MOUNT_VALID`. A literal containerd dump-header grep did not match and stopped that inspection command, but the format-independent retry completed the Docker/mount checks. Workload qualification remains pending.
### 2026-09-28 — live Docker host-namespace PID fix passes; exec exposes next boundary

- Exact runner SHA-256 remains `15e12e197160e2f3c287f381a9f7110a795b88d36225c670c3a254aff7f35658`. On exact revision `34b1f9c…`, all preflights passed, `ctr run` reached `RUNNING`, and Docker successfully created container `99d3dfa27452…` and immediately reported `running` while the ctr child remained alive. This is direct live proof that Docker can now bind/use the runtime's host-visible network namespace PID; the former `/proc/0/ns/net` failure is cleared.
- The unchanged suite then reached its first ctr exec and failed with `ctr: INTERNAL: guest process start failed at executable` for `/bin/sh -c ...`. The EXIT trap ran its Docker/ctr cleanup sequence. This is a new, deeper guest-root/executable boundary; no network-PID regression is implied. Independent cleanup/resource audit and exact rootfs diagnosis are pending before any retry.
### 2026-09-28 — post-failure audit finds host cleanup but retry-idempotency defects

- After the suite trap, both exact children, mknetd endpoints, host links/rules, and lifecycle sandboxes were gone; mkruntimed/mknetd/containerd/Docker remained stable with zero restarts. However, default and moby each retained a stopped task/container, two shim supervisor/worker pairs remained, and both prepared rootfs files remained.
- Normal non-force deletion of each exact stopped task fails identically: `stop agent relay: waitid: no child processes` plus `stop sandbox: NOT_FOUND: sandbox not found`. The first deletion attempt evidently completed the irreversible relay/sandbox teardown but returned before clearing its in-memory relay owner, treating an already-reaped child (`ECHILD`) and an already-absent sandbox as fatal on retry. The resulting error prevents rootfs cleanup, Task delete acknowledgement, shim shutdown, and container metadata removal.
- This is a retry-idempotency defect in the newly broadened cleanup path, distinct from the original guest `CloseNetwork` short-circuit. `terminateRelay` must accept an already-reaped child, and sandbox stop/delete retry must accept authenticated `NOT_FOUND` as completion before rootfs cleanup proceeds. No further workload will run until this exact stopped state is recoverable.
### 2026-09-28 — teardown retry-idempotency correction passes broad Go gates

- Daemon mutations now preserve the authenticated protocol error code in a typed error. Shim cleanup accepts only exact `NOT_FOUND` as terminal completion for stop/delete retries; unrelated daemon failures remain errors. Relay termination accepts both an already-reaped `exec.Cmd` and kernel `ECHILD`, while retaining other wait failures.
- The normal Delete path, fallback cleanup, and create rollback now share the typed absence rule. Focused tests prove mutation-code preservation and successful init deletion with an already-waited relay plus absent sandbox, in addition to the existing injected guest-network-failure test.
- Full `go test -race ./...`, `go vet ./...`, and `git diff --check` pass. Exact commit/build/live recovery of the two currently stopped tasks remains pending.
### 2026-09-28 — exact cleanup-recovery revision selected

- Cleanup retry correction is committed as `3fcccc903b92039e9efc1becb0d67166f5e7f10b` (`runtime: make task cleanup retry idempotent`). Its tracked-source archive `/tmp/mklinux-3fcccc9.tar.gz` hashes to `75dc0e7b99f6927bf51e97093415ba5eabd7bf7e321d872c8d433d9fd4dc98e6`. This is the exact candidate for recovering the two preserved stopped tasks; findings and the untracked historical evidence tree remain excluded.
### 2026-09-28 — exact `3fcccc9` recovery build selected on guest

- Guest verified archive `75dc0e7b…`, built all revision-stamped binaries, and selected immutable release `0.1.0-dev-3fcccc903b92039e9efc1becb0d67166f5e7f10b`. Exact hashes: release manifest `cf2f58ca488710778c78df1b085d57decfb98e496e6350737d9c584381771c2c`, shim `7964770a6ad1f1af76bb7902329096e56699b018c0a35b070c024b395779b8a1`, mkruntimed `9f66edbea322a5c7e21e33701419753dda4d39b6735873babfd02514db8dc772`, mknetd `17fcd2a22ed42a79a15cdeb2127506b32810d99b575b75ebb4647aa2c5b5661c`, agent `63190c5e569e2e60cf9892ccdb6b4d36c832f9adbb3f368c3251fe0f7c41d019`.
- Selection changes future shim/daemon launches only; the two preserved shim processes still execute immutable `34b1f9c…` paths. Their controlled termination and containerd fallback cleanup remain separate and must be audited exactly.
### 2026-09-28 — dead-shim fallback reaches a stricter absent-sandbox authorization boundary

- Both verified old shim process groups were terminated. The first multi-group kill syntax affected only default; a second command used explicit `-- -12170` for the independently revalidated moby process group. Containerd removed both task objects and invoked `/usr/local/bin/containerd-shim-multikernel-v2 ... delete`, resolving through the newly selected `3fcccc9…` release.
- Both fallback invocations refused with `daemon did not confirm cleanup ownership for the held bundle identity` because the old normal Delete attempts had already removed the lifecycle sandboxes. Containerd therefore retained both container metadata records, and the two exact rootfs records/files remain `PREPARED`; no shims, children, endpoints, host links/rules, or live sandbox records remain.
- The fallback currently conflates an absent sandbox (terminal cleanup already progressed) with a conflicting sandbox generation. Safe retry must reject a present same-ID mismatch but permit a truly absent sandbox to proceed using the root-owned bundle identity, validated persisted task identity/storage digest, and rootfs service's own exact directory identities. This is required to finish without manually editing or deleting state.
### 2026-09-28 — absent-sandbox fallback correction passes broad Go gates

- Fallback cleanup now scans daemon state by sandbox ID: a present same-ID generation or bundle-identity conflict is rejected, while true absence is allowed to continue through the already validated root-owned bundle/recovery identity. Rootfs cleanup still supplies exact task identity and storage digest to the rootfs service, which independently validates held directory identities.
- mknetd `RELEASE` now treats only authenticated `NOT_FOUND` as idempotent success and clears the recovered endpoint; malformed or other errors remain failures. Tests cover absent-sandbox rootfs cleanup call order, present identity conflict, and already-absent endpoint release.
- Full `go test -race ./...`, `go vet ./...`, and diff checking pass. Exact commit/build and a second fallback invocation against the preserved rootfs records remain pending.
### 2026-09-28 — exact absent-sandbox recovery revision selected

- Commit `7927f40ff2cf452a752225d1c6105f2802fb8faa` (`runtime: recover absent sandbox cleanup`) freezes the fallback correction. Its tracked-source archive hashes to `971b4bfd55686e63cd1427805cd47418771cc52b2bf4b753c3f969163cfa423c`. It is the only candidate authorized for the second identity-bound fallback invocation.

### 2026-09-28 — recovery archive independently verified on disposable guest

- The recreated guest independently hashes `/tmp/mklinux-7927f40.tar.gz` to `971b4bfd55686e63cd1427805cd47418771cc52b2bf4b753c3f969163cfa423c`, exactly matching the selected local tracked-source archive. No extraction, build, selection, or cleanup claim is implied yet.
- A subsequent isolated-build command created the unique empty directory `/var/tmp/mklinux-build-7927f40ff2cf452a752225d1c6105f2802fb8faa` but stopped before extraction because the verified `/tmp` archive was no longer present. This is transfer-path volatility, not a source build result. The retry must remove only that verified-empty directory and stage the same archive in durable `/var/tmp` before re-verifying it.

### 2026-09-28 — user restart changes the preserved recovery baseline

- The user confirmed an instance restart, explaining the transient `/tmp` loss. The new boot ID is `162500c3-5973-4d8d-b95b-098e28df2f10`; the restaged durable archive at `/var/tmp/mklinux-7927f40.tar.gz` independently hashes to the selected `971b4bfd…`, and the unique extraction directory is verified empty.
- All four services are active after reboot: mkruntimed PID 1470 (`NRestarts=1`), mknetd PID 1201, containerd PID 1341, and Docker PID 1472 (the latter three `NRestarts=0`). Default and moby task inventories are empty, while both exact container metadata records remain. No Multikernel child exists.
- Unlike the pre-restart snapshot, `/srv/multikernel-storage/runtime` now inventories empty while mkruntimed rootfs/storage state files and mknetd state still exist. This is a changed recovery state, not proof that the new fallback performed cleanup: revision `7927f40…` has not been built or invoked. State contents, mount ordering, and boot journal require inspection before any recovery claim.
- The retained disk is correctly mounted from `/dev/sdb` at `/srv/multikernel-storage` as ext4 `rw,nosuid,nodev`; the empty inventory is therefore not a hidden boot-filesystem view. Rootfs state (`aa610942…`) is version 4 with no records, network state (`7aa7fc47…`) has no endpoints, and storage state (`8ecd2b72…`) retains both exact exports as `RELEASED` with their earlier `00:58:22Z`/`00:58:23Z` release times. Thus the files and rootfs ownership were cleared before this reboot, while release audit records remain.
- mkruntimed's one current-boot restart is independently explained by host qualification racing the Google guest agent: PID 1009 failed `GUEST_AGENT` at `11:57:08Z`; systemd restarted it once at `11:57:12Z`, after which PID 1470 remained active. The transport `File exists` message on restart is benign module idempotency, not the cause of the first failure. No cleanup action from `7927f40…` can now be live-proved against those two rootfs records because the reboot baseline contains none.
- The first durable extraction command unpacked the archive successfully but produced no build artifacts. Direct inspection proves the tree is present and `runtime/bin`/release manifest are absent. The command stopped at `scripts/verify-host.sh`, whose default expected instance name is `mklinux-lab`; this replacement is named `mklinux-g4-g6-final-20260905`. The retry must supply the explicit instance identity, and no build success is claimed from the silent compound command.

### 2026-09-28 — exact `7927f40` guest build succeeds after explicit identity

- With `INSTANCE=mklinux-g4-g6-final-20260905`, host verification passes on kernel `7.0.0-mk2-gce-lab`, CPUs `0-15`, mounted Multikernel filesystem, and guest address `10.148.0.58/32`. The guest then built all seven static binaries stamped with exact revision `7927f40ff2cf452a752225d1c6105f2802fb8faa` and emitted a validated release manifest.
- Exact hashes are release manifest `580e2306e4a35e542bf6ae22ab122c72baeaad8868898277d54edfe89603292b`, shim `867872716e187565ea8c977cee46d5fa903ac2327dc0c785953cf2284b7c1ba7`, mkruntimed `b06c25909a80a2e2c1a65085d9479248929a2f14201f4e56142fae8ea87a9334`, mknetd `66043fe135e4215bd15f37387f11582503a7d9c4bd275078dd8092dc24e1528f`, and agent `5403c1d475f0d9cfaae558ac010a27cdc6d40be2cf70bc71bc22cb9d87dd2c15`. Installation/selection and any live cleanup behavior remain separate claims.

### 2026-09-28 — exact `7927f40` managed release selected

- Managed inspection first showed active `3fcccc9…` with the `34b1f9c…` and `3fcccc9…` immutable releases present. Install then returned `installed_and_active` for `0.1.0-dev-7927f40ff2cf452a752225d1c6105f2802fb8faa`; post-install inspection shows all managed links valid and the three immutable releases retained.
- The public shim, mkruntimed, and mknetd entry points each report exact revision `7927f40ff2cf452a752225d1c6105f2802fb8faa`. This selects future launches only; running daemon PIDs were not restarted, and the reboot had already removed the rootfs records needed for the originally planned fallback replay.
- The first metadata-removal command stopped before either removal because this Docker version's `docker ps` formatter does not expose `.Runtime`. With `set -e`, the unsupported read-only format field terminated the sequence; both exact metadata records remain unchanged for a corrected inspection/removal.
- Corrected Docker inventory was empty. The default `mk-proof-ctr` container metadata removal succeeded; Docker then returned `No such container` for the orphaned moby ID and stopped the sequence before the after-audit. The moby record belongs only to containerd metadata after reboot and requires exact `ctr -n moby containers rm`, followed by a full empty-state audit.
- Exact moby `ctr` removal then succeeded. Default/moby task and container tables plus Docker inventory are all empty. The broader audit stopped at a stale `/sys/kernel/multikernel/children` pathname, which does not exist on this boot; the mounted ABI exposes instances below `/sys/fs/multikernel`. Storage/state/process/network/service checks after that point were not executed and must be rerun with the live ABI path.

### 2026-09-28 — post-restart qualification baseline is resource-clean

- Corrected audit finds no Multikernel instances, runtime-storage entries, rootfs records, network endpoints, Task v2 bundle directories, managed network namespaces, runtime links, runtime firewall rules, or shim/relay/storage-server processes. Together with the already empty default/moby/Docker inventories, the qualification resource baseline is clean.
- mkruntimed/mknetd/containerd/Docker remain active as PIDs 1470/1201/1341/1472, with restart counts 1/0/0/0 and exit status 0; the sole mkruntimed restart is the separately recorded guest-agent boot race. The managed release link resolves to exact `7927f40…`. Non-root `/proc/<pid>/exe` resolution was inconclusive for the two daemons, so their running binary identity remains unproved until privileged inspection or coordinated restart.

### 2026-09-28 — clean services activated onto exact `7927f40`

- Privileged `/proc/<pid>/exe` inspection proves pre-restart mkruntimed PID 1470 and mknetd PID 1201 still executed immutable `3fcccc9…`, as expected from selection without restart. With the complete resource-clean preflight recorded, both services were restarted normally.
- New mkruntimed PID 8100 and mknetd PID 8079 each resolve to their immutable `7927f40…` release binaries, report exact revision `7927f40ff2cf452a752225d1c6105f2802fb8faa`, and are active with `NRestarts=0`/status 0. This activates the host daemons and future shim launches; the active kernel manifest still intentionally supplies the previously qualified `34b1f9c…` guest agent/initramfs because these fallback-only changes do not modify that guest payload.

### 2026-09-28 — cached image disproves the `/bin/sh` symlink hypothesis

- A temporary read-only containerd image mount of exact cached BusyBox 1.36 snapshot `sha256:97e4ece8…` shows `/bin/sh`, `/bin/sleep`, and `/bin/busybox` are regular mode-0755 hardlinks sharing inode 6035832 with link count 405—not symlinks. The executable is x86-64 ELF and hashes to `f060103f9d9c62ab124afbe4017e444b3d9082d734deb6c3a7b9949b093c3688`. The temporary image view was unmounted and removed by the command trap.
- Therefore the prior exec-stage `executable` failure cannot be explained by a BusyBox `/bin/sh` symlink rejected by the agent. Since `/bin/sleep` from the same inode successfully started as init, the next live experiment must inspect the materialized rootfs identity/availability at exec time and compare direct `/bin/busybox` versus `/bin/sh` without assuming image content is at fault.

### 2026-09-28 — isolated live exec no longer reproduces the executable failure

- On activated `7927f40…`, bounded ctr task `mk-exec-diag` reached `RUNNING` as host PID 8722. Its exact owned rootfs was `/srv/multikernel-storage/runtime/task-79975f2a9e59bbed11b593b52b48b207/root.ext4`.
- While the NBD-backed filesystem was live in the child, host `debugfs` could not resolve either `/bin/sh` or `/bin/sleep`; that read does not observe the guest's live mounted/cache state and is not valid evidence of absence. In contrast, guest-mediated execs of `/bin/busybox echo`, `/bin/sh -c`, and `/bin/sleep 0` all returned rc 0 and produced the expected `BUSYBOX_EXEC_OK`/`SH_EXEC_OK` output.
- Normal exact cleanup then left the default task/container inventories, runtime-storage tree, Multikernel instance inventory, and mknetd endpoints empty. The previously observed `/bin/sh` failure is therefore not reproducible in an isolated ctr task on the restarted clean host. Concurrent ctr+Docker qualification is still required to determine whether the former failure was stale-state-specific or remains concurrency-dependent.

### 2026-09-28 — unchanged concurrent suite reproduces a two-child rootfs/exec defect

- Local and guest copies of `scripts/test-runtime-g4-g6.sh` both hash to the unchanged `15e12e197160e2f3c287f381a9f7110a795b88d36225c670c3a254aff7f35658`. Preflights passed on boot `162500c3…`; ctr reached `RUNNING`, then Docker container `42fad574395d…` also reached `running` under `io.containerd.multikernel.v2`.
- Immediately after the second child became live, the ctr child's first `/bin/sh -c` exec again failed `INTERNAL: guest process start failed at executable`, and the runner invoked its cleanup trap. This reproduces the prior boundary on a clean reboot with exact activated host revision `7927f40…`, while the isolated ctr experiment on the same boot had executed `/bin/sh` successfully.
- Combined with the image hardlink evidence, the failure is specifically correlated with concurrent second-child activation, not the `/bin/sh` file type and not stale pre-reboot task state. Cleanup completeness and storage/export state must be audited next; then a controlled two-child experiment must compare all executable paths and the first child's storage service before/after the second start.

### 2026-09-28 — concurrent-failure cleanup remains complete

- The suite trap leaves default/moby task and container inventories, Docker inventory, Multikernel instances, runtime-storage files, rootfs records, network endpoints, and shim/relay/storage-server processes empty. mkruntimed/mknetd/containerd/Docker remain active at the same PIDs with zero post-activation restarts and no warning-or-higher journal entries in the run window.
- Durable storage audit records both failed-run exports as cleanly `RELEASED`: Docker's export used port 4062 and ctr's used 4061, with offline e2fsck-clean attestations and nonzero read/write/flush counters. The earlier isolated ctr run also released cleanly on 4061. Cleanup is therefore not implicated in this reproduction; live ownership and liveness of the first export while the second child starts remain the missing evidence.

### 2026-09-28 — two-child probe localizes failure to vanished first storage server

- Controlled ctr child `mk-mk-two-ctr-b86b3f0bf5d26cfc` executed `/bin/sh` successfully before Docker started. Its port-4061 export generation `65afa602…` was journaled `ACTIVE` with a live generation record.
- After Docker container `0d16fccdabc8…` and its second child reached running, all three first-child exec attempts—direct `/bin/busybox`, `/bin/sh`, and `/bin/sleep`—failed at `executable`; the first child itself still reported `active`. Crucially, process inventory showed only Docker's port-4062 `mkvsock-nbd` PID 11423. The ctr port-4061 server had vanished even though durable storage still reported that export `ACTIVE` and its `/run/mkstorage` JSON record remained.
- This directly explains executable validation: the first child loses live access to its NBD root when the second child starts, while its lifecycle/storage projections falsely remain active. It is not shell-specific. The attempted Kerf console attach returned `Kernel image not loaded for instance ... (ID: 40)`, so it supplied no guest log. The command's unprivileged shell also failed to expand root-only storage-log globs; exact retained log reads remain pending.
- The command's final exact cleanup nevertheless emptied both namespaces, runtime storage, and the child inventory. The next source target is the transport/storage-server lifetime boundary that lets a second child sever the first server without updating the active lease.

### 2026-09-28 — first export exits through a clean client-side close

- Correct privileged read of the exact ctr generation log shows `MKNBD_SERVER_READY`, one `MKNBD_SERVER_CLIENT_ACCEPTED`, then canonical `MKNBD_SERVER_CLOSED synced=1` with 272 reads, 10,420,224 read bytes, 18 writes, 282,624 write bytes, and four flushes. The server did not crash or report an I/O/protocol error; its sole client connection closed and the server finalized successfully.
- Docker's exact generation log has the same ready/accepted/canonical-close shape when later cleaned. Host kernel records show both instances became active and were removed only by the diagnostic cleanup. Thus second-child activation causes the first child's mediated-storage connection to close cleanly underneath an otherwise active child, while mkruntimed fails to observe/update the now-dead `ACTIVE` export until teardown.
- This moves the primary cause below executable/rootfs construction into multi-client transport connection isolation or guest NBD connection lifetime. The transport implementation must be inspected for singleton/global connection ownership before changing the agent or executable validator.

### 2026-09-28 — source trace identifies idle socket timeout as the exact cause

- The pinned transport module routes sockets by the full vsock connected/bound lookup and retains per-socket queues; it has no singleton that intentionally replaces an earlier connection. The failure is instead in `tools/mkvsock-nbd.c`: `vsock_socket()` installs 15-second receive/send socket timeouts, the accepted server socket receives the same timeouts, and the post-handshake request loop treats `EAGAIN`/`EWOULDBLOCK` from an idle header read as a terminal close.
- Docker creation takes substantially longer than 15 seconds while the already-running ctr workload performs no root I/O. The port-4061 server therefore exits through its clean final-sync path exactly as observed; subsequent executable validation cannot stat the first root. The immediate isolated execs complete before this idle deadline, explaining their success without invoking any cross-child routing theory.
- The authentication handshake should remain bounded, but after the hello identities match, both endpoints must clear `SO_RCVTIMEO`/`SO_SNDTIMEO` for the long-lived NBD stream. The guest's existing `NBD_SET_TIMEOUT=15` still bounds an active block request that stops making progress; it must not be conflated with permission to destroy a healthy idle root disk.

### 2026-09-28 — post-handshake timeout correction implemented

- `mkvsock-nbd` now clears both receive and send socket timeouts on server and client only after the authenticated hello exchange succeeds. Handshake timeout and identity refusal remain unchanged; the guest still configures the kernel NBD active-request timeout. The production helper compiles with `-O2 -Wall -Wextra -Werror`, runs its usage path, and passes diff checking.
- A focused socket-option regression probe was added. Its first local execution reached the environment's socket-option boundary and returned `EPERM` before the production helper call, matching the known local permission-gated socket class rather than testing the correction. The probe now explicitly skips only that exact preflight restriction; an unskipped disposable-guest pass and a live idle-over-15-seconds two-child replay are mandatory.
- The adjusted local probe now emits only `MKVSOCK_NBD_POST_HANDSHAKE_TIMEOUT_CLEAR_SKIP reason=EPERM`. Production C compilation, `go test -race ./...`, `go vet ./...`, the complete documentation/schema/evidence/OCI/bind/bootstrap/storage/image/release/deployment/ledger/capture/containerd/final-audit chain, shell syntax, and diff checking all pass. The known socket-permission subcase is likewise the sole skip in the repository documentation gate.

### 2026-09-28 — exact idle-root correction revision selected

- Commit `b9fe085` (`runtime: preserve idle mediated root sessions`) contains only the NBD helper correction and its focused probe. Its tracked-source archive `/tmp/mklinux-b9fe085.tar.gz` is 1,012,019 bytes and hashes to `2f3a7dc41a546a1e7155e22f9d151b63b2534f1855cd1287e599505a99fd2691`; continuously updated findings and the pre-existing untracked evidence tree are excluded. This is the sole candidate for guest build/deployment/replay.

### 2026-09-28 — guest independently passes post-handshake timeout probe

- The disposable guest independently verified durable archive SHA-256 `2f3a7dc41a546a1e7155e22f9d151b63b2534f1855cd1287e599505a99fd2691`, extracted it into fresh `/var/tmp/mklinux-build-b9fe085`, and emitted `MKVSOCK_NBD_POST_HANDSHAKE_TIMEOUT_CLEAR_PASS` without the local skip. A warning-clean dynamic diagnostic helper built there as SHA-256 `299b9b116ae53061baf51def8c687e1ab80153367390224f32ffaeba7e70ea71`.
- This proves the socket options transition from 15 seconds to zero on the guest kernel; it is not yet the static production-helper identity, initramfs/deployment activation, or live NBD idle-survival result.

### 2026-09-28 — exact `b9fe085` release and static helper build succeed

- Explicit-identity host verification passed again. The guest built all seven binaries stamped `b9fe085c7adad374f310dd5fd409b29bd725483a` plus a warning-clean statically linked x86-64 NBD helper from the selected source.
- Exact hashes: release manifest `d4800b4d9590df9531a98ec53506d3f889500223b2a9b8bd58e1c274ef86bfe5`, shim `639f0d28d314ad2894b8286e8fa6ca024f9b089223802ef007e39dcf1c60ee3e`, mkruntimed `102bf3512f9b5849f3baeeb13f6bf36792720a29c6413f6bc2b2926bbfa9d42e`, mknetd `7e22b58d528547d21610323ab07a65fbcdf8d6f9662a034737e512fd19788c2c`, agent `50dd8e74e4719230a71d0d62ede7b3fa47a58300d0cc17540639b65579159f94`, and static NBD helper `95e886d625a33de702989907f442f98a1812d315f2c6a4195214d60852be43ac`.
- These are coherent build inputs only. Managed binary installation, child initramfs assembly, kernel-manifest validation, support deployment, and activation remain unclaimed.
- Agent initramfs assembly then passed gzip/listing checks and contains exact `init`, `mk-agent`, `mk_transport.ko`, and `mkvsock-relay` entries. It hashes to `440a0d965a00157a3c8eb05f5ddc1e398e0f8482894534fd0c5d104903038fe5` and binds agent `50dd8e74…`, unchanged pinned module `bef1b888…`, and relay `293ff1ea…`. Root-owned staging, candidate manifest validation, and activation remain separate.

### 2026-09-28 — root-owned `b9fe085` candidate manifest validates

- Exact agent, initramfs, and NBD helper were staged as root-owned regular artifacts under immutable release directory `/opt/mkruntime/artifacts/releases/b9fe085c7adad374f310dd5fd409b29bd725483a`. Their hashes re-match `50dd8e74…`, `440a0d96…`, and `95e886d6…`.
- Candidate `gce-mk2.json` hashes to `1b31a8181b02c0b972b1c36e74dee8601637f490557d30c38190df02a0134acd`. The deployed root-owned strict validator resolves the exact agent/initramfs plus unchanged pinned kernel, module, relay, compatibility, required configuration, protocol, transport direction, and OCI feature set. Nothing has been activated yet.
- Activation preflight on unchanged boot `162500c3…` is empty across both namespaces' tasks/containers, Docker objects, Multikernel instances, runtime-storage entries, rootfs records, network endpoints, and exact shim/relay/NBD processes. Existing mkruntimed/mknetd/containerd/Docker PIDs 8100/8079/1341/1472 are active with zero restarts/status 0. Atomic activation is now permitted but not yet claimed.

### 2026-09-28 — exact idle-root correction activates coherently

- The old helper was preserved root-only at `/opt/mkruntime/artifacts/mkvsock-nbd-a0259098bba0a431`. Managed release `0.1.0-dev-b9fe085c7adad374f310dd5fd409b29bd725483a` installed/selected, then stopped daemons received atomic same-directory replacements of the regular NBD helper and validated kernel manifest.
- Active manifest/helper re-hash to `1b31a818…`/`95e886d6…`; strict validation resolves the exact `b9fe085…` agent/initramfs and all pinned dependencies. Public shim/mkruntimed/mknetd report the full revision. New daemon PIDs 14857/14838 execute immutable `b9fe085…` binaries and are active with zero restarts/status 0; containerd/Docker PIDs remain healthy and unchanged. Boot ID remains `162500c3…`.
- This establishes coherent activation only. A deliberate idle interval longer than 15 seconds with both children alive must now prove the server remains present and exec works before the unchanged suite is rerun.

### 2026-09-28 — live idle-root survival correction passes decisively

- ctr task `mk-idle-ctr` executed before idling. Its port-4061 server PID 15163 ran exact helper hash `95e886d625a33de702989907f442f98a1812d315f2c6a4195214d60852be43ac`, survived an explicit 20-second idle interval with the same PID, and the child executed `/bin/sh` successfully afterward.
- Docker startup then occupied 57 seconds (`1790598818` to `1790598875`) while the first root was again idle. PID 15163 remained identical; Docker added independent port-4062 server PID 15441. Direct first-child `/bin/busybox`, `/bin/sh`, and `/bin/sleep` execs all returned rc 0 after Docker became live, and durable state showed both exact exports `ACTIVE` on their distinct ports/generations.
- Exact normal cleanup left default/moby task/container tables, Docker, runtime storage, Multikernel instances, rootfs records, and network endpoints empty. This directly live-proves that clearing post-handshake socket timeouts preserves an idle mediated root beyond both the former 15-second deadline and concurrent second-child startup, without regressing cleanup. The unchanged full qualification suite is next.

### 2026-09-28 — unchanged suite clears idle-root failure and exposes Docker exec compatibility gap

- Exact unchanged runner hash `15e12e19…` passed preflights, started ctr and Docker concurrently, and the ctr network exec now succeeded after the long Docker start. It returned distinct child boot ID `2dc16095-2dd1-4f3d-a484-c1e3aea9475b`, live `mkn0` address `172.31.0.2/30`, successful outbound HTTP, and `CTR_NETWORK_PASS`. This is direct suite-level confirmation that the idle-root defect is fixed.
- The next Docker exec did not enter the guest: Docker printed `not implemented: unsupported exec process field`, and the suite trap ran. The shim's `validateExecProcess` still rejects several OCI fields wholesale, while the already-qualified Docker init adapter accepts only explicit `apparmor=unconfined` and integer `oomScoreAdj=0` as safe no-op values. Exact Docker exec field/value capture and cleanup audit are required before extending the same narrow contract to exec.
- Post-failure audit is fully empty across default/moby tasks and containers, Docker, Multikernel instances, runtime storage, rootfs records, network endpoints, and exact shim/relay/NBD processes. All four service PIDs remain unchanged, active, zero-restart/status 0, with no warning-or-higher entries in the run window. This is an exec-validation compatibility failure, not cleanup or service instability.

### 2026-09-28 — Docker exec no-op fields receive the same narrow contract

- Exec validation now accepts only `apparmorProfile="unconfined"` and integer `oomScoreAdj=0`, omitting them from the child projection exactly as the already-live-qualified init adapter does. Any named AppArmor profile, nonzero OOM adjustment, or other previously unsupported exec field remains `NOT_IMPLEMENTED`; fixed field names are included in the error without echoing user values.
- Focused tests prove the exact Docker pair passes without altering the guest process projection and that a requested profile/nonzero adjustment plus every other unsupported field still fails closed. The focused test and complete shim package pass under the race detector; diff checking is clean. Full-tree gates and live Docker exec remain pending.
- Full `go test -race ./...`, `go vet ./...`, and the complete documentation/schema/evidence/OCI/bind/bootstrap/storage/image/release/deployment/ledger/capture/containerd/final-audit chain pass. The focused NBD probe and existing socket subcase classify only the known local `EPERM` restriction; generated Python cache was removed and diff checking remains clean. Exact commit/build/live replay are next.

### 2026-09-28 — exact Docker exec compatibility revision selected

- Commit `aa1238b83a06ecac63f1d3786993e201556767e6` (`runtime: accept Docker exec opt-out fields`) freezes the narrow exec correction atop the live-proved idle-root fix. Its 1,012,377-byte tracked-source archive hashes to `4f16e301fda9825a80b064511260cbe362ad030144b8faba8ba1d133e52a0efb`. Findings and the pre-existing untracked evidence tree remain excluded.

### 2026-09-28 — exact `aa1238b` coherent guest build succeeds

- The guest independently verified archive `4f16e301…`, passed host verification, and built the complete release stamped `aa1238b83a06ecac63f1d3786993e201556767e6`. Exact hashes: release manifest `6f67cfe1394db931b3be3abc7c0cb45514ba6770da82f2f7fd411150ca6d579e`, shim `18e3a0133f0a0b669d880a87d72b1a21db74678906bc8b48ce758f61a0a657c5`, mkruntimed `fe3fe773bb586a0ee3b89d6120b48d2dde188b2891bf76140814be0a68740582`, mknetd `9cff4985d4e1e72527a24dc99775fcfa8dc0b5903820d5bed39142374513bb58`, and agent `cf73a7e35aadb073b352c696926614a1a814ff32261c55a9af75ba6d7d05c422`.
- Static helper exactly reproduces live-proved `95e886d6…`; dependent gzip-valid agent initramfs hashes to `51e0c02e1579006b4daa2ecfe49853efaace01329a4c436289c78c5e7d65797a`. Root-owned staging/manifest validation/activation remain pending.
- Root-owned immutable staging re-matches the agent/initramfs/helper hashes. Strict validator accepts candidate manifest `4627d38bb090578561e68928b5dfb924f8a4d157986f5e27125e2d4b96039d46` with unchanged pinned kernel/module/relay and exact feature contract. Activation remains unclaimed.

### 2026-09-28 — exact `aa1238b` release activates coherently after restart

- Empty-state activation preflight passed, then managed release `0.1.0-dev-aa1238b83a06ecac63f1d3786993e201556767e6` installed and selected atomically. Active manifest/helper re-hash to `4627d38bb090578561e68928b5dfb924f8a4d157986f5e27125e2d4b96039d46`/`95e886d625a33de702989907f442f98a1812d315f2c6a4195214d60852be43ac`.
- Public shim, mkruntimed, and mknetd report exact revision `aa1238b83a06ecac63f1d3786993e201556767e6`. New daemon PIDs 19891/19872 execute the immutable `aa1238b…` release binaries and are active with zero restarts/status 0; containerd/Docker PIDs 1341/1472 remain unchanged and healthy.
- The restarted host remains on boot ID `162500c3-5973-4d8d-b95b-098e28df2f10`. This establishes coherent activation only; the unchanged live qualification suite must prove the Docker exec correction and complete lifecycle behavior.

### 2026-09-28 — qualification invocation guard prevents a root-run false claim

- The pinned runner hash matched, but an initial `sudo bash` invocation exited at its explicit ordinary-user guard before preflight or workload creation. Its cleanup trap found nothing to remove. This is not a suite result; the runner must be invoked as the SSH user because it performs narrowly scoped `sudo` operations internally.

### 2026-09-28 — unchanged live G4–G6 qualification passes end to end

- Unmodified runner SHA-256 `15e12e197160e2f3c287f381a9f7110a795b88d36225c670c3a254aff7f35658` passed on exact active revision `aa1238b…`. ctr child boot ID `a7d02acc-25db-4448-aae7-8400d6e4f794` and Docker child boot ID `05157cb3-a5bf-4b65-bd06-ce7a527e2347` are distinct from each other and host boot `162500c3-5973-4d8d-b95b-098e28df2f10`.
- ctr and Docker exec both entered their children, observed independent `mkn0` addresses `172.31.0.2/30` and `172.31.0.6/30`, completed outbound HTTP, and emitted `CTR_NETWORK_PASS`/`DOCKER_NETWORK_PASS`. Direct cross-child ping was blocked and the runner emitted `CROSS_SANDBOX_ISOLATION_PASS`; subsequent `/bin/echo` execs emitted `CTR_EXEC_PASS` and `DOCKER_EXEC_PASS`.
- SIGKILL produced expected ctr `STOPPED` and Docker `exited`/137 states. Normal removal emptied Multikernel instances, temporary links/rules, default namespace tasks/containers, and the named Docker object while preserving the host boot ID. The unchanged runner exited 0 with `DISTINCT_CHILD_KERNEL_BOOT_IDS_PASS`, `CTR_DOCKER_LIFECYCLE_PASS`, `PRIMARY_MEDIATED_NETWORK_PASS`, and `G4_G5_G6_MVP_PROOF_PASS`. An independent post-run inventory/service audit remains required before treating cleanup and stability as fully corroborated.

### 2026-09-28 — independent post-pass audit corroborates cleanup and health

- Independent commands find empty default and moby task/container inventories, no Docker objects, no Multikernel instances, no `mkv*` link, no MK firewall/NAT rule, no shim/relay/NBD process, empty rootfs records, and an empty mknetd endpoint map. `/srv/multikernel-storage/runtime` contains no task artifact; retained quarantine/fixture directories are pre-existing evidence assets rather than active runtime ownership.
- Storage history intentionally retains only `RELEASED` records; the passing run's ctr and Docker exports are both released with offline-check/counter evidence and no live backing path. Historical applied configfs transaction nodes remain kernel audit history, while `/sys/fs/multikernel/instances` is empty. The host reports all 16 CPUs available and `MemAvailable` 64,470,928 kB after cleanup.
- mkruntimed/mknetd/containerd/Docker remain active/running at PIDs 19891/19872/1341/1472 with `NRestarts=0` and `ExecMainStatus=0`; their warning-or-higher journal slice from before this run is empty. This closes the independent cleanup/service-health qualification for the unchanged end-to-end suite.

### 2026-09-28 — gate-closure audit keeps G4–G6 open beyond the MVP pass

- The end-to-end MVP pass does not satisfy the full evidence contract. `g4-g6-evidence-requirements-v1.json` still requires 10 G4, 8 G5, and 14 G6 assertion groups, including storage fault/recovery, CNI/fault/policy matrices, restart/reconnect, FIFO/deadline, event, and churn coverage absent from the basic runner.
- The replacement evidence bundle currently contains only `resources-before.json`; it has no per-gate manifests, command transcripts, `resources-after.json`, or immutable hash index and therefore cannot pass `audit-g4-g6-evidence.py`. No gate-closure claim is made from the basic suite.
- The next bounded step is to run the repository's fuller shared ctr/Docker feature matrix on exact `aa1238b…`, preserving each observed boundary and independent cleanup result. Evidence packaging remains pending until the complete required assertion set has been executed rather than inferred.

### 2026-09-28 — exact fuller feature matrix begins from a clean host

- Exact matrix SHA-256 `66bfbe6545ed97b03bafb108e5998006d134405b3a8aa01c938a8e6953212d0b` passed its service/mount/device preflight on host kernel `7.0.0-mk2-gce-lab`, boot `162500c3…`, and active `aa1238b…`. Initial inventory is explicitly `children=0 links=0 nat_rules=0 filter_rules=0 ctr_tasks=0 docker_containers=0`.
- Image inspection records BusyBox OCI index/repository digest `sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662`, Docker architecture `amd64`, and the advertised multi-platform index. The `image-pull-and-inspect` row passed for both clients; split ctr task creation is now in progress.

### 2026-09-28 — fuller matrix exposes Docker generated-bind contract gap

- Split ctr create/start succeeded, but Docker start failed closed before its child allocation: `OCI configuration rejected: read-only bind '/etc/resolv.conf' differs from the enforced option contract`. The fuller matrix does not use the basic runner's container-wide `--read-only`, so Docker generated a different read-only bind option set than the currently admitted exact contract.
- The EXIT trap killed/removed the ctr task and removed Docker metadata. This is a new OCI adapter compatibility boundary, not a matrix pass. Exact generated mount options and an independent post-failure inventory must be captured before deciding whether the variant is semantically equivalent and safe to admit.
- Independent audit confirms empty default/moby task and container inventories, no Docker object, child, endpoint, rootfs record, active export, runtime artifact, `mkv*` link, MK rule, or shim/relay/NBD process. All four service PIDs are unchanged, active, zero-restart/status 0. Failure rollback is complete.
- An equivalent short-lived stock-runc Docker container exposes the exact generated OCI mounts: `/etc/resolv.conf`, `/etc/hostname`, and `/etc/hosts` each bind from `/var/lib/docker/containers/<same-container-id>/…` with options exactly `["rbind","rprivate"]`. Absence of `ro`/`rro` is intentional writable-container-root behavior. Generic writable host binds must remain rejected; compatibility requires a narrow identity-bound copy of only these Docker-managed metadata files into the private child root, with no host write-through.

### 2026-09-28 — Docker metadata binds become identity-bound private seeds

- OCI admission now recognizes only the three exact Docker managed destinations, exact `/var/lib/docker/containers/<64-hex-id>/<matching-name>` sources, exact `["rbind","rprivate"]` options, and an identical ID in `/var/lib/docker/rootfs/overlayfs/<id>`. They are descriptor-held copies into the already-private root and are omitted from the guest mount projection; arbitrary writable binds remain rejected.
- Materialization marks these inputs `private-writable-seed-copy`, rejects directory seeds, verifies before/source-after/copied manifests, preserves numeric metadata, and proves that guest-side mutation of the copy cannot write through to the host source. The privileged Go verifier independently rechecks source/root/destination identity before accepting retained provenance.
- Focused materialization passes, the OCI boundary expands to 90 passing semantic cases with mismatched IDs/destinations/options rejected, and the rootfs verifier package passes under `-race`. Full-tree gates and exact guest replay remain pending.
- Full `go test -race ./...`, `go vet ./...`, and the complete documentation/schema/evidence/OCI/bind/bootstrap/rootfs/storage/image/release/deployment/ledger/capture/containerd/final-audit chain pass. The only skip is the known sandbox-local socket `EPERM` subcase. Exact commit/build/deployment/live replay remain separate.

### 2026-09-29 — exact Docker private-seed correction selected

- Commit `1832e62ad6c54128f0fcd25df6ce7ed16607b3e0` (`runtime: seed Docker managed metadata privately`) freezes only the tested source and tests; the continuously updated findings and pre-existing evidence tree remain outside the commit. Its 1,014,109-byte tracked-source archive hashes to `6c6f0b6f606e07a05917097a31f01e2a543b26324d7162c6c6b9c59a94b419eb`.
- This is source identity only. Independent guest verification/build, root-owned staging, activation, and live fuller-matrix replay remain unclaimed.

### 2026-09-29 — exact `1832e62` guest build succeeds; unneeded helper rebuild excluded

- The restarted guest independently re-hashed archive `6c6f0b6f…`, passed explicit-instance host verification on `7.0.0-mk2-gce-lab` with CPUs 0–15, and built all seven components stamped `1832e62ad6c54128f0fcd25df6ce7ed16607b3e0`. Release manifest/shim/mkruntimed/mknetd/agent hashes are `d3b4eab8eea5b880c514c31a7993b53db4d0a9b3a194dbdbcf2991db480dea06`, `12f01ba7ba93cbf1ebc078d08a0fc40244c8fce8e3763227a0e9812227dc684b`, `2f56955eb88eac1eadbce112567a58711a9b99b4d83b7325b336d8a9b232e80a`, `9cd72078aec3a4f601dcdbb6cbb49562e1e04f75464a279b835aaf57a6075c93`, and `23acdcad2079828af8d307fba8fb763e079abdfa1d0bbb46139e69872820ce4b`.
- The dependent agent initramfs hashes to `44a9e493a0577df49ca1669ace80544470d99acfddcf7c1e59d9a3c273518228`. An ad-hoc helper rebuild hashes to `fba35ea4…`, not the already-live-proved `95e886d6…`; because this revision does not modify helper source, that unneeded rebuild is explicitly excluded and activation will preserve the existing exact helper. Root-owned staging/validation remain separate.

### 2026-09-29 — support staging ownership guard stops partial selection safely

- The binary manager installed and selected immutable release `0.1.0-dev-1832e62ad6c54128f0fcd25df6ce7ed16607b3e0`; existing daemons continue executing their old immutable `aa1238b…` paths until coordinated restart. The deployment manager then refused the user-owned extracted support tree as `unsafe deployment input` before installing anything.
- This is not coherent activation: public binary links select `1832e62…`, running daemon processes/support/kernel manifest remain the prior generation, and no new workload will run in this state. A root-owned source copy, completed support/artifact staging, strict validation, empty-state preflight, and coordinated restart are required next.

### 2026-09-29 — root-owned `1832e62` candidate validates

- A fresh root-owned tracked-source copy installed/selected immutable support generation `809c5033ca4ac7a023e9d29df0e36dfd20f927d740b19aa24a22717662d4074e`. Exact agent/initramfs were staged under `/opt/mkruntime/artifacts/releases/1832e62ad6c54128f0fcd25df6ce7ed16607b3e0` with candidate manifest hash `3f2300d68070d7ca333ecd9711c9ef286cbf78a2b9da976f6e376a292163ab42`.
- Root-run strict bootstrap validation resolves the exact candidate agent/initramfs and unchanged pinned kernel/module/relay plus feature contract. A subsequent ordinary-user `sha256sum` stopped on the intentionally mode-0600 initramfs; this does not invalidate the preceding privileged validation, but privileged re-hash and manager inspection remain required before activation.
- Privileged re-hash exactly matches manifest/agent/initramfs `3f2300d6…`/`23acdcad…`/`44a9e493…`; binary and deployment managers report selected immutable generations `1832e62…` and `809c5033…` with all links managed. A combined preflight then stopped at a mistyped daemon CLI path (`/usr/local/bin` rather than managed `/usr/local/sbin`) before inventory checks. No empty-state or activation claim is made from that procedural stop.

### 2026-09-29 — restarted-host preflight is empty but refreshes health baseline

- Corrected preflight confirms public shim/mkruntimed/mknetd links report `1832e62…` and finds no default/moby task or container, Docker object, Multikernel child, rootfs record, active export, mknetd endpoint, `mkv*` link, or MK firewall/NAT rule.
- The user restart changed authoritative boot ID to `7d620d19-11d5-44fa-816b-0e1cd87c8cf6` and current service PIDs to mkruntimed/mknetd/containerd/Docker 1456/1200/1340/1458. All are active/status 0, but mkruntimed reports `NRestarts=1` while the other three report zero. Current-boot journal and executable-path inspection are required before coordinated activation; earlier boot/PID health values are historical only.
- Executable inspection confirms mkruntimed/mknetd still run immutable `aa1238b…` binaries. Current-boot journal explains the single mkruntimed restart: its first boot attempt correctly failed host qualification while the Google guest agent was not yet confirmed active; systemd retried once four seconds later, storage validation passed, and the service has remained active since. The module `File exists` message on retry is expected idempotent load behavior. This is a bounded boot-order retry, not runtime-state instability, so coordinated empty-state activation may proceed.

### 2026-09-29 — exact `1832e62` generation activates coherently

- With all inventories empty, mkruntimed/mknetd were stopped, candidate manifest `3f2300d6…` was atomically installed, systemd reloaded the selected support generation, and both daemons restarted. Initial stop printed the expected warning that selected unit links had changed on disk; the explicit daemon reload occurred before either new process started.
- Strict active-manifest validation resolves exact `1832e62…` agent/initramfs and all pinned dependencies. Active manifest/helper hashes are `3f2300d68070d7ca333ecd9711c9ef286cbf78a2b9da976f6e376a292163ab42` and preserved live-proved `95e886d625a33de702989907f442f98a1812d315f2c6a4195214d60852be43ac`; public components report exact revision `1832e62…`.
- New mkruntimed/mknetd PIDs 7423/7404 execute the exact immutable `1832e62…` release, active with zero restarts/status 0. Containerd/Docker remain healthy at unchanged PIDs 1340/1458, also zero-restart/status 0. Boot remains `7d620d19-11d5-44fa-816b-0e1cd87c8cf6`. Fuller matrix replay remains the behavioral gate.

### 2026-09-29 — private-seed correction passes live; matrix reaches storage high-water guard

- Exact matrix `66bfbe65…` starts from measured zero resources on active `1832e62…`. Normal writable-root Docker now crosses the former generated-bind rejection, reaches `running`, and executes successfully. ctr/Docker child boot IDs `f2c0d771-26ae-4626-87d9-5cac7a8dea8a`/`9076b87a-a69a-482a-8344-85819f3369f2` are distinct from each other and host, and both report child kernel `7.0.0-mk2-gce-lab`.
- Shared split create/start, state/inspect, exec stdout/stderr, and private writable-root rows pass for both clients; private values remain `ctr-private` and `docker-private`. This directly live-proves the Docker metadata seed correction without host bind projection.
- The next independent ctr read-only-bind child is refused before allocation by the storage guard: `free=2684854272 required=3233402880`. The runner trap removed the two live workloads and temporary host bind tree. This is a correct high-water capacity refusal, not a bind-validation failure; independent cleanup plus disk ownership/capacity inspection are required before any expansion or reclamation.
- Independent audit finds empty default/moby/Docker/child/rootfs/endpoint/runtime-artifact inventories and stable service PIDs 7423/7404/1340/1458 with zero restarts/status 0. The retained `/dev/sdb` (`mk-mediated-storage-20260830`) is a 20 GiB ext4 filesystem: preserved child-A/child-B fixtures consume 4 GiB each and two reboot-quarantine roots consume 2 GiB each, leaving 6,979,842,048 bytes when clean. Two concurrent matrix roots consume roughly 4 GiB, explaining the observed 2.68 GiB refusal for a third root.
- Preserved evidence will not be deleted. The safe environmental correction is a non-destructive size increase of only the exactly identified retained disk followed by online ext4 growth; this changes capacity, not runtime code or assertions. One nested jq active-export projection was mistyped during the audit and remains to be rerun separately before expansion.
- The corrected active-export query returns `[]`. GCE independently identifies `mk-mediated-storage-20260830` as the attached 20 GB zonal `pd-balanced` disk used only by this qualification instance. A requested resize to 30 GB was rejected by the execution safety reviewer because it is persistent, billable infrastructure mutation without separate explicit approval; no disk or filesystem change occurred. User authorization is required before the exact-disk resize/online ext4 growth path can proceed.

### 2026-09-29 — focused bind qualification avoids accidental three-root overlap

- Commit `958a0f626add0dd32f6da14f24d79f30c819d652` adds only `scripts/test-runtime-readonly-binds-live.sh` atop the active runtime revision. Harness SHA-256 `a107ed08bebb5e7a3e12c9286e4d5b4ac2f8d9b1367c8e86fe82a939ecd820d4` passes `bash -n` and ShellCheck when available.
- The focused harness runs ctr and Docker bind cases one at a time while preserving the same directory/file bytes, guest write-rejection assertions, host-source immutability checks, and initial/final resource inventories as the fuller matrix. It does not reduce runtime storage size or remove retained evidence; live execution remains pending.
- Exact uploaded harness hash matches and starts with all six measured inventories at zero. The ctr child reads `ctr-host-immutable|ctr-file-immutable`; both attempted writes fail inside the guest with `Read-only file system`, and the primary-side directory/file bytes remain exactly `ctr-host-immutable`/`ctr-file-immutable` after exit. ctr read-only materialization and no-write-through are live-proven; Docker execution is in progress.
- Docker likewise reads `docker-host-immutable|docker-file-immutable`; both guest writes fail read-only and both primary sources retain exact original bytes. The unchanged exact harness exits 0 with `RUNTIME_READONLY_BIND_LIVE_PASS` and final `children=0 links=0 nat_rules=0 filter_rules=0 ctr_tasks=0 docker_containers=0`. This live-proves ctr/Docker directory and file read-only materialization plus no host write-through; deeper independent durable/process audit remains separate.
- The independent audit contradicts the harness's narrow cleanup count: Docker metadata, live child/network, and active export are gone, but moby retains stopped task/container `7ab0dc6d…`, its `PREPARED` rootfs/image, and shim supervisor/worker PIDs 9687/9692. The harness counted only default-namespace ctr tasks and the named Docker object, so its final zeros did not cover orphaned moby state. Read-only enforcement/no-write-through remains proved; complete cleanup does not. One bounded asynchronous-cleanup wait precedes exact delete-error diagnosis.
- The orphan persists past a bounded wait. Exact normal Task deletion fails `stop network namespace holder: waitid: no child processes`: the first cleanup reaped the owned namespace-holder child, but retry treats `ECHILD` as fatal and returns before rootfs cleanup, Task acknowledgement, and shim exit. This is a retry-idempotency defect in holder termination, analogous to the already-fixed relay case; identity validation must remain strict while an already-reaped owned child is accepted as terminal completion.

### 2026-09-29 — namespace-holder cleanup retry becomes idempotent

- `commandNamespaceHolder.Stop` now treats exact `ECHILD` after kill/already-done handling as terminal completion, matching the owned relay contract without suppressing other wait errors. Namespace identity remains validated at acquisition; this change applies only after the owned child is already unavailable to wait.
- The existing Delete retry test now includes both a relay reaped through `Cmd.Wait` and a namespace holder reaped directly through `Process.Wait`, then requires absent-sandbox retry to clear relay, holder, and init process ownership. Twenty consecutive race-enabled focused repetitions pass. Full-tree gates and live orphan recovery/replay remain pending.
- Full local qualification passes: `GOCACHE=/tmp/mklinux-gocache go test -race ./...`, `go vet ./...`, the complete documentation/schema/evidence/runtime checker, and `git diff --check`. An initial Go invocation without the explicit temporary cache failed only because the sandboxed default cache is read-only; the corrected invocation passed and is the authoritative result. Live orphan recovery and replay remain pending.
- The narrow source change is committed as `586e3b932857d3e1372fbc93f85b6c592abd44d1` (`runtime: make namespace holder cleanup idempotent`). Exact archive `/tmp/mklinux-586e3b9.tar.gz` is 1,015,225 bytes with SHA-256 `806260d47def435e7a243e5d2de752115085233c45c9e3f46ced5abb539d8087`; only the two shim source/test files are in the commit. Transfer, guest build, activation, and live recovery remain pending.
- The disposable VM is reachable and still reports authoritative boot `7d620d19-11d5-44fa-816b-0e1cd87c8cf6`, kernel `7.0.0-mk2-gce-lab`, 16 CPUs, and active mkruntimed/mknetd/containerd/Docker services. Its independent `/var/tmp/mklinux-586e3b9.tar.gz` hash/size exactly match `806260d4…` and 1,015,225 bytes. Extraction/build and any activation remain separate claims.
- Explicit-instance host verification passes, and the guest builds the complete release stamped `586e3b932857d3e1372fbc93f85b6c592abd44d1`. Exact release-manifest/shim/mkruntimed/mknetd/agent hashes are `7d3a7a9ce22d97eacd0c205838df8c0816bc3b8d1ca0504c5ce60e95bb3dd988`, `a1eeef5d2e3ef7a4068836f299f1b392822a9ec339fc017d66c7e2b92e42f746`, `9d71a29e5f1e3ec100822a5c9356fb22648a0b79308d1965cd009e52b70f87fc`, `cbd8344f0c49237d51284c510694f6635aab98a796b2e7b5053a81e6f78edf6b`, and `c7e97f702389532144c5513fcb8bbf59b70f011014af2ccc6777b4c6fed254ec`. Installation/activation and recovery remain unclaimed.
- Pre-activation recovery inventory still has exact stopped moby task/container `7ab0dc6d…`, old immutable `1832e62…` supervisor/worker PIDs 9687/9692, and 2 GiB root `/srv/multikernel-storage/runtime/task-a8ebe1eeca4af813f5fb849c4e90444b/root.ext4`. All four services remain at PIDs 7423/7404/1340/1458, active with zero restarts/status 0. Because those old shim processes cannot consume a new public symlink, recovery will use the authenticated built-in crash/reboot cleanup path after exact release activation; no manual runtime-state or preserved-evidence deletion is authorized or planned.
- Binary manager installed and selected immutable release `0.1.0-dev-586e3b932857d3e1372fbc93f85b6c592abd44d1`; all six managed links are valid and the public shim/mkruntimed/mknetd report exact revision `586e3b9…`. Existing daemons and the orphan shim remain old immutable processes until reboot, so this is selection only, not coherent running-process activation or recovery success.

### 2026-09-29 — authenticated reboot recovery removes stale holder/root ownership

- Controlled reboot advances the authoritative boot ID to `75c6e9b5-50f9-4039-af47-2b04bdea66cf`. Public and running mkruntimed/mknetd processes report/resolve to exact immutable revision `586e3b9…`; their PIDs are 1485/1255. Containerd/Docker are active at 1391/1493. mkruntimed has one boot-time retry requiring journal attribution; the other three services have zero restarts/status 0.
- Deep audit finds the stale moby task absent, old shim supervisor/worker absent, no Multikernel child, no file under `/srv/multikernel-storage/runtime`, empty rootfs state, empty mknetd endpoints, no shim/relay/NBD helper process, and no MK firewall/NAT rule. Thus the built-in reboot/crash recovery removed the exact stale holder/shim/root ownership without manual state deletion.
- Inert moby container metadata `7ab0dc6d…` remains although Docker has no object. This is not a live task or retained root; journal/state attribution and exact normal container-metadata removal remain pending. One link-audit `awk` expression was mangled by nested SSH quoting and produced no usable link result; it must be rerun directly rather than treated as zero.
- Direct retry confirms zero `mkv*` links and rootfs state is exactly `{"version":4,"records":{}}`. The sole mkruntimed restart is the same bounded boot-order condition: first qualification reports guest agent unconfirmed, then one systemd retry passes storage validation and remains active; the module-already-loaded message is expected. Containerd has no warning-or-higher current-boot entries.
- Exact normal `ctr -n moby containers rm 7ab0dc6d…` succeeds. Immediate inventories are zero for default/moby tasks and containers, Docker objects, children, runtime artifacts, links, and MK NAT/filter rules; service PIDs/restarts remain 1485/1, 1255/0, 1391/0, 1493/0 with all active/status 0. A `pgrep -f` shim counter returned 2 by matching the remote shell command containing its own search text; that value is excluded and an anchored executable query remains required.
- Anchored executable queries return no shim, relay, or NBD helper process, while boot remains `75c6e9b5…` and all four services are active. This closes recovery of the prior orphan; exact focused-harness replay plus an independent deep audit remain the behavioral regression gate.

### 2026-09-29 — exact focused bind replay passes on holder cleanup fix

- Uploaded harness independently re-hashes to pinned `a107ed08bebb5e7a3e12c9286e4d5b4ac2f8d9b1367c8e86fe82a939ecd820d4` and begins from six zero measured inventories. ctr and Docker each read the exact directory/file values, both attempted guest writes fail `Read-only file system`, and all primary-side bytes remain unchanged.
- The unmodified harness exits 0 with `RUNTIME_READONLY_BIND_LIVE_PASS` and final `children=0 links=0 nat_rules=0 filter_rules=0 ctr_tasks=0 docker_containers=0`. This re-proves bind materialization/no-write-through on active `586e3b9…`; independent moby/rootfs/process/service audit remains required because the same narrow counters previously missed the orphan.
- Independent post-replay audit is fully clean: zero default/moby tasks and containers, Docker objects, Multikernel children, runtime artifacts, rootfs records, mknetd endpoints, anchored shim/relay/NBD processes, `mkv*` links, and MK NAT/filter rules. Boot stays `75c6e9b5…`; service PIDs/restarts remain 1485/1, 1255/0, 1391/0, 1493/0, all active/status 0. The one mkruntimed retry predates the replay and is already attributed to guest-agent boot ordering. This live-closes the namespace-holder `ECHILD` cleanup regression.

### 2026-09-29 — qualification clean markers cover durable/process ownership

- Focused and full-matrix inventory functions now include default and moby tasks/containers, Docker objects, runtime-storage artifacts, durable rootfs records, mknetd endpoints, and anchored shim/relay/NBD processes in addition to children/links/rules. The matrix asserts the exact all-zero inventory at initial, post-delete, and final checkpoints instead of merely printing it.
- Both scripts pass `bash -n` and `git diff --check`. ShellCheck is unavailable on the workstation (`command not found`) and is not claimed. Exact live execution of the broadened focused harness remains pending.
- Exact broadened harness hash `4e19610897721354d2c6a1b960e6c414d729f4cfc114a0ea8081bfe13e0d5294` starts with all 13 inventories zero and both ctr/Docker read-only cases pass. Its immediate final assertion correctly refuses a pass because `shim_processes=2` while every durable/resource counter is zero. This differs from the original orphan (which retained moby/rootfs state); bounded process-convergence measurement is required before deciding whether the assertion needs a wait or exposes another defect.
- Follow-up anchored query finds the shim pair gone while moby/runtime/rootfs remain empty, classifying the immediate count as normal asynchronous exit. Both harnesses now poll the complete exact inventory for at most 30 seconds, return immediately on all-zero, and fail while retaining the last nonzero observation on timeout. Syntax/diff checks pass; final focused/matrix hashes are `05a3ca317e6025643f816536e0a8be946ed304a66d4c0c15e8e6422c67c59c81` and `0d94685c67386a05d95a812c4a3b68857f19d069da63c5eb25bc512882feb7e6`. Live focused replay remains pending.
- Guest independently matches final focused hash `05a3ca31…`. Both ctr/Docker bind cases pass; the final deep checkpoint observes the expected transient two-shim state, converges within the 30-second bound, then reports all 13 counters zero and emits `RUNTIME_READONLY_BIND_LIVE_PASS`. The strengthened cleanup marker is therefore live-qualified; the full matrix script itself remains unexecuted pending sufficient retained-disk capacity.
- Complete documentation/schema/evidence/runtime validation, Bash syntax, and diff checks pass for the final harnesses. Guest-uploaded focused/matrix hashes match local source. ShellCheck is unavailable on both workstation and guest, so no ShellCheck pass is claimed; checker-generated Python cache files were removed.
- The full matrix's bind row has no dependency on the two long-lived lifecycle/isolation children. It is now sequenced after those children are deleted and the expanded deep-clean assertion passes, so only one bind root exists at a time; a new post-bind deep-clean assertion precedes later rows. Concurrency/isolation proofs remain on the original live pair, while the unrelated third-root high-water collision is removed without resizing or deleting evidence. Final matrix hash is `26d67e809f7d380507cef21b8cc1d1c4ac1551e3485d1c2109a53bbda23e492d`; syntax/diff checks pass and live execution remains pending.
- Guest matches exact matrix hash `26d67e80…` and begins with all 13 inventories zero. Both clients pass image inspection, split create/start, state/inspect, distinct child boot IDs (`1fc0b96e…`/`650ffbdf…`) and child kernel, exec stdout/stderr, private writable-root isolation, outbound DNS/HTTP, and bidirectional sibling isolation (`ping` exits 1 both directions).
- The next row restarts mkruntimed, after which ctr exec `/bin/cat /proc/sys/kernel/random/boot_id` fails `INTERNAL: guest process start failed at executable`; the matrix exits 1 and invokes its trap. Thus the capacity-safe ordering crosses the former storage guard and exposes a genuine daemon-restart continuity failure. No runtime-restart pass or later-row result is claimed; cleanup audit and storage/session journal diagnosis are required.
- Trap cleanup cannot complete while mkruntimed restart-loops: default/moby each retain one task/container, two children, two prepared 2-GiB roots, two shim pairs, and two NBD helpers. mkruntimed reaches `NRestarts=5`; mknetd/containerd/Docker remain stable. Every retry fails `rootfs reconcile: verify prepared rootfs: prepared storage content differs from journal`.
- Root cause: lifecycle-owned `PREPARED` records deliberately retain the original deterministic image SHA for identity/cleanup, but their writable ext4 contents necessarily change after `/tmp/matrix-owner` and normal container activity. Reconcile correctly proves exact lifecycle path+digest ownership, held bundle/storage roots, regular artifacts, build metadata, initramfs/manifests, and directory identities, then incorrectly re-hashes mutable root content against the pre-run digest.
- Backend verification now takes an explicit content-policy flag. Prepare/idempotent prepare and verified boot remain strict about the immutable root digest; restart reconciliation for an exact lifecycle owner skips only that mutable-content comparison while preserving all static artifact and held-identity checks. Tests prove prepare replay requests strict content verification, owned reconciliation requests static-only verification, a mutated root fails strict verification but passes owned-static verification, and changed bootstrap/build artifacts remain rejected. Race-enabled `./internal/rootfs` passes. An initial gofmt invocation from `runtime/` used redundant `runtime/` prefixes and changed nothing; the corrected invocation succeeded.
- The restart loop reached 28 failures before the explicit service stop took effect; mkruntimed is now failed/stopped rather than activating, with the two exact tasks/roots preserved for recovery. Full `go test -race ./...`, `go vet ./...`, and diff checks pass. Fix commit is `783e15df242dee7240bc69199b3d0a41fb38f596` (`runtime: reconcile owned writable roots`); its 1,016,223-byte exact archive hashes to `0f627579adfd3d5198c1c0b6cc05549926eca507585caaf0842773e00d4ec543`. Guest build/deployment/recovery remain pending.
- Guest independently matches archive hash/size, passes host verification with the two children still owning CPUs, and builds exact revision `783e15d…`. Release-manifest/shim/mkruntimed/mknetd/agent hashes are `c8db094523141ed93effe6cbc7b070cd4e75fbdba8820caa1e6e5997ce539ef5`, `c26e487414e0c05037746da2818c67df18b73cb636fd5f6449bdeda44f48bb9f`, `74f51e110820f5fc647346fad273d7bd58c1c12268a997e0b64ddcdb8a11a413`, `822dc7433b2f67a79d2ed0a6bcec0cfa10e69e8a5652cfa3e1a2e3bd1b2f0fd7`, and `927263fe4cb9ec6fda36cd008063645538c100c4357bf4ae9d1f983d125339d7`.
- Binary release `0.1.0-dev-783e15d…` installs/selects, failed state is reset, and new mkruntimed PID 12956 resolves to the immutable exact release. It remains active/running for the 12-second gate with `NRestarts=0`/status 0 while both mutated roots remain owned, directly proving reconciliation no longer rejects expected writable content. Preserved-child exec and normal cleanup remain pending.
- Both trap-preserved tasks are already `STOPPED`; attempted ctr/moby exec correctly returns `failed precondition`, so no post-restart exec continuity is claimed from those stopped processes. Normal task deletion succeeds in both namespaces with expected exit 137, followed by normal container-metadata removal.
- Deep recovery audit is fully empty across default/moby tasks/containers, children, runtime artifacts, rootfs records, endpoints, anchored shim/helper processes; mkruntimed/mknetd/containerd/Docker are active at PIDs 12956/1255/1391/1493 with zero restarts/status 0 after failed-state reset. This closes recovery of the diagnostic run; a fresh matrix is required to prove live restart continuity.
- Capacity-safe matrix ordering is committed separately as `b3894d1` (`test: avoid accidental three-root overlap`). Before fresh replay, idle mknetd is restarted onto immutable `783e15d…`; public shim/mkruntimed/mknetd all report the exact revision and services are healthy.
- Fresh exact `26d67e80…` replay again starts from all 13 zeros and passes through sibling isolation with new child boot IDs `9d30716b…`/`eee0d17e…`. After the deliberate mkruntimed restart, ctr exec still fails at the guest executable boundary even though the corrected daemon no longer restart-loops. Therefore mutable-root reconciliation is fixed, but mediated storage/session continuity across daemon/helper replacement remains broken. No restart-continuity or later-row pass is claimed; helper/session audit and cleanup are required.
- Subsequent authoritative audit finds both default/moby task inventories empty, no NBD helper, and mkruntimed active at PID 14651 with zero restarts/status 0. The matrix trap therefore eventually cleaned the second failed run once the corrected daemon remained serviceable. This proves cleanup convergence only; it does not change the observed live-continuity failure.
- Storage backend explicitly launches each NBD server in its own process group and documents that the export must outlive mkruntimed; durable records plus PID start time, argv, image fd identity, binary identity, port/image/generation, and pidfd liveness gate exact adoption. The systemd unit omits `KillMode`, so default `control-group` semantics kill those intended survivors on `systemctl restart`. Reconcile can start replacement servers, but the children retain dead NBD sessions and cannot execute. Deployment must use process-only daemon stop semantics so exact helpers survive for authenticated adoption; shutdown still receives the machine-wide process teardown.
- Managed mkruntimed unit now explicitly sets `KillMode=process`, with an adjacent rationale tying survival to exact generation-bound helper adoption. Deployment lifecycle tests require exactly one such directive in the installed managed unit. Focused deployment test, complete documentation/schema/evidence/runtime chain, and diff checks pass. Live managed-support installation and restart replay remain pending.
- Unit fix commit is `43d038f` (`deploy: preserve storage helpers across daemon restart`). Its exact 1,016,303-byte archive hashes to `ebf81becd5a6caa26098c7d350fd66250eff30f95ba0e08cbc7c162c1188c400`; findings/evidence remain outside the commit. Guest verification and managed deployment remain separate claims.
- Guest independently matches archive hash/size. Root-owned extraction plus the active validated runtime/host inputs install and select managed deployment `40d4a96ec0ac3c89fc1e5b3b6bcde4d1c4fa2072e76e6ec7755a66840c1cd55a`; all managed links validate. After `systemctl daemon-reload`, authoritative unit state reports `KillMode=process`. This proves deployment activation only; live helper survival/adoption remains the matrix gate.
- Decisive fresh matrix proves the policy: with new child boot IDs `81d6f841…`/`306e82de…`, deliberate mkruntimed restart changes PID 14651→26854, both clients immediately exec successfully, and each returns its unchanged child boot ID. `runtime-daemon-restart-continuity`, pause/resume, and signal/exit rows pass. The run then fails during normal ctr task deletion with `DeleteSandbox: BACKEND_FAILURE`; no delete-cleanup or later row is claimed. Helper survival/adoption is closed, while lifecycle backend deletion after daemon adoption is the next diagnosed boundary.
- Immediate failure audit preserves the distinction between adoption success and delete failure. Systemd reports the two exact NBD helpers (PIDs 26194/26445) remaining in the unit cgroup across the intentional daemon stop and warns about those leftover processes when PID 26854 starts; that is the observable consequence of `KillMode=process`, and mkruntimed remains active with zero restarts. Docker's instance 41 subsequently halts, unloads, and is removed with its resources returned. The ctr instance 40 reaches the kernel's halted state, but the captured kernel sequence has no corresponding successful unload/removal before `DeleteSandbox` returns `BACKEND_FAILURE`. Default namespace retains the stopped `mk-matrix-ctr` task while the moby task is absent. The lifecycle snapshot was not at the initially guessed `/var/lib/mkruntimed/state.json`; only rootfs/storage snapshots were found there, so the configured lifecycle state location and exact backend error must be inspected rather than inferred or manually repaired.
- Configuration proves lifecycle state is `/var/lib/mkruntime`, distinct from `/var/lib/mkruntimed/{rootfs,storage}`. The retained ctr rootfs is still `PREPARED`; its exact storage export generation `956783ba…` is durably `QUIESCING`, and no NBD helper now remains. Therefore `DeleteSandbox` entered `releaseStorage`, stopped the adopted export, and failed before marking it `RELEASED`; Kerf unload/delete necessarily occurs later in the lifecycle method. The halted-but-not-removed kernel instance is a downstream consequence, not evidence that Kerf deletion itself failed. All four services remain active/status 0 and mkruntimed has zero restarts after the intentional restart.
- Exact retained evidence identifies the release defect. Lifecycle journal sequences 299–302 record two retryable delete failures while the lifecycle snapshot remains `STOPPED`; the storage lease is `QUIESCING`. Its generation-specific helper log is canonical and terminal: READY for export `956783ba…`, CLIENT_ACCEPTED, then `MKNBD_SERVER_CLOSED synced=1` with 265 reads, 49 writes, and 12 flushes. The process record still exists although the helper is absent, and a later non-mutating `e2fsck -fn` completes all five passes with exit 0. After daemon adoption, the child halt can close the NBD client and let the helper exit naturally before `Release` calls `Stop`; `LinuxBackend.Stop` sees no in-memory `managed` entry and rejects the absent process without consulting the exact graceful-close log. By contrast, `Observe` already accepts that log as closed evidence. This asymmetric idempotency rule strands a valid export in `QUIESCING`; the fix must authenticate the terminal log, remove only the exact retained process record, and continue the offline check.
- Storage backend recovery now accepts an already-exited server only when its exact lease-specific log contains the canonical READY marker and terminal `synced=1` close record. Both `Observe` and `Stop` return the authenticated counters and remove only the identity-matched retained process record; absent processes with missing/malformed/conflicting evidence still fail closed. A focused regression exercises both direct observation and direct release-stop after recovered process exit. Twenty race-enabled repetitions, the existing live-process adoption test, counter-evidence test, formatting, and diff checks pass; full-tree gates remain pending.
- Full local qualification passes: `GOCACHE=/tmp/mklinux-gocache go test -race ./...`, `go vet ./...`, the complete documentation/schema/evidence/runtime checker (including 90 OCI semantic cases), and `git diff --check`. The only checker skip is the previously classified sandbox-local socket `EPERM` subcase. Four checker-generated Python cache files were removed; the untracked user evidence tree remains untouched. Exact commit, guest build/deployment, retained-task recovery, and clean matrix replay remain separate claims.
- The narrow source/test fix is commit `e37094bd19d15a792eaf802eb49f328a54c60549` (`runtime: recover gracefully closed storage exports`). Exact archive `/tmp/mklinux-e37094b.tar.gz` is 1,016,611 bytes and hashes to `6c49d1714853dbf63887113c3b6dc858ea7474ca5e5b213caf52c859cd5cf638`. Findings remain deliberately uncommitted and the preserved evidence tree is unchanged. Transfer/build/deployment and live recovery are not yet claimed.
- Guest independently matches archive hash/size, extracts it into the unique new build directory, and passes explicit-instance host verification on kernel `7.0.0-mk2-gce-lab`. The complete revision-stamped release builds successfully. Exact release-manifest/shim/mkruntimed/mknetd/agent hashes are `daa7be65b7b85f0c89c89a9ee87a35687cbf36b63a02943f711c2571e61c91e9`, `512e20207b223ef953557de841934b7af75c5dfffc0a97ae34a2fafe90612ec5`, `d9fea9b1aab191820d4787f294b2b8a564b1461dd169108144a4539091625111`, `fd687ade023b3d3f18619c8c367b95c4c75b8b02755bb883a66e0b9a012af3d2`, and `5dfefe3fe620d3e98f4db0dd76bf3f2752782a55177f7e34ed8add93cabddfa2`. Deployment/restart and retained-task recovery remain unclaimed.
- Binary manager installs/selects immutable release `0.1.0-dev-e37094bd…`; all managed links validate and public shim/mkruntimed report the exact revision. Existing daemon PID 26854 remains old until an explicit restart, so selection alone makes no recovery claim.
- Explicit daemon restart starts PID 28723 from the exact immutable `e37094bd…` path, active with zero restarts/status 0. Reconciliation authenticates the retained terminal helper log, removes the exact stale process record, runs the offline check, and advances export `956783ba…` from `QUIESCING` to `RELEASED` with counters 265 reads/49 writes/12 flushes and digest `e2fsck-clean-sha256:a783216b…`. It deliberately leaves lifecycle sandbox/task `STOPPED` and kernel instance 40 present for the ordinary caller-owned delete retry; normal `ctr tasks rm` remains the next recovery gate.
- The exact ordinary retry `ctr -n default tasks rm mk-matrix-ctr` now exits 0. Its warning preserves the workload's prior nonzero exit status 42, while lifecycle sequence advances to 303 with an empty sandbox map, the task disappears, and the kernel instance is removed. mkruntimed remains PID 28723, active/status 0 with zero restarts. This directly live-proves recovery of the previously stranded delete through the normal caller path; inert container metadata removal and the independent deep-clean audit remain pending.
- Normal ctr container-metadata removal succeeds. Independent audit then finds zero default/moby tasks and containers, Docker objects, Multikernel children, runtime artifacts, rootfs records, mknetd endpoints, executable-matched shim/relay/NBD processes, `mkv*` links, and MK NAT/filter rules. All four services are active/status 0 with zero restarts; PIDs are mkruntimed/mknetd/containerd/Docker 28723/13433/1391/1493. Exact executable inspection shows mkruntimed on `e37094bd…` while idle mknetd remains immutable `783e15d…`; a coherent mknetd restart onto the selected generation precedes full matrix replay.
- Idle mknetd restarts coherently as PID 29500. Both mkruntimed and mknetd now execute exact immutable `e37094bd…`, active/status 0 with zero restarts. The guest independently hashes the uploaded full matrix to the pinned local `26d67e809f7d380507cef21b8cc1d1c4ac1551e3485d1c2109a53bbda23e492d`; decisive replay remains pending.
- A first replay command incorrectly invoked the harness itself under `sudo`. Its UID safety preflight refused before qualification with `run as an ordinary sudo-capable user`; the empty-state trap had no workload to remove. This is an excluded operator invocation error, not a runtime result. The unchanged exact harness must be invoked as the ordinary SSH user and elevate only its scoped operations.
- Correct ordinary-user replay starts on authoritative boot `75c6e9b5…` with kernel `7.0.0-mk2-gce-lab` and all four services active. Its broadened initial checkpoint reports all 13 inventories exactly zero. ctr and Docker independently inspect BusyBox 1.36 at shared digest `sha256:73aaf090…`/architecture amd64, and `image-pull-and-inspect` passes for both clients. The live run has proceeded into ctr split create/start; later rows remain pending on the same process handle.
- The same run passes split create/start, state/inspect, distinct child/kernel identity (ctr `60dc93e8…`, Docker `a7834fc8…`, host `75c6e9b5…`), exec stdout/stderr, private writable roots, mediated DNS/HTTP networking on distinct `/30` links, and bidirectional sibling isolation. It then passes deliberate mkruntimed restart continuity with both boot IDs unchanged, pause/resume, signal/exit status 42, and—critically—normal task/container deletion for both clients. The post-delete deep inventory observes transient shim counts 4→2→0 within its bound and finishes with all 13 counters zero. This closes the fresh-workload form of the prior `DeleteSandbox` defect; read-only binds and later rows are now in progress.
- The subsequent ctr read-only-bind child reads exact directory/file values `ctr-host-immutable|ctr-file-immutable`; both attempted writes fail with `Read-only file system`. Docker's independent bind child is now running on the same live handle. Primary-source immutability and the combined row are not claimed until both cases and host-byte assertions complete.
- Docker independently reads `docker-host-immutable|docker-file-immutable` and both guest writes fail read-only. All four host directory/file sources retain their exact original bytes. Post-bind deep inventory observes transient shim processes and converges to all 13 counters zero, then `readonly-bind-inputs` passes for both clients. The matrix has moved into repeated attach/error/exit cycles; those remain pending.
- Repeated-exit cycle 1 passes for both clients: each returns exact exit 17, exposes both its client-specific stdout and stderr markers, Docker inspect/wait both report 17, and normal task/container cleanup returns host child/network inventories to empty before cycle 2 starts. The second cycle remains in progress.
- Cycle 2 independently repeats exact status 17, both stdout/stderr markers, matching Docker inspect/wait, and normal cleanup. Consequently `foreground-wait-stdio-and-nonzero-exit` and same-name reuse both pass for ctr and Docker. The matrix has begun stdin forwarding; that and later terminal/OCI rows remain pending.
- Guest stdin forwarding passes: ctr maps `ctr-stdin` to exact `guest-ctr-stdin`, Docker maps `docker-stdin` to `guest-docker-stdin`, then both clean normally. The suite is now testing attach to detached tasks; attach and subsequent rows remain pending.
- Attach to detached tasks passes with exact outputs `ctr-attached-stdin` and `docker-attached-stdin`, followed by normal child/network cleanup. Pseudo-terminal allocation/resize is now running; its result and later OCI rows remain pending.
- TTY testing exposes the next exact compatibility boundary. ctr succeeds with a real terminal, reports requested size `37 91`, and prints `ctr-terminal-ok`. Docker fails before child creation with `OCI configuration rejected: unsupported process field(s): consoleSize`; the matrix exits 125 and invokes its trap. Docker's current OCI request populates `process.consoleSize`, while admission has not modeled it even though terminal resize is already an advertised feature. No combined TTY row or later OCI row is claimed. Cleanup must be independently audited before adding exact bounded console-size validation/propagation.
- Independent post-failure audit is fully clean across default/moby tasks and containers, Docker objects, Multikernel children, runtime artifacts, rootfs records, mknetd endpoints, executable-matched shim/relay/NBD processes, host links, and MK rules. mkruntimed/mknetd/containerd/Docker are active/status 0 with zero restarts at PIDs 30966/29500/1391/1493; mkruntimed's PID change is the matrix's already-passed deliberate restart. The console-size rejection leaked no resource.
- OCI admission now accepts `process.consoleSize` only as an exact `{width,height}` integer object, only with `terminal=true`, and only within the Linux PTY bound 0–65535; incomplete, boolean, oversized, unknown-field, and non-terminal forms remain rejected. Guest projection preserves the validated value. The authenticated agent spec carries it and uses it as initial size only when no Task API resize supersedes it; exec-process validation/projection follows the same rules. Twenty race-enabled focused repetitions pass across agent/shim tests, including an actual PTY size observation at 91×37, and the expanded fail-closed OCI suite passes 95 semantic cases. Full-tree gates and live replay remain pending.
- Full local qualification passes: repository-wide `go test -race ./...`, `go vet ./...`, the complete documentation/schema/evidence/runtime chain with 95 OCI semantic cases, and `git diff --check`. The only skip remains the classified sandbox-local socket `EPERM` subcase. Checker-generated Python caches were removed and the preserved evidence tree is untouched. Exact commit/build/deployment and live TTY replay remain separate claims.
- Console-size support is frozen in commit `1f81cb2aec7f4774c89506c71eb8348c37147e9e` (`runtime: support OCI terminal console size`). Its exact 1,017,425-byte tracked-source archive hashes to `a06697189cd12606167011745b7b628663f9a090ac7005ea19e796d7734b50f0`. Findings remain outside the commit and preserved evidence remains unchanged. Guest verification/build/deployment and replay are unclaimed.
- Guest independently matches archive hash/size, passes explicit-instance host verification with all 16 CPUs returned, and builds the complete exact revision. Release-manifest/shim/mkruntimed/mknetd/agent hashes are `bde83869f35f06f82a0654acda8efe75beb3c6bac15fda9abd5adffc30bce32a`, `d1888952eb9be8e5e615caf18ff91e52d15325774984ff2b659ad95a5dad5594`, `e10bbdc630b13058f4213ba649f95a0a370211b4a5ecd60b364034492e7dd6e4`, `edaf5c59bf18da6bfbed7ee9df5f4cdd0cdb974d40feb9462404a9398f5a8ac3`, and `c79210c60289e4f4fe14ae14e2c142e109002f96c77949e96a192397b75db83d`. Because the change spans shim, rootfs validator, and guest agent, binary selection alone would be incoherent; a matching support generation and agent initramfs are required before replay.
- Pre-deployment reinspection after the VM restart finds all four services active, the managed support link still at generation `40d4a96ec0ac3c89fc1e5b3b6bcde4d1c4fa2072e76e6ec7755a66840c1cd55a`, `KillMode=process`, and active kernel-manifest hash `3f2300d68070d7ca333ecd9711c9ef286cbf78a2b9da976f6e376a292163ab42`. The selected config inputs are regular root-owned mode-0600 files. The cleanup inventory check emits no finding. This is the recorded rollback baseline; no deployment change is yet claimed.
- The guest builds a new initramfs from the exact `1f81cb2…` agent plus the already pinned transport module and relay. The agent hash remains `c79210c60289e4f4fe14ae14e2c142e109002f96c77949e96a192397b75db83d`; the resulting initramfs hashes to `2dec85b8ee8d8fb7b4e1b601aacc96f99f63e1ba61e3c2e5478bad295c88b21d`. The active manifest is captured as the candidate template, preserving its exact kernel, relay, module, compatibility, protocol, required-config, and OCI-feature pins. Staging has not changed the active manifest or services.
- Root-owned exact-source installation selects binary release `0.1.0-dev-1f81cb2aec7f4774c89506c71eb8348c37147e9e` and managed-support generation `d36b6940116af48cc13668f8443a87f0c69f3eb979f048a071ea11c586e64e3e`. A root-owned release directory contains the exact agent and initramfs, and candidate manifest `1d79c5644caef494e96453495795d83d71d1c35ac9043a84aac3d878e70999f2` passes the strict bootstrap validator with the pinned kernel/module/relay and advertised feature set. All services remain active and the active manifest deliberately remains old hash `3f2300d6…`; selection/staging alone is not yet an activated coherent runtime claim.
- Coordinated empty-host activation stops mkruntimed/mknetd, retains the old manifest as a non-JSON rollback file with exact hash `3f2300d6…`, atomically installs candidate `1d79c56…`, reloads systemd, and starts both daemons. Strict validation passes at the active path. mkruntimed/mknetd execute exact immutable `1f81cb2…` paths at PIDs 38582/38547; both report the exact revision, are active with zero restarts/status 0, while containerd/Docker remain active. `KillMode=process` remains loaded for mkruntimed, support OCI validator hash is `ddd77fb869fb258268fc5d6cadacc6ca664f3753a5b34cf747331dab087b8804`, and cleanup inventory emits no finding. The coherent deployment is active; workload behavior remains to be replayed.
- Guest copies of the full matrix and its previously unreached live-resize helper independently match local hashes `26d67e809f7d380507cef21b8cc1d1c4ac1551e3485d1c2109a53bbda23e492d` and `30c27d36d313a7bc54acfcf1a10a24da31007518220b87be77460401a260d58a`. Bash syntax and Python byte-compilation succeed, and all four services are active immediately before execution. The live matrix has not yet started.
- The exact matrix starts as ordinary UID 1001 on host boot `75c6e9b5…`/kernel `7.0.0-mk2-gce-lab`, with all four services active. Its initial broadened checkpoint reports every one of 13 inventories zero. ctr and Docker image inspection passes for BusyBox 1.36 at shared digest `sha256:73aaf090…` on amd64. The same live process has begun ctr split create/start; no later row is yet claimed.
- The same run passes split create/start, state/inspect, distinct child identities (ctr `c9b0ab66…`, Docker `c91d47c5…`, host `75c6e9b5…`) on the child kernel, exec stdout/stderr, private writable roots, distinct mediated `/30` networking with DNS/HTTP, bidirectional sibling isolation, deliberate mkruntimed-restart continuity with unchanged child boot IDs, pause/resume, signal/exit 42, and normal deletion. Post-delete inventory observes expected transient shim convergence and reaches all 13 zeros. The matrix has entered read-only-bind testing; later rows remain pending.
- Both read-only-bind cases pass: ctr and Docker read their exact client-specific directory/file bytes, guest writes fail `Read-only file system`, and all four host sources remain unchanged. The post-bind inventory waits through transient shims and reaches all 13 zeros. The matrix is now running the repeated nonzero-exit/name-reuse cycles.
- Both repeated cycles return exact status 17 for ctr and Docker, preserve client-specific stdout/stderr, report Docker inspect/wait status 17, clean normally, and reuse the same names on cycle 2. `foreground-wait-stdio-and-nonzero-exit` and `name-reuse` pass for both clients. Stdin forwarding is now running.
- Guest stdin forwarding passes with exact outputs `guest-ctr-stdin` and `guest-docker-stdin`, followed by normal host cleanup. Attach to detached tasks is now running; terminal/resize and later OCI rows remain pending.
- Attach to detached tasks passes with exact `ctr-attached-stdin` and `docker-attached-stdin` outputs and clean teardown. At the former boundary, ctr obtains a real PTY at `37 91` and succeeds; Docker's request now passes `consoleSize` admission and reaches child execution. The combined terminal/resize row is not claimed until Docker size/output and both live-resize probes complete.
- The terminal row still fails, but past the remediated Docker admission boundary: Docker reports exact `37 91` plus `docker-terminal-ok`; ctr's captured stream is `$'^@37 91\r\r\nctr-terminal-ok\r\r'`, so the exact line assertion rejects the unexpected leading literal `^@`. The matrix exits at that assertion and its trap runs before either live-resize probe or later OCI rows. No combined terminal/resize pass is claimed. Independent cleanup audit and tracing of the control character are required.
- Independent post-failure audit finds all four services active/status 0 with zero restarts; mkruntimed PID 40047 reflects the matrix's intentional restart. Children, links, NAT/filter rules, default/moby tasks and containers, Docker containers, runtime artifacts, rootfs records, endpoints, shims, and helpers are all zero. A focused exact ctr reproduction captures 28 raw bytes (SHA-256 `23ece05ab748b163823d566a707106fafe4bf8056e0e602bca1170be57aa8bdf`) and byte-dump proves the prefix is literal ASCII `5e 40` (`^@`), not display substitution for a retained NUL. Focused cleanup succeeds.
- A guest byte probe switches the child PTY raw and reads the queued byte as exact hex `00`, proving ctr supplied a NUL on terminal stdin and Multikernel faithfully forwarded it; canonical echo renders that input as `^@`. The identical nested-PTY command through standard `io.containerd.runc.v2` produces the same `5e 40` prefix (29-byte transcript hash `45d7ec78c61e8b06e17c323d40782ee400745747c221747917138b7a3f05e061`). This classifies the rejection as a harness portability defect: the size assertion must normalize only this known ctr/control echo while retaining exact size/output checks. Both probes clean normally.
- The matrix now removes only a leading literal `^@` from ctr transcript lines before the exact `37 91` comparison; Docker and success-marker checks remain exact. Direct shell probes prove both prefixed and unprefixed size lines normalize to exactly one match; Bash syntax and diff checks pass. The narrow change is commit `a7b1ad9` (`test: normalize ctr terminal control echo`), and the new exact matrix hash is `1d1d324b7bdb748da7c1502dcb4e74818bddad82ada1951b00fd192e617d1996`. Guest upload and complete replay remain pending.
- The guest independently matches corrected matrix hash `1d1d324b…` and unchanged resize-helper hash `30c27d36…`; all services are active and the new complete ordinary-user replay begins with all 13 inventories zero. Image inspection passes for both clients and split create/start is in progress. This is a new coherent run, not a resume after the failed assertion.
- The corrected replay passes the lifecycle/isolation block with new distinct child boot IDs `f3a346cf…`/`50ca36c5…`: split create/start, inspect, exec I/O, private roots, mediated networking, sibling isolation, daemon-restart continuity, pause/resume, signal/exit, normal deletion, and all-13-zero post-delete convergence. Read-only bind testing is now running.
- The corrected replay subsequently passes read-only binds/post-bind all-zero convergence, both exit-17/name-reuse cycles, stdin, and detached attach. Crucially, the complete terminal row now passes: ctr's known `^@` echo is narrowly normalized, while ctr and Docker each independently report exact `37 91` plus their exact success marker. This live-proves Docker `consoleSize` admission and propagation. The first post-start live-resize helper is now running; the resize row and final suite marker remain unclaimed.
- The newly reached ctr live-resize helper times out without observing its initial `ready:24 80` marker and reports `live resize failed: initial guest size was not observed`; the matrix trap runs. Neither post-start resize nor the final suite marker is claimed. Because the helper collapses PTY read errors/EOF directly into termination, independent cleanup plus native/runc comparisons are required before attributing this to runtime resize delivery.
- The helper passes both native and identical `io.containerd.runc.v2` commands, observing `ready:24 80`, signaling the live PTY, observing `resized:37 91`, and emitting `LIVE_RESIZE_PASS`. Multikernel startup is materially slower and can temporarily leave the helper's PTY master with no slave, so its blanket EIO-as-EOF handling kills ctr before the task console opens. That also exposes a failure-cleanup race: the immediate trap sees no task and removes metadata, while in-flight CreateTask later leaves exact `mk-matrix-ctr` shims and a `PREPARED` rootfs despite an empty lifecycle sandbox map. The first audit correctly did not claim clean; this retained state must be normally recovered, and both EIO waiting and late-create cleanup must be hardened.
- The retained pair is identity-verified as supervisor/worker PIDs 55368/55373 on exact immutable `1f81cb2…`, holding deleted bundle inode 13917; no task/container or lifecycle sandbox remains. After exact supervisor termination, its child exits by parent-death policy. Normal removal of the unrelated trace container plus mkruntimed restart lets authenticated rootfs reconciliation observe the absent, unowned bundle and remove the PREPARED record/artifact. An immediate read races state publication and still sees one record; the subsequent authoritative audit sees children, links, rules, both namespaces, Docker, runtime artifacts, rootfs records, endpoints, shims, and helpers all zero, with all services active. No evidence tree was deleted.
- The resize helper now treats PTY-master `EIO` or zero-length reads as a transient no-slave window while the client remains alive, but still terminates on those conditions after client exit. Matrix trap cleanup now preserves ctr container metadata while a slow exact shim is present, polls for a late task or a bounded quiet interval, and only then removes task/container state for each matrix ID. Local native resize observes exact 24×80→37×91, Bash/Python syntax and diff checks pass; live Multikernel replay remains pending.
- The full documentation/schema/evidence/runtime checker passes (95 OCI semantic cases); the only skip is the classified sandbox-local socket `EPERM`. Checker caches are removed. Commit `6927c70` (`test: tolerate slow terminal creation`) freezes helper hash `484229a83e2504c1b22506d0fd895291884cdd1abed953051516845f6e62fbcb` and matrix hash `5e850f3f45b8bc25101d9d1394c01cb510d42274c2ba31f2a3bca5bf57b66b69`. Findings and retained evidence remain outside the commit.
- After the user-restarted VM, the new authoritative boot is `c25eebdb-1f8d-481a-b741-e23417445772` on kernel `7.0.0-mk2-gce-lab`. Persistent selectors remain exact binary `1f81cb2…`, support `d36b6940…`, and manifest `1d79c564…`; uploaded helper/matrix independently match `484229a…`/`5e850f3…`. All 13 inventories are zero and all four services are active/status 0, but mkruntimed reports `NRestarts=1` (others zero). That restart must be explained from the current boot journal before calling the restarted baseline healthy or launching workloads.
- Current-boot journals prove an ordering defect: mkruntimed first starts at 23:03:20 UTC and fails `GUEST_AGENT`; `google-guest-agent.service` starts at 23:03:25 and reaches active at 23:03:32, after which mkruntimed's single retry begins at 23:03:38 and remains healthy. Storage and Multikernel mounts were already ready, so they are excluded. The managed unit now Requires and orders After `google-guest-agent.service`, consistent with the daemon's existing fail-closed host prerequisite; deployment tests require exactly that dependency and ordering. `python3 scripts/test-manage-runtime-deployment.py`, the full documentation/schema/evidence/runtime gate (including 95 OCI cases), and `git diff --check` pass; only the already-classified sandbox socket `EPERM` subcase is skipped, and generated caches were removed. Commit `8edc7cc5c653218679ae7cd1d3567c3f38b3a77c` freezes only this unit/test change; findings and retained evidence remain outside it. Live managed-support activation and a clean reboot observation remain pending.
- The exact tracked-source archive for `8edc7cc…` is 1,018,157 bytes with SHA-256 `efaf3af613fbd41597935bb4a5a36bd0752db0c1aebd4f3006e9e8b2b6646c25`. Guest transfer, independent verification, and activation remain unclaimed.
- The guest independently matches that exact archive hash/size, extracts it into a unique root-owned source tree, and verifies the unit source hash as `9d921d2fc93716bc7b57a17697977b0b119d36b8d4b43afd981fe1f3c9f95063`. Managed installation selects support generation `fe456acc4cf762526508a83b711ded09ac3275eedf11e464ac99fab65f4f755e`. Systemd reload, effective dependency inspection, pre-reboot cleanliness, and reboot proof remain pending.
- After daemon reload, systemd's effective `Requires` and `After` sets both contain `google-guest-agent.service`; deployed and generation unit paths independently match hash `9d921d2f…`. Support/binary selectors are `fe456acc…`/unchanged `1f81cb2…`, and the frozen matrix/helper still match `5e850f3…`/`484229a…`. All five relevant services are active and all 13 inventories are zero. The retained `NRestarts=1` belongs to the pre-fix boot; the clean reboot observation remains pending.
- The in-guest reboot request did not change the boot ID, so no success was inferred from it. An explicitly authorized GCE reset produces new boot `d08895c1-0d19-4c66-ac7e-c5f77fd23451` on kernel `7.0.0-mk2-gce-lab`. The guest agent enters active at monotonic 22,801,501 µs and mkruntimed starts at 24,201,242 µs, approximately 1.400 seconds later. The effective graph still Requires/After the agent; the journal contains no `GUEST_AGENT` qualification failure. Agent, mkruntimed, mknetd, containerd, and Docker are active/status 0 with `NRestarts=0`; system state is `running`. Persistent support/binary selectors and unit/matrix/helper hashes remain exact, and all 13 inventories are zero. This closes the boot-order defect with live evidence.
- The active kernel manifest is resolved from configured directory `/etc/mkruntime/kernels` and still hashes exactly `1d79c5644caef494e96453495795d83d71d1c35ac9043a84aac3d878e70999f2`; the retained rollback manifest is not selected. A focused wrapper for the previously failing ctr resize path passes shell syntax and hashes to `a5fd67a0a6f0ab8cd1ef1c94309ab35c1a8016c94ed66f808de8c24fb240bfcc`; guest verification/execution remain pending.
- The guest independently matches focused-wrapper/helper hashes `a5fd67a0…`/`484229a…`, but the focused Multikernel ctr replay still times out before observing `ready:24 80`; no resize success is claimed. Trap cleanup then runs for roughly one minute, showing the slow-create boundary is still exercised. Independent all-13 inventory, shim/rootfs, and service auditing are required before diagnosis or another run.
- The independent audit is not clean: `runtime_artifacts=2`, `rootfs_records=1`, and `shim_processes=2`, while every other inventory is zero and all services stay active/status 0. The retained exact-release supervisor/worker are PIDs 2526/2531, parented 1→2526, in one session, and still alive after 169 seconds. They hold the deleted `.mk-resize-focused` bundle/log/runtime directory; ctr task/container metadata and the visible bundle are already absent, while rootfs remains `PREPARED`. This proves the 30-second helper deadline can cancel a valid cold CreateTask and the 60-second cleanup deadline still removes metadata despite an exact shim. Exact orphan recovery is required before retry.
- After revalidating both exact PIDs and the worker's parent, SIGTERM to only supervisor 2526 removes both shims through parent-death policy. Restarting mkruntimed lets authenticated reconciliation remove the absent bundle's PREPARED rootfs; the subsequent authoritative audit returns all 13 inventories to zero and all five services remain active. The matrix now supplies an explicit 180-second live-resize deadline. Cleanup tracks whether any task was observed and, when none was, refuses metadata removal if the exact creating shim survives its bounded poll. Bash syntax, `git diff --check`, and the full documentation/schema/evidence/runtime gate pass with 95 OCI cases; only the classified sandbox socket `EPERM` subcase is skipped and generated caches are removed. Commit `3eaf828ecce07ee4d1f3eace88b11ee887c5be7c` freezes matrix hash `fafc2f802e9ac7ce138cf313a92c5a9c2e0067f8018b90efbea65fc521c9fdbc`; the focused wrapper hashes to `f9c018c40418b0eef014261c98e14a243dda61abaf42f557029721de7e3be5b1`. Live retry remains pending.
- The guest independently matches matrix/wrapper/helper hashes `fafc2f80…`/`f9c018c4…`/`484229a…`. The corrected focused Multikernel ctr replay completes after roughly 70 seconds, reports exact guest sizes `ready:24 80` then `resized:37 91`, and emits both `LIVE_RESIZE_PASS` and `FOCUSED_MULTIKERNEL_LIVE_RESIZE_PASS`. This live-proves post-start terminal resize and confirms the former 30-second deadline was invalid. Independent teardown audit remains pending before the complete matrix.
- Independent focused-replay teardown is all 13 inventories zero; mkruntimed, mknetd, containerd, and Docker remain active/status 0 with zero restarts. The complete matrix can therefore begin from a proven clean state rather than inheriting focused-test resources.
- The fresh complete ordinary-user matrix confirms host boot `d08895c1…`, all four runtime services active, initial all-13-zero inventory, and shared BusyBox digest `sha256:73aaf090…` on amd64. It then passes split create/start, state/inspect, distinct child boot identities `4b1621c7…`/`0613920a…`, exec stdout/stderr, private writable roots, mediated `/30` DNS/HTTP networking, bidirectional sibling isolation, deliberate mkruntimed restart continuity with unchanged child boots, pause/resume, signal/exit 42, normal deletion, and post-delete all-13-zero convergence for ctr and Docker. Read-only-bind and subsequent rows remain in progress.
- Both read-only-bind cases pass: ctr and Docker read their exact client-specific directory/file values, guest writes to both targets fail read-only, and all four host sources remain byte-identical. The post-bind checkpoint converges to all 13 inventories zero. Repeated nonzero-exit/name-reuse and subsequent rows remain in progress.
- Both repeated cycles return exact exit status 17 for ctr and Docker, retain their client-specific stdout/stderr markers, clean normally, and successfully reuse the same names in cycle 2. `foreground-wait-stdio-and-nonzero-exit` and `name-reuse` therefore pass for both clients. Stdin forwarding and subsequent rows remain in progress.
- Guest stdin forwarding passes with exact outputs `guest-ctr-stdin`/`guest-docker-stdin`. Detached-task attach also passes with exact `ctr-attached-stdin`/`docker-attached-stdin`; both rows tear down normally. Initial terminal allocation and post-start resize are now running and remain unclaimed.
- Initial terminal mode passes for both clients: ctr's classified leading `^@` echo is narrowly normalized, while ctr and Docker each report exact `37 91` plus their success marker. The ctr post-start resize independently reports `ready:24 80`, `resized:37 91`, and `LIVE_RESIZE_PASS`. Docker's resize and the combined/final matrix markers remain pending.
- Docker independently reports the same exact `ready:24 80`→`resized:37 91` transition and `LIVE_RESIZE_PASS`. The suite's final checkpoint is all 13 inventories zero, `post-start-terminal-resize` passes for ctr and Docker, and the run emits `G4_G6_CTR_DOCKER_FEATURE_MATRIX_PASS`. This completes the frozen full live matrix; an independent post-suite audit remains pending.
- Independent post-suite audit confirms the same boot `d08895c1…`, expected kernel, and system state `running`; support/binary selectors remain `fe456acc…`/`1f81cb2…`. Effective unit, active kernel manifest, matrix, and helper retain exact hashes `9d921d2f…`, `1d79c564…`, `fafc2f80…`, and `484229a…`. All 13 inventories are zero. Guest agent, mkruntimed, mknetd, containerd, and Docker are active/running with status 0 and `NRestarts=0` at PIDs 1066/12806/1216/1445/1543. The final live matrix is independently closed cleanly.
- Gate-closure reconciliation keeps G4–G6 open: the matrix directly closes only the shared-client lifecycle/I/O, post-start resize, mkruntimed-restart continuity, distinct network identity/sibling isolation, and runtime-cleanup slices. It does not prove G4 persistence/exhaustion/corruption/reset/clone, the G5 UDP/MTU/load/fault/reconnect/spoof/primary-health matrices, or G6 containerd/Docker restart, forced-shim reconstruction, event replay, FIFO/cancellation, and concurrent-churn requirements. A fresh schema-valid mode-0600 resource-before ledger is retained at `evidence/runtime-20260930/g4-g6-final-live/resources-before.json` (5,007 bytes, SHA-256 `fc6eb570941ece1b17b4875e87d7ee4fbf51600eaa17b106e32a42408748c83b`), recording one running n2-standard-16 instance, two disks, three snapshots, no addresses, and six firewall rules. The evidence-grade rerun will emit assertion markers only for the five proven slices; no broader claim is inferred.
- The scoped assertion markers pass Bash syntax, `git diff --check`, and the full documentation/schema/evidence/runtime gate. Commit `f41211f2127b3414f503084e39f48d93dcb18512` freezes matrix SHA-256 `00204de19035bff337ac22bd6556340d7c343b3b12b64c7327a25b07377c69a8`; generated caches are removed. Exact guest upload and retained transcript execution remain pending.
- The immutable evidence capture starts at `2026-09-29T23:51:00.495900Z` in `evidence/runtime-20260930/g4-g6-final-live/g6-shared-matrix.log`, retaining exact gcloud argv. The guest verifies matrix hash `00204de1…`, reports boot `d08895c1…`, starts with all 13 inventories zero, and confirms the shared BusyBox index digest/amd64 image for ctr and Docker. Later rows and the capture exit trailer remain pending on the same live process.
- The retained run's lifecycle block passes with new distinct child boots `f60ce340…`/`a8d7caf8…`, exact exec I/O, private roots, distinct `/30` endpoints, DNS/HTTP, bidirectional sibling rejection, pause/resume, exit 42, and normal deletion. Its deliberate mkruntimed restart changes PID 12806→22465 without changing either child boot, and post-delete inventory returns all 13 counters to zero. Bind and later rows remain in progress in the same transcript.
- The retained run subsequently captures exact read-only binds and post-bind all-zero cleanup, both exit-17/name-reuse cycles, stdin, detached attach, exact initial 37×91 terminals, and ctr live resize 24×80→37×91. Docker then receives a `WINCH` while the guest still reports `24 80`; the unconditional trap prints `resized:24 80` and exits before the intended update, so the immutable capture closes at `2026-09-30T00:11:00.288985Z` with exit status 1. This is a retained failed run, not a pass. Because the preceding complete run passed the identical runtime path, the immediate hypothesis is a probe race: the guest must ignore unchanged-size `WINCH` events and exit only after observing 37×91. Independent cleanup is required before changing or retrying it.
- Independent post-failure audit is all 13 inventories zero on unchanged boot `d08895c1…`; guest agent and all four runtime services remain active/status 0 with zero restarts. The guest resize trap now ignores any `WINCH` whose observed size is not exact 37×91, while the helper reapplies the idempotent PTY size and `SIGWINCH` every 500 ms until the expected guest observation or its bounded deadline. Native validation passes both the ordinary 24×80→37×91 path and 20 repetitions of an injected case that deliberately ignores the first correctly sized signal (`ignored-resize:37 91` followed by `resized:37 91`). The full documentation/schema/evidence/runtime gate and diff checks pass with 95 OCI cases; only the classified socket `EPERM` skip remains, and generated caches are removed. New helper/matrix/focused-wrapper hashes are `fe7059cfea96a75e6851b45bdc4fa3dc8e7545328059da13a4194b9f04162e51`, `4b83a01cba234328de00bb95153c83fe83c0c207b5e8cb9ac7c1dbd53aaa67f6`, and `fedf12899ec6646f44cd5cd99ccf667880beb66b604de9736b3558164b939579`. Live replay remains pending.
- Commit `79e97eba2662f96d895de3b67a0d74f7a6142f8d` freezes the helper/matrix correction. A Docker-only focused wrapper passes syntax and hashes to `b031b9a9670ab72dd4b0121f44b847b9d4ddac09407914b3564bdad231a5f731`; exact guest upload and focused Docker replay remain pending before another full retained run.
- Retained focused capture `g6-docker-resize-focused.log` independently verifies helper/wrapper hashes `fe7059cf…`/`b031b9a9…`, observes exact `ready:24 80`→`resized:37 91`, emits both resize pass markers, and closes at `2026-09-30T00:16:58.814611Z` with exit status 0. Independent teardown remains pending before the complete evidence rerun.
- Independent focused teardown returns all 13 inventories to zero; mkruntimed, mknetd, containerd, and Docker remain active/status 0 with zero restarts. The complete retained rerun therefore starts from a clean host.
- The next immutable candidate `g6-shared-matrix-pass.log` verifies corrected matrix/helper hashes and passes lifecycle/isolation plus bind rows with all-zero checkpoints, but exits 1 silently before cycle 1 at `2026-09-30T00:25:01.215394Z`; transcript SHA-256 is `4814439c48bc42378e97fc806ae7fe258d1dc44d842750207c128b41226695e2`. Independent audit is all 13 zeros with services healthy and storage at 65% blocks/1% inodes, excluding cleanup or ENOSPC. Bounded journals identify Docker CreateTask `BACKEND_TIMEOUT` after roughly 71 seconds. Live host config sets only 30 seconds, which is incompatible with measured 70-second cold/private-root creation. Containerd's subsequent dead-shim cleanup also logs `fork/exec /usr/local/bin/containerd-shim-multikernel-v2: no such file or directory`, although immediate identity inspection finds that link valid and resolving to exact release inode 6284664/hash `d1888952…`. The timeout must be corrected coherently before retry; the cleanup diagnostic remains a separate observation, not yet a persistent missing-link finding.
- A replacement host config changes only the bounded backend deadline from 30 to 180 seconds, exceeding the observed ~71-second create without becoming unbounded. Retained `host-config-timeout180.json` is schema-valid, mode 0600, 449 bytes, and SHA-256 `4056c1825c2f0ad43c4b20dced355c8be6034ba77200b1f112bc71534b1b0a4a`. Managed installation and effective-service validation remain pending.
- Root-owned exact config installation selects managed generation `25e6d343e1e18d7f5d5a55a7d029163a9289960ffe18c078f529467cfeb6beb7`. Coordinated empty-host daemon reload/restart makes the effective config/root copy match `4056c182…` and report 180 seconds; binary selector remains exact `1f81cb2…` and unit hash remains `9d921d2f…`. mkruntimed/mknetd/containerd/Docker are active/status 0 with zero restarts at PIDs 36974/36955/1445/1543, and all 13 inventories are zero. The exact formerly timing-out Docker boundary remains to be focused-replayed before another full capture.
- Retained `g6-docker-exit17-timeout180.log` verifies focused-wrapper hash `72044c84674165fdf1dfe0b2a9ff736c4da0be815d888eac35c87d0134be5556`, then the exact Docker boundary completes in 71 seconds with client and inspect status 17 plus both stdout/stderr markers. It emits `FOCUSED_DOCKER_EXIT17_PASS` and closes at `2026-09-30T00:31:07.311525Z` with exit 0. Because 71 seconds exceeds the old 30-second limit, this directly attributes the prior `BACKEND_TIMEOUT` to configuration. Independent teardown remains pending.
- Independent focused teardown again reaches all 13 inventories zero with all runtime services active/status 0 and zero restarts. The next complete retained run begins from that clean state under support generation `25e6d343…`.
- Capture `g6-shared-matrix-final.log` stops before matrix execution: ordinary-user verification correctly cannot read the mode-0600 managed config, so the capture closes at `2026-09-30T00:32:01.705852Z` with exit 1. This is a capture-command privilege error and provides no runtime result. It is preserved; the replacement command will elevate only the config hash check and use a new transcript path.
- Replacement capture `g6-shared-matrix-final2.log` verifies matrix/helper/private-config hashes and passes the lifecycle/isolation block, including mkruntimed PID 36974→39424 with child continuity and all-zero deletion cleanup. It then fails Docker's bind CreateTask with `BACKEND_FAILURE` and exits 125 at `2026-09-30T00:37:16.888569Z`; no bind or later marker is claimed. Independent audit is all 13 zeros and all services healthy. Sanitized kernel evidence identifies the cause: ctr bind teardown returns the 16-GiB Multikernel pool to the host, and Docker's following create twice fails to reallocate that pool with `-ENOMEM`. This is repeated idle pool teardown/recreation and host-memory fragmentation, not bind admission or storage. Kernel command-line output contained an authentication token and is deliberately neither retained nor reproduced. Pool lifetime/ownership must be corrected before another replay.
- The user's latest instance restart did not reboot the guest: authoritative boot ID remains `d08895c1-0d19-4c66-ac7e-c5f77fd23451` on kernel `7.0.0-mk2-gce-lab`. System state is `running`; guest agent, mkruntimed, containerd, and Docker are active with `NRestarts=0`. The production selector remains `0.1.0-dev-1f81cb2…`. `/etc/mkruntime/config.json` resolves into managed generation `25e6d343…`; its target is a root-owned mode-0600 449-byte regular file with exact SHA-256 `4056c1825c2f0ad43c4b20dced355c8be6034ba77200b1f112bc71534b1b0a4a` and effective backend timeout 180 seconds. The earlier apparent mode 0777/44-byte result described the symlink itself, not its target. This is a service/instance restart on the same boot and cannot by itself prove memory defragmentation or close the pool-lifetime defect.
- Pool ownership is now daemon-scoped across successful zero-sandbox intervals: ordinary last-sandbox Delete no longer releases/reallocates the 16-GiB contiguous pool, and the next sequential first sandbox reuses the initialized pool. Failed first creates and explicit cancellation retain immediate rollback. A graceful daemon shutdown releases the pool only when durable sandbox inventory is empty; a shutdown with any live durable sandbox is a no-op so restart continuity is preserved. Startup recovery is limited to the exact authenticated crash residue of a configured pool, zero backend instances, zero durable sandboxes, and no other stale resource; all other stale combinations still fail closed. Focused race tests for lifecycle and mkruntimed plus focused vet and `git diff --check` pass. Full-tree qualification and live activation/replay remain pending.
- Full local qualification passes: repository-wide `go test -race ./...`, `go vet ./...`, the complete documentation/schema/evidence/runtime chain, and 95 OCI semantic cases. The sole skip is the previously classified sandbox-local socket `EPERM` subcase. Checker-generated caches are being removed before the exact revision is frozen; live build, activation, sequential-pool reuse, graceful idle release, live-sandbox restart preservation, and full retained replay remain separate claims.
- Commit `e27ab26` (`runtime: retain pool across sequential sandboxes`) freezes only the four mkruntimed/lifecycle source and test files. Its exact tracked-source archive `/tmp/mklinux-e27ab26.tar.gz` is 1,020,028 bytes with SHA-256 `87440b1fe04664d57e7ce95a5467ec779219236f2dc05ea82778e320134e206b`; the learning documents and historical evidence trees are not in the commit. A final focused test additionally proves ambiguous pool initialization is unconditionally released by authenticated Create cancellation. Guest transfer, independent hashing, build, activation, and live replay remain unclaimed.
- The disposable guest independently measures `/var/tmp/mklinux-e27ab26.tar.gz` as the same 1,020,028 bytes and SHA-256 `87440b1fe04664d57e7ce95a5467ec779219236f2dc05ea82778e320134e206b` on unchanged authoritative boot `d08895c1…` and kernel `7.0.0-mk2-gce-lab`. This proves transfer identity only; extraction, build, selection, and execution remain unclaimed.
- The first guest build is explicitly rejected before installation because its manually supplied linker revision `e27ab26e3f2dba70fb4f8fc0f981d45dd59614df` does not equal authoritative commit `e27ab263e12980c670085495f21625e480bcc879`. Although compilation succeeded, none of its manifest/binary hashes is a deployable-candidate claim. The root-owned source/archive still independently match; the same tree must be rebuilt with the exact revision.
- An attempted in-place correction rebuilds binaries with the right stamp but the manifest publisher correctly refuses to overwrite the rejected tree's existing manifest (`EEXIST`). No mixed candidate is installed or claimed. The rejected directory is preserved intact; correction will use a second unique root-owned extraction so exclusive publication remains meaningful.
- Corrected unique root-owned extraction `/var/tmp/mklinux-src-e27ab26-correct` re-verifies archive SHA-256 `87440b1f…`, builds the complete release with exact revision `e27ab263e12980c670085495f21625e480bcc879`, and publishes manifest SHA-256 `7dccd39fe99192508ee859f1ae8b8ede0b2eb298b1e9c1841231f679e0fe9a94`. Exact mkruntimed/shim/mknetd/agent hashes are `be810c3c0098745b9a86158f98c984e9d1d61daf38bfb5a308bfb5ac13fd8882`, `043e94d45ce3edfd65ca043454671b9f112a965ae01b1092bd88125ac2c5b1a4`, `a78392c619cf80ee0cd1b4da17a347d53c32a1c105dad93f410be9a4efce474f`, and `b0f0ba521cc4330bee851cc3df964a7a1b7b131541893a76bbb1414c11b9889f`. The built daemon reports the exact revision. Installation/selection and execution remain unclaimed.
- Pre-activation inspection on unchanged boot `d08895c1…` uses the configured Kerf executable `/opt/mkruntime/kerf-venv/bin/kerf` and reports no configured memory pool, no Multikernel instance or `/proc/kimage` entry, zero default/moby tasks and containers, zero Docker objects, and zero sysfs children. mkruntimed, mknetd, containerd, and Docker are active; binary selector is still `1f81cb2…`. An earlier probe stopped harmlessly on the incorrect `/usr/local/sbin/kerf` path before any mutation. The host is clean for candidate installation.
- Binary manager installs and selects immutable release `0.1.0-dev-e27ab263e12980c670085495f21625e480bcc879`; empty-host mkruntimed restart produces PID 52355, active/status healthy with `NRestarts=0`. The running executable resolves into that exact immutable release and both running/public SHA-256 equal candidate `be810c3c0098745b9a86158f98c984e9d1d61daf38bfb5a308bfb5ac13fd8882`; the public daemon reports exact revision `e27ab263…`. mknetd, containerd, and Docker remain active. An initial ordinary-user `/proc/52355/exe` read stopped before hashing; the privileged replacement check supplies the authoritative identities. Live behavioral qualification remains pending.
- The evidence matrix now measures `pool_configured` without retaining raw Kerf output. It requires pool 0 at the initial clean checkpoint, pool 1 with every other resource zero after lifecycle deletion and bind cleanup, pool 1 again after the last workload, then deliberately restarts idle mkruntimed and requires a changed daemon PID plus pool 0/all-resource-zero final state. Thus the retained transcript will directly distinguish sequential reuse, live-restart preservation, and graceful idle release. Bash syntax, full documentation/schema/evidence/runtime checks, `git diff --check`, and matrix SHA-256 `3015518c904a432772d2acf2886d06bb8f48b08472d79e6f2147f7c1d6860db6` pass; only the classified socket `EPERM` subcase skips. Generated caches are removed. Exact commit/upload remain pending.
- Commit `94396f7` (`test: verify retained pool lifecycle`) freezes only the strengthened matrix at SHA-256 `3015518c904a432772d2acf2886d06bb8f48b08472d79e6f2147f7c1d6860db6`. Learning documents/evidence remain outside it. Guest upload, independent hash, and execution remain pending.
- Unique guest staging independently matches matrix/helper hashes `3015518c…`/`fe7059cf…`; matrix Bash syntax passes. Live preflight confirms unchanged boot `d08895c1…`, all four services active, exact running candidate revision `e27ab263…`, and no configured Kerf pool. Retained execution can begin from a proved released state.
- Immutable capture `g6-shared-matrix-pool-retained.log` starts at `2026-09-30T15:57:47.501147Z` with exact gcloud argv. On authoritative boot `d08895c1…`, its initial observation is `pool_configured=0` with every existing resource/process counter zero. Shared BusyBox digest `sha256:73aaf090…` and amd64 identity pass for ctr/Docker; lifecycle creation is in progress and no subsequent row is yet claimed.
- That immutable capture closes at `2026-09-30T15:59:03.940856Z` with exit 1 when the very first ctr Task returns `BACKEND_FAILURE`; no pool-retention or later feature row is claimed. Transcript is mode 0600, 128,724 bytes, SHA-256 `a89f9168f555aa7760c1ea3750fa66f13582e5a0d6f7702117e7573a02a017fe`. Because initial inventory proved no configured pool but the guest boot never changed after the user's instance restart, the leading hypothesis is failure to obtain the first 16-GiB contiguous allocation on the already fragmented boot. Independent cleanup/cause audit is required before the authorized GCE reset and retry.
- Independent failure audit is clean: pool 0; zero default/moby tasks and containers, Docker objects, children, runtime artifacts, rootfs records, and exact shim processes; all four services remain active. Narrow non-secret kernel filtering reports `Baseline pool allocation failed: -12` twice, directly identifying `ENOMEM` for the initial allocation. This confirms the unchanged boot remains physically fragmented despite no logical resource residue. An authorized GCE reset is now required to obtain a new boot; the candidate's retention behavior remains untested.
- Authorized GCE reset advances authoritative boot to `768706da-cc24-486b-8fb1-92d205010c44`. After bounded readiness, system state is `running`; guest agent, mkruntimed, mknetd, containerd, and Docker are active with `NRestarts=0`. Exact binary selector/revision remains `e27ab263…`; effective config target remains root-owned mode 0600, 449 bytes, SHA-256 `4056c182…`; Kerf reports no configured pool. This is the first valid post-defragmentation baseline for candidate replay.
- Replacement immutable capture `g6-shared-matrix-pool-retained-pass.log` starts at `2026-09-30T16:03:59.729374Z` on boot `768706da…`, again proving pool 0/all-resource-zero initially. Both lifecycle clients pass split create/start, state, distinct child boots (`ee457fda…`/`4a731acf…`), exec I/O, private roots, distinct mediated networking and isolation, live mkruntimed restart continuity, signals/exit, and deletion. The decisive post-delete observation is `pool_configured=1` with every other resource/process counter zero. The following ctr bind begins from that retained pool instead of reallocation; bind/Docker and later rows remain in progress.
- The exact former failure boundary is live-closed: ctr and Docker read-only bind rows both pass after lifecycle teardown, with exact source bytes preserved and guest writes rejected read-only. Post-bind inventory again records `pool_configured=1` with every other counter zero. Thus Docker successfully follows ctr from the retained 16-GiB pool instead of attempting the `-ENOMEM` reallocation seen in `g6-shared-matrix-final2.log`. Exit/name-reuse and later rows remain in progress.
- Both repeated ctr/Docker cycles return exact status 17, retain client-specific stdout/stderr, clean normally, and reuse the same names in cycle 2. Foreground wait/I/O/nonzero-exit and name-reuse rows pass while the single retained pool remains serviceable. Stdin/attach/terminal/final release rows remain in progress.
- ctr/Docker stdin forwarding passes with exact `guest-ctr-stdin`/`guest-docker-stdin`; detached attach passes with exact `ctr-attached-stdin`/`docker-attached-stdin`, followed by normal cleanup. Initial terminal allocation and later final pool-release proof remain in progress.
- Initial terminal allocation passes for ctr and Docker at exact 37×91; ctr's classified leading `^@` echo is narrowly normalized. Corrected post-start resize and final retained-pool release are in progress.
- Corrected live resize passes exact 24×80→37×91 for both ctr and Docker. Before shutdown, the suite observes `pool_configured=1` with all other counters zero; idle mkruntimed restart changes PID 3854→10964 and the final observation is `pool_configured=0` with all 13 existing resource/process counters zero. All scoped assertions emit true and `G4_G6_CTR_DOCKER_FEATURE_MATRIX_PASS`; immutable capture closes at `2026-09-30T16:20:22.902782Z` with exit 0. The mode-0600 318,154-byte transcript SHA-256 is `9b67a1042f83c7b608b6a4a8d742167b56792c1b70a78b0e464c04f4523361b0`.
- Independent post-suite audit on unchanged boot `768706da…` confirms system `running`, exact immutable selector `e27ab263…`, running mkruntimed SHA-256 `be810c3c…`, pool 0, and all 13 resource/process counters zero. Guest agent, mkruntimed, mknetd, containerd, and Docker are active with zero restarts. A non-restarting daemon reload clears the stale-unit warning without changing PID 10964. This live-closes the repeated pool teardown/reallocation defect: one pool survived all sequential workloads and the live-sandbox daemon restart, then released exactly at empty-daemon shutdown.
- Exclusive post-run ledger `resources-after.json` is schema 1, mode 0600, 5,007 bytes, SHA-256 `0f5f350812422600c7b506a2aeef2ac473f9d3cdaa9abffb81d25b3080548211`; ledger tests pass. It records the same one instance, two disks, three snapshots, zero addresses, and six firewall rules as `resources-before.json`. Removing only `captured_at` makes the normalized ledgers byte-identical, proving the qualification created no GCE resource leak.
- Final local reconciliation passes Bash/diff validation and the full documentation/schema/evidence/runtime chain with 95 OCI cases; the sole skip remains the classified local socket `EPERM`. Direct transcript inspection finds the six required pool/resource observations, all five scoped true assertions, the combined pass marker, exit-0 capture trailer, and no `mk.token`/password/credential/private-key value pattern. Generated checker caches are removed. Current-verdict text and checklist state now close only live resize and before/after resource ledgers; G4 persistence/fault matrices, G5 UDP/MTU/load/fault/security matrices, and G6 containerd/Docker restart, forced-shim reconstruction, event/FIFO/cancellation, and isolated fault matrices remain open, so no broad gate completion is claimed.

### 2026-10-01 continuation — daemon-restart evidence boundary

- Workspace reconciliation confirms the continuously maintained findings files
  and both historical evidence trees remain present, `git diff --check` is
  clean, and code HEAD is `94396f7` atop the live-qualified pool correction.
- The next bounded G6 requirement is two independent proofs rather than one
  combined service bounce: containerd restart must preserve a running ctr
  task, while Docker-daemon restart must preserve a running Docker container.
  Each retained transcript must include host boot and service PID before/after,
  child boot identity and task state, exec/stdout/stderr before and after,
  relevant Task event observations, exact normal deletion, and the complete
  final pool/resource inventory. No restart-continuity claim is made before
  those focused runs complete.
- Read-only live preflight on unchanged qualified boot `768706da…` confirms
  containerd exposes the expected `events` and task `attach` commands, but
  Docker reports `LiveRestoreEnabled=false`. Restarting Docker in that state
  would deliberately stop its container and cannot prove daemon reconnect
  continuity. Containerd restart can be tested independently; Docker restart
  requires an explicit validated live-restore configuration change before its
  focused run. No service was restarted and no workload was created by this
  preflight.
- A focused containerd-restart harness is frozen at SHA-256
  `46fe64373df48346b95b384a4e1b26cc53113958164effd560c505a280f93065`.
  It requires an initially released pool/all-zero inventory, retains init
  stdout/stderr across the original FIFO set, brackets restart with separate
  event subscribers, proves host/child identity and pre/post exec, performs
  normal task deletion, requires a retained-pool/all-other-zero checkpoint,
  and restarts idle mkruntimed to require final pool release/all-zero state.
  Bash syntax and `git diff --check` pass; shellcheck is unavailable locally
  and was not run. Guest transfer and live execution remain unclaimed.
- Guest staging at `/tmp/test-runtime-containerd-restart-live-46fe6437.sh`
  independently matches the full `46fe6437…` SHA-256 and passes guest-side
  Bash syntax validation (8,187 bytes; ordinary transferred executable mode
  0775). This proves input identity only; containerd has not yet been restarted
  and no behavioral result is claimed.
- Immutable capture `g6-containerd-restart-continuity.log` is preserved
  mode 0600, 85,012 bytes, SHA-256 `63e7f220e90ddd56025d12dbf26779c283a035320a1d0a8e7451f4e899610338`,
  and closes exit 1 at `2026-09-30T16:41:46.282154Z`. Before the harness-only
  terminal-state failure, it directly observes containerd PID `1481→12225`,
  unchanged host boot `768706da…`, unchanged mkruntimed PID 10964, running task
  continuity, unchanged child boot `369f8b53…`, successful pre/post exec
  stdout and stderr, and all four pre/post init stdout/stderr markers delivered
  through the original attached stream. The failed capture is not a pass.
- The harness expected the exited task to remain listed as `STOPPED`, but this
  ctr attach path had already removed it and returned an empty task row. Its
  cleanup then removed the remaining exact container metadata. The first
  independent audit command contained invalid arithmetic syntax at its helper
  count and established no cleanup inventory beyond `pool=1`. The corrected
  audit found pool 1 and every workload/child/storage/network/shim/helper count
  zero; all four services were active with zero restarts. An idle mkruntimed
  restart changed PID `10964→14230` and restored pool 0. The correction must
  accept either an explicit `STOPPED` row followed by delete or already-absent
  state after successful attach, while still requiring exit/delete events and
  exact final inventories.
- The corrected harness now implements exactly that two-state terminal rule.
  Local Bash syntax and diff validation pass; shellcheck remains unavailable.
  A new hash and guest transfer are required before replay, so the earlier
  `46fe6437…` input remains the only input associated with the failed capture.
- Corrected input is frozen at SHA-256
  `dbc4630c1ce984adbd0948d00a2f4408f3eea3a66071a9f94b1dc50868b12599`
  (8,373 bytes). Guest staging under its hash-specific name independently
  matches and passes Bash syntax. Replay preflight confirms unchanged boot
  `768706da…`, released pool, and empty default task/container inventories.
  The corrected behavioral replay remains unclaimed.
- Corrected immutable capture `g6-containerd-restart-continuity-pass.log`
  closes exit 0 at `2026-09-30T16:49:12.337918Z`, is mode 0600 and 92,240
  bytes, and hashes to
  `afa9187e3d22ae914b6abedb74344fc4efa6227711399b7670362fc0f3726a9e`.
  It records containerd PID `12225→14999`, unchanged host boot `768706da…`,
  unchanged mkruntimed PID 14230, running task state and child boot
  `bf941fef…` before/after, exact pre/post exec stdout and stderr, and all four
  init stdout/stderr markers from the original stream across restart. Separate
  subscribers retain create/start and post-restart exec/exit/delete events;
  attach's already-absent task outcome is explicit before normal container
  metadata removal. Cleanup converges through a transient two-shim interval to
  pool 1/all-other-zero, then idle mkruntimed PID `14230→15736` releases the
  pool and yields the complete pool-0/all-zero inventory. The scoped pass marker
  and exit-0 capture trailer are present, with no credential-pattern match.
- Independent post-pass audit confirms the same boot, pool 0, all audited
  workload/child/storage/network/shim/helper counts zero, and mkruntimed,
  mknetd, containerd, and Docker active with `NRestarts=0`. This closes the
  current-revision containerd restart slice only. The combined checklist item
  remains unchecked because Docker restart continuity is still unproved and
  the host currently has Docker live restore disabled.
- Docker configuration tooling now exposes live restore only through an
  explicit `--enable-live-restore` opt-in; ordinary runtime registration and
  removal remain unchanged. The option accepts an absent or already-true
  setting, preserves existing/default-runtime values, and refuses false or
  malformed existing policy. It is mutually exclusive with runtime removal,
  and deployment guidance requires an exact previous-config backup for
  byte-for-byte rollback. All six focused Docker-config tests, Python compile,
  and diff validation pass; generated caches are removed. Guest deployment
  and Docker restart behavior remain unclaimed.
- Live rollback baseline: `/etc/docker/daemon.json` is a root-owned mode-0644
  120-byte regular file with SHA-256 `7861303c7fbea5cdac88b5fdd522006451392e27f75e3fee3e884552553272af`.
  Its only top-level key is `runtimes`, containing the exact Multikernel
  `runtimeType`; default-runtime and live-restore are absent, and Docker reports
  effective `runc`/`false`. Updated merger SHA-256 is `d94dd4c2…`. No guest
  configuration has yet changed.
- On an empty released host, the guest independently matches merger
  `d94dd4c2…`; the generated candidate passes `dockerd --validate`. The exact
  original is preserved at a hash-specific backup with unchanged
  `7861303c…` digest. Atomic active candidate and staging copy both hash to
  `a49bee79a177f8ccf8bfe42be3ca283297bdeef6f5ef6c0442cbd0bfb84b553f`.
  Docker reload retains PID 1563 and reports live restore true while preserving
  default runtime `runc`. This proves policy activation only; no Docker-daemon
  restart or live-container continuity is yet claimed.
- Focused Docker-restart harness SHA-256 is
  `ab0cea4e9027b96844baef1fb352603b176a20df09531e3502858e39c6701f54`.
  It requires effective live restore and an initially released/all-zero host,
  keeps one moby event subscriber across Docker restart, verifies Docker PID
  replacement with unchanged containerd/mkruntimed/host/child identities,
  checks exec and init stdout/stderr on both sides, requires create/start/
  exit/delete events and normal removal, then asserts retained-pool/all-other-
  zero and final released/all-zero inventories. Bash syntax, focused config
  tests, and diff checks pass; shellcheck remains unavailable. Guest transfer
  and execution are unclaimed.
- Guest staging independently matches `ab0cea4e…` and passes Bash syntax.
  Replay preflight confirms effective live restore true, unchanged boot
  `768706da…`, released pool, and empty default/moby/Docker workload
  inventories. This is input/baseline proof only; restart behavior remains
  unclaimed.
- Docker restart capture `g6-docker-restart-continuity.log` closes exit 0 at
  `2026-09-30T17:03:36.799906Z`, is mode 0600 and 99,408 bytes, and hashes to
  `fa381592a9e39c64eb10b8d0d61913360d91c0be456d6d63be901a2b42531859`.
  Docker PID changes `1563→17416` while containerd PID 14999, mkruntimed PID
  15736, and host boot `768706da…` remain stable. Container `604b6f52…`
  stays running with unchanged child boot `7b6bc24d…`; exact pre/post exec
  stdout/stderr and init stdout/stderr retained by Docker logs all pass. One
  continuous moby event subscriber spans restart and retains create/start plus
  exec/init exit/delete events. Normal removal reaches pool 1/all-other-zero
  after transient shim convergence; idle mkruntimed restart `15736→18312`
  yields pool 0/all-zero and the scoped pass marker.
- Independent audit confirms unchanged boot, effective live restore true,
  active config `a49bee79…`, pool 0, all audited resource counts zero, and all
  services active with `NRestarts=0`. Direct transcript audit finds the event
  and final-inventory observations, pass marker, exit-0 trailer, and no
  credential-pattern match. Together with the separate `afa9187e…` containerd
  transcript, this closes the dedicated replacement-instance daemon-restart
  row. It does not close exhaustive durable event replay, forced-shim recovery,
  FIFO/cancellation, or packaging upgrade/rollback.
- Commit `43ea792a00030e193cf86e3365a0dad0867e0a5b` (`test: qualify
  runtime daemon restarts`) freezes exactly the explicit live-restore merger
  option, six focused tests, two live restart harnesses, and their deployment/
  script-index guidance. Continuously maintained findings and historical/live
  evidence trees remain outside the commit. The complete documentation/schema/
  evidence/runtime check passes with 95 OCI cases; the sole skip is the known
  local socket `EPERM` subcase. Generated caches are removed.

### 2026-10-01 continuation — forced shim death evidence boundary

- The existing `test-runtime-recovery.sh` cannot close the current evidence
  requirement: it uses obsolete `mkn*`/`172.30.*` cleanup predicates and does
  not retain recovery-record fields, Task events, service journal evidence,
  complete resource/process inventories, or final pool release. Its narrow
  marker is not being reused as final proof.
- The current supervised shim presents two separable fault boundaries. Killing
  only `.multikernel-worker.pid` leaves the authenticated supervisor/listener
  alive and must reconstruct the running task, original child boot, guest PID,
  recovery record, I/O, and later events. Separately killing the supervisor
  makes reconnect impossible because the worker is configured with parent-
  death `SIGKILL`; containerd must then converge through bounded cleanup-only
  reclaim without manual runtime-state deletion. A replacement focused harness
  must retain exact killed/replacement PIDs, sanitized recovery summaries,
  task/client behavior, filtered journals, events for the reconnect case, and
  retained/final inventories for both outcomes. No forced-death result is yet
  claimed.
- Replacement two-case harness `test-runtime-shim-death-live.sh` is frozen at
  SHA-256 `af51e5a9163ce37508d1b5cb833dde2ed4081252fc07c56259ed94885f1bcc59`
  (12,783 bytes). Its fallback exec probe is bounded to 30 seconds and its
  cleanup-only convergence wait to 180 seconds. Recovery evidence is reduced
  to schema/sandbox/generation/bundle/network/process identities, offsets, and
  state-file metadata; tokens and raw kernel command lines are never emitted.
  Bash syntax and diff validation pass; shellcheck is unavailable. Transfer and
  execution remain unclaimed.
- Guest staging independently matches full `af51e5a9…` SHA-256 and passes
  Bash syntax. Preflight confirms unchanged boot `768706da…`, released pool,
  and empty default/moby/Docker workloads. This is input/baseline proof only;
  neither forced-death outcome is yet claimed.
- The first immutable forced-shim capture is retained as a failed qualification,
  not overwritten: `g6-forced-shim-death-matrix.log` starts at
  `2026-09-30T23:16:18.661043Z`, ends at `23:19:52.150203Z` with exit 1, is
  mode 0600 and 354,963 bytes, and hashes to
  `5a4dedc7d0bc1b0a41e4b6f3d7b0cbf154713049edcc30c0ab6c3371a74ff8ef`.
  Its role labels were wrong: Task v2 reported namespace-holder PID 20697,
  while `.multikernel-worker.pid` correctly reported serving worker 20504.
  Killing 20504 injected the intended worker fault, but the harness then used
  the namespace holder's `/proc/20697/cwd` as though it were the stable
  supervisor anchor. That holder vanished with the worker, containerd recorded
  `shim disconnected`, cleanup after disconnect, and dead-shim cleanup at
  `23:17:40Z`, and the invalid poll could not observe reconstruction. No
  forced-death pass is claimed. Post-trap audit on the unchanged boot shows
  both recorded PIDs dead, pool retained, every logical/process counter zero,
  and all four services active with `NRestarts=0`.
- Corrected input is frozen at SHA-256
  `9e6a697e73bbd0cdf7934cdf329ad6a85f2de6a4e0f748dcb4743dff954e22bf`
  (13,651 bytes). It derives the worker from the Task row, derives the
  supervisor from `/proc/<worker>/stat`, verifies the supervisor-owned PID file
  points back to that worker before both faults, and suppresses only repetitive
  bounded-poll tracing. Local Bash syntax and diff validation pass. A unique
  guest transfer, guest-side hash/syntax check, and replay are still required.
- Unique guest staging independently matches full `9e6a697e…` SHA-256, is
  13,651 bytes/mode 0755, and passes guest Bash syntax. On unchanged boot
  `768706da…`, all four services are active, Kerf reports no pool, and default/
  moby/Docker task-container plus child/runtime-artifact counts are zero. This
  closes input identity and replay preflight only; fault behavior is unclaimed.
- The second immutable capture, despite its provisional `-pass` filename, is
  also retained as failed evidence: it closes exit 1 at
  `2026-09-30T23:25:42.796311Z`, is mode 0600 and 20,246 bytes, and hashes to
  `f0499c5a80a43c0233c579a9188bcc61cfaaccf356f3abd4e0db375dfd561ba1`.
  It injected no fault: its pre-fault assertion observed Task PID 29425 with
  parent 29239 and PID-file value 29239, then failed because it incorrectly
  called Task PID the worker and its parent the supervisor.
- A separate cleaned role probe resolves the actual three-process hierarchy:
  supervisor 30366 (PPID 1) → serving worker 30371 → Task namespace holder
  30555; the bundle PID file contains 30371. Therefore the stable supervisor
  is the worker's parent, not the Task PID's parent; reconnect must preserve
  the supervisor while replacing both worker and namespace holder. Independent
  post-probe audit shows unchanged boot, pool retained, zero children/default
  tasks/default containers/runtime artifacts/rootfs records/endpoints/shims/
  helpers, and all four services active with `NRestarts=0`. A third harness
  revision is required; neither prior capture closes either fault outcome.
- Third revision is frozen at SHA-256
  `1a2e557a2adca84bbba84a7aed0a7c502c5466ea81ea5dc8ffa80b71147190e8`
  (14,545 bytes). It proves holder→worker→supervisor parent links plus the
  supervisor-owned worker PID file before each fault. Reconnect keeps the
  supervisor stable and requires distinct replacement worker and holder PIDs;
  fallback requires all three old PIDs dead. Local Bash syntax and diff checks
  pass. Guest transfer and live execution are unclaimed.
- Third-revision guest copy independently matches full `1a2e557a…`, is 14,545
  bytes/mode 0755, and passes Bash syntax. On unchanged boot `768706da…`, the
  pool is released, task/container/child counts are zero, and all four services
  are active. This is exact input and baseline proof only.
- Third capture `g6-forced-shim-death-matrix-v3.log` is a valid product-failure
  result: it closes exit 1 at `2026-09-30T23:36:05.473765Z`, is mode 0600 and
  26,806 bytes, and hashes to
  `c1670e5b35b2f5512f7b3ddd34efd74cd3a6ab91bff3bcf27bfa9da7ed4e401e`.
  Before injection it proves supervisor 31483 → worker 31488 → namespace holder
  31675, RUNNING state, child boot `1f97a684…`, exact exec stdout/stderr, and a
  mode-0600 schema-3 recovery record. Killing only worker 31488 at 23:33:46 UTC
  caused containerd to record shim disconnect, disconnect cleanup, and dead-shim
  cleanup immediately; no replacement worker/holder or RUNNING Task appeared
  during the bounded two-minute wait. All three PIDs are dead afterward.
  Independent audit shows pool retained, zero child/task/container/artifact/
  rootfs/network/shim/helper counts, and all four services active with zero
  restarts. Thus cleanup safety passed, but supervised in-place reconstruction
  is a confirmed current-revision defect and remains open. The supervisor-death
  fallback case did not run and must be qualified separately.
- The harness now permits exact `matrix`, `reconnect`, or `reclaim` case
  selection so the fallback can be evidenced without skipping or disguising
  the failed reconnect result. This revision is frozen at SHA-256
  `16fb3a027a568237b7c46847b54492d0c2ee754a3e1b95ffd9e409f4f0f62fd7`
  (14,818 bytes); it emits the selected mode in the host observation and
  rejects unknown modes. Local Bash syntax and diff checks pass. Reclaim-mode
  guest staging and execution remain unclaimed.
- Guest staging independently matches full `16fb3a02…`, is 14,818 bytes/mode
  0755, and passes Bash syntax. The unchanged-boot reclaim preflight has no
  configured pool and zero task/container/child counts after idle mkruntimed
  PID `30919→44343` released the failed run's retained pool. This is input and
  baseline proof only.
- Independent fallback transcript `g6-forced-shim-reclaim.log` closes exit 0
  at `2026-09-30T23:40:44.443927Z`, is mode 0600 and 47,705 bytes, and hashes
  to `e833dcd1baa2127f66f73a1a01ddbe2c9eed8bc8ec846fe3e5d887bbcff28f5a`.
  It proves supervisor 44895 → worker 44901 → namespace holder 45097, RUNNING
  state, child boot `1b3584d3…`, and a private schema-3 recovery record before
  killing only the supervisor. All three PIDs die; Task state becomes absent,
  post-fault exec fails closed, containerd records disconnect/dead-shim
  cleanup, and child/artifact/rootfs/network/shim/helper counts reach zero.
  Normal metadata removal reaches pool 1/all-other-zero; idle mkruntimed restart
  `44343→45437` produces pool 0/all-zero. Direct transcript audit finds the
  scoped reclaim pass and no credential-pattern match. Independent audit
  confirms the same all-zero state, unchanged boot, and all services active
  with zero restarts. The transcript also contains a mechanically emitted
  generic matrix marker from that harness revision; it is not used because the
  reconnect case did not run and remains failed/open. The harness source now
  emits a mode-specific final marker outside matrix mode.
- Source remediation now moves ownership of containerd's accepted TTRPC
  connection into the stable supervisor. Workers serve on fresh random private
  Unix listeners and a supervisor bridge preserves one containerd byte stream
  across generations, removing the disconnect race demonstrated by
  `c1670e5b…` while retaining same-UID handshaking, parent-death behavior,
  held-bundle identity, exclusive PID publication, and the bounded restart
  budget. The full shim package passes; the new two-generation same-client
  bridge test and existing signaled-worker test pass 25 race-detector
  repetitions. This is local-only proof: build provenance, disposable-host
  activation, and live reconstruction remain open.
- The marker-corrected harness is 14,925 bytes with SHA-256
  `7b77bb91552dbfdbe1e4b54bb07df91cc980cca2c4102af7a155666534eb32ef`.
  It reserves the generic matrix pass for actual matrix mode and uses a scoped
  mode pass otherwise. The successful fallback transcript remains tied to its
  executed `16fb3a02…` input; no evidence is rewritten.
- Commit `7d50218f593eb1b548107eb9d82328e925eab448` freezes exactly the
  supervisor connection bridge, its focused test, the marker-corrected live
  harness, and script-index entry. Continuously maintained findings and both
  evidence trees remain intentionally outside the commit.
- Exact committed-source archive for guest build is 1,464,320 bytes, mode 0644,
  and SHA-256 `5f5c4fc86eec56262fd8618ceb2c9e6916f53442460294e38fd8d2f8cf499a25`.
  It was generated directly from commit `7d50218f…` and contains only Makefile,
  runtime source, and the release-manifest/binary-manager tools. Guest transfer,
  build, install, and activation remain unclaimed.
- Guest archive independently matches full `5f5c4fc…` and was extracted into a
  new unique build directory. Pre-build rollback baseline is the exact active
  selector `0.1.0-dev-e27ab263e12980c670085495f21625e480bcc879` on unchanged
  boot `768706da…`; Kerf has no pool and default/moby/Docker workload counts
  are zero. Build and activation remain unclaimed.
- Guest build succeeds for all seven components with embedded version
  `0.1.0-dev` and exact revision `7d50218f…`. The mode-0644, 1,907-byte release
  manifest hashes to `70345fb18a5460913b1df68d1736282305a41f32485ba39c1258688bace37079`.
  Component SHA-256 values are shim `65a9256c…`, agent `bdae9b3a…`, agentctl
  `5057066f…`, CNI `1ff91069…`, host check `b96f3d40…`, mknetd `88c1b80e…`,
  and mkruntimed `38a7a2f3…`. An initial read-only component-name projection
  used the wrong JSON shape and exited 1 after already proving manifest/binary
  hashes and revision; the corrected projection lists all seven names and every
  `--version` identity. No installation or activation is yet claimed.
- Immutable release manager installs and atomically selects exact candidate
  `0.1.0-dev-7d50218f593eb1b548107eb9d82328e925eab448`; all six host command
  links remain manager-owned, and inspect retains prior exact rollback release
  `e27ab263…`. Active shim reports the full candidate revision and re-hashes to
  build SHA-256 `65a9256c…`. No service restart or workload occurred during
  installation; live reconstruction remains unclaimed.
- Candidate replay input independently matches full `7b77bb91…`, is 14,925
  bytes/mode 0755, and passes guest Bash syntax. Preflight proves active exact
  selector/shim revision `7d50218f…`, unchanged boot `768706da…`, no pool,
  zero default/moby/Docker workload and child counts, and all four services
  active with zero restarts. This closes candidate/input identity only.
- Candidate capture `g6-forced-shim-reconnect-candidate.log` is retained exit 1
  at `2026-09-30T23:53:45.044495Z`, mode 0600, 36,646 bytes, SHA-256
  `ebfcf74926540648c4af5070322797d53060d8100dacd84da1ab7fc163f38c32`.
  It directly proves the repair's core behavior: supervisor 51313 remains;
  worker `51318→51613` and namespace holder `51504→51637`; Task remains
  RUNNING; child boot `868b0395…`, guest PID 163, sandbox/network generations,
  recovery file identity, and output offsets remain unchanged; post-fault exec
  stdout/stderr succeeds. The fault-time journal observation is empty—no
  containerd disconnect cleanup occurs during reconstruction. The harness then
  releases the task successfully but fails because its late attach sees only
  post-release init output, while it incorrectly requires pre-fault bytes that
  had already traversed the old pump. No scoped pass is claimed. Trap cleanup
  leaves pool retained/all audited logical/process resources zero and services
  healthy. A later normal task/shim completion produces containerd's ordinary
  disconnect cleanup at 23:53:44Z; this is after the empty fault-time journal.
  The corrected replay must keep an attach reader open across the fault.
- Continuous-attach correction is frozen at SHA-256
  `e47804f5ce942b6de2f294dc1b7676c40de0a63749e439707039e059e03d2135`
  (15,639 bytes). The init blocks on an explicit readiness file; the harness
  opens one attach reader first, releases the init only after attachment, proves
  pre-fault stdout/stderr arrived, keeps that reader across worker replacement,
  then requires both post-fault streams from the same reader. Cleanup tracks and
  stops the attach reader. Bash syntax and diff checks pass; guest replay is
  unclaimed.
- Idle mkruntimed restart `45437→52142` releases the retained pool and restores
  zero task/container/child counts. An attempted `/proc/<pid>/exe --version`
  execution printed systemd's version and is rejected as an identity probe.
  The corrected stable read-only unit audit proves PID 52142 active/running,
  `NRestarts=0`, executable path in exact candidate `7d50218f…`, and executable
  SHA-256 `38a7a2f3…`. The replay baseline is released and candidate-bound.
- Corrected guest harness independently matches full `e47804f5…`, is 15,639
  bytes/mode 0755, and passes Bash syntax. Exact candidate selector `7d50218f…`
  remains active with no pool and zero task/container/child counts. This is
  input/preflight proof only; replay behavior remains unclaimed.
- Continuous-attach capture `g6-forced-shim-reconnect-candidate-pass.log` is
  retained as exit 137 at `2026-10-01T00:04:13.478649Z`, mode 0600, 39,468
  bytes, SHA-256
  `ab089bafb424f98c9519b9f9b42bfb1f049a609f009f800ffbff674e1c5a62e8`.
  Reconstruction again succeeds at the service boundary: stable supervisor
  52809, worker `52814→53164`, holder `53012→53189`, RUNNING Task, unchanged
  child boot `d75df727…` and recovery identities, post-fault exec success, and
  no immediate containerd disconnect journal. The same attach reader proves
  both pre-fault init streams. After release, the durable stopped record shows
  stdout offset `22→43` and stderr `26→51`, proving the replacement worker read
  and acknowledged both post-fault streams, but the attach file never receives
  them and its client never completes. TERM does not stop the exact attach
  chain; explicit KILL of only PIDs 53049/53048/53045 ends the bounded evidence
  run, whose trap removes the stopped task/container. Independent audit finds
  pool retained, every other scoped resource zero, exact candidate still
  selected, unchanged boot, and all services active with zero restarts. This
  confirms the raw bridge cannot recover the attach client's already in-flight
  `Wait` RPC; new post-fault RPCs work, but complete Task/FIFO reconnect remains
  open.
- Second source remediation makes the supervisor bridge TTRPC-frame aware. It
  retains each client stream until a terminal response and replays outstanding
  request/data frames in increasing stream-ID order to a replacement worker,
  so the killed worker's in-flight `Wait` is reconstructed before newer RPCs.
  Replay is fail-closed and bounded to the protocol's 4-MiB frame maximum, 256
  pending streams, and 64 MiB total; duplicate request IDs, data for unknown
  streams, oversized frames, and bound exhaustion are rejected. A focused test
  now kills the first worker after it consumes a request, proves the second
  receives the identical replay, and proves its response reaches the unchanged
  client connection. All runtime packages pass, and the two bridge tests plus
  existing signaled-worker test pass 25 race-detector repetitions. Live build
  and replay of this second remediation remain unclaimed.
- Commit `1dbe2d93ee722e292adfa4169c32eb66974d4847` (`runtime: replay in-flight
  shim RPCs`) freezes exactly the framed replay repair, expanded bridge tests,
  and continuous-attach harness. Findings and evidence remain uncommitted.
- Exact commit archive for the second guest build is 1,474,560 bytes/mode 0644
  with SHA-256 `00d782d6aea7784c1db4cc5ad4abbc1e3498f8e890342510d3e55f91f5b296a7`.
  It is generated directly from `1dbe2d93…`; transfer/build remain unclaimed.
- Guest independently matches full `00d782d6…` and extracts it to a new unique
  directory. Idle mkruntimed restart `52142→54167` releases the failed run's
  pool; the running daemon is still exact first-candidate `7d50218f…`, current
  selector is unchanged, and task/container/child counts are zero. Second build
  and activation remain unclaimed.
- Second guest build succeeds for all seven components with exact embedded
  revision `1dbe2d93ee722e292adfa4169c32eb66974d4847`. The mode-0644, 1,907-byte
  manifest hashes to
  `c21bd04a9faae89fc3d0bfeaf665750cb782e7b185fea0f40604e44949db5422`.
  Component hashes are shim `d01237a4…`, agent `bb2c6068…`, agentctl
  `dfbbe5e0…`, CNI `d08eca6a…`, host check `364c832a…`, mknetd `d2c82c7f…`,
  and mkruntimed `350ea5da…`. The manifest names exactly all seven components
  and every `--version` output agrees. Activation and live replay remain
  unclaimed.
- Immutable manager installation selects exact release
  `0.1.0-dev-1dbe2d93ee722e292adfa4169c32eb66974d4847`; inspect shows all six
  command links present/managed and retains both `7d50218f…` and original
  `e27ab263…`. The active shim reports the full revision and hashes to the
  build's `d01237a4…`; all four services stay active with zero restarts. A
  read of the wrong `/opt/multikernel/runtime/current` path was blank and is
  rejected; activation is established by manager inspect, with the actual
  `/usr/local/lib/multikernel/current` target to be captured in preflight.
  Live reconnect remains unclaimed.
- Replay preflight resolves the actual selector to exact release `1dbe2d93…`,
  retains boot `768706da…`, and finds zero default/moby tasks or containers,
  zero Docker containers, no shim/agent children, and all four services healthy
  with zero restarts. A hand-written curl used the wrong runtime socket and
  hashed empty input after connection failure; that value is rejected, and the
  harness's established API/resource assertions remain the pool authority.
- Framed-replay transcript `g6-forced-shim-reconnect-framed-pass.log` is
  retained exit 1 at `2026-10-01T11:44:49.271150Z`, mode 0600, 38,857 bytes,
  SHA-256
  `b2f6ca26861616e19da22a855642a821fe0834be4c2af12b1cf2f5397459fce6`.
  It proves stable supervisor 57905, worker `57911→58244`, holder
  `58097→58268`, RUNNING Task, unchanged child boot `68299503…` and durable
  identities/offsets, working post-fault exec, and no fault-time containerd
  disconnect cleanup. The replayed attach/Wait now returns normally after task
  release, but its output has only both pre-fault markers and neither post-fault
  marker; no pass is claimed. Credential scan finds only the Kerf `Cmdline`
  heading. Independent cleanup audit is retained-pool/all-other-resource-zero,
  exact selector/boot, and healthy services. Root cause is the remaining FIFO
  lifetime boundary: old-worker death closes stdout/stderr writer FDs, causing
  containerd's unchanged readers to see EOF before replacement reopen. TTRPC
  replay repairs Wait continuity but the stable supervisor must also retain
  output-writer lifetime across worker replacement. Combined G6 row stays open.
- Third source remediation transfers each identity-verified output FIFO guard
  to a minimal helper via inherited FD. Explicit normal close signals and reaps
  it immediately; worker SIGKILL yields control-pipe EOF, so it retains the
  writer endpoint without reading data for a bounded 15-second replacement
  window, then exits. The grace is bounded to 1 ms–60 s, helper launch failure
  fails I/O open, and regular-file output is unchanged. A focused crash test
  proves the raw nonblocking reader gets `EAGAIN` rather than EOF and consumes
  replacement output; 10 repetitions and the full shim package pass. Initial
  redundant paths, read-only default Go cache, and a Go-poller-based assertion
  are recorded setup/test mistakes, not product results. Live proof remains
  required.
- Keeper/reattach, framed bridge, and signaled-worker tests pass 25
  race-detector repetitions (93.574 seconds); all runtime packages pass and
  `git diff --check` is clean. Commit/build/live qualification remain required.
- Commit `1edd368f640f28e880480c638c0936a5cbaba6b0` freezes only the two
  source/test files. Exact build archive is 1,474,560 bytes/mode 0644, SHA-256
  `26c5b3d1b4617eb23a2ef33349860f15b98ab35cf622aa1e1f16a663c7a19696`;
  findings/evidence remain uncommitted and guest transfer/build are unclaimed.
- Guest independently matches mode/size/full `26c5b3d1…`, extracts to unique
  `/tmp/multikernel-runtime-build-1edd368`, and successfully builds all seven
  components plus manifest with exact embedded revision `1edd368f…`. Hash and
  activation inspection remain pending.
- Independent inspection records manifest mode 0644/size 1,907/SHA-256
  `95d45ebc5b7f57e8f2ef2bfaeff0ee44f522bc3b2a34c210fa027ee6fe8fadbd`;
  component hashes are shim `45fd9f3a…`, agent `30a10c02…`, agentctl
  `b8ca2a5f…`, CNI `cac1f808…`, host check `964e9385…`, mknetd `d5cd8aff…`,
  and mkruntimed `c7090da6…`. All seven manifest names and `--version` outputs
  agree on exact `1edd368f…`; activation remains unclaimed.
- Manager activation moves the exact selector `1dbe2d93…1edd368f…`, preserves
  all six managed links plus prior candidates/original rollback, and the active
  shim reports `1edd368f…` with build hash `45fd9f3a…`; all services remain
  healthy with zero restarts. The failed run's idle retained pool must be
  released by controlled mkruntimed restart before replay; no behavior is yet
  claimed.
- Controlled mkruntimed restart `54167→59671` releases the pool; the new PID's
  executable is exact `1edd368f…/bin/mkruntimed` with build hash `c7090da6…`.
  Selector/boot agree, Kerf reports no pool/instances, every workload and shim
  count is zero, and all services remain active with zero restarts. Replay now
  has a clean candidate-bound baseline.
- Live reconnect transcript `g6-forced-shim-reconnect-fifo-pass.log` closes
  exit 0 at `2026-10-01T11:57:02.480703Z`, mode 0600/179,780 bytes/SHA-256
  `fe6eee0d3711afc79c7153afbaba5f91efb478db139d812537018a9d383d6a18`.
  Supervisor 60078 stays fixed; worker `60083→60462`, holder `60275→60486`;
  Task remains RUNNING; child boot `60f644af…`, guest PID 162 and every durable
  recovery/network/I/O identity are unchanged; post-fault exec succeeds; and
  no containerd disconnect cleanup appears at the fault. The same attach stream
  contains both pre-fault and both post-fault stdout/stderr markers. Ordered
  Task/exec exit/delete events are captured. Two old bounded keepers briefly
  remain, expire inside the harness wait, then retained-pool/all-other-zero and
  both scoped pass markers are reached. Restart `59671→62681` releases the pool
  to full all-zero. Credential scan has no matches; independent audit binds
  exact selector/PID/hash/boot, zero resources, and four healthy zero-restart
  services. Paired with fallback pass `g6-forced-shim-reclaim.log`
  (`e833dcd1…`), both required forced-death outcomes are now substantiated.
- Focused mkruntimed restart harness is mode 0755/11,065 bytes/SHA-256
  `c914296e60b4a86eb8fc27ddc49f0c8297d14d2d1772488003c624aeac09610e`.
  It records continuous stdio, exact process/boot identities, safe durable
  lifecycle/rootfs/storage hashes and projections, service-journal metadata,
  events, and both cleanup states without printing cmdlines. Syntax/diff checks
  pass; commit `19270ec` freezes the harness/index. Live result is unclaimed.
- First focused transcript `g6-mkruntimed-restart-continuity-pass.log` is
  retained harness-only exit 1 at `2026-10-01T12:05:58.257050Z`, mode 0600/
  128,479 bytes/SHA-256 `767e79734bbb0b4633caf6a9c837d0449d3a94369cbb078cc9ed87abb830f0ae`.
  Before its terminal assertion it proves daemon `62681→63908`, stable Task/
  shim/child/recovery identities, post-restart exec, exact durable summaries,
  zero severe journal entries, and all four continuous-stream markers. Attach
  validly removed the exited task, but the harness accepted only STOPPED rather
  than ABSENT. Trap audit is retained-pool/all-workload-state-zero with healthy
  services; no pass/closure is claimed.
- Corrected STOPPED/ABSENT branch is commit `00cb1e4`; harness mode 0755/11,186
  bytes/SHA-256 `23170b264c3463f8b1b37f4ab7aba31bddf4ce50be479d6941e92a202fd9692b`.
  Idle restart `63908→65694` releases the pool with no instances/tasks/
  containers; guest copy matches and passes syntax. Rerun remains unclaimed.
- Corrected capture `g6-mkruntimed-restart-continuity-v2-pass.log` closes exit
  0 at `2026-10-01T12:10:42.454130Z`, mode 0600/136,321 bytes/SHA-256
  `e99f610e008679cda2dbabf1f615d8fcc09d0ce9d07654d19d79871005bfd6f7`.
  The deliberate restart changes mkruntimed `65694→66493` while host boot
  `768706da…`, containerd PID 14999, supervisor 66114, holder 66313, child boot
  `0b2413c0…`, guest PID 162, task `task-9511…`, recovery generation `b8c1a855…`,
  and network generation `6060fedb…` stay fixed. Journal (1,490 entries),
  snapshot (sequence 1,489/743 results), rootfs, and empty storage projections
  are byte-identical before/after; post-restart exec succeeds. The 13-entry
  service journal hashes to `b487687f…` with zero error-or-higher entries; one
  attach stream contains all four pre/post stdout/stderr markers. Timestamped
  events retain create/start, exec, init exit, and delete; attach-driven ABSENT
  is accepted. Cleanup observes retained-pool/all-other-zero, final restart
  `66493→67287`, then pool released/all-zero and the scoped pass marker.
  Independent audit resolves PID 67287 to exact `1edd368f…/bin/mkruntimed`,
  SHA-256 `c7090da6…`, unchanged boot, no Kerf pool/instances, zero shim/keeper
  processes, and all four services active/running with zero restarts. The
  focused mkruntimed-restart evidence row is closed.
- New focused `test-runtime-task-events-live.sh` is mode 0755/7,214 bytes,
  SHA-256 `5ad270835f191adda7cd97e01a19bc7eda8eff1b2b0f67a4b22d95d5660781ae`.
  It requires exec exit 17, exec/init SIGKILL exit 137, exact ordered Task v2
  create/start/exec-added/exec-started/exit/delete events with monotonic
  timestamps, and retained/released clean inventories. Bash syntax and diff
  checks pass. Commit `d7e2670` freezes only the harness/index. Guest transfer
  independently matches mode/size/full hash and passes Bash syntax. Exact
  selector is candidate `1edd368f…`; Kerf reports no pool or instances. Live
  result remains unclaimed.
- First live capture `g6-task-events-live-first.log` is retained exit 1, mode
  0600/22,037 bytes/SHA-256
  `c545888459dfeebbfe950601fe00e2ee68c65efae0cb06ca2e5d3d82eb761ef6`.
  Runtime observations reach exec exit 17 and exec/init SIGKILL exits 137.
  Validation alone fails because Python `%f` rejects containerd's seven-digit
  fractional event timestamp. No event-order pass or row closure is claimed;
  the parser must preserve nanosecond ordering without `%f`.
- Parser correction commit `836f39e` treats the fractional field as an exact
  zero-right-padded nanosecond integer (one through nine digits), preserving
  ordering without truncation. Corrected harness is mode 0755/7,362 bytes,
  SHA-256 `bb7287e22af518e902873e4d348592676964e5520facb5c2bf94e5de6371dfb2`;
  syntax/diff checks pass. Corrected live replay remains unclaimed.
- Guest corrected copy independently matches mode/size/full `bb7287e2…` and
  syntax. Controlled idle restart `67287→69091` releases the failed run's pool;
  Kerf reports no pool/instances and default tasks, containers, and runtime
  artifacts are zero. Corrected replay now has a clean baseline.
- Corrected focused transcript `g6-task-events-live-pass.log` closes exit 0,
  mode 0600/68,210 bytes/SHA-256
  `43cf0ba4422247b04c282be475e9fc2fcd3b0c42107e642a85a248161d1f3506`.
  Its exact 12-event sequence is init create/start; nonzero exec added/started/
  exit/delete; signaled exec added/started/exit/delete; init exit/delete. All
  publication timestamps are retained at their emitted fractional precision
  and validated monotonic. Exec nonzero reports status 17 in client, exit, and
  delete; exec and init SIGKILL each report 137 in client, exit, and delete.
  Provenance binds exact selector `1edd368f…`, host boot `768706da…`, daemon
  PID 69091, and containerd PID 14999. Cleanup reaches retained-pool/all-other-
  zero, restart `69091→70302`, then released all-zero and the scoped pass marker.
  Independent audit binds PID 70302 to mkruntimed hash `c7090da6…`, confirms no
  pool/instances, zero links/rules/workloads/artifacts/records/endpoints and
  exact process-name shim/NBD/relay counts zero, with four active zero-restart
  services. The exact Task v2 event evidence row is closed.
- Complete shared-matrix replay is staged from unchanged commit `94396f7`:
  matrix mode 0775/23,206 bytes/SHA-256 `3015518c…`, resize helper mode
  0775/4,841 bytes/SHA-256 `fe7059cf…`. Guest copies match both full hashes,
  pass Bash/Python checks, select exact final candidate `1edd368f…`, and start
  with no Kerf pool/instances or ctr/Docker workload. Live result is unclaimed.
- In-progress exact-candidate transcript has reached observations for image
  provenance, split create/start/state, distinct child identities, exec I/O,
  private roots, mediated networking, sibling isolation, mkruntimed restart,
  pause/resume, signal exit, normal deletion, and post-delete clean inventory.
  It is inside the read-only-bind pair; no terminal pass is yet claimed.
- The same run has now emitted `readonly-bind-inputs` and post-bind clean
  inventory for both clients and entered repeated exit-17/name-reuse cycle 1.
  The original session remains active; no terminal pass is claimed.
- Both repeated cycles now emit exact exit 17 with client stdout/stderr for ctr
  and Docker, clean normally, and reuse the same names. The same session has
  entered guest-stdin forwarding; terminal matrix status remains unclaimed.
- Guest stdin forwarding now passes for both clients; the same run has entered
  detached-task reattachment. No terminal matrix pass is yet claimed.
- Detached reattachment now passes with guest I/O for ctr and Docker. PTY
  allocation/size verification is in progress; terminal pass remains unclaimed.
- PTY mode now passes at exact `37 91` for both clients; ctr post-start resize
  has completed and Docker live resize is running. Final cleanup/pass remains
  unclaimed.
- Complete final-candidate matrix closes exit 0: transcript mode 0600/315,263
  bytes/SHA-256 `8ee2f800c96a7b49f62875f73b1b26f04d99c828570eeb4b40ed956471a10746`.
  All 19 feature rows pass for ctr and Docker; child boots `5e71eaba…` and
  `81ddc67b…` stay distinct from host `768706da…`; mkruntimed restart
  `70302→72792` preserves both. Both live resizes report exact `37 91`.
  Pre-shutdown is retained-pool/all-other-zero; restart `72792→80554` yields
  released all-zero. All five scoped assertions and the matrix marker are true;
  credential scan has no matches. Independent audit binds PID 80554 to exact
  candidate hash `c7090da6…`, finds no pool/instances, zero default/moby/Docker
  workloads, artifacts/records/endpoints/links/rules and exact shim/NBD/relay
  process counts zero, with four active zero-restart services. The complete
  replacement-instance shared matrix row is closed; explicit mount/FIFO final
  resource proof remains open.
- Dedicated final-return audit is mode 0755/4,061 bytes/SHA-256
  `bc5e2f7d52bdf095a26bea6b019187a530fb848cbafb6ff21e95ef554bdecfac`.
  It asserts no Kerf pool/instances plus zero runtime mounts, storage/bundle
  artifacts, FIFOs, links/routes/rules, workloads, durable records, and exact
  shim/NBD/relay process names; syntax/diff checks pass. Commit/live result are
  unclaimed. Commit `c27bf3b` now freezes only the audit/index; live result
  remains unclaimed.
- Final-return transcript `g6-final-resource-return-pass.log` closes exit 0,
  mode 0600/12,896 bytes/SHA-256
  `3bc958626d131aa44389eaa9f6fdc3fb694200487890a825aa01fc9e8b9d7ff5`.
  It binds selector `1edd368f…`, boot `768706da…`, PID 80554, and executable
  hash `c7090da6…`; Kerf proves no memory pool/instances, returning child CPUs
  and memory. Runtime mounts, storage/initramfs and bundle artifacts, FIFOs,
  TUN links, routes, NAT/filter rules, default/moby tasks and containers,
  Docker containers, rootfs/network records, and exact shim/NBD/relay process
  counts are all zero. Four services remain active/running with zero restarts;
  scoped pass marker is present and credential scan is empty. The final
  resource-return row is closed.
- Shim qualification now accepts only explicit worker signal `KILL` (default)
  or `TERM`; `TERM` reuses the complete reconstruction/stdio/identity/event/
  cleanup assertions and emits a distinct clean-restart marker. Updated harness
  is mode 0775/15,953 bytes/SHA-256 `703acc35482b10970b0d1d289c66a360360b712de655dab5280d7173e92a9871`;
  syntax/diff checks pass. Commit/live result remain unclaimed.
  Commit `2ce142b` freezes the harness change; live result remains unclaimed.
- Guest clean-restart harness independently matches mode/size/full `703acc35…`
  and syntax. Exact selector `1edd368f…`, no pool/instances, and zero ctr/Docker
  workload establish a released baseline. Live result remains unclaimed.
- First `TERM` capture terminates harness-only exit 1 at 27,703 bytes/SHA-256
  `8d1c209cf3952cc76a87733bf1d8898be5c6e2edac5bd61405c8f690addbb445`:
  orderly worker termination removes the Task instead of reconstructing it in
  place, so the forced-reconnect expectation is inapplicable. The subsequent
  host reboot removed the `/tmp` transcript before local transfer; only the
  terminal metadata/output already observed are retained. It is diagnostic,
  not closure evidence. Clean restart must assert orderly shutdown followed by
  same-name shim/task recreation and cleanup.
- Post-reboot audit records new host boot `d9cdfa98-df65-4bce-b3a2-08565857c69e`,
  unchanged kernel and exact selector `1edd368f…`; mkruntimed/containerd/Docker/
  mknetd are active with zero restarts, Kerf has no pool/instances, and both
  containerd namespaces plus Docker are empty. This is the new live baseline.
- Corrected clean-shim harness is mode 0755/6,608 bytes/SHA-256
  `84e982801a17ec534eaed0d9d97ba4cb785e08de278e0a7320f5d8c41aebafdb`.
  It runs two same-name clean lifecycles, requires old supervisor/worker/holder
  exit, distinct replacement shim and child identities, both event sequences,
  and retained then released cleanup. The forced-death harness is restored
  byte-exact to its pre-`TERM` source (`e47804f5…`). Syntax/diff checks pass;
  commit `a8cdc1c` freezes the separation; live result remains unclaimed.
- Guest corrected harness independently matches mode/size/full `84e98280…` and
  syntax on exact selected candidate `1edd368f…`; Kerf has no pool/instances
  and ctr/Docker inventories are empty. Live result remains unclaimed.
- First corrected-harness capture `g6-clean-shim-restart-v2-first.log` is
  retained pre-workload exit 1, mode 0600/12,987 bytes/SHA-256
  `54465f4b323f572cfc4a6d9ba1366c43a2b94cb91781ccff70c1994001021724`.
  Bash expanded the stream path before assigning same-statement local `cycle`
  under `set -u`; no task was created and cleanup kept the released baseline.
  Splitting the declarations corrects only the harness; no product claim.
- Correction commit `29af02b` produces mode 0755/6,616-byte harness SHA-256
  `80ddbd858ae928007fd6b34d03e30c35b7aade008955ec75901c9ac578a18ff5`;
  syntax/diff checks pass. Corrected live result remains unclaimed.
- In-progress corrected run completes cycle 1 with normal output, clean exit of
  supervisor/worker/holder, and retained-pool/all-other-zero inventory. The
  same task name is accepted for cycle 2; no terminal pass is yet claimed.
- Corrected clean restart transcript `g6-clean-shim-restart-pass.log` closes
  exit 0, mode 0600/80,418 bytes/SHA-256
  `754a05f60c0bb906aa9ad2c6c5ff8d2e971b36e10f541f7fde03e69c5f201de8`.
  Cycle 1 shim identities 3012/3017/3213 and child boot `d9282f53…` exit; the
  same task name creates distinct 3514/3519/3763 and child `69a9ff52…`; both
  attach streams contain start/exit markers and both event lifecycles are
  retained. Inventories converge retained-pool/all-other-zero after each;
  restart `1466→4061` releases the pool to all-zero. Credential scan is empty.
  Independent audit binds PID 4061 to exact candidate hash `c7090da6…`, no
  pool/instances/workloads/artifacts/shim/NBD/relay processes, and four healthy
  zero-restart services on reboot `d9cdfa98…`. Combined with containerd, Docker,
  mkruntimed, forced reconnect, and bounded reclaim transcripts, the aggregate
  restart matrix and ownership-transfer row are closed.
- Expanded signal/event harness is mode 0755/8,800 bytes/SHA-256
  `b09bf5209dcee122d056ea31675cc06201794d10a4a7ddcfe2ff530e3f21a525`.
  It now sends and observes ignored SIGTERM for exec and init, SIGKILLs an exec
  process group and proves its descendant PID gone, retains attach wait/delete
  behavior, recreates the same task name after init SIGKILL, and requires two
  exact init event lifecycles plus released cleanup. Syntax/diff checks pass;
  commit `1fb265d` freezes the harness; live result remains unclaimed.
- In-progress live run proves exec exit 17, ignored exec SIGTERM/no exit event,
  exec-group SIGKILL exit 137 with descendant gone, ignored init SIGTERM while
  RUNNING, and init SIGKILL exit 137 through the attached waiter. Same-name
  recreation after signal failure is running; no terminal pass yet claimed.
- Expanded transcript `g6-signal-lifecycle-pass.log` closes exit 0, mode 0600/
  78,247 bytes/SHA-256
  `6647cc0e15d68dd6b78ed8c29e915ffa685df4f4126938961ba9049654f627b8`.
  Exec SIGTERM leaves its client waiting and publishes no exit; exec SIGKILL
  returns 137 and guest descendant PID 177 is absent. Init SIGTERM leaves Task
  RUNNING/no exit event; init SIGKILL returns 137 through attach, whose normal
  completion drives delete. The identical task name then runs successfully.
  Twenty monotonic timestamped events contain two exact init lifecycles.
  Retained cleanup reaches all-other-zero and restart `4061→6187` releases the
  pool. Combined with repeated nonzero-17/name-reuse in final matrix
  `8ee2f800…`, every named signal/exit/reuse variant is live-proven. Credential
  scan is empty; independent audit binds PID 6187 to hash `c7090da6…`, all
  workload/artifact/helper counts zero, and four healthy zero-restart services.
  The aggregate signal/exit/churn row is closed.
- New concurrent-churn harness is mode 0755/9,148 bytes/SHA-256
  `d425cc399297954edb34fa6f88104b37f9507bca7f76bb948262d71049982e48`.
  It starts ctr/Docker concurrently; projects durable lifecycle/recovery state
  and requires disjoint CPUs, memory owners, generations, bundles, storage,
  agent ports/CIDs/sockets, task and network identities; runs 12+12 parallel
  execs and simultaneous pause/resume; then audits retained/released cleanup.
  Syntax/diff checks pass. Commit and live result remain unclaimed.
  Commit `9616398` freezes the harness/index; live result remains unclaimed.
- Guest concurrency harness independently matches mode/size/full `d425cc39…`
  and syntax; Kerf has no pool/instances and ctr/Docker inventories are empty.
  Live result remains unclaimed.
- Concurrent capture `g6-concurrent-churn-pass.log` closes exit 0, mode 0600/
  96,522 bytes/SHA-256
  `09a0c135eeb56c2ac9b1bec45038754e615c444faf66862f689fc612124ec543`.
  ctr/Docker use disjoint CPUs `[8,10]`/`[12,14]`, separate 3-GiB memory
  allocations, generations `474f49fe…`/`12724968…`, bundles, agent ports
  7200/7201, child CIDs 40/41, socket inodes 3116/3189, storage paths and ports
  4061/4062, task identities, network generations and addresses
  `172.31.0.2/30`/`.6/30`. Both complete 12 parallel execs, simultaneous pause/
  resume, and retain distinct child boots. Cleanup reaches retained-pool/all-
  other-zero; restart `6187→9410` reaches released all-zero and scoped pass.
  Credential scan is empty. Independent audit binds PID 9410 to candidate hash
  `c7090da6…`, finds no pool/instances/workloads/artifacts/helpers, and four
  healthy zero-restart services. The concurrent-churn row is closed.
- 2026-10-02 resumed-VM checkpoint: direct observation after the operator
  restart records boot `d9cdfa98-df65-4bce-b3a2-08565857c69e`, kernel
  `7.0.0-mk2-gce-lab`, selector `1edd368f640f28e880480c638c0936a5cbaba6b0`,
  and mkruntimed PID 9410 with executable SHA-256 `c7090da6…`. mkruntimed,
  containerd, Docker, and mknetd are all active/running with `NRestarts=0`.
  This is a continuity checkpoint only; it does not promote an unchecked row.
- The first resumed focused Go-test enumeration used isolated `/tmp` build and
  module caches but stopped before compilation: the cache was empty and the
  filesystem sandbox denied DNS access to `proxy.golang.org`. No test result or
  completion claim is derived from that diagnostic; the identical operation
  must be repeated with approved dependency access.
- Approved dependency access then enumerated 128 top-level shim tests. The full
  package race baseline is retained as `g6-task-v2-unit-race-baseline.log`,
  mode 0600/53,953 bytes/SHA-256
  `d1b5fa5e6a6e69c037e74d6b614191027a69256bd8e20c388880261ab02614ee`.
  It closes exit 0 in 8.516 s with 327 `RUN` entries and 138 passing groups.
  Two real pathname-socket tests skip because the local sandbox forbids Unix
  pathname listeners; they require a disposable-VM rerun. This is a clean
  baseline, not evidence that the deliberately exhaustive aggregate rows are
  complete.
- Pre-transfer disposable-VM check records Go 1.26.0/linux-amd64, 32 GiB free
  on `/tmp`, and confirms unique destination
  `/tmp/multikernel-g6-source-9616398` is absent. The installed release and
  service state will not be modified by the source-only qualification copy.
- Source transfer provenance: the VM copy contains 100 files/45,747,628 bytes;
  local and remote SHA-256 values match for `main_test.go` (`6051decb…`),
  `go.mod` (`407622e6…`), and `go.sum` (`9f40acd1…`). The VM suite therefore
  exercises the same source and dependency lockfiles as the recorded baseline.
- First VM race run is retained as `g6-task-v2-vm-race-first-fail.log`, mode
  0600/55,813 bytes/SHA-256
  `9a80ebc2df7d13c598afc7d5705ec4d99c860165a6515b2114ca34b4e4f6b579`.
  It exits 1 with 327 run entries, 135 passing groups, zero skips, and five
  failures: unsafe token mode, group-accessible FIFO, unsafe journal mode,
  deterministic socket cleanup, and authenticated fallback socket cleanup.
  No row is promoted. The shared permission/ownership validation boundary and
  VM `/tmp` mount semantics must be diagnosed before rerun.
- Failure diagnosis: the remote wrapper's `umask 077` rewrote requested test
  fixture modes 0644/0660 to 0600. Thus the three deliberately unsafe fixtures
  became safe, while two containerd address fixtures requiring exact 0644
  became invalid. The product validators behaved consistently; the wrapper
  must use ordinary `umask 022` and separately `chmod 0600` only its transcript.
- Corrected VM transcript `g6-task-v2-vm-race-pass.log` is mode 0600/53,765
  bytes/SHA-256
  `ff7cc0e0f49e6334dd7948b1cb12c8df356140cda6aba01c6e31349d849a10db`.
  It closes exit 0 under `-race -count=1` in 8.294 s with 327 `RUN` entries,
  140 passing groups, zero skips, and no failures. Both pathname-socket tests
  skipped locally execute and pass on the disposable VM. This substantiates
  the current focused suite but does not by itself prove every combination
  demanded by the remaining exhaustive matrix rows.
- Added `TestEventJournalCompleteLifecycleOrderAndPersistenceFailureMatrix`.
  It queues create/start/exec-added/exec-started/exit/delete while every broker
  attempt fails, verifies durable sequences 1..6, reconstructs and replays the
  exact order, and proves the acknowledged journal disappears. Six subtests
  independently inject persistence failure before each topic and require zero
  publication plus exact queue/sequence rollback. Twenty race-detector
  repetitions pass in 1.435 s; full-suite and VM reruns remain pending.
- Updated local full race transcript `g6-task-v2-unit-race-event-matrix.log`
  is mode 0600/55,752 bytes/SHA-256
  `54c55b82d1f126151cd4ed91b099ce53461bceebbc90daff4d761d3621cb6949`.
  It exits 0 in 8.509 s with 334 run entries and 139 passing groups; only the
  same two pathname-socket cases skip under the local sandbox. VM rerun remains
  required before promoting the event row.
- Repository-wide cancellation inventory finds direct tests in all named
  domains: rootfs/storage mutation and hash loops, bounded builder descendants,
  daemon dial/write/read/default timeout, Kerf child boot commands, agent
  connect/call, stdio open/pumps, Task locks/wait, and teardown/network cleanup.
  Full local `runtime/...` race transcript
  `runtime-all-packages-race-cancellation-baseline.log` is mode 0600/144,982
  bytes/SHA-256
  `939368f0806b3e64a6e0c8d0fa5d20a90e94abc1a628f96ae647ac83b95902ea`.
  It exits 0 with 968 run entries, 466 passing groups and 22 tested packages;
  11 Unix-socket/descriptor tests skip under the local sandbox. VM zero-skip
  qualification and a live cross-service cancellation/leak run remain open.
- First full-module VM transcript
  `runtime-all-packages-vm-race-cancellation-first-fail.log` is retained mode
  0600/143,337 bytes/SHA-256
  `78117ffdcebcbb018694513cab3bceca425850f5bc657f0914ceef6e9a4f5af1`.
  It exits 1 with 959 run entries, 468 passing groups and zero skips. Exactly
  nine `internal/storage` tests fail before their target assertions because
  source/test execution under VM tmpfs makes allocated image extents
  uninspectable; the backend correctly rejects them as sparse/unverifiable.
  A persistent-disk workspace rerun is required; no row is promoted.
- VM filesystem check confirms `/tmp` is tmpfs while `/var/tmp` is `/dev/root`
  ext4. Exact copied source at `/var/tmp/multikernel-g6-source-dbdaf1b`
  retains `main_test.go` SHA-256 `10101b2d…` and occupies 45,754,932 bytes.
  The rerun must also set `TMPDIR` to a unique ext4-backed directory so Go test
  fixtures, not merely the source, obtain inspectable extents.
- Ext4-backed rerun `runtime-all-packages-vm-race-cancellation-ext4-fail.log`
  is retained mode 0600/144,996 bytes/SHA-256
  `87856460975953392b6c1f554db5cc02a1734a248f55112050e1ffa19ee499d5`.
  It reaches 968 run entries, 472 passing groups and zero skips; all storage
  tests now pass. Five Unix-socket tests fail with `bind: invalid argument`
  because the long ext4 `TMPDIR` plus generated test names exceeds the Linux
  Unix-socket pathname limit. The final rerun must retain ext4 while using a
  short unique test-temp prefix; no product claim is derived from this run.
- Final short-ext4-`TMPDIR` rerun passes exit 0 in 12 s with 968 run entries,
  477 passing groups, all 22 tested packages and zero skips/failures. Retained
  transcript `runtime-all-packages-vm-race-cancellation-pass.log` is mode
  0600/144,078 bytes/SHA-256
  `94f077c3b6837ef90f8416202f0779c4bd3bba0957c3b9b5b1368f737fc4edb0`.
  The automated cancellation/deadline row is closed; live leak qualification
  remains explicitly open.
- Live OCI pre-allocation qualification will use containerd's
  `--apparmor-profile multikernel-deliberately-unsupported`, not an annotation:
  the latter is intentionally supported, while the former reaches the
  canonical `process.apparmorProfile` rejection. Before injection, the
  restarted candidate VM reports no configured pool, child kernel, workload,
  or runtime-tree entry. `test-runtime-oci-preallocation-live.sh` records the
  exact candidate identities, compares a 19-category inventory before and
  after rejection, runs a supported positive control, releases the reusable
  pool, and repeats the zero-resource audit. Execution evidence remains
  pending; no row is promoted by harness construction.
- The first live OCI harness attempt stopped before workload injection: it
  assumed `/usr/local/bin/mkruntimed`, while the unit's `ExecStart` and public
  command are `/usr/local/sbin/mkruntimed`. Retained mode-0600 transcript
  `g6-oci-preallocation-live-first-harness-fail.log` is 3,064 bytes/SHA-256
  `95a5a7207b75bfae14aaee279706627735bd440eef815173b2c9fb088b645d21`.
  This is explicitly a harness-path failure with no allocation claim; the
  corrected script binds the public version check to the actual unit path.
- A second preflight-only attempt identifies the installed shim spelling as
  `containerd-shim-multikernel-v2`, not `containerd-shim-mk-v2`. It stopped
  before injection and is retained as
  `g6-oci-preallocation-live-second-harness-fail.log`, mode 0600/3,761 bytes/
  SHA-256 `7bec72a3b20f87928256159de14d58d082e2cfbed3e64893cf8d5c2a6e0c1334`.
  The corrected harness resolves and records the selected generation behind
  `/usr/local/bin/containerd-shim-multikernel-v2`; no product claim comes from
  this provenance failure.
- A third preflight-only attempt passes exact revision/daemon/shim/builder
  identity checks but finds that this containerd CLI lacks `ctr images info`.
  Retained `g6-oci-preallocation-live-third-harness-fail.log` is mode 0600/
  8,305 bytes/SHA-256
  `863512bd7826f7cef09c6ac604e079f487bf11b1becdeccf6c55725e32f92311`.
  Exact `ctr images list -q` output independently confirms busybox 1.36 is
  present and now supplies the compatible preflight. Injection was not reached,
  so the transcript promotes no row.
- The first attempt to exercise product behavior reaches the precise
  `validate OCI bundle before allocation` AppArmor rejection. At its immediate
  audit the pool is still absent and every allocation-bearing category is zero,
  but two shim processes are still being reaped; both disappear before the
  follow-up inspection with no task/container/directory residue. The harness
  correctly exits 1. Retained
  `g6-oci-preallocation-live-fourth-transient-shim-fail.log` is mode 0600/
  28,379 bytes/SHA-256
  `6503a004b20fe324d6e3a6306af83229a437777c36d7e418b69fdc4a0eba070c`.
  The revised harness keeps the immediate no-allocation assertion, then applies
  a bounded 60-second reap deadline and demands exact zero before continuing.
- Corrected live transcript `g6-oci-preallocation-live-pass.log` exits 0 and is
  retained mode 0600/129,202 bytes/SHA-256
  `c409237a4041960a6cd0acfb506ba89387a450fe0ba0f6ff8672a4b2da7c7cd3`.
  Seven observation blocks bind exact `11a65f08…` daemon/shim/builder identity,
  initial zero state, canonical pre-allocation rejection, immediate absence of
  every allocation-bearing resource, bounded shim reaping to exact 19-category
  zero, `MK_OCI_SUPPORTED_PASS`, and final exact zero after reusable-pool
  release. mkruntimed/mknetd/containerd/Docker remain active/running with zero
  restart counts. The private transcript has zero credential-pattern matches.
  This completes the pre-allocation clause only; deterministic later partial-
  allocation/application boundary injection remains required, so the composite
  checklist row stays open.
- The follow-on source audit names the second clause's missing matrix rather
  than treating it generically. After canonical validation, failures can occur
  during rootfs preparation, runtime-directory identity handoff, token
  acquisition, ambiguous sandbox create/cancel, sandbox load, network
  provision, namespace-holder creation, recovery persistence, or create-event
  publication. Rollback must stop the holder, release the owned endpoint,
  delete the exact sandbox generation, clean the prepared root, clear token,
  and remove process state as applicable. Only pre-allocation rejection and one
  ambiguous-create cancellation case currently have focused coverage; an
  injected every-stage rollback matrix is still required.
- `TestCreatePostValidationFailureRollbackMatrix` now injects each of those
  nine stages and passes once under `-race`. It asserts exact root cleanup,
  cancellation/deletion/release/holder-stop calls according to acquired
  ownership and zero surviving shim state or runtime directory. The first two
  invocations are non-evidence: one used incorrect repo-root paths plus the
  sandbox's read-only default Go cache, and the next stopped at a missing
  test-only `slices` import. The corrected compile/run passes all subtests in
  1.049 s. Repeated race execution, full suites, commit binding, and VM rerun
  remain mandatory before closing the composite row.
- Twenty race repetitions pass all nine subtests (180 deterministic injected
  stage executions) in 1.431 s. The entire shim package then passes
  `go test -race -count=1` in 9.582 s. Full-module execution and exact-commit
  disposable-VM qualification remain pending.
- Full local runtime `go test -race -count=1 ./...` passes all 22 tested
  packages. The changed shim package completes in 9.561 s, with agent/rootfs/
  storage at 10.432/1.722/3.793 s. This is a local baseline only; exact commit
  transfer and zero-skip VM execution remain required.
- Focused commit `3fd1238` freezes only the nine-stage matrix at full revision
  `3fd1238667899b0a6c721f14685cef4253556990`; `main_test.go` SHA-256 is
  `374f8a3ade7ac99964df5c0e3b1d16f57a98ca4a0ad89f6a6cad1feee8f0f763`.
  Documentation/evidence remain separate. Exact-commit disposable-VM transfer
  and execution are still pending.
- Exact archive SHA-256 `43e23a00…` matches across transfer; the fresh ext4 VM
  extraction contains 638 files and exact `main_test.go` hash `374f8a3a…`.
  The first full VM run repeats the known wrapper error `umask 077`, converting
  intentionally permissive fixtures to 0600 and invalidating unsafe-mode plus
  exact-0644 socket/address cases. The new nine-stage matrix itself passes.
  Retained `g6-oci-postvalidation-rollback-vm-race-first-umask-fail.log` is mode
  0600/147,009 bytes/SHA-256
  `59b40720f69b02bfc46425418b2b278c3395a301156eddedc7107b1461976197`,
  with 981 run entries, 965 pass lines, zero skips, and overall exit 1. It is
  explicitly non-evidence pending an ordinary-umask rerun with a presecured
  transcript.
- Corrected exact-commit VM transcript
  `g6-oci-postvalidation-rollback-vm-race-pass.log` exits 0 and is retained mode
  0600/146,004 bytes/SHA-256
  `20a3f6a2079a7a63536318c4b0ebebf5ed8d1e61318128f84b4f2b54e6e73652`.
  It contains 981 run entries, 981 pass lines, all 22 package results, zero
  skips, and zero failures. The nine-stage matrix executes and passes on ext4,
  proving cleanup after prepare, handoff, token, create/cancel, load, network,
  holder, recovery-persistence, and event-persistence failures. Combined with
  installed-candidate live transcript `c409237a…`, which proves canonical
  unsupported OCI rejection before allocation plus supported-path success and
  exact final zero resources, both clauses of the composite row are now closed.
  Credential-pattern scanning of the VM transcript returns zero matches.
- Independent final audit `g6-oci-final-resource-audit.log` exits 0 and is mode
  0600/12,607 bytes/SHA-256
  `29340822f6c5c5078bd078ac3fa1358cc052b52fd1524199e369158b25788e5f`.
  It binds mkruntimed PID 19810 to selected `11a65f08…`/SHA `0e1c87c3…`, proves
  no pool or child and all 19 resource categories zero, and records all four
  services active/running with `NRestarts=0`. Its credential scan is empty.
  After closing the OCI row, top-level checklist totals are 34 checked and 51
  unchecked; overall G4-G6 remediation is therefore still incomplete.
- `scripts/check-docs.sh` passes after the closure update: link structure,
  schemas, current evidence manifests, 95-case OCI validation, supporting
  runtime fixtures/managers, GCE ledger, capture, containerd config, and final
  evidence audit all pass. Its one socket fixture is explicitly skipped under
  local sandbox `EPERM`; exact-commit VM Go evidence above has zero skips.
- Packaging is the next closure target. The first live preflight performs no
  mutation: manager CLIs are not exposed at the assumed public libexec paths.
  Current selectors remain binary `11a65f08…` and support `b4d185c6…`;
  mkruntimed PID 31453 hashes `0e1c87c3…`, reports exact revision `11a65f08…`,
  and has `NRestarts=0`. Exact-source binary-manager inspection finds every
  managed link valid and 15 preserved releases. Ordinary-user support-manager
  inspection correctly rejects root-owned `/etc`, so it is not activation
  evidence; the next step is a root-owned, hash-verified temporary manager copy
  for privileged inspection and reversible rollback/forward restoration.
- The retained coherent rollback pair is now verified: binary `1edd368f…`
  hashes daemon/shim/mknetd to `c7090da6…`/`45fd9f3a…`/`d5cd8aff…`, while
  support `25e6d343…` hashes its builder to `0ded581c…`. ctr/Docker are empty.
  Root-owned manager copies match exact-source hashes `39026a02…`/`ec38d8d2…`;
  privileged support inspection sees seven deployments and all 21 links valid.
  Fail-safe harness commit `6194e02` (full `6194e02bc2b9e5596075b2bc5fbf3fd2be311863`)
  has script SHA-256 `642e382b…`; no selector mutation is claimed until its
  live run completes.
- The first harness run stops before mutation because the root-owned binary
  manager copy lacked sibling `runtime-release-manifest.py`. Candidate selectors
  and all four service PIDs/states/restart counts remain unchanged. Retained
  `g6-packaging-rollback-live-first-manager-dependency-fail.log` is mode 0600/
  4,697 bytes/SHA-256
  `1feda951d90987977926ad6fc18b179d84a58fc72dd495c4f17e413168ecfcf2`.
  The root-owned mode-0644 dependency now matches exact-source SHA `4ae16ce4…`;
  binary inspection succeeds with candidate active, 15 releases, and every
  link present/managed. This tooling stop supplies no rollback evidence.
- Restarted-instance preflight is also non-mutating and passes: candidate
  binary/support selectors remain `11a65f08…`/`b4d185c6…`; mkruntimed, mknetd,
  containerd, and Docker are active with zero restarts. The uploaded qualifier
  matches commit `4c33b7198209560d7b36ace692576031158f65cb` and SHA-256
  `1c8f836b…`; both root-owned manager CLIs are 0755 and their required 0644
  sibling retains exact SHA `4ae16ce4…`. This commit removes a harness-only
  `grep -q`/`pipefail` SIGPIPE hazard. Rollback/forward execution remains open.
- The corrected harness safely activates the retained rollback generation and
  verifies exact selectors, revision `1edd368f…`, running daemon/public shim/
  support-builder hashes `c7090da6…`/`45fd9f3a…`/`0ded581c…`, active services,
  and unchanged Docker default `runc`. It then exposes a real packaging gap:
  `containerd config dump` has no named multikernel runtime because
  `/etc/containerd/config.toml` is absent, leaving the managed `conf.d`
  fragment unloaded. The trap restores candidate `11a65f08…`/`b4d185c6…`;
  follow-up proves candidate daemon SHA `0e1c87c3…`, all services active with
  zero restarts, empty ctr/Docker inventories, and no pool/child. Retained
  `g6-packaging-rollback-live-second-containerd-config-fail.log` is mode 0600/
  27,566 bytes/SHA-256 `1dc4121d934eeb8b672ae038cb648d5c5bf58a28bafa9b5592e0c8e3cb915aa3`.
  No closure claim is made; containerd config integration must be fixed first.
- The gap is the documented fresh-host host-configuration step, not a need to
  synthesize a partial product config: containerd 2.2.2's generated complete
  default already imports `/etc/containerd/conf.d/*.toml`. Its candidate is
  validated before atomic mode-0644 installation and proves default `runc`,
  `io.containerd.runc.v2`, and named multikernel type
  `io.containerd.multikernel.v2`. Installed main config SHA is `54a1d02d…` and
  selected versioned fragment SHA is `54c85792…`. The restarted live dump
  proves the same mapping, Docker remains default `runc`, all default/moby task
  and container inventories stay zero, and containerd has `NRestarts=0`.
  `g6-containerd-fresh-host-config-pass.log` is mode 0600/3,225 bytes/SHA-256
  `20417ecae26f2c7863bd9742634ddf540e3ef8e816e37207e3ef2ff8a5fb4437`;
  credential scanning is empty. Generation rollback/forward remains open.
- The next full run proves the rollback selectors/hashes and imported named
  runtime, then remains non-passing because ctr races RPC socket publication.
  Systemd declares the old daemon active at monotonic 521.708 s; the create
  sees `/run/mkruntimed.sock` absent and the trap starts restoration at
  522.132 s. Candidate selectors and daemon SHA `0e1c87c3…` are restored;
  independent follow-up finds both sockets, all four services active with zero
  restarts, six empty ctr/Docker inventories, and no pool/child. Retained
  `g6-packaging-rollback-live-third-service-readiness-fail.log` is mode 0600/
  30,790 bytes/SHA-256 `ef40e1cd9323ca3770f8337c0d7b0c3f8ade5d650126de9dd4ce7168a90c22fc`.
  The harness must await both runtime sockets after activation; no workload or
  packaging closure is claimed from this run.
- Exact qualifier commit `4db2d9cdf7b1bc8d3be354d636ff82032d3e7541`
  now waits up to 30 seconds for both Unix sockets, asserts their type, and
  records type/mode/ownership for each activated generation. Syntax/shellcheck/
  diff checks pass and script SHA-256 is `2fa3188d…`; live rerun remains open.
- Exact `4db2d9c` live qualification exits 0 across the full rollback/forward
  cycle. It proves old `1edd368f…`/`25e6d343…` and restored candidate
  `11a65f08…`/`b4d185c6…` selectors, revisions and daemon/shim/builder hashes;
  ready 0660 root-owned sockets; named containerd runtime; Docker default
  `runc`; `MK_PACKAGING_ROLLBACK_PASS` and `MK_PACKAGING_FORWARD_PASS`
  workloads; and exact 19-category zero cleanup after each workload. Final
  services are active/running with zero restarts. Private
  `g6-packaging-rollback-live-pass.log` is mode 0600/126,393 bytes/SHA-256
  `15a1d4d931f8047bafda0dd3f42a55e18a9601723ec3246ce0fc067a8641fb59`;
  wrapper exit is 0 and credential scanning is empty.
- Independent `g6-packaging-final-resource-audit-pass.log` exits 0 and is mode
  0600/12,601 bytes/SHA-256
  `c5748c810df83094cc504207099b42c3ac16ae553cb21b05c16f91c5c0519d6b`.
  It binds PID 6485 to candidate selector/hash `11a65f08…`/`0e1c87c3…`, proves
  no pool/child plus all 19 resource categories zero, and records all four
  services active/running with `NRestarts=0`; credential scanning is empty.
  One narrow live configuration/service-dependency audit precedes row closure.
- The first narrow audit is non-passing only because it resolves the CNI link
  without privilege across root-private deployment ancestry. Privileged
  follow-up proves the link valid into the candidate deployment and its target
  mode 0644/SHA-256 `7de30fd1…`; selectors, active services, and empty
  inventories remain unchanged. Retained
  `g6-packaging-config-service-audit-first-permission-fail.log` is mode 0600/
  21,903 bytes/SHA-256 `88f4c0607677f364f5e37408e7dffe7c318750cecf1ebf2554c6305ab87dccac`
  with no credential matches. It supplies no closure claim; corrected sudo
  resolution is required.
- Corrected `g6-packaging-config-service-audit-pass.log` exits 0 and is mode
  0600/14,018 bytes/SHA-256
  `72012fcb4901f8d08df8fcd34675f57f74e4cac54c63391e4515795e11ed2e2f`.
  Live containerd and Docker retain default `runc` plus the named opt-in
  runtime; four public binary/CNI paths resolve into release `11a65f08…`, and
  unit/containerd/CNI support paths resolve into deployment `b4d185c6…`.
  Systemd proves the multikernel mount, guest-agent, storage-mount, and network
  dependencies/orderings before containerd and Docker; all services remain
  active/running with zero restarts. Credential scanning is empty. Packaging
  tests and the full repository evidence gate remain before checking the row.
- Packaging closure gates pass: Docker config 6/6, deterministic release
  manifest, immutable binary lifecycle, immutable support-deployment lifecycle,
  and containerd import parsing all pass. `scripts/check-docs.sh` also passes
  links, 7 schemas/22 cases, 17 current evidence manifests, 95 OCI cases, all
  supporting runtime fixtures/managers, GCE ledger, capture, containerd config,
  and final evidence audit. Its explicit local socket `EPERM` skip is unrelated
  to the privileged live evidence. Manager tests plus fresh-host `20417eca…`,
  rollback/forward `15a1d4d9…`, final-audit `c5748c81…`, and config/service
  `72012fcb…` evidence cover every packaging clause, so the composite row is
  checked. Totals are now 35 closed and 50 open; broader G4-G6 work continues.
- G4 initramfs reproducibility is the next target. New fail-fast qualifier
  `scripts/test-runtime-initramfs-repro-live.sh` builds twice across mtime and
  sparse/dense differences, verifies byte-identical archive/manifest output and
  normalized mode/hardlink/symlink metadata, demands a changed-input digest
  control, then runs all 19 corruption/capacity/publication tests. Its local run
  passes with archive/manifest SHA `9b2a4f20…`/`cfca02b0…`, changed-control SHA
  `7febef49…`/`a29b223d…`, and 19/19 tests; only the known local socket `EPERM`
  subcase is explicitly skipped. Script SHA is `f1dabbbf…`; VM proof is open.
- Clean commit `7725e171da2a88deeb8be8d9f7d902084ad96667` archive is
  4,730,880 bytes/SHA-256 `ecfbf93e…` and matches on the VM. Fresh ext4 source
  contains 640 files; qualifier/builder/verifier/suite hashes are
  `f1dabbbf…`/`e1c7234d…`/`2cfe4ad8…`/`f4ecf345…`. Execution remains open.
- Exact-commit VM execution exits 0 with zero skips. Two ext4 builds across
  mtime and sparse/dense changes independently verify and match byte-for-byte
  at archive/manifest SHA `2532e1b5…`/`bf6f3e04…`; normalized newc metadata,
  modes, hardlinks, symlink, sparse/xattr/device policies are audited. A content
  change diverges at `5850d990…`/`fd059b19…`, and all 19 rejection, corruption,
  capacity, exclusive-publication, peer-rollback, and replacement-preservation
  tests pass. `g4-initramfs-repro-live-pass.log` is mode 0600/9,148 bytes/
  SHA-256 `5a1d5ae6765e6bbbfc16218b5c020cdb7ccdf98c9de92490a5d8c00b6165dbf3`
  with no credential matches.
- Independent `g4-initramfs-final-resource-audit-pass.log` exits 0 and is mode
  0600/12,601 bytes/SHA-256
  `c5748c810df83094cc504207099b42c3ac16ae553cb21b05c16f91c5c0519d6b`.
  Candidate PID/hash are unchanged, no pool/child and all 19 counters are zero,
  and four services remain active/running with zero restarts. Credential scan
  is empty. The reproducibility row is checked.
- The post-closure full gate passes links, 7 schemas/22 cases, 17 current
  manifests, 95 OCI cases, all runtime fixture/manager suites, GCE ledger,
  capture, containerd, and final evidence audit. Its documented local socket
  `EPERM` skip is covered by the zero-skip VM run. Totals are 36 closed and 49
  open; generated bytecode has been removed and broader G4-G6 work continues.
- Unsafe-input closure is being audited separately. New verbose driver
  `scripts/test-runtime-g4-input-boundaries-live.sh` passes locally across 7
  root canonical/traversal/symlink/held-root cases, 19 archive metadata/type/
  hardlink/mutation/publication cases, 10 ext4 capacity/copy/collision cases,
  and bind source/target mutation semantics. Local socket creation retains the
  known sandbox `EPERM`; script SHA is `edd56360…` and root VM proof is open.
- Clean commit `e81d8ca2d0a31a1df2cbe8f438021a991006f1c0` archive is
  4,730,880 bytes/SHA-256 `35b2d4a4…` and matches after transfer. Its fresh
  ext4 extraction has 641 files and exact driver SHA `edd56360…`; run pending.
- Exact-commit root VM run exits 0 with zero skips. Its 36 named tests comprise
  7 canonical/traversal/symlink/allowlist/held-root cases, 19 escaping-link/
  FIFO/socket/device/whiteout/xattr/malformed-opacity/external-hardlink/
  mutation/corruption/capacity/publication cases, and 10 ext4 exhaustion/
  metadata/collision/held-source-copy cases. The bind matrix additionally
  passes copy/type/mode/hardlink/symlink, source/target races, mutation, limits,
  and writable-seed isolation. `g4-input-boundaries-live-pass.log` is mode
  0600/7,091 bytes/SHA-256
  `81b7078a2b3e084a481312f3a183a0983efbf316b2bb8eb90c0b5fc49cea298f`
  with no credential matches.
- Independent `g4-input-boundaries-final-resource-audit-pass.log` exits 0 and
  is mode 0600/12,601 bytes/SHA-256
  `c5748c810df83094cc504207099b42c3ac16ae553cb21b05c16f91c5c0519d6b`.
  Candidate identity is unchanged, all 19 counters and pool/child are zero,
  and four services are active/running with zero restarts. Credential scan is
  empty. The composite unsafe-input row is checked.
- The post-closure full repository gate passes again across schemas/manifests,
  95 OCI cases, G4 fixtures, managers, GCE ledger, capture, containerd, and
  final evidence audit. The local socket `EPERM` is covered by zero-skip VM
  evidence. Totals are 37 closed and 48 open; generated bytecode is removed.
- Architecture-before-allocation is next. New
  `scripts/test-runtime-image-architecture-live.sh` validates the real selected
  kernel manifest, records digest/release plus required-config/OCI-feature
  counts, binds a real x86-64 BusyBox to those values, rejects an AArch64
  control, and runs held-root/held-child fixtures. Syntax/shellcheck/local suite
  pass; script SHA-256 is `6ff46cb5…` and real-manifest VM proof remains open.
- Clean commit `f83ff2178738751e87d12cd5e19c4eeec42e8c4a` archive is
  4,730,880 bytes/SHA-256 `9cfeb51d…` and matches on VM. Fresh ext4 extraction
  has 642 files and exact qualifier SHA `6ff46cb5…`; root run remains open.
- Exact-commit privileged VM run exits 0 with zero skips. Actual selected
  manifest SHA `1d79c564…` is amd64, release `7.0.0-mk2-gce-lab`, and records
  four required configs plus 18 OCI features after ownership/hash/pin/ELF/
  module-vermagic validation. BusyBox SHA `8d4e5a13…` binds to those exact
  values; an executable AArch64 control rejects status 1, and all ELF/shebang/
  escape/held-root/held-child cases pass. `g4-image-architecture-live-pass.log`
  is mode 0600/5,558 bytes/SHA-256
  `7fa430c243ab4d764d9b9866fc1e7e5b0fcea899ac48643886417077c89cc3e2`
  with no credential matches.
- Independent `g4-image-architecture-final-resource-audit-pass.log` exits 0,
  mode 0600/12,601 bytes/SHA-256
  `c5748c810df83094cc504207099b42c3ac16ae553cb21b05c16f91c5c0519d6b`.
  No pool/child or any of 19 resource types exist; candidate identity and four
  zero-restart services are unchanged. Credential scan is empty. The
  architecture/kernel-feature row is checked.
- The post-closure full repository gate passes links, schemas, 17 current
  manifests, 95 OCI cases, G4 fixtures, managers, GCE ledger, capture,
  containerd config, and final audit. The local socket `EPERM` is covered by
  zero-skip VM evidence. Totals are 38 closed and 47 open; bytecode is removed.
- Before the caller-snapshot qualification, GCE reports the retained instance
  `RUNNING` with start timestamp `2026-10-03T06:53:49.759-07:00` and boot ID
  `304feec7…`. Docker, containerd, mkruntimed, and mknetd are all active with
  `NRestarts=0`. The first readiness command stopped before its final marker
  because it incorrectly asserted `/run/mkruntime/{mkruntimed,mknetd}.sock`;
  this is a probe-path failure, not runtime evidence. Privileged enumeration
  then proves the installed root-owned mode-0660 sockets are actually
  `/run/mkruntimed.sock` and `/run/mknetd.sock`, each held by the expected
  zero-restart daemon. No caller-snapshot closure claim is made from this
  availability check.
- Read-only live inventory identifies the qualification subject without
  creating a task: both `default` and `moby` cache
  `docker.io/library/busybox:1.36` at OCI index digest `sha256:73aaf090…` and
  each has exactly one committed snapshot, key `sha256:97e4ece8…`, with no
  parent. Docker reports the same index digest. This records the candidate
  index and snapshot identity; the selected linux/amd64 manifest, source mount
  table, metadata/Merkle digest, and before/after equality remain to be
  captured by the live qualifier.
- A first mount-table query supplied the committed snapshot directly to
  `ctr snapshots mounts`; containerd rejected it as `not active or view` with
  `failed precondition`. It created no mount or snapshot. The live harness must
  obtain the projection through a temporary read-only image view, record that
  view's mount table, unmount it, and compare independently generated source
  manifests around both runtime workloads. This command is retained as probe
  design feedback, not failure evidence against the runtime.
- The temporary-view prototype mounts the exact committed filesystem as ext4
  `ro` and records root inode 6035824 plus BusyBox inode 6035832. The independent
  442-entry normalized source manifest reports digest `65714ed0…`. A cleanup
  audit caught that `ctr images unmount` removes the mount but leaves the view
  snapshot record; the test-owned view was then removed explicitly, after
  which only committed snapshot `97e4ece8…` remains and the probe target is
  unmounted. The final harness must trap both operations and must not infer
  complete cleanup from unmount success alone.
- New `scripts/test-runtime-caller-snapshot-live.sh` now composes exact OCI
  digest/snapshot identity, temporary read-only mount and normalized-manifest
  capture, focused race tests, relative ctr and absolute Docker root-path guest
  writes, retained build-result inspection, byte comparison, and final cleanup.
  Local `bash -n` and `git diff --check` pass; `shellcheck` is unavailable on
  this workstation and is not claimed. The first local Go invocation reached
  no test because the default build cache is read-only; rerunning with an
  isolated `/tmp` cache passes all 11 selected rootfs tests and the shim mount
  sanitation test under `-race -v`. Live `ctr snapshots info` inspection also
  corrected the harness to consume capitalized `Kind`/`Name` and the omitted
  parent field. No workload or row closure is claimed yet.
- The qualifier is committed alone as
  `49bb985182eb4a82164278f0beb29eb706c1dc3a`; the two running evidence ledgers
  and existing evidence trees remain outside that commit. Its exact Git archive
  is 4,751,360 bytes with SHA-256 `32d61a2b…`, and the committed script SHA-256
  is `ab52789b…`. Transfer, fresh extraction, and VM execution remain next.
- The transferred archive matches exact SHA-256 `32d61a2b…`; fresh extraction
  `/var/tmp/mksrc-49bb985` contains 643 regular files. The executable qualifier
  is mode 0775, owned by the ordinary VM user, matches SHA-256 `ab52789b…`, and
  passes VM `bash -n`. `shellcheck` is unavailable on the VM as well. The live
  test itself has not yet run.
- The first live invocation is non-qualifying and was interrupted. Preflight
  called `cleanup`, whose `set +e` changed the parent shell, so unsupported
  containerd 2.2.2 syntax `ctr snapshots ls -q` did not stop the script. Before
  interruption, clean baseline, exact index `73aaf090…`, selected amd64
  manifest `b7f3d86d…`, config `b116e155…`, layer `034d6572…`, diff ID/committed
  snapshot `97e4ece8…`, all focused VM race tests, and read-only ext4 view plus
  manifest `65714ed0…` were observed; none are promoted to row closure from a
  fail-open harness. Interruption left only the exact default task/container
  `mk-snapshot-relative` in `UNKNOWN` state with its active snapshot and no
  child. Targeted removal plus an idle daemon restart restored one committed
  snapshot, zero tasks/containers/rootfs records/children, four active
  zero-restart services. The private retained failure transcript is mode 0600,
  50,894 bytes, SHA-256 `87275953…`. Cleanup now runs in a subshell so shell
  options cannot leak, and snapshot inventory uses the supported table parser.
- The fail-closed correction is committed alone as
  `bcd7ad7025417c7a8da0864e0484a3f6c9bc1146`. Its script SHA-256 is
  `3249db88…`; the 4,751,360-byte exact Git archive hashes to `a0dedf40…`.
  A fresh transfer/extraction is required before rerun.
- The VM copy matches archive and script hashes, extracts to a new 643-file
  `/var/tmp/mksrc-bcd7ad7`, and passes `bash -n`. The clean-host rerun is next.
- The corrected run fails closed at the Docker source assertion after fully
  passing the relative path. The ctr bundle records relative `rootfs`, source
  manifest `65714ed0…`/442 entries both before and after copy, matching the
  independent committed view; guest `/etc/passwd` changes from `466afb85…` to
  `2fd5b4e5…` and its new private-root marker is readable. Docker then records
  the exact absolute allowlisted root and equal before/after source digest
  `e9573a41…` with 452 entries. Requiring that Docker active source to equal
  the pristine 442-entry base was wrong: Docker adds container-specific root
  content before runtime handoff. Trap cleanup leaves only each namespace's
  committed snapshot, with zero tasks, containers, rootfs records, endpoints,
  or children and four active zero-restart services. Retained private
  `g4-caller-snapshot-live-second-docker-base-assumption-fail.log` is mode 0600,
  71,325 bytes, SHA-256 `868bc5b8…`. The absolute-path branch now independently
  rescans the exact live Docker source after guest mutation and requires that
  digest/entry count to equal its retained before/after build scans.
- The direct-source correction is committed as
  `fe9a2deefffad48ae5aa17c91b3ff6330a7e12e9`; driver SHA-256 is `51f98998…`,
  and its 4,751,360-byte exact archive hashes to `e390030a…`. Fresh transfer,
  extraction, and rerun remain pending.
- Archive and driver hashes match in fresh 643-file VM extraction
  `/var/tmp/mksrc-fe9a2de`; `bash -n` passes. The third live run is next.
- Exact `fe9a2de` qualification exits 0 with the scoped pass marker. OCI index
  `73aaf090…` selects linux/amd64 manifest `b7f3d86d…`, config `b116e155…`,
  layer `034d6572…`, and diff ID/committed snapshot `97e4ece8…`. All 11 focused
  rootfs cases plus shim sanitation pass under the race detector, covering
  descriptor-pinned inputs, forced `ro,nodev,nosuid,noexec`, pre-mount rejection,
  build/mount rollback, uncertain partial mount, ordinary unmount failure, and
  recovery. The relative ctr root records equal 442-entry `65714ed0…` scans;
  the absolute Docker root records equal 452-entry `e9573a41…` scans, and an
  independent rescan of its exact still-live overlay after guest mutation
  matches. Both guests change `/etc/passwd` from `466afb85…` to `2fd5b4e5…`
  and create the private marker without changing their caller source. The
  committed snapshot's before/after manifests are byte-identical at
  `65714ed0…`, and exact snapshot inventory stays `97e4ece8…`. Final cleanup
  reports every one of 19 counters zero and four active/running zero-restart
  services. The VM transcript is private mode 0600/136,343 bytes/SHA-256
  `33e1d92e…`; local copy, credential scan, and independent audit remain before
  row closure.
- Local retained pass transcript matches mode 0600/136,343 bytes/SHA-256
  `33e1d92e…`, and its credential-pattern scan is empty. The first independent
  audit launch did not execute: the restart removed the formerly staged
  `/tmp/audit-runtime-final-resources-live.sh`, while the one-line wrapper
  lacked `set -e` and continued from its failed existence test to exit 127.
  This supplies no audit result; the known independent script must be
  recreated, hash-verified, and run with a fail-closed wrapper.
- The audit was then run fail-closed from exact `fe9a2de` source after
  verifying script SHA-256 `bc5e2f7d…`. It exits 0 on the operator-restarted
  boot `c5537cb9…`, binding running mkruntimed PID 1522 to candidate SHA-256
  `0e1c87c3…`. Kerf reports no pool or child, all 19 independent resource
  counters are zero, and mkruntimed, mknetd, containerd, and Docker are
  active/running with `NRestarts=0`. Local private
  `g4-caller-snapshot-final-resource-audit-pass.log` is mode 0600/15,227 bytes/
  SHA-256 `e169a3da…`; credential scanning is empty. Repository gates remain
  before closing the implementation and replacement-evidence rows.
- The post-closure full gate passes documentation/links, 7 schemas/22 cases,
  17 current manifests, 95 OCI cases, bind/bootstrap/rootfs/storage/mount/image
  fixtures, release and lifecycle managers, GCE ledger, evidence capture,
  containerd configuration, and final evidence audit. The local socket
  `EPERM` remains the explicitly reported sandbox-only skip and is covered by
  the zero-skip VM tests. The caller-snapshot implementation row and exact
  replacement-instance snapshot-evidence row are checked; totals are now 40
  closed and 45 open. Four generated Python bytecode files were removed.
- The next open-row audit confirms that writable-root ownership is explicitly
  durable and generation-bound rather than inferred from one private initramfs.
  `rootfs.Store` rejects simultaneous records that claim the same bundle or
  storage port. `storage.Service` keys ownership by sandbox ID/generation,
  mints a separate export generation, rejects stale release and conflicting
  live generations, and retains `PREPARING`/`QUIESCING` ownership across
  ambiguous failures for exact reconciliation. The Linux backend validates an
  exact image inode under a held private parent and transfers one exclusive
  open-file-description lock through inherited fd 3 into `mkvsock-nbd`; the
  offline checker separately holds that exclusive lock for its entire run.
  Existing race tests exercise each of those boundaries, including direct
  contention and lock release. This is source/test inventory only: the
  single-owner row remains open until an exact-source disposable-host run also
  observes durable live generations, real image-lock contention and release,
  conflict/stale-generation rejection, clean teardown, and an independent
  post-run resource audit.
- A read-only probe on restarted boot `c5537cb9…` confirms the deployed
  storage snapshot is version 2 with top-level `exports`, not `records`; it
  currently retains 183 historical exports and no ephemeral `/run/mkstorage`
  directory while idle. The compound probe exits 1 solely because its final
  `find` receives that expected absent directory. This catches an older
  evidence helper's ineffective `records` lookup before it can be reused: the
  new qualifier must select exactly one non-`RELEASED` export from `exports`
  while live and must not require historical released exports to be deleted.
- New `scripts/test-runtime-single-owner-live.sh` combines exact durable
  rootfs/export/process-record identity, deployed NBD fd-3 inode binding, a
  real second-owner lock attempt, post-unlink acquisition on the same held
  descriptor, exact released-generation/offline-check evidence, and cleanup.
  Its focused local race run passes the rootfs duplicate-bundle/port cases and
  storage duplicate path/port/UUID/owner, state-transition, stale/conflicting
  generation, ambiguous reconciliation, server/checker lock, stale artifact,
  and missing-runtime-directory cases. `bash -n` and `git diff --check` pass;
  local `shellcheck` is unavailable. Review caught that storage map keys embed
  NUL between sandbox ID and generation; the unexecuted harness initially
  attempted to move that key through a shell variable. It now carries the two
  validated components separately and reconstructs the exact key inside
  Python. No live qualification claim has yet been made.
- The isolated qualifier commit is
  `287a817f73054c8eff0efce980c83e5a6bd28e62`. Its executable driver SHA-256
  is `80969159…`; the 4,761,600-byte exact Git archive hashes to
  `ce8dec4e…`. The evolving documentation and existing evidence trees are not
  included in that commit. Fresh VM transfer, hash verification, and execution
  remain pending.
- Exact-source transfer succeeds on boot `c5537cb9…`: the remote archive and
  driver match `ce8dec4e…` and `80969159…`, the fresh extraction contains 644
  files, and VM `bash -n` passes. The first run then fails closed before any
  workload. All rootfs/store/service generation tests pass, but the three
  real-ext4 backend fixtures report that their images are sparse because Go's
  default `t.TempDir()` lands on VM `/tmp`, a tmpfs; production storage is a
  separate ext4 `/dev/sdb`, and `/var/tmp` is root ext4. Trap cleanup has no
  task to remove. The retained private failure transcript is mode 0600, 19,236
  bytes, SHA-256 `56c325e5…`, with an empty credential scan. A first follow-up
  resource-audit command also supplies no audit result because it mistakenly
  invoked the ordinary-user-only script through `sudo`; its filesystem probes
  remain valid. The qualifier now binds both `TMPDIR` and `GOCACHE` to its
  ext4-backed `/var/tmp` scratch before rerun.
- The corrected ordinary-user audit after that failed run exits 0: boot and
  candidate daemon identity are unchanged, Kerf has no pool or child, all 19
  resource counters are zero, and all four services remain active/running with
  zero restarts. The two-line fixture correction is isolated as commit
  `a39896e3dd10e1fe3e567335921ebef8c6596a4f`; its driver SHA-256 is
  `3e121076…`, and the 4,761,600-byte archive SHA-256 is `127c7011…`.
  Transfer and fresh rerun remain pending.
- The freshly verified `a39896e` rerun passes every focused race test and
  creates a live task with exactly one rootfs record and one non-released
  export, then fails closed inside the evidence summary. The parser required
  `/proc/<pid>/comm` to equal `mkvsock-nbd`, but the production daemon executes
  the exact server through `/proc/self/fd/4`; the comm label is neither stable
  nor an authenticated invariant. Private retained transcript is mode 0600,
  24,637 bytes, SHA-256 `45c4099a…`, with empty credential scan. Trap cleanup
  releases the task, export, rootfs record, and child, but the first independent
  audit correctly stops because the now-idle daemon retains its configured
  Kerf pool. After proving zero live exports/records, an idle mkruntimed restart
  releases that pool and a second audit passes all 19 zero counters with four
  healthy zero-restart services (audit SHA-256 `dcbbe91f…`). The parser now
  checks the security boundary actually used by recovery: the live executable
  device/inode equals the durable process record, in addition to fd 3 matching
  the exact image inode.
- The executable-identity correction is isolated as commit
  `69e9ea947a22ba3cfc0c72950d4d4631671bf71a`; its driver SHA-256 is
  `5f1f6fce…`, and its 4,761,600-byte exact archive hashes to `e77ae19e…`.
  Fresh transfer and rerun remain pending.
- Exact `69e9ea9` starts from a zero-resource independent baseline, passes all
  focused race tests, and records one internally consistent live owner:
  rootfs version 4 `PREPARED`, storage version 2 `ACTIVE`, different 32-hex
  sandbox/export generations, fd 3 matching image device/inode, and the pinned
  executable matching its durable binary device/inode. The next root helper
  opens that same image and signals that its nonblocking exclusive lock was
  contended, but evidence-wide `umask 077` makes its result mode 0600/root; an
  ordinary-user `grep` fails with permission denied before the observation is
  emitted. This is a harness-read failure, not a lock claim. Retained private
  transcript is mode 0600/36,289 bytes/SHA-256 `8996f431…`, credential scan
  empty. Zero live records/exports were verified, an idle daemon restart
  released the retained pool, and the independent 19-counter audit passes
  again (SHA-256 `588ab8d1…`). The one result check now uses `sudo grep`.
- That correction is isolated as commit
  `65903b8f600e545222daa6138ff3ad68d0d20390`; driver SHA-256 is
  `505ae529…`, and the 4,761,600-byte archive hashes to `22b0bfff…`.
  Transfer and fresh rerun remain.
- Exact `65903b8` qualification exits 0 with
  `G4_SINGLE_OWNER_LIVE_PASS`. All focused race cases pass on ext4-backed VM
  scratch. The live owner ties one version-4 `PREPARED` rootfs record to one
  version-2 `ACTIVE` export and one process record: sandbox generation
  `bc2bf70e…`, independent export generation `230b0bac…`, image/fd-3 identity
  `2064:524349`, and pinned executable identity `2049:6079391`. A second
  nonblocking exclusive lock on that same inode is contended. Exact teardown
  preserves the same generations in `RELEASED`, records nonzero read/write/
  flush counters and offline check `e2fsck-clean-sha256:d7c3d5d2…`, removes
  the pathname and all live records, then lets the independently held
  descriptor acquire the lock at the same identity with link count zero.
  Private retained pass log is mode 0600/46,525 bytes/SHA-256 `5790e421…`;
  credential scan is empty. After the scoped idle-daemon restart, independent
  private audit mode 0600/15,232 bytes/SHA-256 `b6b0b230…` binds boot
  `c5537cb9…`, candidate daemon SHA `0e1c87c3…`, no Kerf pool/child, all 19
  counters zero, and four active/running zero-restart services. This closes the
  single-owner/generation/duplicate-attach/stale-lock row; repository gates
  remain before recording updated totals.
- The post-closure full repository gate passes documentation/links, 7 schemas/
  22 cases, 17 current evidence manifests, 95 OCI cases, bind/bootstrap/
  rootfs/storage/mount/image fixtures, lifecycle and deployment managers, GCE
  ledger, evidence capture, containerd config, and final-evidence audit. The
  explicitly reported local socket `EPERM` skip remains covered by the VM's
  zero-skip runs. `git diff --check` is clean, four generated Python bytecode
  files are removed, and checklist totals are now 41 closed and 44 open.
- The writable-state-model audit finds that Plan 04 already places writable
  host-path volumes and configured persistence in a later storage-format and
  ownership contract. The current implementation matches that intended narrow
  v1 boundary: every task gets an ephemeral private ext4 root; generic writable
  binds and shared/slave propagation are rejected; admitted read-only inputs
  are copied with numeric metadata and no host path/propagation; Docker's exact
  three metadata files alone become identity-bound private writable seed
  copies; and OCI UID/GID mapping fields are rejected as unknown Linux fields.
  The row remains open pending one explicit supported-model contract, focused
  UID/GID-mapping rejection assertions, and exact-source live proof that an
  unconfigured write disappears after delete/name reuse while rejected generic
  writable binds allocate no runtime state.
- The v1 supported model is now explicit in Plan 04, the ownership/trust table,
  and the runtime README: private roots are ephemeral per sandbox generation;
  persistence, writable volumes, generic writable host binds, UID/GID mapping,
  and shared/slave propagation are unsupported; read-only copies preserve
  numeric ownership with propagation `none`; only exact Docker metadata seeds
  are privately writable. Focused tests pass with 97 OCI semantic cases,
  including explicit UID- and GID-mapping rejection, while materialization now
  asserts numeric UID/GID preservation. Live name-reuse/non-persistence and
  zero-allocation writable-bind rejection remain before row closure.
- The contract, focused tests, and live driver are isolated as commit
  `3ae721df0338d2336c3dac3c775fc4e0e61a59e5`. The 4,771,840-byte exact
  archive hashes to `20c5b8f1…`; live driver SHA-256 is `c6cf3c29…`, OCI test
  SHA-256 is `181bd6ae…`, and bind test SHA-256 is `df6effc2…`. Fresh transfer,
  verification, and VM execution remain.
- Exact `3ae721d` qualification exits 0 with
  `G4_WRITABLE_STATE_MODEL_LIVE_PASS`. VM-focused tests pass all 97 OCI cases,
  including UID/GID mapping rejection, plus numeric-owner-preserving bind
  materialization. Two sequential `mk-ephemeral-state` lifecycles use the same
  image/name: generation one writes `generation-one`; generation two first
  requires that path absent, then writes `generation-two`. Each lifecycle
  converges to 13 zero live-state counters. Generic writable ctr bind exits 1,
  generic writable Docker bind exits 125, and shared propagation exits 1; each
  diagnostic matches the fail-closed bind boundary, the exact host control
  remains `host-immutable`, and cleanup again reaches all 13 zeros. The private
  pass transcript is mode 0600/314,832 bytes/SHA-256 `cd5b328f…`; credential
  scan is empty. After idle-daemon pool release, independent private audit is
  mode 0600/15,072 bytes/SHA-256 `01e0ec0d…`, binds boot `c5537cb9…` and
  candidate daemon SHA `0e1c87c3…`, finds no pool/child and all 19 counters
  zero, and records four active/running zero-restart services. Together with
  the earlier live read-only bind/no-write-through and caller-snapshot evidence,
  this closes both the supported writable-state-model row and its corresponding
  persistence/isolation evidence row under the explicit v1 scope.
- The post-closure full gate passes documentation/links, 7 schemas/22 cases,
  17 current evidence manifests, the expanded 97-case OCI boundary, bind/
  bootstrap/rootfs/storage/mount/image fixtures, lifecycle/deployment managers,
  GCE ledger, evidence capture, containerd configuration, and final-evidence
  audit. The local socket `EPERM` skip remains explicit and is covered by VM
  execution. `git diff --check` is clean, generated bytecode is removed, and
  checklist totals are now 43 closed and 42 open.
- The next capacity/ENOSPC audit distinguishes existing proof from the open
  claim. `build-runtime-storage.py` checks host free bytes before staging and
  again after the private copy, fully allocates the requested image with
  `posix_fallocate`, records equal size/quota plus the requested ext4 inode
  limit, and verifies those identities from the superblock. Its focused tests
  already prove pre-allocation high-water refusal and natural in-image inode
  and block exhaustion with no image, metadata, or staging residue.
  `build-runtime-rootfs.py` separately enforces payload-byte and entry-count
  limits plus an output-filesystem high-water check. This is not yet the full
  row: no test deterministically injects host `ENOSPC` into staging copy,
  image allocation/fsync, ext4 tools, image publication, metadata write/fsync/
  publication, or initramfs archive/manifest construction and publication;
  host free-inode reserve/accounting is also absent. The broad fault-matrix row
  and the narrower capacity row therefore remain open pending boundary-complete
  tests and exact-source disposable-host evidence.
- The first capacity correction adds an explicit unprivileged free-inode
  reserve alongside free bytes in both builders and passes it from the
  production wrapper (`MK_STORAGE_MIN_FREE_INODES` and
  `MK_INITRAMFS_MIN_FREE_INODES`, each defaulting to 1024). Storage reserves
  the admitted clone plus staging/output metadata inodes before copying, then
  rechecks the two remaining output inodes after staging; initramfs reserves
  its archive/manifest pair (or one manifest-only inode). Focused high-water
  refusal cases leave no outputs. During that work, direct failure review
  found `runtime_safe_publish.atomic_write` captured temp identity only after
  write/flush/fsync, so an earlier ENOSPC could strand its private temp file.
  It now captures identity immediately after `mkstemp`, revalidates before
  publication, and rolls back that exact inode after any failure. Direct
  injected write, fsync, and no-replace-publication ENOSPC cases all leave no
  public or private attempt artifacts; the 10 storage and 19 initramfs cases
  still pass, with only the explicitly known local socket-permission skip.
  Builder-specific failure boundaries and live evidence remain open.
- The next deterministic failure matrix passes 11 storage cases and 20
  initramfs cases. Storage injects `ENOSPC` at staging-directory allocation,
  private copy, `posix_fallocate`, image fsync, `mke2fs`, `e2fsck`, image
  publication, and metadata creation; every subcase leaves no public image,
  metadata, staging directory, or private output temp. Existing debugfs
  normalization failure covers the remaining external ext4 mutation step.
  Initramfs now additionally proves that metadata ENOSPC after archive
  publication removes the exact archive and leaves no output temp; direct
  shared-publisher tests cover archive/manifest write, fsync, and link
  publication failures. This closes the deterministic builder-allocation
  subproblem only. A real constrained filesystem must still demonstrate
  block/inode high-water refusal and actual ENOSPC behavior, followed by the
  live runtime's zero-allocation/cleanup audit, before either checklist row can
  close.
- The unexecuted disposable-host qualifier is now present as
  `test-runtime-capacity-enospc-live.sh` (SHA-256 `368ad6d5…`) with its
  boundary helper `test-runtime-real-enospc.py` (`e256d6d6…`). It uses
  separately mounted, kernel-enforced tmpfs limits: ordinary builder calls
  must refuse byte and inode high water with empty mounts, while a test-only
  override of capacity observation forces the unchanged builders to receive
  real kernel ENOSPC at storage block allocation, storage staging-copy inode
  allocation, initramfs archive writing, and initramfs second-output inode
  allocation. The override cannot manufacture a pass because each case also
  requires absent public/private artifacts on the real constrained mount.
  Full 19-counter audits bracket the sequence. Local Bash syntax, Python
  compilation, the 1-case publisher, 11-case storage, 20-case initramfs, and
  diff checks pass; four generated bytecode files were removed. No VM or live
  result is claimed yet.
- The full repository gate passes with the expanded 20-case initramfs,
  11-case storage, and 1-case shared-publisher suites, plus all documentation,
  schema, 97-case OCI, deployment, and evidence checks. Four generated bytecode
  files were removed and `git diff --check` is clean. The first scoped commit
  attempt made no change because this workspace exposes `.git` read-only and
  Git could not create `index.lock`; the exact source/test/plan file list will
  be retried with repository-write approval, while both running ledgers and
  pre-existing evidence trees remain excluded.
- Repository-write approval succeeded for the unchanged scoped list. Commit
  `97bcae5` (`97bcae5…`) contains 11 source/test/Plan 04 files and no running
  ledger or evidence tree. Its exact 4,792,320-byte Git archive at
  `/tmp/mksrc-97bcae5.tar` hashes to `2b3c836f…`; live driver, real-ENOSPC
  helper, and publisher-test hashes remain `368ad6d5…`, `e256d6d6…`, and
  `21044321…`. VM upload and guest verification remain pending.
- Upload and fresh extraction now succeed on boot `c5537cb9…`. The guest
  reports the same 4,792,320-byte archive and `2b3c836f…` digest, the driver,
  real-ENOSPC helper, and publisher test match `368ad6d5…`, `e256d6d6…`, and
  `21044321…`, and the tree contains 648 regular files. Guest Bash syntax and
  helper compilation pass; mkruntimed, mknetd, containerd, and Docker are all
  active. This establishes transfer provenance only. The live qualifier has
  not yet run.
- Exact `97bcae5` live qualification exits 0 with
  `G4_CAPACITY_ENOSPC_LIVE_PASS`. The VM-focused 1/11/20 publisher, storage,
  and initramfs suites pass. Real constrained filesystems prove storage byte
  and inode high-water refusal, real block ENOSPC during image allocation,
  real inode ENOSPC during staging copy, initramfs byte/inode high-water
  refusal, real block ENOSPC during archive output, and real inode ENOSPC at
  its second output after 30 filler inodes; each case requires empty public and
  private attempt state. Full audits before and after both report
  `G6_FINAL_RESOURCE_RETURN_PASS`: no Kerf pool/child, all 19 counters zero,
  candidate daemon `0e1c87c3…`, unchanged boot `c5537cb9…`, and four
  active/running zero-restart services. The private remote transcript is mode
  0600, 82,525 bytes, SHA-256 `f216d797…`, wrapper exit 0, with an empty
  credential-pattern scan. Local retention and post-copy verification remain
  before row closure.
- Local retention matches mode 0600, 82,525 bytes and exact SHA-256
  `f216d797…`; the repeated credential scan is empty and all four high-water,
  four real-ENOSPC, two audit-pass, suite-pass, qualifier-pass, and wrapper-
  exit markers are present. The focused capacity/accounting row is now closed.
  The later composite fault-matrix row remains open for its non-capacity
  requirements. Repository gates and updated totals follow separately.
- The post-closure full gate passes documentation/links, 7 schemas/22 cases,
  17 current evidence manifests, 97 OCI cases, the 20-case initramfs,
  11-case storage, and direct publisher suites, plus all bind/bootstrap/mount/
  image, release/deployment, ledger, capture, containerd, and final-audit
  checks. The known local socket `EPERM` remains explicitly reported and is
  covered by the zero-skip VM execution. `git diff --check` is clean, four
  generated bytecode files are removed, and totals are now 44 closed / 41
  open.

- The current post-initramfs documentation gate passes all documentation,
  schema, 97-case OCI, 20-case initramfs, 12-case storage, publisher, bind,
  bootstrap, mount, image, release/deployment, ledger, capture, containerd,
  and final-audit checks. Generated Python bytecode was removed. The checklist
  now has 47 closed and 38 open rows.
- Source tracing for the still-open storage teardown row proves the intended
  implementation order but also identifies the exact remaining live-evidence
  gap. The shim authenticates `Quiesce` before terminal `Shutdown`; guest
  shutdown executes filesystem sync, read-only root remount, a second sync,
  exact `/dev/nbd0` disconnect, then poweroff. A disconnect failure prevents
  poweroff. Storage release requires the generation-specific terminal
  `MKNBD_SERVER_CLOSED synced=1` record before it accepts counters and runs the
  locked offline check. The existing live release evidence proves durable
  `RELEASED`, counters, offline check, and zero final inventory, but does not
  retain the guest disconnect console marker or directly join it to the
  server-close record. Therefore the teardown row remains open pending a
  console-captured live run (or an equivalent durable ordered transcript).
- The restarted disposable VM reports `RUNNING`, the same boot ID
  `c5537cb9…`, and selector `0.1.0-dev-11a65f08…`. The first read-only
  service/Kerf probe is invalid: local double-quote expansion consumed its
  remote loop variable and systemd rejected the empty unit name. It is
  classified as a qualifier harness failure and supplies no service-health or
  teardown evidence; the corrected probe follows separately.
- The corrected probe confirms mkruntimed, mknetd, containerd, and Docker are
  all active with zero restarts, and Kerf reports no pool or instances. Its
  configured executable is `/opt/mkruntime/kerf-venv/bin/kerf`; bare `sudo
  kerf` is unavailable through sudo's secure path. `/dev/mktty` is the
  root-owned mode-0600 character console, and the configured CLI supports
  attaching to a running instance by name.
- Guest shutdown now emits a bounded
  `MK_STORAGE_ROOT_QUIESCE_PASS stages=sync,remount-ro,sync` marker only after
  the second sync succeeds. The existing disconnect pass/fail marker remains
  after the exact NBD ioctl, and disconnect failure still prevents poweroff.
  Ten race-enabled mk-agent package repetitions pass. The first combined
  format/test command used repository-relative paths while already inside
  `runtime`, so only gofmt failed with three `lstat` errors; the tests did run
  and pass. This is a local harness-path failure, not formatting evidence, and
  a corrected gofmt/diff check remains next.
- Corrected gofmt, another ten race-enabled mk-agent repetitions, the complete
  documentation/runtime gate, and `git diff --check` all pass. The gate still
  reports only the explicitly known sandbox-local socket `EPERM` skip; its VM
  counterpart was already zero-skip. Generated Python bytecode is removed.
  Only the three mk-agent source/test files will be frozen next; ledgers and
  evidence remain excluded.
- Commit `e396511` (`runtime: expose mediated root quiescence evidence`)
  freezes exactly the three mk-agent files (11 insertions, 6 deletions). Its
  exact tracked-source tar is 4,792,320 bytes with SHA-256 `8dccf1fa…`, and
  authoritative revision is `e396511c18ad3faa78c3080de477ce11e265f1d0`.
  Both ledgers and pre-existing evidence trees remain outside the commit.
  Guest transfer/build/activation and live observation remain unclaimed.
- The guest independently verifies the uploaded archive as exact `8dccf1fa…`
  and completes the full static build with revision `e396511c…`, including
  exclusive release-manifest publication. The compound command then exits 2
  because a follow-up invocation incorrectly assumed the manifest tool had a
  `--check` mode; it instead requires version, revision, and an output path.
  Thus compilation is observed, but manifest verification, hashes,
  installation, and activation are deliberately unclaimed until a corrected
  read/installer verification runs. The unique root-owned build is retained.
- Corrected verification independently regenerates the manifest into a new
  exclusive file and byte-compares it equal to the build output. Exact hashes
  are release manifest `ee31cd3c…`, mkruntimed `29dc06dd…`, shim
  `1b245d66…`, and mk-agent `b57bae66…`; the candidate daemon reports exact
  full revision `e396511c…`. The temporary verifier output was removed only
  after equality. Installation and execution remain separate claims.
- Binary management installs and selects immutable release
  `0.1.0-dev-e396511c18ad3faa78c3080de477ce11e265f1d0`. On the empty host,
  mkruntimed and mknetd restart cleanly; all four services are active with
  zero restarts. The running daemon hashes exactly to candidate `29dc06dd…`,
  reports the full revision, and Kerf still reports no pool or instance.
- A single-purpose live teardown qualifier is now present at SHA-256
  `4b9b84e5…`. It writes and syncs mediated-root data, attaches the named Kerf
  console while the child is live, retains only the two fixed safe shutdown
  markers, requires their line order, authenticates the exact generation's
  terminal `synced=1` counter line, durable offline check/counters, image and
  record removal, then deliberately restarts the idle daemon to return the
  retained pool and checks four-service health. Raw console content is kept
  only in a private temporary directory and is never printed. Bash syntax,
  ShellCheck (when installed), and diff checks pass. Live execution remains
  pending.
- First immutable capture is mode 0600, 105,658 bytes, SHA-256
  `8524fff5…`, and exits 1. It proves exact remote qualifier hash, clean
  pre-state, candidate selector/daemon, an exact ACTIVE storage generation,
  and normal task/container deletion, but the post-start Kerf console attach
  observes neither fixed shutdown marker before its bounded poll expires. The
  private raw console is deleted by the trap and is not retained or printed.
  This is classified as a console-attachment evidence failure, not a storage
  teardown failure and not a pass. The teardown row remains open while exact
  initramfs agent provenance and capture timing are inspected.
- Bootstrap inspection identifies the cause: `/etc/mkruntime/kernels/gce-mk2.json`
  still pins agent artifact release `1f81cb2…` with SHA-256 `c79210c6…`, not
  candidate agent `b57bae66…`. Activating the host binary release therefore
  did not alter the guest agent embedded into the first run's initramfs. The
  diagnostic then stopped before durable-state output because it incorrectly
  passed the entire `{path,sha256}` agent object to `sha256sum` rather than
  `.agent.path`; this is a read-only probe-shape failure. The agent mismatch is
  nevertheless directly observed and explains the absent new marker. Managed
  kernel-artifact update and corrected post-failure audit remain next.
- Corrected audit proves declared and observed old agent hashes both equal
  `c79210c6…`, while the candidate is `b57bae66…`. Independently of the missed
  console, the first run did execute a clean storage teardown: its exact
  generation is durable `RELEASED`, offline check is `e2fsck-clean-sha256:
  bf24cda1…`, counters are reads 285/10,452,992 bytes, writes 49/696,320 bytes,
  flushes 9, and the exact terminal log is
  `MKNBD_SERVER_CLOSED synced=1` with identical counters. All services remain
  active/zero-restart and no child remains; the 16-GiB pool is intentionally
  retained idle by current daemon policy. These facts validate teardown but
  cannot validate the new guest marker, so the row stays open.
- The first manifest-selection attempt is rejected before activation because
  its candidate JSON was placed under world-writable `/var/tmp`; strict
  bootstrap validation reports an unsafe artifact parent. The candidate agent
  and rollback manifest were already copied into a new root-owned immutable
  release directory, but `/etc/mkruntime/kernels/gce-mk2.json` remains the old
  `1d79c564…` file. This is expected fail-closed behavior. Only the rejected
  temporary JSON will be removed; validation will be repeated with the
  candidate inside the trusted artifact directory.
- Retrying from the trusted root-owned artifact directory passes strict
  validation before and after atomic selection. Active manifest SHA-256 is
  `d4230978…`; the preserved rollback manifest remains exact `1d79c564…`.
  Active agent path is the immutable `e396511c…` artifact, a root-owned
  single-link mode-0755 regular file, and its declared/observed SHA-256 both
  equal candidate `b57bae66…`. No service restart is required because the
  rootfs builder resolves this manifest independently for every new task.
  Replay remains unclaimed.
- Unchanged qualifier replay passes against that exact agent. Guest console
  line order is 7 then 9 for quiesce then disconnect; exact terminal storage
  close is `synced=1` with nonzero reads/writes/flushes; durable release and
  offline check match; image and runtime record are removed; pool return and
  four-service checks pass. Local retention is mode 0600/30,921 bytes/SHA
  `8e7d93e0…`, exit 0, with empty credential scan. The independent full audit
  is mode 0600/14,362 bytes/SHA `49f4e2e2…`, exit 0, empty credential scan,
  all 19 counters zero, no pool/child, exact candidate daemon, and four healthy
  zero-restart services. This closes the storage teardown/recovery row.
- The post-closure full gate passes documentation/links, 7 schemas/22 cases,
  17 current evidence manifests, 97 OCI cases, 20 initramfs cases, 12 storage
  cases, publisher, bind/bootstrap/mount/image, release/deployment, ledger,
  capture, containerd, and final-audit checks. `git diff --check` is clean,
  generated bytecode is removed, and totals are now 48 closed / 37 open.
- After the user's instance restart, authoritative boot advances from
  `c5537cb9…` to `95e59482…`. Persistent selection remains coherent:
  immutable runtime release `e396511c…`, active kernel manifest `d4230978…`,
  and declared/observed guest-agent digest `b57bae66…`. mkruntimed, mknetd,
  containerd, and Docker are active with zero restarts at new PIDs; Kerf has
  no pool or instance. This is the new baseline for subsequent evidence and
  does not alter the already retained pre-restart teardown proof.

- The accepted-client-loss implementation completes the full local
  documentation/runtime packaging gate: documentation and links, 7 schemas/22
  cases, 17 current evidence manifests, 97 OCI cases, 20 initramfs cases, 12
  storage cases, safe publisher, bind/bootstrap/mount/image checks, release/
  binary/deployment/ledger/capture/containerd checks, and the final audit.
  `git diff --check` is clean. The gate-created bytecode was limited to the
  four expected validator/builder cache files and all four have been removed;
  no source artifact was deleted. This substantiates the local change but does
  not close the live server-loss row.

- Commit `65b2528` (`runtime: fail closed after storage client loss`) now
  isolates the seven reviewed implementation, test, and contract files. The
  accumulated learning records and evidence trees were deliberately excluded.
  VM qualification must bind its build and running process to this exact
  revision before any live claim is accepted.

- The exact `65b25284ad1a0549140d51eecedffbaab731db59` source archive is
  `/tmp/mksrc-65b2528.tar`, 4,823,040 bytes, SHA-256
  `f997ab20502aba0e0f61af01f2b8e1e5d4bdef4cddcabfd12c51bf3f0b312aeb`.
  This immutable identity is the upload/build input for VM qualification.

- The first post-restart baseline probe confirms boot `95e59482…`, 40-minute
  uptime, all four services active/running with zero restarts, the prior
  `a32e4dd…` immutable mkruntimed selection, and running PID 11432. The probe
  then stops at a mistaken `/usr/local/sbin/mkshim` hash path; this is a probe
  path error, not a VM/product failure, and leaves the Kerf checks unexecuted.
  The corrected baseline remains required before upload.

- A second baseline probe again stops before mutation because unprivileged
  `command -v mkshim` has no result. This is another harness lookup assumption,
  not runtime evidence; even the proposed `/usr/local/bin` fallback was never
  reached. The installed path will be discovered read-only rather than guessed.

- Read-only discovery finds no standalone file/link named `mkshim` under
  `/usr/local`; this deployment supplies shim behavior through its installed
  runtime support layout rather than that guessed command name. Boot identity
  remains unchanged. The same probe then exposes an obsolete Kerf CLI guess:
  this version has no `pool` command, so no absence claim is taken from it.
  CLI help must select the deployed command spelling.

- The corrected baseline uses deployed `kerf show`: boot is still
  `95e59482…`, there is no memory pool and no multikernel instance, and no
  loaded `/proc/kimage` entry. `/usr/local/sbin/mkruntimed` resolves to the
  prior immutable `a32e4dd…` release and hashes `5dde0c70…`. Its support
  executable is correctly named `containerd-shim-multikernel-v2`, not
  `mkshim`; every release file shown is root-owned and single-linked. The VM
  is clean for upload.

- Upload completes, but the first remote verification command exits before
  printing because its boot-ID command substitution was evaluated by the
  local shell inside the SSH argument. The remote mutation sequence was after
  that failed assertion and therefore did not run. This is a harness quoting
  failure only; archive hash and extraction remain unclaimed until a command
  without substitution verifies them.

- Corrected verification binds the remote archive to unchanged boot
  `95e59482…`, exact 4,823,040-byte SHA-256 `f997ab20…`, then extracts it only
  into new `/var/tmp/mksrc-65b2528-build`. The tree is mode 0755, root-owned,
  and stripped of group/other write bits. Extracted `service.go` hashes
  `a885bbd5…` and `backend_linux.go` hashes `09b75e79…`. This is the sole source
  tree authorized for the candidate build.

- Exact-source VM build passes on boot `95e59482…`: `runtime-manifest` embeds
  full revision `65b25284ad1a0549140d51eecedffbaab731db59`, and the resulting
  daemon reports that revision. Candidate hashes are manifest `cd2502b3…`,
  mkruntimed `61fe95c8…`, shim `c5263b12…`, mknetd `dfb95b17…`, and agent
  `caa36a28…`. Raw evidence
  `g4-storage-client-loss-build-65b2528.log` is mode 0600, 5,200 bytes,
  SHA-256 `2d6ce0f0…`, exit 0, with an empty credential-pattern scan.

- Candidate installation and activation succeed: manager selects immutable
  release `0.1.0-dev-65b25284…`, both daemons restart, the socket appears
  after four 250-ms waits, and all four services are active with zero restarts.
  The verifier then repeats the known public-path mistake by invoking absent
  `/usr/local/bin/mkruntimed`; the service actually uses
  `/usr/local/sbin/mkruntimed`. Thus activation is real, but candidate process
  hash and Kerf cleanliness remain unproven by this failed log. Retained log is
  mode 0600/3,193 bytes/SHA `3de1db6b…`, exit 127, empty credential scan.

- Corrected activation audit passes. Current release, public daemon link,
  public shim link, daemon/shim version output, and live PID 20720 executable
  all bind to full revision `65b25284…`; live and linked daemon hashes both
  equal build hash `61fe95c8…`, and shim equals `c5263b12…`. The runtime socket
  is present, all four services remain active/zero-restart, and `kerf show`
  proves no pool, instance, or loaded kernel. Retained audit is mode 0600,
  3,590 bytes, SHA-256 `5d6013f0…`, exit 0, empty credential scan.

- The first focused VM test invocation binds to the correct candidate but
  fails during Go package setup because `/tmp/mk-go-cache` was created by the
  root build and is unreadable to the ordinary test user. No test executes and
  no product claim is taken. The retained failure is mode 0600/2,007 bytes,
  SHA-256 `916f29ff…`, exit 1, empty credential scan. Retry will use a new
  user-owned cache rather than changing the build cache.

- The user-cache retry executes but two generic ext4-inspection cases fail
  because their temporary 64-MiB `fallocate` files on this VM are reported
  sparse/uninspectable; the five non-image accepted-loss/reconcile cases are
  mixed into the same failing run and cannot be claimed independently. Despite
  its premature `-pass` filename, this retained log is a failure: mode 0600,
  2,331 bytes, SHA-256 `5840b414…`, exit 1, empty credential scan. The policy
  cases will be rerun alone; disposable clone testing will choose and record a
  storage filesystem that can satisfy the allocation invariant.

- Isolated exact-revision policy qualification passes on the VM under the race
  detector in 1.026s: accepted-client crash retention, fail-closed reconcile,
  safe pre-accept restart, canonical-close recovery, and interrupted-release
  recovery all execute. The mode-0600 1,559-byte log hashes `5484a6a8…`, exits
  0, and has an empty credential scan. Live process death and clone behavior
  remain separate requirements.

- The user's subsequent instance restart advances authoritative boot from
  `95e59482…` to `3f80aee1-df81-4fdb-9040-f35afcc73361`. On the new boot, all
  four services are active/running with zero restarts, immutable selector and
  daemon version still bind full `65b25284…`, and `kerf show` reports no pool,
  instance, or loaded kernel. This is direct clean-host-reset evidence for the
  V1 non-persistent writable-root model; it does not invent workload-data
  durability or replace the pending accepted-client-loss reset test.

- Allocation probing explains the earlier ext4-test mismatch: `/tmp` is a
  quota-enabled tmpfs, while `/srv/multikernel-storage` is the dedicated ext4
  `/dev/sdb`. A bounded 67,108,864-byte `fallocate` on that storage reports
  exactly 131,072 512-byte blocks (fully allocated), root ownership, one link;
  only the uniquely named probe file/directory were then removed. Disposable
  clone qualification will use this dedicated filesystem.

- Re-running the exact production backend ext4 tests with `TMPDIR` on the
  dedicated storage disk passes under `-race` in 2.407s. This directly covers
  clean fully allocated ext4 identity/quota acceptance, wrong UUID/inode limit
  rejection, dirty clean-bit rejection, pristine digest mutation rejection,
  and permitted current-image content mutation with stable identity. The
  unique test directory is removed afterward. Evidence is mode 0600/1,775
  bytes/SHA `4b4eb6ba…`, exit 0, empty credential scan. Explicit corrupt and
  recovered disposable clones remain pending.

- The first local clone-matrix formatting command repeats the already-known
  cwd/path mistake (`runtime/…` while cwd is `runtime`) and stops at `lstat`
  before formatting or tests. It makes no change beyond the preceding patch
  and supplies no product evidence; the corrected relative path is required.

- The corrected formatter succeeds, but its chained local test cannot write
  the sandboxed default Go cache under the home directory and stops during
  setup. This is a local harness/cache permission failure, not a test result;
  retry must set `GOCACHE` under `/tmp`.

- With a writable cache, the new clone test reaches the dirty-copy offline
  check but strict executable provenance rejects the environment's system
  `e2fsck` parent (owner 65534 versus caller 1000). Clean-clone and dirty
  inspection steps ran first, but the overall test fails and proves no matrix.
  The test must copy the resolved checker bytes into its caller-owned private
  directory, matching the backend's existing provenance test pattern.

- The next formatter invocation again uses the repository-root path from the
  `runtime` cwd and stops at `lstat` before tests. This repeated harness error
  changes no source and supports no claim; subsequent commands are issued from
  the repository root to eliminate the ambiguity.

- The corrected test now reaches the real copied `e2fsck`; contrary to the
  draft expectation, `OfflineCheck` correctly rejects the dirty clone as
  non-clean rather than returning evidence. This is the desired fail-closed
  contract. The assertion will require that rejection, then prove explicit
  disposable-copy repair and reinspection before claiming recovery.

- Explicit repair then reveals the test's raw clean-bit write invalidates the
  ext4 superblock checksum, so `e2fsck -fy` classifies it as corruption (exit
  8) rather than a merely dirty filesystem. This failure is useful separation:
  dirty and corrupt cases must not be conflated. The dirty clone will instead
  be marked through `debugfs`, which updates ext4 metadata checksums; raw
  superblock damage remains reserved for the corrupt-clone case.

- With checksum-correct `debugfs` dirtying, production behavior becomes
  precise: pristine `Inspect` rejects the unclean bit, while read-only forced
  `e2fsck` finds the disposable clone structurally consistent and emits valid
  offline-check evidence. The test fails only because its interim assertion
  expected rejection. It will assert this split explicitly, then require
  writable repair before pristine reinspection.

- Corrected clone matrix passes with the accepted-client policy cases under
  `-race` in 1.855s. The full storage package then passes under `-race` in
  4.663s. The chained `go vet` uses the default unwritable sandbox cache and
  fails before analysis, so vet/diff inspection remain unclaimed until retried
  with the explicit `/tmp` cache.

- Explicit-cache `go vet ./internal/storage` and `git diff --check` pass after
  review of the 146-line test-only diff. The complete runtime tree then passes
  `go test -race -count=1 ./...` (storage 4.633s) and `go vet ./...`. The clone
  matrix now distinguishes inode identity from byte identity, requires
  fail-closed pristine inspection of dirty/corrupt copies, authenticates
  read-only offline-check evidence for a consistent dirty clone, repairs only
  that disposable copy, and re-proves the source digest and identity unchanged.

- Commit `f9971d8` (`test: qualify disposable storage clone recovery`) contains
  only the reviewed 146-line test change. Its parent is the live production
  candidate `65b2528`; no daemon behavior changed. VM execution must still use
  an exact archive of this test commit before the clone claims can close.

- Exact test commit `f9971d81da8ae6f8d1ec75159bd6ccf4aa374fea` archives to
  `/tmp/mksrc-f9971d8.tar`, 4,823,040 bytes, SHA-256 `7fd114e4…`.
  Learning files and evidence remain excluded from the archive.

- VM verification on new boot `3f80aee1…` matches remote archive SHA-256
  `7fd114e4…` and extracts only into root-owned mode-0755
  `/var/tmp/mksrc-f9971d8-build` with group/other writes removed. The extracted
  test file hashes `995f2c48…`. This exact tree is ready for dedicated-disk
  execution; no runtime service was changed.

- Exact VM clone matrix passes on dedicated `/dev/sdb` ext4 under `-race`:
  the named case completes in 3.65s/package 4.669s and its unique temporary
  image tree is removed. It binds active production parent `65b25284…` to
  exact test commit/archive `f9971d8…`/`7fd114e4…`. Retained evidence
  `g4-storage-disposable-clone-matrix-f9971d8-pass.log` is mode 0600, 2,277
  bytes, SHA-256 `96b861aa…`, exit 0, empty credential scan. This completes the
  clean/dirty/corrupt disposable-copy portion; live accepted-client server
  death remains open.

- A bounded destructive live qualifier is now implemented and passes `bash
  -n`, available `shellcheck`, and `git diff --check`. It starts from a clean
  host, records exact boot/selector/daemon/script provenance, runs concurrent
  guest read/write/sync loops, authenticates accepted-client state, SIGKILLs
  only the recorded NBD server, and requires restart reconciliation to retain
  the exact record/ACTIVE lease/image with zero replacement servers and the
  explicit unsafe-recovery error. It deliberately leaves the disposable VM in
  that diagnostic state for reset evidence; VM execution is still pending.

- Commit `232a49a` (`test: qualify accepted storage client loss`) contains only
  the 228-line live qualifier. Full commit is `232a49a91036ceaab1ec66876d6c0f9613cc8a88`;
  script SHA-256 is `c4d7a5e3…`. Its exact 4,833,280-byte archive
  `/tmp/mksrc-232a49a.tar` hashes `a2da685a…`. Learning/evidence files remain
  excluded and active production code is still parent commit `65b2528`.

- Remote pre-fault verification on boot `3f80aee1…` matches archive
  `a2da685a…` and script `c4d7a5e3…`, extracted only into root-owned mode-0755
  `/var/tmp/mksrc-232a49a-build` with group/other writes removed. Active daemon
  reports exact production `65b25284…`; Kerf has no pool, instance, or loaded
  image. The destructive qualifier therefore starts from a clean bound state.

- First destructive-qualifier attempt never arms the fault: task reaches
  RUNNING, but the initial synced 64-MiB seed never creates readiness within
  240 quarter-second probes and the task becomes STOPPED. Final exec returns
  `failed precondition`; the pre-fault trap removes its task/container. No NBD
  server is killed and no fail-closed claim is made. Retained failure is mode
  0600/109,436 bytes/SHA `684cf368…`, exit 1, empty credential scan. The seed
  exceeds a practical private-root workload bound; retry will use a small seed
  while maintaining continuous read/write/sync loops.

- Immediate audit corrects the cleanup assumption: although trap commands
  suppressed their errors, the stopped task/container, one rootfs record, one
  live export, pool, and kernel instance remain; services are still healthy.
  Therefore the host is not clean and retry is forbidden until an explicit
  ordinary stopped-task delete completes durable release. No sensitive kernel
  command-line value from the interactive audit is copied into retained docs
  or evidence.

- Explicit stopped-task deletion returns `BACKEND_FAILURE: backend operation
  failed` before container deletion or pool return. This is a substantive
  teardown/recovery outcome, not a harness error. The retained state must be
  inspected for canonical close versus active helper before choosing the
  already-qualified daemon-reconcile/delete retry; no destructive state-file
  edit is permitted.

- Read-only diagnosis shows the failed delete durably advanced the exact
  export to `QUIESCING` with zero counters/no offline check, while its process
  record names PID 6341 but no `mkvsock-nbd` process exists. The exact log is
  only generation-bound READY→CLIENT_ACCEPTED and has no canonical close. The
  stopped task and child instance remain. This is correctly unrecoverable by
  ordinary release and must fail closed across daemon/host restart; it is not
  the intended ACTIVE-state fault case and does not close that requirement.

- Pre-reset daemon restart supplies an additional fail-closed proof: restart
  command returns before the simple service exits, then systemd records
  `reconcile: ... quiescing storage server is absent without a graceful close
  record`, result `exit-code`, with no helper. Durable generation remains
  `82ce4d15…`, record SHA `eee80879…`, log SHA `b90fec3c…`, and no terminal
  close. Mode-0600 evidence is 2,671 bytes/SHA `e161c742…`, exit 0, empty
  credential scan. Host reset is now safe to observe, not a recovery claim.

- GCE reset succeeds in mode-0600 465-byte control evidence (SHA `0dd1a85a…`,
  exit 0, empty credential scan) and advances boot to `2caa6744…`. Post-reset
  audit binds selected daemon `65b25284…`/`61fe95c8…` and proves the durable
  generation remains `QUIESCING` with zero counters/no offline check and one
  rootfs record, while ephemeral `/run/mkstorage`, kernel instances, and ctr
  tasks are zero. Container metadata remains. mkruntimed repeatedly exits on
  the exact no-graceful-close error (28 restarts observed); other three services
  remain healthy. The mode-0600 audit is 25,326 bytes/SHA `5170a8de…`, exit 0,
  empty credential scan. This proves fail-closed ownership across host reset,
  not workload-data durability or automatic cleanup.

- Before environment reset, mkruntimed is stopped and every stale durable
  input is hashed: lifecycle state/journal `8b6fb6e…`/`68f2527c…`, rootfs
  `03f88351…`, storage `f78dd205…`; mknetd is independently empty at
  `7aa7fc47…`. The only storage artifact is the exact 2-GiB root image under
  one mode-0700 task directory, and only stopped container metadata remains.
  These inputs will be moved intact to uniquely named root-only quarantine;
  none will be deleted or treated as product cleanup evidence.

- Quarantine reset moves all three hashed trees intact, recreates private
  empty directories, removes only orphan container metadata, resets failure
  accounting, starts mkruntimed, observes its socket after three waits, and
  verifies all four services active/zero-restart. Its final audit incorrectly
  assumes an empty lifecycle store eagerly creates `state.json`; the daemon
  validly leaves it absent until first mutation, so the log exits 1 before
  Kerf checks. Retained partial log is mode 0600/5,131 bytes/SHA `f8cc3c05…`,
  empty credential scan. Moves are complete; corrected read-only audit remains.

- Corrected audit proves all quarantine directories root-only with the four
  exact pre-move hashes, all services active/zero-restart, socket ready, and
  zero sandbox/rootfs/export/endpoint/storage/kernel/Kerf resources. However,
  its two ctr command substitutions accidentally run unprivileged inside
  `sudo test`, emit permission errors, and collapse to empty strings; the
  misleading `-pass` log (mode 0600/5,098 bytes/SHA `5b061663…`, exit 0,
  empty credential scan) does not prove ctr inventories. A corrected privileged
  ctr audit is mandatory before retry.

- The independent exact-source audit validates Kerf and the first 15 resource
  counters, including privileged default/moby ctr inventories all zero, then
  fails because a freshly empty rootfs store has no `state.json`. Retained
  failure is mode 0600/8,902 bytes/SHA `7afb6e5e…`, exit 1, empty credential
  scan. The audit is corrected to count an absent never-created rootfs or
  endpoint state file as zero; its 19-counter contract remains unchanged.
- The destructive qualifier retry is reduced from a 64-MiB to a 4-MiB seed
  and 4-MiB write iterations, preserving concurrent read/write/sync coverage
  below the observed workload bound. Its readiness loop now aborts immediately
  if the task stops instead of emitting 240 misleading probes.

- Both corrected scripts pass syntax validation, available `shellcheck`, and
  `git diff --check`; review confirms the audit's expected 19-counter string is
  unchanged and only empty-store handling plus bounded workload sizing changed.

- Commit `33b2afc81457198c3921e728f860b734d1e63e62` (`test: handle fresh
  runtime audit state`) isolates those two script corrections. Audit SHA is
  `7010e3da…`, qualifier SHA `0d6cbec4…`; exact 4,833,280-byte archive
  `/tmp/mksrc-33b2afc.tar` hashes `0a7e0f92…`.

- VM verifies archive `0a7e0f92…` and both corrected script hashes in a new
  root-owned exact tree. Corrected independent audit then passes all 19 zero
  counters, empty Kerf pool/instances, exact active daemon `65b25284…` hash
  `61fe95c8…`, and four active zero-restart services with terminal
  `G6_FINAL_RESOURCE_RETURN_PASS`. Mode-0600 evidence is 16,292 bytes/SHA
  `ef641bc6…`, exit 0, empty credential scan. The VM is clean for retry.

- The first `33b2afc` retry exposes two additional harness defects and does
  not arm the intended fault. Its entry `live_counts` still opens the absent
  fresh rootfs `state.json` unconditionally; command substitution masks that
  nonzero status, so 240 `FileNotFoundError` probes do not stop execution.
  The workload is then created, but its helper already has a canonical
  `MKNBD_SERVER_CLOSED` before injection, so the explicit guard exits 1 and
  the trap removes the task/container. Retained evidence is mode 0600,
  213,948 bytes, SHA `f3dd782a…`, exit 1, empty credential scan. Immediate
  read-only inspection shows both ctr inventories empty, no kernel instance,
  all relevant services active, and only the closed helper log retained in
  `/run/mkstorage`; therefore this is neither client-loss evidence nor a
  completed qualification. The fresh-state reader and gate error propagation
  must be corrected before retry, and the premature canonical close requires
  diagnosis rather than weakening the guard.

- Retained diagnosis binds boot `2caa6744…` to the exact helper log: after
  READY and CLIENT_ACCEPTED it records
  `MKNBD_SERVER_DISCONNECT_DURING_WRITE len=2031616 offset=144769024`, then a
  canonical synced close with 301 reads/10,448,896 bytes, 48 writes/15,278,080
  bytes, and 7 flushes. Durable release preserves the same counters and an
  offline-clean digest; both ctr inventories are empty and all four services
  active. Mode-0600 evidence is 2,591 bytes/SHA `f1e0aac0…`, exit 0, empty
  credential scan. Thus the 4-MiB repeated fsync writer plus independent sync
  loop caused a real guest disconnect before injection. The retry now uses a
  1-MiB seed and continuous one-block reads plus one-block fsynced writes,
  retaining read/write/flush pressure without multi-megabyte write bursts.
  `live_counts` also treats absent never-created state files as empty, tests
  its own command-substitution status, and the top-level entry gate explicitly
  exits on failure. These are harness changes only; the canonical-close guard
  remains strict.

- Revised qualifier passes `bash -n` and `git diff --check`; local
  `shellcheck` is unavailable and is not claimed. Commit
  `31a23181934938484e4225211bf5c03d283316c5` (`test: harden accepted client
  loss qualifier`) contains only the script change. Script SHA is
  `6207c48a…`; its exact 4,833,280-byte archive hashes `8d6c55da…`.

- Exact `31a2318` archive/script hashes verify in a new root-owned VM tree.
  The corrected destructive qualifier then passes on boot `2caa6744…` against
  selected production `65b25284…`/daemon `61fe95c8…`. Generation
  `435cefca…` reaches ACTIVE with READY→CLIENT_ACCEPTED and no terminal close;
  guest process evidence shows both continuous read and fsynced-write loops,
  while server `/proc` I/O shows 2,489 reads, 1,390 writes, and 73,383,936
  written bytes. Killing exact PID 7509 leaves it absent, retains the identical
  process-record SHA `6dcbaba3…`, and still has no close marker. A post-loss
  guest exec fails precondition. Daemon start initially returns 0 but the
  service immediately exits on exact error `active storage server disappeared
  after client acceptance; automatic session recovery is unsafe`; zero
  replacement helpers appear, the root image/record remain, durable ACTIVE
  generation is byte-for-byte unchanged, and the kernel instance/pool remain
  allocated. Terminal marker is `G4_STORAGE_ACCEPTED_CLIENT_LOSS_PASS`, exit 0.
  This closes the live accepted-client-loss/fail-closed behavior before reset;
  it intentionally does not claim cleanup or post-reset durability yet.

- Evidence hygiene correction: the raw verbose Kerf/process transcript exposed
  the ephemeral sandbox credential in guest command lines, so it was moved out
  of the evidence tree at mode 0600. The retained mechanical derivative redacts
  both credential spellings and otherwise preserves the successful transcript;
  it is mode 0600, 74,239 bytes, SHA `ab11a88c…`, with an expanded empty
  credential scan and exit 0. Future qualifier output uses non-verbose Kerf
  inventory. After derivative verification, the raw credential-bearing
  temporary file is removed; it is neither retained nor treated as evidence.

- Host-reset qualification exposes an unresolved production defect. GCE reset
  control succeeds (mode 0600/460 bytes/SHA `fa723789…`, exit 0, empty
  credential scan) and advances boot to `0fc9d4d8…`. On that boot, durable
  generation `435cefca…` is still ACTIVE, but its pre-reset accepted-client
  marker was ephemeral in `/run`. Storage reconciliation therefore creates a
  replacement mode-0600 record/log for the same generation (new SHAs
  `13a4c568…`/`79e5da23…`) and leaves a child process in the service cgroup.
  Rootfs reconciliation subsequently rejects the missing ephemeral owned
  bundle and drives mkruntimed into an auto-restart loop, so no kernel instance
  or ctr task is recreated and only container metadata remains. That later
  failure does not restore the violated storage invariant: the dirty image was
  already reopened automatically after accepted-client loss. Retained audit is
  mode 0600/35,439 bytes/SHA `c49da816…`, exit 0, empty credential scan. The
  accepted-client fact must become durable (or equivalent durable policy must
  distinguish pre-acceptance from post-acceptance absence) before this row can
  close; current host-reset behavior is explicitly not fail-closed.

- Focused process evidence removes ambiguity: leftover PID 1481 is exact
  deployed `mkvsock-nbd` SHA `95e886d6…`, argv-bound to the same image, port,
  and export generation `435cefca…`; fd 3 and fd 5 both hold the durable root
  image, fd 6 is its listening socket, and stdout/stderr target the replacement
  log. The replacement record matches PID/start-time 1481/2302 and image inode
  1179704. Its log contains only the exact READY marker—no client acceptance or
  close—while mkruntimed has reached 119 restart attempts. Mode-0600 evidence
  is 2,293 bytes/SHA `a43ea4ff…`, exit 0, empty credential scan. This proves an
  actual image-owning server restart, not merely stale record creation.

- Remediation now makes ACTIVE restart conditional on explicit exact
  pre-acceptance evidence. Backend observation distinguishes: live exact
  process; canonical close; accepted-client loss; exact log containing only
  the generation-bound READY marker; and unknown absence. Only that
  exact-READY-only case is restartable. An empty/missing runtime directory,
  absent log, or noncanonical trailing data is unknown and returns
  `active storage server is absent without exact pre-acceptance evidence`
  before image inspection/start. Existing same-boot pre-acceptance recovery is
  retained; accepted-client and unknown host-reset loss both remain owned and
  fail closed. Focused storage/lifecycle packages and the full runtime Go suite
  pass with a workspace-local `/tmp` build cache; qualifier syntax and
  `git diff --check` also pass.

- Production remediation is isolated in commit
  `ebbb2db89c70595c4abb5355f97f30bf28cba8d2` (`runtime: require proof before
  active storage restart`); evidence-hygiene-only commit
  `ca7d7d013485a2ddb764707e63fb2c35ab1ed72b` removes verbose Kerf output from
  the qualifier. Final qualifier SHA is `bcef69b9…`; exact combined
  4,833,280-byte source archive SHA is `a54f8988…`.

- VM verifies archive `a54f8988…`, service SHA `ff920f45…`, backend SHA
  `0692f25b…`, and qualifier SHA `bcef69b9…` in a new root-owned tree. Exact
  VM build embeds full revision `ca7d7d013485a2ddb764707e63fb2c35ab1ed72b`.
  Candidate hashes are manifest `a9e5de5e…`, mkruntimed `a4a91006…`, shim
  `baaab3fd…`, mknetd `5eed6606…`, and agent `6ae77e65…`. Mode-0600 build
  evidence is 5,056 bytes/SHA `a48e9343…`, exit 0, empty credential scan.

- Candidate installation atomically selects immutable release
  `0.1.0-dev-ca7d7d013485a2ddb764707e63fb2c35ab1ed72b`; linked daemon/shim hashes
  match build values `a4a91006…`/`baaab3fd…`. mkruntimed is deliberately
  stopped before selection and the known unsafe replacement PID 1481 remains
  until the qualification reset, preserving the exact faulted input rather
  than manufacturing clean state. Mode-0600 activation evidence is 3,562
  bytes/SHA `57431f4b…`, exit 0, empty credential scan.

- The first candidate post-reset audit is a harness-only failure: after the
  controlled reset exposed intermediate boot `9819413d…`, an additional
  operator restart advanced the host to `f93f21b6…`. The audit's exact
  intermediate-boot assertion therefore exits 1 immediately, before selector,
  journal, helper, durable-state, ctr, or Kerf assertions. Renamed retained
  failure is mode 0600/2,906 bytes/SHA `bf1850b0…`, empty credential scan; it
  proves no candidate behavior and is not a pass. Retry must bind to the
  current boot while still excluding the known pre-candidate boot.

- Current-boot retry supplies partial positive product evidence but remains a
  harness failure. On boot `f93f21b6…`, exact candidate revision/hash and
  selector verify; 50 observed daemon restarts all fail on `active storage
  server is absent without exact pre-acceptance evidence`, and the boot journal
  contains no `rootfs reconcile:` entry. `/run/mkstorage` is absent entirely,
  which is stronger than an empty directory, but the audit's unguarded `find`
  exits 1 there before helper-count, durable-state, ctr, Kerf, or terminal-pass
  assertions. Renamed mode-0600 failure is 57,262 bytes/SHA `b142ac2d…`, empty
  credential scan. It is not the final pass; corrected audit must count an
  absent never-created runtime directory as zero and complete remaining checks.

- Candidate reset control is mode 0600/460 bytes/SHA `2a9e8138…`, exit 0,
  empty credential scan; the operator's subsequent restart advances the final
  observed boot to `f93f21b6…`, adding another restart boundary. Corrected
  final audit passes against exact selected revision `ca7d7d01…` and daemon
  SHA `a4a91006…`. Across the entire boot journal, every reconcile attempt
  stops at `active storage server is absent without exact pre-acceptance
  evidence`, with no `rootfs reconcile:` execution. Runtime files=0 even when
  `/run/mkstorage` is never created; exact storage-server processes=0; durable
  state SHA remains pre-fault `c05b754d…`, with one unchanged ACTIVE generation
  `435cefca…`, sandbox generation `c4b8a9f6…`, zero untrusted terminal counters,
  and exact image device/inode 2064/1179704. ctr tasks=0, Kerf has no pool or
  instances, and container metadata alone remains. Terminal marker
  `G4_STORAGE_HOST_RESET_FAIL_CLOSED_PASS` exits 0. Mode-0600 evidence is
  15,346 bytes/SHA `d895049b…`, empty credential scan. This closes the
  accepted-client-loss behavior across host reset: the candidate retains
  ownership and never reopens the ambiguous image.

- 2026-10-07 resumed-instance checkpoint: GCE reports disposable instance
  `mklinux-g4-g6-final-20260905` (resource ID `6701540373796488780`) `RUNNING`,
  last started `2026-10-07T06:21:49.238-07:00`, at internal/external addresses
  `10.148.0.58`/`34.126.166.142`. Guest observation binds the new boot ID
  `8ef29982-c227-45b8-83a6-08b7af3c419b`, kernel
  `7.0.0-mk2-gce-lab`, hostname, and selected release `ca7d7d013485…`.
  `mkruntimed`, `mknetd`, containerd, and the Google guest agent were active
  with PIDs 1452/1235/1464/1083 and zero systemd restarts; Docker had PID 1533
  and was still `activating` during this first post-restart sample. Both ctr
  task and running-Docker-container inventories were zero. This is a baseline,
  not a qualification pass; Docker readiness and an all-resource audit remain
  to be checked before the next live suite.

- Post-restart follow-up observes Docker `active/running` at the same PID 1533.
  All four runtime services are now `active/running`, with unchanged PIDs and
  `NRestarts=0`; the durable storage-file listing and Kerf inventory are empty.
  The appended exact-process probe is not evidence: its double-quoted remote
  `awk $1` expanded in the outer `set -u` shell and aborted that final subcheck.
  A shell-safe strict audit remains required before workload execution.

- Shell-safe post-restart audit closes that baseline prerequisite. Local and
  guest copies of `audit-runtime-final-resources-live.sh` are byte-identical at
  SHA-256 `7010e3dae0e4fd6faafa0ff5c405d9874e05e02f6bad021bff83450ef5521821`.
  Retained transcript `20261007-post-restart-final-resource-audit.log` is mode
  0600, 13,364 bytes, SHA-256
  `585449523ecb3548f2dd1343c6d7a82cfeefb3ac0b60a8ed7979823ffa452f6e`,
  exits 0, and has zero credential-pattern matches. It binds boot
  `8ef29982…`, selected release `ca7d7d013485…`, daemon PID/hash
  `1452`/`a4a91006…`, no Kerf pool or instances, all 19 resource counters zero,
  and four active/running zero-restart services. The VM is clean for the next
  G6 mutation.

- G6 OCI capability checkpoint: local and VM SHA-256 values match exactly for
  the validator `ddd77fb8…`, 97-case test matrix `181bd6ae…`, installed-live
  harness `46000f5f…`, agent server `8afdf699…`, and shim `a1349318…`.
  Source inspection maps hooks, seccomp, namespaces, mounts, resources/cgroups,
  rlimits, capabilities, read-only root, hostname, masked/read-only paths, and
  canonical path controls to explicit accepted or fail-closed cases; guest and
  shim also share the exact required OCI feature set. VM transcript
  `20261007-g6-oci-capability-matrix-vm.log` is mode 0600, 510 bytes, SHA-256
  `019608e701d60056ae1320532e292da83b32e0fe396b6dcbb1197efe7a285daf`,
  exit 0, credential-pattern clean, and reports 97 semantic cases plus
  namespace, file-identity, and outer-cleanup boundaries. Installed-service
  pre-allocation behavior and post-run resource return remain to be observed
  before closing the OCI-scope row.

- Installed OCI qualification completes that pending observation.
  `20261007-g6-oci-preallocation-live-pass.log` is mode 0600, 133,211 bytes,
  SHA-256 `379a03bc23aadfc9a9f96f5d8ef3be90ecb16ed3cbcf15e709b0cf94935e8aa0`,
  exit 0, and credential-pattern clean. It binds boot/release/daemon/shim/
  builder identities; records the unsupported-profile exit 1 and exact
  pre-allocation rejection; shows no child, pool, mount, artifact, network,
  task, container, rootfs, endpoint, NBD, or relay allocation immediately;
  reaps the two transient shim wrappers; obtains exact positive-control output
  `MK_OCI_SUPPORTED_PASS`; and finishes at the all-zero 19-counter inventory
  with four healthy zero-restart services. Independent post-run transcript
  `20261007-g6-oci-final-resource-audit.log` is mode 0600, 13,364 bytes,
  SHA-256 `9e6005722e3e9e207302aa3602329636da600e9692c7ce41ae0963a77a30c475`,
  exit 0, credential-pattern clean, and independently repeats empty Kerf/all
  19 zeros. Together with the exact-source 97-case matrix this closes the OCI
  support/fail-closed row; it does not close the separate hostile path-race row.

- Stale-relay live-test design finding: installed `ctr tasks start` accepts a
  created container but exposes no command that stops after Task `Create` and
  before Task `Start`. The relay pathname is generation-qualified and exists
  only at Start, while its port/generation become authoritative during Create.
  A fast filesystem watcher would therefore turn the security claim into a
  scheduling race. The qualifier will instead use a source-controlled helper
  that invokes containerd `NewTask`, publishes an exact CREATED barrier, waits
  for a continuation file while the harness installs the root-owned stale Unix
  socket at the derived authoritative path, and only then invokes `Start`.

- First local barrier-helper check is not evidence and changed no VM state.
  The compound command was launched from `runtime/` while its chmod/gofmt/
  syntax paths were repository-root-relative, so those checks addressed no
  files. The subsequent build did locate `../scripts/runtime-task-barrier.go`
  but correctly stopped because the high-level containerd client activates
  transitive modules whose checksums are absent from the deliberately lazy
  `go.sum`. The retry must run formatting/syntax from the repository root and
  accept the helper only if a normal module download produces a reviewable,
  bounded checksum-only dependency delta.

- Corrected local barrier checkpoint: repository-root gofmt/Bash syntax and
  build pass. Normal `-mod=mod` resolution adds 30 checksum lines to `go.sum`
  and no `go.mod` requirement, a bounded consequence of importing the already
  direct containerd dependency's high-level client. Helper source/binary hashes
  are `6efb947e…`/`c5fa47a7…`; the live qualifier is mode 0755, 7,625 bytes,
  SHA `62dfe9bb…`. The full non-race runtime suite passes all 22 packages. This
  proves the qualification tooling compiles and preserves the product baseline;
  stale-socket behavior itself remains unclaimed until exact-source VM execution.

- Commit `df1d147566323bc40d553dc8e96a228b1a80005c` freezes the helper,
  qualifier, checksum delta, and running findings. Its exact Git archive is
  5,795,840 bytes/SHA-256 `1c71938d3c6b8aa453bc2e82cff489b1fd429a1e7d3021e5318ae9ab285e552b`;
  the guest verifies the same digest before extracting 661 files/5,243,520
  bytes into new `/var/tmp/mksrc-df1d147`. Guest helper/qualifier hashes match
  local `6efb947e…`/`62dfe9bb…`, and guest `go.sum` hashes `bc6646c9…`.
  This binds the next run to an immutable clean commit; no live result is yet
  claimed.

- First commit-bound stale-relay attempt is a harness-only failure before
  installing a stale socket or invoking Task Start. Containerd successfully
  creates task `mk-stale-relay-live` with holder PID 11948 and emits the helper
  barrier as lowercase status `created`; the qualifier expected uppercase and
  exits 1. Retained mode-0600 transcript is 101,206 bytes/SHA-256
  `b1e9b261eb3f3c2aa42eafb88488c13343576a5dddfc681ea2f99445a55f9075`,
  credential-pattern clean. `ctr tasks rm -f` cannot drive a CREATED task
  through the CLI precondition, so the helper now has a scoped
  `--start-existing` recovery mode and the trap uses it before kill/delete.
  The verified recovery binary hashes `10408c51…`: it starts the exact retained
  task, SIGKILL records exit 137, and bounded task/container deletion succeeds.
  Its first immediate audit sees only two reaping shim wrappers and exits 1;
  retained recovery transcript is mode 0600, 9,739 bytes/SHA `e33caf23…`,
  credential-pattern clean. After bounded wrapper reaping, strict audit
  `20261007-g6-stale-relay-first-cleanup-final-audit.log` is mode 0600, 13,512
  bytes/SHA `5f17ecd8…`, exit 0 and credential-pattern clean, with empty Kerf, all
  19 counters zero, and four healthy services. An earlier failed audit printed
  the ephemeral sandbox token in Kerf command-line output; its mode-0600 raw
  file (4,506 bytes/SHA `c8385175…`) was quarantined outside the evidence tree
  and is explicitly excluded. No stale-socket behavior is claimed from this
  attempt.

- Corrected recovery-safe helper/qualifier hashes are `c2d9c070…` and
  `0dd9d82f…`. Bash syntax, `git diff --check`, and the complete 22-package
  local runtime suite pass. A repeated working-directory typo stopped one
  command before checks and is not evidence; the root-directory rerun above is
  the valid result. Commit/archive binding and a fresh VM retry remain required.

- Commit `28d1a1023ed26b5bf23248041c35028e5bde31f0` freezes those corrections.
  Its 5,795,840-byte archive has SHA-256 `fa43e9eef33efa80c52f4c6d664a6c251f627965d5243a3f547881384787779d`
  locally and on the VM. New extraction `/var/tmp/mksrc-28d1a10` contains 661
  files/5,249,376 bytes; guest helper/qualifier hashes exactly match
  `c2d9c070…`/`0dd9d82f…`. The VM is strictly clean from `5f17ecd8…`; the next
  attempt is source-bound and starts from zero resources.

- The second exact-commit attempt proves the deterministic Task Create barrier
  and the new CREATED-task recovery path, but is still harness-only failure
  evidence. Task `mk-stale-relay-live` reaches lowercase `created` with holder
  PID 19868 at `2026-10-07T13:50:02.618832079Z`; the qualifier then exits 1
  before deriving or placing the stale socket because it incorrectly assumes a
  monolithic `/var/lib/mkruntimed/state.json`, which this installed release does
  not have. Its trap starts that exact CREATED task through the helper and then
  kill/deletes the task and container. Retained
  `20261007-g6-stale-relay-live-second.log` is mode 0600, 107,582 bytes,
  SHA-256 `db8b971af97f1afbd78c4a89fbe54ee51005e4b1d32d013ba358ceffe1cce184`,
  exit 1, and credential-pattern clean. This makes no stale-socket product
  claim; the next revision must derive the authoritative relay coordinates
  from the installed daemon interface/state layout and re-establish a strict
  zero-resource baseline before retrying.

- Read-only post-failure discovery resolves the path mismatch: the selected
  release's `/etc/mkruntime/config.json` is a symlink whose decoded
  `state_directory` is `/var/lib/mkruntime` (not `/var/lib/mkruntimed`). Its
  mode-0600 `/var/lib/mkruntime/state.json` has top-level keys
  `results,sandboxes,sequence,version` and currently zero sandboxes. The
  separate `/var/lib/mkruntimed` tree contains only rootfs/storage stores.
  `ctr tasks list` and `ctr containers list` are empty after trap recovery.
  The qualifier correction will read the state directory from the same strict
  host configuration consumed by mkruntimed, rather than embedding either
  pathname. An independent strict post-failure audit is still being captured.

- The first independent post-second-attempt audit is retained as bounded
  cleanup-progress evidence, not a clean baseline. It is mode 0600, 2,667
  bytes, SHA-256
  `e6d2d2ba9dd924b95e933a42c4def1b8cdbbde3c2da61d6e51884e3ab97fcbe0`,
  exit 1, and credential-pattern clean. Kerf has no instances and has allocated
  zero bytes, but its otherwise fully available 16-GiB pool is still configured,
  so the strict auditor correctly stops at its first nonzero lifecycle
  condition. This is not yet a zero-resource baseline; a bounded follow-up must
  show pool release before any retry.

- The configured-but-empty pool does not self-release within the explicit
  120-second follow-up bound (`pool_release_timeout`). This distinguishes it
  from ordinary asynchronous shim reaping and blocks the retry baseline. The
  daemon state still reports zero sandboxes and containerd reports no task or
  container, so the next step is read-only lifecycle/log inspection followed,
  if consistent with the documented idle-pool recovery contract, by a scoped
  mkruntimed restart and another independent strict audit. No product
  stale-relay claim is inferred from this cleanup-path observation.

- Source inspection explains that timeout and narrows it away from a product
  cleanup defect: `lifecycle.Service` intentionally retains an initialized
  pool across ordinary zero-sandbox intervals to avoid repeated large
  contiguous allocations, and `ReleaseIdlePool` returns it during graceful
  daemon shutdown. The live qualifier's normal epilogue already performs
  `systemctl restart mkruntimed` before its strict audit, but this pre-epilogue
  harness failure never reached that line. A manual scoped restart therefore
  follows the same source-controlled recovery path; its PID transition and
  strict post-restart inventory will be retained together.

- Scoped recovery passes. `20261007-g6-stale-relay-second-cleanup-recovery.log`
  is mode 0600, 13,162 bytes, SHA-256
  `a27b2fb4716a1d89ddccdb0e7a711e7d526c0e90ef5345724dd2adbba6d245e8`,
  exit 0, and credential-pattern clean. It binds zero pre-restart sandboxes,
  mkruntimed PID 12829 -> 21126, active state and `NRestarts=0`, then proves
  `No memory pool configured`, no instances, all 19 inventory counters zero,
  and four active/running zero-restart services (`G6_FINAL_RESOURCE_RETURN_PASS`,
  `RECOVERY_PASS`). The next harness revision will both decode
  `state_directory` from the installed config and perform this guarded idle-pool
  release in its failure trap once durable sandbox state is empty.

- The third qualifier revision implements both corrections. It reads the
  strict installed host config, joins its `state_directory` with `state.json`,
  and uses that same resolved snapshot for sandbox identity/port lookup and
  guarded failure cleanup. The trap restarts mkruntimed only when durable
  sandbox count is exactly zero and Kerf still reports a configured pool; its
  Kerf check is piped directly into a quiet predicate so instance command lines
  cannot enter the transcript. Bash syntax, ShellCheck (when installed), and
  `git diff --check` pass. The revised qualifier is SHA-256
  `2611d5c435b39dddeaf99fe85adfe6bfbc06b711b7349c1a5ee5d77e11248e3a`;
  the unchanged helper remains `c2d9c070…`. Commit/archive binding and a fresh
  exact-source VM run remain required before claiming stale-socket behavior.

- Commit `49acc614a30d0316e2f46405c7f9b9bd80bc1038` freezes the third
  revision and findings. Its exact Git archive is 5,816,320 bytes with SHA-256
  `0e778c73760cece17788a7157aa6b8a30ab3ffe8821121cf751ecde58e72fdf5`
  both locally and on the VM. Fresh `/var/tmp/mksrc-49acc61` contains 661 files
  and 5,260,859 bytes; guest qualifier/helper hashes match
  `2611d5c4…`/`c2d9c070…`, guest Bash syntax passes, and its pre-run strict audit
  again reports no pool/instances, all 19 counters zero, and four healthy
  zero-restart services. This establishes an immutable clean start for the
  third live attempt; transfer and preflight alone are not behavior evidence.

- The third exact-source attempt reaches the intended hostile-object boundary
  and supplies partial product evidence, but the qualifier still exits 1. Task
  Create publishes holder PID 28145; authoritative identity resolves agent
  port 7200, generation `7fd9e5bac156cc97032cac0d2023fc7a`, and relay path
  `/run/mk-agent-7200-7fd9e5bac156.sock`. Before Start, the harness installs a
  root-owned mode-0755 one-link socket at device 28/inode 3321 and proves
  `ECONNREFUSED`. Task Start then succeeds and the replacement identity check
  gets past the required socket/root/one-link/different-inode predicates, but
  its extra second-client `connect()` returns `ECONNREFUSED`, causing exit 1
  before the live-replacement observation is emitted. Retained
  `20261007-g6-stale-relay-live-third.log` is mode 0600, 111,256 bytes,
  SHA-256 `d270af0eea9e6bd2ed5ea15c1483280e72343131c38f3caa88e773e1c982901e`,
  and credential-pattern clean. The guarded trap kill/deletes the running task,
  sees zero durable sandboxes, and restarts mkruntimed to release the idle pool.
  This proves stale-path replacement occurred, but does not yet prove the
  workload or final strict cleanup; relay accept semantics must be inspected
  before deciding whether the second-client assertion is valid.

- Source resolves the third failure as another qualifier assertion error. The
  installed relay is built from `tools/mkvsock-relay.c`: `userver()` binds and
  listens, accepts exactly the shim's one agent connection, closes the listener,
  and then pumps that accepted stream. Therefore a second connection after
  successful Task Start is expected to get `ECONNREFUSED`; it is not a relay
  health test. The valid live proof is the already-established replacement
  inode plus successful task workload/agent operations and the running relay
  pump process. The next revision will retain the identity checks, remove only
  the invalid second connect, and continue to those behavioral assertions.

- Independent post-third cleanup is fully clean.
  `20261007-g6-stale-relay-third-cleanup-final-audit.log` is mode 0600, 13,027
  bytes, SHA-256
  `cea8a319995ad7c5897951a8dbc19c41b814de07b34b5fa9ce1c2932f3375891`,
  exit 0, and credential-pattern clean. It records mkruntimed PID 28297, no
  pool/instances, all 19 counters zero, and all four services active/running
  with zero restarts (`G6_FINAL_RESOURCE_RETURN_PASS`). The guarded failure
  cleanup is therefore independently proven and the next run again starts
  from zero resources.

- Work now pivots from the deferred live stale-relay retry to the still-open
  hostile-input/path-race matrix. New source-controlled qualifier
  `scripts/test-runtime-hostile-paths-vm.sh` defines five exact groups with 59
  named tests: 22 shim namespace/task/OCI/I/O/token/recovery/event/relay cases,
  17 rootfs mount/bundle/artifact/cleanup cases, seven descriptor-safe file
  publication cases, seven Unix-socket ownership/replacement cases, and six
  storage identity/path cases. It lists and verifies every selected test before
  running 100 race-detector repetitions on VM-local ext4 scratch, rejects any
  `--- SKIP:`, and brackets execution with the strict 19-counter resource
  audit. This is qualification design only; the user-restarted VM requires a
  new boot/identity/clean-baseline record and exact-commit execution before the
  matrix can close any row.

- Hostile-path qualifier preflight passes locally: mode 0755, 5,526 bytes,
  SHA-256 `eeffc269623f72f1bd62e2e62fc21e9b121198a9ee6ee8d40898d2b81c15d47a`;
  Bash syntax, available ShellCheck, and `git diff --check` are clean. These
  checks validate harness form only, not the selected test names or product
  behavior; exact test enumeration is deliberately rechecked by `go test
  -list` inside the guest before its race runs.

- The user-restarted instance is requalified before mutation. Resource ID
  `6701540373796488780` is RUNNING with GCE start
  `2026-10-07T17:39:59.195-07:00`, boot ID
  `7db90e79-7bf7-4ff8-befd-a3e1a5aff949`, kernel `7.0.0-mk2-gce-lab`, internal
  IP `10.148.0.58`, and new external IP `34.142.184.77`. The selected release
  remains `0.1.0-dev-ca7d…`; live daemon SHA remains `a4a91006…` at PID 1451.
  mkruntimed/mknetd/containerd/Docker are active/running with zero restarts at
  PIDs 1451/1232/1465/1513. Mode-0600 preflight transcript
  `20261008-post-user-restart-hostile-preflight.log` is 13,795 bytes, SHA-256
  `2ce224c2f7d569be1768b71e82ef923bd592df0554f86e2c73b40c2beb241d75`,
  exit 0, and credential-pattern clean; it proves no pool/instances and all 19
  resource counters zero (`G6_FINAL_RESOURCE_RETURN_PASS`).

- First exact-commit hostile-path execution is a deterministic fixture failure,
  not a product failure and not a passing matrix. Commit
  `252add21366ab4e07b17c00dc41057bb89087711` archives to 5,826,560 bytes/SHA-256
  `b6240462e1059ef720f1f1b09e707e5c2510a1c0e026683959a82e37d7608bfb`
  locally and on the VM; fresh extraction has 662 files/5,274,915 bytes and
  exact qualifier SHA `eeffc269…`. The qualifier enumerates all 22 shim tests,
  then the 100-repeat race run passes the other selected hostile cases but
  fails `TestStaleRelayCleanupRemovesExactSafeSocket` in every repetition. That
  test alone uses raw `t.TempDir()`, which inherits mode 0775 from the VM's 002
  umask; production correctly rejects its group-writable Unix-socket parent.
  Retained `20261008-g6-hostile-path-race-matrix.log` is mode 0600, 1,038,905
  bytes, SHA-256
  `a23690c54ca8ec32370e3c0295d88809674d418c22333919f5b6cd2af79a9230`,
  exit 1, with exactly 100 failure lines, zero skips, no race report, and no
  credential-pattern match. Later groups do not run after the shim command
  fails. The correction is to use existing `privateTestDirectory(t)` mode 0700;
  product socket policy must not be weakened.

- Independent cleanup after that mutation-free test failure is clean.
  `20261008-g6-hostile-path-first-failure-final-audit.log` is mode 0600,
  13,022 bytes, SHA-256
  `ec0730ce098dfc2703ee24f6b182a9c1d7260e6fc124c7654640199e21eb0905`,
  exit 0, and credential-pattern clean. It preserves the restarted boot/service
  PIDs, reports no pool or instance, all 19 counters zero, and all four services
  active with zero restarts (`G6_FINAL_RESOURCE_RETURN_PASS`). The fixture now
  uses `privateTestDirectory(t)`, and the qualifier itself sets umask 077 before
  creating scratch/cache directories; verification and exact-commit rerun are
  pending.

- The corrected safe-socket fixture passes 100 race-detector repetitions
  locally in 1.062 seconds. Root-level Bash syntax, available ShellCheck, and
  `git diff --check` pass; the revised qualifier hashes to
  `f5b2059e01bae4219fa829d75ef5b0e1aa48d303b36e19637b5c1376e204d31d`.
  One preliminary syntax invocation repeated the known subdirectory/root-path
  mistake and found no script; it is excluded, while the immediately repeated
  repository-root checks above are authoritative. Full exact-source VM matrix
  execution remains required.

- Commit `595ebbe7dabebbdb86862a7f29270beef7030485` freezes the fixture and
  qualifier corrections. Its exact 5,836,800-byte archive has SHA-256
  `94b0b1d1b6fff3a22d208ff6c37ffbcab3116318c8b8a35f30c8431587d8216b`
  locally and on the VM. Fresh `/var/tmp/mksrc-595ebbe` has 662 files/5,281,042
  bytes; guest qualifier/test hashes are `f5b2059e…`/`32fa942b…`, guest syntax
  passes, and another strict preflight is all-zero on boot `7db90e79…`. The
  corrected complete matrix can now run from an exact clean source/baseline.

- The `595ebbe` rerun proves the safe-socket fixture correction but finds a
  qualifier-level environment error: global `umask 077` changes intentionally
  permissive fixtures into private objects. Consequently exactly two selected
  tests fail in all 100 repetitions—unsafe I/O reports “group-accessible FIFO
  was accepted” because requested 0660 became 0600, and permissive token state
  reports “unsafe existing token accepted” because requested 0666 became 0600.
  The corrected stale-socket test and other selected cases pass; there are zero
  skips and no race report. Retained mode-0600 transcript
  `20261008-g6-hostile-path-race-matrix-corrected.log` is 1,040,961 bytes,
  SHA-256 `690fd371cff491a5c39645ce2a63d82ff35c12047638bc78bf3c2d466d8c3e9b`,
  exit 1, with exactly 200 top-level failure lines and no credential-pattern
  match. The global umask must be removed; only qualifier-owned scratch/cache
  directories should be explicitly chmod 0700 so product rejection fixtures
  retain their requested modes.

- Independent post-umask-failure audit again passes. Mode-0600 transcript
  `20261008-g6-hostile-path-second-failure-final-audit.log` is 13,022 bytes,
  SHA-256 `ddf835267d0202d2ed2cd9b0ef6bd9c683b8e680f1bec8ff9ede317a578de3f1`,
  exit 0, and credential-pattern clean, with no pool/instances, all 19 counters
  zero, and unchanged four active zero-restart services. The qualifier now
  leaves process umask untouched and explicitly sets only its three owned
  scratch/cache/tmp directories to 0700; validation and a new exact-commit run
  remain pending.

- The final qualifier correction passes Bash syntax, available ShellCheck, and
  diff hygiene. Under an explicitly reproduced umask 002, the fixed stale
  socket case and both intentionally permissive I/O/token cases pass together
  for 100 race-detector repetitions in 1.241 seconds. Revised qualifier SHA-256
  is `214c801ef71e99d9afb6a824bc5a45535a828aaf8392bf02764fd6695718afc6`.
  This closes the observed fixture/harness regressions locally; the five-group
  VM aggregate remains unclaimed until exact-commit execution passes.

- Commit `27ffedc167404bda0b6067c82f9a8fb305029fe1` freezes the final qualifier.
  Its exact 5,836,800-byte archive SHA-256 is
  `527783a30b17fa4ff2fbcd4079859babeeba42f9cf27674407472a76d4260f1e`
  locally and on the VM. Fresh `/var/tmp/mksrc-27ffedc` contains 662 files and
  5,286,177 bytes; guest qualifier and corrected test-source hashes match
  `214c801e…` and `32fa942b…`, and guest syntax passes. The next complete run is
  therefore exact-source bound; no aggregate pass is inferred from transfer.

- Running `27ffedc` matrix checkpoint: exact enumeration and 100 race-detector
  repetitions have completed for the 22-test shim group and 17-test rootfs
  group, each with zero skips and status pass. This includes the three formerly
  distorted fixtures under the VM's native umask. The seven-test safefile group
  is active; Unix-socket, storage, and final strict audit results remain
  unclaimed until the same process completes.

- The `27ffedc` run confirms shim (22), rootfs (17), and safefile (7) groups
  through 100 race repetitions each with zero skips, then fails only two of the
  seven Unix-socket selections in all 100 repetitions. Both report `bind:
  invalid argument`, not a policy assertion: the qualifier's long
  `/var/tmp/mk-hostile-paths.XXXXXX/tmp` prefix plus Go's long test directory
  exceeds Linux `sockaddr_un` for those names. Storage and final audit are not
  reached. Retained mode-0600
  `20261008-g6-hostile-path-race-matrix-final.log` is 1,743,744 bytes, SHA-256
  `f7a5de01e498d4245058fdec32d3343fa12e25bfdf11fb49fb50861fb40f4e1c`,
  exit 1, exactly 200 top-level failure lines, zero skips/race reports, and
  credential-pattern clean. The qualifier must keep ext4 while using a short
  root such as `/var/tmp/h.XXXXXX` directly as TMPDIR/GOTMPDIR.

- Independent post-path-length audit passes: mode-0600
  `20261008-g6-hostile-path-third-failure-final-audit.log` is 13,022 bytes,
  SHA-256 `92cf27d895b030e753bcf727d748148ec46bc7c204bb0992ee504ac2a9c85f12`,
  exit 0, and credential-pattern clean, with no pool/instances, all 19 counters
  zero, and unchanged active zero-restart services. The qualifier now uses
  `/var/tmp/h.XXXXXX` directly for TMPDIR/GOTMPDIR and a short `c` cache child;
  validation and a new exact-source run are pending.

- Short-root validation passes: all seven selected Unix-socket tests complete
  100 local race-detector repetitions in 1.179 seconds using an equivalently
  short private temp root, with Bash syntax, available ShellCheck, and diff
  hygiene clean. Revised qualifier SHA-256 is
  `33c62163f1f11c77755649091a895151e64ca4529c26908c5675c2e3f0c1d983`.
  This validates the path-capacity correction locally; the VM aggregate is
  still open.

- Commit `b750ec4a2027320f514267930b83fe8565bbf1f4` freezes the short-path
  qualifier. Its exact 5,847,040-byte archive hashes to
  `7716916d8408d371556a21389fec77f36ee3383e8b43e899bb4d68276aeea297`
  locally and on the VM. Fresh `/var/tmp/mksrc-b750ec4` contains 662 files and
  5,291,293 bytes; guest qualifier/test-source hashes match
  `33c62163…`/`32fa942b…`, and syntax passes. A complete one-run aggregate is
  still required despite the prior partial group passes.

- Active `b750ec4` checkpoint: shim 22, rootfs 17, safefile 7, and Unix-socket
  7 groups have each completed 100 race-detector repetitions with zero skips
  and explicit pass markers. The short temp root eliminates the prior bind
  failures. The six-test storage group is active; aggregate and final resource
  audit remain unclaimed.

- The final exact-source aggregate passes. Mode-0600 transcript
  `20261008-g6-hostile-path-race-matrix-pass.log` is 1,888,866 bytes, SHA-256
  `e9b9a241b86baec3f56e3294a54956447978b82a5136d3b13478c0147f5270d9`,
  exit 0, and credential-pattern clean. It independently enumerates and runs
  shim 22 + rootfs 17 + safefile 7 + Unix-socket 7 + storage 6 = 59 exact
  tests for 100 race repetitions each, with zero failures, skips, or race
  reports. Both embedded strict audits report no pool/instances, all 19 counters
  zero, and four active zero-restart services at unchanged PIDs. Group markers,
  `groups=5 selected_tests=59 iterations=100 skips=0 status=pass`,
  `G6_FINAL_RESOURCE_RETURN_PASS`, and
  `G6_HOSTILE_PATH_RACE_MATRIX_PASS` are present. This closes the broader
  source path-race matrix evidence; the parent implementation row stays open
  only for the separately deferred complete live stale-relay workload proof.

- Independent post-pass audit `20261008-g6-hostile-path-final-independent-audit.log`
  is mode 0600, 13,022 bytes, SHA-256
  `b972d661960eb5fd0f636363ee01a1114a1812328fd502172cc366e6bc009b59`,
  exit 0, and credential-pattern clean. It independently repeats no Kerf pool
  or instances, all 19 counters zero, and the same active zero-restart service
  PIDs 1451/1232/1465/1513. This completes the source path-race evidence without
  overstating the deferred live stale-relay workload check.

- The next open G6 automated gap is now represented by source-controlled
  `scripts/test-runtime-task-v2-exhaustive-vm.sh`. It aggregates 87 exact tests
  into lifecycle (20), process/I/O (33), control/read (14), and
  delete/event/recovery (20) groups. Together they exercise all 17 implemented
  Task RPC entry points, supported and explicitly excluded methods, valid and
  invalid transitions, retry/duplicate semantics, ordered events, exact exit
  state, and normal/fallback cleanup. Every name is checked with `go test
  -list`; each selected test must pass 20 race-detector repetitions with zero
  skips/race reports, bracketed by strict resource audits. Local qualifier
  preflight passes Bash syntax, available ShellCheck, and diff hygiene; mode is
  0755, size 7,128 bytes, SHA-256
  `4553faf1a0af607f55776f79cc641fe175fad34d28aad0b6ad2e8eb858ff48c7`.
  This records scope and harness form only; exact-source VM execution remains
  required before closing the fake-daemon row.

- Static pre-bind audit finds exactly 87 unique selected names, zero missing
  functions in the shim test source, and zero duplicate selections. This guards
  the pending commit against a stale or accidentally repeated test list; the
  guest's compiled `go test -list` check remains the authoritative runtime
  enumeration.

- Commit `fdb15d73cf2625174b3a06633b423121029707b9` freezes the Task v2
  aggregate. Its exact 5,857,280-byte archive hashes to
  `fd2a4af4e80ab382c3c65b369c71c1e5dfb7206bf16012636581449d171de583`
  locally and on the VM. Fresh `/var/tmp/mksrc-fdb15d7` contains 663 files and
  5,305,432 bytes; guest qualifier/test-source hashes match
  `4553faf1…`/`32fa942b…`, syntax passes, and the strict preflight again proves
  no pool/instances plus all 19 counters zero on boot `7db90e79…`. Execution is
  now exact-source bound; no Task matrix result is yet claimed.

- Active Task v2 checkpoint: the 20-test lifecycle group completes 20
  race-detector repetitions with zero skips/race reports and status pass. The
  33-test process/I/O group is active and has reached resize, lost-reply kill,
  guest-delete reconnect, CloseIO, and FIFO cases without failure. Control/read,
  delete/event/recovery, aggregate, and final audit remain unclaimed.

- Final Task v2 aggregate result: lifecycle 20, process/I/O 33, control/read
  14, and delete/event/recovery 20 each complete 20 race-detector repetitions
  with status pass. The aggregate reports `methods=17 groups=4
  selected_tests=87 iterations=20 skips=0 races=0 status=pass` and
  `G6_TASK_V2_EXHAUSTIVE_MATRIX_PASS`. Mode-0600 transcript
  `20261008-g6-task-v2-exhaustive-matrix.log` is 810,938 bytes, SHA-256
  `d2ab3c84ca4ee92abc5faf5015dc1eb343b88893bd082ff26d7b38a8e6b59b2d`,
  and wrapper exit 0. Direct transcript scans find zero top-level failures,
  skips, race reports, and credential-pattern matches. Strict audits before and
  after both emit `G6_FINAL_RESOURCE_RETURN_PASS`; the final inventory has no
  Kerf pool/instances, all 19 counters zero, and mkruntimed/mknetd/containerd/
  Docker active with unchanged PIDs 1451/1232/1465/1513 and zero restarts.
  This closes the fake-daemon automated-test row without claiming any separate
  live fault-injection row.

- Independent post-aggregate audit
  `20261008-g6-task-v2-final-independent-audit.log` is mode 0600, 13,358
  bytes, SHA-256
  `bcba3c7fb4dc9c5406f1377f13725290834483c5cb8220ae4d56befba8d9361d`,
  exit 0, and credential-pattern clean. It independently repeats no Kerf pool
  or instances, the exact all-zero 19-counter inventory, and unchanged active
  zero-restart PIDs 1451/1232/1465/1513, with
  `G6_FINAL_RESOURCE_RETURN_PASS`. Cleanup evidence therefore survives a
  separate SSH command after the aggregate has exited.
