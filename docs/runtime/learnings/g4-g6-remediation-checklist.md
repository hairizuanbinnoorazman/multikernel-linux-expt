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
| G4 | The current tree builds and verifies canonical manifests and deterministic newc roots, rejects observed source mutation and unsafe metadata, produces bounded fully allocated private ext4 images, generation-binds one mediated export, journals graceful teardown/recovery, and locally verifies a materialized read-only bind-input subset. Earlier live runs only prove the narrower BusyBox/private-write MVP. | Configured persistence and writable host volumes remain incomplete. Read-only bind enforcement still needs privileged live proof. Exhaustion, corruption, server-loss, host-reset, clone, cross-export, and replacement-instance evidence matrices have not passed on the current revision. |
| G5 | The current tree contains `mknetd`, CNI 1.0 `ADD`/`CHECK`/idempotent `DEL`, generation-bound endpoint state, negotiated MTU/DNS, bounded exchange/counters, restart reconciliation, and exact-address anti-spoof/firewall policy. Earlier live runs only prove static-link networking. | The CNI implementation and complete firewall CHECK have automated coverage but no current-revision live proof. Traffic, MTU/load/fault, restart, spoof/bypass, primary-health, and cleanup evidence matrices remain open. |
| G6 | The current tree implements the core Task v2 lifecycle, faithful versioned guest PIDs, pause/resume/stats, standard OCI process controls, durable task/process/I/O offsets, a supervised shim worker, and generation-bound task reconstruction. Earlier live runs prove only the narrower lifecycle/I/O MVP. | Current-revision forced-shim reconstruction remains live-unproved. Durable event replay, complete cancellation/FIFO/race matrices, Docker restart, packaging upgrade/rollback, and evidence-grade shared and isolated reruns remain incomplete. |

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

- [ ] Generate a deterministic image/root manifest and verify it before any
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
- [ ] Make initramfs generation reproducible, not merely sorted with a
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
  complete; reproducibility and publication still require disposable-host
  evidence before the G4 rows may close.
- [ ] Reject unsafe paths, traversal, escaping symlinks, unsupported file
  types, device nodes, inconsistent hardlinks, malformed metadata, and input
  mutation during the copy/build window.
- [ ] Validate OCI image architecture against the selected child-kernel
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
- [ ] Preserve the caller snapshot as containerd-owned input. Mount it with the
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
- [ ] Define and enforce single-owner writable-root identity, generation,
  duplicate-attach prevention, and stale-lock handling rather than relying
  only on one private initramfs per current shim.
- [ ] Decide the supported writable-state model. Implement private writable
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
- [ ] Add capacity accounting, block/inode quotas, a high-water refusal policy,
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
- [ ] Implement the storage teardown and recovery sequence appropriate to the
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
  persistence, and proof that unconfigured writes do not persist. Local tests
  now cover bind admission/materialization rejection and metadata identity;
  privileged guest write rejection and all persistence cases remain open.
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

- [ ] Explicit child-to-primary, outbound TCP, outbound UDP, DNS query/answer,
  and return-traffic assertions; retain destination and response details.
- [ ] Two sandboxes with overlapping internal names but distinct network
  identity, plus positive allowed routing and negative default isolation.
- [ ] MTU boundaries, fragmentation, checksums, malformed/oversized frames,
  loss, reordering, burst traffic, sustained load, and slow readers. A
  socketpair-backed shim pump suite now proves exact-MTU bidirectional
  forwarding, oversized primary ingress and guest egress drops, exact counter
  increments, disconnected-packet loss accounting, and retry-until-success
  reconnect after two injected failures. Fragmentation, checksum, ordering,
  load, and slow-reader coverage remain open.
- [ ] Agent transport disconnect/reconnect, child restart, networking-service
  restart, `mkruntimed` restart, shim death, and primary restart. Focused pump
  coverage now proves exchange disconnect detection and authenticated reconnect
  retry before later traffic succeeds; the cross-process restart matrix remains
  open.
- [ ] CNI failure after every partial `ADD` boundary, repeated `CHECK`, repeated
  `DEL`, stale namespace/link/rule cleanup, and name/address reuse. CNI stdin
  now rejects a valid JSON prefix followed by bytes beyond its one-MiB limit;
  cache creation rejects a symlinked ancestor before creating redirected
  directories; and no-replace publication makes an exact generation replay
  idempotent while refusing to overwrite a conflicting generation. Focused
  tests prove all three boundaries; the remaining repeated/fault matrix stays
  open.
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
  exit completion. An injected first `StateProcess` reply loss now requires one
  relay reconnect and restores the exact PID in 100 race-detector repetitions;
  reconstruction state and stopped-state wait reads use the same bounded
  idempotent path. Reconstruction cleanup ownership begins immediately after
  network-descriptor acquisition; a forced relay-start failure proves the
  descriptor, command, and socket identity are released. Forced-death process
  continuity and live identity evidence remain open.
- [ ] Define ownership transfer for containerd restart, shim restart, daemon
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
  These changes still need live revalidation.
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
  apply the no-symlink, stable-identity contract described above; the broader
  hostile-input/failure matrix and disposable-host evidence remain open. The
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
  as recorded in the 2026-09-18 checkpoint; the broader disposable-host
  path-race matrix remains open.
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
- [ ] Expand OCI support required by the agreed G6 scope, or keep each omitted
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
  with end-to-end tests. Privileged service activation and live
  upgrade/rollback evidence remain open. Service and configuration files now
  have a separate immutable generation manager: it validates operator runtime
  environment and host config input, hashes the fixed systemd/CNI/containerd
  assets and the complete rootfs builder/helper/guest-init support set, refuses
  unrelated paths or unsafe directory ancestry, switches all
  managed links atomically through one selector, supports exact rollback, and
  preserves generations on ownership-safe uninstall. End-to-end alternate-root
  tests cover fresh install, upgrade, rollback, dry-run/uninstall, inactive
  removal, collisions, invalid input, and symlinked installation ancestry.

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
  The remaining signal/exit/churn and live matrix is open.
  The descendant process-group signal test now waits for the terminal marker
  value instead of treating its earlier ready value as a terminal failure,
  eliminating a false negative while preserving the two-second bound.

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
