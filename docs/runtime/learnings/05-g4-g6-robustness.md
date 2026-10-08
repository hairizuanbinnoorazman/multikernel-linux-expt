# G4-G6 robustness follow-up

## 2026-10-01 continuation

The retained-pool correction and its clean-boot shared-matrix pass are recorded
in the remediation checklist. The next live qualification boundary is split
deliberately: one focused run will restart containerd around a live ctr task,
and a separate focused run will restart Docker around a live Docker container.
Both must retain service and child identities, pre/post exec and stdio, event
observations, exact deletion, and full cleanup inventory before either restart
row is described as proved. Initial live inspection found Docker live restore
disabled on the qualified host, so an immediate Docker restart would test
configured shutdown rather than reconnect continuity. The containerd case can
proceed separately; Docker needs a validated live-restore configuration first.
The focused containerd input is frozen at SHA-256 `46fe6437…`; no result is
claimed until the guest independently matches that hash and the retained run
closes with exact identity, event, stdio, deletion, and inventory assertions.
The guest now independently matches that hash and validates Bash syntax; this
is transfer provenance, not restart evidence.

The first immutable focused run proves the live core—containerd PID changed,
host and child boot identities did not, the task remained running, pre/post
exec succeeded, and the original FIFO stream delivered init output from both
sides of the restart—but exits 1 because attach had already removed the exited
task while the harness required a visible `STOPPED` row. Exact cleanup is zero
apart from the intentionally retained pool; an idle mkruntimed restart released
it. The transcript is retained as a failed run, and the lifecycle assertion
must be corrected and rerun before marking the row passed.

The narrowly corrected harness is now frozen and guest-verified as
`dbc4630c…`; its replay starts from the unchanged boot with a released pool and
empty default namespace. No corrected-run result is claimed yet.

The corrected retained replay passes. Containerd changes PID while the host,
mkruntimed, running task, and child boot identities remain stable; exec and the
original init stdout/stderr stream work on both sides; create/start and
post-restart exec/exit/delete events are retained; cleanup reaches a retained
pool with every other count zero and then a fully released all-zero state.
Independent audit agrees. The mode-0600 transcript hashes to `afa9187e…` and
closes exit 0. This proves only containerd live-task restart continuity;
Docker-daemon restart remains open because live restore is disabled.

The Docker merger now has a separate explicit live-restore opt-in rather than
changing runtime-registration defaults. Focused tests pass for preservation,
idempotence, conflicts, and option exclusivity. This is local configuration
tooling only; no guest policy or Docker behavior is yet claimed.

The exact live rollback baseline is retained by identity: active Docker config
SHA-256 `7861303c…`, mode 0644, runtime-only, effective default `runc`, and live
restore false. No guest config change is claimed at this checkpoint.

The validated opt-in candidate is now active as `a49bee79…`, while the exact
`7861303c…` prior file is retained for rollback. A reload made live restore
effective without changing Docker PID or the `runc` default. This is activation
evidence only, not a daemon-restart continuity result.

The focused Docker-restart input is frozen at `ab0cea4e…` with explicit
identity, exec/log, continuous event, deletion, and inventory assertions.
Transfer and execution remain unclaimed.

Guest hash/syntax and the released, empty, live-restore-enabled replay baseline
now pass on unchanged boot `768706da…`; no restart result is yet claimed.

The Docker restart replay passes separately from containerd. Docker changes
PID while containerd, mkruntimed, host boot, running state, and child boot stay
stable; pre/post exec and retained init logs pass; one moby event stream spans
create/start through post-restart exit/delete; exact removal converges through
retained-pool cleanup to a released all-zero state. Independent audit agrees.
The mode-0600 transcript hashes to `fa381592…` and closes exit 0. Combined with
the `afa9187e…` containerd transcript, the dedicated replacement-instance
daemon-restart evidence row is complete, without implying exhaustive event,
forced-shim, FIFO/cancellation, or packaging coverage.

Commit `43ea792a00030e193cf86e3365a0dad0867e0a5b` freezes the
configuration opt-in, tests, focused live harnesses, and operator guidance.
The running findings and evidence remain outside that scoped commit. Full
repository documentation/evidence checks pass; only the already classified
local socket `EPERM` subcase skips.

The next evidence target is forced shim death. The old recovery harness has
obsolete network cleanup checks and lacks recovery-record, journal, event, and
complete inventory evidence. Final qualification will separate recoverable
worker death from deliberately unrecoverable supervisor death and record both
without manually deleting runtime ownership state.

The replacement two-case input is frozen at `af51e5a9…`; it has bounded fault
waits and emits only a sanitized non-secret recovery summary. Guest transfer and
execution remain unclaimed.

Guest hash/syntax and the released, empty replay baseline now pass on unchanged
boot `768706da…`; no forced-death behavior is yet claimed.

## Result

The 2026-09-01 follow-up on disposable GCE instance
`mklinux-gates-20260901` remains **provisional** and supplied useful operator
observations about several fresh-host and recovery gaps. The host used the
qualified `7.0.0-mk2-gce-lab` kernel, an `n2-standard-16` machine, and an
auto-delete 100 GB boot disk restored from
`mklinux-lab-pre-daxfs-20260828-2030`.

The operator recorded that:

- containerd restart reconnected to a running shim and retained the same child
  boot ID;
- `mkruntimed` restart retained the same running child and boot ID;
- an injected initramfs-builder ENOSPC error failed creation before allocating
  a child or network resource;
- forced shim death safely reclaimed the Kerf child, memory pool, TUN device,
  NAT rule, and forwarding rules after network recovery identity was persisted.

Only the narrative evidence README and its manifest were retained. There is no
raw box transcript, service journal, before/after process and network state, or
failure output. These statements are useful observations and implementation
leads, but they do not by themselves close the checked containerd-restart,
shim-reclaim, or ENOSPC evidence requirements.

## Fresh-host fixes

The snapshot deliberately lacked runtime packages and image caches. Recreating
the service exposed bootstrap assumptions that the earlier warm-host MVP did
not catch:

- `make sync` omitted `guest/mk-agent-init`;
- `/sys/fs/multikernel` was not mounted by a persistent unit;
- Docker needed the Runtime v2 `runtimeType` registration rather than a
  runc-compatible `path` registration;
- the baseline proof assumed BusyBox was already present in both image stores.

The sync list, systemd mount dependency, Docker example configuration, and
explicit image pulls now cover these conditions.

## Remaining formal failures

- On the robustness-test build, terminal task creation was rejected before
  child allocation. Consequently terminal resize could not be exercised in
  that retained live run.
- Forced shim death now has bounded, leak-free safe reclaim, but the running
  task is not reconstructed or reconnected. Safe reclaim is not equivalent to
  the G6 reconnect requirement.
- No CNI configuration or Multikernel CNI binary was installed; normal CNI
  `ADD`, `CHECK`, and `DEL` remain unimplemented.
- The injected ENOSPC rollback covers only one storage failure point. Metadata,
  malformed-image variants, inode/block exhaustion, high-water refusal,
  persistence, corruption, and recovery still require the full G4 matrix.

The operator-recorded final inventory contained no Multikernel instances,
containerd tasks or containers, Docker containers, `mkn*` links, Multikernel
NAT/forwarding rules, or temporary systemd environment overrides. Raw
inventory output was not retained for this run.

## Shared `ctr` and Docker feature matrix

On 2026-09-01, a new disposable `n2-standard-16` instance,
`mklinux-g4-g6-matrix-20260901`, was restored from the qualified pre-DAXFS
snapshot and rebuilt from repository commit
`5e9fe33b7d54273c987fd21673f34ccde2121018` plus the working-tree matrix
harness. The host qualification report passed before the runtime was used.

The executable matrix then passed through both clients:

- image pull/inspection, combined foreground run, and split create/start;
- state inspection, exec, stdout/stderr, blocking wait, and nonzero exit;
- TERM, exit observation, deletion, narrow tested-resource return, and two clean
  same-name reuse cycles;
- distinct child-kernel identity, private writable roots, outbound DNS/HTTP,
  and bidirectional sibling-link isolation; and
- `mkruntimed` restart while both clients' workloads remained live, preserving
  both child boot IDs.

Terminal/resize and pause/resume were also invoked through both clients and
rejected while leaving no resources behind. Those are confirmed unsupported
paths, not passes. Task `Stats`, `Update`, and `Checkpoint`, guest stdin/attach,
faithful guest PIDs, full OCI controls, CNI, and shim-crash task reconnection
remain unimplemented or partial. Containerd 2.2.2 does not expose a standalone
`ctr tasks wait`; the blocking `ctr run` path exercised Task `Wait`, while
Docker was additionally checked with `docker wait`.

The exact command mapping is kept in the [top-level feature matrix](../../../README.md#g4-g6-ctr-and-docker-feature-matrix).
Raw proof, component hashes, image digest, qualified-host report, and the final
clean inventory are indexed in the
[feature-matrix evidence](../../../evidence/runtime-20260901/g4-g6-feature-matrix/README.md).
This expands the proved G4-G6 surface but does not close the full gates.

The retained transcript contains feature-row markers rather than expanded
commands and observed values. The exact harness hash makes the assertions
traceable to the test source, but the dirty source tree has no retained diff.
Its manifest is not schema-valid because `${HOST_BOOT_ID}` is not a valid boot
ID. The final inventory also does not cover rootfs mounts, generated artifacts,
FIFOs, relay/shim processes, and recovery records.

## Post-matrix terminal implementation

After the disposable matrix host was deleted, the child agent gained a real
PTY path for init and exec processes and a validated `ResizeProcess` operation.
The Runtime v2 shim now accepts terminal tasks, forwards `ResizePty`, and
retains an initial window size sent before process start. Terminal execution,
combined terminal output, live resize, invalid-size rejection, and pre-start
resize retention pass local unit tests and the Go race detector.

## Guest I/O and terminal live revalidation

On 2026-09-02, disposable instance `mklinux-g4-g6-io-20260901` was restored
from the qualified snapshot and ran the updated shared matrix to completion.
Both `ctr` and Docker passed foreground guest stdin, detach followed by live
reattachment, terminal execution, and propagation of a 91-column by 37-row
terminal size.

The run exposed and fixed two fresh-host-only gaps before the retained pass:

- Task `CloseIO` could overtake bytes already buffered in the stdin FIFO. The
  shim now drains those bytes before delivering guest EOF.
- The guest initramfs mounted `devtmpfs` but not `devpts`, so accepting
  `terminal=true` still left PTY allocation unusable. The guest now mounts
  `devpts` before binding `/dev` into the OCI root, and the private OCI config
  preserves the caller's terminal bit.

The final qualified-host report passed, the matrix ended with
`G4_G6_CTR_DOCKER_FEATURE_MATRIX_PASS`, all child/container/network inventories
were empty, and the disposable VM and auto-delete disk were removed. Raw proof
is indexed in the [2026-09-02 guest-I/O evidence](../../../evidence/runtime-20260902/g4-g6-io-live/README.md).

This closes the earlier implemented-but-not-live-revalidated terminal status
and the guest stdin/attach implementation gap. The live harness sets the host
terminal size before launching each task and does not change it after the guest
process is running. It therefore proves PTY creation and initial-size
propagation, not a post-start live resize; the latter is supported by local
agent tests but still needs a deliberate live rerun. It does not close
pause/resume, `Stats`, `Update`, `Checkpoint`, faithful guest PIDs, CNI, the
full G4 storage matrix, or shim-crash task reconnection.

A later local fault-injection review found that a failed guest
`CloseProcessStdin` call was incorrectly remembered as completed. The shim now
persists close-requested and close-acknowledged as separate states, retries a
pending acknowledgement after FIFO EOF or recovery until the process stops,
and makes repeated `CloseIO` calls idempotent only after acknowledgement.
Focused race-detector tests cover transient failure with and without a FIFO,
and the recovery-state test covers durable acknowledgement. This later change
still requires disposable-host revalidation; it is not part of the 2026-09-02
live claim above.

The final I/O transcript is also marker-oriented: it does not retain the
asserted stdin, attach, or `37 91` guest output. Its exact harness hash and exit
status support the scoped assertions, but the dirty source diff is absent. Its
manifest is not schema-valid because component versions are missing and
`${HOST_BOOT_ID}` is invalid, and `gate: G6` plus `result: pass` can be mistaken
for a full-gate result.

## 2026-09-02 audited verdict

| Gate | Supported learning | Gate-blocking remediation |
| --- | --- | --- |
| G4 | Containerd- and Docker-prepared BusyBox roots can be copied into private per-sandbox initramfs artifacts; private writes and narrow normal cleanup passed. | No verified root manifest, reproducible build, snapshot immutability proof, metadata/path validation matrix, architecture check, persistence model, ownership generations, quotas, corruption/recovery coverage, or complete partial-artifact rollback. |
| G5 | Two static `/30` TUN links, primary forwarding/NAT, outbound hostname HTTP, bidirectional sibling-link rejection, and narrow link/rule cleanup passed. | No `mknetd`, CNI `ADD`/`CHECK`/`DEL`, negotiated configuration, bounded queues/backpressure/counters, reconnect semantics, policy-bypass matrix, or primary-controller/health evidence. |
| G6 | The core shared `ctr`/Docker lifecycle, exec, stdio, nonzero exits, signals, name reuse, `mkruntimed` restart, stdin/attach, PTYs, initial-size propagation, and narrow cleanup passed. | No forced-shim task reconnect, Docker-daemon restart proof, event ordering/replay, faithful PIDs, cancellation/deadline matrix, broad FIFO/fault coverage, complete OCI support or fail-closed rejection, and no implementation of pause/resume, stats, update, or checkpoint. |

All three canonical gate rows remain open. A checked MVP feature is not a full
gate pass.

## Claim corrections from the remediation audit

- The child boot IDs are retained, but the child kernel release is not.
- The root builder appears read-only toward its source, but snapshot
  non-mutation was not measured.
- Sorted cpio input and `gzip -n` do not establish reproducible initramfs
  output or metadata fidelity.
- The older SIGKILL proof verifies Docker exit `137`; it records only a stopped
  state for the `ctr` task.
- Outbound hostname HTTP supports the narrow DNS/HTTP path, but no DNS answer,
  response details, UDP exchange, packet counters, or live rule state is
  retained.
- Containerd restart, forced-shim reclaim, and ENOSPC are narrative operator
  observations until rerun with raw output.
- “Full resource return” means only the inventories explicitly checked by the
  harness; it is not proof of every mount, artifact, process, FIFO, allocation,
  or recovery record.
- Unsupported OCI controls do not currently fail closed end to end. The
  initramfs builder rewrites the caller's OCI configuration to a supported
  subset, and exec translation similarly drops unsupported process fields
  before the agent can reject them.

## Implementation defects exposed by the audit

- Failed `Create` removes the rootfs mount but can leave the token, initramfs,
  runtime directory, and recovery artifacts. Event-publication failure after
  allocation also lacks a complete sandbox rollback.
- Failed `Exec` or exec-event publication can leave a stale process entry.
- Normal delete and crash cleanup ignore several agent, daemon, network-rule,
  unmount, and event-publication errors, so success can be reported without
  proving complete cleanup.
- Agent RPCs have no connection deadlines. Network teardown can wait on a
  blocked exchange, and context cancellation is not propagated through all
  lifecycle boundaries.
- Guest process output is retained in unbounded memory, and FIFO/network
  backpressure and slow-peer behavior are not bounded.

## Evidence remediation carried forward

- Validate every committed runtime evidence manifest, not only schema
  fixtures, from the normal documentation/evidence check.
- Retain expanded safe commands and observed values, exact dirty diffs,
  component versions, valid or explicitly redacted host identities, and
  structured cloud resource ledgers.
- Scope manifests to their actual G4, G5, or G6 assertions; a shared-matrix
  success must not look like a complete gate pass.
- Retain service journals and before/during/after storage, network, process,
  mount, allocation, container, and cloud-resource inventories for every
  restart and injected failure.
- Rerun terminal resize with a deliberate size change after the guest process
  is confirmed running and retain both sizes from inside the guest.
- Publish a final claim-to-assertion-to-file matrix before changing any gate or
  project task to closed.

The evidence audit, unresolved implementation work, and required replacement
GCE runs are tracked in the
[`G4-G6 remediation checklist`](g4-g6-remediation-checklist.md).

## Remediation implementation after the audit

Work begun on 2026-09-05 is locally verified but is not yet disposable-host
proof. The runtime now builds deterministic newc initramfs artifacts with a
separate canonical root manifest, detects source mutation, validates canonical
relative roots and allowlisted absolute roots, rejects escaping symlinks,
unsupported file types, and xattrs that it cannot faithfully reproduce, and
applies payload, inode, and host-free-space admission limits before sandbox
allocation. Focused tests include identical-build and changed-input controls,
archive extraction, hardlinks, symlinks, modes, unsafe roots, FIFOs, xattrs,
and capacity refusal.

The rootfs adapter no longer trusts those builder outputs merely because the
builder exited successfully. It uses bounded `O_NOFOLLOW` opens, verifies
owner/link/mode and stable file identity, checks the complete storage identity
contract, and verifies the image's declared size, allocation, and digest both
before publication and during recovery. Exclusive result creation prevents a
pre-planted file or symlink from being overwritten; focused adversarial tests
cover malformed metadata, hardlinks, symlinks, sparse images, wrong sizes, and
existing publication targets.

Builder cancellation previously targeted only the direct command, while
`CombinedOutput` retained arbitrary output before checking its size. Builder
execution now has a ten-minute default bound, inherits any earlier caller
deadline, and kills a dedicated process group so a descendant cannot retain
the output pipes. A draining writer retains at most one MiB and returned error
text has a smaller cap. Focused tests cover overflow, normal combined output,
and a background child killed at deadline; disposable-host fault proof remains
open. Rootfs service entry points additionally reject pre-cancelled work before
mutation, while mount entry and large image hashing observe the request context;
focused tests prove cancelled preparation, cleanup, and reconciliation preserve
backend calls, artifacts, and journal ownership.

Storage operations now follow the same rule. Provision, Release, and Reconcile
reject cancellation before changing durable ownership; inspection, start,
observation, stop, and offline-check entry points reject it before external
mutation. The image digest loop polls between four-MiB reads. Focused tests
prove a cancelled release remains `ACTIVE`, invokes no backend stop, and that
mid-inspection cancellation interrupts hashing rather than scanning the rest
of the image.

The rootfs, storage-export, and network-endpoint ownership stores no longer
reopen validated state directories and publish through their pathnames. A
shared primitive walks from the filesystem root with no-follow `openat`,
creates missing components relative to already opened parents, binds the final
directory device/inode, performs private bounded state reads relative to that
descriptor, and publishes synchronized replacements with `openat`/`renameat`.
Each store rejects a post-open directory substitution before mutation, and
focused tests also cover symlinked ancestry, unsafe files, and publication to
an intentionally renamed but still-open directory inode.

The shim now removes partial Create artifacts and allocated sandboxes on later
failure, gives agent RPCs bounded deadlines with context cancellation, removes
failed Exec entries, and reports a versioned guest-PID mapping. Guest
process-group statistics, pause, and resume are implemented through the Task
v2 API. A versioned atomic recovery record captures sandbox/network ownership,
process/FIFO/terminal state, guest PIDs, exits, and output offsets. A supervised
worker and reconstruction code path have been added for signaled shim-worker
death; that path remains unproved until focused fault tests and the raw GCE
restart transcripts required by the checklist pass.

`make docs-check` now validates every committed runtime manifest and every
referenced evidence path. All 17 historical runtime manifests have immutable
hash-pinned nonconformance classifications in
`evidence/runtime-manifest-exceptions.json`; this prevents their earlier
schema, dirty-source, narrative-only, or cleanup-ledger deficiencies from
being mistaken for final gate evidence.

The same remediation branch now contains a locally race-tested G5 replacement
for the static shim-owned link. `mknetd` durably allocates collision-free
primary endpoints and owns their routes, NAT, and per-generation firewall
chains. The CNI 1.0.0 adapter caches endpoint generations for stale-safe
`ADD`/`CHECK`/`DEL`; external CNI endpoints and runtime-owned standalone
namespaces share the same generation-bound `mknetd` contract. The unprivileged
shim receives an already-open TUN descriptor and requests a fresh descriptor
after worker reconstruction without performing namespace operations. The
descriptor receiver now parses and marks all received rights close-on-exec
before payload validation, then closes them on every rejection path. A real
Unix-socket/pipe test sends a descriptor beside a truncated reply and proves
the rejected duplicate leaves no hidden pipe reader when the host permits
`net.FileConn`; it is skipped by the current local sandbox and therefore awaits
disposable-host execution. The server also requires the complete response and
ancillary payload to be sent together; its synthetic short/error matrix passes.
packet path is negotiated-MTU bounded and
single-flight, detects a 250 ms stalled exchange, reconnects without resetting
the authenticated sequence, and reports monotonic packet/drop/error counters.
DNS configuration and regular-file/symlink/absent restoration are tested.
Guest configuration is now replay-safe: the agent retains the complete
successful configuration and accepts only an exact repeat, while the shim
reconnects after a lost response. Different configuration is rejected without
ownership mutation, and cleanup retains that replay identity through failure
then clears it after a successful retry. Agent and shim fault matrices pass 100
race-detector repetitions.
Linux command-order tests inject failure at every partial-`ADD` boundary and
assert reverse cleanup, source-spoof, sibling, metadata, and default-drop
rules. The anti-spoof and NAT match is now the exact child `/32`, rather than
the whole allocated `/30`, and `CHECK` verifies the complete source, metadata,
sibling, egress, return-flow, default-drop, and NAT rule set. These are
implementation observations from the local test suite, not
G5 gate evidence: privileged namespace traffic, policy bypass, restart/load,
and final cleanup still require the disposable-instance matrix and raw bundle.
Privileged primary and guest `ip`, `iptables`, `nsenter`, sysctl, and
egress-preflight execution previously relied on unbounded `CombinedOutput`.
It now shares the bounded process-group runner, observes primary caller or
guest server cancellation, and has a 30-second default. It retains no more
than one MiB of combined output and limits returned diagnostics to 16 KiB.
Guest setup rollback has its own
five-second cleanup context; link and DNS cleanup identities survive failure,
while successful DNS restoration makes repeated close a no-op. Focused tests
cover a blocked descendant, overflow, combined stdout/stderr, pre-mutation
cancellation, repeated close, and failed DNS restoration; disposable-host
timeout and leak evidence remains open.

The G4 backend now produces a fully allocated ext4 image with a deterministic
positive e2fsprogs fake epoch; zero was found by execution to mean “wall clock”
on the supported toolchain. A later repeated-build check exposed that
`mke2fs -d` still copied source ctime and could observe changing atime. The
builder now imports an allocation-accounted private staging clone, normalizes
its atime/mtime and the completed image's imported inode ctime, and performs
offline validation before publication. Repeated local builds with an explicit
source-atime change are byte-identical while a content change produces a new
digest. The Go consumer enforces that exact metadata contract. Root scanning
revalidates every admitted path after content reads so
later membership or inode-identity mutation fails the build. Storage restart
also retains `PREPARING` ownership when export start has an ambiguous outcome;
an exact retry or reconciliation stops any matching partial server,
re-inspects the image, and restarts the same generation before publishing it
as active. A conflicting live generation remains untouched and fails closed.
Durable storage state is now semantically validated before recovery, including
forward-only transitions and uniqueness of every live owner, path, port, and
filesystem UUID; no-follow, stable-inode, ownership, mode, and link checks
protect the state file itself.
The backend applies those file checks to process records and logs as well,
binds readiness and graceful-close evidence to the exact export tuple, and
accepts counters only from the terminal canonical close line. Managed stop
checks for an already-reaped child before revalidating PID/start-time/argv,
then signals immediately and waits within its configured bound.
The backend now also holds the private runtime-directory descriptor and binds
its device and inode for its lifetime. Process-record publication is exclusive,
and record/log inspection, readiness reads, cleanup, restart observation, and
stop all operate relative to that descriptor. Start refuses an unsafe stale
log or an existing record without replacing prior evidence, and failures before
ownership publication reap the child and remove only the log created for that
attempt. Focused race-detector tests replace the entire runtime directory,
inject a symlink log and record collision, and fail command startup; the
replacement and prior artifacts remain untouched and failed startup leaves no
owned residue.
Rootfs recovery applies the same untrusted-state rule and additionally proves
that every recursive-cleanup target is derived from the canonical bundle or
configured storage root. Forged paths, phase regressions, duplicate bundle or
port claims, and caller alias mutation fail before cleanup. Recursive removal
is descriptor-anchored, with tests proving root symlinks are rejected and a
post-open rename cannot redirect deletion into the replacement tree.
Restart reconciliation can complete an exact `QUIESCING` lease only after
observing a still-running exact export or its generation-specific
graceful-close counter record, followed by an offline check. An unexplained
missing server remains durably diagnosable. These behaviors pass focused local
tests but are not G4 live evidence until the replacement-instance
storage/fault matrix retains the observed values.

Offline `e2fsck` formerly used unbounded `Cmd.Output` and had no default
deadline. It now shares the rootfs process-group runner, retains at most one
MiB of combined stdout/stderr for a successful evidence digest, defaults to a
five-minute bound, and kills descendants on timeout or cancellation. Overflow
and non-clean exits fail without disclosing checker output. Focused tests cover
each boundary; live dirty/corrupt-image recovery evidence is still required.

The mkruntimed lifecycle snapshot and mutation journal previously remained an
exception to the durable-file rules: snapshot load ignored every error except
a successfully read malformed JSON document, journal load errors were ignored,
and creation, append, reads, and replacement all reopened pathnames without
size or identity bounds. They now use the descriptor-anchored state directory,
strict bounded snapshot and journal reads, an exclusively created and
directory-synchronized append file, a 64-MiB journal ceiling, one-MiB entry
ceiling, contiguous sequence validation, and explicit poisoning after an
ambiguous write or sync failure. Directory and journal pathname replacement,
unsafe snapshot files, truncated/duplicate/unknown journal input, sequence
gaps, and capacity exhaustion have focused tests. Failed snapshot persistence
also rolls back the affected in-memory sandbox/result mutation.

The G6 shim now journals typed Task events before publication and replays them
in local sequence after worker reconstruction. Exit completion persists an
`exit_event_queued` invariant, and Delete queues a missing exit before its own
event, closing the earlier wait/delete ordering window. Publication failure is
therefore durable and lifecycle mutation is not falsely rolled back. The
documented contract is at-least-once because containerd's event-forwarding API
cannot atomically combine remote acceptance with the shim's local
acknowledgement; a crash in that interval can replay a duplicate. Local tests
cover ordered replay, pre-publication journal failure, acknowledgement failure,
unsafe journal files, and retry after a transient disconnect without another
lifecycle request. Restart/event transcripts are still required before
the broad checklist row can close.

Stdio path strings are another restart trust boundary, not harmless containerd
metadata. The shim now validates them before Create or Exec mutation and uses
Linux `openat2` for every start/reconstruction descriptor, rejecting symlinked
ancestors and magic links. Stable device/inode, mode, ownership, link-count,
and ctime comparisons detect replacement and same-inode metadata races; stdin
is restricted to a private FIFO. Focused tests exercise each rejection and
request cancellation. Recovery format version 2 now durably records each
configured stream's immutable device, inode, owner, group, mode, and link count
at Create or Exec. Start and reconstruction must reopen that exact object, so
a private same-type replacement made before either boundary is preserved and
rejected. Version-1 records are rejected because their original stdio identity
cannot be reconstructed safely from a pathname. Twenty race-detector
repetitions cover FIFO and regular-output substitution. A real nonblocking FIFO
test additionally leaves stdin without an initial writer, attaches and detaches
two writers in sequence, observes both exact byte strings at the guest-call
boundary, and proves descriptor teardown terminates the pump. Live
attach/restart revalidation remains open.

Process output polling formerly converted the first agent-call error directly
into a synthetic exit status 255, even when the child remained live and only
the relay transport had disconnected. Output and final Wait reads now retry
idempotently through the generation-bound relay within one 30-second overall
budget, retaining the last acknowledged stdout/stderr offsets. Agent replies
retain their structured error type, so an authenticated operation rejection is
returned once rather than reconnected and replayed. Twenty race-detector
repetitions cover transient output and Wait recovery, unchanged offsets,
initial reconnect failure, terminal remote rejection, and deadline exhaustion;
the cross-process live disconnect case remains open.
The background monitor no longer converts even a fully exhausted reconnect
budget into exit 255. It retains the running state, waits 100 milliseconds,
and starts another bounded epoch until an authenticated `WaitProcess` supplies
the exact exit. A sustained-outage/recovery test proves one expired epoch does
not close the task wait channel and the later observed exit 37 is published;
100 race-detector repetitions pass.
Output replies are now validated before destination I/O or offset mutation:
both streams must fit the requested 4096-byte chunk, advance contiguously
without overflow, and report `RUNNING` or `STOPPED`. Focused gap, regression,
overflow, oversize, and unknown-state cases pass 100 race-detector repetitions.
Output acknowledgement persistence is no longer ignored. Failure atomically
restores both prior stream offsets and re-delivers the same bounded chunk,
making the failure contract explicitly at-least-once. An injected missing-state
directory produces requests at offsets 0, 0, then 4 after repair and observes
the exact later exit 11; 100 race-detector repetitions pass. Empty unchanged
polls no longer rewrite the recovery file.
An authenticated guest exit is likewise not exposed through Task Wait until
its stopped state, exact code, and timestamp are durable. Injected missing
recovery storage retains the prior running state and open wait channel; after
repair, the on-disk process is `STOPPED` with exit 37 before completion.
One hundred race-detector repetitions pass.
The final exit-event acknowledgement write is no longer ignored either. If
recovery disappears after durable event publication, memory restores
`exitEventQueued=false` to match the stopped disk record, leaving the existing
Delete/reconstruction repair path authoritative. The injected boundary retains
exact exit 37 and passes 100 race-detector repetitions.
Exec creation no longer discards cleanup errors after a recovery or event
publication failure. Its five-second cancellation-independent rollback treats
authenticated `NOT_FOUND` as confirmed absence, removes ownership only after
that result or successful deletion, republishes the exact resulting registry,
and returns both guest and recovery errors. Success, already-absent, and failed
deletion cases keep disk and memory aligned for 100 race-detector repetitions.
Exec creation now persists its CREATED owner before contacting the guest. A
lost `ExecProcess` reply followed by exact CREATED state succeeds and publishes
the exec-added event; confirmed absence rolls the intent back, while unavailable
state plus failed deletion retains the owner identically in memory and on disk.
Reconstruction independently accepts only exact CREATED ownership, drops
authenticated absence, rejects wrong or RUNNING state, and replays the
exec-added event at least once. Both matrices pass 100 race-detector
repetitions.
Normal Task Delete now treats authenticated guest `NOT_FOUND` as confirmed
idempotent absence instead of trapping a successfully deleted process behind a
lost reply. An injected absence returns the exact PID and exit status, publishes
only the delete event, and removes local ownership; the paired arbitrary-error
case retains retry ownership. An injected transport loss after deletion now
reconnects through the owned relay and requires authenticated `NOT_FOUND` on
the retry. The three boundaries pass 100 race-detector repetitions.
Pause/resume event-failure rollback no longer suppresses a failed recovery
rewrite. Injected directory replacement after reverse signaling proves memory
returns to its prior state while the old durable transition remains
diagnosable; both the event and labeled recovery failure are returned for
Pause and Resume across 100 race-detector repetitions.
Failure of the initial transition recovery write now takes the same
compensating path. A directory is removed for the attempted write and restored
for reverse signaling; the shim republishes the original state as a new inode
before returning the transition error. Pause and Resume pass 100
race-detector repetitions.
Post-start cleanup no longer discards `SIGKILL` failures. Invalid PID,
recovery, and start-event failure paths retain RUNNING ownership and monitors,
use a cancellation-independent five-second signal call, and return its error;
authenticated `NOT_FOUND` is successful cleanup. Injected persistence plus
signal failure retains guest PID 41 and later records exact exit 9 after
storage repair across 100 race-detector repetitions.
Arbitrary signals now use the advertised `signal-operation-id-v1` contract.
The generation-scoped guest ledger retains up to 4,096 exact
target/signal/results, returns exact replay even after process exit, rejects
cross-target or changed-signal reuse, and refuses before mutation instead of
evicting uncertainty when full. Task Kill durably moves through intent and
result-observed phases, reconnects/replays after reply loss, retires the guest
entry with idempotent `AcknowledgeSignal`, then persists local retirement.
Reconstruction resumes the correct phase. Cleanup and pause/resume use the same
operation identity and acknowledgement. Focused guest, protocol, Kill,
reconstruction, and transition matrices pass 100 race-detector repetitions;
the Kill fault proves two calls, one reconnect, and one applied signal.
PID validation now also rejects positive agent values above Task v2's `uint32`
range in both Start and reconstruction. An oversized PID remains an unverified
RUNNING owner without a fabricated PID, publishes no start event, is durably
recorded, and stays monitored through cleanup; 100 race-detector repetitions
pass.
Init Start no longer blindly repeats `CreateProcess` after a partial prior
attempt. It probes authenticated state, creates only on `NOT_FOUND`, and reuses
only exact `init`/`CREATED`/zero-PID/zero-exit state. Injected absence, existing
creation, wrong identity, live state, fabricated PID, and transport failure
pass 100 race-detector repetitions without a duplicate mutation.
The create call itself now has the matching ambiguity boundary. A lost
successful `CreateProcess` reply is not replayed; the shim reconnects and
accepts only exact `init`/`CREATED` state under an independent five-second
observation. The injected broken-transport case performs one create and one
reconnect across 100 race-detector repetitions. An authenticated create
rejection remains terminal.
An errored `StartProcess` call now reconciles with an independent five-second
state read that reconnects after transport loss. Exact `CREATED` preserves
retryable local state; exact `RUNNING` and rapid `STOPPED` prove the start
applied and return success; unresolved transport failure retains unverified
ownership, monitoring, durable state, and bounded cleanup. The five-boundary
matrix, including one lost state reply and one reconnect, passes 100
race-detector repetitions.
Start and reconstruction now also validate the agent process ID and state, not
only its PID. Wrong identity and `CREATED` observations after a successful
start retain unverified RUNNING ownership, publish no start event, and remain
monitored through cleanup across 100 race-detector repetitions; a valid rapid
`STOPPED` observation is accepted as an applied start. Reconstruction admits
only exact `RUNNING` or validated completion state.
Stopped responses now validate the requested agent ID, exact `STOPPED` state,
established PID, and the agent's `0..255` exit-status contract before durable
completion. Wrong ID, state, PID, negative exit, and exit 256 are each retried
without state or event mutation before an exact reply completes; 100
race-detector repetitions pass. Reconstruction uses the same validator and can
learn a valid PID for a previously unverified started owner.
The agent server formerly left one goroutine waiting on the server-wide context
after every peer disconnect. Connection cancellation now uses a callback that
is unregistered and joined on session return. Focused tests prove cancellation
still unblocks an active read and later cancellation does not touch a completed
session; 100 race-detector repetitions pass.

Stdin previously had the complementary unsafe choice: the pump consumed FIFO
bytes and issued an unversioned write, so retrying a lost response could
duplicate input while declining to retry silently lost the live attachment.
The agent now advertises `stdin-offset-v1`. It accepts only the exact next byte
offset and retains the last offset, length, and SHA-256 so an identical
lost-response replay is acknowledged without a second write, including after
the process exits. When a local writer accepts a prefix before returning an
error, the agent retains that position and an exact replay resumes with only
the unaccepted suffix. Recovery v2 records the shim's acknowledged offset and a
bounded pending chunk before guest contact; acknowledgement clears it only in
a second durable update. Focused tests prove ordered writes, changed/gapped
rejection, wire-level deduplication, partial-write continuation, reconnect
replay, intent-before-mutation,
acknowledgement-failure rollback, recovery bounds, and compatibility for an
older offset-free controller. Twenty race-detector repetitions pass; live
FIFO disconnect/restart evidence remains open.
The real FIFO regression also keeps one writer attached across an empty
interval. The nonblocking reader now retries `EAGAIN` instead of terminating,
so later bytes from the same attachment and a subsequent attachment both reach
the guest; 100 race-detector repetitions pass.
Conversely, a continuously readable peer can no longer starve `CloseIO`: the
pump completes its one in-flight read, acknowledges the durable close, and
returns before issuing another read. A controlled-reader cutoff test passes
100 race-detector repetitions.
`CloseProcessStdin` acknowledgement now uses the same bounded relay reconnect
transaction while retaining the Task caller's earlier deadline. Its existing
requested-versus-acknowledged durable state makes replay idempotent; a focused
test injects a transport loss, reconnects, observes one successful retry, and
records the acknowledgement.

The shim-to-`mkruntimed` client previously used the request context only for
Unix-socket dialing; a daemon that accepted and then stopped reading or
replying could hold the operation forever. The client now applies the earlier
of the caller deadline and a 30-second default to the whole exchange, and a
context callback wakes blocked socket I/O. Focused tests cover blocked writes,
blocked reads, pre-dial cancellation, the default bound, and normal response
decoding. Shared protocol writes also complete across injected short-success
transport wrappers and reject zero progress; focused agent framing covers both
directions, the daemon client covers request writes, and mknetd covers request
and response writes. Replies are strict-decoded and bound to the originating
protocol version and request ID. Typed bodies are now strict-decoded directly rather
than through a permissive generic-map round trip, and complete response input
uses a one-byte overflow sentinel so a valid one-MiB prefix cannot hide trailing
data. The mknetd request client uses the same overflow rule; its descriptor
ATTACH path also requires an untruncated newline-terminated response. Other
blocking boundaries and live leak checks remain to be audited.
The daemon and mknetd servers also formerly closed only their listeners on
service cancellation; accepted peers stalled on an incomplete request could
outlive the service. Both now derive a handler context from the accept loop and
use the joined close callback for the listener and each connection. Focused
blocked-peer tests and the shared active/completed callback tests pass 100
race-detector repetitions.
Both concurrent accept loops now use a bounded handler semaphore (128 by
default and no more than 1,024), close excess authenticated connections, and
join admitted handlers on every return. In-memory listener tests exercise the
exact production serve loops under saturation and cancellation for 100
race-detector repetitions. Guarded pathname-listener execution remains part of
the disposable-host matrix because the local sandbox forbids its socket option.
Daemon response generation now enforces the client's one-MiB limit and converts
oversized or unencodable bodies to a bounded INTERNAL error retaining the
request ID. Agent reply decoding now requires exactly one body or error and
strict-decodes the caller's typed body. Unknown/duplicate fields, omitted body,
and body-plus-error cases fail. Both adversarial groups pass 100 race-detector
repetitions.

Kerf lifecycle execution already had a nominal timeout, but retained unbounded
`CombinedOutput`; verbose load output could also echo the guest authentication
token embedded in the kernel command line. It now uses the shared bounded
process-group runner with caller cancellation, a 30-second default, and a
one-MiB retention ceiling. Failures report only retained byte count and SHA-256.
Caller cancellation is checked before execution and observation, so an
already-matching sysfs state cannot convert cancellation into success. Focused
tests prove timeout kills a background descendant, overflow remains secret-safe,
and canceled observation never executes or succeeds; live child-boot
cancellation and leak evidence remains open.

Host qualification also used separately bounded `CommandContext` calls whose
`CombinedOutput` retention was unlimited, while its guest-agent `systemctl`
probe had neither a deadline nor descendant cleanup. All external read-only
probes now share the process-group runner with caller cancellation, a
five-second default, and a 64-KiB combined-output ceiling. Overflow or timeout
leaves a critical qualification signal unknown or unavailable, so it fails
closed. Focused tests cover descendant timeout, overflow, and mixed output;
replacement-host report capture remains required.

The direct-agent evidence controller previously performed framed writes and
reads without deadlines, so an accepted but silent relay could stall the live
matrix. Its complete exchanges, including malformed and authentication probes,
now have a 30-second deadline. Controller-owned reconnect relays run in
dedicated process groups and termination kills and reaps the group rather than
only the leader. Focused tests cover blocked writes, blocked reads, bounded
termination, and descendant cleanup; current-revision VM revalidation remains
open.

Shim-owned agent relays had no symmetric lifecycle: they were started without
a process group, several connection-failure paths left them running, and normal
task deletion never reaped them after guest shutdown. Relay ownership is now
explicit. Starts create a dedicated process group; failed connection or
reconstruction, normal Delete, and fallback Cleanup kill and reap it. Normal
Delete now closes guest networking, retries an idempotent authenticated
`Quiesce` across transport reconnect, and only then attempts terminal
`Shutdown`. Quiescence performs storage shutdown once and seals later agent
mutations. Injected loss of the first quiescence reply reconnects and succeeds;
an injected lost `CloseNetwork` reply also reconnects and retries exactly.
Loss of the terminal reply completes safely, while an invalid quiescence body
or authenticated shutdown rejection fails closed. Cancellation after the
quiescence acknowledgement also returns cancellation without attempting
terminal shutdown. The paired agent and shim matrices pass 100 race-detector
repetitions. Relay-socket removal remains retryable; live process inventories
remain required.

Fallback shim Cleanup previously followed and loosely decoded `sandbox.json`,
accepted incomplete ownership, and ignored every network, relay, daemon, and
rootfs error before returning success. Reconstruction and Cleanup now share a
strict bounded `openat2` loader that checks caller ownership, link count, mode,
stable identity, exact task/generation/storage/network ownership, and bounded
unique process state. Invalid state fails before mutation. Cleanup aggregates
teardown failures, will not delete after a failed stop, and reaches rootfs
cleanup only after confirmed sandbox deletion. Focused adversarial state and
ordered failure tests pass; forced-shim live cleanup evidence remains open.

Reconstruction also previously loaded its authentication token through a plain
path read even though normal Create performed stronger checks. Both paths now
use one bounded `openat2` loader that rejects symlinked ancestors, hardlinks,
wrong ownership or mode, malformed length or encoding, and identity changes
while the token is opened or read. Focused adversarial tests cover the static
path and link failures. Exclusive creation of an initially absent token now
uses the same validated parent descriptor, and failure cleanup removes only
the inode created by that attempt; an empty symlinked-parent test proves it
cannot redirect creation. Current-revision reconstruction evidence remains
open.

Rootfs mount validation no longer has a weaker shim-side precheck followed by
the complete policy only after sandbox allocation. The shim and rootfs service
now share the complete bounded mount validator, so nil entries, malformed or
duplicate options, unsafe sources, escaping paths, and unsupported mount
semantics fail before the first allocation mutation.

OCI process validation is now independently bounded on both sides of the
primary/guest boundary. Argument and environment counts and combined bytes,
embedded NULs, duplicate environment names, non-canonical working directories,
and duplicate or excessive supplementary groups are rejected before primary
allocation or guest process-record mutation rather than being deferred to
`execve`.

OCI configuration loading now treats the file identity as untrusted at both
the primary adapter and guest agent. Descriptor-anchored no-follow reads bind
owner, mode, link count, size, inode and timestamps, cap the complete document
at one MiB, and reject mutation while reading. In particular, a valid JSON
prefix followed by data beyond the former decoder limit can no longer be
accepted as the complete configuration.

Privileged deployment configuration is no longer a sequence of unrelated
manual copies. A dedicated manager builds immutable, hash-verified generations
containing the host configuration, environment, systemd units, CNI config and
containerd import fragment, together with the complete rootfs builder/helper
and guest-init dependency set referenced by the service. Stable destinations resolve through one atomically
selected generation, allowing exact rollback while collision and uninstall
rules preserve operator-owned files. Service activation remains an explicit
post-qualification action and is reported by `inspect`.

Task v2 request identity is now enforced uniformly rather than only during
Create. Every supported RPC rejects nil input and a task ID that differs from
the per-shim task before taking its mutation lock or contacting the guest; the
focused method table also proves these failures leave process and shutdown
state unchanged.

Wait no longer changes from cancellable locking to an unconditional mutex wait
after observing process exit. Its final exit snapshot uses the same
cancellation-aware lock acquisition, so concurrent teardown cannot indefinitely
retain a caller whose deadline has expired.

Storage high-water accounting now validates its free-space reserve before any
directory or image creation; negative values can no longer reduce the required
capacity through arithmetic. A focused ext4 build deliberately exceeds a
128-inode image with 256 source entries and proves the formatter failure leaves
no image, metadata, or staging directory. A separate fully allocated 63-MiB
payload into the minimum 64-MiB filesystem exercises block exhaustion and
proves the identical cleanup invariant.

The shim's network-namespace projection used to re-open OCI `config.json` with
an unbounded path read even though the primary adapter and guest used stronger
loaders. It now enforces the same no-symlink, caller-owned, single-link,
one-MiB, stable-identity contract before decoding the namespace needed by
`mknetd`.

The shim packet pump now has direct socketpair-backed fault coverage rather
than relying only on service/store tests. Exact-MTU packets traverse both
directions, oversized ingress and egress increment their specific drop
counters, an exchange disconnect accounts the in-flight loss, and two injected
reconnect failures are retried before subsequent traffic is accepted.
Network report failures are no longer discarded. A joined worker retains only
the latest state, retries each two-second-bounded `mknetd` call after 100
milliseconds, and is cancelled with the packet pump. An injected
`DISCONNECTED` failure superseded by `READY` retries twice to success with the
current counters, then accepts a later `DEGRADED` report; 100 race-detector
repetitions pass.
The shim now reuses mknetd's durable endpoint validator at each network RPC
boundary. PROVISION cannot retain a mismatched workload, ATTACH requires exact
namespace/generation/sandbox ownership and closes an invalid received
descriptor, and REPORT/RELEASE reject malformed generations before request-ID
slicing or RPC contact. These boundaries pass 100 race-detector repetitions.
Guest network close was the remaining unbounded teardown RPC. It now has an
independent five-second default; an injected blocked close returns its deadline
while local TUN closure and subsequent teardown continue. The focused timeout
case passes 100 race-detector repetitions.

The CNI binary no longer truncates stdin at one MiB and then attempts to parse
the valid prefix; oversize is explicit failure. Cache directory creation now
walks and creates components relative to no-follow parent descriptors, and
private bounded reads, `RENAME_NOREPLACE` publication, and deletion remain
relative to the verified final descriptor. Exact replay is idempotent, a
different generation cannot replace the durable ownership record, and DEL
keeps the descriptor across its daemon request so it refuses an in-flight
generation change without removing the substituted record.
CHECK also validates mknetd's complete returned endpoint identity and no longer
accepts an empty or mismatched successful response.

The mknetd, mkruntimed, and mk-agent listeners formerly inspected and removed stale Unix
socket paths, then bound and chmodded by pathname; mkruntimed also ignored a
stale removal failure, and mknetd's outer cleanup could unlink a replacement.
A shared listener guard now opens the parent without following symlinks,
requires caller ownership and safe parent mode, removes only a caller-owned
single-link stale socket with the exact service mode, binds through the held
directory descriptor, and captures the published inode. Cleanup removes only
that inode. Guest init places the agent control socket beneath a private
`/run/multikernel-agent` directory instead of shared `/tmp`. Real
pathname-socket tests also exposed that Go listeners unlink
their configured path automatically on Close, so `SetUnlinkOnClose(false)` is
mandatory for the identity guard to be meaningful. Focused unsandboxed tests
exercise connection, stale replacement, normal cleanup, and preservation of a
hostile post-bind replacement; GCE restart evidence remains open.

Fresh connection and reconstruction formerly unlinked the derived relay path
without checking either its type or the removal result. They now remove only a
caller-owned, single-link Unix socket with non-writable group/other mode and
fail closed on every other path. Focused tests prove regular files,
directories, and symlinks are rejected without removal; the privileged live
socket replacement case remains part of the replacement run. The relay helper
itself no longer unlinks before bind or after accept; the supervising shim is
the sole component permitted to remove that pathname. Before agent dialing,
the shim captures the helper-published socket inode relative to a held,
no-symlink parent descriptor; normal teardown removes only that exact inode
and preserves cleanup ownership for retry if the pathname was replaced.

The reconstruction path now has a focused successful-reconnect test rather
than only state-validation and fallback-cleanup coverage. Injected daemon,
network, agent, and relay boundaries prove that the exact sandbox and network
generations restore a recorded running guest PID, authenticated agent
parameters, output/wait loop, and its later exact exit completion before owned
resources are reaped. The same reconstruction test now injects loss of its
first `StateProcess` reply, requires one relay reconnect, and restores the exact
PID across 100 race-detector repetitions. Reconstruction's idempotent state
and stopped-state wait reads share that bounded reconnect path. Live
forced-death continuity for a running process is still required.

That focused path also exposed a descriptor leak before agent reconnect:
reconstruction acquired the generation-bound TUN descriptor before installing
its failure-cleanup defer. Cleanup ownership now begins at acquisition, and a
forced relay-start failure proves the descriptor is closed and relay command
and socket state are cleared.

Task cancellation coverage now includes the read side as well as mutation.
State, Wait, Pids, Connect, and Stats reject an already-cancelled caller before
locking or guest contact; focused coverage proves no guest request is emitted
and the process completion record remains unchanged. Task entry locks now use
cancellation-aware acquisition too; contended read and mutation tests return
while the competing owner still holds the lock. Non-cancellable locks remain
only where post-mutation cleanup must finish to preserve ownership. The live
cross-service deadline and leak matrix remains open. Event queue and flush
locks use the same cancellation-aware acquisition, with contention tests
proving cancellation does not mutate pending events or sequence state.

Task Stats formerly treated a lost guest reply as terminal despite being an
idempotent observation. Each `StatsProcess` read now uses the shared bounded
reconnect path before it contributes to the task aggregate. An injected first
reply loss reconnects once and returns the exact CPU, RSS, and PID metrics in
100 race-detector repetitions; authenticated rejection remains non-replayed.

Event-journal recovery now applies the same untrusted-file contract as shim
recovery state and tokens: caller ownership, one link, private mode, bounded
size, no symlinked ancestors, and stable device/inode/metadata through the
read. Focused hardlink and symlinked-ancestor tests supplement the existing
schema, symlink, and mode cases; live event replay evidence remains open.

Recovery and event-journal writes no longer reopen their parent by pathname
during atomic publication. The shared writer validates and opens a canonical,
caller-owned, non-world-writable parent through `openat2`, creates the private
temporary file relative to that descriptor, and publishes with same-directory
`renameat` plus directory sync. Focused replacement, symlinked-parent, and
unsafe-parent-mode tests prove state cannot be redirected. Event-journal
ownership now also tracks the exact inode loaded or created by the shim. Every
subsequent rewrite verifies that inode before replacement, and final
acknowledgement unlinks relative to the held parent only when the identity still
matches. A substituted journal is preserved and the published event is put
back at the front of the in-memory queue for at-least-once replay.

Terminal size is another reply-loss boundary because the shim persists the
requested dimensions before applying them in the guest. `ResizeProcess` is an
exact set operation, so live resize and reconstruction now reconnect and replay
the identical process ID, width, and height within the shared bounded RPC
budget. An injected first-reply loss reaches the requested dimensions after one
reconnect in 100 race-detector repetitions; authenticated rejection remains
terminal and restores the prior durable intent. Disposable-host validation of
this current revision remains pending the approved source transfer.

Rootfs recovery records now bind both the original containerd bundle and the
configured storage root to their device, inode, and owner. A same-owner
whole-directory substitution made before replay or cleanup is rejected before
backend verification or unmount, and cleanup remains descriptor-anchored after
opening the recorded roots. Focused bundle and storage-root replacement tests
prove the substitute remains untouched and the durable cleanup record remains
available for retry. The identity-bearing store is now explicitly version 4,
including the pre-mount rootfs and exclusive runtime/per-task storage
device/inode/owner tuples. Empty version-1 through version-3 stores upgrade
atomically; nonempty legacy ownership is rejected because its original
directory identities cannot be reconstructed safely from pathnames.

Read-only OCI bind inputs now have an implemented v1 subset instead of a
blanket mount rejection. The adapter accepts only bounded, non-overlapping
directory or private single-link regular-file inputs using standard read-only
`bind`/`rbind` forms (including
Docker's `rbind,rprivate,ro`), normalizes them to guest
`bind,ro,nodev,nosuid,noexec` semantics, and emits
separate host and sanitized guest projections. A primary-side helper manifests
each source before and after copying it into the private ext4 staging root,
requires the copied manifest to match, and publishes a normalized manifest
whose digest and summary are independently checked by the rootfs daemon during
build and recovery. Host paths never enter the child. The agent advertises the
subset, validates the sanitized destinations again, then self-binds and
remounts them read-only before the first process. Existing type-matched real
directory or regular-file destinations are replaced only in the private staging copy,
reproducing normal bind hiding without mutating the caller snapshot. Local fault tests cover
metadata/link preservation, destination replacement/type rejection, symlinked
sources, injected source mutation, malformed provenance, and unsanitized guest configurations.
Privileged guest write rejection, writable volumes, configured persistence,
and current-revision disposable-host evidence remain open.

The shim previously relied on replay-safe stdin, replay-safe signals,
two-phase shutdown, and the complete projected OCI policy without querying the
connected agent. Initial connection and reconstruction now require an
authenticated bounded `Capabilities` exchange before guest network configuration or
process observation. Protocol v1, runtime identity objects, unique bounded
feature sets, and every protocol/OCI capability used by this shim revision are
mandatory. Transport loss can use the existing bounded reconnect path, while
an authenticated incomplete or malformed peer fails terminally. Focused
wrong-version, missing-feature, duplicate, missing-identity, success, and full
reconstruction cases pass 100 race-detector repetitions.

## Continuation checkpoint: 2026-09-17

The remediation work above is committed through `0bcbe0f`; it is not merely an
uncommitted working-tree experiment. Before this checkpoint, the full Go race
suite, `go vet ./...`, documentation checks, the focused read-only bind tests,
and the focused capability/reconstruction tests passed locally. Those results
substantiate local implementation claims only and do not replace privileged
disposable-host evidence.

The retained replacement-instance name is
`mklinux-g4-g6-final-20260905` in `asia-southeast1-b`. It was observed stopped
and restarted on 2026-09-17; GCE subsequently reported `RUNNING` with start
timestamp `2026-09-17T05:31:55.506-07:00`. No source was uploaded and no new
evidence artifact was retained during that observation. Qualification,
deployment, and the current-revision matrices remain pending explicit approval
to upload the source-only archive and retain the described infrastructure
evidence.

Explicit stopped-task deletion returns `BACKEND_FAILURE: backend operation
failed` before container deletion or pool return. This is a substantive
teardown/recovery outcome, not a harness error. The retained state must be
inspected for canonical close versus active helper before choosing the
already-qualified daemon-reconcile/delete retry; no destructive state-file
edit is permitted.

Read-only diagnosis shows the failed delete durably advanced the exact export
to `QUIESCING` with zero counters/no offline check, while its process record
names PID 6341 but no `mkvsock-nbd` process exists. The exact log is only
generation-bound READY→CLIENT_ACCEPTED and has no canonical close. The stopped
task and child instance remain. This is correctly unrecoverable by ordinary
release and must fail closed across daemon/host restart; it is not the intended
ACTIVE-state fault case and does not close that requirement.

On 2026-09-20, the operator explicitly authorized transferring the current
source to the disposable VM and running the live qualification suite. This
removes the source-transfer and execution pause. Because the authorization did
not reproduce the separately documented exact evidence-retention sentence,
permanent raw-evidence publication and live-closure claims remain conditional
on that distinct approval. Findings from the run are to be added to both
learning records immediately as they are observed.

The first GCE control-plane check found
`mklinux-g4-g6-final-20260905` already `RUNNING`, so neither restart nor
recreation was required. GCE reported the expected `n2-standard-16` shape,
disposable/purpose labels, auto-delete 100-GB boot disk, internal address
`10.148.0.56`, ephemeral external address `136.85.103.235`, and last start
timestamp `2026-09-19T20:36:29.072-07:00`. This observation only establishes
resource state; it does not qualify the guest or substantiate a runtime claim.

The initial SSH readiness probe then observed hostname
`mklinux-g4-g6-final-20260905`, custom kernel
`7.0.0-mk2-gce-lab`, boot ID
`3b4d5c5d-9436-4bab-9c17-609a4dcd181d`, active
`google-guest-agent`, and present Multikernel sysfs. Transfer is scoped to a
`git archive` of committed revision
`2051d0131428b3e75ab177b3f3e63bf4b41ad6ea`, excluding repository metadata,
the pre-existing untracked evidence directory, and these uncommitted notes.

The archive reached `/tmp` on the guest, but the first verification command
failed before unpacking because it combined a stale expected digest with an
incorrectly escaped `awk` field. The actual local SHA-256 is
`6cae2c16495742dd9f7d9f8b8f579e5a83e199bd0f2431de2bf05d89b1b9972a`.
This is an orchestration failure, not a runtime qualification result; the
failed command created no source directory.

The corrected remote verification matched the archive SHA-256, unpacked it
once at `/home/hairizuan-tw/mklinux-src-2051d013-20260920`, confirmed the
absence of `.git` and `evidence/runtime-20260907/`, and found
`runtime/go.mod`. This establishes that the guest's source tree came from the
exact committed archive described above.

The current source's host verifier passed on kernel
`7.0.0-mk2-gce-lab`, with CPUs `0-15` online, Multikernel sysfs mounted, and
the primary `ens4` address intact. Managed binary/deployment inspection then
reported no releases, generations, or stable links, while `mkruntimed`,
`mknetd`, `containerd`, and Docker were all inactive. The compound command
stopped at that nonzero service-state check before later idle/hash probes. This
is a qualified but currently unprovisioned runtime host, not a live-suite
failure.

The first read-only prerequisite inventory yielded no usable finding because
the remote shell expanded `dpkg-query`'s `${binary:Package}` format token under
`set -u` and stopped on an unbound variable. No guest state changed, and this
must not be misclassified as a missing dependency.

The corrected inventory showed a genuinely bare runtime image: Docker,
containerd, Go, and socat are absent, as are `/opt/mkruntime` and all runtime,
containerd, and Docker configuration directories. BusyBox, cpio, e2fsprogs,
GCC, iproute2, iptables, jq, make, Python, rsync, and util-linux are already
present. The retained 20-GB ext4 disk labeled `mk-mediated-host` is attached as
`/dev/sdb` through its Google by-id link but is not mounted. A complete runtime
and dependency provisioning step is required before the live suite can start.

The matching custom-kernel image/config and NBD module remain installed, and
the user's home retains Kerf, Linux, and `multikernel-artifacts` trees; neither
NBD nor `mk_transport` is presently loaded. The attached disk independently
reported serial `mk-mediated-storage-20260830`, exact 20-GB size, ext4 label
`mk-mediated-host`, UUID `507c0523-8e58-4ae3-9524-3b7513aad344`, and clean
filesystem state. It should be mounted by verified identity, not passed to the
destructive first-format path.

The retained upstream trees match the required pins exactly: Kerf commit
`8b72b3e9b266f8d32e707e2c1743ad7afc50b1ec` reports version `0.2.0`, and
Linux is commit `3bdd35b64413da0b4e089ce931bfc2e8b031cbf7`. Only the earlier child
initramfs is present in `~/multikernel-artifacts` (SHA-256 `487127ea…`); there
is no packaged transport module, relay, mediated-NBD helper, or current agent.
Those runtime-specific artifacts therefore require a new pinned/current-source
build before activation.

The missing documented packages then installed successfully: containerd
`2.2.2-0ubuntu1.1`, Docker `29.1.3-0ubuntu4.1`, Go `1.26.0`, socat
`1.8.1.1-1ubuntu0.1`, and musl-tools `1.2.5-3build1`. Their installation
enabled containerd/Docker units; runtime activation still awaits exact-source
artifact construction and validation.

Exact-source artifact construction then passed. All seven Go components are
static x86-64 executables stamped `0.1.0-dev` and revision `2051d013…`; the
release manifest hashes to `579ad1e9…`. Static warning-clean helper hashes are
`a0259098…` for `mkvsock-nbd` and `293ff1ea…` for `mkvsock-relay`.
`mk_transport` hashes to `bef1b888…`, reports the expected module name, and
has exact running-kernel vermagic
`7.0.0-mk2-gce-lab SMP preempt mod_unload modversions`. The individual
manifest hashes were also emitted for the shim, agent, agentctl, CNI, host
check, mknetd, and mkruntimed before privileged installation.

A new fallback initramfs containing the current agent/relay/init and exact
transport module passed gzip validation and hashes to `f8ce18d4…`. The pinned
static x86-64 `vmlinux` re-matched retained hash `5cdf26d0…`, and its installed
config hashes to `f7a61b04…`; the agent, relay, and module hashes also
re-matched. These exact objects were selected for manifest installation.

The runtime environment, host config, and kernel manifest passed JSON parsing
and hashed to `ea011183…`, `82270b0c…`, and `36ebbc7f…`. A fake-root run of
the current deployment manager accepted them, created generation `227c1fca…`,
and reported all expected stable links. Privileged host installation had not
yet begun at this preflight checkpoint.

Remote SHA-256 checks for all three transferred configuration inputs exactly
matched the local values before privileged installation. Subsequent claims can
therefore bind installation to those validated bytes.

After repeating all disk identity checks, the retained filesystem mounted from
`/dev/sdb` at `/srv/multikernel-storage`; it contains only the historical
`child-a`/`child-b` trees and `lost+found`, not a runtime subtree. A root-owned
pinned Kerf venv reports version `0.2.0`. All installed kernel/bootstrap/agent/
relay/transport/NBD artifacts re-matched their build hashes, and the current
bootstrap validator accepted manifest `36ebbc7f…`, the exact kernel release,
required config, OCI capabilities, and artifact identities. Services were not
yet activated at this checkpoint.

The immutable binary manager successfully activated release
`0.1.0-dev-2051d013…`, with every command/CNI link managed. The deployment
manager then correctly refused its first privileged install because the source
assets lived beneath an ordinary-user-owned extraction; it identified the
systemd mount unit as an unsafe input. It created no deployment generation and
started no service. A root-owned extraction of the same verified archive is
required for the intended trust boundary.

After rechecking the archive digest, extraction into private root-owned
`/root/mklinux-src-2051d013-20260920` allowed the deployment install to pass.
Generation `227c1fca…` is active, every declared config/unit/CNI/containerd/
support link is managed, and the root-owned source contains neither `.git` nor
the untracked evidence directory. Runtime service activation was still
deliberately deferred.

Containerd and Docker were active but empty before integration, and no child
instance existed. Containerd's effective default is config version 3 with an
import of `/etc/containerd/conf.d/*.toml`, despite no explicit main config.
Docker had no daemon config and remained defaulted to `runc`. This establishes
an idle, non-default-changing integration boundary.

The pre-restart integration guard found that the import was not actually
materialized: without an explicit main config, the effective containerd dump
advertised the glob but contained no `multikernel` stanza. The command stopped
before restarting containerd or changing Docker. This requires staging and
validating the complete default main config rather than assuming the displayed
default import is active.

The first staged-config command itself never reached the guest because a
nested grep quote caused local shell parsing to fail. No candidate or host
configuration changed; this was an operator orchestration error.

The quote-free retry generated the complete default containerd v3 config,
resolved its import to the named Multikernel runtime, and confirmed runc
remained the default. It installed only onto the previously absent main-config
path, restarted an empty containerd, and re-observed both the Multikernel
stanza and runc default with an empty task inventory.

The Docker merge candidate passed daemon validation, installed only on the
previously absent config path after an empty-inventory check, and reloaded
successfully. Docker now advertises the opt-in Multikernel runtime but remains
defaulted to runc, with no containers present.

Starting the managed units exposed a real config-publication mismatch.
`mknetd` started and `mk_transport` loaded, but `mkruntimed` exited status 2
because its strict loader rejected `/etc/mkruntime/config.json`: the deployment
manager intentionally exposes that path as a generation link, while the
loader requires a regular non-link pathname. The first `is-active` sample hit
the restart window and is not health evidence. No runtime state or prepared
storage subtree was created. Source remediation is required before live tests.

The loader now consumes a managed generation link safely: it resolves a
canonical snapshot, validates both public and target parent chains, enforces
owner/mode/size on the exact regular target, and checks identity before and
after its bounded read. Tests prove a safe deployment-shaped link succeeds and
a writable target parent fails. The hostconfig package passed once, 100 race
repetitions, and vet; repository-wide validation and live redeployment are
still pending.

Full local validation passed after the loader change: the complete Go race
suite, full vet, documentation/evidence chain, schemas, OCI, bind/bootstrap,
rootfs/storage/image/release, binary/deployment, ledger/capture/containerd, and
final evidence-audit checks all succeeded, and `git diff --check` was clean.
The known locally permission-gated socket subcase remains for privileged-host
execution.

This compatibility fix and the running findings were committed as `0792c9b`.
The source-only archive for that exact commit hashes to `263ea869…` and was
transferred to the guest; the pre-existing untracked evidence directory was
not included or modified.

The guest verified that archive and rebuilt the `0792c9bc…`-stamped release.
The release manifest is `1529d73b…`, mkruntimed is `b84ffb3a…`, the shim is
`2a832f17…`, mknetd is `41571385…`, and the current agent is `730826b0…`.
The regenerated gzip-valid initramfs is `d6af7590…`; helper and transport hashes
remained unchanged. These bytes still required a coordinated installed
artifact/manifest upgrade before service restart.

The coordinated upgrade activated release `0.1.0-dev-0792c9bc…`, atomically
updated the agent/initramfs/private manifest, and passed bootstrap validation
against manifest `5791ed6c…`. The sequence then stopped before service start
when an unprivileged hash command was correctly denied access to the mode-0600
manifest. The artifacts had validated; mkruntimed remained intentionally
stopped pending the corrected privileged check.

With the hash check run under the required privilege, manifest `5791ed6c…`
matched and mkruntimed revision `0792c9bc…` started successfully. PID `15246`
was stable across two delayed observations, private runtime/storage directories
and the empty journal appeared, and no child existed. The journal window shows
the old failures and then a clean final start with no repeated config error,
providing live proof of the corrected generation-link loader.

The exact corrected `mk-host-check` reported `qualified=true`: expected custom
kernel and Kerf, CPUs `0-15`, 67.4 GB memory, successful 16-GB pool dry-run,
active guest agent, and no findings/stale resources/instances. All five host
services were active, workload inventories were empty, and installed shim,
runtime, network daemon, and CNI hashes matched revision `0792c9bc…`. This is
the pre-suite clean-state observation.

The first basic live-suite workload failed during ctr task creation, before any
Docker workload: root construction returned `OCI configuration rejected:
[Errno 20] Not a directory: 'self'`. Image pull had succeeded and the harness
cleanup trap ran, but it emitted no gate pass marker. This is a current-source
runtime failure pointing at the descriptor-backed `/proc/self/fd` validation
path; cleanup inventories and daemon state must be checked before remediation.

The immediate cleanup audit found every inventory empty: no child, ctr task or
container, Docker container, runtime link, MK rule, journal entry, prepared
rootfs, or endpoint record. Services stayed active. The failed create was
therefore fail-clean and left only the expected private empty state
directories.

The error came from the OCI validator treating the intentional inherited path
`/proc/self/fd/3/config.json` as an ordinary pathname and refusing procfs
`self`. The validator now recognizes only that exact numeric-fd/config form,
checks the inherited fd is a caller-owned non-writable directory, and opens
exactly `config.json` relative to it no-follow before retaining the prior
bounded stable-file validation. A real pass-fd subprocess test and the focused
55-case OCI suite pass; broad checks and a live retry remain pending.

The full local race and vet suites plus the entire documentation/evidence,
schema, OCI, bind/bootstrap, rootfs/storage/image/release,
binary/deployment/ledger/capture/containerd, and final-audit chain passed after
the fix; `git diff --check` was clean. Only the known locally permission-gated
socket case remains for the privileged host.

The fix was committed as `7c94ab1`; its source-only archive is `fbeb4eb9…` and
was transferred to the guest. Exact-revision qualification requires rebuilding
the binaries and dependent initramfs as well as deploying the changed support
script, rather than retaining a mixed-revision installation.

The exact guest build produced release manifest `631f4d29…`, mkruntimed
`7cdd068d…`, shim `a34a5027…`, mknetd `72717e2f…`, agent `8ba5738b…`, and a
gzip-valid `47129dfc…` initramfs. A matching private manifest and corrected
deployment generation remain to be activated before retry.

That coordinated activation passed. The guest rechecked the exact archive,
manifest, agent, and initramfs hashes before stopping the empty services. It
selected immutable release
`0.1.0-dev-7c94ab1448dbabce2df59d2e1a20099b77b01802`, installed the corrected
support assets from a new root-owned extraction as deployment
`c4c1677859455084c197a3a8c37e7cc4f0700b02d77739399dd57f1b037b0636`,
and atomically replaced the agent, initramfs, and private manifest. The active
bootstrap validator accepted the resulting manifest, installed hashes matched,
and `mknetd` plus `mkruntimed` were both active after the restart health delay.
This is the coherent `7c94ab1` pre-workload boundary; it does not yet assert a
live workload pass.

After activation, mkruntimed retained PID `17732` over a five-second health
window and all four required services were active. The host check requires its
allocation probe as explicit flags: a bare invocation truthfully reports that
probe as unperformed even when `runtime.env` is loaded. With pinned Kerf and
the intended APIC 8-15/16 GB dry-run, it returned `qualified=true`, allocation
`ready`, no pool, instances, stale resources, or findings. The basic live suite
still remained pending at this checkpoint.

The next basic live retry passed the inherited-config-descriptor boundary and
then failed closed before task creation because containerd's BusyBox OCI spec
did not match the validator's device-resource allowlist: `only the default
deny-all device resource contract is supported`. Exit status was 1 and the
harness cleanup trap ran. This is a new live compatibility finding, not a G4-G6
pass; diagnosis must retain fail-closed device policy rather than broadly
accepting arbitrary cgroup device rules.

Cleanup was verified complete: no ctr/Docker workload, Multikernel child, or
runtime link remained. The generated contract is the conventional ordered
default list: a global `rwm` deny followed only by character-device allowances
for null, random, full, tty, zero, urandom, console, PTYs, and ptmx. Since the
guest projection intentionally drops host cgroup resource policy, compatibility
can safely recognize that exact list as the second inert default form while
continuing to reject missing, reordered, duplicated, or custom device grants.

The implementation recognizes exactly those two ordered defaults and retains
fatal rejection for every other resources object. The expanded 59-case focused
suite accepts the current containerd form and rejects a missing final rule,
reversal, and an added block-device grant; `git diff --check` is clean. Broad
local verification and another exact-source live retry remain pending.

The 59-case OCI suite passed 100 repetitions, followed by a clean full Go race
suite, vet, documentation/schema/evidence chain, rootfs/storage/image and
deployment boundaries, containerd configuration tests, final-evidence audit,
and `git diff --check`. The only local skip is the expected permission-gated
socket rejection intended for the privileged guest. A new immutable revision
and live rerun are still required.

The resulting `c409ad4` source archive (`53c97291…`) was digest-verified on
the guest and rebuilt with the full commit stamped into every binary. It
produced release manifest `ba5f2541…`, shim `43a38b7f…`, mkruntimed
`0db79260…`, mknetd `9340d38e…`, agent `6ac23778…`, and gzip-valid initramfs
`6d382231…`. No new live workload claim is made until these exact artifacts
and the matching deployment generation are active.

Those exact artifacts are now active as immutable release
`0.1.0-dev-c409ad4fe2efef9e5b7e6c2fe98a3a367414ce16` and deployment
`d2c096937c91f7f06f1ca569e07f12ede2b34a3eb068f272cd10f2f5fb7accef`.
Pre-stop inventories were empty, bootstrap validation passed, installed hashes
matched, and both services were active after a five-second restart check. This
is the corrected pre-workload boundary, not a live pass.

The exact live retry nevertheless failed at the same boundary with the new
exact-default rejection text. This falsifies the assumption that the device
array was the only content of the task's resources object; the earlier
metadata-only diagnostic printed only that nested array. Exit status was 1 and
no live pass is claimed. Diagnosis must inspect the complete resources object
before any further compatibility change.

The one-shot live-bundle diagnostic resolved the discrepancy: ctr adds
`cpu: {shares: 1024}` beside the standard device list only in the task bundle.
The builder link was atomically restored to the managed deployment target and
the diagnostic container removed. Shares 1024 is the kernel/cgroup default, so
it is another inert default contract at this boundary; only that exact CPU
object may be dropped, while all non-default shares and other CPU/resource
controls must continue to fail closed.

The validator now permits only an approved exact device list with no CPU
object or exactly `{shares: 1024}`. The 62-case focused suite accepts the live
ctr default and rejects shares 512, a quota-bearing CPU object, and all prior
device mutations. One focused run plus `git diff --check` passes; repeated and
full verification remain pending.

The expanded suite passed 100 repetitions and the complete Go race/vet plus
documentation, schema, evidence, builder, deployment, containerd, and final
audit chain passed afterward. The permission-gated local socket case remains
for the live host. The temporary diagnostic wrapper and captured resources
file were deleted only after the managed link and empty inventories were
reverified. A committed exact-source live rerun remains required.

Commit `3598056` was archived as source-only SHA-256 `a4acedb4…`, verified on
the guest, and rebuilt with the full revision stamp. The output identities are
release manifest `a60f72a7…`, shim `04daf0b8…`, mkruntimed `40b42a65…`, mknetd
`d1f2ff6f…`, agent `da6d8e95…`, and gzip-valid initramfs `4f58a729…`. These
exact outputs are recorded before activation; no new live claim exists yet.

The coherent activation selected release
`0.1.0-dev-3598056becf7beb698dbdb3388c2e3268b44efdf` and deployment
`5df65a4b65c5b5bdcc9174098887d8e151998c9fc0f37a21f65b45670b6f7f0d`.
Prechecks found no workload or child, bootstrap validation and installed hashes
passed, and both services survived the five-second health check. Live workload
behavior remains the next separate assertion.

The `3598056` workload retry cleared the prior explicit resource rejection but
still failed before task creation with an empty builder-error suffix. Its host
boot ID had changed from `3b4d5c5d…` to `f9d00c5f…`, so an intervening reboot
or host restart is now part of the evidence boundary and service/storage state
must be requalified. The suite exited 1; no live behavior is claimed.

Journal history shows an orderly shutdown at 04:34:44 UTC and a later boot at
09:17:27 UTC, not a runtime-triggered crash. The restarted host passes the
explicit qualification probe. Its attached `/dev/sdb` still has the approved
serial, label, and UUID, but it was not mounted: `/srv/multikernel-storage`
resolved to the root disk, where startup created an empty `runtime` directory.
Thus the retry uncovered a real reboot-safety defect. Service startup and the
live harness need an identity-checked storage mount precondition so runtime
data can never silently fall back to the boot filesystem.

The deployment now includes a pre-start storage-mount validator. It requires
the configured root-owned by-id device, whole-disk and byte-size identity,
udev serial, label, UUID, one read-write ext4 mount, matching device numbers,
and non-aliasing with `/`. Runtime environment storage fields are strictly
validated before deployment. Focused validator and deployment lifecycle tests
pass. Against the currently unmounted live disk, the validator failed at the
mountpoint check with status 1, proving the reboot fallback is closed before
service startup once this generation is installed.

After stopping the idle runtime, the retained disk's unmounted state and all
five identity properties were rechecked. Mounting the approved by-id target
with `nodev,nosuid` made the validator return
`RUNTIME_STORAGE_MOUNT_VALID`; the observed mount is `/dev/sdb`, read-write
ext4, at `/srv/multikernel-storage`. Mkruntimed then stayed active for five
seconds. This supplies both negative and positive live evidence for the new
precondition before its managed generation is installed.

The validator passed 100 local repetitions; deployment lifecycle and harness
shell-syntax tests also passed. `systemd-analyze verify` could not be used as a
standalone workstation gate because the host lacks the production mkruntimed
and mknetd executable paths, causing its expected executable-existence error.
Live generation activation is therefore the authoritative systemd check; the
remaining broad local gates still followed separately.

The entire Go race/vet and documentation, schema, evidence, OCI, bind,
bootstrap, rootfs, storage, image, release, deployment, ledger, containerd,
and final-audit chain then passed, including the new mount validator;
`git diff --check` was clean. Only the expected locally permission-gated socket
case remains for the guest. Committed exact-source activation and live retry
remain separate checkpoints.

Committed revision `1124739` was transferred as source-only archive
`08e22eb1…` together with validated environment `8d6184b8…`; both hashes
matched on the guest. Its exact build produced release manifest `fde70509…`,
shim `35ab7058…`, mkruntimed `1ad3d4be…`, mknetd `89091fb9…`, agent
`f819e3cb…`, and gzip-valid initramfs `7aae45ac…`. These are recorded before
deployment; no new behavior claim is attached yet.

Activation exposed a deployment-manager evolution bug. The binary release and
new agent/initramfs/manifest were installed, but support-generation install
failed closed while verifying the active predecessor: its manifest naturally
lacks the newly introduced storage-validator asset, while verification demands
the current exact file set. Both managed services remain stopped and no task
ran. Historical known-subset verification and link selection must be made
generation-aware before the upgrade can continue safely.

Upgrade verification is now generation-aware but not open-ended: only the
current file set and the exact predecessor lacking the storage validator are
accepted. Deployment IDs are recomputed from sorted recorded file hashes;
activation links only present assets and transactionally removes/restores the
optional link. The lifecycle test synthesizes an identity-correct predecessor,
proves activation has no dangling validator link, upgrades, and rolls back.
Focused deployment, storage-validator, and diff checks pass; broad verification
remains pending.

Independent focused teardown again reaches all 13 inventories zero with all
runtime services active/status 0 and zero restarts. The next complete retained
run begins from that clean state under support generation `25e6d343…`.

Capture `g6-shared-matrix-final.log` stops before matrix execution:
ordinary-user verification correctly cannot read the mode-0600 managed config,
so the capture closes at `2026-09-30T00:32:01.705852Z` with exit 1. This is a
capture-command privilege error and provides no runtime result. It is
preserved; the replacement command elevates only the config hash check and
uses a new transcript path.

Replacement capture `g6-shared-matrix-final2.log` verifies
matrix/helper/private-config hashes and passes lifecycle/isolation, including
mkruntimed PID 36974→39424 with child continuity and all-zero deletion cleanup.
It then fails Docker's bind CreateTask with `BACKEND_FAILURE` and exits 125 at
`2026-09-30T00:37:16.888569Z`; no bind or later marker is claimed. Independent
audit is all 13 zeros and all services healthy. Sanitized kernel evidence
identifies the cause: ctr bind teardown returns the 16-GiB Multikernel pool to
the host, and Docker's following create twice fails to reallocate that pool
with `-ENOMEM`. This is repeated idle pool teardown/recreation and host-memory
fragmentation, not bind admission or storage. Kernel command-line output
contained an authentication token and is deliberately neither retained nor
reproduced. Pool lifetime/ownership must be corrected before another replay.

The generation-evolution lifecycle passed 100 repetitions, followed by clean
full Go race/vet and documentation/schema/evidence, OCI, rootfs/storage/image,
deployment, containerd, and final-audit gates. The expected permission-gated
socket subcase remains a live-host check. Exact-source deployment and service
recovery remain pending.

Commit `3ce80c8` was archived as `e4a1cd26…`, verified on the guest, and built
with the full revision. It produced release manifest `bfa37246…`, shim
`eb418b63…`, mkruntimed `7d74471f…`, mknetd `a498f963…`, agent `6e0a59af…`,
and gzip-valid initramfs `0d61926c…`. These are recorded before the recovery
deployment and do not yet establish service or workload success.

The live schema-evolution recovery succeeded: release
`0.1.0-dev-3ce80c89e10f8aefcf70c625d232b6705b1f490d` and deployment
`01c685c211346c6d8eec3930e8bee84f04348a4461b35938063e1cf4952d97d0`
are active, bootstrap and installed hashes match, and both services remained
active for five seconds. This is authoritative unit parsing and mounted-path
startup proof; the deliberate missing-mount startup rejection follows
separately.

The deliberate service-level fault test passed. With empty inventories,
unmounting storage made `systemctl start mkruntimed` fail status 1 in the
pre-start validator, whose journal named the missing distinct mount; no child
was created. Remounting the same by-id disk produced
`RUNTIME_STORAGE_MOUNT_VALID`, and the restarted service retained PID `10351`
for five seconds. The root-disk fallback is therefore rejected by the actual
managed unit, not only by a standalone validator invocation.

The next basic workload retry again stopped before ctr task creation with a
blank-stderr `build child root: exit status 1`. The boot ID stayed stable, the
qualified mount remained present, and the explicit OCI rejection did not
return. This isolates a later silent builder command rather than the two fixed
preconditions. Exit status was 1 and no workload success is claimed pending a
one-shot stage trace.

The trace reached the very end: OCI validation, bootstrap, image validation,
two source scans, storage image construction, initramfs construction, and
independent verification all succeeded. The final jq equality check omitted
`-n`; with no stdin, `jq -e` exits 4 before evaluating its two slurped result
files, explaining the blank error. The builder link was restored and the
diagnostic workload removed. This requires a `jq -n -e` correction plus a
regression assertion before another exact-source live run.

The comparison now supplies null input explicitly with `jq -n -e`, and the
OCI/builder test asserts that exact form. Shell syntax, all 62 semantic OCI
cases, and diff checks pass once. The temporary root-only trace artifacts were
removed only after the managed link, empty inventories, mount, and service
health were reverified. Repeated and full gates remain pending.

The augmented OCI/builder suite passed 100 repetitions, followed by a clean
complete Go race/vet and documentation/schema/evidence, builder, deployment,
containerd, and final-audit chain; `git diff --check` also passed. A committed
exact-source guest rebuild and workload rerun remain required.

Exact commit `2864cff` was transferred as source archive `4ff24454…`, verified,
and rebuilt on the guest. It produced release manifest `ce2426bc…`, shim
`1de0faae…`, mkruntimed `6d8e5fca…`, mknetd `3c77289d…`, agent `793bcaf9…`,
and gzip-valid initramfs `8125169d…`. These identities are recorded before
activation; no workload claim is inferred from the build.

The exact artifacts are active as release
`0.1.0-dev-2864cff996498f32df0ae1ce5c6bd698fddcff06` and support generation
`5967012e86593a26b1aae8e184e335e459969c5af2d4fa0ad948c60c5ab6f896`.
Pre-switch inventories were empty, storage remained qualified, bootstrap
validation passed, and mkruntimed held PID `12242` for five seconds alongside
active mknetd. This is a pre-workload boundary only.

The live retry still failed before ctr task creation with blank builder stderr,
now as exit 1 rather than the former jq exit 4. This demonstrates a separate
post-comparison silent command. Host boot stayed stable and cleanup ran. No
workload claim is made pending a second trace of the remaining builder tail.

The second trace shows the complete builder now succeeds through final result
publication. The diagnostic progressed to task state `CREATED` and a real
Multikernel child named `mk-mk-builder-trace-diag2-fc2448f635970bae`; its
initiating SSH call produced no transcript while start remained unresolved.
The active objects are being preserved briefly for launch/agent-readiness
diagnosis before cleanup, so no lifecycle pass or clean-state claim is made.

The preserved state contains an initialized pool, a loaded 3 GB/2-CPU child,
and its mediated-storage server, but no network link. Normal forced task delete
failed with `invalid state LOADED`, leaving the task and child intact. Thus a
lost/cancelled start exposes a real lifecycle gap: cleanup must support the
post-load/pre-run state by unloading and deleting it. No cleanup success is
claimed yet.

Inspection localized the cleanup failure to a lifecycle state mismatch.
Deletion already unloads and deletes a non-running `LOADED` sandbox, and
journal recovery regards `LOADED` as a completed physical stop. The public
stop transition alone accepts only `RUNNING`, while the shim intentionally
uses stop-before-delete. The safe repair is a durable `LOADED -> STOPPED`
no-op with no `kerf kill`; running guests must continue through the backend
stop before the existing delete path.

That correction now passes 100 race-detector repetitions. Its regression
checks the durable `STOPPED` result, retained physical `LOADED` state, absence
of a backend stop/kill call, and successful subsequent deletion. Repository-
wide gates and live recovery of the preserved task are still required.

The full local qualification chain then passed: repository-wide Go race and
vet checks, documentation/link/schema/evidence validation, all privileged-
boundary simulation suites (including 62 OCI semantic cases), release and
deployment lifecycle tests, resource-ledger/capture/containerd checks, final
evidence audit, and `git diff --check`. The one permission-gated socket test is
still intentionally deferred to the disposable host. No live recovery claim
is made before exact-source deployment.

The committed correction is
`04da537332ce018c173ae2351679be5a09b20d81`; its source-only qualification
archive is
`479287d56ba6bafd95319a8ca61d7b16df0b365ca6bf7e0385c7658bbf961ab4`.
The disposable instance remains `RUNNING`. Transfer, independent guest hash
verification, rebuild, deployment, and recovery of the preserved task remain
unclaimed until each completes.

The disposable guest independently matched the full archive hash, extracted it
to a new mode-0700 source directory, and found the expected lifecycle source.
This closes only the source-transfer boundary; rebuild and live behavior are
not yet claimed.

The VM independently matches the 4,792,320-byte `2b3c836f…` archive and all
three script hashes in a fresh 648-file extraction. Syntax/compile checks pass
on unchanged boot `c5537cb9…`, and all four services are active. This is
transfer provenance, not live capacity evidence; execution remains pending.

The updated full local race suite is retained as
`g6-task-v2-unit-race-event-matrix.log`, mode 0600, 55,752 bytes, SHA-256
`54c55b82d1f126151cd4ed91b099ce53461bceebbc90daff4d761d3621cb6949`.
It passes in 8.509 seconds with 334 run entries and 139 passing groups. Only the
same two real pathname-socket tests skip locally; the updated VM rerun remains
pending.

The updated VM tree matches `main_test.go` SHA-256 `12c8d34f…`. Its full race
suite passes in 8.301 seconds with 334 run entries, 141 passing groups, zero
skips, and zero failures. The retained mode-0600/55,565-byte transcript
`g6-task-v2-vm-race-event-matrix.log` hashes to
`e95e63d1223953ccfdc8dbbd1305db1ce5500db1d16410c6eedf9a724f0b9f8b`.
Together with containerd-restart transcript `afa9187e…`, which preserves
create/start and post-restart exec/exit/delete events, this closes the exact
event ordering/publication-failure automation row under the documented
at-least-once replay contract.

The FIFO matrix audit identified two missing literal cases: terminal versus
non-terminal `CloseIO`, and teardown while stdin/stdout peers remain attached.
The first 20-repetition race run of the added tests failed at test setup: the
test could teardown before the new pump goroutine captured `p.stdinReader`.
No product claim is derived from that scheduling race. The test must establish
an active pump by sending and observing one byte before teardown.

After that correction, both additions pass 20 race-detector repetitions in
21.541 seconds. Terminal and non-terminal running execs acknowledge CloseIO
with one guest call and make repetition idempotent. The attached-peer teardown
test proves stdin delivery before teardown, prompt pump exit, nilled process I/O
handles, loss of the attached writer's reader, and EOF at the attached stdout
reader. Full local and VM suite reruns remain pending.

The full updated local package passes under the race detector in 9.567 seconds.
`g6-task-v2-unit-race-fifo-matrix.log` is mode 0600, 56,274 bytes, SHA-256
`ea8e826e6fd054bd9f5f6c65aa713e319e2b0a04f6728cbc6ed6f3e47f2f8b77`,
with 338 run entries, 141 passing groups, and only the two known local socket
skips. The zero-skip VM rerun is pending.

The VM copy matches updated `main_test.go` SHA-256 `10101b2d…`. Its complete
race suite passes in 9.335 seconds with 338 run entries, 143 passing groups,
zero skips, and zero failures. `g6-task-v2-vm-race-fifo-matrix.log` is mode
0600/56,087 bytes/SHA-256
`6fb8ffff1e22a32ed288fc6c42922bebe4e75f8d24f1f5812074a3cb26b00a8f`.
Together with shared live attach and exact post-start 37x91 resize evidence
`8ee2f800…` and forced worker-death FIFO continuity `fe6eee0d…`, this closes
both the named FIFO/attach/resize automation row and the broader FIFO hardening
implementation row.

All seven exact-source, revision-stamped guest binaries and their manifest now
build successfully. The release manifest is `122bf1a…`; shim is `f809e51a…`,
mkruntimed `bae3b11e…`, mknetd `0892a2fd…`, and agent `6600d82b…`. Daemon and
shim version output independently names the full `04da537…` revision. These
are build identities only; activation and live recovery are still unclaimed.

An activation precheck retained the original boot ID, found all three relevant
services active, the diagnostic task still `CREATED`, and `/dev/sdb` mounted
rw ext4 with `nosuid,nodev`. The command then failed before installation
because it passed an unsupported `--environment` option to the standalone
mount validator, whose interface requires explicit identity fields. This was
a command-use error with no release or service mutation.

The exact release subsequently installed and activated as
`0.1.0-dev-04da537332ce018c173ae2351679be5a09b20d81`. Restarting only
mkruntimed produced stable active daemon/network services after five seconds;
PID `14571` reports the exact revision and the host boot ID did not change.
This establishes activation only. Cleanup of the preserved old-shim task is
the next live boundary.

The first cleanup retry did not exercise the lifecycle fix: the preserved old
shim dialed its recorded `/run/mkruntimed.sock`, which was absent after daemon
restart, and ctr returned `UNAVAILABLE` before stop/delete. No cleanup is
claimed and the preserved resources remain live. This exposes an endpoint-
continuity/restart boundary that must be diagnosed before retry.

The daemon journal explains the absent socket and supersedes the earlier
five-second health observation: each start passes storage validation, then
about 18 seconds later rootfs reconciliation exits on `prepared build result
differs from journal`. Systemd repeatedly restarts it, so a momentary `active`
state is not durable health. The diagnostic task remains `CREATED` and its
child remains present. This prepared-rootfs upgrade/reconcile mismatch must be
remediated before the loaded-stop correction can be exercised live.

The precise cause is representation drift: the build artifact retains the
builder's compact JSON bytes, but rootfs state persists its `json.RawMessage`
inside indented JSON. Reloading that state returns an equivalently indented raw
message, and reconcile incorrectly compares it byte-for-byte with the compact
file. A safe compatibility check can compact both valid JSON documents before
byte comparison; unlike generic object equality, this still preserves member
order and exact number/string spellings and admits only insignificant JSON
whitespace introduced by persistence.

The resulting comparison and integration regression pass 100 rootfs race-
detector repetitions. A real compact builder result is indented to reproduce
durable-state reload and is accepted only after all existing content/digest
checks; altered artifacts remain rejected. Repository-wide qualification and
live exact-source recovery are still pending.

The complete local race/vet and documentation/schema/evidence/boundary/
deployment/final-audit chain then passed, as did `git diff --check`; generated
Python cache files were removed. An immutable commit and exact-source live
recovery are still required.

The restart correction is now immutable at
`89aada60b430386b0e5ec51c322006e480e5488e`; its source archive is
`2d1e93c18d5e92588d9db829ae76b369677cf575783f6ac3aa1f5513ded2cf76`.
No live claim is made before independent guest verification, rebuild,
activation, and a health observation longer than the former 18-second crash.

The disposable guest independently verified that archive and rebuilt the
complete release: manifest `db262432…`, shim `66dc651c…`, mkruntimed
`6f2f2348…`, mknetd `a5ab9e91…`, and agent `31369e71…`. The daemon identifies
the full `89aada6…` revision. No activation or stability claim is yet made.

The exact release is active, and a deliberate daemon restart retained PID
`15958`, the daemon socket, and the `89aada6…` version for 35 seconds, beyond
the former roughly 18-second failure latency. The host boot ID is unchanged;
the prior mismatch in the displayed journal predates this new process. This is
durable restart evidence, while preserved-task deletion remains unclaimed.

Normal forced ctr task deletion now succeeds against the corrected daemon. The
first post-delete audit shows an empty task inventory, no Multikernel child,
and zero rootfs records. Its `pgrep -af` expression produced command-line false
positives, then an assumed lifecycle state pathname was absent and stopped the
remaining checks. This proves core reclamation but not yet the complete clean-
state inventory; a corrected exact-process/path audit follows.

A second audit attempt likewise ended early when a missing `/run/mkruntimed`
operand made `find` fail under `pipefail`. Its partial output exposed a retained
generation-specific NBD log under `/run/mkstorage`. Whether this is only stale
logging or a broader storage cleanup defect remains open pending exact process
and journal inspection.

The corrected inventory establishes empty task/child/NBD-process/rootfs
inventories, all CPUs returned online, stable daemon PID `15958`, active
services, and the unchanged boot. It also confirms a genuine residual: one
storage export remains journaled with its runtime log. Container metadata is
being retained until that leak is understood. The loaded-task remediation is
therefore not yet a complete leak-free cleanup result.

Targeted inspection corrects the leak interpretation. The retained export is
terminal `RELEASED`, has a release timestamp, successful offline-check result,
zero counters, and an exact `MKNBD_SERVER_CLOSED synced=1` terminal record;
there is no server process or prepared storage artifact. The journal entry and
generation log are intentional durable release/idempotency evidence, not live
resource ownership. Only container metadata removal and a final inventory
remain.

Exact `69e9ea9` starts from an independently proven zero-resource baseline,
passes all focused races, and records one coherent live owner: version-4
`PREPARED` rootfs state, version-2 `ACTIVE` storage, distinct validated sandbox
and export generations, exact fd-3 image inode, and exact pinned executable
inode. The competing root helper signals only after its nonblocking lock is
contended, but global evidence `umask 077` creates its result mode 0600/root;
the next ordinary-user `grep` fails with permission denied before publishing
the observation. No lock claim is taken from that harness failure. Private log
is mode 0600/36,289 bytes/SHA-256 `8996f431…`, credential scan empty. After
zero live owner verification and idle daemon restart, the 19-counter audit
again passes (SHA-256 `588ab8d1…`). The result assertion now uses `sudo grep`.

That correction is isolated as exact commit
`65903b8f600e545222daa6138ff3ad68d0d20390`; driver SHA-256 is `505ae529…`,
and its 4,761,600-byte archive SHA-256 is `22b0bfff…`. Transfer and a fresh
rerun remain.

Exact `65903b8` qualification exits 0 with
`G4_SINGLE_OWNER_LIVE_PASS`. Every focused race case passes using ext4-backed
VM scratch. One live version-4 `PREPARED` rootfs record is tied to one
version-2 `ACTIVE` export and its exact process record: sandbox generation
`bc2bf70e…`, distinct export generation `230b0bac…`, image/fd-3 inode
`2064:524349`, and pinned executable inode `2049:6079391`. An independent
nonblocking exclusive lock against that inode is contended. Teardown retains
the same generations in `RELEASED`, supplies nonzero I/O/flush counters plus
offline check `e2fsck-clean-sha256:d7c3d5d2…`, removes the pathname and all
live records, and then the same independent descriptor acquires the lock at
the same inode with link count zero. Private pass transcript is mode 0600/
46,525 bytes/SHA-256 `5790e421…`; credential scan is empty. Following the
scoped idle mkruntimed restart, independent private audit is mode 0600/15,232
bytes/SHA-256 `b6b0b230…` and proves boot `c5537cb9…`, candidate daemon SHA
`0e1c87c3…`, no Kerf pool or child, all 19 counters zero, and four active/
running zero-restart services. This closes the single-owner, generation,
duplicate-attach, and stale-lock requirement; repository gates remain.

The post-closure full repository gate passes documentation and links, all 7
schemas/22 cases, 17 current manifests, the 95-case OCI boundary, bind/
bootstrap/rootfs/storage/mount/image fixtures, lifecycle and deployment
managers, GCE ledger, evidence capture, containerd configuration, and the
final-evidence audit. The explicit local socket `EPERM` skip is covered by the
zero-skip VM execution. `git diff --check` is clean, four generated bytecode
files are removed, and checklist totals are 41 closed and 44 open.

The next-row audit finds that Plan 04 already defines the intended narrow v1
writable-state model: writable host-path volumes and configured persistence
belong to a later storage-format and ownership contract. Current code matches
that boundary. Each task has an ephemeral private ext4 root; generic writable
binds and shared/slave propagation fail closed; read-only inputs are
materialized with numeric metadata and no host path or propagation; only the
three exact Docker metadata files become identity-bound private writable seed
copies; and OCI UID/GID mapping fields are rejected as unknown Linux fields.
Closure still needs a single explicit contract, focused mapping-rejection
assertions, and exact-source live proof that ordinary private writes disappear
after delete/name reuse and rejected generic writable binds allocate nothing.

The supported-model contract is now explicit in Plan 04, the ownership table,
and runtime README. V1 roots are ephemeral per sandbox generation; persistence,
writable volumes, generic writable host binds, UID/GID mappings, and shared/
slave propagation are unsupported; read-only copies retain numeric ownership
with propagation `none`; only exact Docker metadata seeds are privately
writable. Focused validation passes 97 OCI semantic cases with explicit UID and
GID mapping rejection, and materialization now asserts numeric UID/GID
preservation. Live name-reuse non-persistence and zero-allocation writable-bind
rejection remain.

The supported contract, tests, and live qualifier are isolated in exact commit
`3ae721df0338d2336c3dac3c775fc4e0e61a59e5`. Its 4,771,840-byte archive
hashes to `20c5b8f1…`; driver, OCI test, and bind test hashes are respectively
`c6cf3c29…`, `181bd6ae…`, and `df6effc2…`. Fresh transfer, verification, and
VM execution remain.

Exact `3ae721d` qualification exits 0 with
`G4_WRITABLE_STATE_MODEL_LIVE_PASS`. The VM passes all 97 OCI/mapping cases and
numeric-owner bind materialization. Two lifecycles reuse exact ctr name
`mk-ephemeral-state`: the first writes `generation-one`; the second requires
that path absent before writing `generation-two`; both converge to 13 zero
live-state counters. Generic ctr and Docker writable binds fail with statuses 1
and 125, shared propagation fails with status 1, all three match the bind
contract, the host control remains `host-immutable`, and every rejection
converges to zero live state. Private pass transcript is mode 0600/314,832
bytes/SHA-256 `cd5b328f…`, credential scan empty. Following idle pool release,
independent private audit is mode 0600/15,072 bytes/SHA-256 `01e0ec0d…`; it
binds boot `c5537cb9…` and daemon SHA `0e1c87c3…`, proves no Kerf pool/child,
all 19 counters zero, and four active/running zero-restart services. Combined
with prior live read-only-bind/no-write-through and caller-snapshot evidence,
this closes both the supported-model row and its persistence/isolation evidence
row under the explicit narrow v1 scope.

The post-closure full gate passes documentation/links, 7 schemas/22 cases, 17
current evidence manifests, the expanded 97-case OCI boundary, bind/bootstrap/
rootfs/storage/mount/image fixtures, lifecycle and deployment managers, GCE
ledger, evidence capture, containerd configuration, and final-evidence audit.
The explicit local socket `EPERM` skip is covered by VM execution. `git diff
--check` is clean, generated bytecode is removed, and checklist totals are 43
closed and 42 open.

After removing the now-resource-free container metadata, all ctr task/
container, child, NBD process, rootfs record/artifact, and runtime-link
inventories are empty; CPUs `0-15` are online; services and PID `15958` remain
stable on the same boot. Kerf reports no pool, no instances, and no loaded
kernel images. This closes the preserved loaded-task recovery boundary without
live leaks. The normal exact-source workload suite remains to be rerun.

The initial suite command made no runtime change: archive extraction left the
script non-executable, and the host sudo policy both ignored `-E` and rejected
direct execution. The exact file will be run through `bash` without relying on
whole-environment preservation.

The subsequent `sudo bash` attempt likewise made no workload change: the suite
deliberately rejects UID 0 and its initial cleanup was empty. It must be
launched by the ordinary SSH user, with its own scoped sudo calls providing
the required privilege.

GCE returned `instance ... was not found` before the correct ordinary-user
suite invocation began. No suite command ran and no workload result is
inferred. A fresh disposable host must be created and qualified before live
execution resumes; prior evidence remains historical evidence for the exact
states already observed, not evidence about the replacement host.

Read-only cloud inventory found no remaining VM, while the persistent 20 GB
mediated-storage disk and the 100 GB qualified custom-kernel snapshot are both
`READY`. Recorded infrastructure evidence identifies the former VM as
`n2-standard-16` with a pd-balanced auto-delete boot disk restored from that
snapshot, the storage disk attached without auto-delete, Secure Boot disabled,
and disposable/purpose labels. The replacement will reproduce this recorded
shape and be qualified afresh.

GCE successfully recreated the recorded shape. The snapshot-derived 100 GB
pd-balanced boot disk is attached to a running `n2-standard-16` instance with
internal address `10.148.0.57`, ephemeral external address `34.87.128.162`,
and the retained storage disk. These are control-plane facts only; no guest or
runtime qualification is inferred yet.

Guest inspection confirms a new boot ID
`e22e4b51-1038-4254-89f8-fc91d57a74a9`, the qualified
`7.0.0-mk2-gce-lab` x86-64 kernel, 16 CPUs, about 64 GiB RAM, active guest
agent, loaded Multikernel support, and no children. The attached whole 20 GiB
ext4 disk exactly retains its expected label, UUID, serial, and size and is not
yet mounted. Remaining host/runtime cleanliness and topology checks are still
open.

The initial package probe is invalid because remote-shell expansion of dpkg's
`${Status}`/`${Version}` fields under `set -u` produced false `missing` lines.
Its independent checks did show inactive containerd, Docker, and runtime
services; empty visible workload inventories; retained pinned Kerf/Linux trees;
and only an old child initramfs artifact. A quoting-safe package check follows.

The rerun reliably establishes missing containerd, Docker, Go, and socat, with
the other named prerequisites installed; its version formatter still emitted
literal `${Version}`, so those strings are not version evidence. Pinned Kerf is
clean at `8b72b3e…` and version 0.2.0, pinned Linux is clean at `3bdd35b6…`,
and topology is eight cores with CPU sibling pairs 0/8 through 7/15, matching
the documented host/pool allocation.

Unambiguous default dpkg output captured the installed toolchain versions
(BusyBox 1.37, cpio 2.15, e2fsprogs 1.47.2, GCC 15.2, jq 1.8.1, Python 3.14.3,
and the other documented prerequisites). Provisioning is limited to the four
confirmed missing packages.

The missing packages installed successfully: containerd 2.2.2, Docker 29.1.3,
Go 1.26, and socat 1.8.1.1. Containerd and Docker are active with their package
defaults; runtime integration has not yet been installed or claimed.

The fresh guest independently verified the full exact-source archive hash,
extracted it privately without `.git`, and passed revision `89aada6`'s
`verify-host.sh`. Source identity and baseline host qualification are now
established for the replacement; artifacts and runtime remain undeployed.

Exact-source outputs reproduce the earlier host byte-for-byte: manifest
`db262432…`, shim `66dc651c…`, daemon `6f2f2348…`, network daemon `a5ab9e91…`,
and agent `31369e71…`. Static warning-clean helper builds likewise reproduce
NBD `a0259098…` and relay `293ff1ea…`, all x86-64 ELF. Remaining kernel/module,
mount, manifest, and deployment work is not yet claimed.

The pinned build reproduced transport module `bef1b888…`, with exact module
name and running-release vermagic. Static x86-64 `vmlinux` is `5cdf26d0…`;
running kernel image/config identities were recorded separately. Root-owned
artifact staging and manifest validation remain open.

The exact gzip-valid agent initramfs is `09314674…`; inspection confirms its
init, agent, transport module, and relay members. This is a build result only,
pending root-owned installation and manifest validation.

Read-only fsck passed all five stages before mounting. An exact-UUID fstab
entry now mounts the retained by-id device rw ext4 with `nosuid,nodev` at the
qualified path. Historical child-a/child-b images are retained outside the
empty runtime subtree. The managed storage validator remains to be exercised
after deployment.

Pinned Kerf 0.2.0 now runs from a non-editable root-owned venv with all imports
verified. Non-secret host config, storage-aware environment, and current
artifact manifest hash to `82270b0c…`, `8d6184b8…`, and `afdbcc23…` and have
been transferred. Their remote integrity and privileged installation are not
yet claimed.

The guest matches archive SHA-256 `8dccf1fa…` and the complete static build
finishes with full revision `e396511c…`, including exclusive manifest
publication. A subsequent verification command exits 2 because it incorrectly
supplied a nonexistent `--check` mode to a tool that requires an output path.
This is a post-build harness-interface error: compilation is observed, while
manifest verification, hashes, installation, and activation remain unclaimed.
The unique root-owned build tree is retained for corrected verification.

Corrected verification publishes an independent exclusive manifest and
byte-compares it equal to the build manifest. Exact SHA-256 values are
`ee31cd3c…` for the manifest, `29dc06dd…` for mkruntimed, `1b245d66…` for the
shim, and `b57bae66…` for mk-agent; the built daemon prints the full exact
`e396511c…` revision. The temporary comparison manifest is removed after the
match. Installation and execution are still unclaimed.

Remote hashes matched before installation. Every root-owned artifact re-matches
its build identity, the exact `89aada6` binary release is solely active with
all managed links, and a root-owned source extraction supplies deployment
assets. Bootstrap validation accepts manifest `afdbcc23…`, compatibility pins,
artifact identities, kernel config, and all ten advertised OCI features.
Deployment and services remain inactive.

Managed generation `5967012e…` is solely active, with every unit, config,
CNI/containerd fragment, builder, validator, guest init, and helper link
verified managed. The installed mount validator accepts the exact qualified
disk. Container-engine integration and runtime service activation remain
separate pending boundaries.

Before engine integration, all ctr/Docker inventories are empty. Containerd
has no main config, but its v3 default advertises the fragment import and runc
default; Docker has no daemon config and exposes only runc as default. The
managed fragment is installed but activation remains a separate idle-engine
restart step.

The first integration attempt made no configuration change: a fixed-string
check mistakenly searched for literal `\x27` around `runc`. Inspection of the
staged candidate proves the import, Multikernel stanza, and exact runc default
are present. Both engines remain active without new config; a corrected check
will activate the already validated candidate.

The corrected idle-engine activation passed. Effective containerd contains the
named Multikernel handler and retains runc default; Docker's merged config
validated and reload exposes Multikernel plus both runc names while keeping
runc default. Task/container inventories remain empty. Runtime services have
not yet been started.

Managed services are now durably healthy: daemon PID `12120` and network PID
`12095` stayed fixed for 35 seconds, both report exact `89aada6…`, the daemon
socket and qualified mount validation pass, no child exists, and the boot ID
is unchanged. This is longer than the former reconcile failure interval. The
ordinary-user workload suite follows.

The exact suite passed every service, mount, shim, TUN, empty-child, image, and
host-identity precondition, then the first ctr task failed closed with silent
`build child root: exit status 1`. Its cleanup trap ran. The jq null-input fix
is present, so the fresh host has reproduced a different or environment-
dependent silent builder boundary. No live workload result is claimed pending
leak audit and stage tracing.

Initial cleanup audit finds no ctr task/container, child, or rootfs record. It
ended on the absent optional storage journal, consistent with failure before
export creation. A tolerant audit is still needed for remaining directories,
services, and daemon journal.

The tolerant audit proves fail-clean behavior: no storage journal, prepared
artifact, or runtime log exists; all services and original runtime PIDs remain
healthy; the daemon journal has no restart. A one-shot root-only xtrace wrapper
will preserve builder stdout, capture stderr privately, and be atomically
removed from the managed path afterward. Raw trace data will not be retained
in learnings because it can include ephemeral authentication inputs.

The private trace completed the entire builder, including final independent
comparison and result JSON, and the managed link was restored. The subsequent
failure is manifest approval: the daemon additionally requires and validates
`config-<kernel_release>` adjacent to vmlinux, but provisioning omitted that
already hashed config. The standalone bootstrap tool does not cover this
daemon-only config check. The correct remedy is to install the exact kernel
config companion, not relax manifest validation.

The root-owned companion config now matches `f7a61b04…` and contains the exact
three required options. The managed builder link is restored, temporary trace
files are removed, and inventories are empty. The ordinary suite can now test
both daemon manifest approval and the normal builder executable.

The normal managed path still exits silently in the builder. The earlier
wrapper completed only when invoking the resolved versioned path, so it changed
the path semantics under test. A clean-state check will precede a narrower
diagnostic: retain the public symlink, temporarily add an ERR trap to the
versioned script itself, and restore/hash-check that immutable asset afterward.

The direct-path ERR trap pinpoints `test ! -L "$artifact"`: production
invocation derives guest-init paths from the public managed-link directory, so
they are managed symlinks and fail the intended artifact rule. The wrapper had
hidden this by using immutable-generation paths. The safe fix is to derive the
support directory from Bash's already-open script descriptor
`/proc/$$/fd/255`, thereby pinning the executed generation while retaining the
strict no-symlink artifact check. The temporary edit was restored byte-exact
and the deployment still inspects cleanly.

Shell syntax and all 62 OCI/builder cases now pass 100 repetitions. The new
regression requires descriptor-derived support paths and exercises an actual
immutable generation behind a public symlink via both shebang and explicit
Bash invocation; both resolve the generation directory. Artifact symlinks
remain forbidden. Repository-wide qualification is still pending.

Repository-wide race/vet plus the full documentation, schema, evidence,
boundary, deployment, and final-audit chain now passes; `git diff --check` is
clean and generated Python cache is removed. The one permission-gated socket
case remains for the guest. An immutable commit and exact-source redeployment
are still required.

The fix is immutable at `21f772c393ca32be28502ce20730343cdb51ea81`; its
source archive is `624c29c459ae40e08d7a016a4ac0f7ace4cada8929bbfce8974eb0fe9b24cffe`.
Transfer, guest verification, full rebuild, deployment activation, and live
rerun remain separate pending boundaries.

Independent guest verification and full rebuild completed: manifest
`4df065cd…`, shim `ca72d720…`, daemon `676131be…`, network daemon `9cf6f6de…`,
agent `d244ef6c…`, and dependent initramfs `63c6ae5d…`. Activation and runtime
behavior remain unclaimed.

The exact `21f772c…` release and manifest `6bebfc81…` are active with new
deployment `ca7bc745…`; the prior generation remains available for rollback.
Bootstrap validation passed, and daemon/network PIDs `14981`/`14964` were
stable for 35 seconds with the exact daemon revision. Live workload execution
is next.

The guest reproduces archive hash `164a1b9d…`, but current boot is now
`1151712d…`, proving an intervening reboot. The `/tmp` cleanup-audit script no
longer exists, so that command yields no resource-state evidence. Current-boot
service and resource inventories must be re-established before proceeding.

The restored audit is fully clean, all five units are active, and exact
`298d3de…` persisted through reboot. mknetd PID 1231 is unrestarted;
mkruntimed PID 1505 has one restart, whose current-boot journal proves the
known ordering case: initial qualification saw the Google guest agent as
unknown, then systemd's retry succeeded. The new interval is clean but cannot
reuse earlier PID-continuity evidence.

The root-owned exact build succeeds with hashes `d56a0d15…` (release),
`1e25d1e2…` (shim), `6d3bc10e…` (daemon), `4daf0150…` (mknetd),
`9534c5b3…` (agent), and `ac25694d…` (initramfs). Privileged inspection
confirms all three embedded bootstrap artifacts. Active state is unchanged.

Managed binary release `f5d8da4…` and support deployment `b8176d6c…` are
selected; exact-source/deployed validator SHA-256 is `31bd16b0…`. Daemon
processes 1231/1505 remain unchanged, so this is filesystem selection only.

Root-only f5 agent/initramfs staging preserves hashes
`9534c5b3…`/`ac25694d…`; strict candidate manifest `bfb05789…` validation
passes. Immediate clean-host audit remains before activation.

The immediate audit again reports every workload/resource/process inventory
empty, with current-boot daemon identities stable. Activation can proceed.

Coordinated activation succeeds on boot `1151712d…`, retaining the 298
manifest for rollback. Active hashes `bfb05789…`/`d56a0d15…`/`1e25d1e2…`,
version reports, and strict bootstrap validation all agree on f5. New
mknetd/mkruntimed/containerd/Docker PIDs 9314/9333/9343/9378 have zero
restarts/status 0. Live workload proof follows.

Exact runner `7fb6cc6e…` proves Docker clears resource validation, then is
rejected because its cgroupsPath is not the currently accepted absolute form.
Exit 125 invokes cleanup. Exact path form/semantics and cleanup remain to be
observed.

The cleanup audit is fully empty and f5 daemons 9314/9333 remain stable with
zero restarts. The open boundary is isolated to cgroupsPath representation.

Docker emits `system.slice:docker:<64 lowercase hex>` cgroupsPath. It is host
placement metadata already omitted from the child projection, paired here
with an explicitly unrestricted resource policy. A narrow bounded grammar can
be accepted alongside canonical absolute paths; all other systemd forms stay
fail-closed.

The exact Docker systemd grammar is implemented and the focused suite passes
78 semantic cases, with wrong-slice, short, and uppercase forms rejected.
Python compilation, qualification shell syntax, and diff checks pass; full
gates and live proof remain pending.

The complete race, vet, documentation/schema/evidence/deployment, and diff
gates pass with 78 OCI cases; only the known local socket `EPERM` subcase is
skipped. The checkpoint is ready to commit and qualify live.

The immutable checkpoint is `92531eb6453f72783c63ba3047b8b5c2666fde9b`;
its source-only archive hashes to `c6d542b9…`. Historical untracked evidence
remains excluded. Guest verification is pending.

Guest archive hashing matches on unchanged boot `1151712d…`; all audited
inventories are empty and f5 daemons remain stable/unrestarted. Exact build
preconditions pass.

The exact build succeeds with release/shim/daemon/mknetd/agent/initramfs
hashes `95382a6f…`/`c5c3bb28…`/`2ccb1418…`/`902b4a23…`/`8c89b5a3…`/
`3ed53121…`; required bootstrap membership is verified. Active state is
unchanged.

Managed release `92531eb…` and deployment `844df6d2…` are selected, with
deployed/source validator hash `a6f2b969…`. Daemon processes remain unchanged;
manifest staging and restart are pending.

The root-owned `92531eb…` artifacts are staged without service activation.
Candidate manifest SHA-256 is `807d1770…`; strict bootstrap validation passes
and its agent/initramfs pins reproduce exact build hashes `8c89b5a3…` and
`3ed53121…`. This establishes candidate identity only. A fresh empty-host
audit must still precede coordinated activation.

That fresh audit passes with zero default/moby tasks or containers, Docker
objects, children, storage files, active state, endpoint/rootfs records,
`mkv*` links, firewall rules, and runtime helper processes. The still-running
f5 mknetd/mkruntimed PIDs 9314/9333 are active with zero restarts. Activation
can proceed without interrupting a workload.

The first activation script incorrectly required nonexistent unit
`opt-mkruntime.mount` and exited before the new revision could start. Automatic
rollback restored the f5 manifest: active, rollback, and preserved copies all
hash to `bfb05789…` on boot `1151712d…`. The resulting service restart is
healthy at PIDs 15366/15385/15392/15427 with zero restarts, and the complete
resource audit remains empty. Because the candidate was moved before the unit
error, it must be regenerated. This is a deployment-orchestration failure,
not evidence for or against `92531eb…` workload behavior.

The first idempotent restaging retry made no candidate: it matched the staged
agent, then attempted to hash the mode-0600 build initramfs as the ordinary
SSH user and received permission denied. The corrected check must hash both
root-only inputs under `sudo`; active state is unchanged.

The privileged retry matches both immutable staged artifacts to their build
outputs and recreates the strictly valid `807d1770…` candidate without
overwriting either artifact. The corrected activation will omit only the
nonexistent mount-unit operation and preserve rollback behavior.

The next activation actually started healthy `92531eb…` processes but a bad
post-check path (`/usr/local/bin/mkruntimed`) falsely failed; the real service
path is `/usr/local/sbin/mkruntimed`. The trap restored the f5 manifest without
restarting already-active services, briefly leaving manifest/process identity
incoherent. Exact path inspection exposed this immediately. Recovery logic now
stops and restarts the whole service set whenever it restores a manifest.

Candidate regeneration and the corrected coordinated activation then pass on
the unchanged boot. Active manifest is `807d1770…`, rollback remains
`bfb05789…`, all three runtime binaries identify exact `92531eb…`, and
mknetd/mkruntimed/containerd/Docker PIDs 18265/18283/18294/18328 are active
with zero restarts. Workload behavior remains a separate boundary.

The immediate post-activation audit reports every workload, child,
storage/state, network, firewall, and helper-process inventory empty while the
exact-revision daemons remain stable. This is the clean boundary for the next
ordinary live suite.

The exact runner (`7fb6cc6e…`) proves ctr reaches `RUNNING` and Docker now
clears cgroupsPath validation. Its next rejection is
`linux.maskedPaths contains a duplicate or unsupported path`; Docker never
starts and cleanup is invoked. This is a new fail-closed OCI boundary, not a
suite pass. Cleanup and the exact requested list require independent evidence
before deciding whether it is equivalent to the child's fixed isolation.

Independent cleanup is fully empty and exact-revision daemons 18265/18283
remain stable with zero restarts. The rejection is fail-clean. The next step
is observation of Docker's exact ordered maskedPaths request, including any
duplicates, rather than broad field acceptance.

The live runc probe contains 12 unique masked paths (`/proc/acpi`,
`/proc/asound`, the standard proc sensitive pseudo-files, `/proc/scsi`,
`/sys/devices/virtual/powercap`, and `/sys/firmware`) and readonly paths
`/proc/{bus,fs,irq,sys,sysrq-trigger}`. Therefore the validator's combined
“duplicate or unsupported” message denotes an unsupported-set mismatch, not a
duplicate. These are access restrictions, so they require comparison with the
child's actual fixed mounts before any exact compatibility projection.

Inside a real Multikernel child, `/proc/interrupts` is present, mode 0444, and
readable (1091 bytes in this observation). Docker's mask is therefore a real
security restriction. Compatibility requires adding precisely this missing
standard path to the bounded agent policy so the existing mount-based masking
is applied; host-side stripping would be incorrect.

The implementation extends the bounded set by `/proc/interrupts` only and
keeps the existing mount-based enforcement. Tests pin Docker's complete exact
12/5 lists and fail closed on duplicates or unknown paths. Focused agent race
tests, vet, and diff checking pass; no live claim is made from local tests.

The full Go race suite and vet pass. The following documentation command did
not run because `scripts/check-docs.sh` was addressed relative to `runtime/`
instead of the repository root. Its “No such file” result is an invocation
error, not evidence about documentation validity.

Rerunning from the repository root passes the entire documentation, schema,
evidence, deployment, boundary, and final-audit chain plus diff checking. The
known locally permission-gated socket subcase is the only classified skip and
generated cache is removed. Combined with the full race/vet result, this
checkpoint is ready to freeze for exact guest qualification.

The immutable checkpoint is
`9bd7e94e598bb9ba439d2cb51c7911402e972e51`; its source-only archive hashes
to `49a337ef…`. It excludes repository metadata and the existing untracked
evidence tree. No guest behavior is inferred until independent digest
verification, exact rebuild, activation, and workload execution complete.

The active validated manifest pins the reusable transport module and relay at
hashes `bef1b888…` and `293ff1ea…`. Exact rebuild will consume those manifest-
approved paths directly, avoiding any assumption that shared bootstrap inputs
live under the preceding revision's directory.

The guest matches archive digest `49a337ef…`, verifies root-only extraction
and exclusions, then builds release/shim/daemon/network-daemon/agent/initramfs
hashes `5028dae7…`/`31fe5c7a…`/`baf0833e…`/`f77e3215…`/`0cbfdf78…`/
`5ea168b0…`. The initramfs contains all three required bootstrap members.
These are build identities only; selection and activation remain unclaimed.

Managed selection activates release `9bd7e94…`; unchanged exact-source
support inputs reproduce deployment `844df6d2…`, with all links managed. A
subsequent hash comparison was invoked without privilege against the root-only
source and stopped before service inspection. No service restart was issued;
the comparison and process-state check must be rerun under `sudo`.

The privileged retry matches source/deployed validator hash `a6f2b969…`.
Daemons remain PIDs 18265/18283 with zero restarts; direct `/proc` executable
inspection reports the running mkruntimed is still `92531eb…`. Filesystem
selection is therefore distinct from activation, as required.

The root-owned agent/initramfs artifacts retain exact build hashes
`0cbfdf78…`/`5ea168b0…`, and candidate manifest `effb90a2…` passes strict
bootstrap validation. It is not active; clean-state and coordinated-restart
boundaries remain.

Fresh pre-activation inventory is zero for every workload/resource category,
and predecessor daemon PIDs 18265/18283 remain healthy with zero restarts.
The staged manifest can be switched without interrupting a workload.

Exact activation succeeds without reboot. Manifest `effb90a2…` is active and
the `92531eb…` manifest remains rollback at `807d1770…`. Service PIDs
26044/26063/26073/26109 are active with zero restarts, and all runtime binary
version reports agree on `9bd7e94…`. Live workload qualification is still a
separate claim.

The immediate post-activation audit is wholly empty and the new daemons remain
stable. The exact-revision live suite therefore starts from a proven clean
boundary.

The first suite command stops at `cd`: the deliberately root-only extraction
is not traversable by the qualification user. No test command ran. The runner
will be copied alone to a temporary executable, digest-verified against the
root-owned source, and then run under the required ordinary identity.

The hash-verified runner executes non-root and again proves ctr `RUNNING`, but
Docker receives the same maskedPaths rejection on exact `9bd7e94…`; cleanup
runs. This falsifies the assumption that adding only the runc-observed missing
path fully described the Multikernel-bound request. Capture of that exact list
is required before another policy change.

Independent inventory confirms zero survivors in every category and stable
new-revision daemons. The repeated rejection is fail-clean and isolated to the
policy input/agent boundary.

The precise error ordering identifies the rejecting layer: host Python emits
“duplicate or unsupported,” whereas the agent emits “unsupported or
duplicate.” `9bd7e94…` changed only the latter. The host allowlist still omits
`/proc/interrupts`, so projection never reached the repaired agent. The two
bounded enforcement sets must be corrected and tested together.

The host set now gains only `/proc/interrupts`, and its tests pin the full
Docker policy plus duplicate rejection. Python compilation, the focused
80-case fail-closed suite, and diff checking pass. Full gates and a newly
stamped exact revision remain separate.

Full Go race/vet and the entire documentation, schema, evidence, deployment,
80-case OCI, supporting-runtime, final-audit, and diff chain pass. Only the
known local socket-permission skip remains, and generated cache is removed.
The synchronized boundary is ready to freeze and qualify live.

The new immutable checkpoint is
`d0c33cc3e7b28fae45056fe58c88a3773384037d`, with source archive digest
`67dd2201…`. Exact guest transfer, rebuild, support deployment, activation,
and workload proof remain unclaimed.

Guest digest/extraction/bootstrap checks pass, followed by a complete exact
build. Release/shim/daemon/network-daemon/agent/initramfs hashes are
`fb85f3ce…`/`610a9b0e…`/`d8d66197…`/`2dc31c1f…`/`f3a6c2c3…`/
`666e30c3…`; required initramfs members are present. Active state is unchanged.

Release `d0c33cc…` and new support deployment `dd3ce8cc…` are selected; the
deployed validator matches exact source at `70a98070…`. Existing daemon PIDs
remain unrestarted and directly identify predecessor `9bd7e94…`, so activation
is still unclaimed.

Candidate manifest `83120879…` strictly validates with exact staged agent and
initramfs hashes. Every pre-activation inventory is zero and predecessor
daemons remain stable. The activation boundary is ready.

Exact `d0c33cc…` activation succeeds on unchanged boot. Active/rollback
manifest hashes are `83120879…`/`effb90a2…`; all four service PIDs are healthy
with zero restarts and all runtime binaries report the exact new revision.
This is coherent activation, not yet workload proof.

Immediate post-activation inventory is zero everywhere and the new daemons
remain stable. Live qualification can start cleanly.

The hash-verified runner proves ctr `RUNNING` and Docker passes the repaired
root-path policy. The next rejection is
`mounts[4].source must be an absolute canonical bounded path`; cleanup runs.
This is positive live proof for the synchronized fix, but not a suite pass.
Exact indexed mount observation and cleanup audit are next.

Independent inventory is empty in every category and exact daemons remain
stable, establishing fail-clean behavior at this boundary.

Runc inspection maps rejected index 4 to the exact read-only cgroup mount
`cgroup -> /sys/fs/cgroup` with `ro,nosuid,noexec,nodev`. Later indices are
three Docker `/etc` binds and are not conflated with this boundary. Whether
the cgroup entry is inert host-container metadata depends on the dedicated
child's observed cgroup view.

The exact child exposes only an empty mode-0555 `/sys/fs/cgroup` directory and
no cgroup/cgroup2 mount. Thus Docker's host-side read-only cgroup mount can be
consumed only in its exact fixed-default form and omitted from projection,
leaving the child strictly less exposed. Any modified form remains fatal.

The initial focused test exposes a stale oracle: runtime projection correctly
consumes the new exact default, but the expected-output filter treats it as a
bind because `/sys/fs/cgroup` is absent from its hardcoded default set. The
oracle, not policy behavior, requires correction and rerun.

The corrected oracle passes the expanded 82-case suite: exact read-only
cgroup metadata is consumed, while a writable variant remains fail-closed.
Compilation and diff checks pass; this is still local evidence only.

Full race/vet and documentation/schema/evidence/deployment qualification pass
with 82 OCI cases. The known socket-permission skip is unchanged and generated
cache is removed. An immutable revision and live rebuild remain required.

The immutable checkpoint is
`0a6668e9a9699ec8373321c347dc778a86038e7d`; its source archive hashes to
`2ae8239c…`. The VM remains clean on predecessor `d0c33cc…`; transfer, exact
rebuild, support activation, and workload proof for this checkpoint are not
yet claimed.

Resumed inspection confirms unchanged boot, active `d0c33cc…`, stable
unrestarted daemons, and a completely empty runtime-resource inventory. The
`0a6668e…` exact build therefore begins from a revalidated clean host.

The independently verified exact build completes with release/shim/daemon/
network-daemon/agent/initramfs hashes `964260e0…`/`2b8cbc9b…`/`a5a7d9b2…`/
`d257448c…`/`b2acc4c5…`/`e9f41b5d…`, and required initramfs membership is
proven. Active state has not changed.

Binary release `0a6668e…` and changed support deployment `7a2e5a67…` are
selected with validator hash `74dc8f63…` matching exact source. Existing
daemon PIDs remain stable and identify predecessor `d0c33cc…`; activation is
still separate.

The exact change is commit `1f81cb2aec7f4774c89506c71eb8348c37147e9e`;
its 1,017,425-byte archive hashes to `a06697189cd12606167011745b7b628663f9a090ac7005ea19e796d7734b50f0`.
Living findings and retained evidence remain outside it. Guest build,
deployment, and replay are unclaimed.

The guest independently verifies and builds exact `1f81cb2…`; all 16 CPUs are
back online. Exact manifest/shim/mkruntimed/mknetd/agent hashes are
`bde83869…`, `d1888952…`, `e10bbdc6…`, `edaf5c59…`, and `c79210c6…`.
Because this spans shim admission, the rootfs validator, and guest agent,
binary selection alone is insufficient; support generation and initramfs must
be staged coherently before replay.

Candidate `747eb832…` strictly validates with exact staged artifact hashes;
all pre-activation inventories remain zero and predecessor daemons are stable.
The activation boundary is ready.

Exact activation succeeds without reboot: active/rollback hashes are
`747eb832…`/`83120879…`, all services are healthy with zero restarts, and all
runtime version reports identify `0a6668e…`. Live workload proof is next.

Post-activation inventory is wholly empty and the new daemons remain stable.
The live suite has a clean starting boundary.

Live `0a6668e…` qualification clears cgroup mount validation and next rejects
Docker's `/dev/shm` default solely because it spells 64 MiB as
`size=67108864` rather than `size=65536k`. No broader mount drift is observed
at this boundary. Cleanup runs; only the two exact byte-equivalent forms may
be accepted.

Independent inventory confirms zero survivors and stable unrestarted exact
daemons, so the `/dev/shm` rejection is fail-clean.

With explicit `--read-only`, Docker emits all three managed `/etc` binds as
`rbind,rro,rprivate` and marks the root read-only. Recursive `rro` is stronger
than `ro`, so accepting exactly one marker and retaining the existing
sanitized read-only guest projection preserves the security boundary. The
qualification command will make this intent explicit.

The initial focused 86-case behavior run passes, while `git diff --check`
finds one trailing fixture space. That formatting defect is corrected and the
focused gate rerun rather than treating the partial chain as green.

The corrected 86-case suite and full Go race/vet, documentation, schema,
evidence, deployment, supporting-runtime, final-audit, syntax, and diff chain
all pass. Only the known local socket-permission skip remains. The combined
compatibility contract is ready for an immutable revision.

The immutable revision is `1dd73eb091325a7983931c4362fcddbeb350fa4c`; its
source-only archive hashes to `338429c3…`. Transfer, exact build/deployment,
activation, and workload execution are not yet claimed.

The exact guest build passes archive, ownership, exclusion, bootstrap, and
membership checks. Release/shim/daemon/network-daemon/agent/initramfs hashes
are `c4000e65…`/`350e55ca…`/`3eff3264…`/`376d383b…`/`851e3bd4…`/
`1c765dee…`. Active state is unchanged.

Release `1dd73eb…` and deployment `bc0517cc…` are selected with exact
validator hash `830353a5…`. Candidate manifest `4cb7ad22…` validates, every
pre-activation inventory is zero, and predecessor daemons remain stable.

Activation succeeds without reboot. Manifest `4cb7ad22…` is active,
`747eb832…` remains rollback, all four services are healthy/unrestarted, and
all binary identities agree on `1dd73eb…`. Live workload execution is next.

The exact runner (`15e12e19…`) gets ctr to `RUNNING` and Docker past the full
OCI compatibility chain. Docker then tries to bind `/proc/0/ns/net` and fails,
showing the shim exposed PID 0 instead of a host-visible namespace owner.
Cleanup runs. This is a deeper task-service/network-namespace integration
boundary, not a validator failure or suite pass.

Independent audit finds the runner did not clean: one default task/container,
child, storage/lifecycle/network/rootfs records, firewall state, two shims, and
a relay survive. Moby is empty and services remain stable. This is a cleanup
defect, not merely a failed create; exact state must be captured before bounded
recovery.

The survivor is exactly stopped default task/container `mk-proof-ctr`, sandbox
`mk-mk-proof-ctr-592a31dcfc440cd2` generation `4edb8056…`, with matching
runtime records and no moby object. Journal timing supports cleanup racing the
still-unwinding Docker create. A post-termination delete retry is bounded to
that known test ID.

The bounded retry reproduces the defect: forced task removal reports failed
precondition, and container removal fails at `close guest network: INTERNAL:
agent operation failed`; all resources remain. This is now current exact-
revision evidence for the previously open cleanup bug, requiring code-level
diagnosis before destructive recovery.

The exact suite now clears builder and daemon manifest approval, creates and
launches the child, then fails at network readiness because expected
`/sys/class/net/mktun0` is absent. Cleanup ran. This proves the generation-
pinning fix live but exposes the next CNI/TUN attachment boundary; no workload
pass is claimed before resource audit and network diagnosis.

Post-failure inventories prove cleanup complete: no task/container, child,
`mkv*` link, named namespace, endpoint state, rootfs record, or active storage
state remains. The network daemon's preceding `CHECK` successfully inspected
`mktun0` with `ip` inside the endpoint namespace. The later attach helper used
`/sys/class/net/mktun0` after only a network-namespace `setns`; because sysfs is
still the host mount namespace's view, that pathname is not a sound
namespace-local existence test. The correction will query the interface with
a socket ioctl on the locked, namespace-entered thread before attaching the
TUN descriptor.

The disposable host then demonstrated the mechanism in a self-cleaning probe:
a temporary namespace's `mktun0` was visible to `ip` under both `ip netns
exec` and network-only `nsenter`, but `/sys/class/net/mktun0` remained absent
under the latter. Namespace deletion was verified at probe exit. This is direct
live evidence that the sysfs test, not TUN creation, caused the attachment
failure.

The proposed exact kernel query also passed live before installation: after
entering another temporary namespace, an `AF_UNIX` datagram descriptor with
`SIOCGIFFLAGS` found `mktun0`, and cleanup again left no namespace. Its focused
local race test and vet pass. This keeps lookup namespace-local without
requiring an IPv4 socket solely for a device ioctl.

Containerd 2.2.2 also emits an independent runtime-discovery warning because
it invokes the shim with `-info` and the current CLI rejects that flag. The
normal shim starts despite the warning, so it is not causal for this failure,
but it is now an explicit open compatibility item rather than omitted evidence.

The compatibility path now intercepts exact `-info` invocation before the
older shim runner, consumes a bounded serialized option `Any`, and emits the
containerd `RuntimeInfo` protobuf with exact build version/revision and echoed
options. Malformed or over-one-MiB input fails before output. No generic runc
feature document is copied: unsupported or unproved feature advertising would
be worse than an absent optional document. Focused race tests and vet pass for
this path together with the namespace-local TUN correction.

The full local qualification chain then passed on 2026-09-26: all-package Go
race tests, vet, the complete documentation/schema/evidence and boundary-test
matrix, final evidence audit, and clean diff checking. The expected local
socket-permission skip still requires its privileged guest execution. Commit,
exact archive transfer, activation, and live workload proof remain distinct.

Commit `cc56f9a238bbab800fdd0c8c76c091d1ba327eab` now fixes both boundaries.
The exact source archive is
`0b963bb9a3c200b50612831ef7e681a00600c0cc90edb20fb1f21ce7e3969b0e`;
it excludes repository metadata and the existing untracked evidence directory.
No guest behavior is inferred from the archive until transfer, rebuild, and
activation complete.

The guest independently verified and privately unpacked that archive, then
completed the full exact build: manifest `6c7fc927…`, shim `6a19fffc…`, daemon
`b6ae56bb…`, network daemon `bffc2dca…`, and agent `9356724f…`. These establish
identity only; the preserved cleanup has not yet been retried.

After exact activation, cleanup-only `delete` no longer panics and removes the
child/network resources. It is not complete: mkruntimed enters a restart loop
while recovering the remaining rootfs record because the containerd bundle's
normal `work` symlink triggers `bundle may not contain symlinks`; the storage
image remains. No remote cleanup process remained when the stale SSH wrapper
was interrupted, and no successful delete response is claimed. The daemon
loop will be stopped before inspecting the exact record.

Guest-side digest verification and a private all-root-owned extraction passed,
followed by a complete exact-revision build. The release manifest is
`ee93424a…`; key outputs are shim `8bd0608d…`, daemon `d2dbc17b…`, network
daemon `562dc51e…`, and agent `f7342e65…`. These hashes precede installation;
they establish build identity, not active behavior.

The binary manager activated the exact `cc56f9a…` release and verified every
managed link. Containerd 2.2.2's native runtime inspection now succeeds and
decodes the exact name/version/revision with null options and no unproved
features. A first combined deployment/service-restart attempt was rejected by
the local shell due to a nested quote, before remote execution; it changed no
guest state and provides no activation evidence beyond the binary-manager
operation already observed.

The corrected retry passed with deployment `ca7bc745…` unchanged and fully
managed. After proving empty container inventories, the runtime daemons and
containerd were restarted; all relevant services, including Docker, remained
active after five seconds. Both daemons identify exact `cc56f9a…` code,
containerd preserves `runc` as default, native runtime inspection succeeds,
and the new journal contains no runtime-info load warning. Workload behavior
remains a separate proof boundary.

The live suite now clears the fixed namespace-local TUN attachment and reaches
agent transport setup. It then fails because configured host helper
`/usr/local/libexec/multikernel/mkvsock-relay` is absent. Cleanup ran. Thus the
TUN correction is observed live, while the full workload claim remains open
pending a clean inventory and exact relay provenance/installation.

The cleanup inventory is empty across clients, children, links, namespaces,
network/rootfs state, and active storage. The qualified host already contains
root-owned static relay `/opt/mkruntime/bin/mkvsock-relay` at hash `293ff1ea…`,
and the approved kernel manifest pins that exact path and digest. The failure
comes from the shim's mismatched `/usr/local/libexec` default. The correct
remediation is to use the already approved `/opt` artifact, not install an
untracked duplicate.

The default is corrected to that approved `/opt` path while retaining an
explicit `MK_RELAY` override. Regression coverage binds both behaviors; the
shim race suite, focused vet, and diff check pass. This is not yet an immutable
or guest-active fix.

The entire local race/vet and documentation, schema, evidence, runtime
boundary, deployment, and final-audit chain now passes. The one locally
permission-gated socket case remains a guest check. Immutable commit and exact
source guest activation still precede any new live claim.

The fix is committed at `f89dd00b804bdac4ac839baa8736d036f17aca65`.
Its clean source archive is
`77d8fc3231275cd7418934dfc15c8222b6f7d9daab1e6c88773faa7483c2075a`;
transfer, rebuild, activation, and workload rerun remain distinct evidence
boundaries.

Guest digest/ownership/exclusion checks and the full exact build passed. The
new manifest is `ca4c3c08…`; shim `45d7f740…`, daemon `2fcd2b3a…`, network
daemon `58fd7513…`, and agent `db46bac6…` are the exact outputs. No activation
or behavior is claimed from build hashes alone.

Exact release `0.1.0-dev-f89dd00…` is now active with fully managed links.
The runtime binaries identify that revision, service health remained active
across five seconds, containerd's inspection passes, and the `/opt` relay
still matches its approved digest. systemd emitted a unit-source-change warning
requiring `daemon-reload`; it will be cleared before workload execution rather
than ignored.

After `daemon-reload`, all four services remained active across a three-second
check and the runtime daemon PIDs were stable (`22235`, `22217`). Activation is
therefore clean for the next exact-source suite.

With exact `f89dd00`, relay execution succeeds and the suite reaches agent
connection. After waiting at that boundary, the shim closes and ctr returns
`ttrpc: closed`; its cleanup trap runs. The relay correction is live-proven,
while workload success remains open pending resource and multi-layer log
evidence for this new readiness/liveness failure.

Containerd captured the exact panic: `stopRelay` called
`(*unixsocket.Path).Remove` on a typed-nil interface. The capture function had
stored its converted nil pointer before handling `os.ErrNotExist`, causing
subsequent retries to skip capture and timeout cleanup to dereference nil.
Dead-shim deletion then timed out. Resource audit consequently finds the
halted child, veth, namespace, relay, and storage image preserved despite empty
client inventories. Both initial and recovery capture sites must publish the
owner only after success, followed by explicit reconciliation evidence.

The held bundle/recovery record and daemon agree on sandbox/generation,
bundle inode, endpoint generation, storage task/digest, and `RUNNING` state.
The orphan relay is PID `22955` with the exact approved executable and expected
generation-qualified argv. Cleanup can therefore terminate that exact process
and invoke authenticated shim recovery rather than deleting resources by
unbound names.

Relay identity was rechecked immediately before PID `22955` was terminated.
A first manual shim cleanup correctly failed closed because its cwd was the
ordinary user's mode-0750 home, not the root-owned held bundle. It performed no
daemon/network cleanup. The retry must reproduce containerd's bundle cwd.

The correct-cwd retry found another typed-nil panic: a failed recovery dial had
published `(*agent.Client)(nil)` into the interface before its error defer.
It also confirms an architectural cleanup issue: the `delete` action cannot
require successful guest recovery before entering authenticated `Cleanup`.
Failed dials now leave the agent interface nil, and strictly parsed terminal
`delete` invocations bypass recovery/event replay while retaining bundle-bound
cleanup checks. The focused shim race suite and vet pass; live cleanup awaits
an immutable deployment of these changes.

The subsequent all-package race/vet and complete documentation, schema,
evidence, runtime-boundary, deployment, and final-audit gate passes. The one
local permission-gated socket case remains for the guest. The preserved live
failure will be cleaned only with an immutable exact build.

Commit `0214f9727baecc94e46c675666477832afdc3cb4` now makes that cleanup path
immutable. The clean source archive is
`27481fec9baae7e83db1fb47845356467d3e73fb546f9ed29d02b0fdfd1ab4f7`;
the preserved failure remains untouched until guest verification, build, and
activation complete.

The exact release's cleanup-only delete then avoided both typed-nil panics and
removed the child, veth, namespace, and containerd bundle, but left the rootfs
storage generation and recovery record. The daemon restart loop reported
`bundle may not contain symlinks`. Source inspection disproves the initial
standard-`work`-symlink explanation: `NewService` calls `validateRequest`, and
its `EvalSymlinks` fails because the recorded bundle is already absent; that
failure is folded into the same generic message. Startup consequently blocks
before `Reconcile` can use the recorded storage identities to retire the
orphan. This is a partial cleanup, not a successful delete.

GCE state, persistent journals, and the serial console also establish that
this cleanup reset the primary guest. The instance remained continuously
`RUNNING`, while its boot ID changed from `e22e4b51…` to `ce405359…`.
Cleanup's SSH session began at 02:13:18 UTC, veth/netns removal was logged at
02:13:18.99, and UEFI started at 02:13:19.13, with no intervening orderly
shutdown, panic, or cloud stop/start. This is a distinct host-safety failure
in the child-stop boundary and must be traced before the operation is retried.

The lifecycle snapshot and journal retain the interrupted transaction exactly:
sequence 23 is a `StopSandbox` intent, state is `STOPPING`, the matching
storage export is `ACTIVE`, and the reset left the Kerf instance absent.
Recovery must therefore release and offline-check that exact export before it
commits lifecycle `ABSENT`; rootfs orphan cleanup is authorized only after the
lifecycle owner map no longer claims the image.

Race-enabled focused Kerf, lifecycle, and rootfs tests now pass for that
design. Pinned Kerf inspection shows `--force` expressly accepts `loaded` and
issues its force-halt reboot command, matching the live reset boundary. The
adapter no longer invokes force-kill: an already-loaded instance returns
success, while observed `RUNNING` uses non-force kill. Kerf's non-force guard
accepts only `active`, so a transition to `loaded` before its syscall is safely
rejected and then accepted through post-observation. An absent backend during
an interrupted stop releases and
offline-checks its exact storage generation before committing `ABSENT`.
Missing-bundle rootfs recovery then preserves a matching owner or, once
unowned, removes only the identity-bound storage directory without a pathname
mount operation.

The complete repository race suite, `go vet ./...`, documentation/link/schema/
evidence and runtime-boundary suites, deployment checks, final evidence audit,
and `git diff --check` then passed on 2026-09-26. The expected local
socket-permission skip remains for the privileged guest. This closes the local
checkpoint only; exact immutable deployment and recovery of the preserved live
failure remain open.

The corrected non-force stop group passed 100 race-detector repetitions, then
the complete race suite, vet, documentation/evidence gate, and diff check all
passed again. Immutable commit and guest execution remain next.

Commit `d0bff83845442883e8ebe9ef1e3b477c94defc83` freezes this recovery
checkpoint. Its source-only archive SHA-256 is
`1306898b584fa450937da448ad32e000e38dac750bbeda5927ac08a4965c5391`.
The disposable-host recovery result remains open.

The disposable guest reverified that archive, extracted it root-owned without
repository metadata or the local evidence tree, and completed the exact build.
Release manifest `4fb83d19…` binds mkruntimed `b1a0f516…`, shim `80bf3a28…`,
mknetd `a2dbf46d…`, and agent `60f711ec…` to revision `d0bff83…`. No live
recovery claim is made before activation.

On empty live inventories with mkruntimed still inactive, the binary manager
activated exact release `0.1.0-dev-d0bff83…`; all command links are managed and
the daemon and shim report the exact revision. Recovery execution remains the
next checkpoint.

The exact-daemon recovery start passed the formerly blocking rootfs
constructor, then failed closed in storage reconciliation with `storage image
digest differs from prepared identity`. Four systemd attempts were stopped;
the lifecycle intent, rootfs record, and storage directory remain preserved.
The current recovery code incorrectly treats the initial whole-image digest as
immutable after writable guest use. Durable inode/generation authentication
and quiesced filesystem validation must replace that pre-run digest comparison
before recovery can proceed.

Storage validation is now explicitly phase-bound: pristine SHA-256 remains
mandatory for provisioning and `PREPARING`, while `ACTIVE` restart authenticates
the recorded inode/path and checks allocation, clean ext4 state, UUID, inode
capacity, and block capacity without the obsolete pristine digest. A real ext4
changed-byte control, active-restart selection, and inode-replacement rejection
passed 100 race-detector repetitions. Full-tree and live checks remain open.

The subsequent all-package race suite, vet, complete documentation/evidence
gate, final audit, and diff check passed. Exact-source guest recovery remains
the outstanding boundary.

Commit `265196dac56713758c1c0a8a24e5a4f21d40784e` freezes the correction. Its
clean source archive hashes to `39a9ca5d…61406`; guest execution remains open.

The guest reverified and root-unpacked that archive and completed the exact
build: manifest `b939e8da…`, daemon `d9f57820…`, shim `cce5baa0…`, network
daemon `67ec4807…`, and agent `84164eb0…`. Activation is still unclaimed.

The exact `265196d…` release is now active on empty external inventories, with
daemon and shim reporting that revision. Recovery replay remains unclaimed.

One replay orchestration call was rejected locally for account usage limits
before reaching GCE; it provides no guest evidence and changed no claimed
state. Explicit live-suite authorization was then renewed.

The retried replay did reach GCE on 2026-09-26. After starting the exact
`265196d…` release and observing it for 15 seconds, systemd reported `active`,
PID `35224`, zero restarts, `Result=success`, and main-process status zero. Its
journal records successful runtime-storage mount validation and no daemon
error. This is an initial process-health checkpoint only: sequence-23
reconciliation and the external inventories still require direct inspection
before the interrupted cleanup can be called recovered.

Direct state inspection closes that narrower question. The boot ID remained
`ce405359…`, PID `35224` remained stable for five seconds, lifecycle has no live
sandboxes, and the existing sequence-23 cleanup result records generation
`0ba4a966…` as `ABSENT`. Storage generation `33a8740c…` is `RELEASED` with a
clean offline-check digest, while the rootfs record map is empty. Sequence 23
is expected: replay completed the durable intent already in progress instead
of opening a new transaction. External inventories and pathname/process
residue remain to be inspected independently.

The first residue pass found the exact task storage directory absent, empty
containerd task/container and Docker inventories, no named network namespace,
and no matching task link. Only the expected daemon process was displayed.
`/sys/kernel/multikernel/instances` is absent on this boot and is recorded as
such, not rephrased as an empty instance directory. The initial
`pgrep mk-storage-server` probe exceeded Linux's 15-character process-name
limit and is therefore inconclusive; full-command-line process inspection is
required before claiming no storage server remains.

The full-command-line retry displayed only `mkruntimed`; it matched because the
daemon arguments contain the configured `mkvsock-nbd` pathname. No process was
shown listening on port 4061. `mk_transport` is loaded and daemon health remains
active/running with zero restarts and no startup warnings. Sysfs discovery
corrects the earlier path assumption: the live API is under
`/sys/fs/multikernel`, not `/sys/kernel/multikernel`. Exact executable-name
inspection and enumeration of that tree remain open.

Exact process-name and `/proc/*/exe` checks then found no `mkvsock-nbd`
process. `/sys/fs/multikernel` contains only its control surface and empty
`instances` and `overlays` directories. Together with the absent exact storage
directory, port listener, task link, namespace, and container inventories,
this closes the live interrupted-cleanup recovery checkpoint on the unchanged
boot.

The next fresh-workload preflight correctly stopped at artifact coherence. The
active manifest still records agent `d244ef6c…` and initramfs `63c6ae5d…`, but
the exact `265196d…` build's agent is `84164eb0…`. The digest-verified archive
and root-owned source extraction remain on the disposable host. The workload
suite will not run against this mixed set; a coordinated exact agent,
initramfs, and private-manifest activation is required first.

The initial release-specific staging command produced a gzip-valid candidate
initramfs (`c3d2b9f0…`) and then failed before candidate-manifest creation: its
nested awk quoting let `$1` reach the remote `set -u` shell. No active manifest
or service changed. The staged pair remains available and validation must
continue using quote-free digest extraction.

The next retry extracted the digests but over-escaped jq's named variables;
the remote `set -u` shell rejected `ap` as unbound before writing the candidate
manifest. Active state again remained unchanged. A single remote-shell escape,
not two, is required around those jq variables.

The corrected construction and independent bootstrap validation passed. The
release-specific exact artifacts hash to agent `84164eb0…`, initramfs
`c3d2b9f0…`, and manifest `fce23175…`; all pinned kernel, relay, module,
compatibility, and feature checks resolved. The active manifest was still the
old `6bebfc81…`, and PID `35224` remained healthy with zero restarts, proving
candidate staging itself did not affect the running service.

Coherent activation passed after proving the container, task, Docker, and child
inventories empty. The manifest was staged and validated within its target
directory, then replaced on the same filesystem while the daemon was stopped.
Final validation reports manifest `fce23175…`, exact agent `84164eb0…`, and
initramfs `c3d2b9f0…`. The daemon restarted as PID `37028`, remained healthy
for ten seconds with zero restarts, and host boot ID `ce405359…` was unchanged.
This establishes coherent pre-workload artifacts, not yet workload behavior.

The exact `265196d…` host preflight passed. Host-check, daemon, and shim report
the full revision; all four required services are active. The report states
`qualified=true`, Kerf 0.2.0, 16 online CPUs, a ready APIC 8-15/16 GiB dry-run,
Secure Boot disabled, lockdown inactive, and no pool, child, stale resource,
or finding. The host is therefore qualified at an empty boundary; live
ctr/Docker behavior remains to be exercised.

The exact basic runner (`3eaa058b…`) passed preflight and started the first ctr
workload. Containerd connected to the shim at 14:22:54, then the client
returned `timed out connecting to child agent` after disconnection at 14:24:39.
The cleanup trap ran. Concurrent inspection found empty ctr task/container and
child inventories while daemon PID `37028` remained healthy with zero
restarts. The run therefore fails cleanly at agent readiness; it does not prove
the basic workload matrix, and cause remains open pending durable and console
evidence.

Durable and kernel evidence localizes the failure. New generation `4067a17f…`
reached CREATED, LOADED, and RUNNING; instance 40 received CPUs 8 and 10 plus
3 GiB and became active at 14:23:52. It halted at 14:24:00, long before the
agent timeout. Cleanup then completed through sequence 33, leaving no live
sandbox, rootfs record, active export, bundle, or storage directory. This is
an early guest bootstrap/readiness failure rather than host allocation or
cleanup. The exact guest console/relay error is still needed.

The first console diagnostic repeat stopped at a different boundary before a
child existed. The shim timed out reading `mkruntimed.sock`, while the polling
wrapper accurately recorded `CONSOLE_ATTACH_MISSED`. It supplies no guest
console evidence and is not another agent timeout; it is a separate
post-cleanup daemon-availability observation that requires direct process and
socket inspection before retry.

The daemon itself remained active with zero restarts, a live listening socket,
13 tasks, low current memory, no builder/server child, and no journal error.
Durable state is clean at sequence 36. Its 2.5 GiB peak memory and increased
CPU time are consistent with the second create continuing image construction
until cancellation and rolling back, not with a dead listener. The exact
sequence-34-to-36 result must determine whether the surfaced timeout is a
client deadline or a server fault.

Durable sequences 35-36 answer that question: generation `4b2416e0…` has only
a create intent/completion and the result `ABORTED: create canceled by runtime
shim`; it never loaded or started. The control-socket timeout was the caller
deadline during create, followed by complete server rollback, not a failed
listener. A further console diagnostic needs a justified request-time bound.

The journal timestamps make the bound defect concrete. Create began at
14:27:49.892 and committed CREATED at 14:28:19.286 (29.394 seconds), but the
shim daemon client expires every request at 30 seconds. The response therefore
lost a deadline race and cancellation released the otherwise successful
allocation. Rootfs preparation separately allows ten minutes behind that same
30-second client, proving a structural mismatch. Remediation needs
operation-specific finite bounds for rootfs preparation and create while
preserving the short default for ordinary calls.

The implemented fix copies and retimes only the concrete production daemon
client: 11 minutes for `PrepareRootfs` (ten-minute server builder cap) and 61
minutes for `CreateSandbox` (one-hour configured backend maximum). All ordinary
calls keep 30 seconds and caller cancellation remains authoritative. An
initial build exposed the service's injectable `daemon.Caller` abstraction;
the corrected helper passes fake callers through unchanged. The full shim
package now passes once under the race detector; broader verification remains.

The complete local race and vet runs then passed, together with the full
documentation/link/schema/evidence, 62-case OCI, bind/bootstrap, rootfs,
storage, image, release/deployment/ledger/capture/containerd, and final-audit
chain; `git diff --check` is clean. The expected locally permission-gated
socket subcase remains assigned to the guest. Generated Python cache was
removed and the pre-existing untracked evidence tree remains untouched.

The correction is immutable at
`cf88beb1c7d61769316fecf7bd33c1a188a5b4a6`. Its clean source archive hashes
to `01044ee9…8c` and excludes Git metadata plus the untouched untracked evidence
tree. Transfer, guest verification, exact shim rebuild, activation, and replay
are distinct remaining checkpoints.

Guest digest, root ownership, and source-only exclusion checks passed. The
first build wrapper stopped before compilation only because it tried to move a
pre-existing release manifest that the clean archive does not contain. Active
runtime state was untouched; the same verified tree can be built directly with
the explicit `cf88beb…` revision.

The exact build completed with release manifest `05b86f57…`, shim
`bf79230d…`, daemon `c3de5ae2…`, network daemon `fe31d09d…`, and agent
`056402d6…`, all stamped `cf88beb…`. Even unchanged agent source changes under
the new embedded revision, correcting the earlier assumption that its existing
initramfs could be retained. Exact agent/initramfs/manifest staging is required
before activation.

The release-specific `cf88beb…` candidate then validated: agent `056402d6…`,
gzip-valid initramfs `52ce2b0a…`, and private manifest `a3656ae8…`, with every
pinned kernel, module, relay, compatibility, and feature check intact. These
are staged identities only; no active-version claim is made yet.

The empty-host coordinated activation passed. Release
`0.1.0-dev-cf88beb…` and validated manifest `a3656ae8…` are active; all four
services remained active after ten seconds. Daemon PID `40581` and mknetd PID
`40562` have zero restarts, and daemon, network daemon, and shim report the full
revision. Installed shim is `bf79230d…`; host boot ID `ce405359…` did not
change. This is an exact pre-workload boundary only.

The exact live replay captured the missing cause. Generation `6cba5f75…`
booted the expected kernel with two CPUs and 3 GiB, attached NBD, mounted the
correct ext4 UUID, and printed `MK_STORAGE_BOOTSTRAP_READY`. Guest `/init` then
reported `mountpoint: not found`; the fallback devtmpfs mount found `/dev`
already busy, `set -e` exited PID 1 with status 255, and the child panicked at
7.835 seconds. The host later surfaced its agent timeout and cleanup ran. The
bootstrap includes a BusyBox binary but no `mountpoint` applet link, while
`runtime-mediated-init` invokes the bare command. This is direct console proof
for an initramfs construction/command-path bug, not an agent transport defect.

`mk-agent-init` now tests fixed inherited mountpoints through controlled
`/bin/busybox grep` on `/proc/mounts` and calls `/bin/busybox mount` only when
needed. It has no optional `mountpoint` dependency and preserves `/dev`,
`/proc`, and `/sys` moved by the mediated bootstrap. Regression checks bind
those command paths and verify the runtime builder installs the tested init.
Shell syntax, the complete 62-case OCI/builder suite, and diff checking pass;
the full local gate remains.

The full repository race/vet and documentation, schema, evidence, OCI, bind,
bootstrap, rootfs, storage, image, release/deployment/ledger/capture/containerd,
and final-audit chain then passed; the diff is clean. The expected local socket
permission skip remains reserved for the guest. Generated Python cache was
removed without touching the pre-existing untracked evidence directory.

The fix and live diagnosis are immutable at
`a209f7cb99863a0902afeff19806dbfe2d28d967`. Its clean source archive is
`5e3408f7…2103a`. Since the changed init is deployment-owned and embedded in
each runtime storage image, exact qualification needs both a new binary/
agent/initramfs set and a new managed deployment generation; replay against a
mixed generation would be invalid.

Guest digest, ownership, and exclusion verification passed, followed by a full
exact build. The release manifest is `18471bfc…`; shim `b24a4acd…`, daemon
`4ec33750…`, network daemon `086bdbfe…`, and agent `01c69ee3…` report
`a209f7c…`. No active behavior is inferred from these build outputs.

Coordinated activation installed binary release `0.1.0-dev-a209f7c…`, managed
deployment `a8a777e3…`, and validated manifest `6c0f897e…` on an empty host.
Every managed link verifies; installed init `edc9284c…` matches source and has
no `mountpoint` dependency. All four services stayed active for ten seconds,
daemon/network daemon had zero restarts, component revisions match, and boot ID
`ce405359…` is unchanged. This closes coherence, not the workload matrix.

The exact rerun progresses beyond the fixed panic but does not pass. The child
remains active, the ctr task reached CREATED, and `ctr run` then surfaced
`INTERNAL: agent operation failed`. Although the harness cleanup began, a
concurrent inventory still contained the active child plus ctr task/container;
both daemons remained healthy. The attached console has not returned, so this
is explicitly incomplete cleanup pending the guest markers and exact agent
error.

Console evidence confirms the fixed init reaches `MK_STORAGE_BOOTSTRAP_READY`
and `MK_AGENT_START` without panic. The first agent operation then fails;
cleanup closes NBD at guest uptime 24.598 seconds and the mounted filesystem
reports the resulting write errors. Containerd's delete retries fail at the
more precise boundary `close guest network: INTERNAL: agent operation failed`,
leaving the exact shim, relay, child, and recovery files intact. Those retained
identities must be inspected before targeted cleanup.

After revalidating and ending only the console collector, the wrapper returned
`RUN_RC=1`. Recovery binds generation `c247ea40…`, network `a2dfb03f…`, storage
`99c68361…`, bundle inode 6530, worker PID 43122, relay PID 43722, and socket
inode 6615; lifecycle remains RUNNING. The shim's authenticated dead-worker
cleanup can bypass guest replay, but process-group identity must be established
before terminating this failed shim.

The exact supervisor group and relay were revalidated and selectively
terminated. Containerd's dead-shim path removed the task, child, and all target
processes, but returned `STALE_COUNTER: network counters cannot decrease` while
persisting recovered-network status; container metadata remains. The recovery
file had zero counters even though mknetd had observed newer traffic/error
counters. This is a cleanup-reconstruction counter regression until the
remaining endpoint/rootfs/storage inventories prove its exact scope.

The audit bounds the defect: lifecycle sequence 55 has no sandbox, rootfs
record, active export, mknetd endpoint, namespace, link, firewall rule, or
runtime storage directory. Only taskless containerd metadata/bundle remains
because the cleanup process returned nonzero. Thus resource reclamation
succeeded; recovered counter reporting and final metadata completion need
correction.

Ordinary containerd metadata removal then emptied the container inventory, but
the failed task bundle remained due to the earlier nonzero dead-shim result.
Rather than deleting evidence, the exact inode-bound bundle will be moved to a
recoverable `/tmp` quarantine after confirming all runtime owners are absent.

After those checks, the bundle content was quarantined at
`/tmp/mk-proof-ctr-a209f7c-c247ea40.bundle` and the deterministic path became
absent. Because `/tmp` is on another filesystem, this was a copy/remove move:
the quarantine inode is 4252 rather than original inode 6530. Content remains
recoverable, but inode identity was not retained.

The rootfs mount backend previously validated snapshot paths and later reopened
them by name in the privileged mount operation, leaving a rename/substitution
window. The identity-bound mountpoint, bind sources, and every overlay lower,
upper, and work directory are now opened with no-symlink `openat2`, rewritten
to held `/proc/self/fd` references, and retained through the mount call.
Focused tests reject mountpoint replacement before the call, replace every
pathname inside an injected mount boundary, verify that all distinct original
inodes remain selected, and verify that descriptors close after return.
Pre-syscall failures are separately classified so rejection never unmounts a
substituted target, while an uncertain mount syscall still receives defensive
unmount. The runtime and per-task storage directories are also created
exclusively, identity-bound in durable state, and inherited by the bounded
builder. Bundle reads and artifact writes use child `/proc/self/fd` paths while
published metadata retains canonical logical paths. Focused replacement tests
prove writes remain on the originals, substitutes remain untouched, and
`PREPARED` publication is refused with recoverable ownership retained.
Prepared replay and reconciliation also verify manifests, metadata, allocation,
and digests through the exact recorded directory descriptors, then recheck the
names. A 100-repetition replacement test proves the originals are read while a
concurrent substitute is rejected and preserved. Recovery additionally binds
the initramfs and generated/source manifests to both generated and independently
verified digests, and requires the exact durable build result, canonical
`initramfs.path`, read-only-bind manifest, and storage metadata/image. Focused
mutations reject every formerly omitted artifact. Removal-time name races and
privileged disposable-host validation remained open at that checkpoint; the
removal race is addressed in the incremental result below.

## Incremental audit finding: 2026-09-18

The next boundary was recorded before implementation. After prepared rootfs
verification, Kerf `Load` reopened `.multikernel/initramfs.path` and the runtime
token by public pathname, then supplied the resulting initramfs pathname to
Kerf. The rootfs service now revalidates the complete prepared result and
returns the exact journal-bound runtime-directory and initramfs descriptors to
lifecycle. Kerf reads metadata and the token relative to that directory and
inherits the exact initramfs as fd 3. The backend hashes the same open archive
descriptor against both durable build-result digests before returning it, so
there is no verifier-to-load reopen. The approved kernel-manifest resolver's
former verify-then-return-path flow is also closed: bounded no-follow stable
reads retain the exact digest-verified kernel descriptor through lifecycle and
Kerf. Replacement tests swap both public directory and kernel names before
`Load`, prove the original inodes are used, preserve substitutes, and verify
descriptor closure. The combined kernel/rootfs/lifecycle/Kerf group passed 100
race-detector repetitions. GCE reported the disposable instance still
`RUNNING`; no restart, source upload, deployment, or new evidence retention
occurred.

Removal no longer performs an identity check followed by recursive deletion of
the public name. It atomically moves the child with no-replace semantics into
an identity-derived quarantine, verifies the opened and named inode, and walks
only beneath the held quarantine descriptor. An injected replacement between
inspection and rename is restored without modification, while a quarantine
left by a crash is resumed by recorded identity. Both cases passed 100
race-detector repetitions locally.

The same audit found that the shared private-file helper and its CNI/storage
callers still performed unconditional `unlinkat` after reading a pathname.
Private reads and exclusive publication now return their exact inode identity;
identity-conditioned removal moves that inode through a no-replace quarantine,
restores a raced substitute, and resumes a matching crash residue. CNI carries
the original cache identity across mknetd `DEL` and rejects a same-content inode
replacement. Storage cleanup retains the identities of its exact published
record and log. Focused safe-file, storage, same-content CNI replacement,
repeated `CHECK`, repeated `DEL`, and generation-reuse tests passed 100
race-detector repetitions. The quarantine name additionally binds the logical
filename hash so a restarted caller can rediscover exactly one residue; a
simulated restart with only a quarantined CNI cache reissued the exact
generation-bound `DEL` and removed it in that focused group. Storage
process-record reads also rediscover their exact quarantined inode after a
simulated restart in 100 repetitions.

Stale and shutdown cleanup in the shared Unix-socket owner now uses that same
no-replace exact-inode quarantine rather than check-then-unlink. The
privilege-independent replacement algorithm passed 100 race-detector
repetitions and restores the substitute intact. The real pathname-socket case
is still skipped under the local sandbox's listener restriction and remains a
required disposable-host execution.

A subsequent caller audit found that `mknetd` still used recursive pathname
creation for the socket parent and `mk-agentctl` still removed relay sockets by
pathname during reconnect. The former is being replaced with a root-anchored,
component-at-a-time `O_NOFOLLOW` walk that creates only missing directories and
does not chmod existing ancestors. The latter now captures the connected relay
socket identity before dialing, connects through the held parent descriptor
only while that identity remains published, and uses the shared no-replace,
exact-inode quarantine on both reconnect and final teardown. The exact parent
mode is set through the newly opened descriptor, so umask cannot weaken the
result and a raced pre-existing directory is not rechmodded. Parent creation,
root-path rejection, the privilege-independent removal algorithm, and both
affected command packages passed 100 race-detector repetitions. The real
captured pathname-socket dial/replacement test is still skipped by the local
listener restriction and remains mandatory on the disposable host; it was not
counted as proof. The subsequent full repository race suite, `go vet ./...`,
documentation/schema/evidence/deployment checks, and `git diff --check` all
passed locally on 2026-09-18.

The following raw-path audit found one remaining relay exception:
`removeStaleRelaySocket` still validated with `Lstat` and then called
`os.Remove`, allowing a removal-time replacement to be deleted. Running relay
ownership did not protect this startup cleanup path. Partial shim-launch
address/PID cleanup and the supervisor's worker-PID marker also still use raw
pathname removal; these are tracked as a separate file-publication boundary.
This paragraph was recorded before implementation. Stale relay startup cleanup
now no-ops only on an initially missing name and otherwise captures and removes
the exact safe socket through the shared quarantine owner. Unsafe-path
rejection and the privilege-independent removal algorithm passed 100
race-detector repetitions; the real safe-stale socket case remains skipped
locally and mandatory on the disposable host. Marker-file remediation remains
open. Containerd's atomic launch address/PID files are now captured immediately
as exact 0644 regular-file identities beneath a held no-symlink,
caller-owned, non-writable directory descriptor. Failure cleanup uses the
identity-conditioned quarantine. Focused tests remove the exact address after
a later PID failure and preserve a same-name substitute plus the moved
original. The supervisor's per-restart PID marker now has the same ownership
discipline: it recovers a bounded exact crash residue, publishes exclusively,
retains each published identity through the worker lifetime, and removes that
inode after exit. The descriptor opener preserves safe shared modes but rejects
group/other write. Its focused helper group and the combined shim
launch/replacement/restart/stale-relay group each passed 100 race-detector
repetitions. The subsequent full repository race suite, `go vet ./...`,
documentation/schema/evidence/deployment checks, and `git diff --check` all
passed locally on 2026-09-18; the real pathname-socket skip remains excluded
from the claim.

The next audit found that guest DNS ownership still used pathname-based
inspection, deletion, replacement, and restoration. A container process can
replace `/etc/resolv.conf` after configuration and have its inode deleted by
`CloseNetwork`; setup has an equivalent race for both supported regular and
symlink originals. Closing this requires retaining the parent descriptor plus
the exact original and agent-published identities, then using only
identity-conditioned removal and exclusive descriptor-relative restoration.
The finding was recorded before implementation. DNS ownership now holds a
no-symlink, caller-owned, non-writable parent descriptor; captures regular data
or a symlink target from a stable exact inode; removes that inode
conditionally; and exclusively publishes the exact-mode managed file.
Teardown conditionally removes that managed inode and exclusively restores the
original form or absence. A same-mode process substitute is preserved and
reported, and restoration succeeds after the exact managed inode is returned.
When setup has already mutated DNS but immediate rollback is blocked, the
manager retains the descriptor and cleanup state for a later `CloseNetwork`
retry. The public regular/symlink helper group and the DNS
regular/symlink/absent plus replacement group each passed 100 race-detector
repetitions. The subsequent
full repository race suite, `go vet ./...`, documentation/schema/evidence/
deployment checks, and `git diff --check` passed locally on 2026-09-18.
Disposable-host validation remains pending.

The allocation audit next found that `MK_SHIM_LOCK` was opened with ordinary
pathname `O_CREATE|O_RDWR`: it followed symlinks, did not validate the object,
could be replaced to split serializers across inodes, and used a blocking
`flock` that ignored caller cancellation. The intended fix is a no-symlink,
caller-owned, non-writable parent descriptor used as the lock object itself,
with nonblocking retries governed by the caller context. This was recorded
before implementation. Allocation now locks that descriptor with nonblocking
context-aware retries and never opens the configured filename. A focused test
proves bounded cancellation under actual contention, preservation of a
configured symlink target, reacquisition after close, and group-writable-parent
rejection in 100 race-detector repetitions. The subsequent full repository
race suite, `go vet ./...`, documentation/
schema/evidence/deployment checks, and `git diff --check` passed locally on
2026-09-18. Disposable-host validation remains pending.

Bundle identity is the next confirmed boundary. `validateServiceIdentity`
currently uses `EvalSymlinks` and `Stat`, but the service stores only the path;
subsequent config/recovery/state operations and the rootfs daemon reopen it.
Those consumers bind the inode they independently open, not necessarily the
one containerd assigned when the shim service was created. A caller-owned
same-mode directory swap can therefore cross either the service/RPC or
shim/daemon handoff, and supervisor restart uses the public working-directory
path again. This is recorded before implementation. Complete closure needs a
held service-lifetime bundle identity, propagation and verification at the
rootfs boundary, durable restart binding, and handoff replacement tests.

The first focused implementation run on 2026-09-18 passed
`internal/safefile` and `internal/rootfs`, including the new rootfs bundle
identity handoff rejection. The shim package did not pass: existing unit
fixtures generally inherit the environment's `0775` temporary-directory mode,
while the new held-bundle opener deliberately rejects group/other-writable
directories. The resulting persistence failures cascade into monitor
timeouts. This is recorded before fixture repair and is not counted as a
passing shim result or as evidence of complete bundle-identity closure.

After changing only bundle fixtures to the existing private-directory test
helper, `go test -count=1 ./cmd/containerd-shim-multikernel-v2` passed in 2.737
seconds. This confirms that the earlier cascade was caused by obsolete fixture
permissions. Adversarial replacement, supervisor restart, repeated race,
full-repository, and disposable-host evidence are still pending.

The focused safefile/rootfs/shim group then passed together once. New
adversarial tests observed that conditional recovery-file exchange rolls back
without deleting either a raced public substitute or the displaced original;
a worker launched after public bundle replacement has the held bundle inode as
its actual cwd and receives the same device/inode/UID handoff; incomplete or
mismatched supervisor handoff is rejected; and a persisted bundle mismatch is
rejected before any daemon RPC. Repeated race execution, descriptor-lifetime
cleanup, durable binding across a completely new supervisor, and
disposable-host proof remain open.

Successful Task shutdown now releases the held bundle descriptor through a
one-shot close, before invoking the shim shutdown callback. The focused
shutdown test passed and observed the closed-file error on a subsequent
descriptor operation. This closes the in-process descriptor-lifetime leak; it
does not solve authoritative identity discovery by a brand-new supervisor.

Authoritative new-supervisor binding is now carried in the daemon's journaled
`SandboxConfig` as bundle device/inode/UID. Lifecycle validation rejects an
absent identity, allocation copies it from the held descriptor, and recovery
compares the daemon-returned value before token loading or any network, relay,
or agent reconnect. The protocol/rootfs/lifecycle/daemon/shim package group
passed once. A focused mismatch test observed exactly one `ListSandboxes` RPC
and no resource reconnect. Schema validation, repeated race execution,
full-repository checks, and disposable-host proof remain pending.

`python3 scripts/check-runtime-schemas.py` passed after the contract change: 7
schemas and 22 fixture cases. The schema result proves structural contract
consistency only; it is not runtime or disposable-host evidence.

The conditional exchange tests, rootfs handoff replacement test, and shim
held-bundle/supervisor/recovery/shutdown identity group each passed 100
race-detector iterations. The shim group took 104.120 seconds because every
supervisor case launches a race-instrumented worker subprocess. This is
repeated local execution evidence; the full repository suite and authorized
disposable-host run remain pending.

`GOCACHE=/tmp/mklinux-gocache go test -race -count=1 ./...` passed across the
complete Go runtime repository on 2026-09-18 after the bundle changes. Vet,
documentation/evidence checks, diff review, and disposable-host proof remain
pending at this point.

A post-suite caller audit found that the direct G2 `CreateSandbox` harness
still emitted the pre-identity JSON shape. It now captures each prepared bundle
with GNU `stat` and sends that exact device/inode/UID. Lifecycle tests
explicitly reject a missing identity, and the shim plan now names recovery
format v3 rather than stale v2. These compatibility edits were recorded before
their verification rerun.

The compatibility rerun passed: `bash -n scripts/test-runtime-g2.sh`, the
complete `scripts/check-docs.sh` chain, and 100 race-detector iterations of
lifecycle input validation. The docs chain again covered 7 schemas/22 cases,
17 classified historical evidence manifests, OCI/bind/bootstrap/image,
release/binary/deployment/resource-ledger/containerd configuration, and the
final G4-G6 evidence-audit tests.

Diff review found two remaining descendants of the bundle boundary before
checkpointing: the event journal still reopened the public bundle path, and
`.multikernel` plus first token creation were not held across operations. A new
child-directory helper also needed a nil-safe `Stat` failure path. The event
journal is now read/replaced/removed relative to the held bundle; the exact
`.multikernel` descriptor is retained once opened, token creation uses
exclusive descriptor-relative publication, and shutdown closes both held
directories. The shim package passes once. Replacement tests observed no event
journal in a substitute bundle, no recovery file in a substitute runtime
directory, and the original recovery remaining at PID 7 rather than the
rejected PID 8 update. Repeated race and final full-suite checks remain
pending.

The guest independently matches matrix/wrapper/helper hashes
`fafc2f80…`/`f9c018c4…`/`484229a…`. The corrected focused Multikernel ctr replay
completes after roughly 70 seconds, reports exact guest sizes `ready:24 80`
then `resized:37 91`, and emits both `LIVE_RESIZE_PASS` and
`FOCUSED_MULTIKERNEL_LIVE_RESIZE_PASS`. This live-proves post-start terminal
resize and confirms the former 30-second deadline was invalid. Independent
teardown audit remains pending before the complete matrix.

Independent focused-replay teardown is all 13 inventories zero; mkruntimed,
mknetd, containerd, and Docker remain active/status 0 with zero restarts. The
complete matrix can therefore begin from a proven clean state rather than
inheriting focused-test resources.

The fresh complete ordinary-user matrix confirms host boot `d08895c1…`, all
four runtime services active, initial all-13-zero inventory, and shared BusyBox
digest `sha256:73aaf090…` on amd64. It then passes split create/start,
state/inspect, distinct child boot identities `4b1621c7…`/`0613920a…`, exec
stdout/stderr, private writable roots, mediated `/30` DNS/HTTP networking,
bidirectional sibling isolation, deliberate mkruntimed restart continuity with
unchanged child boots, pause/resume, signal/exit 42, normal deletion, and
post-delete all-13-zero convergence for ctr and Docker. Read-only-bind and
subsequent rows remain in progress.

Both read-only-bind cases pass: ctr and Docker read their exact client-specific
directory/file values, guest writes to both targets fail read-only, and all
four host sources remain byte-identical. The post-bind checkpoint converges to
all 13 inventories zero. Repeated nonzero-exit/name-reuse and subsequent rows
remain in progress.

Both repeated cycles return exact exit status 17 for ctr and Docker, retain
their client-specific stdout/stderr markers, clean normally, and successfully
reuse the same names in cycle 2. `foreground-wait-stdio-and-nonzero-exit` and
`name-reuse` therefore pass for both clients. Stdin forwarding and subsequent
rows remain in progress.

Guest stdin forwarding passes with exact outputs
`guest-ctr-stdin`/`guest-docker-stdin`. Detached-task attach also passes with
exact `ctr-attached-stdin`/`docker-attached-stdin`; both rows tear down
normally. Initial terminal allocation and post-start resize are now running
and remain unclaimed.

Initial terminal mode passes for both clients: ctr's classified leading `^@`
echo is narrowly normalized, while ctr and Docker each report exact `37 91`
plus their success marker. The ctr post-start resize independently reports
`ready:24 80`, `resized:37 91`, and `LIVE_RESIZE_PASS`. Docker's resize and the
combined/final matrix markers remain pending.

Docker independently reports the same exact `ready:24 80`→`resized:37 91`
transition and `LIVE_RESIZE_PASS`. The suite's final checkpoint is all 13
inventories zero, `post-start-terminal-resize` passes for ctr and Docker, and
the run emits `G4_G6_CTR_DOCKER_FEATURE_MATRIX_PASS`. This completes the frozen
full live matrix; an independent post-suite audit remains pending.

Independent post-suite audit confirms the same boot `d08895c1…`, expected
kernel, and system state `running`; support/binary selectors remain
`fe456acc…`/`1f81cb2…`. Effective unit, active kernel manifest, matrix, and
helper retain exact hashes `9d921d2f…`, `1d79c564…`, `fafc2f80…`, and
`484229a…`. All 13 inventories are zero. Guest agent, mkruntimed, mknetd,
containerd, and Docker are active/running with status 0 and `NRestarts=0` at
PIDs 1066/12806/1216/1445/1543. The final live matrix is independently closed
cleanly.

Gate-closure reconciliation keeps G4–G6 open: the matrix directly closes only
the shared-client lifecycle/I/O, post-start resize, mkruntimed-restart
continuity, distinct network identity/sibling isolation, and runtime-cleanup
slices. It does not prove G4 persistence/exhaustion/corruption/reset/clone, the
G5 UDP/MTU/load/fault/reconnect/spoof/primary-health matrices, or G6
containerd/Docker restart, forced-shim reconstruction, event replay,
FIFO/cancellation, and concurrent-churn requirements. A fresh schema-valid
mode-0600 resource-before ledger is retained at
`evidence/runtime-20260930/g4-g6-final-live/resources-before.json` (5,007
bytes, SHA-256 `fc6eb570…`), recording one running n2-standard-16 instance, two
disks, three snapshots, no addresses, and six firewall rules. The
evidence-grade rerun will emit markers only for the five proven slices.

The scoped markers pass Bash syntax, `git diff --check`, and the full
documentation/schema/evidence/runtime gate. Commit `f41211f2127b3414f503084e39f48d93dcb18512`
freezes matrix SHA-256 `00204de1…`; generated caches are removed. Exact guest
upload and retained transcript execution remain pending.

The immutable evidence capture starts at `2026-09-29T23:51:00.495900Z` in
`evidence/runtime-20260930/g4-g6-final-live/g6-shared-matrix.log`, retaining
exact gcloud argv. The guest verifies matrix hash `00204de1…`, reports boot
`d08895c1…`, starts with all 13 inventories zero, and confirms the shared
BusyBox index digest/amd64 image for ctr and Docker. Later rows and the capture
exit trailer remain pending on the same live process.

The retained run's lifecycle block passes with new distinct child boots
`f60ce340…`/`a8d7caf8…`, exact exec I/O, private roots, distinct `/30` endpoints,
DNS/HTTP, bidirectional sibling rejection, pause/resume, exit 42, and normal
deletion. Its deliberate mkruntimed restart changes PID 12806→22465 without
changing either child boot, and post-delete inventory returns all 13 counters
to zero. Bind and later rows remain in progress in the same transcript.

The in-guest reboot request did not change the boot ID, so no success was
inferred from it. An explicitly authorized GCE reset produces new boot
`d08895c1-0d19-4c66-ac7e-c5f77fd23451` on kernel `7.0.0-mk2-gce-lab`. The guest
agent enters active at monotonic 22,801,501 µs and mkruntimed starts at
24,201,242 µs, approximately 1.400 seconds later. The effective graph still
Requires/After the agent and no `GUEST_AGENT` failure appears. Agent,
mkruntimed, mknetd, containerd, and Docker are active/status 0 with
`NRestarts=0`; system state is `running`. Persistent selectors and
unit/matrix/helper hashes remain exact, and all 13 inventories are zero. This
closes the boot-order defect with live evidence.

The active kernel manifest is resolved from configured directory
`/etc/mkruntime/kernels` and still hashes exactly `1d79c564…`; the retained
rollback manifest is not selected. A focused wrapper for the formerly failing
ctr resize path passes shell syntax and hashes to `a5fd67a0…`; guest
verification/execution remain pending.

The guest independently matches focused-wrapper/helper hashes
`a5fd67a0…`/`484229a…`, but the focused Multikernel ctr replay still times out
before observing `ready:24 80`; no resize success is claimed. Trap cleanup then
runs for roughly one minute, showing the slow-create boundary is still
exercised. Independent all-13 inventory, shim/rootfs, and service auditing are
required before diagnosis or another run.

The independent audit is not clean: `runtime_artifacts=2`,
`rootfs_records=1`, and `shim_processes=2`, while every other inventory is zero
and all services stay active/status 0. The retained exact-release
supervisor/worker are PIDs 2526/2531, parented 1→2526, in one session, and still
alive after 169 seconds. They hold the deleted `.mk-resize-focused`
bundle/log/runtime directory; ctr metadata and the visible bundle are absent,
while rootfs remains `PREPARED`. This proves the 30-second helper deadline can
cancel a valid cold CreateTask and the 60-second cleanup deadline still removes
metadata despite an exact shim. Exact orphan recovery is required before retry.

After revalidating both exact PIDs and the worker's parent, SIGTERM to only
supervisor 2526 removes both shims through parent-death policy. Restarting
mkruntimed lets authenticated reconciliation remove the absent bundle's
PREPARED rootfs; the subsequent authoritative audit returns all 13 inventories
to zero and all five services remain active. The matrix now supplies an
explicit 180-second live-resize deadline. Cleanup tracks whether any task was
observed and, when none was, refuses metadata removal if the exact creating
shim survives its bounded poll. Bash syntax, `git diff --check`, and the full
documentation/schema/evidence/runtime gate pass with 95 OCI cases; only the
classified sandbox socket `EPERM` subcase is skipped and generated caches are
removed. Commit `3eaf828ecce07ee4d1f3eace88b11ee887c5be7c` freezes matrix
hash `fafc2f80…`; the focused wrapper hashes to `f9c018c4…`. Live retry remains
pending.

The full race, vet, documentation/schema/evidence/deployment, and diff gates
all pass with 75 OCI cases. The sole skip is the known local socket `EPERM`
subcase. Commit and exact live proof remain pending.

The immutable checkpoint is `f5d8da448f23710e3777ed011afaad003881877a`;
its source-only archive SHA-256 is
`164a1b9d6895a34253fd1b5a6b7107ae748ce62dd9eff523b7dd0e19114586eb`.
The historical untracked evidence tree remains excluded. Guest verification
is next.

Descriptor-relative token create/reuse, event-journal bundle replacement,
recovery runtime-directory replacement, and two-descriptor shutdown cleanup
passed 100 race-detector iterations as a combined focused group. This closes
the locally identified bundle-descendant handoff cases; current-revision
disposable-host validation remains unauthorized and pending.

Final semantic review found a rootfs-to-shim window between creation of
`.multikernel` and the shim's first held open. `PrepareRootfs` now returns the
exact runtime-directory identity on first preparation and replay, and the shim
verifies its held child descriptor before token creation. The focused rootfs
and shim packages pass, including a prepared-directory replacement rejection.
Repeated race and final full-tree verification are pending.

Rootfs prepare/replay runtime-identity return and shim prepared-directory
replacement rejection each passed 100 race-detector iterations.

On the final current tree, the complete repository race suite,
`go vet ./...`, `scripts/check-docs.sh`, G2 shell syntax, and
`git diff --check` all passed on 2026-09-18. This establishes the local
bundle-identity checkpoint. It does not replace the still-unapproved
disposable-instance execution and evidence collection.

Post-checkpoint audit after `52ea8a3` found that the containerd delete/reclaim
`Cleanup` path still read `.multikernel/sandbox.json` through a relative
pathname and trusted that local record before destructive daemon/network
cleanup. This finding is recorded before implementation. The path must reuse
held bundle/runtime descriptors and match daemon-journaled bundle identity
before stopping ownership.

Fallback cleanup now loads recovery through the held `.multikernel` descriptor,
requires its bundle identity to match the service handoff, and lists daemon
sandboxes to confirm the same ID, generation, and journaled bundle identity
before network, relay, sandbox, or rootfs cleanup. The focused fallback suite
passes: existing stop/rootfs failure propagation is preserved, daemon mismatch
performs only `ListSandboxes`, and public bundle replacement performs zero
daemon calls. Repeated race and full-tree checks remain pending.

The complete fallback-cleanup group passed 100 race-detector iterations after
descriptor and daemon identity binding.

The subsequent full repository race suite, `go vet ./...`, documentation/
schema/evidence/deployment checks, and `git diff --check` all passed locally on
2026-09-18. Disposable-host validation remains pending authorization.

The next fallback audit confirmed that containerd's `ReadAddress` plus
`RemoveSocket` path ultimately performs raw `os.Remove` on the socket named by
the bundle file. A same-owner address-file replacement could redirect cleanup
to another shim socket. This is recorded before implementation; cleanup must
compare the held address bytes to this task's deterministic containerd address
and remove only a captured socket inode.

Fallback socket cleanup now reads `address` through the held bundle descriptor,
parses a unique nonempty containerd `-address` invocation value, recomputes the
namespace/task-specific socket, requires byte-for-byte equality, and hands the
canonical path to the shared exact-inode socket owner. Redirected content never
reaches socket capture. Missing recovery can still remove this authenticated
startup socket without a daemon call. The combined focused
address/flag/fallback suite passes once; repeated race and full-tree
verification remain pending.

The deterministic address parsing, redirected-address rejection, exact-socket
owner handoff, no-recovery cleanup, and authenticated fallback group passed 100
race-detector iterations.

The subsequent full repository race suite, `go vet ./...`, documentation/
schema/evidence/deployment checks, and `git diff --check` passed locally on
2026-09-18. The local socket-ownership checkpoint is complete; live proof is
still pending the explicit source/evidence authorization.

The next startup audit found that `StartShim` still treated every
`EADDRINUSE` as stale, raw-removed the deterministic socket, and rebound it.
This can unlink a live shim endpoint, while a name replaced after the failed
bind can be deleted instead of the inode that caused the collision.
Containerd's grouping-aware manager preserves a connectable socket but still
uses a pathname probe/remove sequence. This finding is recorded before code
changes: the remediation must capture the exact socket owner, dial through
that captured identity, preserve a live listener, and conditionally remove
only an unchanged stale inode before retrying the bind once.

The focused implementation passes. Shim startup now binds with the shared
held-parent exact-identity listener in exclusive mode. An `EADDRINUSE`
collision captures and dials one inode: a live endpoint is preserved and its
capture descriptor released, a refused endpoint is conditionally quarantined
and rebound once, and a replacement aborts without a second bind. Error
rollback closes the identity-owning listener and performs no raw pathname
remove. Focused Unix-socket coverage also observed that a second exclusive
bind leaves the first listener connectable and that releasing a captured live
owner leaves its path intact. These pathname tests ran without a local skip;
repeated race and full-tree verification remain pending.

The startup-collision group and the Unix-socket exclusive-bind/release/exact-
cleanup group each passed 100 race-detector repetitions. The real pathname
cases were exercised rather than skipped. Full repository verification remains
pending.

The subsequent complete repository race suite, `go vet ./...`, the full
documentation/schema/evidence/deployment validator, and `git diff --check`
passed on 2026-09-18. The docs chain again validated 7 schemas/22 cases and 17
explicitly classified historical manifests. Its independent Python
socket-rejection subcase was still skipped by that sandbox; unlike it, the Go
pathname-socket cases in this checkpoint executed. Current-source live proof
is still gated on the explicit source-transfer/evidence-retention approval.

Final semantic review refined the probe before checkpointing. Captured-socket
dial now accepts the caller context, and only an observed `ECONNREFUSED` is
treated as authority to conditionally remove the captured stale inode. A
cancelled or otherwise failed probe releases the held owner without removal.
The added cancelled-probe case and both focused packages pass once; the prior
repeated-race and full-tree results must be rerun against this refinement.

The refined groups passed 100 race-detector repetitions, including an
integrated real-path test that preserves and reuses a live listener and then
conditionally removes and exclusively rebinds a closed stale listener. That
case ran without a skip, and cancellation remained non-destructive. Final
full-tree verification is pending.

On the refined tree, the complete repository race suite, `go vet ./...`, the
documentation/schema/evidence/deployment chain, and `git diff --check` all
passed on 2026-09-18. The separate Python socket-rejection subcase retains its
documented sandbox skip, while the integrated Go collision case executed.
This supersedes the earlier pre-refinement full-tree result and completes the
local startup-socket checkpoint.

A final cancellation-boundary case cancels after the captured dial reports
refusal and proves removal is never attempted. The helper also checks context
before initial bind and before rebind. Both focused groups passed another 100
race-detector repetitions; the preceding full-tree result must be rerun for
these last boundary checks.

The final post-boundary complete repository race suite, `go vet ./...`, the
documentation/schema/evidence/deployment chain, and `git diff --check` all
passed on 2026-09-18. This is the authoritative local result for the
startup-socket checkpoint; current-source live execution remains unclaimed
without the required authorization.

The following startup audit found one remaining publication window.
Containerd's atomic pathname helpers installed `address` and `shim.pid`, and
the shim only then reopened the public name to capture rollback ownership. A
same-owner replacement in between could become the recorded rollback target,
while a pre-existing entry was overwritten without proof. This is recorded
before code changes: each file must be exclusively published relative to a
held safe parent and return its exact created identity in the same operation.

Both launch metadata files now use descriptor-relative exclusive regular-file
publication. An existing name remains intact and aborts launch; a successful
create returns the precise inode retained for conditional rollback, eliminating
the capture reopen. The focused shim package passes, including exclusive
collision, raced replacement preservation, address cleanup after PID failure,
and the existing address-replacement case. Repeated race and full-tree checks
remain pending.

After daemon reload, systemd's effective `Requires` and `After` sets both
contain `google-guest-agent.service`; deployed and generation unit paths match
hash `9d921d2f…`. Support/binary selectors are `fe456acc…`/unchanged
`1f81cb2…`, and the frozen matrix/helper still match `5e850f3…`/`484229a…`.
All five relevant services are active and all 13 inventories are zero. The
retained `NRestarts=1` belongs to the pre-fix boot; clean reboot proof remains
pending.

The immutable checkpoint is `298d3debe832696e18c749dd1bbed4d03e64433b`.
Its source-only archive SHA-256 is
`9520d18ef58d88f0a82b48560ae87a9a5b0ae1fb988c069e59af432e766be2b5`;
the historical untracked evidence tree was not staged. Guest verification is
next.

The exact runner (`ac1d54a7…`) proves Docker passes all four newly addressed
fields, then stops at `only exact default resource contracts are supported`.
It exits 125 and runs cleanup. This is not a pass; cleanup and the precise
Docker resource object must be observed next.

Post-failure cleanup is complete across every audited inventory; daemons
18614/18633 remain active and unrestarted. The open issue is solely the exact
Docker resource shape.

Docker emits an ordered device-cgroup program plus inert empty `blockIO`, not
the accepted containerd default. Because the guest bind-mounts full devtmpfs,
stripping those restrictive rules would be unsound. The next experiment is an
explicit Docker device-cgroup allow-all opt-out; absent an effective allow-all
OCI result, child-side device enforcement is required.

The explicit device-cgroup allow-all probe yields Docker's fixed rule program
followed by a terminal all-device `rwm` allow and empty `blockIO`. Its effective
policy now matches child devtmpfs. Exact-full-object acceptance/removal is
therefore sound only with that final rule; the restrictive, modified,
reordered, or nonempty-blockIO forms remain rejected.

The exact unrestricted resource object and explicit qualification opt-out are
implemented. Focused validation passes 75 cases, including restrictive,
modified-terminal-rule, and nonempty-blockIO rejection, followed by Python
compilation, shell syntax, and diff checks. Full gates and exact live proof are
pending.

The guest independently reproduces archive hash `9520d18e…` on unchanged boot
`e76edca2…`. All workload, child, storage, durable active state, network, and
helper-process inventories are zero, while daemon PIDs 11328/11347 remain
stable with zero restarts. Build preconditions pass.

Exact root-owned build hashes are `f302816e…` (release), `33128b4c…` (shim),
`13513537…` (daemon), `db1a3dd1…` (mknetd), `5aa29a55…` (agent), and
`7b79a2a7…` (initramfs). Privileged inspection verifies the three required
bootstrap artifacts. Active state is still the prior revision.

Binary release `0.1.0-dev-298d3de…` and support deployment `6508b98f…` are
selected with valid links; deployed/source OCI validator SHA-256 is
`65f04894…`. Daemon PIDs 11328/11347 have not restarted, so filesystem
selection is not yet a coherent running release.

The root-only `298d3de…` agent/initramfs are staged at hashes
`5aa29a55…`/`7b79a2a7…`, and strict candidate manifest `50c74397…` validation
passes. An immediate clean-host audit still precedes activation.

The immediate audit is fully empty across every workload/resource/process
inventory, with daemon PIDs 11328/11347 stable and unrestarted. Coordinated
activation is safe on the current boot.

Exact `298d3de…` activation succeeds without reboot, retaining the a72
manifest for rollback. Active kernel/release/shim hashes are
`50c74397…`/`f302816e…`/`33128b4c…`; all selectors, version reports, strict
bootstrap validation, and five active services agree. New daemon/container
service PIDs 18614/18633/18643/18676 have zero restarts. Workload proof remains
next.

The exclusive publication, raced replacement, and partial-launch cleanup
group passed 100 race-detector repetitions. Full-tree verification remains
pending.

The subsequent complete repository race suite, `go vet ./...`, the complete
documentation/schema/evidence/deployment chain, and `git diff --check` passed
on 2026-09-18. The local exclusive launch-metadata checkpoint is complete;
current-source disposable-host proof remains unauthorized.

## 2026-09-26 resumed live qualification

The retained live evidence and current source agree on the cleanup-only
counter failure: recovery has a durable endpoint to release but no live agent,
pump, or TUN descriptor whose counters it owns. Reporting zero counters from
that state regresses mknetd's durable values. The implementation now skips
only that unowned final report and preserves ordinary final reporting for a
live network owner.

Guest network setup errors were intentionally secret-safe but too coarse to
qualify the next failure. Setup failures now carry one fixed, non-secret stage
through the wire redactor, while arbitrary command output, paths, and error
text remain hidden. No root cause stage is claimed until this instrumentation
is built from an exact commit and exercised on the disposable VM.

The focused `go test -race` runs for `./agent` and
`./cmd/containerd-shim-multikernel-v2` pass. Coverage includes both sides of
the cleanup ownership branch and verifies that the new wire diagnostic does
not disclose an injected private path or secret value.

Full local qualification also passes under the race detector, `go vet`, and
the documentation/schema/evidence/deployment chain. The sole skipped check is
the previously classified socket-permission subcase (`EPERM` in this local
sandbox). Exact commit creation and disposable-host execution remain next.

The exact qualified source is commit
`f34b6bbce490b6715d5185f989039bbb155951de`; `git archive` produced
`/tmp/mklinux-f34b6bb.tar.gz` with SHA-256
`25ecbc772d003bb6e3db9dac116647394237ceef7b19d7822f549bd085a3cf3a`.
Only tracked source is present; the historical untracked evidence directory
was not included or modified.

The existing disposable VM is running and did not require restart. A clean
pre-transfer check shows the unchanged boot ID
`ce405359-2f14-4655-9120-277e292af6da`, expected custom kernel, four active
required units, zero mknetd/mkruntimed restarts, empty containerd task and
container inventories, no listed `/run/multikernel` residue, and only the
durable lifecycle state/journal under `/var/lib/mkruntime`.

Remote transfer and build integrity passed. The guest rechecked archive digest
`25ecbc77…`, extracted into a new root-only source tree without `.git`, and
produced release manifest `90869f47…`, shim `ac971043…`, mkruntimed
`e2e65944…`, mknetd `0fcd9b75…`, and agent `a5edf911…`, all for exact revision
`f34b6bb…`. No activation claim is made at this checkpoint.

The first install command revealed that binary `install` is activating, not
staging-only. Because the build version had redundantly included the revision,
it selected an exact-source release whose identifier repeats `f34b6bb…`.
Services were not restarted and no workload was launched. That release and its
agent/initramfs (`a5edf911…`/`79b312a2…`) are explicitly excluded from the
corrected qualification; a clean extraction will rebuild version
`0.1.0-dev` with the revision supplied only through the revision field.

The corrected rebuild and manager activation now select the conventional
single-revision release `0.1.0-dev-f34b6bb…`. Exact hashes are release
manifest `d581bc41…`, shim `dcb023c4…`, mkruntimed `b9d751ca…`, mknetd
`c64f1fa4…`, agent `8cdb40db…`, initramfs `9b0f2bcd…`, and candidate kernel
manifest `1650f945…`. The strict bootstrap validator accepts the candidate;
running services still require an empty-host coordinated restart before these
identities can support runtime evidence.

The empty-host coordinated activation is healthy. The validated manifest was
renamed into place while runtime services were stopped; ten seconds after
restart all four required units are active, mknetd PID 47177 and mkruntimed
PID 47196 show zero restarts/status 0, installed manifest/agent/initramfs
re-match their staged hashes, executable identities report `f34b6bb…`, and the
boot ID is unchanged. Containerd task and container inventories remain empty.

The exact `3eaa058b…` basic runner failed during the first `ctr run` with the
same generic `INTERNAL: agent operation failed`, after all host preconditions
passed. Its EXIT cleanup executed. Crucially, no newly whitelisted network
setup stage reached the client, so the present evidence does not localize the
failure to `tun-open`/create or an `ip`/DNS stage. State, journal, console, and
residue collection follows before further implementation.

The retained state now disproves a network-setup root cause. Lifecycle
generation `efe83a02…` reached `RUNNING`, and mknetd endpoint `185af25c…` is
`READY` with two received and one transmitted packet and zero errors. Exact
`f34b6bb…` supervisor/worker PIDs 47443/47448 and relay PID 47657 remain live.
Cleanup's own generic `CloseNetwork` failure is distinct: it left task state
`CREATED`, active child/storage, and owned endpoint/link/firewall resources.
No forced teardown is performed before guest-console evidence is collected.

An intervening VM reboot occurred before the retained guest console could be
captured; available output does not establish its cause. On the new boot,
mknetd refused inconsistent durable state because endpoint `185af25c…`
survived while link `mkv185af25c9ca` did not. This is correct fail-closed
behavior, but it means pre-reboot live process/console evidence is gone. The
remaining durable ownership must be reconciled through runtime cleanup rather
than by deleting state files.

The new boot ID is `a0798b60-d6ff-4490-a176-ff09081f63a9`. Kerf has no pool or
children and no shim, relay, link, or namespace exists, while lifecycle
sequence 62, mknetd endpoint ownership, root image, and container metadata are
durable. mknetd and mkruntimed both restart-loop: the former rejects the
missing link and the latter rejects the reboot-volatile `/run` bundle's
absence during rootfs reconciliation. Detection is safe, but automatic owned
recovery is incomplete; state files remain untouched for source-level repair.

The cleanup error has a separate deterministic source: `CloseNetwork` closed
the non-persistent TUN before asking `ip` to delete it, allowing descriptor
close to remove the link first and turn the later command into a false cleanup
failure. Teardown now deletes while the descriptor is held and retains
ownership on failure. Since the network pump demonstrably ran, fixed
secret-safe diagnostic stages are also added at the subsequent process
`bundle-load` and `root-policy` boundary for the next exact live run.

Focused agent/shim race tests pass, including ordered link-before-descriptor
teardown, retry retention, bundle-load classification, and proof that injected
private path/token text does not cross the agent wire boundary.

Repository-wide race tests, vet, and the entire documentation/schema/evidence
gate pass. The only skip is the already classified unprivileged socket
rejection (`EPERM`); no new failure is hidden by that skip.

The exact next live candidate is commit
`bbd49c1d88a5e03e5350f80aa6719510518122ee`. Its source-only archive is
`/tmp/mklinux-bbd49c1.tar.gz`, SHA-256
`3d93538e1d1a2934a01a7191158ca4f9f8147b39b76b7a33aaf7fc2765effc6e`;
repository metadata and the untracked evidence directory are absent.

Remote digest/ownership checks and the exact `bbd49c1…` build passed. The
release manifest hashes to `8092999c…`, shim `6782283c…`, daemon `e13d5164…`,
mknetd `126782bd…`, and agent `9571b4f2…`. No activation or workload behavior
is inferred from these build outputs.

With all workload inventories empty, managed release
`0.1.0-dev-bbd49c1…` became current and all linked executable identities match
the exact revision. Its revision-matched agent is `9571b4f2…` and dependent
gzip-valid initramfs is `7f08fca1…`. Service restart and manifest activation
remain separate, unclaimed steps.

Candidate manifest `4ffc3661…` passed strict validation and was atomically
activated on empty inventories. After ten seconds all five required services
are active; mknetd PID 12565 and mkruntimed PID 12584 show zero restarts/status
0, executable/manifest identities match `bbd49c1…`, and boot ID remains
`a0798b60…`. Workload behavior is still unclaimed.

Exact `bbd49c1…` still returns generic `INTERNAL` during the first `ctr run`.
The new process-create stages did not surface, excluding the instrumented
bundle-load/root-policy paths from the observed error. EXIT cleanup ran; its
result and the likely later `StartProcess` boundary are audited next.

Cleanup now completes live: lifecycle sequence 9, mknetd, containerd,
Multikernel, runtime-storage, shim, and relay inventories are all empty, with
both daemon PIDs unchanged and zero restarts. The previous `CloseNetwork` and
`STALE_COUNTER` failures are absent. Thus both teardown corrections have exact
disposable-host evidence; only the later process-start failure remains.

The next diagnostic patch covers only the later `StartProcess` operational
stages (`executable`, `constraints`, `terminal`, `stdio`, `exec`). The wire
still carries fixed messages only, never raw paths or workload-controlled error
text. Live localization remains unclaimed until an exact rebuild/rerun.

Focused race tests pass for the new start stages and the existing shim suite.
The redaction test confirms private exec-path and token text does not appear on
the wire.

Full repository race tests, vet, and the documentation/evidence chain pass;
the already classified local socket `EPERM` is the sole skip.

Exact live candidate `f7c6f475c5c0dc0c35c07e68f51b74f409e211a6` is archived as
`/tmp/mklinux-f7c6f47.tar.gz`, SHA-256
`c851cd2541549d272da4c34cab7632933c40b08edbe2c6a84b342e89d9ddf4c4`.
Only tracked source is included.

Independent guest verification/build passed for `f7c6f47…`: release manifest
`525cbadb…`, shim `2d252204…`, daemon `da058880…`, mknetd `5c080d9e…`, and
agent `a727b7ab…`. These remain build-only identities until activation.

The empty-host manager selected release `0.1.0-dev-f7c6f47…`; exact agent and
dependent initramfs are `a727b7ab…` and `a9b85660…`. Strict bootstrap
validation accepts manifest `11113950…`; service activation remains pending.

Coordinated activation is healthy: after ten seconds all five services are
active, mknetd PID 14500 and mkruntimed PID 14520 have zero restarts/status 0,
manifest/executable identities match `f7c6f47…`, and the boot ID is unchanged.

Exact live execution now returns the fixed stage
`guest process start failed at exec`. Earlier StartProcess stages passed, and
raw error data remained redacted. The failure is therefore at final child
launch, not networking, bundle load, root policy, executable validation,
constraints encoding, or stdio setup. Cleanup and helper/chroot launch state
are inspected next.

The exec failure is a chroot/helper path mismatch: constrained starts selected
`/mk-agent`, but the agent binary is outside the OCI root applied before exec.
The already trusted procfs is bind-mounted into that root, so the helper now
re-execs through `/proc/self/exe`, an exact handle to the running agent also
used by its constraint tests. No helper is copied into the workload root.
Live confirmation remains pending.

Focused race tests pass, with the constraint executor still applying
no-new-privileges/rlimits and a new guard pinning the chroot-safe trusted
re-exec path.

The full repository race/vet/documentation-evidence gate passes, with only the
previously classified local socket `EPERM` skip.

The exact fix is commit `1e31080a1d3099fec0a0f2f9fa790eff94719779`;
source-only archive `/tmp/mklinux-1e31080.tar.gz` hashes to
`9b0b9ac2339ca466923524c394233b5a6e850332df7e7b9e2b6f376c53ae8d5d`.

The guest independently verified and built `1e31080…`: release manifest
`e695cecb…`, shim `db39bcec…`, daemon `4d3b4609…`, mknetd `095e3931…`, and
agent `eb3a2dc1…`. No activation claim yet follows.

On the empty host, release `0.1.0-dev-1e31080…` became current; exact
agent/initramfs are `eb3a2dc1…`/`1cd089b1…`, and candidate manifest
`cefc96b8…` passes strict bootstrap validation. Services remain to be
coordinated onto this set.

After atomic manifest replacement and service restart, all five units remain
active; mknetd PID 16385 and mkruntimed PID 16404 show zero restarts/status 0,
installed identities match `1e31080…`/`cefc96b8…`, and boot ID is unchanged.

Live `1e31080…` advances materially: ctr reaches task state `RUNNING`, proving
the `/proc/self/exe` constraint re-exec across chroot. Docker then fails its
host-side task creation at `inspect stdout: too many levels of symbolic links`.
The error precedes guest launch and points to the strict stdio path ancestry
check. The full suite remains failed; cleanup and Docker's concrete path
contract are the next evidence points.

The post-failure audit is clean: lifecycle and mknetd state files are absent,
containerd task/container and shim/relay/runtime-process inventories are
empty, and the exact mknetd/mkruntimed PIDs remain active with zero restarts.
On this host `/var/run` is a root-owned symlink with exact target `/run`, and
Docker's root is `/var/lib/docker`. That observed compatibility alias supports
the ancestry diagnosis without justifying general symlink traversal; the
accepted path must be normalized to a descriptor-checked canonical spelling.

One independent diagnostic is visible in the same journal. Containerd's
asynchronous dead-shim cleanup got `no such file or directory` while executing
the configured `/usr/local/bin/containerd-shim-multikernel-v2` path. The
managed link and exact `1e31080…` target are present at follow-up, so the
evidence does not establish removal or a harness cause; the transient exec
failure needs reproduction. No runtime resource remained.

The implementation now treats only the observed exact `/var/run/` spelling as
a compatibility alias. It checks that alias is caller-owned, single-link,
identity-stable, and points exactly to `/run`; all inspection, persistence,
recovery, and opening then use `/run/...` under the original strict
`openat2` boundary. Focused race tests accept that one alias and continue to
reject lookalike prefixes, changed targets, arbitrary symlink ancestors,
identity replacement, and invalid pre-mutation requests.

The full repository race suite and `go vet ./...` pass after the alias change.
Documentation/integrity and exact live qualification remain separate gates.

The full documentation, schema, evidence, deployment, and final-audit chain
also passes, with only the known local socket `EPERM` skip; `git diff --check`
is clean. Live confirmation remains pending.

The exact implementation checkpoint is
`444428ae88e0575e1a6a4a915ea9c83c001b6fc5`; its source-only archive
`/tmp/mklinux-444428a.tar.gz` has SHA-256
`ed539d24dd58c92300472042795d862403defe20d35b427c98dfeed33026efda`.
No guest result is inferred yet.

The guest-side first attempt extracts the archive but reaches no compiler: a
nested `awk` expression is misquoted and trips `set -u`. This is recorded as a
harness failure; the unique root-only extraction remains available for a
quoting-free hash check and build.

The retry verifies `ed539d24…` and successfully produces the seven runtime
binaries and exact release manifest for `444428a…`. Initramfs assembly fails
closed on a missing `install` input, so activation is not attempted; each
external initramfs input must be resolved first.

The absent input is the optional `mkvsock-relay`; all required local inputs
exist. Successful build hashes are retained (`8a9d5f15…` manifest,
`e176c997…` shim, `199529cb…` daemon, `72db893c…` mknetd, `52e81fa1…`
agent). The first current-image inspection command itself has unmatched shell
quotes and executes nothing, so no image-content inference is made.

Quote-safe inspection shows the active initramfs includes agent, transport
module, and relay. The manifest-bound relay is
`/opt/mkruntime/bin/mkvsock-relay` (`293ff1ea…`), not the missing libexec path;
the active module is `bef1b888…` and matches its libexec copy. The next build
can therefore reuse the exact active transport inputs without ambiguity.

The corrected image is `cb1d9771…` and contains the exact agent, verified
module, and relay. No activation claim follows from assembly alone.

The managed binary installer reports `installed_and_active` and advances the
`current` release link to `444428a…`; it does not restart the already-running
services. This is only a selected-release boundary, not coherent activation;
the guest artifact manifest and coordinated restarts remain pending.

The release-specific agent/initramfs copies pass exact hash checks, but the
candidate jq invocation over-escapes its named variables and fails on unbound
`ap` before writing JSON. Active manifest and services are untouched; only
candidate construction/validation needs rerun.

With one remote escape, candidate manifest `9e9c1908…` passes the strict
bootstrap validator against exact agent `52e81fa1…`, initramfs `cb1d9771…`,
and the pinned kernel/transport set. Active manifest stays `cefc96b8…` and the
old daemon PIDs remain unrestarted; staging alone changes no running service.

The pre-activation inventory is otherwise empty on boot `a0798b60…`, but its
shim subcheck uses overlong `pgrep -x` and is therefore inconclusive. Exact
`/proc/*/exe` resolution is required before the atomic switch.

The exact executable audit is empty. Atomic activation then passes without a
reboot: all five units are active, daemons are PIDs 20606/20628 with zero
restarts/status 0, binaries report exact `444428a…`, and manifest/release/shim
identities are `9e9c1908…`/`8a9d5f15…`/`e176c997…`. This establishes the
coherent pre-workload boundary only.

Live exact-source execution proves the `/var/run` normalization: ctr reaches
`RUNNING`, and Docker passes stdio inspection into OCI/rootfs construction.
Validation then rejects Docker's nonempty process-field presence for
`apparmorProfile` and `oomScoreAdj`; the runner exits 125 and invokes cleanup.
Those values and their safe compatibility semantics are the next boundary;
the shared basic suite remains failed.

EXIT cleanup is live-clean across both namespaces, Docker, child, storage,
state, networking, and exact runtime-process inventories, with both daemon
PIDs unchanged and unrestarted. The asynchronous dead-shim exec `ENOENT`
reproduces despite the installed managed link, but leaves no resource; it is
tracked independently from the OCI compatibility failure.

Direct inspection of a live Docker-generated OCI bundle establishes the two
values: `apparmorProfile="docker-default"` and integer `oomScoreAdj=0`. Only
the latter is semantically inert. The former requires either real child-kernel
enforcement or an explicit unconfined client contract; silent projection loss
would violate the fail-closed boundary.

Inside an exact Multikernel child, AppArmor reports enabled, the attr files
exist, securityfs profiles are unavailable, and current confinement is
`unconfined`. The probe's immediate kill/delete sequence races convergence and
gets `task must be stopped`; cleanup must be completed explicitly before this
observation can be used further.

The task subsequently reports `STOPPED`; task/container/child still exist only
because delete did not run. A cleanup wrapper then fails before mutation on a
misquoted `awk` field, so direct deletion from the observed stopped state is
required.

The direct delete attempt fails inside teardown at
`close guest network: INTERNAL: agent operation failed`. Cleanup is therefore
unproven and must be reconciled from durable/runtime state before any new
probe.

The failed delete leaves the stopped task/container, child, MK filter/NAT
rules, and storage/rootfs records. Separately, daemon PIDs are now 1236/1466
and mkruntimed shows one restart after a first later host check could not
confirm the Google guest agent. Earlier PID continuity cannot be projected
across this service/boot interval; ownership and boot state need fresh audit.

The interval change is an actual VM reboot to `e76edca2…` at 07:26 UTC. The
probe's exact `444428a…` shim PIDs 1976/1981 were created afterward and are
therefore current-boot resources. Durable lifecycle, network, storage, and
rootfs records—not the older quarantine—must drive reconciliation.

Current records agree on generation `372e816c…`: lifecycle remains `RUNNING`,
network endpoint `f5152c21…` is `READY` at 5/5 packets with no drops/errors,
storage export is `ACTIVE`, and rootfs phase is `PREPARED`. Host teardown did
not advance past guest close; the same idempotent delete may be retried only
after examining shim recovery/log state.

Recovery records init exit 137 and exact ownership; mknetd counters rose from
the shim snapshot's 2/1 to 5/5 during close. Both exact shim processes and
their pinned descriptors/sockets remain live. Because the RPC error is safely
redacted, bounded console evidence is needed to localize the guest failure.

Console attachment provides no raw guest error: Kerf says the retained
instance's kernel image is not loaded. With ownership and shim state intact,
the only justified next action is one retry of the idempotent delete.

The retry reproduces the same CloseNetwork failure. Further identical retries
are unjustified; the retained stopped task must now exercise the supervised
worker recovery path before cleanup is attempted again.

The exact worker termination leads the supervisor to exit rather than spawn a
replacement. Containerd observes disconnect and begins dead-shim cleanup; the
task disappears but container metadata remains. Recovery outcome is pending
the cleanup command and resource audit.

Cleanup-only recovery leaves no lifecycle sandbox, network endpoint, child,
host rule/link, shim, relay, or storage-server process; lifecycle sequence is
49. The sole visible remainder is containerd container metadata, pending
storage/rootfs verification and normal removal.

A combined verification/removal command fails at jq parsing before mutation.
State projection and container-metadata removal remain separate pending steps.

Read-only state confirms a `RELEASED` export, no rootfs record, and no storage
directory. Removing inert container metadata yields empty task/container/child
inventories, closing the probe through runtime recovery rather than state-file
deletion.

Because the guest image contains no AppArmor policy loader or profile and its
process is unconfined, silently accepting `docker-default` is unsound. Docker
must explicitly request `apparmor=unconfined`; only integer OOM adjustment zero
is eligible as a no-op compatibility field.

Docker's explicit unconfined request generates the exact pair
`apparmorProfile="unconfined"`, `oomScoreAdj=0`. This provides a narrow,
testable adapter contract: accept and omit only those values, reject every
requested profile and nonzero/malformed adjustment, and pass the opt-out on
all Docker qualification creates/runs.

Before upgrade, the `f7c6f47…` failure cleanup rechecks fully empty across all
runtime/network/container/child/storage inventories, with both daemon PIDs
unchanged and zero restarts.

The post-reboot environment reset uses recoverable root-only quarantine, not
manual deletion. Exact durable hashes were captured before moving lifecycle,
network, and storage-task records; container metadata was then removed.
mknetd starts cleanly (PID 6958, zero restarts), but mkruntimed still fails,
showing its rootfs ownership store is separate from `/var/lib/mkruntime`.
Quarantine is explicitly excluded from cleanup qualification and remains
available for audit.

The separate storage/rootfs store hashes (`f7d4e4b1…`, `231c3776…`) were then
captured and moved into that quarantine. Empty stores start successfully:
mknetd PID 6958 and mkruntimed PID 7683 have zero restarts/status 0, and task,
container, child, and runtime-storage inventories are empty. This is a
recoverable disposable-environment reset, not evidence that runtime cleanup
handled the reboot residue.

The next G4 builder audit found that deterministic content did not imply safe
publication. `build-runtime-rootfs.py` used `os.replace` for archive and
manifest output, overwriting any pre-existing entry. If the manifest publish
succeeded and archive publication then collided or failed, the new manifest
could also remain alone. This is recorded before implementation: output needs
no-replace publication plus exact-identity rollback across the two-artifact
transaction.

The builder now links each completed temp inode into place with no-replace
semantics, verifies that exact publication, and conditionally rolls it back
through a `renameat2(RENAME_NOREPLACE)` identity quarantine. Archive publishes
first; manifest collision removes only the archive inode created by that
attempt. Focused cases preserve pre-existing archive/manifest bytes in every
collision order, leave no orphan peer, and preserve a same-name rollback
replacement. All 17 rootfs-builder cases pass once; repeated and full checks
remain pending.

The no-replace transaction and replacement-preserving rollback cases passed
100 repetitions. Full repository and documentation verification remain
pending.

Final semantic review also moved chmod and identity capture onto the open temp
descriptor and replaced unconditional temp-name unlink with the same
identity-conditioned quarantine. The 17-case suite and another 100 repetitions
of both adversarial cases pass after that refinement.

The subsequent complete repository race suite, `go vet ./...`, the full
documentation/schema/evidence/deployment chain, and `git diff --check` passed
on 2026-09-18. The local no-replace builder-publication checkpoint is complete;
G4 closure still requires current-source disposable-host evidence.

The adjacent ext4-builder audit found the same boundary in a more privileged
form. Image and metadata publication use replacing renames after a racy
`exists()` check, failure cleanup unconditionally unlinks both public names,
and `mke2fs`/`debugfs`/`e2fsck` reopen the random temp pathname. A raced entry
can therefore be overwritten, deleted, or supplied to an external filesystem
tool. This is recorded before implementation; the image descriptor must remain
inherited through every tool and both outputs need no-replace, exact-identity
publication and rollback.

The ext4 builder now retains the exact allocated image descriptor through
`mke2fs`, `debugfs`, and `e2fsck`, passing only inherited `/proc/self/fd`
references. Its debugfs commands live in an anonymous inherited file and are
rewound before execution. Image and metadata publication share the no-replace
identity-quarantine helper; failure removes only this attempt's inodes.
Focused raced-image and raced-metadata cases preserve replacements and roll
back the owned peer. All 17 rootfs, 7 storage, and deployment lifecycle cases
pass once, and the shared helper is included in immutable deployments.
Repeated and full checks remain pending.

The real ext4 raced-image/raced-metadata transaction then passed 100
repetitions. Full repository and documentation verification remain pending.

The subsequent complete repository race suite, `go vet ./...`, Python
compilation, the full documentation/schema/evidence/deployment chain, and
`git diff --check` passed on 2026-09-18. The local descriptor-bound ext4 and
exact-publication checkpoint is complete; broader G4 fault and live evidence
remain open.

The next staging audit found that the ext4 image descriptor fix did not bind
the private clone itself. Copy, normalization, filesystem import, and final
`shutil.rmtree(staging)` still use the random public directory pathname, so a
same-name replacement can redirect work or be recursively deleted at cleanup.
This is recorded before implementation; every staging consumer and cleanup
decision must share one held staging-directory inode and preserve a public
replacement.

On 2026-09-19, a builder-input audit found that both deterministic rootfs scan
and ext4 source copy call `Path.resolve()` on their supplied roots. An inherited
`/proc/self/fd/...` root is thereby converted back into a caller-visible path
before traversal, invalidating the descriptor-bound input claim. This is
recorded before code changes: both builders must open the supplied root once
without following its final component and operate only through a newly held
fd path.

The first focused rootfs test run after that change produced 10 failures and 3
errors across 18 cases. No-follow metadata treated the synthetic
`/proc/self/fd/N` root path as a symlink, so traversal admitted only `.`. This
was recorded before correction: metadata for the synthetic root must follow
the already-held descriptor, while every child observation remains no-follow.
The fail-fast command did not start the ext4 suite.

After applying that distinction, all 18 focused rootfs cases passed on
2026-09-19. The adversarial case replaced the public root after descriptor
acquisition; the scan returned the original file bytes through the held inode
and preserved the replacement tree.

All 9 focused ext4-builder cases then passed on 2026-09-19. In the new source
replacement case, `debugfs` read the original `one\n` from the emitted image
while the replacement tree still contained `replacement\n`; `cp` inherited
the held source descriptor explicitly.

A follow-on validator audit on 2026-09-19 found that root-path validation
resolved the inherited bundle and returned a public pathname to the parent,
and image validation resolved its root before executable inspection. This is
recorded before implementation. Root-path validation must preserve and verify
the inherited-fd anchor; image validation must acquire and retain a no-follow
root descriptor through all entrypoint reads.

On 2026-09-19, all 7 focused root-path cases passed. The new descriptor case
replaced the public bundle while validation retained the exact inherited
`/proc/self/fd/N/rootfs` result. Image validation also passed the ELF,
interpreter, wrong-architecture, escape, and held-root race cases; the race
hashed the original executable and left a wrong-architecture replacement
untouched.

A 2026-09-19 repetition campaign passed 100 consecutive public-path
replacement runs at each of the rootfs scan, real ext4 source copy,
inherited-bundle validation, and image-entrypoint validation boundaries.

The first full race-gate invocation on 2026-09-19 stopped during Go setup
because it selected the sandbox's read-only default cache. No tests executed;
this environmental failure is recorded before rerunning with the established
writable `/tmp/mklinux-gocache` and is not treated as software evidence.

With the writable cache configured, `go test -race -count=1 ./...` passed
across every runtime package on 2026-09-19.

`go vet ./...` also passed with that writable cache on 2026-09-19.

`bash scripts/check-docs.sh` passed on 2026-09-19, including 18 rootfs cases,
9 storage cases, 7 root-path cases, the image held-root race, 55 OCI semantic
cases, schema/evidence audits, deployment lifecycle, and resource-ledger
checks.

A subsequent outer-transaction audit on 2026-09-19 found that
`build-runtime-container-initramfs.sh` still unconditionally `rm -f`s public
output names in its failure trap. An inner builder can safely publish an inode,
then a later failure and same-name substitution can cause the outer trap to
delete the substitute. This is recorded before implementation; cleanup must
be exact-identity conditional or deferred to the backend's descriptor-bound
directory cleanup.

The outer shell now cleans only its private `mktemp` workspace. Production
artifact cleanup remains with the service, which records and quarantines the
exact private runtime/storage directory inodes. A focused 2026-09-19
early-failure run pre-populated all 8 former public cleanup targets and proved
every replacement byte remained unchanged; all 55 OCI semantic cases and the
existing namespace/file-identity boundaries passed with it.

A follow-on bind-materialization audit on 2026-09-19 found that validation and
the pre-copy manifest release the source before `cp` reopens its public path,
then the post-copy manifest reopens it again. A same-content directory
replacement can change the copied inode while preserving both manifests. The
private target root is also pathname-only. This is recorded before code
changes: both roots must be opened component-by-component without following
symlinks and held across admission, copy, and verification.

The first focused bind run after descriptor conversion failed its regular-file
case because `cp --archive` preserved the `/proc/self/fd/N` magic link itself;
copied-target validation then rejected the symlink. Directory sources were
unaffected. This is recorded before correction: regular sources must
dereference only the fd magic-link boundary, without dereferencing symlinks in
directory trees.

The next focused run did reject the symlinked source, but failed the diagnostic
assertion because raw `ELOOP` text replaced the established “must not traverse”
message. The rejection remained fail-closed; this is recorded before restoring
the stable diagnostic contract.

After regular-fd dereference and diagnostic correction, the focused bind suite
passed on 2026-09-19. Replacing the public source immediately before `cp`
still copied `original\n` through the held inode and preserved the substitute;
replacing the public target root before destination creation wrote only into
the held original root and preserved the marked replacement.

The complete bind-materialization suite, including both descriptor races,
then passed 100 consecutive repetitions on 2026-09-19.

The subsequent `go test -race -count=1 ./...`, `go vet ./...`, and
`bash scripts/check-docs.sh` all passed on 2026-09-19. The documentation gate
explicitly reports held source/target races together with the complete
builder, validator, evidence, deployment, and resource-ledger suites.

A subsequent image-validation audit on 2026-09-19 found that a held top-level
root does not secure child traversal while entrypoint resolution still uses
separate pathname `lstat`, `readlink`, and open calls. A replaced child
component can redirect traversal outside the OCI root. This is recorded before
implementation: the kernel must enforce in-root resolution, and ELF/shebang
bytes must be read from the resulting exact descriptor.

A subsequent outer-source audit on 2026-09-19 found that root-path validation,
both manifests, image validation, and archive copy still acquire separate root
descriptors. A same-content replacement can change the copied inode without
changing either manifest. This is recorded before implementation: root
validation must return its observed device/inode and the shell must bind every
consumer to one matching inherited directory fd. A local probe confirmed Bash
retains a directory fd across child commands and `/proc/self/fd/N` reads the
held inode.

The validator now returns its observed path/device/inode as JSON. The shell
opens that path once, rejects an `fstat` mismatch, and supplies the exact fd
path to image validation, both manifests, and `cp`. Focused 2026-09-19 suites
passed with 7 root-path cases, 19 rootfs cases, 10 real ext4 cases, and the
image suite. The shell handshake accepts the exact identity and rejects a
`rootfs` replacement installed after validation.

The root identity handshake, exact-fd rootfs scan, real ext4 copy, and image
validation boundaries then passed 100 consecutive repetitions each on
2026-09-19.

The subsequent `go test -race -count=1 ./...`, `go vet ./...`, and full
`bash scripts/check-docs.sh` gate passed on 2026-09-19, including the expanded
19-case rootfs and 10-case storage suites and all evidence/deployment audits.

A subsequent transaction review on 2026-09-19 found that the post-copy source
manifest is safely built as `.after` but then published with replacing `mv`.
A same-name final artifact can bypass the inner builder's no-replace guarantee.
This is recorded before implementation; the post-copy scan must publish
directly to the final no-replace name before comparison.

The pre-copy manifest now exists only under the private `mktemp` workspace;
the post-copy scan publishes directly to the retained final name, and the
shell compares them without replacing `mv` or public temporary cleanup. On
2026-09-19, shell syntax, the 55-case OCI/transaction suite, and the focused
rootfs no-replace collision/rollback case passed.

The subsequent `go test -race -count=1 ./...`, `go vet ./...`, and full
`bash scripts/check-docs.sh` gate all passed on 2026-09-19.

A storage-backend audit on 2026-09-19 found that `Inspect` validates a public
image path, `Start` later passes that public name to `mkvsock-nbd`, and
`OfflineCheck` reopens it again. Parent replacement can redirect every
boundary; a replacement between inspection and start can become the exported
writable disk. This is recorded before implementation. Inspection, inherited
server fd 3 with restart authentication, and offline `e2fsck` must all use
exact descriptors opened beneath one held no-symlink parent.

The first focused compile after the durable-identity conversion found 8
storage-backend test call sites still expecting the former error-only
`Inspect` result, so storage tests did not run; `safefile` and lifecycle tests
passed. This compatibility failure is recorded before updating fixtures to
assert the returned image identity.

After correcting signatures, 6 storage cases stopped before their assertions
because local `t.TempDir()` parents were mode `0775`; the no-symlink opener
correctly requires a caller-owned, non-group/other-writable image directory.
`safefile` and lifecycle remained green. This is recorded before making the
ext4 fixture match production's private `0700` storage root.

That left one fixture failure: the command-start cleanup backend retained
required UID 0 and rejected the non-root test image before creating its runtime
directory. This pre-start rejection is recorded before assigning the caller
UID so all artifact subtests reach their intended cleanup boundaries.

Storage state is now version 2 and retains the inspected image device/inode.
Only empty version-1 state upgrades; active legacy and identity-less records
are rejected. Inspection hashes an exact private fd, server start revalidates
and inherits that inode as fd 3, recovery authenticates the process's fd 3,
and offline `e2fsck` inherits another exact fd. Public parent identity is
rechecked after every operation. The race-enabled storage, `safefile`, and
lifecycle packages passed on 2026-09-19, including real ext4 parent replacement
at inspect/start/offline-check, process-record inode mismatch, and
reconciliation refusal before restart.

The real-ext4 inspect/start/offline-check parent-replacement group then passed
100 race-detector repetitions in 63.852 seconds on 2026-09-19.

The subsequent authoritative full repository gate passed on 2026-09-19:
`go test -race -count=1 ./...` passed every package, including storage in
3.582 seconds, and `go vet ./...` completed with no diagnostics. The full
`bash scripts/check-docs.sh` chain then passed, covering links, schemas, 17
current evidence manifests, 55 OCI semantic cases, bind/rootfs/image race
suites, release/deployment lifecycle, resource-ledger, command-capture,
containerd, and final-evidence audits; `git diff --check` was also clean.

A post-gate ownership review then found a remaining lock-transfer defect:
`Start` releases the inspection `flock` before `mkvsock-nbd` opens and locks fd
3, leaving a second-owner/content-mutation window, while `OfflineCheck` does
not lock the image at all. This is recorded before correction. Server launch
must retain one open-file-description lock across exec, and the complete
offline check must hold its own exclusive lock.

The server now duplicates inherited fd 3 instead of reopening it, preserving
the locked open file description from validation through serving; offline
checking locks its exact fd for the complete command. The production C server
passed `-Wall -Wextra -Werror` syntax compilation, and race-enabled storage,
`safefile`, and lifecycle packages passed. Direct contention tests prove the
lock is unavailable while either server or checker owns it and is available
again after exact server stop or checker exit. The combined server
handoff/recovery and offline-check locking/bounds group then passed 100
race-detector repetitions in 58.265 seconds on 2026-09-19. Full-tree
verification remains pending for this correction.

The next full Go race suite passed, but its combined gate harness named
`tools/mkvsock-nbd.c` while running from `runtime/`; that nonexistent path
made C compilation fail before `go vet` was attempted. This command-path
failure is recorded before rerunning the skipped checks from the correct
location.

The corrected production C compilation and `go vet ./...` then passed with no
diagnostics. Together with the immediately preceding all-package race pass
(storage: 3.571 seconds), the full code gate is complete; documentation and
repository-integrity verification remain pending. The subsequent full
`bash scripts/check-docs.sh` chain passed across links, schemas, evidence, OCI,
bind/rootfs/image races, release/deployment, resource-ledger, command-capture,
containerd, and final-evidence audits; `git diff --check` was clean. This
completes the local exact-image identity and continuous-lock checkpoint.
Disposable-host proof remains unauthorized.

The next recovery audit found that process authentication still opens
`/proc/<pid>/stat`, `cmdline`, and `fd/3` independently and teardown then
signals the numeric PID/process group. Exit plus PID reuse between those
operations can cross identities. This is recorded before implementation;
recovery must retain one pidfd for exact liveness/signaling and one held
`/proc/<pid>` directory for all metadata inspection.

The first pidfd-backed implementation passed the race-enabled storage suite,
including daemon-restart adoption and exact stop. Its review found an adjacent
executable-identity gap: launch still resolves the server binary by pathname
and recovery trusts argv without authenticating `/proc/<pid>/exe`. This is
recorded before extending the durable process record and exact recovery check
to the executable device/inode as well.

The first executable-binding run then stopped at three fixture assumptions:
two compiled fake-server parents inherited mode `0775` and were correctly
rejected as group-writable, while the missing-binary cleanup case now fails
before runtime-directory creation but still assumed that directory existed.
`safefile` passed; these fixture-boundary failures are recorded before aligning
the tests with the new pre-launch trust boundary.

After fixing the parents, the compiler's output was observed as mode `0775`
and was correctly rejected too; production installs the helper as `0755`, so
the compiled fixtures must explicitly reproduce that deployed mode.

With fixture modes aligned, race-enabled storage and `safefile` suites pass.
Launch now executes a held server fd, process-record version 3 binds both image
and executable inodes, recovery reads stat/cmdline/fd 3/exe through one held
proc directory, and teardown signals only the retained pidfd. Adversarial
executable-replacement and repeated verification remain pending.

The first adversarial executable-replacement test did not compile because its
new `bytes.Equal` assertion omitted the `bytes` import; no test executed. This
harness error is recorded before adding the import and rerunning it.

After restoring the import, the race-enabled executable-replacement,
recovered-pidfd stop, managed stop, and startup-cleanup group passed. The held
original reaches the post-exec identity check, the unsafe substitute is
preserved, launch fails closed, and no runtime artifact remains. Repeated and
full-tree verification remain pending.

A follow-up recovery review found that the boolean matcher still collapses
“process absent” and “same recorded process conflicts with executable/image
metadata.” `Observe` would remove the record in both cases and could abandon a
live server after executable-path replacement. This is recorded before
separating proven absence/PID reuse from a diagnosable live conflict.

The split now passes focused race testing. A forged executable inode returns a
specific conflict and preserves the process record byte-for-byte; after
restoring it, replacement of the public binary pathname does not break
adoption because `/proc/<pid>/exe` still matches the durably recorded launched
inode, and exact pidfd stop succeeds. The combined executable-substitution and
recovered-process identity group then passed 100 race-detector repetitions in
55.082 seconds on 2026-09-19.

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

Pre-commit error-path review then found that `pidfd_open` and signal-0 failures
other than `ESRCH` were treated as absence, so descriptor exhaustion or
permission/kernel errors could allow stale-record removal. This is recorded
before correction: only `ESRCH` may prove absence; every other pidfd/proc
failure must preserve state and surface an error.

After restricting absence to `ESRCH`, race-enabled storage, `safefile`, and
lifecycle suites passed. Other pidfd/proc errors now preserve the process
record and return a diagnostic failure. The final all-package race run then
passed (storage: 3.775 seconds), and `go vet ./...` was clean. The final
documentation chain then passed across links, schemas, evidence, OCI,
bind/rootfs/image races, release/deployment, resource-ledger, command-capture,
containerd, and final-evidence audits; `git diff --check` was clean. The local
pidfd/executable checkpoint is complete.

The next offline-check audit found that the image is descriptor-bound but
`CheckBinary` is still executed by public pathname; a substituted checker
could return success and falsely certify a bad filesystem. This is recorded
before implementation. The deployed checker contract is a regular `0755`,
single-link executable and must use the same held-executable boundary.

`OfflineCheck` now revalidates the exact image name, executes a held checker as
fd 4 while the image remains fd 3 and exclusively locked, and rejects and
preserves a public checker substitute after execution. The focused
race-enabled parent-replacement and complete bounded-check matrix passed.
The complete offline-check matrix then passed 100 race-detector repetitions in
27.721 seconds on 2026-09-19, covering checker substitution, exclusive locking,
timeout/descendant cleanup, output bounds, evidence hashing, and secret-safe
failure. Full-tree verification remains pending.

The first full race run then found one environment-specific fixture failure:
the integrated graceful-stop case used `/usr/sbin/e2fsck`, exposed here as UID
65534 while the test runs as UID 1000, so the new caller-owned check rejected
it; all other packages passed and `go vet` was skipped. This is recorded before
copying the real checker bytes into the fixture's private caller-owned
directory and rerunning the full gate.

The integrated graceful-stop/offline-check case now passes with an exact byte
copy of the real `e2fsck` in its private caller-owned fixture directory,
retaining real checker behavior while matching the production trust contract.
The corrected all-package race suite then passed (storage: 3.780 seconds), and
`go vet ./...` completed without diagnostics. Documentation and
repository-integrity verification remain pending. The full documentation chain
then passed across links, schemas, evidence, OCI, bind/rootfs/image races,
release/deployment, resource-ledger, command-capture, containerd, and
final-evidence audits; `git diff --check` was clean. This completes the local
exact-checker checkpoint; disposable-host proof remains unauthorized.

The next guest-teardown audit found that the agent syncs, remounts `/`
read-only, syncs again, and powers off, but never issues `NBD_DISCONNECT`; the
helper supports it, yet the runtime shutdown path neither calls it nor emits
ordered disconnect evidence. This is recorded before implementation.
Disconnect must occur after the authenticated Shutdown reply is delivered and
immediately before poweroff, using a no-follow, exact `/dev/nbd0` block-device
check and fail-closed behavior.

The first shutdown test compile found that this platform's `Stat_t.Rdev` is
`uint64`, while two fixture assignments used `int64`; mk-agent tests did not
run, and the independent agent suite passed. This harness type error is
recorded before correcting the fixture assignments.

After correction, race-enabled mk-agent and agent suites passed. Tests prove
exact `sync -> remount-ro -> sync -> NBD disconnect -> poweroff` ordering,
no-follow `/dev/nbd0` block major/minor validation, ordered PASS evidence, and
that disconnect failure emits FAIL evidence and prevents poweroff. Header
verification, repetition, and full-tree gates remain pending.

The first ioctl header probe failed to link because `<linux/nbd.h>` exposes
`NBD_DISCONNECT` via `_IO` without including the userspace `<sys/ioctl.h>`
definition; no probe binary ran. This harness failure is recorded before
rerunning with the header pair used by the production C helper.

The corrected host-header probe reported `NBD_DISCONNECT = 0xab08`, exactly
matching the Go implementation. The expanded device/order/failure group then
passed 100 race-detector
repetitions in 1.021 seconds on 2026-09-19; remount failure stops before
disconnect/poweroff, and disconnect failure stops before poweroff. The
complete repository race suite then passed, including mk-agent, and
`go vet ./...` completed without diagnostics. Documentation and
repository-integrity verification remain pending. The full documentation chain
then passed across links, schemas, evidence, OCI, bind/rootfs/image races,
release/deployment, resource-ledger, command-capture, containerd, and
final-evidence audits; `git diff --check` was clean. A static deployment-form
mk-agent build remains to be checked before this local checkpoint closes.
`CGO_ENABLED=0 go build -trimpath ./cmd/mk-agent` then produced a statically
linked x86-64 ELF, and its version entrypoint ran successfully. The local
ordered-disconnect checkpoint is complete; live child/primary proof remains
unauthorized.

Final diagnostic review changed the generic failure prefix from `poweroff:` to
`shutdown:` because a disconnect failure deliberately prevents poweroff;
retained console evidence now names the failed phase accurately.

The complete repository race suite and `go vet ./...` passed again after that
correction. The final documentation/evidence chain and `git diff --check` also
passed. Local implementation and verification are complete; privileged live
ordering evidence remains open.

The following server-sync audit found that production calls final `fdatasync`
before its close marker, but the marker contains only counters and fake servers
can emit an indistinguishable line without syncing. This evidence-contract gap
is recorded before implementation. The canonical close record must include
`synced=1` emitted only after successful final `fdatasync`; recovery must reject
every legacy or forged unsynced line.

The production C helper now emits that field only after final `fdatasync` and
passes warning-clean compilation. Focused graceful-stop/recovery and canonical
parser tests pass; `synced=0` and legacy records without the field are rejected.
The graceful-stop, pidfd-recovery, sync-proof parser, and managed-stop group
then passed 100 race-detector repetitions in 59.239 seconds on 2026-09-19.
The production C helper, all-package race suite (storage: 3.910 seconds), and
`go vet ./...` then passed. Documentation and repository-integrity verification
remain pending. On 2026-09-20, the full documentation chain passed across
links, schemas, evidence, OCI, bind/rootfs/image races, release/deployment,
resource-ledger, command-capture, containerd, and final-evidence audits;
`git diff --check` was clean. The local explicit server-sync evidence
checkpoint is complete; live child/primary proof remains open.

The first descriptor-walker focused run stopped before its new race because
the fixture still held the preceding `/escape` configuration; the resolver
correctly rejected it. This harness setup failure is recorded before resetting
the fixture to `/bin/program` and rerunning the child replacement.

After resetting it, the focused image suite passed on 2026-09-19. The new race
replaces `bin` with a symlink to a wrong-architecture executable after opening
the original directory; validation hashes the original amd64 file through the
held child descriptor and preserves the replacement.

The complete image-validation suite, including held-root and held-child
replacements, then passed 100 consecutive repetitions on 2026-09-19.

The subsequent `go test -race -count=1 ./...`, `go vet ./...`, and full
`bash scripts/check-docs.sh` gate all passed on 2026-09-19; documentation
output explicitly includes the held root/child image races.

After this change, `bash -n`, the focused OCI/cleanup suite,
`git diff --check`, `go test -race -count=1 ./...`, and `go vet ./...` all
passed on 2026-09-19; the full documentation gate follows separately.

`bash scripts/check-docs.sh` then passed on 2026-09-19 with the outer-cleanup
boundary explicitly included, together with the complete builder, validator,
evidence, deployment, and resource-ledger suites.

The storage builder now opens staging once, creates its root relative to that
descriptor, and uses inherited `/proc/self/fd` paths for copy, normalization,
inode accounting, and filesystem import. Cleanup quarantines only the exact
public staging inode before recursive removal. In the focused replacement
test, the original is moved and a marked substitute installed. Construction
continues only from the held original, but exact cleanup is mandatory: the
builder fails closed, rolls back its image/metadata, and preserves the
substitute. The 8-case suite and this real ext4 case for 100 repetitions pass
after final transaction review. Full verification remains pending.

The subsequent complete repository race suite, `go vet ./...`, the full
17-case rootfs/8-case storage and documentation/schema/evidence/deployment
chain, and `git diff --check` passed on 2026-09-18. This completes the local
staging-identity checkpoint; privileged interruption and ENOSPC evidence remain
open.

The next handoff review found that only supervisor-to-worker restart was fully
descriptor-bound. The initial supervisor command still used the public
`Getwd` pathname from `newCommand`, allowing a same-owner bundle replacement
before `exec` to become its cwd before the worker identity handoff existed.
This is recorded before implementation: the first supervisor must chdir via
the service-lifetime held bundle descriptor as well.

Initial supervisor launch now assigns `cmd.Dir` from the held bundle's
`/proc/self/fd` path, and `newCommand` no longer snapshots the public cwd.
Supervisor startup opens its inherited current directory before deriving a
display path. In the focused test, the public bundle was moved and replaced
before launch; the supervisor marker appeared only in the held original. The
existing descriptor-bound worker restart and pre-cancelled-start cases pass
alongside it. Repeated race and full-tree checks remain pending.

The initial-supervisor replacement, descriptor-bound worker restart, and
pre-cancelled startup group passed 100 race-detector repetitions in 105.230
seconds. Full-tree verification remains pending.

The subsequent complete repository race suite, `go vet ./...`, the full
documentation/schema/evidence/deployment chain, and `git diff --check` passed
on 2026-09-18. The local initial-supervisor bundle-handoff checkpoint is
complete; current-source disposable-host proof remains unauthorized.

The collision-reuse audit then found that connectivity alone is not an
authenticated existing shim. The current path does not match the listener to
the held bundle's `address`/`shim.pid`, supervisor executable/cwd/process
group, or namespace/task/containerd-address arguments. A same-owner listener
can therefore occupy the deterministic name and be returned as this task's
shim. This is recorded before implementation; reuse must require the complete
launch metadata plus an anchored `/proc/<pid>` identity match.

Collision reuse now reads exact `address` and numeric `shim.pid` metadata
through the held bundle, retains an open `/proc/<pid>` directory, and verifies
the supervisor's cwd inode, executable inode, live process-group leadership,
and exact namespace/task/containerd-address invocation. Focused tests accept
the exact supervisor and reject mismatched cwd, executable, group ownership,
task ID, and containerd address; the earlier live/stale/replaced socket cases
also pass. Repeated race and full-tree checks remain pending.

The authenticated existing-supervisor and live/stale/replaced socket group
passed 100 race-detector repetitions. Full-tree verification remains pending.

The subsequent complete repository race suite, `go vet ./...`, the full
documentation/schema/evidence/deployment chain, and `git diff --check` passed
on 2026-09-18. The local authenticated collision-reuse checkpoint is complete;
current-source disposable-host proof remains unauthorized.

On 2026-09-27 the Docker OCI compatibility boundary was tightened after live
bundle inspection. Only explicit `apparmorProfile="unconfined"` and exact
integer `oomScoreAdj=0` are accepted and removed from the child projection;
Docker's default profile and nonzero, boolean, floating-point, or malformed OOM
values remain rejected. Every Docker qualification create/run path now makes
the network and AppArmor decisions explicit with `--network none` and
`--security-opt apparmor=unconfined`. The focused validator passed 67 semantic
cases plus namespace, file-identity, and outer-cleanup boundaries, followed by
Python compilation, both qualification-script syntax checks, and
`git diff --check`. Exact-revision disposable-host execution remains pending.

The ensuing complete local gate passed on 2026-09-27: the full Go race suite,
`go vet ./...`, all documentation/schema/evidence/deployment checks, and
`git diff --check`. A source audit additionally verified that every Docker
create/run in both live qualification entry points uses the explicit network
and AppArmor settings, including stdin, attach, PTY, and resize branches. The
checkpoint is still local until its exact committed archive is activated and
observed on the disposable host.

The resulting immutable source checkpoint is
`a72c5cb97c899bbfb567137ddfa48bf42165a887`; the source-only archive
`/tmp/mklinux-a72c5cb.tar.gz` has SHA-256
`bbc2eb49b0aaa4fe1a4846fd500613f569441276a99d860c0bd9aa9e1d880a6c`.
The pre-existing untracked evidence tree was excluded and left untouched.
Host-side digest verification and activation have not yet occurred.

The disposable instance lookup then confirmed the intended labeled resource
still exists and is `RUNNING` in `asia-southeast1-b` (instance resource ID
`8436995220542526424`, last start `2026-09-27T00:26:24.270-07:00`). Therefore
neither recreation nor restart is justified before transfer; guest identity
and archive digest verification remain next.

The exact archive transfer completed, and guest-side SHA-256 matched
`bbc2eb49b0aaa4fe1a4846fd500613f569441276a99d860c0bd9aa9e1d880a6c`
(992193 bytes). The guest reports boot ID
`e76edca2-d8d6-4800-9df8-f9ee9dccca8f`, kernel
`7.0.0-mk2-gce-lab`, all three required services active, and empty containerd
task/container inventories. The same command used a stale guessed Kerf path
and stopped before child/network inventory, so those preconditions remain
unproven until the installed paths are discovered and queried.

The corrected preflight establishes a quiescent host: child, Docker,
containerd, `mkv*` link, Multikernel firewall, active lifecycle, mknetd
endpoint, rootfs, and runtime-storage inventories are all empty. The retained
lifecycle results and five storage records are historical, with every export
`RELEASED` and offline-checked. mknetd/containerd remain at zero restarts;
mkruntimed's one restart is the previously recorded boot-time guest-agent
ordering failure. Kerf is installed under its venv and exposes no `list`
command, so sysfs plus durable service state provide the child/network
inventory evidence.

The exact root-owned source extraction and all seven revision-stamped binary
builds succeeded. Bootstrap assembly did not: an assumed transport-module path
inside the previous revision directory was absent, producing `install: No
such file or directory`. Nothing was installed or activated. Active
manifest-bound module/relay paths and hashes must be resolved before retrying
only the bootstrap assembly.

Manifest inspection re-established the prior agent, initramfs, and relay
identities, but its generic path loop selected the transport module object
instead of `.transport.module.path` and stopped at `{`. This inspection error
does not change the guest; direct module/kernel and partial-output checks are
still required before retry.

The corrected direct check confirms root-owned module `bef1b888…`, kernel
`5cdf26d0…`, and relay `293ff1ea…` at the active manifest's exact paths, and
there is no partial candidate initramfs. Bootstrap assembly can therefore be
retried in isolation with those verified inputs.

The isolated bootstrap build succeeds with initramfs `a78778e2…`; the exact
release/representative binary hashes are `55e4ea88…` (manifest), `f32e4a1c…`
(shim), `497ca3f1…` (daemon), `327dcb51…` (mknetd), and `0ee464e9…`
(agent). A non-root archive listing then hits the intended mode-0600 read
denial, which the non-pipefail shell masks at the final `sort`. Membership
inspection remains pending under sudo; active state is unchanged.

The first privileged/pipefail listing assertion yields no matches because it
assumed `./`-prefixed cpio paths. An unfiltered listing is required to learn
the archive's actual spelling before making the membership claim.

The privileged unfiltered listing succeeds: the cpio paths omit `./` and
include `mk-agent`, `mk_transport.ko`, `mkvsock-relay`, `init`, BusyBox, and
the fixture bundle. Candidate bootstrap completeness is now established;
nothing is active yet.

The binary manager installed and selected exact release
`0.1.0-dev-a72c5cb97c899bbfb567137ddfa48bf42165a887`, and all managed links
report the revision. mkruntimed/mknetd PIDs and restart counts remain
1466/1 and 1236/0, proving no service process restarted; deployment support
and the kernel manifest are not yet coherent with the selected links.

Managed support deployment `c9e5b67094f8f4aba5783ea5fcaac57489a62ec9da77507f7d0eb220717a24ce`
is now selected with every link valid. The deployed and exact-source OCI
validators share SHA-256 `5303e914…`. Service identities remain unchanged,
so this proves filesystem selection only; manifest staging and coordinated
restart remain pending.

Before manifest staging, the active manifest is confirmed as a root-owned
mode-0600 regular file—not a symlink—and both its directory and the artifact
parent are root-owned mode 0755. Revision-specific artifacts and an atomic
same-directory candidate are therefore appropriate.

The staged root-owned revision artifacts retain hashes `0ee464e9…` (agent)
and `a78778e2…` (initramfs). Strict validation of candidate kernel manifest
`8a8322a4…` passes across its kernel/module/relay paths, compatibility pins,
required config, and feature declaration. It is not active yet; an immediate
clean-host/process preflight must precede replacement and restart.

The final preflight passes on boot `e76edca2…`: both containerd namespaces,
Docker, child, storage, durable active state, host network, and exact-executable
shim/relay/server process inventories are empty. Coordinated activation can
proceed without displacing a workload.

Coordinated activation completes on unchanged boot `e76edca2…`. The previous
manifest remains available as a rollback artifact. All five units are active;
mknetd/mkruntimed/containerd/Docker have new PIDs 11328/11347/11357/11389,
zero restarts, and successful status. Active hashes are `8a8322a4…` (kernel
manifest), `55e4ea88…` (release), and `f32e4a1c…` (shim); all runtime version
reports, managed selectors, and strict bootstrap validation agree on
`a72c5cb…`. Live workload behavior remains unproven.

The exact basic runner (`808df3e0…`) proves ctr reaches `RUNNING` and Docker
passes the newly implemented AppArmor/OOM boundary. Docker's next fail-closed
rejection is `unsupported linux field(s): seccomp, sysctl`; it exits 125 and
the runner invokes cleanup. The suite is not a pass. Cleanup and the exact
requested seccomp/sysctl values must be observed before changing policy.

The initial cleanup-audit one-liner is unusable because remote quote nesting
lets jq's `| length` escape into shell syntax. It yields no cleanup evidence
and performs no intended mutation. A transferred, syntax-checked audit script
is required.

The corrected audit proves complete cleanup across default/moby, Docker,
children, storage, active durable state, host links/rules, and runtime helper
processes. Daemon PIDs remain 11328/11347 with zero restarts. Thus the failed
start leaves no resource; the next work is to observe and preserve the actual
seccomp/sysctl intent rather than merely allow their field names.

Live runc bundle inspection proves Docker's default seccomp field is a real
deny-by-default syscall policy; stripping it would be a security regression.
Explicit `seccomp=unconfined` removes the field. Docker nevertheless emits
`net.ipv4.ip_unprivileged_port_start="0"` and
`net.ipv4.ping_group_range="0 2147483647"` under `--network none`; these are
behavioral settings, not metadata. Probe containers were removed. Child
defaults must be observed before choosing implementation versus an explicit
default-value compatibility contract.

Docker's live OCI output for explicit child defaults is the exact two-string
map `1024`/`1 0`, and `seccomp=unconfined` removes the seccomp field. The safe
compatibility boundary is therefore exact-map-only acceptance/removal plus
explicit qualification flags; all partial, extra, permissive, malformed, or
typed variants and any seccomp policy remain fail-closed.

Exact-map validation/projection and explicit Docker qualification flags are
implemented. The focused suite passes 72 semantic cases, including permissive,
partial, extra, typed, and non-object sysctl rejection; Python compilation,
both qualification-script syntax checks, and `git diff --check` also pass.
This remains local until the complete gates and exact live revision run.

The full local Go race suite, vet, documentation/schema/evidence/deployment
chain, and diff check all pass with the 72-case OCI boundary. Only the already
classified local socket `EPERM` subcase is skipped. Commit and exact live proof
remain pending.

The exact child kernel defaults are `ip_unprivileged_port_start=1024` and
`ping_group_range="1 0"`, not Docker's requested permissive values. The
probe cleans normally with every inventory zero and stable daemons. Therefore
Docker's defaults cannot be stripped. The next narrow experiment is whether
explicitly requesting the child-default values plus unconfined seccomp yields
an exact OCI map suitable for fail-closed compatibility.
### 2026-09-28: Docker needs a host namespace PID, and teardown must not short-circuit

- The shim currently uses one `process.pid` field for the child-guest PID returned by the agent and for containerd's host-facing task PID. Docker consumes the latter as `/proc/<pid>/ns/net`; the live `/proc/0/ns/net` failure therefore exposes a model error, not merely a timing error. A correct implementation needs separate guest-process identity and a host-resident namespace-holder identity tied to the mknetd endpoint generation. The ordinary shim PID cannot substitute because its network namespace is the host namespace.
- Init `Delete` currently treats `CloseNetwork` as a gate for all later cleanup. The live failed deletion consequently retained the relay, shims, mknetd endpoint/firewall rules, sandbox, storage, and rootfs. Teardown should preserve the guest error for reporting but continue bounded host cleanup, because stopping/deleting the dedicated child ultimately closes its guest resources and host ownership must not depend on a successful guest RPC.
### 2026-09-28: VM restart/recovery removed the task but retained container metadata

- The disposable GCE instance is already running, but its temporary audit helper has disappeared. On first inspection, both Multikernel daemons reported `activating`; containerd and Docker were active. The default namespace contained no task and still contained the `mk-proof-ctr` container. This is a materially different post-recovery state from the earlier live leak and is preserved before further inspection.
### 2026-09-28: fail-closed reboot recovery currently becomes an infinite restart loop

- Live boot `4bd0f8ce-5c42-48d7-a19d-0379a97949ec` removed `/run` task/netns/link objects, but durable records still describe the leaked task as owned. `mknetd` repeatedly exits because endpoint generation `c315962acb7a9a27638ddb5b448a66d1` has no `mkvc315962acb7` link. `mkruntimed` repeatedly exits because sandbox generation `4edb8056f26c66fcc11e94ce812771b5` is journaled `RUNNING`, backend `ABSENT`, with an absent owned rootfs bundle.
- Refusing to guess is correct, but automatic restart without an explicit bounded terminal/operator state is not operational recovery: both units exceeded 215 restarts and continued rising. Reboot reconciliation needs a deliberate orphan policy (or a one-shot operator-action state that stops retrying), and qualification must assert bounded restart behavior.
- The reboot also left no `/opt/mkruntime/current` or `/opt/mkruntime/releases`; only installed system artifacts remain. A fresh exact-revision deployment is required before further live qualification.
### 2026-09-28: verified host namespace identity replaces the guest-PID overload

- The holder enters the endpoint namespace through `nsenter` before exec, then signals readiness. The shim compares namespace object identity against `/proc/<holder>/ns/net` before returning the PID. This is stronger than calling `setns` inside Go, where only the scheduled OS thread changes and the process-leader namespace may remain wrong.
- Host-facing init Task v2 responses/events now use the holder PID while durable agent reconciliation continues to use the guest PID. Recovery constructs a fresh holder for the recovered endpoint; rollback and deletion stop it before endpoint release.
- A failure-injected deletion test demonstrates that a guest `CloseNetwork` error no longer strands host ownership. Full repository race tests, vet, and diff checking pass. The implementation still requires an exact-revision disposable-host build and Docker rerun.
### 2026-09-28: exact live-candidate revision

The verified namespace-holder and deletion correction is committed as
`34b1f9cb2ce2bb87241a40449fff97f3ef07b3ba`. The live build must report this
exact revision; findings remain working-tree notes and are not included in the
runtime build identity.
### 2026-09-28: disposable qualification host recreated

The restart-looping instance and auto-delete boot disk were replaced. The new
instance uses the qualified `mklinux-lab-pre-daxfs-20260828-2030` snapshot,
the documented `n2-standard-16` shape, and the preserved non-auto-delete
storage disk. GCE reports it `RUNNING` at `10.148.0.58` / `136.85.39.91`;
guest-side qualification is not yet implied.
### 2026-09-28: fresh guest baseline and recoverable storage quarantine

- The new guest independently reports boot `83ef4350-79ea-475f-b73c-5d856040ad5d`, qualified kernel `7.0.0-mk2-gce-lab`, 16 CPUs, approximately 64 GiB RAM, no Multikernel children, and clean pinned Kerf/Linux trees.
- Read-only fsck passed on the exact retained 20 GiB disk identity. Instead of deleting prior evidence, its active runtime tree was renamed `reboot-quarantine-20260928-83ef4350`; a new empty runtime tree was created, and the disk was mounted with `nodev,nosuid` through its exact UUID.
- The four absent runtime prerequisites (containerd, Docker, Go, and socat) were installed successfully. Exact-source transfer/build/deployment remains next.
### 2026-09-28: exact `34b1f9c` build identities

The guest verified tracked-source archive `ba10dcd…` and built the complete
revision-stamped release. Representative hashes are release manifest
`0d64cd97…`, shim `8910812c…`, mkruntimed `b3be88f1…`, mknetd `65130b15…`,
agent `718f6bfc…`, and agent initramfs `9c9ad269…`. The pinned transport module
reproduced `bef1b888…`; static NBD/relay helpers reproduced `a0259098…` and
`293ff1ea…`. No installation or activation is implied yet.
### 2026-09-28: deployment ownership boundary holds live

The exact managed binaries were selected and bootstrap manifest `a072a17f…`
validated. Support installation then rejected the ordinary-user source tree as
an unsafe deployment input. This is the intended root-ownership boundary; no
support generation or service was activated. The retry must use a separately
verified root-owned extraction of the same archive.
### 2026-09-28: exact deployment active on the replacement host

Root-owned archive extraction produced support generation `f895d655…`. After
an empty workload/resource preflight, Docker configuration validation and
coordinated activation succeeded. The Multikernel filesystem, mkruntimed,
mknetd, containerd, and Docker are active; mkruntimed/mknetd/containerd each
show zero restarts and status 0. Docker advertises
`io.containerd.multikernel.v2`, and exact storage validation passes. No
workload claim follows yet.
### 2026-09-28: namespace-holder correction passes live Docker creation

The exact basic runner created a concurrent ctr task and Docker container;
Docker container `99d3dfa27452…` reached `running`. The previous host lookup of
`/proc/0/ns/net` is therefore cleared by the verified holder PID. The suite
next failed at ctr exec startup of `/bin/sh` with the sanitized agent error
`guest process start failed at executable`, then invoked its cleanup trap.
Cleanup audit and the deeper rootfs/executable cause remain pending.
### 2026-09-28: teardown progressed but retry is not idempotent

The failed suite left no child, endpoint, link, firewall rule, or lifecycle
sandbox, and all daemons remained stable. It did retain both stopped Task v2
objects, their shim pairs, and two rootfs files. Exact non-force deletion fails
with `waitid: no child processes` for the already-reaped relay and authenticated
`NOT_FOUND` for the already-deleted sandbox. These terminal states must be
accepted on retry so rootfs and Task metadata cleanup can finish.
### 2026-09-28: terminal teardown states are now typed and idempotent

Daemon mutation errors retain their authenticated code, allowing cleanup to
accept only exact sandbox `NOT_FOUND`. Relay teardown likewise accepts an
already-waited command or kernel `ECHILD`. Focused failure tests and the full
race/vet/diff gates pass; live recovery of the preserved stopped tasks is the
next check.
### 2026-09-28: exact retry-recovery candidate

Commit `3fcccc903b92039e9efc1becb0d67166f5e7f10b` freezes the cleanup
idempotency correction. Its tracked-source archive hashes to `75dc0e7b…` and
is the sole candidate for the preserved-task recovery attempt.
### 2026-09-28: exact recovery binary selected

The guest selected immutable release `0.1.0-dev-3fcccc9…`; shim hash is
`7964770a…` and release-manifest hash is `cf2f58ca…`. Existing stopped-task
shims still execute the prior immutable release, so selection alone does not
alter or clean them.
### 2026-09-28: dead-shim cleanup must distinguish absence from conflict

Containerd invoked the newly selected cleanup binary for both terminated old
shims, but fallback refused because the sandboxes were already absent. Both
task objects and shims are now gone; container metadata and exact `PREPARED`
rootfs records remain. A present same-ID generation mismatch must still fail,
but true absence can safely continue through the root-owned bundle/recovery
identity and rootfs service's exact ownership checks.
### 2026-09-28: fallback authorizes absence without accepting conflict

Fallback now permits a truly absent lifecycle sandbox to proceed through its
root-owned recovery and rootfs identities, while rejecting a present same-ID
generation/bundle conflict. Authenticated mknetd `NOT_FOUND` is likewise
idempotent for release. Focused tests and full race/vet/diff gates pass.
### 2026-09-28: exact absent-sandbox recovery candidate

Commit `7927f40ff2cf452a752225d1c6105f2802fb8faa` and archive SHA-256
`971b4bfd…` freeze the second fallback correction.

### 2026-09-28: recovery archive verified at the execution boundary

The recreated disposable guest independently reports SHA-256 `971b4bfd…` for
the uploaded `7927f40…` tracked-source archive. Build, release selection, and
identity-bound cleanup remain separate live checks.

The first isolated-build attempt then found that exact archive absent from
`/tmp` after creating only its unique empty `/var/tmp` extraction directory.
No extraction or compilation occurred. The same archive must be staged and
re-verified in durable `/var/tmp` before that empty directory is reused.

The user then confirmed an instance restart. On new boot `162500c3…`, the
durably restaged archive again verifies as `971b4bfd…`; all services are
active, though mkruntimed reports one restart. Both task inventories and the
child inventory are empty, both container metadata records remain, and the
runtime storage tree now appears empty despite retained daemon state files.
Because `7927f40…` has not run, this change cannot be attributed to its
fallback fix; persisted state and the boot journal must explain it first.

The retained ext4 disk is mounted correctly, not obscured. Rootfs and network
state are empty; storage retains both exports as authenticated `RELEASED`
audit records with their pre-reboot release timestamps. The live rootfs files
therefore no longer exist to exercise the new fallback. mkruntimed's single
restart was instead a boot race: initial host qualification could not yet
confirm the Google guest agent, then the service succeeded on its first
systemd retry. This restart does not constitute cleanup-fix evidence.

Durable extraction succeeded, but the first compound verifier/build command
left no manifest or binaries. Direct inspection localizes the stop before
compilation: `verify-host.sh` defaults to instance `mklinux-lab`, whereas this
replacement is `mklinux-g4-g6-final-20260905`. The retry must pass the explicit
instance identity; the silent command is not counted as a build pass.

The corrected explicit-identity verifier passes, and the guest builds the
complete `7927f40…` binary set. Exact release-manifest/shim/mkruntimed/mknetd/
agent hashes are `580e2306…`, `86787271…`, `b06c2590…`, `66043fe1…`, and
`5403c1d4…`. This proves the candidate build only; selection and live behavior
remain unclaimed.

Managed release installation selects immutable `0.1.0-dev-7927f40…` while
retaining both prior releases. Inspection reports every managed link valid,
and the public shim/mkruntimed/mknetd entry points report the exact revision.
Running services were not restarted, so this is selection—not daemon or
workload activation evidence.

An initial metadata-cleanup sequence made no change: its leading Docker
inventory used unsupported template field `.Runtime`, and `set -e` stopped
before both removals. This is a harness-format error, not runtime behavior.

The supported Docker inventory is empty. Exact default container metadata was
removed, while Docker correctly reports no engine object for the orphaned
moby containerd record. That final record must therefore be removed through
its containerd namespace before auditing the clean baseline.

Exact moby metadata removal succeeds, leaving both task/container namespace
tables and Docker empty. The subsequent broad audit used a stale kernel path
and stopped before its later checks; the live mounted ABI is under
`/sys/fs/multikernel`, so full baseline proof still requires a corrected run.

The corrected audit is empty across Multikernel instances, runtime storage,
rootfs/network ownership, Task v2 bundles, managed namespaces, links,
firewall rules, and shim/relay/storage-server processes. All four services
remain active, and the managed release resolves to `7927f40…`. Running daemon
binary identity still needs privileged inspection or coordinated restart;
selection alone is not activation evidence.

Privileged inspection confirms the old service PIDs still executed
`3fcccc9…`. After the recorded clean preflight, normal service restart places
mkruntimed PID 8100 and mknetd PID 8079 on immutable `7927f40…`; both report
the exact revision with zero restarts and status 0. The kernel manifest's
already-qualified `34b1f9c…` guest payload remains unchanged because this
candidate changes host fallback behavior only.

Read-only inspection of cached BusyBox snapshot `97e4ece8…` disproves the
initial symlink theory: `/bin/sh`, `/bin/sleep`, and `/bin/busybox` are
mode-0755 regular hardlinks to the same inode, and the x86-64 ELF hashes to
`f060103f…`. Because init `/bin/sleep` worked from that inode, the earlier
`/bin/sh` validation failure instead requires live materialized-root and
exec-time identity evidence.

An isolated live ctr task on activated `7927f40…` now executes
`/bin/busybox`, `/bin/sh -c`, and `/bin/sleep` successfully. Host `debugfs`
cannot resolve those paths while the NBD filesystem is live, so that view is
not authoritative for guest cache/mount contents. Normal teardown leaves
tasks, containers, rootfs storage, child instances, and endpoints empty. The
old shell failure is no longer reproducible in isolation; the unchanged
concurrent ctr+Docker suite remains the decisive test.

The unchanged runner (`15e12e19…`) makes the defect concurrency-specific:
ctr reaches `RUNNING`, Docker container `42fad574…` reaches `running`, and the
ctr child's first `/bin/sh` exec then fails at executable validation. This
reproduces on the clean reboot and exact activated host revision despite the
same path succeeding in the immediately preceding one-child experiment.
Second-child activation/rootfs-export interaction is now the evidence-backed
boundary; cleanup and live export state require the next audit.

The failure trap again leaves every live resource inventory empty, all four
services stable, and no warning-or-higher service journal entries. Both run
exports retain clean `RELEASED` audit records with offline-fsck attestations
and I/O counters (ctr port 4061, Docker port 4062). Teardown is not the
regression here; the missing evidence is first-export liveness during second-
child startup.

The controlled two-child probe supplies that evidence. The ctr child executes
`/bin/sh` before Docker starts. After Docker is running, direct busybox, shell,
and sleep execs all fail at `executable`; the first child still says `active`
and its export still says `ACTIVE`, but its port-4061 `mkvsock-nbd` process is
gone. Only Docker's port-4062 server remains. This is a first-root transport/
server-lifetime failure with stale durable liveness, not an executable-format
problem. Kerf could not attach because it considered the active instance's
kernel image unloaded; retained root-only server logs require a corrected
privileged read. Final cleanup was empty.

The exact root-owned ctr server log records ready, one accepted client, and a
canonical `CLOSED synced=1` with nonzero I/O counters—no crash or protocol
error. Both kernels remained active until explicit cleanup. The second child
therefore causes a clean close of the first child's mediated-storage client
while its storage lease remains falsely `ACTIVE`. Rootfs building and ELF
validation are downstream symptoms; transport connection isolation/lifetime
is the source boundary to inspect next.

Source inspection makes the cause exact. `mkvsock-nbd` leaves the 15-second
handshake socket timeout on the authenticated server stream, and its request
loop converts an idle `EAGAIN` into a clean close. Docker startup simply keeps
the first root idle past that deadline. The fix is to clear socket send/receive
timeouts after the hello exchange on both endpoints; the separate guest
`NBD_SET_TIMEOUT=15` continues to bound active, stalled block requests.

The helper now clears send/receive socket timeouts after successful hello on
both endpoints, while preserving the bounded handshake and kernel NBD request
timeout. It compiles warning-clean. A focused option-state probe is added; its
first local run was permission-gated by exact `EPERM`, so it may skip only that
known sandbox boundary and must pass unskipped on the disposable guest before
the live replay.

The complete local race/vet and documentation/evidence chain passes after the
change, as do warning-clean C compilation, shell syntax, and diff checking.
The focused option probe and the pre-existing socket test both classify only
the local `EPERM` restriction; an unskipped guest run remains required.

Commit `b9fe085` freezes only this helper correction and focused probe. Its
tracked-source archive is 1,012,019 bytes with SHA-256 `2f3a7dc4…`; findings
and the untouched historical evidence tree are excluded.

The guest independently re-verifies archive `2f3a7dc4…` and the focused probe
passes unskipped. A warning-clean dynamic helper hashes to `299b9b11…`. This
proves socket-option state on the guest, not yet production static-helper or
live mediated-root behavior.

The guest then builds a full release stamped `b9fe085c…` and a static x86-64
NBD helper. Release/shim/daemon/network-daemon/agent/helper hashes are
`d4800b4d…`, `639f0d28…`, `102bf351…`, `7e22b58d…`, `50dd8e74…`, and
`95e886d6…`. They remain build inputs pending coherent initramfs, manifest,
deployment, and activation.

The dependent agent initramfs passes gzip and archive-content checks, binds
agent `50dd8e74…` plus pinned module/relay `bef1b888…`/`293ff1ea…`, and hashes
to `440a0d96…`. It is not active until root-owned staging and strict manifest
validation complete.

Root-owned immutable staging re-matches the agent/initramfs/helper hashes.
Strict bootstrap validation accepts candidate manifest `1b31a818…`, binding
those artifacts to the unchanged pinned kernel, module, relay, compatibility,
transport, and OCI feature contract. Activation remains unclaimed.

The same-boot activation preflight is fully empty across workload, child,
storage, rootfs, network, and exact runtime-process inventories. All four
services are healthy and unrestarted. This authorizes atomic activation; it
does not itself prove activation.

Coherent activation succeeds on the same boot. The previous helper is retained
root-only; active helper/manifest hash to `95e886d6…`/`1b31a818…`, all public
components report `b9fe085c…`, and daemon PIDs 14857/14838 resolve to that
immutable release with zero restarts/status 0. This is activation evidence;
the >15-second idle-root replay remains decisive.

The decisive replay passes. Exact-helper PID 15163 survives a 20-second idle,
then remains the same while Docker startup consumes another 57 seconds and a
separate port-4062 server appears. First-child busybox, shell, and sleep execs
all succeed afterward; both exports are concurrently `ACTIVE`. Normal cleanup
is empty across both namespaces, Docker, child, storage, rootfs, and network
state. This directly closes the idle-session root cause; the unchanged full
suite remains the broader qualification gate.

The unchanged suite confirms the fix at its original boundary: concurrent ctr
network exec returns a distinct child boot ID, `mkn0` address, outbound HTTP,
and `CTR_NETWORK_PASS` after Docker startup. It then reaches a new host-side
gap: Docker exec is rejected as `unsupported exec process field`. The exact
field/value pair must be captured before applying the existing narrow Docker
init compatibility contract to exec.

The suite trap is fully clean across workload, child, storage, rootfs,
network, and exact-process inventories. All services remain stable with no
warnings. The next correction is therefore confined to fail-closed exec OCI
validation.

Exec validation now mirrors the narrow init contract: only explicit
`unconfined` AppArmor and zero OOM adjustment are accepted and omitted; every
requested profile, nonzero adjustment, and other unsupported field still
fails closed with fixed field names only. Focused and full shim race tests pass;
full-tree and live Docker exec checks remain.

The full race/vet and documentation/evidence chain passes. Only the known
local socket `EPERM` paths skip; generated cache is removed and the diff is
clean. Exact commit/build/live replay remain.

Commit `aa1238b83a06ecac63f1d3786993e201556767e6` freezes the narrow exec
correction atop the live-proved idle-root fix. Its tracked-source archive is
1,012,377 bytes and hashes to `4f16e301…`.

Independent guest build succeeds for exact `aa1238b…`: release/shim/daemon/
network-daemon/agent hashes are `6f67cfe1…`, `18e3a013…`, `fe3fe773…`,
`9cff4985…`, and `cf73a7e3…`. The static helper reproduces live-proved
`95e886d6…`; dependent initramfs is `51e0c02e…`. Activation remains separate.

Root-owned staging re-matches all artifacts, and strict validation accepts
candidate manifest `4627d38b…` with the pinned dependencies and feature set.

Exact `aa1238b…` activation succeeds after empty-state preflight. The active
manifest/helper hashes are `4627d38b…`/`95e886d6…`; public components report
the exact revision, and daemon PIDs 19891/19872 execute its immutable binaries
with zero restarts/status 0. Containerd and Docker remain healthy on unchanged
PIDs 1341/1472, and boot ID remains `162500c3…`. This is coherent activation;
the unchanged live suite remains the behavioral qualification gate.

The pinned suite rejected an accidental root-shell invocation at its explicit
ordinary-user guard before creating workloads; its trap had nothing to clean.
That attempt carries no qualification result. The suite must run as the SSH
user and elevate only its own scoped operations.

The unchanged runner `15e12e19…` then passes end to end on exact `aa1238b…`.
ctr/Docker children have distinct non-host boot IDs `a7d02acc…`/`05157cb3…`,
independent `mkn0` addresses `172.31.0.2/30`/`172.31.0.6/30`, outbound HTTP,
and successful ctr plus Docker exec. Direct cross-child ping is blocked.
SIGKILL yields ctr `STOPPED` and Docker exit 137; normal removal clears the
runner's child, link/rule, container, task, and Docker inventories. The runner
exits 0 with all four terminal PASS markers, including `G4_G5_G6_MVP_PROOF_PASS`.
An independent residual-state and service-health audit is still separate.

The independent audit is clean: both containerd namespaces, Docker, child
instances, endpoint/rootfs state, active exports and backing artifacts,
`mkv*` links, MK rules, and exact shim/relay/NBD processes are absent.
Storage retains only intentional `RELEASED` history; applied configfs nodes
are transaction history, not live instances. All 16 CPUs are available and
`MemAvailable` is 64,470,928 kB. All four services retain PIDs
19891/19872/1341/1472, zero restarts/status 0, and no warning-or-higher journal
entries in the run window. Cleanup and service stability are corroborated.

This remains an MVP qualification, not full gate closure. The machine-readable
contract requires 10 G4, 8 G5, and 14 G6 assertion groups beyond this runner.
The replacement evidence directory currently contains only the before-cloud
ledger, with no final manifests/transcripts/after-ledger/hash index, so the
final evidence auditor cannot pass yet. The fuller shared feature matrix is
the next live boundary; missing assertion groups must be executed, not inferred.

Exact fuller matrix `66bfbe65…` starts clean on `aa1238b…`: zero children,
links, rules, ctr tasks, and Docker objects. BusyBox resolves to OCI/repository
digest `sha256:73aaf090…` and Docker reports `amd64`; the shared image pull and
inspection row passes. Split ctr creation is the next live step.

The fuller matrix then fails closed at Docker start, after ctr split start,
because Docker's generated read-only `/etc/resolv.conf` bind differs from the
adapter's enforced option contract. The runner omits the basic suite's global
read-only root, exposing a distinct generated-mount variant. Its trap removed
both workloads. Exact options and independent cleanup are required before any
compatibility change.

Independent failure cleanup is complete across both containerd namespaces,
Docker, child/network/rootfs state, active exports/artifacts, links/rules, and
exact runtime processes. All four service PIDs remain unchanged with zero
restarts/status 0.

Stock-runc capture identifies the exact variant: Docker binds its same-ID
managed `/etc/resolv.conf`, `/etc/hostname`, and `/etc/hosts` files from
`/var/lib/docker/containers/<64-hex-id>/…` with `["rbind","rprivate"]`.
They are deliberately writable inside a writable-root container. Generic
writable host binds must stay rejected; the safe compatibility model is a
narrow identity-bound seed copy into the private root with no host write-through.

That model is now implemented. Only the three exact Docker paths with matching
64-hex root/source identity and exact `rbind,rprivate` options become private
writable seed copies; they never enter the guest as host mounts. Directory
seeds, arbitrary writable binds, mismatched identity/name/destination, and
extra options fail closed. Focused materialization, 90 OCI semantic cases, and
the privileged rootfs verifier race test pass. Full-tree and live replay remain.

The full race/vet tree and complete documentation/evidence contract chain now
pass; only the known local socket `EPERM` subcase skips. Exact commit, guest
build, activation, and fuller-matrix replay remain separate.

Commit `1832e62ad6c54128f0fcd25df6ce7ed16607b3e0` freezes the tested
private-seed change. Its tracked-source archive is 1,014,109 bytes and hashes
to `6c6f0b6f…`; findings/evidence are excluded. Guest build and live replay are
still separate claims.

The guest re-hashes the archive, passes host verification, and builds exact
`1832e62…`. Release/shim/mkruntimed/mknetd/agent hashes are `d3b4eab8…`,
`12f01ba7…`, `2f56955e…`, `9cd72078…`, and `23acdcad…`; its dependent
initramfs is `44a9e493…`. An ad-hoc unchanged-helper build does not reproduce
the proven helper hash, so it is excluded; activation will preserve exact
live-proved `95e886d6…`. Staging and activation remain separate.

Binary install selected immutable `1832e62…`, but support installation correctly
refused the user-owned extracted assets before mutation. Existing daemons still
execute `aa1238b…`; support and kernel manifest remain old. This mixed selected-
link/old-process state is not activation and will run no workload before a
root-owned support stage plus coordinated empty-state restart.

Root-owned staging now succeeds: support generation `809c5033…` is selected,
and strict privileged validation accepts candidate manifest `3f2300d6…` with
the exact agent/initramfs and pinned kernel/module/relay. A later non-root hash
command stopped only on the intended mode-0600 initramfs; privileged re-hash
and manager inspection are still required before activation.

Privileged re-hash matches `3f2300d6…`/`23acdcad…`/`44a9e493…`, and both
managers report the intended generations with managed links. The combined
preflight then stopped on an incorrect `/usr/local/bin` daemon path before its
inventories; it provides no empty-state or activation result.

The corrected inventory is fully empty, but the user restart establishes a
new authoritative boot `7d620d19…` and PIDs 1456/1200/1340/1458. Services are
active/status 0; mkruntimed has one restart while the other three have zero.
Current-boot journal and executable-path inspection must qualify this fresh
baseline before activation; earlier boot/PID observations are historical.

The journal explains mkruntimed's one restart: its first boot attempt failed
closed while the Google guest agent was not yet confirmed active, then the
single retry passed storage/host startup and remained active. Daemon executable
paths are still immutable `aa1238b…`; the retry is bounded boot ordering, not
runtime-state instability. Empty-state coordinated activation may proceed.

Exact `1832e62…` activation is now coherent. Active manifest/helper hashes are
`3f2300d6…`/preserved proven `95e886d6…`; strict validation resolves the new
agent/initramfs and pinned dependencies. New daemon PIDs 7423/7404 execute the
immutable revision with zero restarts/status 0; containerd/Docker stay healthy
at 1340/1458. Boot remains `7d620d19…`. Fuller matrix replay is still required.

Live replay closes the private-seed defect: normal writable-root Docker reaches
`running`, exec/stdio works, both children expose distinct non-host boot IDs and
the exact child kernel, and their private root values remain distinct. The
matrix then reaches a new environmental boundary: the next bind-input child is
correctly refused by storage high-water (`2,684,854,272` free versus
`3,233,402,880` required). The trap removed workloads/temp input. Cleanup and
disk-ownership audit must precede any capacity change.

Cleanup is otherwise empty and services remain stable. Exact retained disk
`/dev/sdb`/`mk-mediated-storage-20260830` is 20 GiB; preserved child fixtures
consume 8 GiB and quarantines 4 GiB, leaving 6.98 GB clean. Two live 2 GiB
roots explain the 2.68 GB third-root refusal. Evidence will not be deleted;
the safe correction is exact-disk expansion plus online ext4 growth. A mistyped
active-export jq projection still needs a separate rerun.

The corrected active-export result is empty, and GCE confirms exact disk
`mk-mediated-storage-20260830` is the attached 20 GB zonal `pd-balanced`
device. The attempted 30 GB resize was rejected by the safety reviewer as a
persistent billable mutation lacking separate explicit approval; nothing was
changed. Explicit user authorization is required for resize plus online ext4
growth.

Harness-only commit `958a0f6…` adds focused live read-only-bind qualification,
hash `a107ed08…`. It runs ctr and Docker sequentially with the same directory/
file bytes, guest write rejection, host immutability, and clean inventories,
avoiding only the accidental three-root overlap. Syntax/static checks pass;
live execution remains pending.

The exact uploaded harness starts with all measured inventories at zero. ctr
reads the expected directory/file bytes, both guest writes fail read-only, and
both primary-side sources retain their exact original bytes. ctr materialization
and no-write-through are live-proven; Docker remains in progress.

Docker also reads its exact directory/file values, both guest writes fail
read-only, and host sources remain unchanged. Exact harness exits 0 with
`RUNTIME_READONLY_BIND_LIVE_PASS` and every measured final count zero. Shared
ctr/Docker read-only materialization and no-write-through are live-proven;
independent durable/process cleanup remains separate.

The deeper audit finds that cleanup is not complete despite the harness zeros:
moby retains stopped task/container `7ab0dc6d…`, its prepared rootfs/image, and
two exact shim processes, while Docker metadata and live child/network/export
are absent. The harness omitted moby namespace and process/rootfs inventories.
Bind enforcement remains proved; cleanup is now a diagnosed-open requirement.

The orphan does not converge. Normal Task deletion fails exactly at `stop
network namespace holder: waitid: no child processes`: cleanup already reaped
the owned holder, but retry treats `ECHILD` as fatal and stops before rootfs,
Task acknowledgement, and shim exit. Holder termination needs the same strict-
identity/already-reaped idempotency semantics as the corrected relay path.

Holder stop now accepts exact `ECHILD` as terminal after owned-process kill/
already-done handling, while preserving all other wait failures. Delete retry
tests combine an already-reaped relay, directly reaped holder, and absent
sandbox, and require all ownership cleared. Twenty race-enabled repetitions
pass; full gates and live orphan recovery remain.

Full local qualification now passes: race-enabled full-tree Go tests, `go vet`,
the complete documentation/schema/evidence/runtime checker, and whitespace
validation. The first Go invocation omitted the required temporary cache and
failed only against the sandbox's read-only default cache; rerunning with
`GOCACHE=/tmp/mklinux-gocache` passed. Live orphan recovery and replay remain.

The implementation is committed narrowly as
`586e3b932857d3e1372fbc93f85b6c592abd44d1`. Its exact 1,015,225-byte archive
has SHA-256
`806260d47def435e7a243e5d2de752115085233c45c9e3f46ced5abb539d8087`;
guest transfer/build/activation and live recovery remain pending.

The restarted disposable VM remains on boot
`7d620d19-11d5-44fa-816b-0e1cd87c8cf6`, kernel `7.0.0-mk2-gce-lab`, with 16
CPUs and all four runtime services active. Its independently measured archive
hash and 1,015,225-byte size exactly match the local candidate. No build or
activation is claimed yet.

Explicit-instance host verification passes and the guest builds the full exact
revision. Release manifest/shim/mkruntimed/mknetd/agent hashes are
`7d3a7a9c…`/`a1eeef5d…`/`9d71a29e…`/`cbd8344f…`/`c7e97f70…`, and the built
public components report revision `586e3b9…`. Installation, activation, and
live recovery remain separate claims.

The pre-activation orphan is unchanged: stopped moby task/container `7ab0dc6d…`,
old immutable `1832e62…` shim PIDs 9687/9692, and the exact 2-GiB prepared
root. Services remain active, zero-restart/status 0. Since those processes
cannot consume a new public symlink, recovery will exercise authenticated
crash/reboot cleanup after activation, without manual state/evidence deletion.

The binary manager installs and selects immutable release `0.1.0-dev-586e3b9…`;
all managed links validate and public components report the exact new revision.
Old daemon/orphan processes remain immutable `1832e62…` executables until
reboot, so selection is not yet coherent process activation or recovery proof.

After controlled reboot the boot ID is `75c6e9b5…`; running daemons resolve to
exact `586e3b9…`. Built-in recovery removes the stale task, old shim pair,
child, prepared 2-GiB root, rootfs record, endpoint, helper processes, and MK
rules without manual state deletion. Only inert moby container metadata
`7ab0dc6d…` remains (Docker has no object). One nested-SSH `awk` link query was
misquoted and is explicitly non-evidence pending a direct retry; journal/state
attribution and normal exact metadata removal remain.

The direct link retry and rootfs JSON prove zero links/records. mkruntimed's
single retry is the bounded guest-agent boot race, followed by stable service;
containerd has no current-boot warnings. Normal removal of exact inert metadata
succeeds and all resource inventories are zero. A `pgrep -f` count of two is
excluded because it self-matched the remote command text; an anchored process
query remains required.

Anchored executable queries find no shim, relay, or NBD helper. The new boot
and four active services remain stable, closing prior-orphan recovery; exact
focused replay and an independent deep audit remain the regression gate.

The exact pinned focused harness passes on active `586e3b9…`: ctr and Docker
read both expected inputs, reject all four guest writes as read-only, preserve
host bytes, and finish with six narrow zero inventories plus
`RUNTIME_READONLY_BIND_LIVE_PASS`. The independent deep cleanup audit remains
mandatory because those counters missed the original orphan.

The independent post-replay audit is fully empty across default/moby metadata,
Docker, children, runtime roots/records, endpoints, anchored shim/helper
processes, links, and MK rules. Boot and service PIDs/restart counts are
unchanged from the post-reboot baseline. This live-closes the namespace-holder
`ECHILD` cleanup regression.

Qualification clean markers now cover both containerd namespaces, Docker,
runtime artifacts/rootfs records, endpoints, and anchored shim/helper processes.
The full matrix asserts the all-zero value at all three checkpoints. Bash
syntax and diff checks pass; ShellCheck is unavailable locally and unclaimed.
Live execution of the broadened focused harness remains.

Exact broadened harness `4e196108…` begins with all 13 counters zero and both
bind cases pass, but it correctly withholds its final marker because two shim
processes remain immediately after Docker cleanup while all durable/resource
counters are zero. This is not yet classified: bounded convergence must be
measured before adding a wait or diagnosing a new orphan.

The pair disappears on follow-up while all durable state remains empty, so the
immediate count is normal asynchronous exit. Both harnesses now wait up to 30
seconds for the full inventory and retain the last nonzero value on failure.
Syntax/diff checks pass; focused/matrix hashes are `05a3ca31…`/`0d94685c…`.
Final focused replay remains.

The guest matches final focused hash `05a3ca31…`. Both bind cases pass; its
deep checkpoint observes the transient shim pair, converges within 30 seconds,
then emits all 13 counters zero and `RUNTIME_READONLY_BIND_LIVE_PASS`. The
stronger marker is live-qualified; full-matrix execution still needs capacity.

The complete documentation/schema/evidence/runtime chain, Bash syntax, and
diff checks pass. Uploaded hashes match local source. ShellCheck is unavailable
on both hosts and remains explicitly unclaimed; generated caches were removed.

The independent bind row now runs after the long-lived concurrent pair is
deleted and deep-cleaned, followed by its own deep-clean assertion. This keeps
the concurrency/isolation proof intact while avoiding the unrelated third-root
high-water collision without disk mutation or evidence deletion. Matrix hash
`26d67e80…` passes syntax/diff checks; live execution remains.

Exact matrix `26d67e80…` starts from all 13 zeros and passes image, split
lifecycle, state, distinct child/kernel, exec streams, private roots, outbound
DNS/HTTP, and bidirectional sibling isolation. Immediately after the matrix
restarts mkruntimed, ctr exec fails at the guest executable boundary. The run
exits 1 via its trap: capacity is no longer the blocker, but daemon-restart
continuity is genuinely failing. Cleanup/journal diagnosis remains.

The trap cannot clean while mkruntimed restart-loops: two tasks/containers,
children, roots, shim pairs, and NBD helpers remain. Every retry rejects the
owned roots because writable ext4 bytes differ from the immutable preparation
digest. Reconciliation already has exact lifecycle ownership and held/static
artifact checks, so verification now skips only the mutable-data digest for an
exact owner; preparation/boot remain strict. Tests cover both policies,
mutated writable data, and unchanged rejection of static artifact changes.
The race-enabled rootfs package passes. A harmless first gofmt path invocation
changed nothing; its corrected retry succeeded.

The explicit stop ends the loop after 28 failures while preserving both exact
tasks/roots. Full race tests, vet, and diff checks pass. Fix commit
`783e15df242dee7240bc69199b3d0a41fb38f596` has a 1,016,223-byte archive with
SHA-256 `0f627579adfd3d5198c1c0b6cc05549926eca507585caaf0842773e00d4ec543`;
guest build/deployment/recovery remain.

The guest matches the archive, host-checks with both children still owning
CPUs, and builds exact `783e15d…`; release/shim/daemon/network/agent hashes are
`c8db0945…`/`c26e4874…`/`74f51e11…`/`822dc743…`/`927263fe…`. After atomic
selection, new mkruntimed PID 12956 stays active for 12 seconds with zero
restarts while reconciling both mutated roots. Child exec and cleanup remain.

The trap had already stopped both tasks, so post-recovery exec correctly returns
failed precondition and is not claimed. Normal task/container removal succeeds
with expected exit 137. Deep inventories are fully empty and all services are
active/zero-restart after reset. Fresh matrix replay must prove continuity.

Capacity-safe ordering is commit `b3894d1`; mknetd is coherently moved to
`783e15d…`. Fresh matrix replay reaches sibling isolation with new distinct
children, then exec still fails after deliberate mkruntimed restart. The fixed
daemon now reconciles stably, narrowing the remaining defect to mediated
storage/session continuity across daemon/helper replacement. Later rows remain
unclaimed pending audit and cleanup.

Later authoritative audit finds both task namespaces and NBD-helper inventory
empty, with mkruntimed active at PID 14651 and zero restarts. The trap therefore
eventually cleaned the failed run once the daemon stayed serviceable; this is
cleanup convergence, not live-restart continuity.

The storage backend already requires NBD servers to outlive the daemon and
authenticates adoption by durable generation plus PID/start-time/argv/image-fd/
binary identities. The unit's implicit `KillMode=control-group` contradicts
that design by killing helpers on daemon restart. Replacement servers cannot
repair the child's dead NBD session. The deployment needs process-only stop
semantics so exact helpers survive and are adopted.

The managed unit now explicitly uses `KillMode=process`, documented as the
exact-helper adoption contract. Deployment tests require exactly one installed
directive. Focused deployment, complete docs/evidence/runtime validation, and
diff checks pass; managed live installation and restart replay remain.

The unit fix is commit `43d038f`; its exact 1,016,303-byte archive hashes to
`ebf81becd5a6caa26098c7d350fd66250eff30f95ba0e08cbc7c162c1188c400`.
Guest verification and managed deployment remain separate.

The guest matches the archive. Root-owned managed deployment `40d4a96e…`
installs/selects with all links valid; after daemon reload, systemd reports
`KillMode=process`. Live helper survival/adoption remains the matrix gate.

The decisive replay proves helper adoption: mkruntimed PID changes
14651→26854 and both clients exec successfully with unchanged child boot IDs.
Restart continuity, pause/resume, and signal/exit pass. Normal ctr deletion then
fails at `DeleteSandbox: BACKEND_FAILURE`; later cleanup rows remain unclaimed
while lifecycle backend deletion after adoption is diagnosed.

The immediate audit shows why this must not be collapsed into an adoption
failure. Exact NBD helper PIDs 26194/26445 remain in the systemd cgroup during
the deliberate daemon stop, and systemd warns about those expected leftover
processes as PID 26854 starts; the daemon stays active with zero restarts.
Docker's instance 41 then halts, unloads, and is removed with resources
returned. The ctr instance 40 reaches the kernel's halted state, but the
captured sequence has no successful unload/removal before `DeleteSandbox`
returns `BACKEND_FAILURE`; default still contains stopped task
`mk-matrix-ctr`, while moby is absent. The lifecycle snapshot is not at the
initially guessed `/var/lib/mkruntimed/state.json` (only rootfs/storage state
exists there), so diagnosis must locate the configured state directory and
capture the exact failing backend operation without manually deleting state.

Configuration locates lifecycle state at `/var/lib/mkruntime`, separate from
the rootfs/storage snapshots. The retained ctr root remains `PREPARED`, while
its exact export generation `956783ba…` is durably `QUIESCING` and its NBD
helper has exited. `DeleteSandbox` therefore reached `releaseStorage`, stopped
the adopted export, and failed before committing `RELEASED`; Kerf unload/delete
is later in the lifecycle method. The halted-but-not-removed kernel instance is
a downstream result, not the initiating operation. Services remain healthy and
mkruntimed has zero post-restart failures.

The retained artifacts make the initiating error concrete. Lifecycle journal
sequences 299–302 contain two retryable delete failures, and the exact export
log is canonical and terminal: READY for generation `956783ba…`, client
accepted, then `MKNBD_SERVER_CLOSED synced=1` with 265 reads, 49 writes, and 12
flushes. Its process record remains although the helper is absent. A subsequent
read-only `e2fsck -fn` completes all five passes and exits 0. After adoption,
the child halt can close NBD and let the server exit before `Release.Stop`;
because the new daemon has no in-memory `managed` entry, `Stop` rejects the
absent process without parsing the exact graceful-close record. `Observe`
already treats that same record as authoritative closed evidence. This
asymmetry strands an otherwise clean export in `QUIESCING`; retry-safe cleanup
must validate the terminal log and remove only the exact retained process
record before the existing offline check.

Recovery now treats that narrowly authenticated terminal state as idempotent.
`Observe` and `Stop` accept an already-exited server only when the exact
lease-specific log has both its canonical READY marker and terminal `synced=1`
close record; they return those counters and remove only the identity-matched
process record. Missing, malformed, or conflicting evidence remains a hard
failure. A focused test covers both observation and direct release-stop after
recovered process exit. Twenty race-enabled repetitions also include the
existing live-process adoption and counter-integrity tests and pass; formatting
and diff checks pass, with full-tree qualification still pending.

The complete local gate passes: all Go packages under the race detector,
repository-wide vet, the documentation/schema/evidence/runtime chain with 90
OCI semantic cases, and diff checking. The sole checker skip is the already
classified sandbox-local socket `EPERM` case. Generated Python cache files were
removed and the user-owned evidence tree was not changed. Commit/build/live
recovery and a clean matrix replay remain unclaimed.

The exact fix commit is `e37094bd19d15a792eaf802eb49f328a54c60549`;
its 1,016,611-byte archive hashes to
`6c49d1714853dbf63887113c3b6dc858ea7474ca5e5b213caf52c859cd5cf638`.
The living findings remain uncommitted by design and retained evidence is
unchanged. Guest transfer/build/deployment and live recovery remain separate.

The guest independently matches the archive and passes explicit-instance host
verification before producing the full revision-stamped release. Exact hashes
for manifest/shim/mkruntimed/mknetd/agent are `daa7be65…`, `512e2020…`,
`d9fea9b1…`, `fd687ade…`, and `5dfefe3f…`. This proves the guest build only;
selection, daemon restart, retained-task recovery, and replay remain unclaimed.

The immutable `e37094bd…` release installs with every managed link valid.
After the explicit daemon restart, PID 28723 executes that exact release and
remains active with zero restarts. Startup reconciliation authenticates the
terminal helper transcript, removes the exact stale process record, completes
the offline check, and advances export `956783ba…` to `RELEASED`, preserving
265 reads, 49 writes, and 12 flushes. The stopped lifecycle sandbox, task, and
halted child are intentionally retained for the normal caller-owned delete
retry; this is storage recovery, not yet complete task cleanup.

The unchanged normal retry `ctr -n default tasks rm mk-matrix-ctr` then exits
0. Its warning carries the workload's already-recorded exit status 42;
lifecycle advances to sequence 303 with no sandbox, the task disappears, and
the kernel instance is removed. mkruntimed stays active at PID 28723 with zero
restarts. This live-closes the stranded delete path itself; inert metadata
removal and an independent deep-clean audit still follow.

Normal ctr metadata removal succeeds, and the independent deep inventory is
empty across both namespaces, Docker, children, runtime artifacts, rootfs and
network records, executable-matched shim/relay/NBD processes, host links, and
MK firewall rules. All four services remain active/status 0 with zero
restarts. mkruntimed executes exact `e37094bd…`; idle mknetd still executes
immutable `783e15d…`, so it must be restarted coherently before the full replay.

Idle mknetd restarts as PID 29500. Both runtime daemons now execute exact
immutable `e37094bd…`, active/status 0 with zero restarts. The guest's matrix
hash independently matches pinned `26d67e80…`; decisive replay is next.

The first command mistakenly ran the harness itself under `sudo`; its UID
safety preflight refused before any qualification and its empty-state trap had
nothing to clean. This operator invocation is excluded. The unchanged harness
must run as the ordinary SSH user and use its own scoped sudo calls.

The corrected ordinary-user replay begins on boot `75c6e9b5…`, kernel
`7.0.0-mk2-gce-lab`, with all services active and all 13 initial inventories
zero. Both clients inspect BusyBox 1.36 at shared digest `73aaf090…` and amd64;
the image row passes. The same live handle is now in split create/start, so no
later row is claimed yet.

The same live process passes split lifecycle, inspection, distinct child boot
IDs (`60dc93e8…`/`a7834fc8…` versus host `75c6e9b5…`), exec streams, private
writable roots, outbound mediated networking, and sibling isolation. It also
passes deliberate daemon-restart continuity with unchanged child identities,
pause/resume, signal/exit 42, and normal deletion for both clients. The
post-delete deep inventory watches shim processes converge 4→2→0 and ends with
all 13 fields zero. The fresh-workload delete regression is therefore closed;
bind and later matrix rows are still running.

The ctr bind child now reads exact values
`ctr-host-immutable|ctr-file-immutable`, and both guest writes fail read-only.
Docker's independent bind case is in progress; host-source immutability and the
combined bind row remain unclaimed until its assertions complete.

Docker independently reads `docker-host-immutable|docker-file-immutable` and
both writes fail read-only. Every host directory/file source retains its exact
bytes, and the post-bind deep inventory converges to all 13 zeros. The combined
bind row passes for both clients; repeated attach/error cycles are now running.

Repeated-exit cycle 1 returns exact status 17 for ctr and Docker, preserves both
clients' stdout/stderr markers, and Docker inspect/wait agree on 17. Normal
cleanup returns child/network resources to empty before cycle 2 begins; the
second cycle is not yet claimed.

Cycle 2 repeats exact status 17, stdout/stderr, Docker inspect/wait, and clean
teardown. The combined foreground wait/I/O/nonzero-exit row and same-name reuse
therefore pass for both clients. Stdin forwarding and later terminal/OCI rows
are still in progress.

Guest stdin forwarding passes with exact outputs `guest-ctr-stdin` and
`guest-docker-stdin`, followed by normal cleanup. Attach-to-detached-task is
now running; it and later rows remain unclaimed.

Attach to detached tasks passes with exact outputs `ctr-attached-stdin` and
`docker-attached-stdin`, followed by clean teardown. Pseudo-terminal allocation
and resize is in progress; terminal and later OCI rows remain unclaimed.

TTY exposes a new Docker admission boundary. ctr obtains a real terminal at
requested size `37 91` and prints `ctr-terminal-ok`; Docker is rejected before
child creation because its OCI request includes `process.consoleSize`, which
the validator currently labels unsupported. The matrix exits 125 and runs its
trap. No combined TTY or later row is claimed; cleanup audit and exact bounded
console-size support are required.

The independent post-failure audit is fully clean across both namespaces,
Docker, children, durable root/network state, runtime files, exact helper/shim
executables, links, and MK rules. All services remain active/status 0 with zero
restarts; mkruntimed PID 30966 is the deliberate restart already proved by the
matrix. The admission rejection leaked no resource.

Console-size support is now exact rather than permissive. Admission requires
an integer `{width,height}` object, `terminal=true`, and both dimensions within
0–65535; malformed, incomplete, boolean, oversized, unknown-field, and
non-terminal variants fail closed. The guest projection and authenticated
agent spec preserve it, and the agent applies it only when no Task resize
overrides it. Init and exec paths share the rule. Twenty race-enabled focused
repetitions, including an observed 91×37 PTY, and the expanded 95-case OCI
validation suite pass. Full gates and live replay remain.

The complete local race suite, vet, documentation/schema/evidence/runtime
chain with 95 OCI cases, and diff checking pass. The only skip is the known
sandbox-local socket `EPERM` subcase. Generated Python caches were removed and
retained evidence was not changed. Commit/build/deployment and live replay are
still separate.

Before deploying the coherent console-size release, reinspection after the VM
restart records the rollback baseline: all four services are active, managed
support remains generation `40d4a96ec0ac3c89fc1e5b3b6bcde4d1c4fa2072e76e6ec7755a66840c1cd55a`,
the loaded unit reports `KillMode=process`, and the active kernel manifest
hashes to `3f2300d68070d7ca333ecd9711c9ef286cbf78a2b9da976f6e376a292163ab42`.
The selected runtime and host inputs are root-owned regular mode-0600 files,
and cleanup inventory reports no finding. No deployment change is claimed yet.

The VM then builds an initramfs from the exact `1f81cb2…` agent together with
the already pinned transport module and relay. The agent remains
`c79210c60289e4f4fe14ae14e2c142e109002f96c77949e96a192397b75db83d` and
the initramfs is
`2dec85b8ee8d8fb7b4e1b601aacc96f99f63e1ba61e3c2e5478bad295c88b21d`.
The active manifest is retained as the candidate template so its kernel,
relay, module, compatibility, protocol, required-config, and feature pins do
not drift. This is staging only; active state and services are unchanged.

Root-owned exact-source installation selects immutable binary release
`0.1.0-dev-1f81cb2aec7f4774c89506c71eb8348c37147e9e` and managed-support
generation `d36b6940116af48cc13668f8443a87f0c69f3eb979f048a071ea11c586e64e3e`.
The root-owned candidate manifest hashes to
`1d79c5644caef494e96453495795d83d71d1c35ac9043a84aac3d878e70999f2`
and passes strict bootstrap validation against the exact new agent/initramfs
and the preserved kernel/module/relay pins. All services remain active and the
active manifest is intentionally still `3f2300d6…`; coherent activation and
process replacement are not yet claimed.

Coordinated empty-host activation stops only mkruntimed/mknetd, retains the
old `3f2300d6…` manifest under a non-JSON rollback name, atomically activates
`1d79c56…`, reloads systemd, and starts both daemons. The active path passes
strict bootstrap validation. PIDs 38582/38547 resolve to immutable
`1f81cb2…` mkruntimed/mknetd binaries and both report that exact revision;
they and containerd/Docker are active, with zero runtime-daemon restarts and
status 0. mkruntimed still loads `KillMode=process`, the active OCI validator
hashes to `ddd77fb869fb258268fc5d6cadacc6ca664f3753a5b34cf747331dab087b8804`,
and cleanup inventory emits no finding. Deployment coherence is proved;
workload qualification remains separate.

The guest copies of the matrix and its live-resize helper match local hashes
`26d67e809f7d380507cef21b8cc1d1c4ac1551e3485d1c2109a53bbda23e492d`
and `30c27d36d313a7bc54acfcf1a10a24da31007518220b87be77460401a260d58a`.
Bash syntax and Python byte-compilation succeed, and all four services are
active immediately before execution. No new live-matrix result is claimed yet.

The exact matrix starts as ordinary UID 1001 on authoritative host boot
`75c6e9b5…` and kernel `7.0.0-mk2-gce-lab`, with all four services active.
Its initial broadened checkpoint has all 13 inventories zero. ctr and Docker
both pass BusyBox 1.36 inspection at shared digest `sha256:73aaf090…` on
amd64. The same process is entering ctr split create/start; later rows remain
unclaimed.

The run then passes split create/start, state/inspect, distinct child boot IDs
(ctr `c9b0ab66…`, Docker `c91d47c5…`, host `75c6e9b5…`), exec split I/O,
private writable roots, distinct mediated `/30` links with DNS/HTTP,
bidirectional sibling isolation, daemon-restart continuity with unchanged
child identities, pause/resume, signal/exit 42, and normal deletion. The
post-delete checkpoint waits through transient shims and reaches all 13 zeros.
Read-only-bind testing is now running; later rows remain unclaimed.

Both read-only-bind cases pass with exact ctr/Docker directory and file bytes;
all guest writes fail read-only and all four host sources retain their bytes.
The post-bind checkpoint waits through transient shims and reaches all 13
zeros. Repeated nonzero-exit/name-reuse cycles are now in progress.

Both repeated cycles return exact status 17 for ctr and Docker, preserve the
client-specific stdout/stderr markers, agree with Docker inspect/wait, clean
normally, and successfully reuse the same names. Foreground wait/I/O/nonzero
exit and name reuse therefore pass for both clients. Stdin forwarding is now
running.

Guest stdin forwarding passes with exact `guest-ctr-stdin` and
`guest-docker-stdin` outputs, followed by normal cleanup. Attach to detached
tasks is now running; terminal/resize and later OCI rows remain pending.

Attach to detached tasks passes with exact `ctr-attached-stdin` and
`docker-attached-stdin` outputs and clean teardown. At the former boundary,
ctr obtains a real PTY at `37 91`, while Docker now passes `consoleSize`
admission and reaches guest execution. Docker's observed size and both live
resize probes remain required before the terminal row can pass.

The terminal row still fails, now beyond the fixed Docker admission boundary.
Docker reports exact `37 91` and `docker-terminal-ok`, but ctr's captured
stream is `$'^@37 91\r\r\nctr-terminal-ok\r\r'`. Its exact size-line assertion
rejects the unexpected leading literal `^@`; the trap runs before either live
resize helper or later OCI rows. No combined terminal/resize pass is claimed.

Independent audit is fully clean across the 13 resource inventories; all four
services are active/status 0 with zero restarts, and mkruntimed PID 40047 is
the matrix's intentional restart. A focused ctr reproduction captures 28 raw
bytes at SHA-256
`23ece05ab748b163823d566a707106fafe4bf8056e0e602bca1170be57aa8bdf`.
Its byte dump begins `5e 40` (`^@`), proving a literal echoed control notation
rather than output rendering of a retained NUL. Focused cleanup succeeds.

A guest probe switches the PTY raw and reads the queued byte as exact hex
`00`: ctr supplied a NUL on terminal stdin, Multikernel forwarded it, and the
canonical line discipline rendered its echo as `^@`. The identical command
through standard `io.containerd.runc.v2` has the same `5e 40` prefix; its
29-byte transcript hashes to
`45d7ec78c61e8b06e17c323d40782ee400745747c221747917138b7a3f05e061`.
This is a harness portability defect. Only that known ctr control echo should
be normalized; the size and success lines must remain exact. Both probes clean.

The matrix now strips only a leading literal `^@` from ctr transcript lines
before the exact `37 91` comparison. Docker and success-marker checks remain
exact. Direct probes cover prefixed and unprefixed lines; Bash syntax and diff
checks pass. Commit `a7b1ad9` (`test: normalize ctr terminal control echo`)
has exact matrix hash
`1d1d324b7bdb748da7c1502dcb4e74818bddad82ada1951b00fd192e617d1996`.
Guest upload and complete replay remain pending.

The guest independently matches corrected matrix hash `1d1d324b…` and the
unchanged `30c27d36…` resize helper. All services are active; a new complete
ordinary-user run starts with all 13 inventories zero and passes image
inspection for both clients. Split create/start is in progress. This is a
coherent replay from the beginning, not a resume at the failed row.

The corrected replay passes the lifecycle/isolation block with fresh distinct
child boot IDs `f3a346cf…` and `50ca36c5…`: create/start, inspect, exec I/O,
private roots, networking, sibling isolation, daemon-restart continuity,
pause/resume, signal/exit, normal deletion, and all-13-zero post-delete
convergence. Read-only bind testing is now running.

The replay then passes binds/post-bind convergence, both exit-17/name-reuse
cycles, stdin, and detached attach. The complete terminal row now passes: the
known ctr `^@` echo is narrowly normalized, and ctr plus Docker each report
exact `37 91` with exact success markers. This live-proves Docker console-size
admission and propagation. The first post-start live-resize helper is running;
that row and the final suite marker remain unclaimed.

The newly reached ctr live-resize helper times out without observing initial
marker `ready:24 80` and reports that exact failure; the matrix trap runs.
Neither post-start resize nor the final suite marker is claimed. The helper
currently turns any PTY read error or EOF directly into termination, so clean
audit plus native/runc comparisons must precede runtime attribution.

The helper passes native and identical `io.containerd.runc.v2` commands,
observing `ready:24 80`, signaling the live PTY, then observing
`resized:37 91`. Multikernel startup is slower and can temporarily expose no
open slave to the PTY master; blanket EIO-as-EOF kills ctr before its console
opens. This also reveals a cleanup race: the trap initially sees no task and
removes metadata, while in-flight CreateTask later leaves exact
`mk-matrix-ctr` shims and a `PREPARED` root despite an empty lifecycle map.
No clean state is claimed. Normal recovery plus EIO and late-create cleanup
hardening are required.

The retained exact `1f81cb2…` supervisor/worker are PIDs 55368/55373 and hold
deleted bundle inode 13917; task/container and lifecycle maps are empty.
After identity-verified supervisor termination, parent-death handling removes
the worker. Normal trace-metadata removal plus mkruntimed restart lets rootfs
reconciliation remove the absent, unowned prepared bundle. One immediate read
races state publication; the subsequent authoritative audit has every runtime
inventory zero and all services active. Preserved evidence is untouched.

The helper now treats PTY `EIO`/zero reads as transient only while the client
is alive. Trap cleanup also retains ctr metadata while an exact slow shim is
present, polling for a late task or a bounded quiet interval before task and
container removal for every matrix ID. Local native 24×80→37×91 resize, Bash
and Python syntax, and diff checks pass. Live Multikernel replay is pending.

The complete documentation/schema/evidence/runtime checker passes with 95 OCI
semantic cases; only the known sandbox-local socket `EPERM` subcase is skipped.
Generated caches are removed. Commit `6927c70` (`test: tolerate slow terminal
creation`) freezes helper hash `484229a83e2504c1b22506d0fd895291884cdd1abed953051516845f6e62fbcb`
and matrix hash `5e850f3f45b8bc25101d9d1394c01cb510d42274c2ba31f2a3bca5bf57b66b69`.
Findings and retained evidence remain outside the commit.

After the user-restarted VM, the authoritative boot is `c25eebdb…` on kernel
`7.0.0-mk2-gce-lab`. Persistent selectors remain exact binary `1f81cb2…`,
support `d36b6940…`, and manifest `1d79c564…`; guest helper/matrix hashes are
the frozen `484229a…`/`5e850f3…`. All 13 inventories are zero and all four
services are active/status 0. mkruntimed alone reports `NRestarts=1`, however,
so its current-boot journal must explain that restart before this baseline is
called healthy or workloads start.

Current-boot journals prove the ordering defect. mkruntimed starts at
23:03:20 UTC and fails `GUEST_AGENT`; the guest-agent unit starts at 23:03:25,
becomes active at 23:03:32, and the sole mkruntimed retry starts at 23:03:38.
Both required mounts were already ready. The managed unit now Requires and
orders After `google-guest-agent.service`, matching the existing fail-closed
host prerequisite. `python3 scripts/test-manage-runtime-deployment.py`, the
full documentation/schema/evidence/runtime gate (including 95 OCI cases), and
`git diff --check` pass; only the already-classified sandbox socket `EPERM`
subcase is skipped, and generated caches were removed. Commit
`8edc7cc5c653218679ae7cd1d3567c3f38b3a77c` freezes only this unit/test change;
findings and retained evidence remain outside it. Live support activation and
a clean reboot observation remain pending.

The exact tracked-source archive for `8edc7cc…` is 1,018,157 bytes with SHA-256
`efaf3af613fbd41597935bb4a5a36bd0752db0c1aebd4f3006e9e8b2b6646c25`.
Guest transfer, independent verification, and activation remain unclaimed.

The guest independently matches that exact archive hash/size, extracts it into
a unique root-owned source tree, and verifies unit-source hash `9d921d2f…`.
Managed installation selects support generation `fe456acc…`. Systemd reload,
effective dependency inspection, pre-reboot cleanliness, and reboot proof
remain pending.

The retained evidence run captures exact read-only binds and post-bind
all-zero cleanup, both exit-17/name-reuse cycles, stdin, detached attach, exact
initial 37×91 terminals, and ctr live resize 24×80→37×91. Docker then receives
a `WINCH` while the guest still reports `24 80`; the unconditional trap prints
`resized:24 80` and exits before the intended update, so
`evidence/runtime-20260930/g4-g6-final-live/g6-shared-matrix.log` closes at
`2026-09-30T00:11:00.288985Z` with exit status 1. This is retained failure
evidence, not a pass. Because the preceding complete run passed the identical
runtime path, the immediate hypothesis is a probe race: ignore unchanged-size
`WINCH` events and exit only after observing 37×91. Independent cleanup is
required before retry.

Independent post-failure audit is all 13 inventories zero on unchanged boot
`d08895c1…`; guest agent and all four runtime services remain active/status 0
with zero restarts. The guest trap now ignores any `WINCH` whose observed size
is not exact 37×91, while the helper reapplies the idempotent PTY size and
`SIGWINCH` every 500 ms until the expected observation or bounded deadline.
Native validation passes both the ordinary 24×80→37×91 path and an injected
case that deliberately ignores the first correctly sized signal, including 20
repetitions (`ignored-resize:37 91` followed by `resized:37 91`). The full
documentation/schema/evidence/runtime gate and diff checks pass with 95 OCI
cases; only the classified socket `EPERM` skip remains, and generated caches
are removed. New
helper/matrix/focused-wrapper hashes are `fe7059cf…`/`4b83a01c…`/`fedf1289…`.
Live replay remains pending.

Commit `79e97eba2662f96d895de3b67a0d74f7a6142f8d` freezes the helper/matrix
correction. A Docker-only focused wrapper passes syntax and hashes to
`b031b9a9…`; exact guest upload and focused Docker replay remain pending before
another full retained run.

Retained focused capture `g6-docker-resize-focused.log` independently verifies
helper/wrapper hashes `fe7059cf…`/`b031b9a9…`, observes exact
`ready:24 80`→`resized:37 91`, emits both resize pass markers, and closes at
`2026-09-30T00:16:58.814611Z` with exit status 0. Independent teardown remains
pending before the complete evidence rerun.

Independent focused teardown returns all 13 inventories to zero; mkruntimed,
mknetd, containerd, and Docker remain active/status 0 with zero restarts. The
complete retained rerun therefore starts from a clean host.

The next immutable candidate `g6-shared-matrix-pass.log` verifies corrected
matrix/helper hashes and passes lifecycle/isolation plus bind rows with
all-zero checkpoints, but exits 1 silently before cycle 1 at
`2026-09-30T00:25:01.215394Z`; transcript SHA-256 is `4814439c…`. Independent
audit is all 13 zeros with services healthy and storage at 65% blocks/1%
inodes, excluding cleanup or ENOSPC. Bounded journals identify Docker
CreateTask `BACKEND_TIMEOUT` after roughly 71 seconds. Live host config sets
only 30 seconds, incompatible with measured 70-second cold/private-root
creation. Containerd's subsequent dead-shim cleanup also logs `fork/exec
/usr/local/bin/containerd-shim-multikernel-v2: no such file or directory`,
although immediate identity inspection finds that link valid and resolving to
exact release inode 6284664/hash `d1888952…`. The timeout must be corrected
coherently before retry; the cleanup diagnostic remains a separate observation,
not yet a persistent missing-link finding.

A replacement host config changes only the bounded backend deadline from 30 to
180 seconds, exceeding the observed ~71-second create without becoming
unbounded. Retained `host-config-timeout180.json` is schema-valid, mode 0600,
449 bytes, and SHA-256 `4056c182…`. Managed installation and effective-service
validation remain pending.

Root-owned exact config installation selects managed generation `25e6d343…`.
Coordinated empty-host daemon reload/restart makes the effective config/root
copy match `4056c182…` and report 180 seconds; binary selector remains exact
`1f81cb2…` and unit hash remains `9d921d2f…`. mkruntimed/mknetd/containerd/
Docker are active/status 0 with zero restarts at PIDs 36974/36955/1445/1543,
and all 13 inventories are zero. The exact formerly timing-out Docker boundary
remains to be focused-replayed before another full capture.

Retained `g6-docker-exit17-timeout180.log` verifies focused-wrapper hash
`72044c84…`, then the exact Docker boundary completes in 71 seconds with client
and inspect status 17 plus both stdout/stderr markers. It emits
`FOCUSED_DOCKER_EXIT17_PASS` and closes at `2026-09-30T00:31:07.311525Z` with
exit 0. Because 71 seconds exceeds the old 30-second limit, this directly
attributes the prior `BACKEND_TIMEOUT` to configuration. Independent teardown
remains pending.

Independent teardown returns all 13 inventories to zero with the runtime
services healthy. The next retained capture first records a harmless evidence
command error: an ordinary user cannot hash the deliberately mode-0600 managed
configuration. Its replacement verifies the privileged hash correctly and
passes lifecycle/isolation, including daemon-restart continuity, before Docker's
bind CreateTask fails with `BACKEND_FAILURE`. Sanitized kernel evidence shows
that the preceding ctr teardown released the 16-GiB pool and the immediate
reallocation failed twice with `-ENOMEM`. Storage remains healthy and the bind
row itself had already passed separately. The failure is therefore repeated
last-sandbox pool teardown/recreation fragmenting host physical memory. A
kernel command line observed during diagnosis contained an authentication token;
it is deliberately neither retained nor reproduced.

The user's latest instance restart does not change the authoritative guest boot:
it remains `d08895c1-0d19-4c66-ac7e-c5f77fd23451` on
`7.0.0-mk2-gce-lab`. System state is `running`; guest agent, mkruntimed,
containerd, and Docker are active with zero restarts. The immutable production
selector remains `1f81cb2…`. The effective config resolves into managed
generation `25e6d343…`; its target is root-owned mode 0600, 449 bytes, hashes
exactly to `4056c182…`, and supplies the 180-second deadline. An initial
0777/44-byte report was the symlink's own metadata, not the target's. This was
not an OS reboot and supplies no memory-defragmentation proof; the pool-lifetime
defect remains the next implementation target.

Pool ownership is now daemon-scoped across successful zero-sandbox intervals.
Last-sandbox Delete retains the already initialized 16-GiB pool, so a following
sequential Create does not request a second contiguous host allocation. Failed
first creates and their cancellation path still release immediately. Graceful
daemon shutdown releases only an idle pool; any durable live sandbox suppresses
release and preserves restart continuity. For crash recovery, startup may clear
only the exact residue `configured pool + zero backend instances + zero durable
sandboxes + no other stale resource`, then reruns full host qualification. More
ambiguous residue remains fail-closed. Race-enabled focused lifecycle and daemon
tests, focused vet, and diff validation pass. Full-tree and live qualification
remain pending.

The complete local qualification gate passes: all Go packages under the race
detector, full vet, documentation/link/schema/evidence checks, deployment and
containerd checks, and 95 OCI semantic cases. The only skip is the previously
classified sandbox-local socket `EPERM` subcase. This establishes source-level
correctness only; exact guest build, activation, sequential live reuse, graceful
idle release, and complete retained matrix evidence remain pending.

Commit `e27ab26` freezes only the four daemon/lifecycle source and test files.
Its exact 1,020,028-byte tracked-source archive hashes to
`87440b1fe04664d57e7ce95a5467ec779219236f2dc05ea82778e320134e206b`.
The final focused gate also covers ambiguous pool initialization: authenticated
Create cancellation releases it even when the original `kerf init` returned an
error before the process could mark the pool ready. Guest transfer and all live
claims remain pending.

The guest independently measures the uploaded archive as exactly 1,020,028
bytes with the same full SHA-256 `87440b1fe04664d57e7ce95a5467ec779219236f2dc05ea82778e320134e206b`
on unchanged boot `d08895c1…` and kernel `7.0.0-mk2-gce-lab`. This establishes
transfer identity only; build, selection, and execution remain unclaimed.

The first compilation is rejected before installation: its manually supplied
linker stamp `e27ab26e3f2dba70fb4f8fc0f981d45dd59614df` differs from authoritative
commit `e27ab263e12980c670085495f21625e480bcc879`. Compilation success and the
resulting hashes therefore make no deployable-candidate claim. The independently
verified root-owned source remains valid and must be rebuilt with the exact
revision.

The attempted in-place correction stops safely when exclusive manifest
publication returns `EEXIST`; no mixed manifest/binary set is installed or
claimed. The rejected directory is preserved. A corrected build must start in
a second unique root-owned extraction, retaining the publisher's no-overwrite
guarantee.

The second unique root-owned extraction re-verifies source hash `87440b1f…`
and builds the complete exact `e27ab263e12980c670085495f21625e480bcc879`
release. Manifest SHA-256 is `7dccd39f…`; exact mkruntimed, shim, mknetd, and
agent hashes are `be810c3c…`, `043e94d4…`, `a78392c6…`, and `b0f0ba52…`.
The candidate daemon reports the exact revision. Installation, selection, and
live execution remain unclaimed.

Pre-activation inspection on unchanged boot `d08895c1…` resolves Kerf from the
active configuration and proves no configured pool, Multikernel instance,
`/proc/kimage` entry, default/moby task or container, Docker object, or sysfs
child. All four runtime services are active and the selector remains `1f81cb2…`.
An earlier probe used a nonexistent Kerf path and stopped before mutation. The
host is clean for installation.

The binary manager installs and selects immutable exact release
`0.1.0-dev-e27ab263e12980c670085495f21625e480bcc879`. Empty-host mkruntimed
restart produces active PID 52355 with zero restarts. Its executable resolves
inside that immutable release and both running/public hashes equal candidate
`be810c3c…`; the daemon reports the exact revision. mknetd, containerd, and
Docker remain active. An ordinary-user `/proc/52355/exe` check stopped before
hashing, so only the successful narrow privileged rerun is authoritative. Live
behavior remains pending.

The evidence matrix now includes a non-sensitive `pool_configured` inventory.
It requires pool 0 initially, pool 1 with all other resources zero between
sequential workloads and after the last workload, then a changed mkruntimed PID
and pool 0 after an explicit idle-daemon restart. This makes sequential reuse,
live restart preservation, and graceful idle release observable rather than
implicit. Bash syntax, the full documentation/schema/evidence/runtime chain,
and diff checks pass; matrix hash is `3015518c…`, the sole skip remains the
classified socket `EPERM`, and generated caches are removed.

Commit `94396f7` freezes only the strengthened matrix at exact SHA-256
`3015518c904a432772d2acf2886d06bb8f48b08472d79e6f2147f7c1d6860db6`.
Learning records and evidence remain outside it; guest verification and
execution are pending.

Fresh guest extraction matches the 4,792,320-byte `56140248…` archive and all
three script hashes across 648 files. Bash syntax passes on unchanged boot
`c5537cb9…`, with four active services. This is provenance only; the expanded
fault run remains pending.

Exact `0672fa1` execution exits 0. The verbose VM builder suite names and
passes the SIGKILL-interrupted copy case; the race subset names and passes
pristine/current wrong-UUID inspection, quota/clean-state identity, single
ownership, and conflicting-generation refusal. All constrained-filesystem and
bracketing all-zero audit cases pass again. Remote transcript is mode 0600/
86,309 bytes/SHA-256 `1eca1db8…`, wrapper exit 0, with an empty credential
scan. Local retention remains before row closure.

The local copy exactly matches mode 0600/86,309 bytes/SHA-256 `1eca1db8…`,
its credential scan is empty, and all required named markers are present.
Together with the retained `5790e421…` single-owner/lock transcript, the
composite exhaustion/high-water/identity/lock/attach/interruption/allocation-
boundary row is closed. The independent server-loss, corruption, and recovery
row remains open.

The post-closure full repository gate passes with the 12-case storage suite;
the diff is clean and four generated bytecode files are removed. Checklist
totals are now 45 closed and 40 open.

The unchecked replacement-instance initramfs row already has strong historical
proof: retained mode-0600 transcript `5a1d5ae6…` records identical verified
archive/manifest hashes `2532e1b5…`/`bf6f3e04…` and divergent changed-input
hashes `5850d990…`/`fd059b19…`; audit `c5748c81…` is all-zero and both
credential scans are empty. Since current builder commit `0672fa1` includes
new capacity/cleanup logic, an exact-current replay will precede closure.

Exact-current commit `0672fa1` replay passes. Two independently built and
verified archive/manifest pairs match at `35771180…`/`416833e3…`; the changed
control diverges at `3bd844ae…`/`e1408c2a…`; all 20 tests and the qualifier
exit 0. Local repro evidence is mode 0600/9,171 bytes/SHA `b32b030e…`; the
independent all-zero audit is mode 0600/15,094 bytes/SHA `314d0b63…`. Both
wrappers exit 0 and their joint credential scan is empty. The replacement-
instance initramfs reproducibility row is closed.

The post-closure full repository gate passes; the diff is clean and four
generated bytecode files are removed. Totals are now 46 closed and 39 open.
Server-loss, host-reset, corruption, and recovery remain separate unproved
work rather than being inferred from graceful restart evidence.

The deterministic-manifest implementation row is complete. Current code
finishes source manifesting, storage identity/build, and generated-versus-
verified initramfs checks in `PrepareRootfs` before `CreateSandbox`. Exact live
caller evidence `33e1d92e…` separates OCI index/manifest/config/layer/diff ID
from source manifests `65714ed0…`/`e9573a41…`; exact-current repro evidence
`b32b030e…` separately records archive/manifest `35771180…`/`416833e3…`.

Unique guest staging independently matches exact matrix/helper hashes
`3015518c…`/`fe7059cf…` and passes Bash syntax. Preflight on unchanged boot
`d08895c1…` confirms all four services active, the exact `e27ab263…` daemon,
and no configured pool. The retained run therefore begins from a proved
released state.

Immutable capture `g6-shared-matrix-pool-retained.log` starts at
`2026-09-30T15:57:47.501147Z` with exact gcloud argv. On boot `d08895c1…`,
the initial observation is pool 0 with every existing resource/process counter
zero. The shared BusyBox digest and amd64 identity pass for ctr and Docker;
lifecycle creation is in progress and later rows remain unclaimed.

The immutable capture closes at `2026-09-30T15:59:03.940856Z` with exit 1:
the first ctr Task returns `BACKEND_FAILURE`, so no pool-reuse or later feature
claim is made. The mode-0600, 128,724-byte transcript hashes to `a89f9168…`.
Initial evidence proved no configured pool, while the guest never actually
rebooted after the user's instance restart. The immediate hypothesis is failure
of the first 16-GiB contiguous allocation on the already fragmented boot;
cleanup and filtered cause evidence are required before reset/retry.

Independent audit is logically clean: pool 0 and zero tasks, containers,
Docker objects, children, runtime artifacts, rootfs records, and exact shims;
all four services remain active. Narrow kernel filtering reports `Baseline pool
allocation failed: -12` twice, directly identifying initial-allocation `ENOMEM`.
The unchanged boot is physically fragmented despite zero logical residue. A GCE
reset is required before the candidate's retention behavior can be tested.

Authorized GCE reset advances the authoritative boot to
`768706da-cc24-486b-8fb1-92d205010c44`. After bounded readiness, system state
is `running`; guest agent and all four runtime services are active with zero
restarts. Exact `e27ab263…` remains selected, the effective mode-0600 config
still hashes to `4056c182…`, and Kerf reports no pool. This is the valid
post-defragmentation candidate baseline.

Replacement immutable capture `g6-shared-matrix-pool-retained-pass.log`
starts at `2026-09-30T16:03:59.729374Z` on boot `768706da…`, proving pool 0
and all other counters zero initially. The paired lifecycle/isolation/network/
daemon-restart rows pass with distinct child boots `ee457fda…`/`4a731acf…`.
After normal deletion, the decisive inventory is pool 1 with every logical and
process resource zero. The next ctr bind begins from that retained pool rather
than reallocation; bind/Docker and later rows remain in progress.

The former failure boundary is now directly closed. ctr and Docker read-only
bind rows both pass after lifecycle teardown, preserving host bytes and rejecting
guest writes. Post-bind inventory again records pool 1 and every other counter
zero. Docker therefore followed ctr from the retained pool instead of attempting
the prior `-ENOMEM` reallocation. Later rows remain in progress.

Both repeated ctr/Docker cycles return exact status 17 with their distinct
stdout/stderr, clean normally, and reuse the same names in cycle 2. Foreground
wait/I/O/nonzero-exit and name-reuse therefore pass while the retained pool
remains serviceable. Later rows remain in progress.

ctr/Docker stdin forwarding passes with exact client-specific output, and
detached attach passes with exact `ctr-attached-stdin`/`docker-attached-stdin`
before normal cleanup. Terminal and final release rows remain in progress.

Initial terminal allocation passes for ctr and Docker at exact 37×91; the
classified ctr `^@` prefix is narrowly normalized. Corrected post-start resize
and final pool release remain in progress.

Corrected live resize passes exact 24×80→37×91 for ctr and Docker. The
pre-shutdown observation is pool 1 with every other counter zero. Idle daemon
restart changes PID 3854→10964, after which the final observation is pool 0
with all 13 existing resource/process counters zero. All five scoped evidence
assertions and `G4_G6_CTR_DOCKER_FEATURE_MATRIX_PASS` emit; the immutable run
closes at `2026-09-30T16:20:22.902782Z` with exit 0. Its mode-0600, 318,154-byte
transcript hashes to `9b67a1042f83c7b608b6a4a8d742167b56792c1b70a78b0e464c04f4523361b0`.

Independent audit on unchanged boot `768706da…` confirms system `running`, the
exact immutable selector and running daemon hash, pool 0, and all 13 counters
zero. Guest agent plus all four runtime services are active with zero restarts.
A non-restarting daemon reload clears the stale-unit warning without changing
PID 10964. The pool-lifetime defect is live-closed: one allocation survived all
sequential workloads and the live-sandbox daemon restart, then released only at
empty-daemon shutdown.

The exclusive post-run GCE ledger is schema 1, mode 0600, 5,007 bytes, and
hashes to `0f5f350812422600c7b506a2aeef2ac473f9d3cdaa9abffb81d25b3080548211`.
Ledger tests pass. It records the same one instance, two disks, three snapshots,
zero addresses, and six firewall rules as the pre-run ledger; after removing
only `captured_at`, the normalized documents are byte-identical. The run leaked
no GCE resource.

Final local reconciliation passes Bash/diff validation and the complete
documentation/schema/evidence/runtime chain with 95 OCI cases; only the known
local socket `EPERM` subcase skips. Direct transcript inspection finds all six
pool/resource observations, five scoped true assertions, the combined pass,
and exit-0 trailer, with no token/password/credential/private-key value pattern.
Generated caches are removed. This closes the pool-lifetime defect and the
shared-matrix/live-resize slices only. G4 persistence/fault matrices, G5 UDP/
MTU/load/fault/security matrices, and G6 service-restart, forced-shim, event,
FIFO/cancellation, and isolated fault matrices remain genuinely open.

The first forced-shim qualification attempt is intentionally retained as
failed evidence rather than replaced. The mode-0600, 354,963-byte transcript
`g6-forced-shim-death-matrix.log` hashes to `5a4dedc7…` and closes exit 1 at
`2026-09-30T23:19:52.150203Z`. Task PID 20697 was actually the namespace
holder, and bundle PID-file value 20504 was the serving worker. The harness did
inject the intended worker kill, but then treated the namespace holder's `/proc`
directory as a stable supervisor anchor; it vanished with the worker, so the
poll could not observe reconstruction. The narrow containerd journal records
shim disconnect, disconnect cleanup, and dead-shim cleanup. Consequently no
reconnect or fallback result is claimed. Trap cleanup and independent audit
leave the pool retained with every workload/storage/network/process count zero,
both observed PIDs dead, and all four services active with zero restarts.

Corrected harness SHA-256 is `9e6a697e73bbd0cdf7934cdf329ad6a85f2de6a4e0f748dcb4743dff954e22bf`
(13,651 bytes). It now verifies Task worker → PPID supervisor → supervisor PID
file worker as a closed relationship before both fault injections, and omits
xtrace only inside high-frequency bounded convergence loops. Local Bash syntax
and diff validation pass; guest identity validation and behavioral replay remain
unclaimed.

The uniquely staged guest copy independently matches full `9e6a697e…`, is
13,651 bytes/mode 0755, and passes Bash syntax. Replay preflight on unchanged
boot `768706da…` proves all four services active, no configured pool, and zero
default/moby/Docker workload, child, and runtime-artifact counts. No fault
outcome is claimed from staging or preflight.

The second immutable capture under the provisional `-pass` filename is also a
failed qualification, not a pass: mode 0600, 20,246 bytes, SHA-256 `f0499c5a…`,
exit 1 at `2026-09-30T23:25:42.796311Z`. It stopped before fault injection when
the revised precondition mislabeled Task namespace-holder PID 29425 as worker,
its parent/actual worker 29239 as supervisor, and then found the PID file also
contained 29239.

A cleaned isolated role probe establishes the full topology directly:
supervisor 30366 (PPID 1) → serving worker 30371 → Task namespace holder 30555,
with `.multikernel-worker.pid=30371`. Reconnect evidence must therefore keep
the supervisor fixed while requiring replacement worker and namespace-holder
PIDs; fallback must kill that supervisor. Independent audit after both cleanup
paths shows pool retained, all scoped logical/process counts zero, unchanged
boot, and every service active with zero restarts. Neither failed capture closes
a forced-death outcome.

The third harness revision is frozen at SHA-256 `1a2e557a2adca84bbba84a7aed0a7c502c5466ea81ea5dc8ffa80b71147190e8`
(14,545 bytes). Its preconditions now prove namespace-holder→worker→supervisor
parentage and PID-file→worker agreement. Worker reconstruction must retain the
supervisor while replacing both descendants; supervisor-death fallback must
remove all three. Bash syntax and diff validation pass; guest replay remains
unclaimed.

The third-revision guest copy independently matches full `1a2e557a…`, is
14,545 bytes/mode 0755, and passes Bash syntax. Its unchanged-boot preflight
shows no pool, zero task/container/child counts, and all four services active.
No fault result is claimed from this input validation.

Third capture `g6-forced-shim-death-matrix-v3.log` is the first valid behavioral
result and confirms a product defect. It is mode 0600, 26,806 bytes, hashes to
`c1670e5b…`, and closes exit 1 at `2026-09-30T23:36:05.473765Z`. Pre-fault
evidence proves supervisor 31483 → worker 31488 → namespace holder 31675,
RUNNING state, child boot `1f97a684…`, pre-fault exec I/O, and authenticated
schema-3 recovery metadata. Killing worker 31488 alone immediately produces
containerd's shim-disconnected, disconnect-cleanup, and dead-shim-cleanup
sequence; no replacement worker/holder or RUNNING Task emerges in two minutes,
and all three PIDs end. Cleanup converges safely to retained pool/all scoped
resources zero with all services healthy, but worker reconstruction is broken
on the selected immutable runtime. The later supervisor-death case never ran
and requires a separate focused qualification.

The live harness now accepts only explicit `matrix`, `reconnect`, or `reclaim`
selection, records that selection, and leaves the reconnect failure intact
while allowing independent fallback proof. The revised 14,818-byte input hashes
to `16fb3a027a568237b7c46847b54492d0c2ee754a3e1b95ffd9e409f4f0f62fd7`;
Bash syntax and diff checks pass. Its reclaim-mode guest run is pending.

The reclaim-mode guest copy independently matches `16fb3a02…`, is 14,818
bytes/mode 0755, and passes Bash syntax. Idle mkruntimed restart `30919→44343`
released the retained pool; unchanged-boot preflight then proves no pool and
zero task/container/child counts. Behavioral fallback proof remains pending.

Independent fallback capture `g6-forced-shim-reclaim.log` passes: mode 0600,
47,705 bytes, SHA-256 `e833dcd1…`, exit 0 at
`2026-09-30T23:40:44.443927Z`. It records supervisor 44895 → worker 44901 →
namespace holder 45097, child boot `1b3584d3…`, and private schema-3 recovery
metadata before killing only the supervisor. All three processes die, Task
state becomes absent, exec fails closed, and the containerd disconnect cleanup
reclaims every child/storage/network/shim/helper resource. Metadata removal
reaches pool 1/all-other-zero; mkruntimed `44343→45437` then yields pool 0/all-
zero. Independent audit confirms unchanged boot and every service active with
zero restarts; transcript scanning finds no credential pattern. The revision's
generic matrix marker is explicitly disregarded because reconnect mode did not
run; source now emits a mode-specific final marker for non-matrix runs. Safe
fallback is closed, while in-place worker reconstruction remains a proved open
defect.

The reconstruction ownership defect is now repaired in source. The stable
supervisor accepts and retains containerd's public TTRPC connection; each
replaceable worker serves through a fresh random private Unix listener, and a
supervisor bridge carries the same containerd byte stream across worker
generations. A worker `SIGKILL` therefore no longer inherently closes the
connection that triggered immediate dead-shim cleanup. Private workers retain
same-UID TTRPC handshaking, parent-death policy, held-bundle identity, recovery
state, exclusive PID-file publication, and the bounded restart budget. The full
shim package passes; a new same-client/two-worker bridge test plus the existing
signaled-worker test pass 25 race-detector repetitions. This is source-only
proof; immutable build, guest activation, rollback identity, and live replay
remain required.

The marker-corrected harness is 14,925 bytes with SHA-256 `7b77bb91552dbfdbe1e4b54bb07df91cc980cca2c4102af7a155666534eb32ef`.
It reserves the generic matrix marker for matrix mode and emits a scoped mode
marker otherwise. The successful fallback transcript remains bound to its
executed `16fb3a02…` input; evidence is not rewritten.

Commit `7d50218f593eb1b548107eb9d82328e925eab448` (`runtime: preserve shim
connection across workers`) freezes exactly the supervisor bridge, its focused
test, the marker-corrected 14,925-byte live harness, and script index. Running
findings and both historical/live evidence trees remain outside the commit.

The exact committed-source guest-build archive is 1,464,320 bytes/mode 0644,
SHA-256 `5f5c4fc86eec56262fd8618ceb2c9e6916f53442460294e38fd8d2f8cf499a25`.
It comes directly from `7d50218f…` and includes only Makefile, runtime source,
and immutable release tooling. Transfer and build remain unclaimed.

The guest independently matches full archive hash `5f5c4fc…` and extracted it
into a unique build directory. Before build, exact rollback selector remains
`0.1.0-dev-e27ab263e12980c670085495f21625e480bcc879`, boot is unchanged
`768706da…`, no pool is configured, and default/moby/Docker workload counts are
zero. No build or activation result is yet claimed.

The guest build succeeds for all seven binaries at exact embedded revision
`7d50218f…`. Release manifest is mode 0644, 1,907 bytes, SHA-256 `70345fb1…`;
component hashes are shim `65a9256c…`, agent `bdae9b3a…`, agentctl `5057066f…`,
CNI `1ff91069…`, host check `b96f3d40…`, mknetd `88c1b80e…`, and mkruntimed
`38a7a2f3…`. The first component-name projection assumed a mapping rather than
the manifest's list and exited 1 after the hashes/revision were printed; a
corrected read-only projection confirms all seven names and version outputs.
Install and activation remain pending.

The immutable manager installs and atomically selects release
`0.1.0-dev-7d50218f593eb1b548107eb9d82328e925eab448`; every host command link
remains manager-owned, inspect retains exact rollback `e27ab263…`, and the
active shim reports `7d50218f…` with build hash `65a9256c…`. Installation did
not restart services or create a workload. Live behavior is still unclaimed.

The candidate guest harness independently matches full `7b77bb91…`, is 14,925
bytes/mode 0755, and passes syntax. Replay preflight proves exact active
selector and shim revision `7d50218f…`, unchanged boot `768706da…`, no pool,
zero workload/child counts, and all four services active with zero restarts.
No reconnect behavior is claimed from preflight.

Candidate capture `g6-forced-shim-reconnect-candidate.log` is a harness-only
exit-1 after successful reconstruction: mode 0600, 36,646 bytes, SHA-256
`ebfcf749…`, closed `2026-09-30T23:53:45.044495Z`. Supervisor 51313 remains
fixed; worker `51318→51613` and holder `51504→51637`; Task stays RUNNING; child
boot `868b0395…`, guest PID/recovery/network identities and offsets are stable;
post-fault exec stdout/stderr succeeds. The immediate filtered journal is empty,
so containerd did not enter disconnect cleanup at the fault. After successful
task release, late attach returns only post-release init output and the harness
wrongly demands pre-fault bytes that the old pump had already delivered. Trap
cleanup is retained-pool/all-resource-zero with healthy services. The normal
completed shim later logs ordinary disconnect cleanup, after the fault-time
empty observation. A continuous attach reader is required for the replay; no
pass marker or final reconnect claim comes from this transcript.

Corrected harness SHA-256 is `e47804f5ce942b6de2f294dc1b7676c40de0a63749e439707039e059e03d2135`
(15,639 bytes). Init waits on a readiness file while one attach reader is opened;
the harness releases init, requires pre-fault stdout/stderr in that reader,
retains it across worker replacement, and later requires post-fault stdout/
stderr from the same stream. Trap cleanup now owns the reader. Bash syntax and
diff checks pass; guest replay remains pending.

Idle mkruntimed restart `45437→52142` releases the pool and restores zero task/
container/child counts. Executing `/proc/<pid>/exe --version` unexpectedly
printed systemd's version, so that probe is explicitly invalidated. A stable
read-only unit audit instead proves PID 52142 active/running with zero restarts,
exact candidate release path `7d50218f…`, and binary hash `38a7a2f3…`. The
replay baseline is therefore both released and candidate-bound.

The corrected guest copy independently matches full `e47804f5…`, is 15,639
bytes/mode 0755, and passes syntax. Exact candidate `7d50218f…` remains active
with no pool and zero task/container/child counts. No replay result is inferred
from staging.

Continuous-attach capture `g6-forced-shim-reconnect-candidate-pass.log` is
retained as failed evidence: mode 0600, 39,468 bytes, SHA-256 `ab089baf…`, exit
137 at `2026-10-01T00:04:13.478649Z`. The bridge again preserves supervisor
52809 while replacing worker `52814→53164` and holder `53012→53189`; Task stays
RUNNING, child/recovery identities remain stable, post-fault exec succeeds, and
the immediate disconnect journal is empty. One attach reader receives both
pre-fault init streams. After release, durable stopped offsets advance exactly
stdout `22→43` and stderr `26→51`, but no post-fault bytes reach that attach
file and its client never returns. TERM is ineffective; only the three exact
attach-client PIDs are KILLed, yielding exit 137 and normal trap cleanup. The
independent audit is candidate-selected, retained-pool/all-other-zero, unchanged
boot, and healthy services. Root cause boundary is now precise: the byte bridge
preserves future RPCs but cannot replay the attach client's in-flight `Wait`
request consumed by the killed worker. Full Task/FIFO reconnect remains open.

The second source repair makes the supervisor bridge TTRPC-frame aware. It
retains streams until terminal response and replays outstanding request/data
frames to a replacement worker in increasing stream-ID order, allowing the
killed worker's in-flight `Wait` to resume before later RPCs. Replay fails
closed at 4 MiB per frame, 256 pending streams, and 64 MiB total, and rejects
duplicate request IDs, unknown-stream data, malformed/oversized frames, and
bound exhaustion. A new test has worker one consume a request and die, then
proves worker two receives the exact replay and its response reaches the same
client. All runtime packages pass; both bridge tests and the signaled-worker
test pass 25 race-detector repetitions. This remains local-only until rebuilt
and replayed on the disposable host.

Commit `1dbe2d93ee722e292adfa4169c32eb66974d4847` (`runtime: replay in-flight
shim RPCs`) freezes exactly the framed replay repair, expanded bridge tests,
and continuous-attach harness. Running findings and evidence remain outside the
commit.

Exact second-build archive is 1,474,560 bytes/mode 0644, SHA-256 `00d782d6…`,
generated directly from `1dbe2d93…`. Transfer and build remain pending.

Guest archive independently matches full `00d782d6…` and is extracted under a
new path. Idle mkruntimed restart `52142→54167` releases the pool while keeping
the running daemon/current selector on exact first candidate `7d50218f…`; task,
container, and child counts are zero. Second build/activation remain pending.

The second guest build succeeds for all seven components with embedded version
`0.1.0-dev` and exact revision `1dbe2d93ee722e292adfa4169c32eb66974d4847`.
Its mode-0644, 1,907-byte manifest has SHA-256
`c21bd04a9faae89fc3d0bfeaf665750cb782e7b185fea0f40604e44949db5422`.
Binary SHA-256 values are shim `d01237a447b8c9c3fa4e4b91a57bd532731094e475cde83c5ad1f26ca2c178ea`,
agent `bb2c606803a457076619e7a5503e86fe35e1d1cd384e38252025bbfc7e8d7dfa`,
agentctl `dfbbe5e0429e06adad3d20efef0e323e9f19a54eea156b6b9192829465842c02`,
CNI `d08eca6a2b9e961e5053b99cc9af082c86f5f080536c064b535efba9ff140fed`,
host check `364c832a65aaa23db877c19541c27efcaa64e7ed6877167fe3ece83ec46d230b`,
mknetd `d2c82c7f3f088b227b47ca30851ef74fcffa1ef6c99e7fc3bff5ee117454ab8e`,
and mkruntimed `350ea5daea65f65732bb2d9e439e74411d50fe348586bff4c97c2205ebb9e39a`.
The manifest lists exactly those seven component names and every binary's
`--version` output agrees. Installation and activation remain unclaimed.

The immutable manager installs and selects exact release
`0.1.0-dev-1dbe2d93ee722e292adfa4169c32eb66974d4847`; inspect reports all six
host command links present and manager-owned while retaining the prior
`7d50218f…` and original `e27ab263…` releases. The active shim reports the full
new revision and re-hashes to build SHA-256 `d01237a4…`. All four services
remain active/running with `NRestarts=0`. An ad-hoc pre/post read of the
nonexistent `/opt/multikernel/runtime/current` path printed blank and is
explicitly rejected; manager inspect establishes activation, and the real
`/usr/local/lib/multikernel/current` target still requires preflight capture.
No live reconnect behavior is claimed from installation.

Read-only replay preflight resolves the real selector to exact release
`1dbe2d93…`, confirms unchanged boot `768706da…`, zero default/moby containerd
tasks and containers, zero Docker containers, no shim/agent child processes,
and all four services active/running with zero restarts. A manual curl guessed
the wrong mkruntimed socket, failed to connect, and consequently hashed empty
input; that hash is invalid and excluded. The established harness/API resource
checks remain authoritative for pool state. Live replay remains unclaimed.

Framed-replay capture `g6-forced-shim-reconnect-framed-pass.log` is retained as
failed evidence: mode 0600, 38,857 bytes, SHA-256
`b2f6ca26861616e19da22a855642a821fe0834be4c2af12b1cf2f5397459fce6`,
exit 1 at `2026-10-01T11:44:49.271150Z`. Supervisor 57905 remains stable while
worker `57911→58244` and holder `58097→58268`; Task remains RUNNING; child
boot `68299503…`, guest PID 163, bundle/task/sandbox/network generations, I/O
offsets, and FIFO identities remain unchanged; post-fault exec succeeds; and
the immediate filtered containerd journal is empty. After release, the replayed
attach/Wait call now returns normally, but its file contains only both pre-fault
markers and neither post-fault marker, so the scoped pass marker is absent.
Credential scan finds only the Kerf table heading `Cmdline`, not a value.
Independent audit finds the pool retained with zero allocation and every task,
container, runtime artifact, rootfs record, endpoint, shim, and helper count
zero; selector/boot/services remain exact and healthy. The remaining failure is
FIFO descriptor continuity: killing the old worker closes its stdout/stderr
writers, the unchanged containerd FIFO readers observe EOF, and reopening the
same persisted FIFO paths in the replacement cannot resurrect those already-
ended readers. Framed RPC replay fixes the in-flight Wait but is insufficient;
the stable supervisor must preserve output FIFO writer lifetime across worker
replacement (with equivalent bounded ownership/cleanup proof). Combined G6
forced-shim qualification remains open.

Third source remediation gives every FIFO output guard a minimal helper process
that inherits only the already identity-checked guard FD and a control pipe.
Normal `closeProcessIO` writes an explicit byte, closes, and reaps the helper
immediately. Worker SIGKILL instead closes the control pipe without the byte;
the helper retains the writer endpoint for a bounded 15-second replacement
window, never reads FIFO data, then exits. Grace input is constrained to
1 ms–60 s, launch errors fail the I/O open, and regular-file output is unchanged.
A focused test simulates descriptor loss without normal close, proves a raw
nonblocking FIFO reader sees `EAGAIN` rather than EOF, and receives replacement
output; it passes 10 repetitions and the full shim package passes. Two setup
failures are excluded: the first command used redundant `runtime/` paths while
already inside that directory, and the next encountered the read-only default
Go cache. A first version of the FIFO assertion used Go's poller and waited
until its 500-ms test grace expired; replacing it with `unix.Read` tests the
kernel's immediate nonblocking state. No live claim is made yet.

The keeper/reattach, framed-bridge, and signaled-worker tests pass 25
race-detector repetitions in 93.574 seconds, and every runtime package passes.
`git diff --check` is clean. This is source-only evidence pending immutable
commit, guest build, identity capture, and live replay.

Commit `1edd368f640f28e880480c638c0936a5cbaba6b0` (`runtime: preserve
output fifos across shim workers`) freezes exactly the two source/test files;
findings and evidence remain outside it. Its exact Makefile/runtime/release-tool
archive is 1,474,560 bytes/mode 0644 with SHA-256
`26c5b3d1b4617eb23a2ef33349860f15b98ab35cf622aa1e1f16a663c7a19696`.
Transfer and guest build remain unclaimed.

The guest independently matches the archive's mode, size, and full `26c5b3d1…`
hash, extracts it into unique directory `/tmp/multikernel-runtime-build-1edd368`,
and successfully builds all seven components plus the release manifest with
exact embedded revision `1edd368f…`. Artifact hashes and activation remain
unclaimed pending independent inspection.

Independent build inspection confirms the mode-0644, 1,907-byte manifest at
SHA-256 `95d45ebc5b7f57e8f2ef2bfaeff0ee44f522bc3b2a34c210fa027ee6fe8fadbd`.
Binary hashes are shim `45fd9f3a26cb1d323d20f9d9a5e56a2e828ab117668007fcc13deaec2cb9def8`,
agent `30a10c0222c0ffc94d3f553c494f029eb484c2d2d06f4fdeb1c6a8af573cac92`,
agentctl `b8ca2a5f1277e572cb1ee0a59e0b7a6960fab906ea5d3fb0fa03791ccd0e5d82`,
CNI `cac1f808a07a3e2980ebf250a549619d640f81ec29370cd0ecdac69cfab14c4c`,
host check `964e938512fda0cca18b52486267e9087b116986c0ed4e5d30a54cdbc1cd0981`,
mknetd `d5cd8aff2560f6267a66451357fed4ad5d6cfcb538e0e075755455832c7cfb0c`,
and mkruntimed `c7090da61fb4b546b7d55d8d0e476098d886c79f0727499f4450e7dc81245048`.
The manifest lists exactly all seven names and every binary reports revision
`1edd368f640f28e880480c638c0936a5cbaba6b0`. Activation remains unclaimed.

The immutable manager atomically changes the selector from exact failed
candidate `1dbe2d93…` to exact release `1edd368f…`, keeps all six command links
managed, and retains `1dbe2d93…`, `7d50218f…`, and original `e27ab263…` for
rollback. The selected shim reports the full new revision and hashes to build
value `45fd9f3a…`; all services remain active with zero restarts. The preceding
failed run still has an idle retained pool, so a controlled mkruntimed restart
is required before the harness's released baseline. No live replay is claimed.

Controlled mkruntimed restart `54167→59671` releases the pool. PID 59671 runs
the exact `1edd368f…/bin/mkruntimed` path and re-hashes to build value
`c7090da6…`; selector and unchanged boot `768706da…` agree. Kerf reports no
pool/no instances, default/moby/Docker counts and shim count are zero, and all
four services are active/running with zero restarts. The replay baseline is
released, clean, and candidate-bound.

Live reconnect capture `g6-forced-shim-reconnect-fifo-pass.log` closes exit 0
at `2026-10-01T11:57:02.480703Z`: mode 0600, 179,780 bytes, SHA-256
`fe6eee0d3711afc79c7153afbaba5f91efb478db139d812537018a9d383d6a18`.
Stable supervisor 60078 replaces worker `60083→60462` and namespace holder
`60275→60486` while Task stays RUNNING; child boot `60f644af…`, guest PID
162, bundle/task/sandbox/network generations, I/O offsets, and FIFO identities
are unchanged; post-fault exec succeeds; and the fault-time containerd journal
contains no disconnect cleanup. One unchanged attach stream contains all four
exact pre/post stdout/stderr markers. Captured events contain Task create/start,
all exec lifecycles, init exit, and delete in timestamp order. The bounded old
FIFO keepers are observable briefly as two shim-path processes, expire within
the harness wait, and retained-pool inventory reaches every other resource
zero. Both `G6_FORCED_SHIM_RECONNECT_PASS` and the scoped mode marker are
present. Final mkruntimed restart `59671→62681` releases the pool and yields the
all-zero inventory. Credential scan has no matches.

Independent post-pass audit binds selector and running mkruntimed PID 62681 to
exact release `1edd368f…`, executable hash `c7090da6…`, and unchanged host boot
`768706da…`; Kerf reports no pool/instances, every task/container/artifact/
record/endpoint/shim/helper count is zero, and mkruntimed, mknetd, containerd,
and Docker remain active with zero restarts. Together with exit-0 fallback
transcript `g6-forced-shim-reclaim.log` (`e833dcd1…`), both required forced-shim
outcomes now have immutable live evidence. Running-task forced-death reconnect
and the paired transcript requirement are closed; broader G6 event variants,
mkruntimed restart, and full final-resource matrix remain open.

Focused mkruntimed-restart harness is mode 0755/11,065 bytes/SHA-256
`c914296e60b4a86eb8fc27ddc49f0c8297d14d2d1772488003c624aeac09610e`.
It retains a continuous attach stream, exact host/daemon/task/child/shim
identities, safe hashes and projections of lifecycle snapshot/journal plus
rootfs/storage records, systemd-journal metadata, events, and retained/released
inventories without printing kernel command lines. Bash syntax and diff checks
pass. Commit `19270ec` freezes only this harness and the script index; live
behavior remains unclaimed.

First focused capture `g6-mkruntimed-restart-continuity-pass.log` is retained
as harness-only exit 1: mode 0600/128,479 bytes/SHA-256
`767e79734bbb0b4633caf6a9c837d0449d3a94369cbb078cc9ed87abb830f0ae`,
closed `2026-10-01T12:05:58.257050Z`. Product behavior through process exit
passes: mkruntimed `62681→63908`, unchanged host boot/containerd PID, RUNNING
Task, stable supervisor/holder, unchanged child boot `f11cae16…` and shim
recovery identity, post-restart exec, byte-identical durable snapshot/journal/
rootfs/storage summaries, a 13-entry restart journal with zero error-or-higher
entries, and all four pre/post continuous-stream markers. The harness then
waits only for STOPPED although `ctr tasks attach` validly removes the exited
task, producing ABSENT. Trap cleanup leaves retained pool with zero allocation,
no instances/tasks/containers/artifacts/rootfs/endpoints, and healthy services.
Credential scan has no matches. No pass marker or row closure is claimed.

Corrected terminal branch accepts STOPPED or attach-driven ABSENT and is frozen
by commit `00cb1e4`; harness is mode 0755/11,186 bytes/SHA-256
`23170b264c3463f8b1b37f4ab7aba31bddf4ce50be479d6941e92a202fd9692b`.
Idle restart `63908→65694` releases the pool and restores no instances/tasks/
containers. Guest copy independently matches the full hash and passes syntax.
Corrected live result remains unclaimed.

Corrected capture `g6-mkruntimed-restart-continuity-v2-pass.log` closes exit 0
at `2026-10-01T12:10:42.454130Z`: mode 0600, 136,321 bytes, SHA-256
`e99f610e008679cda2dbabf1f615d8fcc09d0ce9d07654d19d79871005bfd6f7`.
The controlled fault changes mkruntimed PID `65694→66493` without changing host
boot `768706da…`, containerd PID 14999, shim supervisor 66114, namespace holder
66313, child boot `0b2413c0…`, guest PID 162, task identity `task-9511…`, recovery
generation `b8c1a855…`, or network generation `6060fedb…`. Durable lifecycle
journal (1,490 entries, final StartSandbox complete), snapshot (sequence 1,489,
743 results), rootfs record, and empty storage projection are byte-identical
before and after. Post-restart exec succeeds; the restart journal has 13 entries
and zero error-or-higher entries (SHA-256 `b487687f…`); the original attach
stream contains both pre- and post-restart stdout/stderr markers. Timestamped
events retain Task create/start, every exec lifecycle, init exit, and delete.
Attach-driven task absence is accepted, retained-pool cleanup reaches every
other counter zero, final idle restart `66493→67287` releases the pool, and the
all-zero inventory plus scoped pass marker are present. Credential scan has no
matches.

Independent post-pass inspection resolves running PID 67287 to the exact
selected `1edd368f…/bin/mkruntimed` and SHA-256 `c7090da6…` on the unchanged
host boot. Kerf reports no memory pool and no instances; default/moby/Docker,
runtime artifact, rootfs, endpoint, shim, and FIFO-keeper inventories are zero.
mkruntimed, containerd, Docker, and mknetd are active/running with zero restarts.
This closes the focused mkruntimed-restart evidence requirement; broader G6
event variants and the complete final matrix remain open.

The next focused harness, `test-runtime-task-events-live.sh`, is mode 0755,
7,214 bytes, SHA-256
`5ad270835f191adda7cd97e01a19bc7eda8eff1b2b0f67a4b22d95d5660781ae`.
It couples client-observed exec exit 17 and exec/init SIGKILL exit 137 with an
exact create/start, exec-added/exec-started, exit/delete subsequence and
monotonic event timestamps. It also requires retained-pool cleanup followed by
released all-zero cleanup. Bash syntax and diff checks pass; commit, transfer,
and live behavior remain unclaimed. Commit `d7e2670` freezes only this harness
and the script index. Guest `/tmp/test-runtime-task-events-live-5ad27083.sh`
independently matches mode 0755, size, and full hash and passes Bash syntax.
The pre-run selector is exact candidate `1edd368f…`; Kerf reports no pool or
instances. Live result remains unclaimed.

First capture `g6-task-events-live-first.log` is retained as harness-only exit
1: mode 0600, 22,037 bytes, SHA-256
`c545888459dfeebbfe950601fe00e2ee68c65efae0cb06ca2e5d3d82eb761ef6`.
The live runtime reaches exec exit 17 and both exec/init SIGKILL exits 137.
Only event validation fails: containerd emits a seven-digit fractional second,
which Python's `%f` parser rejects. No ordering pass is claimed; the correction
must compare the full fractional timestamp rather than reducing its precision.

Commit `836f39e` corrects only that parser: one-to-nine fractional digits are
validated and zero-right-padded into an exact nanosecond ordering key. The
corrected harness is mode 0755/7,362 bytes/SHA-256
`bb7287e22af518e902873e4d348592676964e5520facb5c2bf94e5de6371dfb2`;
Bash syntax and diff checks pass. Corrected live replay remains unclaimed.

Guest corrected copy independently matches mode, size, full `bb7287e2…`, and
syntax. Controlled idle mkruntimed restart `67287→69091` releases the failed
run's pool; Kerf reports no pool or instances and default task/container plus
runtime-artifact counts are zero. Corrected replay has a clean baseline.

Corrected capture `g6-task-events-live-pass.log` closes exit 0: mode 0600,
68,210 bytes, SHA-256
`43cf0ba4422247b04c282be475e9fc2fcd3b0c42107e642a85a248161d1f3506`.
It retains exactly 12 Task events with their emitted fractional timestamps:
init create/start; nonzero exec added/started/exit/delete; signaled exec
added/started/exit/delete; then init exit/delete. The validator proves the
sequence exact and timestamps monotonic. The nonzero exec returns 17 to its
client and carries exit status 17 in both exit/delete events. SIGKILL of the
exec and init each returns 137 to its attached client and carries 137 in both
corresponding exit/delete events. Provenance binds selector `1edd368f…`, host
boot `768706da…`, mkruntimed PID 69091, and containerd PID 14999.

Cleanup reaches retained-pool/all-other-zero, idle restart `69091→70302`, then
released all-zero and `G6_TASK_EVENT_ORDER_PASS`. Independent post-pass audit
resolves PID 70302 to candidate mkruntimed hash `c7090da6…`, confirms no Kerf
pool/instances, zero links/rules/workloads/artifacts/rootfs/endpoints, and exact
process-name shim/NBD/relay counts zero. All four services are active/running
with zero restarts. This closes the exact Task v2 event order/timestamp row.

The complete shared matrix is now staged for final-candidate replay from its
unchanged commit `94396f7`. Guest matrix mode/size/hash are 0775/23,206 bytes/
`3015518c…`; resize helper is 0775/4,841 bytes/`fe7059cf…`. Both full hashes
match local frozen sources and Bash/Python checks pass. The exact selector is
`1edd368f…`; Kerf reports no pool/instances and ctr/Docker workload counts are
zero. Live replay remains unclaimed.

The in-progress final-candidate transcript has emitted observations through
image provenance, split lifecycle/state, distinct child identities, exec I/O,
private roots, mediated networking, sibling isolation, deliberate mkruntimed
restart, pause/resume, signal exit, normal deletion, and post-delete clean
inventory. It is currently inside the independent read-only-bind pair. This is
an execution checkpoint, not a terminal matrix-pass claim.

Both client-specific read-only-bind cases and their post-bind clean inventory
have now completed in that same session. It has entered repeated foreground
exit-17/name-reuse cycle 1. The run remains active; no terminal pass is claimed.

Both repeated ctr/Docker cycles now return exact 17 with their stdout/stderr,
clean normally, and reuse identical names. The same session has advanced into
guest-stdin forwarding; terminal matrix status remains unclaimed.

Guest stdin forwarding now passes for ctr and Docker. The same live session has
entered detached-task reattachment; no terminal matrix pass is yet claimed.

Detached reattachment now passes with guest I/O for both clients. The matrix is
inside PTY allocation and terminal-size verification; no final pass is claimed.

PTY mode now passes with exact `37 91` for both clients. The ctr post-start
resize has completed and Docker live resize is running; final cleanup and the
matrix pass marker remain unclaimed.

Complete final-candidate capture `g6-shared-matrix-final-candidate-pass.log`
closes exit 0: mode 0600, 315,263 bytes, SHA-256
`8ee2f800c96a7b49f62875f73b1b26f04d99c828570eeb4b40ed956471a10746`.
All 19 feature rows pass for ctr and Docker with expanded commands and observed
values. Child boots `5e71eaba…` and `81ddc67b…` are distinct from unchanged
host boot `768706da…`; deliberate mkruntimed restart `70302→72792` preserves
both. Both post-start resizes report exact `37 91`. Pre-shutdown inventory is
retained-pool/all-other-zero; idle restart `72792→80554` yields released
all-zero. All five scoped assertions plus the matrix marker are true, and the
credential scan has no matches.

Independent post-pass audit resolves PID 80554 to selected release `1edd368f…`
and mkruntimed hash `c7090da6…`. Kerf reports no pool/instances; default/moby/
Docker workload counts, runtime artifacts, rootfs records, endpoints, links,
NAT/filter rules, and exact shim/NBD/relay process counts are zero. All four
services remain active/running with zero restarts. This closes the complete
replacement-instance shared-matrix row. Explicit final mount and FIFO counts
remain necessary before closing the broader final-resource row.

Dedicated `audit-runtime-final-resources-live.sh` is mode 0755/4,061 bytes,
SHA-256 `bc5e2f7d52bdf095a26bea6b019187a530fb848cbafb6ff21e95ef554bdecfac`.
It requires no Kerf pool/instances and explicitly counts runtime mounts,
storage/bundle artifacts, FIFOs, links/routes/firewall rules, both containerd
namespaces, Docker, rootfs/network records, and exact shim/NBD/relay process
names. Bash syntax and diff checks pass; commit and live result are unclaimed.
Commit `c27bf3b` freezes only the audit and script index; live result remains
unclaimed.

Final-return capture `g6-final-resource-return-pass.log` closes exit 0: mode
0600, 12,896 bytes, SHA-256
`3bc958626d131aa44389eaa9f6fdc3fb694200487890a825aa01fc9e8b9d7ff5`.
It binds exact selector `1edd368f…`, unchanged boot `768706da…`, PID 80554,
and executable hash `c7090da6…`. Kerf reports no memory pool and no instances,
which proves child CPU/memory return. All 19 explicit residual counters are
zero: runtime mounts, storage/initramfs and bundle artifacts, FIFOs, TUN links,
routes, NAT/filter rules, default/moby task and container inventories, Docker,
rootfs/network records, and exact shim/NBD/relay process names. All four
services remain active/running with zero restarts. The scoped pass marker is
present and credential scan is empty. This closes the final-resource row.

The remaining aggregate restart gap is clean shim-worker replacement. The
existing shim harness now accepts only `KILL` (unchanged default) or `TERM`;
the `TERM` mode runs the same reconstruction, stable identity, continuous I/O,
events, and cleanup assertions and emits a distinct clean-restart marker. The
updated harness is mode 0775/15,953 bytes/SHA-256
`703acc35482b10970b0d1d289c66a360360b712de655dab5280d7173e92a9871`;
Bash syntax and diff checks pass. Commit and live behavior remain unclaimed.
Commit `2ce142b` freezes the harness change; live behavior remains unclaimed.

Guest clean-restart copy independently matches mode, size, full `703acc35…`,
and syntax. The host selects exact `1edd368f…`, Kerf reports no pool/instances,
and ctr/Docker workload counts are zero. Live result remains unclaimed.

The first `TERM` attempt terminates harness-only exit 1: 27,703 bytes, SHA-256
`8d1c209cf3952cc76a87733bf1d8898be5c6e2edac5bd61405c8f690addbb445`.
Orderly worker termination removes the Task instead of reconstructing it in
place, so reusing forced-death continuity assertions was the wrong clean-shim
contract. The later host reboot removed this `/tmp` transcript before local
copy; only terminal metadata and output observed through the live execution
handle survive. It is not closure evidence. The corrected qualification must
prove orderly shutdown, shim exit, same-name task/shim recreation, and cleanup.

After the operator restart, the host is on new boot
`d9cdfa98-df65-4bce-b3a2-08565857c69e` with unchanged kernel and exact selected
release `1edd368f…`. mkruntimed/containerd/Docker/mknetd are active with zero
restarts, Kerf reports no pool or instances, and default/moby/Docker workload
inventories are empty. This is the new live-evidence baseline.

Corrected `test-runtime-clean-shim-restart-live.sh` is mode 0755/6,608 bytes,
SHA-256 `84e982801a17ec534eaed0d9d97ba4cb785e08de278e0a7320f5d8c41aebafdb`.
It runs two complete same-name clean lifecycles, requires the first supervisor,
worker, and namespace holder to exit, requires distinct replacement shim and
child identities, retains both Task event sequences, and audits retained then
released cleanup. The forced-death harness is restored byte-for-byte to its
pre-`TERM` source (`e47804f5…`), separating graceful shutdown/recreation from
forced continuity. Syntax/diff checks pass; commit/live result are unclaimed.
Commit `a8cdc1c` freezes the separated harnesses; live result remains unclaimed.

Guest corrected harness independently matches mode, size, full `84e98280…`,
and syntax on exact selected candidate `1edd368f…`. Kerf has no pool/instances
and ctr/Docker inventories are empty. Live result remains unclaimed.

First corrected-harness capture `g6-clean-shim-restart-v2-first.log` is retained
pre-workload exit 1: mode 0600/12,987 bytes/SHA-256
`54465f4b323f572cfc4a6d9ba1366c43a2b94cb91781ccff70c1994001021724`.
Bash expanded the stream path before the same `local` statement assigned
`cycle` under `set -u`. No task was created and cleanup retained the released
baseline. The split declaration is a harness-only correction; no product claim.

Correction commit `29af02b` produces a mode 0755/6,616-byte harness at SHA-256
`80ddbd858ae928007fd6b34d03e30c35b7aade008955ec75901c9ac578a18ff5`;
Bash syntax and diff checks pass. Corrected live result remains unclaimed.

The corrected live run has completed cycle 1 with normal output, confirmed exit
of its supervisor/worker/namespace holder, and retained-pool/all-other-zero
inventory. The same task name is accepted for cycle 2; no final pass is claimed.

Corrected capture `g6-clean-shim-restart-pass.log` closes exit 0: mode 0600,
80,418 bytes, SHA-256
`754a05f60c0bb906aa9ad2c6c5ff8d2e971b36e10f541f7fde03e69c5f201de8`.
Cycle 1 supervisor/worker/holder `3012/3017/3213` and child boot `d9282f53…`
exit cleanly. The identical task name then creates distinct shim identities
`3514/3519/3763` and child boot `69a9ff52…`. Both attach streams contain exact
start/exit markers, both Task event lifecycles are retained, and each cleanup
reaches retained-pool/all-other-zero. Final mkruntimed restart `1466→4061`
releases the pool to all-zero. Credential scan has no matches.

Independent audit binds PID 4061 to selected `1edd368f…` and executable hash
`c7090da6…`, with no Kerf pool/instances, workloads, runtime artifacts, or
shim/NBD/relay processes and four active zero-restart services on boot
`d9cdfa98…`. Combined with the exact containerd (`afa9187e…`), Docker
(`fa381592…`), mkruntimed (`e99f610e…`), forced reconnect (`fe6eee0d…`), and
bounded reclaim (`e833dcd1…`) captures, every named restart variant and the
cross-process ownership-transfer row now has direct exit-0 evidence.

The focused signal/event harness is expanded to mode 0755/8,800 bytes/SHA-256
`b09bf5209dcee122d056ea31675cc06201794d10a4a7ddcfe2ff530e3f21a525`.
It now sends ignored SIGTERM to exec and init and proves they remain live with
no exit event, SIGKILLs the exec process group and verifies its descendant PID
is gone, retains attach wait/delete behavior, recreates the same task name after
init SIGKILL, and requires two exact init event lifecycles plus released
cleanup. Bash syntax and diff checks pass; commit/live result are unclaimed.
Commit `1fb265d` freezes the expanded harness; live result remains unclaimed.

The in-progress run has proved exec exit 17, ignored exec SIGTERM with no exit
event, exec-process-group SIGKILL exit 137 with its descendant gone, ignored
init SIGTERM while RUNNING, and init SIGKILL exit 137 through the attached
waiter. Same-name recreation after the signal failure is running; no terminal
pass is claimed yet.

Expanded capture `g6-signal-lifecycle-pass.log` closes exit 0: mode 0600,
78,247 bytes, SHA-256
`6647cc0e15d68dd6b78ed8c29e915ffa685df4f4126938961ba9049654f627b8`.
Exec SIGTERM leaves the client waiting and publishes no exit event. Exec-group
SIGKILL returns 137 and guest descendant PID 177 is confirmed absent. Init
SIGTERM leaves the Task RUNNING with no exit; init SIGKILL returns 137 through
the attached waiter, whose completion drives delete. The identical task name
then runs successfully. Twenty monotonic timestamped events include two exact
init lifecycles. Retained cleanup is all-other-zero and restart `4061→6187`
releases the pool.

Final-candidate matrix `8ee2f800…` independently supplies repeated nonzero 17
and same-name reuse. Credential scan is empty. Post-pass audit resolves PID
6187 to candidate hash `c7090da6…`, finds no pool/instances/workloads/artifacts
or shim/NBD/relay processes, and all four services healthy with zero restarts.
Together these close the aggregate signal/exit/descendant/wait-delete/reuse row.

New `test-runtime-concurrent-churn-live.sh` is mode 0755/9,148 bytes/SHA-256
`d425cc399297954edb34fa6f88104b37f9507bca7f76bb948262d71049982e48`.
It starts ctr and Docker concurrently, projects safe durable lifecycle/recovery
state, and requires disjoint CPU sets, memory owners, generations, bundles,
storage paths/ports, agent ports/CIDs/sockets, task identities, and network
generations/addresses. It then runs 12+12 parallel execs, simultaneous pause/
resume, distinct child boots, and retained/released cleanup. Bash syntax and
diff checks pass; commit and live behavior remain unclaimed.
Commit `9616398` freezes the harness/index; live behavior remains unclaimed.

Guest concurrency harness independently matches mode, size, full `d425cc39…`,
and syntax; Kerf has no pool/instances and ctr/Docker inventories are empty.
Live behavior remains unclaimed.

Concurrent capture `g6-concurrent-churn-pass.log` closes exit 0: mode 0600,
96,522 bytes, SHA-256
`09a0c135eeb56c2ac9b1bec45038754e615c444faf66862f689fc612124ec543`.
The safe durable projection proves ctr/Docker use disjoint CPU sets `[8,10]`
and `[12,14]`, separate 3-GiB memory allocations, generations `474f49fe…` and
`12724968…`, distinct bundles, agent ports 7200/7201, child CIDs 40/41, agent
socket inodes 3116/3189, storage paths/ports 4061/4062, task identities,
network generations, and addresses `172.31.0.2/30`/`172.31.0.6/30`. Both run
12 parallel execs, simultaneous pause/resume, and retain distinct child boots.

Cleanup reaches retained-pool/all-other-zero; restart `6187→9410` produces
released all-zero and the scoped pass marker. Credential scan is empty.
Independent audit binds PID 9410 to candidate hash `c7090da6…`, finds no pool,
instances, workloads, runtime artifacts, or shim/NBD/relay processes, and four
healthy zero-restart services. This closes the concurrent-churn row.

On 2026-10-02, immediately after the operator-reported VM restart, a fresh
direct checkpoint observes the same boot `d9cdfa98-df65-4bce-b3a2-08565857c69e`,
kernel `7.0.0-mk2-gce-lab`, exact selector `1edd368f640f28e880480c638c0936a5cbaba6b0`,
and mkruntimed PID 9410/executable SHA-256 `c7090da6…`. mkruntimed, containerd,
Docker, and mknetd are active/running with zero service restarts. This records
continuity before further qualification and makes no new completion claim.

The first resumed focused Go-test enumeration did not reach compilation. Its
isolated `/tmp` module cache was empty, and sandboxed DNS denied downloads from
`proxy.golang.org`. This is recorded only as a diagnostic; it is not test
evidence and requires an approved rerun with dependency access.

With approved access, enumeration finds 128 top-level shim tests. The complete
package race run is retained in `g6-task-v2-unit-race-baseline.log`, mode 0600,
53,953 bytes, SHA-256
`d1b5fa5e6a6e69c037e74d6b614191027a69256bd8e20c388880261ab02614ee`.
It passes in 8.516 seconds with 327 run entries and 138 passing groups. Only
`TestAcquireStartShimSocketWithRealLiveAndStalePaths` and
`TestStaleRelayCleanupRemovesExactSafeSocket` skip because this local sandbox
forbids pathname Unix listeners. Those tests must run on the disposable VM;
the clean baseline alone does not satisfy the exhaustive aggregate rows.

Before source transfer, the disposable VM reports Go 1.26.0/linux-amd64,
32 GiB free on `/tmp`, and no existing
`/tmp/multikernel-g6-source-9616398`. The qualification copy therefore uses a
unique temporary path and does not replace the selected installed release.

The transferred tree has 100 files and 45,747,628 bytes. Local and remote
SHA-256 values match for `main_test.go` (`6051decb…`), `go.mod` (`407622e6…`),
and `go.sum` (`9f40acd1…`), binding the VM qualification to the same source and
dependency lockfiles as the local race baseline.

The first full VM race run is preserved rather than overwritten:
`g6-task-v2-vm-race-first-fail.log`, mode 0600, 55,813 bytes, SHA-256
`9a80ebc2df7d13c598afc7d5705ec4d99c860165a6515b2114ca34b4e4f6b579`.
It exits 1 with 327 run entries, 135 passing groups, and no skips. Five tests
fail at the common safe-file boundary: unsafe token permissions,
group-accessible FIFO permissions, unsafe journal permissions, deterministic
socket cleanup, and authenticated fallback socket cleanup. No completion is
claimed pending diagnosis of fixture behavior and VM `/tmp` filesystem modes.

The common cause is the qualification wrapper, not a filesystem or product
defect: wrapper `umask 077` reduced requested fixture modes 0644/0660 to 0600.
That made three intentionally unsafe fixtures safe and made two exact-0644
containerd address fixtures invalid. The corrected run uses normal umask 022
for tests and explicitly seals only the resulting transcript to mode 0600.

The corrected VM result is `g6-task-v2-vm-race-pass.log`, mode 0600, 53,765
bytes, SHA-256
`ff7cc0e0f49e6334dd7948b1cb12c8df356140cda6aba01c6e31349d849a10db`.
It exits 0 under `-race -count=1` in 8.294 seconds with 327 run entries, 140
passing groups, no skips, and no failures. Both real pathname-socket tests that
the local sandbox skipped run and pass on the disposable VM. This validates the
present focused suite; exhaustive matrix rows still require exact coverage
mapping rather than promotion from package success alone.

`TestEventJournalCompleteLifecycleOrderAndPersistenceFailureMatrix` now covers
the exact create, start, exec-added, exec-started, exit, delete sequence under a
failed broker attempt for every enqueue. It requires durable sequences 1..6,
restart reconstruction, exact ordered replay, and journal removal after full
acknowledgement. Six per-topic subtests inject persistence failure before
publication and require no broker call and exact in-memory rollback. The new
matrix passes 20 race-detector repetitions in 1.435 seconds; broader reruns are
not yet claimed.

The cancellation/deadline inventory maps direct tests to all eight named
domains: rootfs/storage mutation and hashing, bounded builder descendants,
daemon dial/write/read/default timeout, Kerf child commands, agent connect and
call, stdio opens/pumps, Task lock/wait, and teardown/network cleanup. The full
local `runtime/...` race run passes 22 tested packages with 968 run entries and
466 passing groups. Its mode-0600/144,982-byte transcript
`runtime-all-packages-race-cancellation-baseline.log` hashes to
`939368f0806b3e64a6e0c8d0fa5d20a90e94abc1a628f96ae647ac83b95902ea`.
Eleven Unix-socket/descriptor tests skip only under the local sandbox; a VM
zero-skip run and live cross-service leak qualification remain pending.

The first full-module VM run is preserved as
`runtime-all-packages-vm-race-cancellation-first-fail.log`, mode 0600/143,337
bytes/SHA-256
`78117ffdcebcbb018694513cab3bceca425850f5bc657f0914ceef6e9a4f5af1`.
It exits 1 with 959 run entries, 468 passing groups and no skips. All nine
failures are `internal/storage` fixtures executed beneath VM tmpfs, where
allocated image extents cannot be inspected; the backend rejects them as
sparse/unverifiable before their target assertions. This is an environment
mismatch, and the same source must be rerun from persistent disk.

Direct filesystem inspection confirms `/tmp` is tmpfs and `/var/tmp` is ext4
on `/dev/root`. The exact copy at
`/var/tmp/multikernel-g6-source-dbdaf1b` retains `main_test.go` SHA-256
`10101b2d…` and totals 45,754,932 bytes. The corrected invocation also sets an
ext4-backed `TMPDIR`, ensuring test fixtures themselves use inspectable extents.

The ext4-backed rerun is preserved as
`runtime-all-packages-vm-race-cancellation-ext4-fail.log`, mode 0600/144,996
bytes/SHA-256
`87856460975953392b6c1f554db5cc02a1734a248f55112050e1ffa19ee499d5`.
It reaches 968 run entries and 472 passing groups with no skips; every storage
test now passes. The only five failures are Unix listeners returning `EINVAL`
because the long ext4 `TMPDIR` makes generated paths exceed Linux `sun_path`.
The next run retains ext4 but uses a short unique temp prefix.

With short ext4-backed `TMPDIR=/var/tmp/mkg6t`, the complete runtime module
passes under `-race`: 968 run entries, 477 passing groups, all 22 tested
packages, zero skips and zero failures in 12 seconds. The retained transcript
`runtime-all-packages-vm-race-cancellation-pass.log` is mode 0600/144,078
bytes/SHA-256
`94f077c3b6837ef90f8416202f0779c4bd3bba0957c3b9b5b1368f737fc4edb0`.
This closes the automated every-blocking-boundary cancellation/deadline row;
the separate live cross-service cancellation/leak requirement remains open.

The next OCI audit identifies a genuine remaining boundary. Exec process shape
is validated before guest mutation, and rootfs/network inputs are checked before
allocation, but the complete init OCI contract is still loaded and rejected by
the guest during Start—after Create can own rootfs, storage, sandbox, and
network resources. Generic lifecycle crash/cancel rollback does not prove the
unsupported-OCI matrix. A shared host/guest validator and deterministic failure
injection at every later application boundary are required before promotion.

Pre-allocation OCI work now introduces `ValidateRootfs` before shim allocation.
The daemon binds it to the held bundle identity; Linux invokes the canonical
descriptor-bound builder with `MK_VALIDATE_ONLY=1`; the builder exits directly
after `validate-runtime-oci.py`, before task/storage requirements or artifacts.
Bash syntax passes. The first compile run is not a product result: test fake
backends need the new interface method and one shim daemon fixture must accept
the added call before focused ordering/no-mutation qualification can run.

The shim pre-allocation rejection case passes 20 race repetitions. The first
rootfs focused command did not execute behavior because its new test passed the
`*os.Root` verifier handle where the backend requires the production-style held
`*os.File`. That non-evidence fixture error is corrected by reopening `.`
descriptor-relative through the verified root.

With the production-style held descriptor, both rootfs validation tests and the
shim reject-before-allocation test pass 20 race-detector repetitions. These
focused results prove local ordering and no artifact/state mutation only. Full
suites and a disposable-VM deployment/behavioral qualification of the changed
daemon, shim, and builder are still required.

The canonical Python OCI suite passes 95 semantic cases plus namespace,
file-identity, and outer-cleanup boundaries, and the changed builder passes
Bash syntax. The first all-Go invocation came from the repository root, outside
the `runtime` module, and therefore ran no tests; it is explicitly non-evidence
pending the same race command from `runtime/`.

From the correct module root, `go test -race -count=1 ./...` passes all tested
runtime packages. The changed shim, daemon, and rootfs packages pass in 9.537,
1.191, and 1.717 seconds respectively; storage passes in 3.777 seconds. This is
a local integration baseline, not yet VM evidence.

The operator restart yields boot `f1d650a2-373b-4fe7-9a96-382e51332172` on
kernel `7.0.0-mk2-gce-lab`. mkruntimed/containerd/Docker/mknetd are active with
zero restarts at PIDs 1467/1485/1528/1236, and ctr/Docker have no running
workloads. The selected release is still control `1edd368f…`; none of the new
OCI validation code is claimed active yet.

Commit `11a65f0` freezes only the pre-allocation implementation and tests.
SHA-256 identities are builder `aae6496c…`, shim `0107ddd4…`, rootfs backend
`0bb75277…`, and daemon server `5c4ca54b…`; the evolving ledgers and evidence
trees remain separate from that focused code commit.

The exact commit archive is 1,043,133 bytes/SHA-256 `bf8a16d3…`. Its unique VM
extraction `/var/tmp/mksrc-11a65f0` has 637 files/4,163,480 bytes, and remote
builder, shim, rootfs-backend, and daemon-server hashes match local byte-for-
byte. Subsequent VM qualification is therefore bound to `11a65f0`.

Exact-commit VM transcript `g6-oci-preallocation-vm-race-pass.log` is mode
0600/146,655 bytes/SHA-256
`f751bdea06bc530852016e748119b0872b67544a7bcb83d725a8519b4c0723fa`.
It closes exit 0 with all 95 canonical OCI cases plus 971 Go run entries, 480
passing groups, 22 tested packages, and zero skips/failures. The new shim
pre-allocation, rootfs identity/no-artifact, and held-descriptor backend tests
all execute and pass. Installed-service qualification remains unclaimed.

The first root-owned build attempt changed only temporary source ownership and
then stopped before compilation: the ordinary outer shell could no longer
enter the root-owned mode-0700 directory. No build or installation claim is
made. The correction runs directory entry, build, and hashing inside one
privileged shell.

The corrected privileged build completes for exact full revision
`11a65f08f07b6bb88a6eb3088301582a37f55005`. SHA-256 values are release
manifest `e22e76a6…`, mkruntimed `0e1c87c3…`, shim `326a8274…`, mknetd
`2dbe69d2…`, and agent `c7e61607…`; the built daemon reports that exact stamp.
No installation or active-service claim is made yet.

The immutable managers install and select exact binary release
`0.1.0-dev-11a65f08f07b6bb88a6eb3088301582a37f55005` plus support generation
`b4d185c68b567c8d44882b34978cd18ac29a67d22f264cc9b4d719971a84371f`.
Inspection reports every managed link present. This is selection evidence only;
services have not yet been reloaded or shown to execute the candidate.

After daemon reload and empty-host restart, running mkruntimed PID 15863
resolves into release `11a65f08…`, hashes to candidate `0e1c87c3…`, and reports
the full revision. Public shim and active builder hash to `326a8274…` and
`aae6496c…`; the builder resolves under support generation `b4d185c6…`.
mkruntimed, mknetd, containerd, and Docker are active/running with zero
restarts. Live workload semantics remain pending.

The installed-service negative case is now fixed precisely. String-valued OCI
annotations are supported and therefore cannot prove fail-closed behavior.
Instead, containerd's `--apparmor-profile` produces
`process.apparmorProfile`; any value other than the explicit inert
`unconfined` compatibility value is rejected by the canonical validator. The
restarted VM begins this test with no configured memory pool, child instance,
containerd/Docker workload, or runtime-tree entry. A retained harness will
prove that exact rejection leaves the same 19-category zero inventory before
running the positive control and final pool-release audit.

The first live harness attempt is retained but is not product evidence. It
stopped before either workload because the harness looked for the public daemon
under `/usr/local/bin`; this image's unit executes
`/usr/local/sbin/mkruntimed`. The mode-0600, 3,064-byte transcript hashes to
`95a5a7207b75bfae14aaee279706627735bd440eef815173b2c9fb088b645d21`.
The harness now uses the service's actual public path; no runtime allocation or
mutation occurred in the failed attempt.

The next attempt also stopped during provenance preflight, before injection:
the installed runtime name is `containerd-shim-multikernel-v2`, not the
abbreviated `containerd-shim-mk-v2` assumed by the harness. Its mode-0600,
3,761-byte transcript hashes to
`7bec72a3b20f87928256159de14d58d082e2cfbed3e64893cf8d5c2a6e0c1334`.
The actual public symlink resolves into the selected `11a65f08…` generation;
the harness now hashes and records that path. This remains a harness-only
failure and supplies no workload claim.

The third attempt proves every candidate provenance assertion, then stops
before inventory or injection because this containerd build has no
`ctr images info` command. Its mode-0600, 8,305-byte transcript hashes to
`863512bd7826f7cef09c6ac604e079f487bf11b1becdeccf6c55725e32f92311`.
An exact `ctr images list -q` match confirms the required busybox reference is
present, and replaces that incompatible preflight command. Again, the stop
precedes product behavior and is not a negative OCI result.

The first behavioral attempt reaches the intended canonical rejection. Its
error states `validate OCI bundle before allocation`; immediately afterward
the memory pool remains unconfigured and all allocation-bearing categories are
zero: children, mounts, storage/bundle artifacts, FIFOs, links/routes/firewall,
container/task records, rootfs records, endpoints, NBD, and relay processes.
Two shim processes still existed during that immediate audit and were gone by
the follow-up inspection, with no task, container, or task-directory residue.
The harness deliberately failed rather than hiding this asynchronous reaping.
Its mode-0600, 28,379-byte transcript hashes to
`6503a004b20fe324d6e3a6306af83229a437777c36d7e418b69fdc4a0eba070c`.
The corrected contract preserves the immediate no-allocation assertion while
allowing up to 60 seconds for shim teardown, then demands the exact full zero
inventory before the positive control.

Corrected live qualification passes on exact `11a65f08…`. Private transcript
`g6-oci-preallocation-live-pass.log` is mode 0600/129,202 bytes/SHA-256
`c409237a4041960a6cd0acfb506ba89387a450fe0ba0f6ff8672a4b2da7c7cd3`.
Its seven observation blocks prove candidate daemon/shim/builder identity,
empty initial state, canonical pre-allocation AppArmor rejection, no immediate
allocation-bearing resources, bounded shim reaping to exact 19-category zero,
supported output `MK_OCI_SUPPORTED_PASS`, and exact final zero after releasing
the intentionally reusable pool. mkruntimed, mknetd, containerd, and Docker are
active/running with zero restart counts. The transcript has no credential-
pattern matches. This closes only the pre-allocation part of the composite
requirement; injected cleanup after every possible later partial application or
allocation boundary remains unproved and the checklist row stays open.

Post-pass source audit enumerates the still-open second clause precisely.
After canonical validation, Create can fail at rootfs preparation, runtime-
directory handoff, token acquisition, ambiguous sandbox creation/cancellation,
sandbox load, network provision, namespace-holder creation, recovery
persistence, or create-event publication. Rollback owns holder stop, endpoint
release, generation-bound sandbox deletion, prepared-root cleanup, token reset,
and process removal. Current focused coverage proves the pre-allocation case
and one ambiguous-create cancellation path, but not a deterministic injected
matrix across every later ownership stage. No composite completion claim is
valid until that matrix exists and passes.

The new deterministic Create rollback matrix now compiles and passes once
under the race detector across all nine enumerated post-validation stages. It
requires exact cleanup counts and no surviving process, sandbox, token,
endpoint, holder, or runtime directory; stage-specific assertions require
create cancellation, generation-bound sandbox deletion, network release, and
holder stop only after ownership is acquired. Two setup attempts are explicitly
non-evidence: one used repository-root paths from the module directory and the
read-only default Go cache, and the next compiled far enough to identify a
missing test-only `slices` import. Repetition, full suites, commit identity, and
VM execution remain required before the second clause can close.

The matrix then passes 20 race-detector repetitions: 180 injected stage
executions complete in 1.431 seconds. The complete shim package also passes
`-race -count=1` in 9.582 seconds. These results remove focused/package-level
regression uncertainty; repository-wide execution and disposable-VM zero-skip
evidence remain pending.

The full local runtime module passes `go test -race -count=1 ./...` across all
22 tested packages. The changed shim package completes in 9.561 seconds;
agent/rootfs/storage complete in 10.432/1.722/3.793 seconds. This is the final
local baseline for the test-only matrix commit, not yet disposable-VM proof.

Commit `3fd1238` freezes only the nine-stage rollback matrix at full revision
`3fd1238667899b0a6c721f14685cef4253556990`. The resulting shim test file
SHA-256 is
`374f8a3ade7ac99964df5c0e3b1d16f57a98ca4a0ad89f6a6cad1feee8f0f763`.
The living ledgers and evidence remain outside that focused commit; exact-
commit VM transfer/execution is next.

Exact archive transfer matches locally and remotely at 1,046,372 bytes/SHA-256
`43e23a00ec23e95073e49edfa4c0a253521a74862070f2341d9e1ed71adfea36`;
the fresh ext4 extraction has 638 files and the remote test-file hash matches
`374f8a3a…`. The first full VM run is retained but fails because the wrapper
repeated the already-known `umask 077` mistake: deliberately permissive mode
fixtures became 0600, causing unsafe-mode and exact-0644 socket/address tests to
exercise different inputs. The nine-stage matrix itself passes all subtests.
Mode-0600 transcript `g6-oci-postvalidation-rollback-vm-race-first-umask-fail.log`
is 147,009 bytes/SHA-256
`59b40720f69b02bfc46425418b2b278c3395a301156eddedc7107b1461976197` with
981 run entries, 965 pass lines, zero skips, and suite exit 1. No completion
claim is derived; rerun requires `umask 022` and a separately presecured log.

The corrected exact-commit VM suite passes with ordinary fixture umask and a
presecured mode-0600 transcript. Retained
`g6-oci-postvalidation-rollback-vm-race-pass.log` is 146,004 bytes/SHA-256
`20a3f6a2079a7a63536318c4b0ebebf5ed8d1e61318128f84b4f2b54e6e73652`:
981 run entries, 981 pass lines, 22 successful packages, zero skips, and zero
failures. All nine rollback stages execute on ext4 and pass. The credential
scan returns zero matches. Together with installed-service live evidence
`c409237a…` for canonical pre-allocation rejection, positive-control workload,
and exact resource return, this closes the composite unsupported-OCI/partial-
ownership cleanup checklist row.

Independent post-qualification audit also passes. Private transcript
`g6-oci-final-resource-audit.log` is mode 0600/12,607 bytes/SHA-256
`29340822f6c5c5078bd078ac3fa1358cc052b52fd1524199e369158b25788e5f`.
It rebinds running mkruntimed PID 19810 to exact selected `11a65f08…` and hash
`0e1c87c3…`, proves no pool or child, and reports every one of the 19 audited
resource categories at zero. mkruntimed, mknetd, containerd, and Docker remain
active/running at PIDs 19810/15841/1485/1528 with `NRestarts=0`. Credential
scanning returns zero matches. The checklist now contains 34 closed and 51
open top-level rows; broader G4-G6 completion remains unfinished.

The repository documentation/evidence gate passes after the closure update:
local-link structure, 7 runtime schemas/22 cases, 17 current evidence
manifests, the 95-case OCI validator, bind/bootstrap/rootfs/storage/image
fixtures, release/deployment lifecycle checks, GCE ledger, evidence capture,
containerd config, and final G4-G6 evidence audit all pass. One local socket
fixture remains explicitly skipped for sandbox `EPERM`; the exact VM Go suite
already runs with zero skips.

Packaging closure audit selects the next concrete gap: immutable binary and
support managers have full alternate-root tests, but the G6 row still lacks a
privileged live rollback/forward-restoration transcript. Initial preflight
correctly changes nothing: manager CLIs are not published under the assumed
`/usr/local/libexec/multikernel` paths. Current selectors remain binary
`11a65f08…` and support `b4d185c6…`; mkruntimed PID 31453 hashes to exact
candidate `0e1c87c3…`, reports full revision `11a65f08…`, and has
`NRestarts=0` with start timestamp 06:48:49 UTC. Read-only exact-source binary
manager inspection reports all managed links valid and 15 preserved releases.
The ordinary-user support inspection is intentionally non-evidence because
the manager rejects `/etc` ownership; a root-owned hash-verified temporary copy
is required before privileged inspection or activation.

The coherent retained rollback pair is confirmed before mutation: binary
revision `1edd368f…` with daemon/shim/mknetd SHA-256 `c7090da6…`/
`45fd9f3a…`/`d5cd8aff…`, and support deployment `25e6d343…` with builder
SHA-256 `0ded581c…`. ctr and Docker inventories are empty. Root-owned temporary
manager copies match exact-source SHA-256 `39026a02…` and `ec38d8d2…`; support
inspection reports seven deployments and all 21 links valid. Fail-safe live
harness commit `6194e02` (full `6194e02bc2b9e5596075b2bc5fbf3fd2be311863`)
has script SHA-256 `642e382b…`; execution remains pending.

The first harness run is retained as a pre-mutation tooling failure. The
root-owned binary manager lacked its required sibling
`runtime-release-manifest.py`; `restore_required` was still zero, and follow-up
proves both selectors, all four service PIDs/states, and zero restart counts
unchanged. Transcript `g6-packaging-rollback-live-first-manager-dependency-fail.log`
is mode 0600/4,697 bytes/SHA-256
`1feda951d90987977926ad6fc18b179d84a58fc72dd495c4f17e413168ecfcf2`.
The dependency is now root-owned mode 0644 with exact-source SHA-256
`4ae16ce4…`; binary inspection succeeds with candidate active, 15 releases, and
all links present/managed. No rollback claim comes from the failed run.

The restarted-instance packaging preflight remains non-mutating and passes.
Both selectors still name candidate binary `11a65f08…` and support deployment
`b4d185c6…`; mkruntimed, mknetd, containerd, and Docker are active with
`NRestarts=0`. The uploaded qualifier exactly matches hardening commit
`4c33b7198209560d7b36ace692576031158f65cb` and SHA-256 `1c8f836b…`.
Its root-owned binary/deployment managers remain mode 0755, while sibling
`runtime-release-manifest.py` is mode 0644 and retains exact SHA-256
`4ae16ce4…`. Commit `4c33b71` replaces a `grep -q`/`pipefail` combination with
a consuming fixed-string check so a successful containerd dump cannot be
misreported as a SIGPIPE failure. Live rollback/forward execution is next.

The next live run uncovers a genuine packaging integration gap after safely
activating and identifying the rollback generation. Old selectors, full
revision `1edd368f…`, running daemon SHA `c7090da6…`, public shim SHA
`45fd9f3a…`, support builder SHA `0ded581c…`, all service states, and Docker
default `runc` pass. The run then finds no multikernel entry in
`containerd config dump`; direct inspection shows `/etc/containerd/config.toml`
does not exist, so the managed `conf.d` fragment is not loaded. The EXIT trap
restores candidate selectors `11a65f08…`/`b4d185c6…`; independent follow-up
binds PID 2618 to candidate daemon SHA `0e1c87c3…`, shows all four services
active with zero restarts, six container/task inventories at zero, and no
pool or child. Retained non-passing transcript
`g6-packaging-rollback-live-second-containerd-config-fail.log` is mode 0600,
27,566 bytes, SHA-256
`1dc4121d934eeb8b672ae038cb648d5c5bf58a28bafa9b5592e0c8e3cb915aa3`.
The packaging row remains open pending an installer-level containerd config
integration fix and a fresh rollback/forward run.

Containerd inspection narrows that gap to the documented fresh-host operator
step: version 2.2.2's generated full default already imports
`/etc/containerd/conf.d/*.toml`; no product-side partial-config merger is
needed. On the idle host, the complete generated candidate is parsed through
containerd before installation and proves default `runc`, `io.containerd.runc.v2`,
and named `multikernel` type `io.containerd.multikernel.v2`. Atomic mode-0644
installation yields `/etc/containerd/config.toml` SHA-256 `54a1d02d…`; its
selected versioned fragment is mode 0644/SHA-256 `54c85792…`. After restart,
the live dump proves the same default and named runtime, Docker still reports
default `runc`, default/moby tasks and containers are all zero, and containerd
reports `NRestarts=0`. Private transcript
`g6-containerd-fresh-host-config-pass.log` is mode 0600/3,225 bytes/SHA-256
`20417ecae26f2c7863bd9742634ddf540e3ef8e816e37207e3ef2ff8a5fb4437`;
its credential-pattern scan is empty. Full generation rollback/forward
qualification remains required before packaging closure.

The following full qualifier proves the rollback generation and imported
containerd runtime, but correctly remains non-passing because the first ctr
call races daemon socket readiness. Systemd reported the old mkruntimed active
at monotonic 521.708 s; the ctr create occurred before `/run/mkruntimed.sock`
was published, and the trap began restoration at 522.132 s. This is a harness
readiness defect: service `active` is not the runtime RPC readiness boundary.
The trap restores candidate selectors and daemon SHA `0e1c87c3…`; independent
follow-up finds both runtime sockets, all four services active with zero
restarts, all six ctr/Docker inventories zero, and no pool or child. Retained
`g6-packaging-rollback-live-third-service-readiness-fail.log` is mode 0600,
30,790 bytes, SHA-256
`ef40e1cd9323ca3770f8337c0d7b0c3f8ade5d650126de9dd4ce7168a90c22fc`.
The qualifier must wait for both mkruntimed and mknetd sockets after every
generation activation before it can substantiate rollback workloads.

Qualifier commit `4db2d9cdf7b1bc8d3be354d636ff82032d3e7541` implements that
readiness boundary: it waits up to 30 seconds for both Unix sockets, asserts
both are sockets, and records their type/mode/ownership in each generation
observation. Bash syntax, shellcheck when available, and diff checks pass; the
script SHA-256 is `2fa3188d…`. A new live run is still required.

The exact `4db2d9c` qualifier now passes the complete privileged packaging
cycle. It activates rollback binary/support generations `1edd368f…`/
`25e6d343…`, verifies their selected paths, full binary revision, running
daemon/public-shim/support-builder hashes, both ready mode-0660 root-owned RPC
sockets, active services, named containerd runtime, and Docker default `runc`.
The old-generation workload prints `MK_PACKAGING_ROLLBACK_PASS`; after exact
19-category cleanup, the manager atomically restores candidate generations
`11a65f08…`/`b4d185c6…`, repeats every identity/default/readiness assertion,
and the candidate workload prints `MK_PACKAGING_FORWARD_PASS`. Its final
cleanup returns all 19 categories to zero with no pool/child and all four
services active/running at `NRestarts=0`. Private transcript
`g6-packaging-rollback-live-pass.log` is mode 0600/126,393 bytes/SHA-256
`15a1d4d931f8047bafda0dd3f42a55e18a9601723ec3246ce0fc067a8641fb59`;
the wrapper observed exit 0 and credential scanning is empty.

The independent final auditor also exits 0. Its private transcript
`g6-packaging-final-resource-audit-pass.log` is mode 0600/12,601 bytes/SHA-256
`c5748c810df83094cc504207099b42c3ac16ae553cb21b05c16f91c5c0519d6b`.
It binds mkruntimed PID 6485 to selected candidate `11a65f08…` and exact daemon
SHA `0e1c87c3…`, proves no pool/child and all 19 categories zero, and records
mkruntimed, mknetd, containerd, and Docker active/running with zero restarts.
Its credential-pattern scan is empty. A narrow configuration/service audit is
still being collected before closing the composite packaging row.

The first narrow configuration audit is retained but not used for closure. It
passes live containerd/Docker named-runtime and `runc`-default checks plus all
versioned binary and initial support links, then calls `readlink -f` on the CNI
link without privilege. Root-private deployment ancestry makes that invocation
return empty. Privileged follow-up proves the link is valid, resolves into the
candidate deployment, and selects a mode-0644 asset with SHA-256 `7de30fd1…`;
selectors, active services, and empty task/container inventories are unchanged.
The non-passing private transcript
`g6-packaging-config-service-audit-first-permission-fail.log` is mode 0600/
21,903 bytes/SHA-256
`88f4c0607677f364f5e37408e7dffe7c318750cecf1ebf2554c6305ab87dccac`
with an empty credential scan. The corrected audit must resolve all support
links under sudo.

The corrected live configuration/service audit exits 0. Containerd's parsed
live dump retains default `runc` and named type `io.containerd.multikernel.v2`;
Docker's live inventory and `daemon.json` likewise retain default `runc` and
the opt-in named runtime. Four public executable/CNI paths resolve into exact
candidate release `11a65f08…`; the mkruntimed/mknetd units, containerd
fragment, and CNI config resolve into exact support generation `b4d185c6…`.
Systemd proves mkruntimed requires and follows the multikernel mount and GCE
guest agent, requires `/srv/multikernel-storage`, and precedes containerd and
Docker; mknetd follows network-online and also precedes both engines. All four
services remain active/running with zero restarts. Private transcript
`g6-packaging-config-service-audit-pass.log` is mode 0600/14,018 bytes/SHA-256
`72012fcb4901f8d08df8fcd34675f57f74e4cac54c63391e4515795e11ed2e2f`;
credential scanning is empty. Repository packaging tests and the full evidence
gate remain before row closure.

Packaging closure gates pass. Docker configuration has 6/6 tests; release
manifest, immutable binary lifecycle, immutable support-deployment lifecycle,
and containerd config parsing all pass. The full documentation/evidence gate
also passes local links, 7 schemas/22 cases, 17 current evidence manifests,
the 95-case OCI validator, bind/bootstrap/rootfs/storage/image fixtures, both
packaging managers, GCE ledger, evidence capture, containerd config, and final
G4-G6 evidence audit. The local socket fixture retains its explicit sandbox
`EPERM` skip; the privileged packaging evidence is unaffected. Taken together,
the immutable manager tests, fresh-host containerd transcript `20417eca…`, live
rollback/forward transcript `15a1d4d9…`, final resource audit `c5748c81…`, and
configuration/service audit `72012fcb…` substantiate every clause of the
composite packaging row, which is now closed. Checklist totals are 35 closed
and 50 open; overall G4-G6 completion remains unfinished.

The next closure target is G4 initramfs reproducibility. New fail-fast live
qualifier `scripts/test-runtime-initramfs-repro-live.sh` builds an ext4 input
twice across mtime and sparse-to-dense changes, independently verifies both,
requires byte-identical archive and manifest, audits normalization/mode/
hardlink/symlink metadata, and requires a changed-content control to diverge.
It then executes all 19 builder tests, including corrupt input, capacity,
no-replace publication, and replacement-preserving rollback. Local execution
passes with archive SHA `9b2a4f20…`, manifest SHA `cfca02b0…`, divergent
control hashes `7febef49…`/`a29b223d…`, and 19/19 tests; only the known local
sandbox socket subcase reports its explicit `EPERM` skip. Script SHA-256 is
`f1dabbbf…`; exact-commit disposable-VM execution remains mandatory.

The clean archive for qualifier commit
`7725e171da2a88deeb8be8d9f7d902084ad96667` is 4,730,880 bytes/SHA-256
`ecfbf93e…` and matches after VM transfer. Fresh ext4 extraction at
`/var/tmp/mksrc-7725e17` contains 640 regular files. The extracted qualifier,
builder, verifier, and 19-case suite hash to `f1dabbbf…`, `e1c7234d…`,
`2cfe4ad8…`, and `f4ecf345…`; live execution is next.

Exact commit `7725e17` VM qualification exits 0 with zero skips. Two ext4
builds across different mtimes and sparse-versus-dense allocation independently
verify and remain byte-identical at archive SHA-256 `2532e1b5…` and manifest
SHA-256 `bf6f3e04…`; the audited manifest retains normalized newc mtime/inode/
sparse/xattr/device policy plus exact modes, hardlink group, and symlink target.
A content change diverges at archive/manifest SHA `5850d990…`/`fd059b19…`.
All 19 focused tests pass as root, covering escaping paths, mutation, held
roots, FIFO/socket/device/xattr rejection, overlay opacity, uid/gid/modes,
hardlinks/symlinks, capacity/high-water refusal, verifier corruption, exclusive
publication, peer rollback, and replacement preservation. Private transcript
`g4-initramfs-repro-live-pass.log` is mode 0600/9,148 bytes/SHA-256
`5a1d5ae6765e6bbbfc16218b5c020cdb7ccdf98c9de92490a5d8c00b6165dbf3`;
credential scanning is empty.

Independent post-run audit `g4-initramfs-final-resource-audit-pass.log` exits 0
and is mode 0600/12,601 bytes/SHA-256
`c5748c810df83094cc504207099b42c3ac16ae553cb21b05c16f91c5c0519d6b`.
It binds the unchanged candidate PID/hash, reports no pool/child and all 19
resource categories zero, and retains four active/running zero-restart services.
Its credential scan is empty. This closes the initramfs reproducibility row;
the full documentation/evidence gate remains to be rerun.

The post-closure repository gate passes: documentation/local links, 7 schemas/
22 cases, 17 current manifests, 95 OCI cases, bind/bootstrap/rootfs/storage/
mount/root/image fixtures, release and both lifecycle managers, GCE ledger,
evidence capture, containerd config, and final G4-G6 audit all pass. The local
rootfs socket subcase retains the documented sandbox `EPERM` skip; exact-commit
VM execution above has zero skips. Checklist totals are now 36 closed and 49
open. Generated bytecode is removed from the worktree.

The adjacent G4 unsafe-input row is now under a separate breadth audit rather
than inferred from the reproducibility run. New verbose driver
`scripts/test-runtime-g4-input-boundaries-live.sh` composes exact root-path,
rootfs archive, ext4 storage, and bind-materialization suites. Its local run
passes 7 canonical/traversal/symlink/held-root cases, 19 file-type/metadata/
hardlink/mutation/publication cases, 10 capacity/copy/collision/held-source
storage cases, and the bind copy/metadata/symlink/mutation/held-source/target
matrix. The known sandbox socket subcase prints `EPERM`; the committed driver
must be rerun as root on the VM. Script SHA-256 is `edd56360…`.

Clean driver commit `e81d8ca2d0a31a1df2cbe8f438021a991006f1c0`
archives to 4,730,880 bytes/SHA-256 `35b2d4a4…`, matching after transfer.
Fresh ext4 extraction contains 641 regular files and the driver retains exact
SHA-256 `edd56360…`. Root VM execution is next.

Exact commit `e81d8ca` root execution on VM ext4 exits 0 with zero skips.
Verbose output names all 36 unittest cases: 7 root canonicalization, relative
traversal, symlink-component, allowlist, type, and held-anchor cases; 19 archive
escaping-symlink, FIFO/socket/device/whiteout/xattr/malformed-opacity,
external-hardlink, metadata, mutation, inherited/held-root, corruption,
capacity, and publication cases; and 10 ext4 exhaustion, metadata failure,
collision/replacement, held-directory, and source-copy cases. The separate bind
matrix passes copy/type/mode/hardlink/symlink, source and target descriptor
races, mutation detection, limits, and private writable seed isolation.
Private transcript `g4-input-boundaries-live-pass.log` is mode 0600/7,091
bytes/SHA-256
`81b7078a2b3e084a481312f3a183a0983efbf316b2bb8eb90c0b5fc49cea298f`;
credential scanning is empty.

Independent post-run transcript
`g4-input-boundaries-final-resource-audit-pass.log` exits 0 and is mode 0600/
12,601 bytes/SHA-256
`c5748c810df83094cc504207099b42c3ac16ae553cb21b05c16f91c5c0519d6b`.
It retains exact candidate PID/hash, no pool/child, all 19 resource counters at
zero, and four active/running services with zero restarts; credential scanning
is empty. This closes the composite unsafe-path/type/metadata/mutation row.

The post-closure full repository gate passes again with the same schema,
manifest, 95-case OCI, G4 fixture, packaging, GCE-ledger, capture, containerd,
and final-evidence coverage. The documented local socket `EPERM` is covered by
the zero-skip VM transcript. Checklist totals are 37 closed and 48 open;
generated test bytecode is removed.

Architecture-before-allocation is the next G4 target. New live qualifier
`scripts/test-runtime-image-architecture-live.sh` strictly validates the
actual selected kernel manifest, records its digest/release and counts of
required config/OCI features, binds a real x86-64 BusyBox hash to those exact
values, requires an AArch64 executable control to reject, and runs held-root/
held-child fixture coverage. Syntax/shellcheck and the local image suite pass;
script SHA-256 is `6ff46cb5…`. Actual-manifest VM execution is still open.

Clean qualifier commit `f83ff2178738751e87d12cd5e19c4eeec42e8c4a`
archives to 4,730,880 bytes/SHA-256 `9cfeb51d…`, matching on the VM. Fresh
ext4 extraction contains 642 regular files and exact qualifier SHA-256
`6ff46cb5…`; privileged actual-manifest execution is next.

Exact commit `f83ff21` privileged VM qualification exits 0 with zero skips.
Strict bootstrap validation proves the real selected manifest SHA-256
`1d79c564…`, amd64 architecture, kernel release `7.0.0-mk2-gce-lab`, four
required configs (`CONFIG_MULTIKERNEL`, VSOCKETS, EXT4, and modular NBD), and
18 OCI features while also validating root ownership, artifact hashes, pinned
compatibility, x86-64 binaries, and transport-module vermagic. Real BusyBox
SHA `8d4e5a13…` binds to those exact manifest values. An executable AArch64
control rejects with status 1 and the expected architecture diagnostic; the
ELF/interpreter/escape/held-root/held-child fixture suite passes. Private
`g4-image-architecture-live-pass.log` is mode 0600/5,558 bytes/SHA-256
`7fa430c243ab4d764d9b9866fc1e7e5b0fcea899ac48643886417077c89cc3e2`;
credential scanning is empty.

Independent `g4-image-architecture-final-resource-audit-pass.log` exits 0 and
is mode 0600/12,601 bytes/SHA-256
`c5748c810df83094cc504207099b42c3ac16ae553cb21b05c16f91c5c0519d6b`.
It proves the rejection created no pool/child or any of 19 residual resource
types and left the exact candidate plus four zero-restart services unchanged.
Credential scanning is empty. This closes the architecture/kernel-feature
binding row.

The post-closure full repository gate passes again across links, schemas,
17 current manifests, 95 OCI cases, G4 fixture suites, packaging managers,
GCE ledger, capture, containerd config, and final evidence audit. The local
socket `EPERM` is covered by the zero-skip VM evidence. Checklist totals are
38 closed and 47 open; generated bytecode is removed.

Before proceeding to caller-snapshot qualification, GCE reports the retained
instance `RUNNING` with start timestamp `2026-10-03T06:53:49.759-07:00` and
boot ID `304feec7…`. Docker, containerd, mkruntimed, and mknetd are active with
zero recorded restarts. The first readiness command used the nonexistent
`/run/mkruntime/` socket directory and therefore stopped before its final
marker; that probe-path mistake is retained as a harness failure and supplies
no product claim. Privileged enumeration instead proves mode-0660 root-owned
`/run/mkruntimed.sock` and `/run/mknetd.sock`, held by the expected daemons.
The caller-snapshot row remains open pending focused fault tests and live
before/after source identity and digest evidence.

Read-only runtime inventory identifies the exact live subject before any test
task is created. The `default` and `moby` namespaces both cache BusyBox 1.36 at
OCI index digest `sha256:73aaf090…`; each snapshotter inventory contains one
parentless committed snapshot, `sha256:97e4ece8…`, and Docker reports the same
index. This is candidate identity evidence only. The selected linux/amd64
manifest, snapshot mount projection, and before/after metadata/Merkle equality
are still required.

Direct `ctr snapshots mounts` inspection of the committed key rejects with
`failed precondition` because the command exposes only active or view mounts;
no snapshot or mount was created. The qualifier will therefore create and
clean a temporary read-only image view for mount-table and source-manifest
capture, rather than treating the committed key as directly mountable.

The first temporary view proves the committed filesystem is exposed as an
ext4 read-only mount and records root inode 6035824, BusyBox inode 6035832, and
a normalized 442-entry manifest digest `65714ed0…`. Cleanup inspection then
found that `ctr images unmount` leaves its view snapshot metadata behind. The
exact test-owned view was explicitly removed and a second inventory proves
only committed snapshot `97e4ece8…` remains with no probe mount. The final
harness must trap unmount and snapshot-view removal separately.

The new caller-snapshot qualifier combines OCI index/amd64 manifest/config/
diff-ID binding, temporary read-only view manifests, descriptor and unmount
fault tests, both root-path forms, guest mutation, retained build-result
inspection, and exact cleanup. Syntax and diff checks pass; this workstation
lacks `shellcheck`. A first local Go command ran no tests because its default
cache was read-only; an isolated `/tmp` cache then passes all 11 focused rootfs
cases plus shim mount sanitization under the race detector. A live read-only
`ctr snapshots info` query additionally showed capitalized `Kind` and `Name`
and no `Parent` key, which the harness now validates correctly. VM execution
has not started and the row remains open.

The qualifier-only commit is
`49bb985182eb4a82164278f0beb29eb706c1dc3a`. Its 4,751,360-byte exact Git
archive hashes to `32d61a2b…`, while the committed driver hashes to
`ab52789b…`. Running evidence notes and retained evidence were intentionally
not included in that source commit. Upload and live execution remain pending.

The VM copy matches archive SHA-256 `32d61a2b…`; its fresh extraction at
`/var/tmp/mksrc-49bb985` contains 643 regular files. The mode-0775 ordinary-
user-owned qualifier matches `ab52789b…` and passes VM `bash -n`.
`shellcheck` is absent there too. No live workload has run yet.

The first live invocation is retained as non-passing. Its explicit preflight
cleanup used `set +e` in the parent shell, disabling fail-fast; containerd
2.2.2 also lacks `ctr snapshots ls -q`. The run was interrupted as soon as the
transcript exposed that combination. Although OCI digest binding, focused VM
race tests, and the first read-only manifest all succeeded, no closure claim is
taken from a fail-open driver. The interruption left only named test object
`mk-snapshot-relative` and its active snapshot, with no child; targeted removal
and idle daemon restart restored the sole committed snapshot, empty task,
container, rootfs-record, and child inventories, and four active zero-restart
services. Private `g4-caller-snapshot-live-first-harness-fail.log` is mode 0600,
50,894 bytes, SHA-256 `87275953…`. Cleanup is now a subshell and snapshot-list
capture uses supported table output.

The isolated correction is commit
`bcd7ad7025417c7a8da0864e0484a3f6c9bc1146`, with driver SHA-256
`3249db88…`. Its 4,751,360-byte exact archive hashes to `a0dedf40…`; the rerun
will use a fresh extraction rather than modifying the first VM source tree.

The corrected VM archive and driver match those hashes in a fresh 643-file
`/var/tmp/mksrc-bcd7ad7` extraction and pass `bash -n`. Rerun remains next.

The second run fails closed only because the harness equated Docker's active
caller root with the pristine committed base. The relative ctr path completes:
its build scans equal the independently captured 442-entry `65714ed0…`
manifest, while private guest mutation changes `/etc/passwd` and creates the
new root marker. Docker proves the absolute path and equal build before/after
digest `e9573a41…` across 452 entries; its ten additional entries exist before
runtime handoff. Trap cleanup returns both namespaces to their sole committed
snapshot and zero task/container/rootfs/network/child state with all services
healthy. Private second transcript is mode 0600/71,325 bytes/SHA-256
`868bc5b8…`. The harness now independently rescans Docker's exact live caller
root after guest mutation and compares that result to Docker's own retained
before/after build scans.

That correction is exact commit
`fe9a2deefffad48ae5aa17c91b3ff6330a7e12e9`, driver SHA-256 `51f98998…`;
its 4,751,360-byte archive hashes to `e390030a…`. A fresh VM tree and rerun are
still required.

The archive and driver match in fresh 643-file `/var/tmp/mksrc-fe9a2de`, and
VM `bash -n` passes. The third run is next.

Exact `fe9a2de` qualification exits 0. It binds the OCI index to selected
amd64 manifest/config/layer and committed diff-ID snapshot, passes the 11
focused rootfs failure/recovery cases plus shim sanitization under `-race`, and
observes both root forms. Relative ctr build scans equal the independently
captured 442-entry `65714ed0…` manifest. Absolute Docker scans are equal at
452-entry `e9573a41…`, and a new independent scan of the exact still-live
caller overlay after guest mutation matches. Both guests demonstrably mutate
their private `/etc/passwd` and create a new root marker. The committed source
view remains byte-identical before/after at `65714ed0…`, its sole snapshot key
stays `97e4ece8…`, and all 19 runtime-resource counters finish zero with four
healthy zero-restart services. Remote private transcript is mode 0600/136,343
bytes/SHA-256 `33e1d92e…`; local retention and an independent audit remain.

The local retained pass transcript matches mode 0600/136,343 bytes and SHA-256
`33e1d92e…`; credential-pattern scanning is empty. A first independent-audit
launch ran no audit because the restarted host no longer had the old `/tmp`
script. Its wrapper also omitted `set -e`, continued past the failed existence
test, and exited 127. No audit claim is taken; the script will be recreated and
hash-verified before a fail-closed run.

The independent audit then runs from exact `fe9a2de` source after its
`bc5e2f7d…` script hash is verified. It exits 0 after the operator restart on
new boot `c5537cb9…`, binds live mkruntimed PID 1522 to candidate SHA
`0e1c87c3…`, finds no Kerf pool or child and all 19 resource counters zero,
and records four active/running zero-restart services. Private retained
`g4-caller-snapshot-final-resource-audit-pass.log` is mode 0600/15,227 bytes/
SHA-256 `e169a3da…`; its credential scan is empty. Repository gates remain
before row closure.

The post-closure full repository gate passes documentation/links, all schema
and current evidence-manifest checks, the 95-case OCI boundary, G4 fixture
suites, release/deployment managers, GCE ledger, capture, containerd config,
and final evidence audit. The known local socket `EPERM` is covered by the
zero-skip VM run. Both the caller-snapshot implementation row and exact
replacement-instance snapshot-evidence row are now closed; checklist totals
are 40 closed and 45 open. Generated Python bytecode was removed.

The post-qualification VM remains on boot `d9cdfa98…`; mkruntimed PID 9410,
containerd 1480, Docker 1519, and mknetd 1227 are active/running with zero
service restarts. ctr and Docker have zero running tasks. Exact procfs command-
name enumeration finds zero multikernel shim, `nbdkit`, or `mk-agent-relay`
processes. A preceding `pgrep -f` value was discarded because the audit command
matched itself and is not used as evidence.

The next open-row audit confirms that writable-root ownership is explicit,
durable, and generation-bound rather than an accidental consequence of one
private initramfs. `rootfs.Store` refuses concurrent records claiming the same
bundle or storage port. `storage.Service` binds state to sandbox ID/generation,
mints a distinct export generation, rejects stale release and conflicting live
generations, and retains ambiguous `PREPARING` and `QUIESCING` ownership for
exact recovery. The Linux backend validates the exact image inode beneath a
held private parent, passes the exclusively locked open file description as fd
3 to `mkvsock-nbd`, and holds a separate exclusive lock for the complete
offline check. Existing race tests include direct contention and prove the
lock becomes available only after exact server/checker exit. This inventory is
not yet disposable-host closure: an exact-source live run must record durable
generations, actual image-lock contention and later release, stale/conflicting
generation rejection, cleanup, and an independent post-run resource audit.

A read-only probe on restarted boot `c5537cb9…` confirms that deployed storage
state is version 2 with top-level `exports`, not `records`. It retains 183
historical exports while idle, whereas ephemeral `/run/mkstorage` is absent.
The compound probe exits 1 only because its final `find` receives that expected
absent directory. This exposes an older evidence helper's ineffective
`records` lookup before reuse. The replacement qualifier must select exactly
one non-`RELEASED` object from `exports` during the workload and treat retained
released history separately from live ownership and resource cleanup.

New `scripts/test-runtime-single-owner-live.sh` composes durable rootfs,
storage-export, and process-record identity; the deployed server's fd-3 inode;
a real competing exclusive-lock attempt; later lock acquisition through the
same descriptor after the pathname has been unlinked; exact release generation
and offline-check evidence; and cleanup. Its focused local race run passes
duplicate bundle/port/path/UUID/owner, state transition, stale/conflicting
generation, ambiguous recovery, server/offline-check contention, stale
artifact, and absent-runtime-directory cases. Shell syntax and diff checks
pass; `shellcheck` is unavailable locally. Review found one harness-only issue
before execution: durable export map keys contain an embedded NUL and cannot be
carried by a shell variable. The driver now carries sandbox ID and generation
separately and reconstructs the exact key inside Python. There is no live claim
from this still-unexecuted script.

The qualifier is isolated as exact commit
`287a817f73054c8eff0efce980c83e5a6bd28e62`; its executable script hashes to
`80969159…`, and its 4,761,600-byte Git archive hashes to `ce8dec4e…`. The
running documentation and existing evidence trees are intentionally outside
that commit. Fresh VM transfer, hash verification, and execution remain.

Exact-source transfer succeeds on restarted boot `c5537cb9…`: remote archive
and driver hashes match, the fresh tree has 644 files, and VM shell syntax
passes. The first run fails closed before creating any task. Rootfs/store/
service generation tests pass, but three real-ext4 backend fixtures reject
their images as sparse because Go's default `t.TempDir()` selects VM `/tmp`, a
tmpfs. Production storage is ext4 on `/dev/sdb`; `/var/tmp` is root ext4. Trap
cleanup therefore has no workload to remove. Private retained failure log
`g4-single-owner-live-first-tempfs-sparse-fail.log` is mode 0600/19,236 bytes,
SHA-256 `56c325e5…`, with an empty credential scan. A first follow-up final-audit
invocation was incorrectly prefixed with `sudo`, so the explicitly ordinary-
user-only audit exited 1 and contributes no audit claim; the preceding
filesystem mount probes are valid. The qualifier now gives Go both an ext4-
backed `/var/tmp` `TMPDIR` and isolated cache before rerun.

The corrected ordinary-user audit then exits 0: the boot and candidate daemon
identity are unchanged, Kerf has no pool or child, all 19 resource counters
are zero, and the four services are active/running with no restarts. The
two-line ext4-fixture correction is isolated in commit
`a39896e3dd10e1fe3e567335921ebef8c6596a4f`; driver SHA-256 is `3e121076…`,
and the 4,761,600-byte archive SHA-256 is `127c7011…`. Transfer and fresh rerun
remain.

The freshly verified `a39896e` rerun passes all focused race tests and reaches
one durable rootfs owner plus one active export, then fails closed in evidence
parsing. `/proc/<pid>/comm` is not guaranteed to say `mkvsock-nbd` because the
daemon execs the pinned binary through `/proc/self/fd/4`; that display name is
not part of process authentication. Private retained transcript is mode 0600/
24,637 bytes/SHA-256 `45c4099a…`, with an empty credential scan. Trap cleanup
removes the task and all live durable ownership, but the first independent
audit correctly finds the idle daemon's Kerf pool still configured. After
confirming zero live exports and rootfs records, an idle mkruntimed restart
releases the pool; the repeated audit passes all 19 zero counters and four
healthy zero-restart services (SHA-256 `dcbbe91f…`). The evidence parser now
asserts the real authentication invariant: `/proc/<pid>/exe` device/inode must
equal the durable binary identity, alongside fd 3's exact image identity.

The executable-identity correction is isolated as exact commit
`69e9ea947a22ba3cfc0c72950d4d4631671bf71a`; driver SHA-256 is `5f1f6fce…`
and its 4,761,600-byte archive SHA-256 is `e77ae19e…`. Fresh transfer and rerun
remain.

The current capacity/ENOSPC audit records the boundary before further work.
The storage builder has two host free-byte high-water checks (before staging
and after the private copy), a fully allocated `posix_fallocate` image, equal
declared byte quota, a declared and superblock-verified inode limit, and focused
tests for pre-allocation refusal plus natural ext4 block/inode exhaustion with
no output, metadata, or staging residue. The initramfs builder separately
limits payload bytes and entries and checks output-filesystem high water. These
tests do not yet inject host `ENOSPC` at staging copy, allocation/fsync, ext4
tool, image publish, metadata write/fsync/publish, or initramfs archive/manifest
boundaries, and the host side has no free-inode reserve policy. Accordingly no
new row is closed: boundary-complete failure tests and exact-source live
qualification remain required.

The first implementation checkpoint adds free-inode high-water reserves to
both builders and production wrapper defaults. Storage accounts for the
admitted clone and staging/output objects before copy and rechecks remaining
output needs after staging; initramfs accounts for its archive/manifest pair or
single manifest-only output. Refusal tests leave no artifacts. This review also
found that `runtime_safe_publish.atomic_write` learned its temp inode only after
write, flush, and fsync, so an earlier ENOSPC could strand the temp. It now
captures identity immediately after `mkstemp`, revalidates it before publish,
and removes only that inode on every failure. Injected write, fsync, and link-
publication ENOSPC tests leave neither public nor private attempt artifacts;
the 10-case storage and 19-case initramfs suites remain green apart from the
known local socket-permission skip. Builder-specific boundary injection and
live qualification are still pending.

The builder-specific injected matrix now passes with 11 storage and 20
initramfs cases. Storage exercises staging allocation, copy, image allocation,
image fsync, `mke2fs`, `e2fsck`, image publication, and metadata creation
ENOSPC, with no public outputs, staging tree, or private temp after each case;
the existing debugfs failure covers its metadata-normalization step. Initramfs
metadata ENOSPC after archive publication removes that exact archive, while
the shared-publisher cases cover write, fsync, and publication failure for
either output. This is deterministic boundary coverage, not yet real host
exhaustion or live-runtime evidence. Both rows remain open until a constrained
filesystem replay and independent resource audit pass.

An unexecuted live qualifier now mounts real byte- and inode-constrained tmpfs
filesystems. Normal calls must prove both high-water policies leave an empty
mount. A separate helper overrides only the admission reading, after which the
unchanged builders must receive real kernel ENOSPC during storage
`posix_fallocate`, storage copy inode allocation, initramfs archive writing,
and initramfs manifest inode allocation; success additionally requires that no
public or private attempt artifact survives. Full 19-counter audits bracket
the run. The driver hashes to `368ad6d5…`, the helper to `e256d6d6…`, and
local syntax/compile plus the 1/11/20 focused suites pass. This is candidate
design provenance only; no VM result is claimed.

The full repository gate passes with 20 initramfs, 11 storage, and one direct
publisher case, along with all schema, OCI, deployment, and evidence gates.
Generated bytecode was removed and the diff check is clean. A first attempt to
freeze only source/tests/Plan 04 made no change because `.git` is read-only in
the current sandbox and Git could not create `index.lock`; the running ledgers
and existing evidence trees were not staged.

The identical scoped file list is now frozen as commit `97bcae5`; neither
running ledger nor evidence tree entered the commit. The exact Git archive
hash is `2b3c836f…`, and the driver/helper/publisher-test hashes remain
`368ad6d5…`, `e256d6d6…`, and `21044321…`. Upload and guest verification are
not yet claimed.

The VM independently matches the 4,792,320-byte `2b3c836f…` archive and all
three script hashes in a fresh 648-file extraction. Syntax/compile checks pass
on unchanged boot `c5537cb9…`, and all four services are active. This is
transfer provenance, not live capacity evidence.

Exact `97bcae5` execution then passes. The focused 1/11/20 suites are green;
normal calls prove byte/inode high-water refusal, and real constrained mounts
deliver kernel ENOSPC during storage image allocation, storage staging-copy
inode allocation, initramfs archive writing, and its second output after 30
filler inodes. Every case leaves no attempt artifact. Bracketing full audits
show all 19 counters zero, no Kerf state, candidate `0e1c87c3…`, unchanged
boot `c5537cb9…`, and four healthy zero-restart services. Remote transcript is
mode 0600/82,525 bytes/SHA-256 `f216d797…`, exits 0, and has an empty
credential scan. Local retention remains before any row closes.

The locally retained transcript exactly matches mode 0600, 82,525 bytes and
SHA-256 `f216d797…`; its second credential scan is empty and every named
high-water, real-ENOSPC, audit, suite, qualifier, and exit marker is present.
The focused capacity/accounting checklist row is therefore closed. The broader
fault matrix stays open for interrupted copy and remaining non-capacity
corruption/generation combinations.

The post-closure full repository gate passes, the diff check is clean, and
four generated bytecode files are removed. Checklist totals are now 44 closed
and 41 open.

The next composite-row audit confirms that prior single-owner evidence already
covers stale/conflicting generations, duplicate claims/attach, real lock
contention, and release; the capacity pass supplies block/inode exhaustion,
high water, and builder allocation failures. What is not yet proved is an
actual signal-interrupted staging copy (the current injected copy case is a
generic nonzero result) and VM selection of the existing ext4 wrong-UUID
backend tests. Those two gaps remain explicit before implementation.

The copy gap is now implemented without a production failpoint: the builder's
copy executable is selectable for testing but defaults to `/bin/cp`; a helper
writes one partial staged file and dies by `SIGKILL`. The parent observes
status `-9` and removes exact staging/output state. All 12 builder cases pass
and the live driver prints this test by name, then selects wrong-UUID,
conflicting-generation, and single-owner Go cases under `-race`. A first local
Go attempt ran no tests because the sandbox default cache is read-only; it will
be repeated with an isolated writable cache.

The isolated-cache retry passes all four selected storage tests under the race
detector: pristine/current wrong-UUID inspection, quota/clean-state identity,
generation-bound single ownership, and conflicting live-generation refusal.
VM execution and the full repository gate still remain.

The full gate now passes with 12 storage cases. Commit `0672fa1` isolates the
four changed source/test files; its 4,792,320-byte archive is `56140248…`, and
driver/helper/storage-test hashes are `6495ebc2…`, `c9b04fca…`, and
`8a92c62f…`. Findings and evidence remain outside the commit. Upload and VM
execution are pending.

The current documentation gate passes after the exact-current initramfs row
closure, with 47 checklist rows closed and 38 open; generated bytecode is
removed. A fresh source trace of the storage teardown path establishes that
the shim authenticates guest `Quiesce` before `Shutdown`, and that mediated
guest shutdown orders sync, read-only root remount, sync, `/dev/nbd0`
disconnect, and poweroff. Disconnect failure suppresses poweroff. On the
primary, release accepts counters only from the exact generation's canonical
terminal `MKNBD_SERVER_CLOSED synced=1` record and then performs the locked
offline check. Existing live evidence proves final `RELEASED` state, counters,
offline check, and clean inventory, but does not retain the guest disconnect
console marker or directly join it to the server close. The teardown row is
therefore deliberately still open pending an ordered live transcript.

The restarted disposable VM is `RUNNING` on the same `c5537cb9…` boot and
`0.1.0-dev-11a65f08…` selector. The first read-only service/Kerf probe is not
evidence: local double-quote expansion erased the remote service-loop variable,
so systemd rejected an empty unit name before health or Kerf output was
collected. This is recorded as a qualifier quoting failure and will be rerun
with literal remote quoting.

The corrected read-only probe finds all four services active with zero
restarts and no Kerf pool or instance. Kerf is configured at
`/opt/mkruntime/kerf-venv/bin/kerf` (not sudo's secure PATH), `/dev/mktty` is a
root-owned 0600 character device, and `kerf console NAME` can attach to the
running child. The guest now prints a bounded root-quiescence success marker
only after sync/remount-read-only/sync completes, complementing the existing
post-ioctl NBD disconnect marker. Ten race-enabled mk-agent repetitions pass.
The accompanying gofmt invocation had incorrectly doubled the `runtime/`
prefix from inside that directory and produced three `lstat` errors, so it is
classified as a local harness-path failure; formatting is not claimed until
the corrected command runs.

The corrected gofmt and another ten race-enabled package repetitions pass,
followed by the full documentation/runtime gate and clean diff check. The only
reported skip is the already classified sandbox-local socket `EPERM`; its live
VM execution is separately zero-skip. Generated bytecode is removed. The
three marker source/test files are ready for an isolated commit; ledgers and
evidence stay outside it.

Commit `e396511` freezes exactly those three mk-agent files. The exact
tracked-source archive is 4,792,320 bytes/SHA-256 `8dccf1fa…`, stamped from
full revision `e396511c18ad3faa78c3080de477ce11e265f1d0`; learning documents and
existing evidence are excluded. No guest build, activation, or live result is
yet claimed.

Later exact-build notes above retain the corrected manifest hashes. Binary
installation now selects immutable release
`0.1.0-dev-e396511c18ad3faa78c3080de477ce11e265f1d0`; empty-host restarts leave
all four services active with zero restarts, running mkruntimed matches
`29dc06dd…`, and Kerf remains empty. The focused qualifier (`4b9b84e5…`)
writes/syncs live mediated-root data, attaches the exact child console, emits
only fixed safe quiesce/disconnect observations, checks their line order,
authenticates the generation-specific terminal `synced=1` log and durable
offline check/counters, proves exact artifacts are removed, and returns the
idle pool by deliberate daemon restart. Raw console bytes never enter retained
stdout. Syntax, optional ShellCheck, and diff checks pass; execution remains
pending.

The first immutable run is retained mode 0600/105,658 bytes/SHA-256
`8524fff5…` with exit 1. Exact hash, clean pre-state, candidate provenance,
and an ACTIVE generation are proved, and normal deletion returns, but the
post-start Kerf console attachment sees neither fixed marker within its bound.
The trap removes the private raw console without printing it. This is a
console-capture evidence failure, not proof of a teardown failure; no row is
closed while initramfs agent provenance and attachment timing are diagnosed.

The bootstrap manifest directly explains the miss: it still selects the
`1f81cb2…` guest-agent artifact with SHA-256 `c79210c6…`, not new agent
`b57bae66…`. Host binary activation does not rewrite that independent
kernel/bootstrap artifact contract. A follow-up read-only command then passed
the complete `{path,sha256}` JSON object to `sha256sum` instead of
`.agent.path` and stopped before its remaining audit; that probe-shape error is
recorded separately. The manifest mismatch itself is conclusive, and a managed
artifact update plus corrected audit is required before replay.

The corrected audit matches the old agent's declared/observed `c79210c6…`
against candidate `b57bae66…`. The same failed-capture workload nevertheless
has an authenticated clean teardown: durable `RELEASED`, offline-check digest
`bf24cda1…`, 285 reads/10,452,992 bytes, 49 writes/696,320 bytes, nine flushes,
and an exact generation terminal `synced=1` line with the same counters. All
services are healthy, no child remains, and the 16-GiB pool is retained idle
by policy. This proves the runtime teardown occurred but not the new guest
marker; the row remains open.

The first manifest update fails closed before activation: strict bootstrap
validation refuses a candidate JSON whose parent is world-writable
`/var/tmp`. The new root-owned immutable directory already contains the agent
and rollback manifest, while the active `1d79c564…` manifest is unchanged.
The rejected temporary JSON will be removed exactly, and candidate validation
will be repeated from the trusted artifact directory.

Trusted-parent retry validates before and after atomic selection. The active
manifest is `d4230978…`, rollback is preserved as exact `1d79c564…`, and the
new immutable agent is a root-owned single-link 0755 regular file whose
declared and observed hashes both equal `b57bae66…`. The builder resolves this
manifest anew per task, so replay can proceed without a daemon restart.

The unchanged qualifier then passes with the exact candidate agent. Its safe
console observations prove quiescence at line 7 before NBD disconnect at line
9. The exact generation closes with `synced=1`, 281 reads/10,452,992 bytes, 49
writes/692,224 bytes, and nine flushes; durable `RELEASED` state repeats those
counters and records clean offline-check digest `bf24cda1…`. Exact record and
backing image disappear, the pool returns, and services remain healthy.
Retained transcript is mode 0600/30,921 bytes/SHA `8e7d93e0…`, exit 0, with an
empty credential scan. Independent mode-0600 audit is 14,362 bytes/SHA
`49f4e2e2…`, exit 0, empty credential scan, all 19 counters zero, no
pool/child, candidate daemon exact, and four active zero-restart services.
Together with the focused failure/recovery tests, this closes the storage
teardown/recovery row.

The post-closure full repository gate and diff check pass; generated bytecode
is removed. Checklist totals are now 48 closed and 37 open.

Audit of the next partial-artifact row deliberately does not close it. The
retained VM race suite proves all nine shim rollback stages plus exact
ambiguous-create cancellation/tombstoning and lifecycle cancellation
boundaries. The live preallocation suite proves an unsupported OCI rejection
before any pool/child/storage/network allocation and a final zero inventory.
What is still absent is the row's stronger requirement: live post-allocation
failure injection at every Create stage with immediate no-leak inventory.

The user's instance restart produces new authoritative boot `95e59482…`.
Runtime selector `e396511c…`, active manifest `d4230978…`, and candidate guest
agent `b57bae66…` persist exactly. All four services are active with zero
restarts, and Kerf reports neither pool nor child. Subsequent evidence uses
this clean post-restart baseline; earlier teardown evidence remains scoped to
its recorded `c5537cb9…` boot.

Kerf fault-adapter preflight is conflict-free: both fixed binaries and the
control file are absent. Active support generation is `b4d185c6…` and config
still points directly at pinned Kerf. A one-shot `load` rejection occurs after
real Create allocation/preparation, making it the appropriate live complement
to the nine-stage rollback unit matrix. No mutation or result is yet claimed.

The focused live qualifier now hashes to `4df7634a…`. It activates a temporary
managed support generation around the one-shot `load` fault, records immediate
20-field inventory after the real post-allocation failure, requires a new
durable clean `RELEASED` storage generation, restores/removes the temporary
generation and adapter paths, then runs the shim/rootfs/lifecycle failure
matrix under the race detector and re-audits cleanliness. The trap restores
the original generation on every exit. Syntax, optional ShellCheck, and diff
checks pass; execution remains unclaimed.

Commit `83b1164` freezes only the 206-line qualifier. Learning documents and
evidence remain outside the commit; transfer and live execution are pending.

First immutable execution exits 1 before mutation because the qualifier
incorrectly required executable mode on the intentionally 0644 Python
deployment manager. The trap confirms no support/adapter staging and removes
only empty scratch paths. Retained failure is mode 0600/5,103 bytes/SHA
`6a725b23…`; it proves no rollback behavior. Preflight now requires a regular
file, consistent with explicit `python3` invocation.

The corrected qualifier passes Bash syntax, optional ShellCheck, and diff
checks and hashes to
`5131c04c97fc9847880f9f384f6bd80f9ddd8d6aa27870a13ddfcef438ff32a2`.
This identity is recorded before transfer. Isolated corrective commit
`3ce774e` changes only the preflight predicate. The uploaded commit-specific
path rehashes exactly to `5131c04c…` on restarted boot
`95e59482-e5c4-4b4c-a520-b96b1943b088`; the source root is present and all
four runtime services are active. No live result is yet claimed.

The second immutable attempt is a caller error, not a runtime result. It is
retained as `g4-partial-artifact-live-second-root-invocation-fail.log`, mode
0600/4,907 bytes/SHA `e2db439c…`, exit 1. Invoking through `sudo` trips the
qualifier's ordinary-user guard before mutation; the trap shows no staged
deployment or adapter and cleans only empty scratch paths. The unchanged
committed script must instead be invoked directly by the sudo-capable SSH
user.

Direct invocation produces a third pre-mutation staging failure, retained as
`g4-partial-artifact-live-third-source-access-fail.log`, mode 0600/4,801
bytes/SHA `a247ed1c…`, exit 1. The qualifier passes its UID guard but cannot
traverse root-owned mode-0700 `/var/tmp/mksrc-e396511-build`; its `runtime`
child and deployment manager are mode 0775. The trap again has
`restore_required=0`. This is not rollback evidence; the exact disposable
source extraction needs read/traverse staging for the ordinary caller.

The full source-mode audit finds only the extraction root nontraversable and
no non-world-readable source file. Changing only that disposable directory
from 0700 to 0755 makes the manager/runtime paths accessible; the uploaded
qualifier remains byte-identical at `5131c04c…` before retry.

The fourth immutable attempt reaches temporary managed deployment staging but
not the injected Kerf call. Retained evidence
`g4-partial-artifact-live-fourth-socket-readiness-fail.log` is mode
0600/30,100 bytes/SHA `e9907b68…`, exit 1. It proves clean 20-field starting
inventory and temporary generation `0e1cbc45…`; however, the active-only
restart wait races `/run/mkruntimed.sock`, so `ctr` fails to connect and the
fault control remains unconsumed. The trap restores `b4d185c6…`, direct Kerf,
and removes every test path/deployment. Post-failure inspection finds the
socket ready, all four services active, and zero `mkruntimed` restarts. The
qualifier needs active-plus-socket readiness at every managed restart.

A bounded active-plus-socket helper is now used for fault activation, normal
restoration, and trap restoration. Bash syntax, optional ShellCheck, and diff
checks pass; the corrected qualifier hashes to
`a64faea1db586fcc31a369abd5f9e31dfa95cc95f25c1101f73624fb38666e56`.
Isolated commit `cf5d0d8` contains only this readiness correction. Transfer and
retry remain pending. The commit-specific upload rehashes exactly to
`a64faea1…`; the pre-retry check reconfirms original deployment `b4d185c6…`,
direct Kerf, a ready socket, and no fixed test path.

The fifth immutable attempt finally exercises the intended post-allocation
fault and exposes a real implementation gap. Evidence
`g4-partial-artifact-live-fifth-postallocation-leak-fail.log` is mode
0600/50,706 bytes/SHA `bba1b94c…`, exit 1. The one-shot `load` fault is consumed
and returns `BACKEND_FAILURE`; immediate inventory has zero child, mounts,
artifacts, FIFOs, network, containers, rootfs records, live exports, NBD, and
relay processes. Two disconnecting shim processes are still transiently
present, and the empty Kerf pool remains configured. Thus the row remains
open. Trap restoration returns to `b4d185c6…`, removes test state and shims,
and original-daemon restart releases the pool; services are active and storage
history advances 190 to 191.

The cause is now localized: post-`CreateSandbox` failure rollback invokes
ordinary `DeleteSandbox`, whose documented policy retains an idle pool.
`CancelCreateSandbox` already owns the original Create journal, accepts both
`ALLOCATING` and `CREATED`, removes storage/backend state, aborts the Create,
and releases a zero-owner first pool. The shim must use that authenticated
cancellation path for later Create-stage failures. The qualifier should keep
the immediate artifact snapshot but give containerd's already-disconnecting
shim supervisor/worker a bounded convergence interval before asserting final
process cleanliness.

The implementation now threads the exact config/Create idempotency key into
deferred rollback and calls `CancelCreateSandbox` for every lifecycle-attempted
failure. All nine post-validation stages now expect no ordinary delete;
successful authenticated cancellation is what permits rootfs cleanup and
clears shim sandbox state. The live qualifier retains its immediate snapshot
and waits at most 30 seconds for containerd's disconnecting shim processes
before strict final inventory. Gofmt, Bash syntax, optional ShellCheck, and
diff checks pass. Focused race tests covering the nine-stage matrix, ambiguous
cancellation, and four lifecycle pool/cancellation cases pass locally using
task-specific `/tmp` caches (the default cache is sandbox-read-only). Updated
qualifier SHA is `7ca683d2…`.

The complete race-enabled shim and lifecycle package suites pass locally in
`9.559s` and `1.851s`. Commit plus disposable-host rebuild/execution remain
pending, so this does not yet close the row.

Commit `a32e4dd` freezes only the runtime rollback, matrix expectation, and
qualifier changes (21 insertions/12 deletions across three files). Its exact
tracked-source tar is 4,812,800 bytes/SHA-256 `d4d7e48c…`, for full revision
`a32e4dde49077d4a0b0ee6cdecf8ae362e7741f6`. Ledgers and evidence remain
outside that commit; guest transfer/build remain unclaimed.

The guest independently verifies the uploaded archive at exact SHA
`d4d7e48c…` and 4,812,800 bytes, confirms unique build path
`/var/tmp/mksrc-a32e4dd-build` is absent, and reports no configured Kerf pool.
Extraction/build remain pending.

Immutable build evidence `g4-partial-artifact-build-a32e4dd.log` is mode
0600/5,429 bytes/SHA `6d5981e9…`, exit 0, with empty credential scan. The guest
rechecks the archive and builds a complete static release stamped with full
revision `a32e4dde…`; manifest SHA is `aed3b718…`, and exact
mkruntimed/shim/mknetd/agent hashes are `5dde0c70…`, `2ff97961…`,
`d99acb5e…`, and `87b33ee3…`. Install/execution remain unclaimed.

The first activation capture does install/select release
`0.1.0-dev-a32e4dde…`, restart both runtime daemons, and observe all four
services active with zero restarts, but exits 1 on an immediate socket check
before identity collection. Retained
`g4-partial-artifact-activate-a32e4dd-socket-race-fail.log` is mode
0600/2,188 bytes/SHA `5ac4512f…`. This repeats the verifier readiness race;
selection/restart are mutations, while coherent running identity awaits a
bounded replacement capture.

The bounded replacement proves socket readiness, four active/zero-restart
services, and candidate selector/shim link, then exits 127 solely because it
assumes nonexistent `/usr/local/bin/mkruntimed`. Retained evidence is mode
0600/2,328 bytes/SHA `3ffe88ac…`. Systemd actually executes
`/usr/local/sbin/mkruntimed`; PID 9967 resolves into the candidate immutable
release, and its `/proc` executable reports exact `a32e4dd…`. Exact hashes and
Kerf state still need the corrected capture.

Corrected activation evidence
`g4-partial-artifact-activate-a32e4dd-pass.log` is mode 0600/3,956 bytes/SHA
`4efa82b1…`, exit 0, with empty credential scan. It proves a ready socket, all
four services active with zero restarts, candidate selector/shim link, exact
running/public mkruntimed SHA `5dde0c70…`, exact shim SHA `2ff97961…`, full
revision reports, and no Kerf pool or instance. Live replay remains pending.

Only the new root-owned extraction directory is changed from mode 0700 to
0755 for ordinary-user traversal. The embedded qualifier rehashes exactly to
`7ca683d2…`, required manager/wrapper access passes, and every fixed test path
is absent immediately before replay.

The repaired live qualification passes. Evidence
`g4-partial-artifact-live-pass.log` is mode 0600/94,007 bytes/SHA
`4e66461f…`, exit 0, credential-clean, and ends in
`G4_PARTIAL_ARTIFACT_LIVE_PASS`. Provenance binds boot `95e59482…`, candidate
revision/hash `a32e4dd…`/`5dde0c70…`, original support generation
`b4d185c6…`, and qualifier `7ca683d2…`. The consumed `load` fault returns the
intended `BACKEND_FAILURE`; immediate state has zero child, mounts, artifacts,
FIFOs, network, containers, rootfs/storage/NBD/relay residue, while only the
disconnecting shim supervisor/worker remain. Twenty quarter-second samples
then reach zero shim processes. Strict inventory proves all 20 counters zero
and no pool/instance. Storage history advances 191→192 with a new clean
`RELEASED` record and offline-check SHA `5648b3d2…`. Nine shim, six rootfs,
and four lifecycle race cases pass on the VM, followed by another all-zero/
no-pool checkpoint and four healthy zero-restart services. The row awaits an
independent resource audit before closure.

Independent script SHA `bc5e2f7d…` passes in
`g4-partial-artifact-final-resource-audit-pass.log`, mode 0600/14,185
bytes/SHA `545ad9ea…`, exit 0, with empty credential scan. It independently
binds boot `95e59482…`, candidate selector `a32e4dd…`, and running daemon SHA
`5dde0c70…`; proves no Kerf pool/instance and all 19 resource counters zero;
and reports all four services active/running with zero restarts. The live
injected post-allocation pass, durable clean storage record, focused failure
matrices, and independent clean audit jointly close the partial-artifact row.

The post-closure full documentation/runtime gate and repository diff check
pass. Four generated bytecode files are removed. Checklist totals are now 49
closed and 36 open.

Audit has started on the next storage server-loss/recovery row. Existing live
material proves exact-helper survival/adoption across mkruntimed restart,
nonzero read/write/flush counters, authenticated already-exited-helper recovery,
offline checking, and final cleanup. Focused tests cover dirty state, wrong
UUID, pristine digest mutation, inode replacement, forged close evidence,
unsafe QUIESCING recovery, exact ACTIVE restart, and generation conflict.
What remains is materially different: death during an outstanding read/write/
flush, primary-host reset behavior, and clean/dirty/corrupt recovery using
disposable copies. V1 makes private writable roots non-persistent, so reset
qualification must prove fail-closed ownership/reconciliation and cleanup—not
claim workload-data durability. The row remains open pending a focused live
qualifier and a documented accepted-client server-loss policy.

Source audit exposed an unsafe recovery claim: every absent `ACTIVE` helper was
restarted even when its exact log showed the child had already connected, but
that replacement cannot restore the lost NBD session. The backend now emits a
distinct `ClientLost` observation only for generation-bound READY→CLIENT_ACCEPTED
without canonical synced close, retains the exact dead-process record, and
prevents automatic restart. Pre-acceptance loss still restarts the same
generation; canonical synced close remains recoverable. The contract now says
so. Focused race tests prove record retention, Stop rejection without close
proof, no replacement Start, and unchanged durable ACTIVE ownership; the full
storage package passes under `-race` in 3.854s. A preceding gofmt invocation
used duplicate `runtime/` path prefixes and stopped before tests; it made no
change and supplies no product evidence.

The backend test also deletes the process record and proves the authenticated
READY→CLIENT_ACCEPTED log by itself still classifies the generation as
`ClientLost`, never safe absence. The focused three-test race run passes in
1.022s. The complete runtime tree passes with the race detector (storage
3.781s) and repository-wide `go vet ./...` also passes. The documentation and
packaging gate remains next.

The complete documentation/runtime packaging gate now passes: documentation
and links, 7 schemas/22 cases, 17 current evidence manifests, 97 OCI cases,
20 initramfs cases, 12 storage cases, safe publisher, bind/bootstrap/mount/
image checks, release/binary/deployment/ledger/capture/containerd checks, and
the final audit. `git diff --check` is clean. The gate-created bytecode was
limited to the four expected validator/builder cache files and all four have
been removed; no source artifact was deleted. This substantiates the local
accepted-client-loss change but does not close the live server-loss row.

Commit `65b2528` (`runtime: fail closed after storage client loss`) isolates
the seven reviewed implementation, test, and contract files. The accumulated
learning records and evidence trees were deliberately excluded. VM
qualification must bind its build and running process to this exact revision
before any live claim is accepted.

The exact `65b25284ad1a0549140d51eecedffbaab731db59` source archive is
`/tmp/mksrc-65b2528.tar`, 4,823,040 bytes, SHA-256
`f997ab20502aba0e0f61af01f2b8e1e5d4bdef4cddcabfd12c51bf3f0b312aeb`.
This immutable identity is the upload/build input for VM qualification.

The first post-restart baseline probe confirms boot `95e59482…`, 40-minute
uptime, all four services active/running with zero restarts, the prior
`a32e4dd…` immutable mkruntimed selection, and running PID 11432. The probe
then stops at a mistaken `/usr/local/sbin/mkshim` hash path; this is a probe
path error, not a VM/product failure, and leaves the Kerf checks unexecuted.
The corrected baseline remains required before upload.

A second baseline probe again stops before mutation because unprivileged
`command -v mkshim` has no result. This is another harness lookup assumption,
not runtime evidence; even the proposed `/usr/local/bin` fallback was never
reached. The installed path will be discovered read-only rather than guessed.

Read-only discovery finds no standalone file/link named `mkshim` under
`/usr/local`; this deployment supplies shim behavior through its installed
runtime support layout rather than that guessed command name. Boot identity
remains unchanged. The same probe then exposes an obsolete Kerf CLI guess:
this version has no `pool` command, so no absence claim is taken from it.
CLI help must select the deployed command spelling.

The corrected baseline uses deployed `kerf show`: boot is still `95e59482…`,
there is no memory pool and no multikernel instance, and no loaded
`/proc/kimage` entry. `/usr/local/sbin/mkruntimed` resolves to the prior
immutable `a32e4dd…` release and hashes `5dde0c70…`. Its support executable is
correctly named `containerd-shim-multikernel-v2`, not `mkshim`; every release
file shown is root-owned and single-linked. The VM is clean for upload.

Upload completes, but the first remote verification command exits before
printing because its boot-ID command substitution was evaluated by the local
shell inside the SSH argument. The remote mutation sequence was after that
failed assertion and therefore did not run. This is a harness quoting failure
only; archive hash and extraction remain unclaimed until a command without
substitution verifies them.

Corrected verification binds the remote archive to unchanged boot
`95e59482…`, exact 4,823,040-byte SHA-256 `f997ab20…`, then extracts it only
into new `/var/tmp/mksrc-65b2528-build`. The tree is mode 0755, root-owned, and
stripped of group/other write bits. Extracted `service.go` hashes `a885bbd5…`
and `backend_linux.go` hashes `09b75e79…`. This is the sole source tree
authorized for the candidate build.

Exact-source VM build passes on boot `95e59482…`: `runtime-manifest` embeds
full revision `65b25284ad1a0549140d51eecedffbaab731db59`, and the resulting daemon
reports that revision. Candidate hashes are manifest `cd2502b3…`, mkruntimed
`61fe95c8…`, shim `c5263b12…`, mknetd `dfb95b17…`, and agent `caa36a28…`.
Raw evidence `g4-storage-client-loss-build-65b2528.log` is mode 0600, 5,200
bytes, SHA-256 `2d6ce0f0…`, exit 0, with an empty credential-pattern scan.

Candidate installation and activation succeed: manager selects immutable
release `0.1.0-dev-65b25284…`, both daemons restart, the socket appears after
four 250-ms waits, and all four services are active with zero restarts. The
verifier then repeats the known public-path mistake by invoking absent
`/usr/local/bin/mkruntimed`; the service actually uses
`/usr/local/sbin/mkruntimed`. Thus activation is real, but candidate process
hash and Kerf cleanliness remain unproven by this failed log. Retained log is
mode 0600/3,193 bytes/SHA `3de1db6b…`, exit 127, empty credential scan.

Corrected activation audit passes. Current release, public daemon link, public
shim link, daemon/shim version output, and live PID 20720 executable all bind
to full revision `65b25284…`; live and linked daemon hashes both equal build
hash `61fe95c8…`, and shim equals `c5263b12…`. The runtime socket is present,
all four services remain active/zero-restart, and `kerf show` proves no pool,
instance, or loaded kernel. Retained audit is mode 0600, 3,590 bytes, SHA-256
`5d6013f0…`, exit 0, empty credential scan.

The first focused VM test invocation binds to the correct candidate but fails
during Go package setup because `/tmp/mk-go-cache` was created by the root
build and is unreadable to the ordinary test user. No test executes and no
product claim is taken. The retained failure is mode 0600/2,007 bytes, SHA-256
`916f29ff…`, exit 1, empty credential scan. Retry will use a new user-owned
cache rather than changing the build cache.

The user-cache retry executes but two generic ext4-inspection cases fail
because their temporary 64-MiB `fallocate` files on this VM are reported
sparse/uninspectable; the five non-image accepted-loss/reconcile cases are
mixed into the same failing run and cannot be claimed independently. Despite
its premature `-pass` filename, this retained log is a failure: mode 0600,
2,331 bytes, SHA-256 `5840b414…`, exit 1, empty credential scan. The policy
cases will be rerun alone; disposable clone testing will choose and record a
storage filesystem that can satisfy the allocation invariant.

Isolated exact-revision policy qualification passes on the VM under the race
detector in 1.026s: accepted-client crash retention, fail-closed reconcile,
safe pre-accept restart, canonical-close recovery, and interrupted-release
recovery all execute. The mode-0600 1,559-byte log hashes `5484a6a8…`, exits
0, and has an empty credential scan. Live process death and clone behavior
remain separate requirements.

The user's subsequent instance restart advances authoritative boot from
`95e59482…` to `3f80aee1-df81-4fdb-9040-f35afcc73361`. On the new boot, all
four services are active/running with zero restarts, immutable selector and
daemon version still bind full `65b25284…`, and `kerf show` reports no pool,
instance, or loaded kernel. This is direct clean-host-reset evidence for the
V1 non-persistent writable-root model; it does not invent workload-data
durability or replace the pending accepted-client-loss reset test.

Allocation probing explains the earlier ext4-test mismatch: `/tmp` is a
quota-enabled tmpfs, while `/srv/multikernel-storage` is the dedicated ext4
`/dev/sdb`. A bounded 67,108,864-byte `fallocate` on that storage reports
exactly 131,072 512-byte blocks (fully allocated), root ownership, one link;
only the uniquely named probe file/directory were then removed. Disposable
clone qualification will use this dedicated filesystem.

Re-running the exact production backend ext4 tests with `TMPDIR` on the
dedicated storage disk passes under `-race` in 2.407s. This directly covers
clean fully allocated ext4 identity/quota acceptance, wrong UUID/inode limit
rejection, dirty clean-bit rejection, pristine digest mutation rejection, and
permitted current-image content mutation with stable identity. The unique test
directory is removed afterward. Evidence is mode 0600/1,775 bytes/SHA
`4b4eb6ba…`, exit 0, empty credential scan. Explicit corrupt and recovered
disposable clones remain pending.

The first local clone-matrix formatting command repeats the already-known
cwd/path mistake (`runtime/…` while cwd is `runtime`) and stops at `lstat`
before formatting or tests. It makes no change beyond the preceding patch and
supplies no product evidence; the corrected relative path is required.

The corrected formatter succeeds, but its chained local test cannot write the
sandboxed default Go cache under the home directory and stops during setup.
This is a local harness/cache permission failure, not a test result; retry must
set `GOCACHE` under `/tmp`.

With a writable cache, the new clone test reaches the dirty-copy offline check
but strict executable provenance rejects the environment's system `e2fsck`
parent (owner 65534 versus caller 1000). Clean-clone and dirty inspection steps
ran first, but the overall test fails and proves no matrix. The test must copy
the resolved checker bytes into its caller-owned private directory, matching
the backend's existing provenance test pattern.

The next formatter invocation again uses the repository-root path from the
`runtime` cwd and stops at `lstat` before tests. This repeated harness error
changes no source and supports no claim; subsequent commands are issued from
the repository root to eliminate the ambiguity.

The corrected test now reaches the real copied `e2fsck`; contrary to the draft
expectation, `OfflineCheck` correctly rejects the dirty clone as non-clean
rather than returning evidence. This is the desired fail-closed contract. The
assertion will require that rejection, then prove explicit disposable-copy
repair and reinspection before claiming recovery.

Explicit repair then reveals the test's raw clean-bit write invalidates the
ext4 superblock checksum, so `e2fsck -fy` classifies it as corruption (exit 8)
rather than a merely dirty filesystem. This failure is useful separation:
dirty and corrupt cases must not be conflated. The dirty clone will instead be
marked through `debugfs`, which updates ext4 metadata checksums; raw superblock
damage remains reserved for the corrupt-clone case.

With checksum-correct `debugfs` dirtying, production behavior becomes precise:
pristine `Inspect` rejects the unclean bit, while read-only forced `e2fsck`
finds the disposable clone structurally consistent and emits valid
offline-check evidence. The test fails only because its interim assertion
expected rejection. It will assert this split explicitly, then require
writable repair before pristine reinspection.

Corrected clone matrix passes with the accepted-client policy cases under
`-race` in 1.855s. The full storage package then passes under `-race` in
4.663s. The chained `go vet` uses the default unwritable sandbox cache and
fails before analysis, so vet/diff inspection remain unclaimed until retried
with the explicit `/tmp` cache.

Explicit-cache `go vet ./internal/storage` and `git diff --check` pass after
review of the 146-line test-only diff. The complete runtime tree then passes
`go test -race -count=1 ./...` (storage 4.633s) and `go vet ./...`. The clone
matrix now distinguishes inode identity from byte identity, requires
fail-closed pristine inspection of dirty/corrupt copies, authenticates
read-only offline-check evidence for a consistent dirty clone, repairs only
that disposable copy, and re-proves the source digest and identity unchanged.

Commit `f9971d8` (`test: qualify disposable storage clone recovery`) contains
only the reviewed 146-line test change. Its parent is the live production
candidate `65b2528`; no daemon behavior changed. VM execution must still use
an exact archive of this test commit before the clone claims can close.

Exact test commit `f9971d81da8ae6f8d1ec75159bd6ccf4aa374fea` archives to
`/tmp/mksrc-f9971d8.tar`, 4,823,040 bytes, SHA-256 `7fd114e4…`. Learning files
and evidence remain excluded from the archive.

VM verification on new boot `3f80aee1…` matches remote archive SHA-256
`7fd114e4…` and extracts only into root-owned mode-0755
`/var/tmp/mksrc-f9971d8-build` with group/other writes removed. The extracted
test file hashes `995f2c48…`. This exact tree is ready for dedicated-disk
execution; no runtime service was changed.

Exact VM clone matrix passes on dedicated `/dev/sdb` ext4 under `-race`: the
named case completes in 3.65s/package 4.669s and its unique temporary image
tree is removed. It binds active production parent `65b25284…` to exact test
commit/archive `f9971d8…`/`7fd114e4…`. Retained evidence
`g4-storage-disposable-clone-matrix-f9971d8-pass.log` is mode 0600, 2,277
bytes, SHA-256 `96b861aa…`, exit 0, empty credential scan. This completes the
clean/dirty/corrupt disposable-copy portion; live accepted-client server death
remains open.

A bounded destructive live qualifier is now implemented and passes `bash -n`,
available `shellcheck`, and `git diff --check`. It starts from a clean host,
records exact boot/selector/daemon/script provenance, runs concurrent guest
read/write/sync loops, authenticates accepted-client state, SIGKILLs only the
recorded NBD server, and requires restart reconciliation to retain the exact
record/ACTIVE lease/image with zero replacement servers and the explicit
unsafe-recovery error. It deliberately leaves the disposable VM in that
diagnostic state for reset evidence; VM execution is still pending.

Commit `232a49a` (`test: qualify accepted storage client loss`) contains only
the 228-line live qualifier. Full commit is
`232a49a91036ceaab1ec66876d6c0f9613cc8a88`; script SHA-256 is `c4d7a5e3…`.
Its exact 4,833,280-byte archive `/tmp/mksrc-232a49a.tar` hashes `a2da685a…`.
Learning/evidence files remain excluded and active production code is still
parent commit `65b2528`.

Remote pre-fault verification on boot `3f80aee1…` matches archive `a2da685a…`
and script `c4d7a5e3…`, extracted only into root-owned mode-0755
`/var/tmp/mksrc-232a49a-build` with group/other writes removed. Active daemon
reports exact production `65b25284…`; Kerf has no pool, instance, or loaded
image. The destructive qualifier therefore starts from a clean bound state.

First destructive-qualifier attempt never arms the fault: task reaches
RUNNING, but the initial synced 64-MiB seed never creates readiness within 240
quarter-second probes and the task becomes STOPPED. Final exec returns `failed
precondition`; the pre-fault trap removes its task/container. No NBD server is
killed and no fail-closed claim is made. Retained failure is mode
0600/109,436 bytes/SHA `684cf368…`, exit 1, empty credential scan. The seed
exceeds a practical private-root workload bound; retry will use a small seed
while maintaining continuous read/write/sync loops.

Immediate audit corrects the cleanup assumption: although trap commands
suppressed their errors, the stopped task/container, one rootfs record, one
live export, pool, and kernel instance remain; services are still healthy.
Therefore the host is not clean and retry is forbidden until an explicit
ordinary stopped-task delete completes durable release. No sensitive kernel
command-line value from the interactive audit is copied into retained docs or
evidence.

Explicit stopped-task deletion then returns `BACKEND_FAILURE`; read-only
diagnosis finds the exact export durably `QUIESCING`, zero counters/no offline
check, retained process record for absent PID 6341, and exact generation-bound
READY→CLIENT_ACCEPTED log without canonical close. The stopped task/child
remain. This is not the intended ACTIVE-state injection and does not close it.

Pre-reset daemon restart supplies an additional fail-closed proof: restart
command returns before the simple service exits, then systemd records
`reconcile: ... quiescing storage server is absent without a graceful close
record`, result `exit-code`, with no helper. Durable generation remains
`82ce4d15…`, record SHA `eee80879…`, log SHA `b90fec3c…`, and no terminal close.
Mode-0600 evidence is 2,671 bytes/SHA `e161c742…`, exit 0, empty credential
scan. Host reset is now safe to observe, not a recovery claim.

GCE reset succeeds in mode-0600 465-byte control evidence (SHA `0dd1a85a…`,
exit 0, empty credential scan) and advances boot to `2caa6744…`. Post-reset
audit binds selected daemon `65b25284…`/`61fe95c8…` and proves the durable
generation remains `QUIESCING` with zero counters/no offline check and one
rootfs record, while ephemeral `/run/mkstorage`, kernel instances, and ctr
tasks are zero. Container metadata remains. mkruntimed repeatedly exits on the
exact no-graceful-close error (28 restarts observed); other three services
remain healthy. The mode-0600 audit is 25,326 bytes/SHA `5170a8de…`, exit 0,
empty credential scan. This proves fail-closed ownership across host reset,
not workload-data durability or automatic cleanup.

Before environment reset, mkruntimed is stopped and every stale durable input
is hashed: lifecycle state/journal `8b6fb6e…`/`68f2527c…`, rootfs `03f88351…`,
storage `f78dd205…`; mknetd is independently empty at `7aa7fc47…`. The only
storage artifact is the exact 2-GiB root image under one mode-0700 task
directory, and only stopped container metadata remains. These inputs will be
moved intact to uniquely named root-only quarantine; none will be deleted or
treated as product cleanup evidence.

Quarantine reset moves all three hashed trees intact, recreates private empty
directories, removes only orphan container metadata, resets failure
accounting, starts mkruntimed, observes its socket after three waits, and
verifies all four services active/zero-restart. Its final audit incorrectly
assumes an empty lifecycle store eagerly creates `state.json`; the daemon
validly leaves it absent until first mutation, so the log exits 1 before Kerf
checks. Retained partial log is mode 0600/5,131 bytes/SHA `f8cc3c05…`, empty
credential scan. Moves are complete; corrected read-only audit remains.

Corrected audit proves all quarantine directories root-only with the four
exact pre-move hashes, all services active/zero-restart, socket ready, and zero
sandbox/rootfs/export/endpoint/storage/kernel/Kerf resources. However, its two
ctr command substitutions accidentally run unprivileged inside `sudo test`,
emit permission errors, and collapse to empty strings; the misleading `-pass`
log (mode 0600/5,098 bytes/SHA `5b061663…`, exit 0, empty credential scan) does
not prove ctr inventories. A corrected privileged ctr audit is mandatory
before retry.

The independent exact-source audit validates Kerf and the first 15 resource
counters, including privileged default/moby ctr inventories all zero, then
fails because a freshly empty rootfs store has no `state.json`. Retained
failure is mode 0600/8,902 bytes/SHA `7afb6e5e…`, exit 1, empty credential
scan. The audit is corrected to count an absent never-created rootfs or
endpoint state file as zero; its 19-counter contract remains unchanged.

The destructive qualifier retry is reduced from a 64-MiB to a 4-MiB seed and
4-MiB write iterations, preserving concurrent read/write/sync coverage below
the observed workload bound. Its readiness loop now aborts immediately if the
task stops instead of emitting 240 misleading probes.

Both corrected scripts pass syntax validation, available `shellcheck`, and
`git diff --check`; review confirms the audit's expected 19-counter string is
unchanged and only empty-store handling plus bounded workload sizing changed.

Commit `33b2afc81457198c3921e728f860b734d1e63e62` (`test: handle fresh runtime
audit state`) isolates those two script corrections. Audit SHA is `7010e3da…`,
qualifier SHA `0d6cbec4…`; exact 4,833,280-byte archive
`/tmp/mksrc-33b2afc.tar` hashes `0a7e0f92…`.

VM verifies archive `0a7e0f92…` and both corrected script hashes in a new
root-owned exact tree. Corrected independent audit then passes all 19 zero
counters, empty Kerf pool/instances, exact active daemon `65b25284…` hash
`61fe95c8…`, and four active zero-restart services with terminal
`G6_FINAL_RESOURCE_RETURN_PASS`. Mode-0600 evidence is 16,292 bytes/SHA
`ef641bc6…`, exit 0, empty credential scan. The VM is clean for retry.

The first `33b2afc` retry exposes two additional harness defects and does not
arm the intended fault. Its entry `live_counts` still opens the absent fresh
rootfs `state.json` unconditionally; command substitution masks that nonzero
status, so 240 `FileNotFoundError` probes do not stop execution. The workload
is then created, but its helper already has a canonical
`MKNBD_SERVER_CLOSED` before injection, so the explicit guard exits 1 and the
trap removes the task/container. Retained evidence is mode 0600, 213,948
bytes, SHA `f3dd782a…`, exit 1, empty credential scan. Immediate read-only
inspection shows both ctr inventories empty, no kernel instance, all relevant
services active, and only the closed helper log retained in `/run/mkstorage`;
therefore this is neither client-loss evidence nor a completed qualification.
The fresh-state reader and gate error propagation must be corrected before
retry, and the premature canonical close requires diagnosis rather than
weakening the guard.

Retained diagnosis binds boot `2caa6744…` to the exact helper log: after READY
and CLIENT_ACCEPTED it records `MKNBD_SERVER_DISCONNECT_DURING_WRITE
len=2031616 offset=144769024`, then a canonical synced close with 301
reads/10,448,896 bytes, 48 writes/15,278,080 bytes, and 7 flushes. Durable
release preserves the same counters and an offline-clean digest; both ctr
inventories are empty and all four services active. Mode-0600 evidence is
2,591 bytes/SHA `f1e0aac0…`, exit 0, empty credential scan. Thus the 4-MiB
repeated fsync writer plus independent sync loop caused a real guest disconnect
before injection. The retry now uses a 1-MiB seed and continuous one-block
reads plus one-block fsynced writes, retaining read/write/flush pressure
without multi-megabyte write bursts. `live_counts` also treats absent
never-created state files as empty, tests its own command-substitution status,
and the top-level entry gate explicitly exits on failure. These are harness
changes only; the canonical-close guard remains strict.

The revised qualifier passes `bash -n` and `git diff --check`; local
`shellcheck` is unavailable and is not claimed. Commit
`31a23181934938484e4225211bf5c03d283316c5` (`test: harden accepted client loss
qualifier`) contains only the script change. Script SHA is `6207c48a…`; its
exact 4,833,280-byte archive hashes `8d6c55da…`.

The exact `31a2318` archive/script hashes verify in a new root-owned VM tree.
The corrected destructive qualifier then passes on boot `2caa6744…` against
selected production `65b25284…`/daemon `61fe95c8…`. Generation `435cefca…`
reaches ACTIVE with READY→CLIENT_ACCEPTED and no terminal close; guest process
evidence shows both continuous read and fsynced-write loops, while server
`/proc` I/O shows 2,489 reads, 1,390 writes, and 73,383,936 written bytes.
Killing exact PID 7509 leaves it absent, retains the identical process-record
SHA `6dcbaba3…`, and still has no close marker. A post-loss guest exec fails
precondition. Daemon start initially returns 0 but the service immediately
exits on exact error `active storage server disappeared after client
acceptance; automatic session recovery is unsafe`; zero replacement helpers
appear, the root image/record remain, durable ACTIVE generation is byte-for-byte
unchanged, and the kernel instance/pool remain allocated. Terminal marker is
`G4_STORAGE_ACCEPTED_CLIENT_LOSS_PASS`, exit 0. This closes the live
accepted-client-loss/fail-closed behavior before reset; it intentionally does
not claim cleanup or post-reset durability yet.

Evidence hygiene correction: the raw verbose Kerf/process transcript exposed
the ephemeral sandbox credential in guest command lines, so it was moved out
of the evidence tree at mode 0600. The retained mechanical derivative redacts
both credential spellings and otherwise preserves the successful transcript;
it is mode 0600, 74,239 bytes, SHA `ab11a88c…`, with an expanded empty
credential scan and exit 0. Future qualifier output uses non-verbose Kerf
inventory. After derivative verification, the raw credential-bearing temporary
file is removed; it is neither retained nor treated as evidence.

Host-reset qualification exposes an unresolved production defect. GCE reset
control succeeds (mode 0600/460 bytes/SHA `fa723789…`, exit 0, empty credential
scan) and advances boot to `0fc9d4d8…`. On that boot, durable generation
`435cefca…` is still ACTIVE, but its pre-reset accepted-client marker was
ephemeral in `/run`. Storage reconciliation therefore creates a replacement
mode-0600 record/log for the same generation (new SHAs
`13a4c568…`/`79e5da23…`) and leaves a child process in the service cgroup.
Rootfs reconciliation subsequently rejects the missing ephemeral owned bundle
and drives mkruntimed into an auto-restart loop, so no kernel instance or ctr
task is recreated and only container metadata remains. That later failure does
not restore the violated storage invariant: the dirty image was already
reopened automatically after accepted-client loss. Retained audit is mode
0600/35,439 bytes/SHA `c49da816…`, exit 0, empty credential scan. The
accepted-client fact must become durable (or equivalent durable policy must
distinguish pre-acceptance from post-acceptance absence) before this row can
close; current host-reset behavior is explicitly not fail-closed.

Focused process evidence removes ambiguity: leftover PID 1481 is exact deployed
`mkvsock-nbd` SHA `95e886d6…`, argv-bound to the same image, port, and export
generation `435cefca…`; fd 3 and fd 5 both hold the durable root image, fd 6 is
its listening socket, and stdout/stderr target the replacement log. The
replacement record matches PID/start-time 1481/2302 and image inode 1179704.
Its log contains only the exact READY marker—no client acceptance or
close—while mkruntimed has reached 119 restart attempts. Mode-0600 evidence is
2,293 bytes/SHA `a43ea4ff…`, exit 0, empty credential scan. This proves an actual
image-owning server restart, not merely stale record creation.

Remediation now makes ACTIVE restart conditional on explicit exact
pre-acceptance evidence. Backend observation distinguishes: live exact process;
canonical close; accepted-client loss; exact log containing only the
generation-bound READY marker; and unknown absence. Only that exact-READY-only
case is restartable. An empty/missing runtime directory, absent log, or
noncanonical trailing data is unknown and returns `active storage server is
absent without exact pre-acceptance evidence` before image inspection/start.
Existing same-boot pre-acceptance recovery is retained; accepted-client and
unknown host-reset loss both remain owned and fail closed. Focused
storage/lifecycle packages and the full runtime Go suite pass with a
workspace-local `/tmp` build cache; qualifier syntax and `git diff --check`
also pass.

Production remediation is isolated in commit
`ebbb2db89c70595c4abb5355f97f30bf28cba8d2` (`runtime: require proof before
active storage restart`); evidence-hygiene-only commit
`ca7d7d013485a2ddb764707e63fb2c35ab1ed72b` removes verbose Kerf output from
the qualifier. Final qualifier SHA is `bcef69b9…`; exact combined 4,833,280-byte
source archive SHA is `a54f8988…`.

VM verifies archive `a54f8988…`, service SHA `ff920f45…`, backend SHA
`0692f25b…`, and qualifier SHA `bcef69b9…` in a new root-owned tree. Exact VM
build embeds full revision `ca7d7d013485a2ddb764707e63fb2c35ab1ed72b`.
Candidate hashes are manifest `a9e5de5e…`, mkruntimed `a4a91006…`, shim
`baaab3fd…`, mknetd `5eed6606…`, and agent `6ae77e65…`. Mode-0600 build
evidence is 5,056 bytes/SHA `a48e9343…`, exit 0, empty credential scan.

Candidate installation atomically selects immutable release
`0.1.0-dev-ca7d7d013485a2ddb764707e63fb2c35ab1ed72b`; linked daemon/shim hashes
match build values `a4a91006…`/`baaab3fd…`. mkruntimed is deliberately stopped
before selection and the known unsafe replacement PID 1481 remains until the
qualification reset, preserving the exact faulted input rather than
manufacturing clean state. Mode-0600 activation evidence is 3,562 bytes/SHA
`57431f4b…`, exit 0, empty credential scan.

The first candidate post-reset audit is a harness-only failure: after the
controlled reset exposed intermediate boot `9819413d…`, an additional operator
restart advanced the host to `f93f21b6…`. The audit's exact intermediate-boot
assertion therefore exits 1 immediately, before selector, journal, helper,
durable-state, ctr, or Kerf assertions. Renamed retained failure is mode
0600/2,906 bytes/SHA `bf1850b0…`, empty credential scan; it proves no candidate
behavior and is not a pass. Retry must bind to the current boot while still
excluding the known pre-candidate boot.

Current-boot retry supplies partial positive product evidence but remains a
harness failure. On boot `f93f21b6…`, exact candidate revision/hash and selector
verify; 50 observed daemon restarts all fail on `active storage server is absent
without exact pre-acceptance evidence`, and the boot journal contains no
`rootfs reconcile:` entry. `/run/mkstorage` is absent entirely, which is
stronger than an empty directory, but the audit's unguarded `find` exits 1
there before helper-count, durable-state, ctr, Kerf, or terminal-pass
assertions. Renamed mode-0600 failure is 57,262 bytes/SHA `b142ac2d…`, empty
credential scan. It is not the final pass; corrected audit must count an absent
never-created runtime directory as zero and complete remaining checks.

Candidate reset control is mode 0600/460 bytes/SHA `2a9e8138…`, exit 0, empty
credential scan; the operator's subsequent restart advances the final observed
boot to `f93f21b6…`, adding another restart boundary. Corrected final audit
passes against exact selected revision `ca7d7d01…` and daemon SHA `a4a91006…`.
Across the entire boot journal, every reconcile attempt stops at `active
storage server is absent without exact pre-acceptance evidence`, with no
`rootfs reconcile:` execution. Runtime files=0 even when `/run/mkstorage` is
never created; exact storage-server processes=0; durable state SHA remains
pre-fault `c05b754d…`, with one unchanged ACTIVE generation `435cefca…`,
sandbox generation `c4b8a9f6…`, zero untrusted terminal counters, and exact
image device/inode 2064/1179704. ctr tasks=0, Kerf has no pool or instances,
and container metadata alone remains. Terminal marker
`G4_STORAGE_HOST_RESET_FAIL_CLOSED_PASS` exits 0. Mode-0600 evidence is 15,346
bytes/SHA `d895049b…`, empty credential scan. This closes the accepted-client-
loss behavior across host reset: the candidate retains ownership and never
reopens the ambiguous image.

Together with disposable clone evidence `96b861aa…` and the accepted-client
live qualifier `ab11a88c…`, this completes the storage server-loss/corruption/
recovery row: mixed continuous reads and one-block fsynced writes are active at
exact helper death; daemon restart and host reset both retain ownership without
replacement; clean, dirty/repaired, and corrupt disposable copies follow their
explicit inspection outcomes; and the final reset returns all ephemeral ctr
and Kerf resources. The claim remains scoped to fail-closed ownership and
resource return, not V1 workload-data persistence across reset.

To prepare subsequent qualification without erasing the diagnosed state, exact
lifecycle state/journal (`1e493170…`/`05c36775…`), rootfs state
(`e359f144…`), storage state (`c05b754d…`), mknetd state (`1b40028d…`), and
2-GiB root image (`437f2a7c…`) are hashed then moved intact into four unique
root-only mode-0700 quarantine trees bound to boot `f93f21b6…`. Only orphan ctr
container metadata is removed; new private empty directories are created and
all four services become active/running with zero restarts. This is an
administrative recoverable reset, not product cleanup evidence. Mode-0600 log
is 3,532 bytes/SHA `31397961…`, exit 0, empty credential scan.

Independent exact-source post-reset audit binds selected candidate
`ca7d7d01…`, live daemon SHA `a4a91006…`, and boot `f93f21b6…`; all 19
resource counters are zero, Kerf has no pool/instances, and all four services
are active/running with zero restarts. Terminal marker
`G6_FINAL_RESOURCE_RETURN_PASS` exits 0. Mode-0600 evidence is 16,301 bytes/SHA
`3aeb7b79…`, empty credential scan. The VM is clean for the next storage-
isolation qualifier, while all fault inputs remain quarantined.

Cross-sandbox storage qualification is now in progress on restarted disposable
boot `f93f21b6…`, beginning from that independently clean exact candidate
state. This is not covered by the existing sibling IP-isolation result. Source
review shows that `mkvsock-nbd` binds the primary Multikernel AF_VSOCK listener
to `CID_ANY`, authenticates image size, image ID, and random export generation
before serving any block request, and accepts only one stream, which the
intended child normally occupies. Those facts are design evidence, not a live
pass. Closure requires reciprocal peer-port attempts from two concurrent
sandboxes with both incorrect and exact identities; neither may create or
mount `/dev/nbd1`, disturb the intended `/dev/nbd0`, alter the peer image or
durable owner record, or leave primary resources after normal teardown.

Restart preflight observes exact boot
`f93f21b6-21f1-4fc0-aebc-806347d6b43d`, selected release
`0.1.0-dev-ca7d7d01…`, live daemon SHA `a4a91006…`, all four services active,
zero kernel instances, and zero ctr tasks. The ad-hoc command then exits
nonzero because it tries to open absent fresh
`/var/lib/mkruntimed/storage/state.json`. The prior administrative cleanup
correctly left no state file; this is a harness-only assumption and no product
failure. The cross-sandbox qualifier will explicitly count missing fresh state
as zero live exports.

The first retained cross-sandbox qualifier is an environment-capacity failure,
not isolation evidence. It binds the exact boot/release/daemon, installed and
read-only-injected NBD helper SHA `95e886d6…`, qualifier SHA `66e71807…`, and
an all-zero initial inventory. On-VM focused tests pass both hello identity
matching/rejection and post-handshake timeout clearing. The first child root
build then correctly fails closed because the storage high-water guard observes
2,684,846,080 free bytes against 3,233,472,512 required; no child and no peer
attempt starts. Mode-0600 transcript
`g4-cross-sandbox-storage-live-first.log` is 1,389 bytes/SHA `b3a871b6…`, exits
1, and has an empty expanded credential scan. Capacity and post-failure
inventory must be resolved before retry; this run does not close the row.

Post-failure inspection proves cleanup returned zero kernel instances, ctr
tasks/containers, and storage/relay helpers; only the expected empty 36-byte
rootfs state remains. The 100-GB boot disk still has 51,274,813,440 bytes free,
but the dedicated 20-GB storage filesystem has 17,182,076,928 used and only
2,684,850,176 available. Four retained 2-GiB forensic quarantine images plus
the earlier dual-root experiment images explain the pressure. Those evidence
inputs will not be deleted; the authorized disposable data disk/filesystem will
be expanded before the retry.

GCE maps `/dev/sdb` to the nonboot persistent disk
`mk-mediated-storage-20260830`; the entire device is ext4 mounted with
`rw,nosuid,nodev,relatime`. It is expanded in place from 20 to 30 GB, then
online `resize2fs` grows the filesystem from 20,957,446,144 to 31,526,436,864
bytes. Used bytes remain exactly 17,182,076,928, while available space rises
from 2,684,850,176 to 12,802,879,488. Thus no quarantined evidence was deleted,
and the unchanged qualifier now has capacity for two concurrent 2-GiB roots.

The capacity-fixed second run is a harness-readiness failure, not storage
isolation evidence. Exact provenance, zero initial inventory, and both focused
C tests pass, and two tasks allocate. The qualifier then incorrectly treats
containerd `RUNNING` as proof that the guest workload has created
`/tmp/storage-isolation-marker`; the first immediate read finds no file and the
run exits before export extraction or any peer attempt. The exit trap returns
zero instances, ctr tasks/containers, helpers, rootfs records, and live
exports. Mode-0600 transcript `g4-cross-sandbox-storage-live-second.log` is
1,291 bytes/SHA `ac8fc9ea…`, exit 1, with an empty credential scan. The next
revision must wait boundedly for each guest marker.

Readiness-fixed run three reaches two real ready guests with distinct durable
owners on ports 4061/4062. It records both full 2-GiB image hashes,
storage/rootfs state hashes, exact server PIDs/I/O counters, and only SHA-256
digests—not plaintext—of both bearer export generations. The first peer
attempt never starts because the harness calls `chmod` before creating its
private per-attempt log. The exit trap is independently verified to return zero
instances, ctr tasks/containers, helpers, rootfs records, and live exports.
Mode-0600 transcript `g4-cross-sandbox-storage-live-third.log` is 2,254
bytes/SHA `a048dccf…`, exit 1, with an empty credential scan. This is partial
two-owner setup evidence only; the narrow correction is to create the log
before applying mode 0600.

Private-log-fixed run four again reaches two distinct live owners, but all four
nominal reciprocal attempts exit immediately with status 126, duration zero,
and identical 32-byte output SHA `848f2037…`. `/dev/nbd1` remains zero sectors
and each mount fails, but these observations result from failure to execute the
injected helper and do not test the transport boundary. The run later exits
without post-integrity, final-inventory, or pass markers. Mode-0600 transcript
`g4-cross-sandbox-storage-live-fourth.log` is 3,555 bytes/SHA `9f13ac7a…`, exit
1, with an empty credential scan. The classifier must fail immediately on
status 126/unknown classes, and guest helper mode/execution must be diagnosed.

Source inspection identifies the design-level cause: readonly inputs are
admitted only with the exact option vector `bind,ro,nodev,nosuid,noexec`, and
their retained guest policy is explicitly
`bind-remount-ro-nodev-nosuid-noexec`. A binary injected through this channel
must not execute. The cross-sandbox retry path is paused rather than weakening
that production boundary; when resumed it requires a purpose-built OCI test
image containing the helper.

Independent post-run inspection confirms the paused fourth run returned zero
kernel instances, ctr tasks, and live exports.

The resumed qualifier preserves that policy. VM inspection proves the exact
deployed helper is a statically linked x86-64 ELF, while BusyBox 1.36 is already
present in both local Docker and containerd stores. The qualifier now builds
locally with `--pull=false --network=none`, copying the helper into
`/usr/local/bin` of a purpose-built OCI image, imports the Docker archive into
containerd, proves the helper's SHA inside both guests, and removes both image
references during cleanup. Peer attempts execute the in-image path; the
read-only bind and its `noexec` policy are untouched. It also hashes its
executing path rather than assuming the source tree copy. Local Bash syntax and
diff checks pass; ShellCheck is unavailable. Mode/size/SHA-256 are `0755`,
14,029 bytes, and
`5efcdf0dd95445939d0100e69b36ac3d6a3672164f66b393e7a184dc570079cf`.
This is implementation evidence only until the exact qualifier runs live.

Guest transfer `/tmp/test-runtime-cross-sandbox-storage-live-5efcdf0d.sh`
independently matches mode `0755`, size 14,029, full SHA-256
`5efcdf0dd95445939d0100e69b36ac3d6a3672164f66b393e7a184dc570079cf`,
and syntax marker `GUEST_STORAGE_ISOLATION_VERIFY_PASS`. No isolation result is
inferred from transfer verification.

Fifth retained run stops during local image construction before provenance,
child creation, or any peer attempt because the VM's legacy Docker builder does
not implement Dockerfile `COPY --chmod`. The private transcript
`g4-cross-sandbox-storage-live-fifth.log` is mode `0600`, 921 bytes, SHA-256
`af6b9cb63405562c93ef601d09fdd751ad0f3d04a9ed5df7ec1e7a9f4ed45ab2`,
exit 1, credential-pattern clean. Cleanup removes the scratch context and any
provisional image reference. The Dockerfile now uses portable `COPY` followed
by `RUN chmod 0755`; this changes only test-image construction and the
isolation row remains open. Corrected mode/size/SHA-256 are `0755`, 14,058
bytes, and
`5c47fe19f64ad0fb8f953beac059108b107d5000378091d91ce9364e1d18b278`;
local syntax and diff checks pass.

Guest transfer `/tmp/test-runtime-cross-sandbox-storage-live-5c47fe19.sh`
matches mode `0755`, size 14,058, and full SHA-256
`5c47fe19f64ad0fb8f953beac059108b107d5000378091d91ce9364e1d18b278`;
guest syntax ends `GUEST_STORAGE_ISOLATION_PORTABLE_VERIFY_PASS`.

Sixth run supplies the first real reciprocal transport evidence but remains a
harness failure. Exact image construction/import and helper SHA `95e886d6…`
pass; distinct live owners occupy ports 4061/4062 with distinct image and
generation digests. All four A→B/B→A wrong/exact attempts execute the in-image
helper, fail after 15–16 seconds as `occupied-listener-no-response`, leave
`/dev/nbd1` at zero sectors, and fail the peer mount. The run then exits before
its integrity marker because it requires online whole-image hashes and server
I/O counters to remain exact, even though its own guest commands create a mount
directory and read/write the legitimate root. Retained mode-0600 transcript is
4,356 bytes, SHA-256
`04437a467eb2efe2931b712d0a8625aa8441ed87dd837d0d53ba43ed9f674b58`,
exit 1, credential-pattern clean. The trap removes all tasks, containers,
instances, helpers, rootfs/live-export records, and both image references. Two
immediate strict audits (mode 0600/406 bytes, SHAs `795e316b…` and
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
exit 0, credential-pattern clean. The following strict audit passes clean Kerf,
all 19 counters zero, and four healthy services: mode `0600`, 1,909 bytes,
SHA-256 `d2d3d9882e79824299c3ef5da778069a959f19a94914b1c4dbfd32b529816896`,
exit 0, credential-pattern clean. The canary/idle-release qualifier passes
local syntax and diff checks and is now mode `0755`, 15,376 bytes, SHA-256
`78b5e81e3d30a287f2d3efae456c98b5546f3b92e9ffab81ad6d6965050d405b`.

Guest upload `/tmp/test-runtime-cross-sandbox-storage-live-78b5e81e.sh`
independently matches mode `0755`, size 15,376, full SHA-256
`78b5e81e3d30a287f2d3efae456c98b5546f3b92e9ffab81ad6d6965050d405b`,
and syntax marker `GUEST_STORAGE_ISOLATION_CANARY_VERIFY_PASS`.

Seventh run closes the row as `g4-cross-sandbox-storage-live-seventh.log`: mode
`0600`, 5,934 bytes, SHA-256
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
returns instances/tasks/rootfs/live exports/helpers to zero with no Kerf pool.
Independent `g4-cross-sandbox-storage-final-resource-audit.log` is mode `0600`,
1,909 bytes, SHA-256
`e6f499cbb9b4fd9b172acf2da55bf35f657a2e0ab3cbec95c1ded9c1a40b110d`,
exit 0, credential-pattern clean; it confirms clean Kerf, all 19 counters zero,
and four active zero-restart services.

Commit `f35de77b4ef159d0f4951032b41f005c27aa4a49` (`test: qualify cross-sandbox
storage isolation`) checkpoints the exact qualifier and both learning records.
The checklist now has 59 closed and 26 open rows.

The next G4 provenance row now has an exact source-derived model. The builder
installs trusted BusyBox, `mk-agent`, relay, and `mk-agent-init` at the outer
ext4 root, copies the caller OCI snapshot only under `/bundle/rootfs`, and
rewrites OCI `root.path` to `rootfs`. The separate initramfs contains BusyBox,
`mkvsock-nbd`, NBD/transport modules, and `runtime-mediated-init`; after NBD
mount it `switch_root`s into the trusted outer `/init`. `mk-agent` launches OCI
processes with chroot `/bundle/rootfs`. Thus task `/bin/busybox` and any ELF
interpreter/libraries must originate in the OCI subtree, while `/mk-agent`,
relay, outer init, and bootstrap-only tools/modules remain outside it. This is
not yet a pass: live child-observed hashes, mount/device/inode identities, and
comparison with exact OCI/bootstrap manifests are still required.

On the current candidate host, `gce-mk2.json` provides exact trusted hashes for
agent `b57bae66…`, relay `293ff1ea…`, initramfs `2dec85b8…`, kernel
`5cdf26d0…`, and transport module `bef1b888…`. The selected release directory
contains only host binaries, so child outer-root observations must be matched
to this approved manifest and exact builder inputs rather than inferred from a
same-named file in the runtime release.

The deployed builder resolves to immutable deployment `b4d185c6…` and selects
`/usr/local/libexec/multikernel/guest/mk-agent-init`; deployed and exact
candidate-source copies both hash to `edc9284c…`. This supplies a concrete
comparison target for child outer `/init` in the live qualifier.

The first retained provenance run is a harness false positive even though it
prints `G4_ROOT_PROVENANCE_LIVE_PASS`. Its `cleanup` helper runs `set +e` in the
parent shell and leaves assertion exits disabled. Primary reads of root-owned
containerd bundle/runtime paths fail with `EACCES`; workload traversal of
`/proc/1/root/...` is also correctly denied; empty values then compare equal
and execution continues. No provenance claim is accepted. The file is renamed
`g4-root-provenance-live-first-false-pass.log`; it is mode 0600/3,436 bytes/SHA
`2da0a7b4…`, wrapper exit 0, empty credential scan. Only the product-level
proc-root denial and genuine all-zero final inventory are useful. Correction
requires subshell cleanup, privileged primary reads, mandatory nonempty
observations, and an explicit assertion that outer proc-root traversal fails.

Corrected provenance run two restores fail-closed assertion behavior and exits
1 without a pass marker. It proves a clean baseline, exact
boot/release/daemon/manifest, qualifier SHA `70c51fa…`, and equality of every
approved trusted artifact on the primary, but ends before live-image or child
observations. Its redirected command emits no diagnostic, so no provenance
claim is accepted. Independent inspection finds zero instances, tasks,
containers, helpers, rootfs records, and live exports. Mode-0600 transcript
`g4-root-provenance-live-second.log` is 1,890 bytes/SHA `5cf0b81a…`, exit 1,
empty credential scan. The retry needs a failing-line/status EXIT diagnostic
and non-precreated private `debugfs` dump targets.

Diagnostic provenance run three also fails closed after only baseline and
approved-artifact observations. Its EXIT hook prints `line=1`, which does not
identify the failed command; no live-image/child observation or pass marker is
present. Mode-0600 `g4-root-provenance-live-third.log` is 1,945 bytes/SHA
`16581e66…`, exit 1, empty credential scan. A credential-free shell trace of
the unchanged harness is required to localize the harness failure.

The retained shell trace identifies the exact failure: privileged `test -f`
of `<containerd-bundle>/rootfs/bin/busybox` returns false because that snapshot
is mounted only inside the shim's private mount namespace. This is expected
isolation. Mode-0600 diagnostic is 13,166 bytes/SHA `f43d49d0…`, exit 1, empty
credential scan, with cleanup completed. The SHA-bound
`initramfs.source-manifest.json` already captures the held OCI snapshot's
`bin/busybox` type, mode, owner, size, and content hash; the corrected qualifier
must use that manifest instead of attempting to cross the shim namespace.

Manifest-based run four provides strong partial product evidence but exits 1
on its mount-string assumption. Exact live ext4 inspection records outer agent
inode 59/hash `b57bae66…`, relay `293ff1ea…`, init `edc9284c…`, OCI BusyBox
inode 21/hash `f060103f…`, and OCI-root inode 19. Inside the workload, `/` is
device/inode `11008:19`, BusyBox is `11008:21` with the same `f060103f…` hash,
outer proc-root traversal exits 1, nbd0 is device `43:0` with 4,194,304 sectors,
and `mk_transport`/`nbd` are live. The assumed `/proc/mounts` form yields an
empty `root_mount`, so line 226 correctly stops the run. Mode-0600 transcript
is 4,855 bytes/SHA `6260575e…`, exit 1, empty credential scan. The final
assertion should bind child `st_dev` to `makedev(43,0)` and retain the actual
nbd0 mount line; expected cleanup misses must also stop firing the ERR trap.

Device-bound run five repeats the exact hash/inode matches and removes cleanup
ERR noise, but exits 1 at line 237 because `/proc/mounts` does not name the
source `/dev/nbd0`. Mode-0600 transcript is 4,693 bytes/SHA `35a2827e…`, empty
credential scan. The authoritative device proof is already exact: workload
root `st_dev=11008`; observed nbd0 major/minor `43:0`; Linux `makedev(43,0)` is
11008. The final revision will retain the complete child mount table and assert
that device equality rather than assume a mount-source pathname.

Mount-table run six retains that complete table: devtmpfs, proc, read-only
sysfs, private `/run`, and protected proc/sys projections. It contains no ext4
row because procfs exposes mounts beneath the workload chroot and omits the
containing ext4 mount. The residual line-216 `grep ' ext4 '` exits 1 before the
device-number assertion. Mode-0600 log is 5,807 bytes/SHA `f7f5b5d4…`, empty
credential scan. The table remains retained; only this contradicted row
assumption is removed, while `st_dev == makedev(nbd0)` remains mandatory.

Final root-provenance qualifier SHA `1212ad47…` passes on exact boot
`f93f21b6…`, selected release `ca7d7d01…`, daemon `a4a91006…`, and approved
manifest `d4230978…`. The held OCI manifest records `bin/busybox` as mode 0755,
uid/gid 0, 1,013,320 bytes, SHA `f060103f…`; live ext4 inspection assigns it
inode 21 and the OCI root inode 19, exactly matching child observations
`11008:21` and `/` `11008:19`. Root device 11008 equals Linux
`makedev(43,0)` for the child's 4,194,304-sector nbd0. This BusyBox has no ELF
interpreter or `DT_NEEDED`, hence no dynamic-library provenance remains.
Outer image hashes exactly match approved agent `b57bae66…`, relay
`293ff1ea…`, and deployed init `edc9284c…`; trusted paths are absent inside the
workload and outer proc-root traversal exits 1. Loaded `mk_transport`/`nbd` and
the complete visible mount table are retained. Teardown returns all tracked
instances, tasks/containers, records, exports, helpers, links, and firewall
rules to zero. `G4_ROOT_PROVENANCE_LIVE_PASS` exits 0. Mode-0600 transcript is
6,827 bytes/SHA `ad9a358e…`, with an empty credential scan. The provenance row
is closed.

The validated provenance harness and focused hello-identity negative tests are
preserved in isolated commit `940e21e749f0…` (`test: qualify runtime root
provenance`). Live runtime binaries remain exact `ca7d7d01…`; the evidence
separately binds uploaded qualifier SHA `1212ad47…`.

The next G4 evidence-row audit separates already retained facts from the
remaining claim. Exact-source runs already record generation-bound storage
ownership, nonzero read/write/flush counters, guest quiesce before NBD
disconnect, offline-clean `e2fsck`, and byte/inode quota plus high-water
behavior. Those facts are insufficient to infer physical-device isolation.
The row stays open because no one retained run binds the live backing image's
allocation/quota and child mount table to before/after inventories proving that
all real cloud block devices and allocatable storage controllers remained in
the primary. A focused replacement-instance qualifier must collect precisely
that missing evidence and must not attach a cloud disk/controller to the child.

Read-only inspection of restarted boot `f93f21b6…` makes that qualifier
machine-specific rather than speculative. The 100-GiB boot disk
`persistent-disk-0` and 30-GiB store `mk-mediated-storage-20260830` are SCSI
LUNs `0:0:1:0` and `0:0:2:0`; their sysfs ancestry converges on the same
primary Virtio-SCSI PCI function `0000:00:03.0` (`1af4:1004`). `/dev/sdb`
remains mounted only at `/srv/multikernel-storage`. Kerf reports an available
CPU/memory pool, no instances, and an empty `/proc/kimage`. The focused run
must retain and compare serial/HCTL/sysfs/PCI inventories before, during, and
after; preserve both primary mounts; prove the child has no physical `sd*`
device or storage PCI controller; and record its NBD/mount view independently.

That focused qualifier now exists as
`test-runtime-storage-primary-ownership-live.sh`. It also binds the fully
allocated image bytes to the declared byte quota and inode limit, records
post-allocation free-byte/free-inode state against production reserves, and
requires generation-identical released counters plus offline-check evidence.
Local Bash syntax and diff checks pass; local `shellcheck` is unavailable. The
11,324-byte script hashes to `09607c71402d…`, and the transferred copy on boot
`f93f21b6…` matches that digest and passes VM-side syntax. Execution evidence
does not yet exist.

The first execution is a fail-closed diagnostic, not a pass. Its private log is
mode 0600/51,893 bytes/SHA-256 `4bd181c60c3c…`, exits 1, and has an empty
expanded credential scan. Before the failure it proves unchanged primary
disk/controller/mount inventories across the live child; a fully allocated
2-GiB image equal to quota, 262,144-inode limit, distinct owner/export
generations and healthy remaining high water; no child `sd*` or storage-class
PCI function; complete child mount data; quiesce-before-disconnect ordering;
and released nonzero counters with offline-clean SHA `b3b2f0ec…`. The final
idle-daemon restart exposes real deployment drift caused by the earlier safe
20→30-GiB data-disk expansion: systemd's storage preflight still expects the
old byte size, so restart loops on `runtime storage byte size mismatch` even
though live workload cleanup is complete. At diagnosis the restart count is
18, Kerf has no pool/instance, durable live counts are zero, and both ctr
inventories are empty. Correct the expected-size deployment configuration and
then rerun from a healthy zero-restart service baseline.

The first immutable correction attempt is itself retained. It verifies the
30-GiB device and candidate environment SHA `7f296d38…`, stops the already
failing service, and then the deployment manager correctly rejects the
ordinary-user-owned uploaded input as unsafe. It installs and activates
nothing. `g4-storage-size-config-correction.log` is private mode 0600/1,990
bytes/SHA-256 `a5f6114e…`, exits 1, and has an empty credential scan. The same
bytes must be made root-owned mode 0600 before the managed retry.

The managed retry succeeds. Exact candidate source installs and atomically
selects immutable support deployment `6182145c5cef…`, whose strict environment
declares the observed 32,212,254,720 bytes. The storage preflight accepts it;
mkruntimed is active with zero restarts; selected runtime `ca7d7d01…` and
daemon SHA `a4a91006…` are unchanged; Kerf has no pool/instance; and both ctr
inventories are empty. Private correction evidence is mode 0600/3,511 bytes,
SHA-256 `83ab9edc…`, exit 0, with an empty credential scan. This restores a
healthy baseline for the focused rerun while preserving the first failure.

The unchanged qualifier rerun reaches the corrected healthy daemon restart and
then fails only on a harness representation assumption: it requires a
configured pool with zero allocated bytes, while a clean idle restart properly
reports `No memory pool configured`. All substantive observations repeat.
Independent inspection confirms active mkruntimed with zero restarts, no Kerf
pool/instance, empty ctr inventories, and zero durable live counts. Private
`g4-storage-primary-ownership-live-second.log` is mode 0600/53,092 bytes,
SHA-256 `fafcd762…`, exit 1, with an empty credential scan. The final assertion
must accept either no pool or a configured zero-byte pool, never a nonzero one.

The narrow final-state correction passes local/VM Bash syntax and diff checks.
Its 11,488-byte SHA-256 is `fa46fb36364a…`, exactly matching the remote
mode-0700 copy. It changes no storage, isolation, ordering, or cleanup
assertion; a third execution is still required.

The exact third execution passes with
`G4_STORAGE_PRIMARY_OWNERSHIP_LIVE_PASS`. Its private transcript is mode 0600/
54,812 bytes/SHA-256 `028e0733…`, exit 0, with an empty expanded credential
scan. Boot `f93f21b6…`, runtime `ca7d7d01…`, daemon `a4a91006…`, and qualifier
`fa46fb36…` are bound. Primary inventories before, during, and after are
byte-identical: the 100-GiB boot and 30-GiB store remain at SCSI LUNs
`0:0:1:0`/`0:0:2:0`, mounted at `/` and `/srv/multikernel-storage`, behind
Virtio-SCSI `0000:00:03.0`. The child contains no `sd*` and no storage PCI
class, only loop devices, 2-GiB nbd0, empty nbd1, and the retained protected
mount table. Its fully allocated image is 2,147,483,648 bytes with
2,147,487,744 allocated bytes, equal byte quota, 262,144-inode limit, and
10,655,387,648 bytes/1,966,045 inodes free above production reserves. Owner
`aa2738aa…` and export `40685c89…` generations survive to release; quiesce line
7 precedes disconnect line 9; counters are 280 reads/10,461,184 bytes, 49
writes/823,296 bytes, and 9 flushes; offline-clean SHA is `b3b2f0ec…`.

The subsequent independent exact-source audit (`7010e3da…`) also exits 0 with
`G6_FINAL_RESOURCE_RETURN_PASS`. All 19 child, mount, artifact, FIFO, network,
ctr/Docker, durable-record, endpoint, and helper-process counters are zero;
Kerf has no pool/instance; and four services are active/running with zero
restarts. Its private credential-clean transcript is mode 0600/16,505 bytes,
SHA-256 `b2f411ea…`, and binds the same boot/release/daemon. Together these
observations close the composite backing-allocation, accounting, teardown,
offline-check, and primary physical-storage-ownership evidence row.

The validated qualifier is isolated in commit `9c6c40f` (`test: qualify
primary storage ownership`). The post-closure repository gate passes every
documentation, schema, OCI, initramfs, storage, bind, bootstrap, mount, image,
release, deployment, ledger, capture, containerd, and final-audit check. Five
generated bytecode files are removed, the diff check is clean, and the running
checklist now reports 52 closed and 33 open.

Audit of the final open G4 replacement-evidence row finds no need for a new
injection. Private exact-source `g4-partial-artifact-live-pass.log` (mode 0600,
94,007 bytes, SHA `4e66461f…`, exit 0, credential-clean) retains the actual
post-allocation one-shot `load` rejection and its immediate 20-field inventory.
Child, mount, artifact, FIFO, network/TUN/rule, container, durable-owner,
endpoint, NBD, and relay counts are zero. Two disconnecting shim processes are
explicitly visible immediately, sampled at 250 ms, and reach zero before the
strict checkpoint. The failed storage generation is `RELEASED` with an offline
clean check. That same raw VM transcript names and passes all nine Create
rollback stages, six rootfs failure/diagnosability cases, and four lifecycle
cancellation/pool cases, then repeats the all-zero inventory. Independent
`g4-partial-artifact-final-resource-audit-pass.log` (14,185 bytes, SHA
`545ad9ea…`) confirms all 19 counters zero, no Kerf state, and healthy services.

The builder/allocation half is likewise raw rather than summary-only.
`g4-storage-fault-matrix-live-pass.log` is private mode 0600/86,309 bytes, SHA
`1eca1db8…`, exit 0, credential-clean, and retains interrupted-copy, wrong-UUID,
every storage/initramfs ENOSPC allocation/publication boundary, real block and
inode ENOSPC, and byte/inode high-water cases; each case asserts no public or
private partial artifact. Bracketing resource audits are zero, and independent
`g4-storage-post-fault-final-resource-audit.log` is private mode 0600/16,301
bytes, SHA `3aeb7b79…`, exit 0. Together the raw named failure outputs,
immediate post-live-fault inventory, convergence observation, durable release,
and independent audits close the G4 injected-failure evidence row.

Audit of the first open G5 flow row confirms that historical markers are not
enough. The shared matrix proves a child address and successful hostname HTTP,
but intentionally discards the resolver answer and HTTP response; it records
neither a primary-owned listener exchange nor a distinct UDP reply. Focused
tests establish configuration and counter behavior only. The row therefore
remains open pending an exact-source live qualifier that retains endpoint
identity, tokenized child↔primary TCP and UDP request/reply details, configured
resolver and DNS answers, external TCP destination/status/body digest,
network counters, and independent clean teardown.

The focused implementation is now
`test-runtime-network-flows-live.sh`. It uses tokenized one-shot primary-owned
TCP and UDP listeners, exact child replies and primary peer/payload JSON,
retained endpoint/DNS data, external HTTP status/body size/digest, identity and
counter comparison, normal deletion, idle-pool return, and the independent
19-counter audit. Local Bash syntax and diff checks pass; `shellcheck` is not
installed. The executable is 9,414 bytes/SHA-256 `25fa8a71dc60…`; execution
remains pending. Transfer provenance is now established: the running disposable
VM reports the uploaded `/tmp/test-runtime-network-flows-live-25fa8a71.sh` as
mode `0755`, owner `hairizuan-tw:hairizuan-tw`, size 9,414 bytes, and complete
SHA-256
`25fa8a71dc60c7c9ae78221e1ae116fc2ad07ae372ba352fe9617552e4a9bc98`;
guest-side `bash -n` passes. This records only the exact-input verification, not
a live qualification result.

The first live execution is preserved rather than discarded:
`g5-network-flows-live-first.log` is mode `0600`, 33,436 bytes, SHA-256
`c8cf853e522147b57cf72ef2c7b4d7a2fd900381e6e105b7d3827821d21cf2d5`,
exit 1, and credential-pattern clean. Before the failure it proves a clean
start and exact boot/release/binary/qualifier provenance; stable endpoint
`172.31.0.2/30`; exact tokenized TCP and UDP request/reply with primary
`10.148.0.58:18080/18081`; DNS through `169.254.169.254` returning two A and
two AAAA answers for `example.com`; and outbound HTTP to `172.66.147.243:80`
returning 200 and a 577-byte body with SHA-256
`25ddf2c883e0d1958ea971d279a7e4f0fd446724ee3db7db19dadabd4a62e484`.
The endpoint identity was unchanged, errors stayed zero, and normal release
returned rootfs/export/endpoint counts to zero. The sole failure occurs after
the deliberate `mkruntimed` restart, when the embedded final-resource audit
exits before its first observation. Readiness/audit state is being diagnosed;
the row remains open and no pass is claimed.

The retained follow-up
`g5-network-flows-first-post-failure-audit.log` ran 62 seconds later and is mode
`0600`, 16,298 bytes, SHA-256
`38f90c26b62d2d7d97ebdd5af18e652f8ca6eebae85de5477badaac80094e370`,
exit 0, and credential-pattern clean. Its exact-source trace proves the same
boot/release/daemon, Kerf with neither pool nor instances, all 19 resource
counters zero, and four active services with zero restarts. The daemon journal
shows only the intended stop/start plus successful storage-mount validation.
Thus the failed embedded audit was an after-restart readiness race, not leaked
state: systemd's active state preceded the Kerf clean response. The qualifier
now includes an up-to-60-second wait for both clean Kerf assertions, retains the
response, and invokes the audit only afterward. The revised file passes local
Bash syntax and diff checks and is 9,807 bytes/SHA-256
`cfbd71542d84d364d3c302279216635ff37b0fd1ba880fca83d2944360d98940`;
the guest independently reports the uploaded
`/tmp/test-runtime-network-flows-live-cfbd7154.sh` as mode `0755`, size 9,807
bytes, the same complete digest, and guest-side syntax-valid. Its execution
is retained as `g5-network-flows-live-second.log`: mode `0600`, 36,258 bytes,
SHA-256
`85bd5da562521fe66ad807ef1bfa182ae680d5ce1abe6e8a16cfee2a8e298af5`,
exit 1, credential-pattern clean. It independently repeats all TCP/UDP/DNS/HTTP
and stable-endpoint assertions, reaches zero rootfs/export/endpoint counts, and
obtains clean Kerf immediately after restart, yet the full audit immediately
afterward still exits before emitting an observation. The traced audit begun
21 seconds later passes in
`g5-network-flows-second-immediate-audit.log` (mode `0600`, 16,298 bytes,
SHA-256
`6372ebaff55876204cddddb20e0321c39f93e94a2f5e67ae6483a126af8f9e1f`,
exit 0, credential-pattern clean), proving all 19 counters zero and all services
active with zero restarts. The corrected conclusion is that clean Kerf is only
one readiness dimension; the qualifier must poll the complete exact-source
audit contract until it succeeds and retain that eventual output. The script
now does so for up to 60 seconds, requires the audit's terminal pass, and emits
the successful output. Local syntax/diff checks pass; this 9,991-byte revision
has SHA-256
`80e3327e3333fd71f9fe668534280e3021b723d63fa31ce34305cbf1492ea30c`;
the uploaded `/tmp/test-runtime-network-flows-live-80e3327e.sh` is independently
verified by the guest as mode `0755`, size 9,991 bytes, the same full digest,
and syntax-valid. Its third live execution passes in
`g5-network-flows-live-third.log`: mode `0600`, 42,937 bytes, SHA-256
`a380aaf75e60ac3c508acd32cff52e0de8ff37ab9ec20be5ec2ff95aff8c0f67`,
exit 0, credential-pattern clean, terminal `G5_NETWORK_FLOWS_LIVE_PASS`.
It retains exact ca7/boot/binary/qualifier provenance; the complete endpoint;
tokenized primary TCP and UDP exchanges; resolver plus two A/two AAAA answers;
HTTP 200 from `104.20.23.154:80` with a 577-byte body/SHA-256
`25ddf2c883e0…`; unchanged identity/zero errors; normal-release zero counts;
clean Kerf; and the exact-source 19-counter audit after three transient retries,
with all counters zero and all four services active/zero-restart. A separate
post-pass audit also passes as
`g5-network-flows-final-resource-audit.log`: mode `0600`, 16,298 bytes,
SHA-256
`40dc7f4ad2ad5e87abbe83011b3f605c965ba0d9ed247cc9073c83ab867d39a1`,
exit 0, credential-pattern clean, exact provenance, clean Kerf, all 19 counters
zero, and all services active with zero restarts. This closes the focused G5
flow row. The post-closure local gate passes documentation/links, all schemas
and current manifests, 97 OCI cases, read-only-bind/bootstrap/initramfs/storage/
mount/architecture/release/lifecycle/deployment/GCE-ledger/capture/containerd/
final-audit checks, qualifier syntax, and `git diff --check`.

Audit of the next G5 row finds partial but insufficient evidence. The shared
matrix retained distinct simultaneous `/30` endpoints and bidirectional sibling
`ping` failures, but its ctr and Docker IDs differ and it never asserts the same
internal hostname in both children. Its outbound marker also discards a
tokenized positive-route response for each sandbox. The new flow qualifier has
such a response for only one child. The row therefore remains open pending a
focused exact-current run with two simultaneous same-hostname sandboxes,
distinct endpoint identities, an exact allowed primary exchange from each,
bidirectional sibling rejection with command results, and the independent
19-counter cleanup audit.

The focused implementation is now
`test-runtime-network-isolation-live.sh`. It creates two simultaneous ctr
sandboxes with the explicit shared hostname, correlates two durable mknetd
identities with child-reported addresses, requires a distinct tokenized reply
from a primary-owned listener for each child, retains both directions of
sibling ICMP rejection, deletes normally, and waits for the complete
exact-source 19-counter audit. Local Bash syntax and diff checks pass. The
executable is mode `0755`, 9,547 bytes, SHA-256
`b49960be4e120b4f4ae3e2cd10c512396577491a08016e1e4b4c03c3cfedc173`;
the uploaded `/tmp/test-runtime-network-isolation-live-b49960be.sh` is verified
by the guest as mode `0755`, size 9,547 bytes, the same complete digest, and
syntax-valid. Its first live run passes in
`g5-network-isolation-live-first.log`: mode `0600`, 44,193 bytes, SHA-256
`de553bbb59a635efb71a5684572f7383b6a9b7a95224eaa2159e4435a929f02b`,
exit 0, credential-pattern clean, terminal
`G5_NETWORK_ISOLATION_LIVE_PASS`. The retained observations prove the same
internal hostname in both children; distinct `172.31.0.2/30` and
`172.31.0.6/30` endpoint/address/generation identities; separate tokenized
primary-route replies whose peer IPs match those children; bidirectional ping
exit 1 with 100% loss; clean normal release; and eventual exact-source Kerf/
19-counter/service audit success after three transient post-restart retries.
A separately invoked audit passes as
`g5-network-isolation-final-resource-audit.log`: mode `0600`, 16,298 bytes,
SHA-256
`a8bf3216fd63af84f9324529fe92fc05a34483f5dce30828987e4e973382d9c7`,
exit 0, credential-pattern clean, same exact release/boot/daemon, clean Kerf,
all 19 counters zero, and four active zero-restart services. This closes the
overlapping-name/distinct-identity/positive-route/default-isolation row. The
post-closure repository gate passes documentation/links, all schemas/current
manifests, 97 OCI cases, all focused runtime suites, qualifier syntax, and
`git diff --check`.

The broad G5 packet-stress row is now narrowed against current source. The
packet pump is byte-opaque and single-flight, yet its focused test covers only
one exact-MTU round trip, oversized ingress/egress drops, and two packets around
a disconnect/reconnect. It does not execute an ordered sequence, representative
fragment/checksum/malformed bytes, a burst or sustained loop, or a deliberately
slow receiver that fills the nonblocking egress queue. Those clauses stay open.
Focused socketpair coverage should establish the pump invariants first, then a
smaller disposable-VM run can exercise the real child/primary kernel path at
the negotiated MTU and under load.

The initial added test run passes byte-exact representative IPv4 fragment/
checksum/truncated-frame forwarding and a 256-packet ordered burst with exact
sequence and counters. The slow-reader subcase stops at its attempted
`SO_RCVBUF` reduction because the local sandbox returns `EPERM`; existing
exact-MTU, oversized-direction, and disconnect/reconnect subcases also pass.
No backpressure success is claimed from that run. The socket buffer mutation is
not required: the retry will fill the default receive queue while withholding
reads, preserving the intended nonblocking-drop assertion without the denied
operation.

After removing only the denied buffer-size mutation, the complete focused run
passes in 0.513 seconds. The new test retains three representative packet
shapes byte-for-byte, including fragment/checksum fields and a truncated frame;
preserves the exact order/content of 256 burst packets with exact zero-drop
counters; fills the default unread receive queue until the shim records a
nonblocking TX drop; then drains and forwards an exact recovery marker without
a fatal error. The older exact-MTU/oversize/disconnect test passes alongside
it. Repeated race execution and exact-source disposable-VM execution are still
required before these clauses can support row closure. The combined old/new
pump selection now passes 20 race-detector repetitions in 10.905 seconds;
exact-source VM and live kernel-stack evidence remain pending. The full local
documentation/schema/manifest/runtime gate and diff check remain green. Commit
`e4d7c9c0c7b66f5a5b8cb8e574c7ca80a26e87a6` freezes the source tests; its
deterministic 1,330,103-byte archive hashes to
`3ba8b51764f5fc458a98059d8324cc214d5a362a0b6abf0157f7688304c28ee6`.
The guest independently matches archive size/digest, extracts 655 regular files
into unique `/var/tmp/mksrc-e4d7c9c`, and reports the same local/guest SHA-256
`0a545d38811f…` for the changed shim test file. Exact guest execution remains
captured in `g5-network-pump-stress-source-live.log`: mode `0600`, 31,692
bytes, SHA-256
`9a8f6f401ff2f4c1be95f1c29685108a041c2961cb67bea871c3023719583d30`,
exit 0, credential-pattern clean. On the disposable VM all seven named subcases
pass in each of 20 race-detector repetitions (10.552 seconds), covering exact
MTU, both oversize directions, disconnect/loss/reconnect, opaque fragment/
checksum/malformed bytes, 256-packet order/load, and slow-reader bounded drop
plus recovery. The source pump slice is now evidenced; the composite row stays
open for real negotiated-MTU and load traffic through the child/primary stack.

The new live-side implementation is
`test-runtime-network-stress-live.sh`. A real child must pass exact 1,400-byte
IPv4 ICMP, 3,000-byte fragmented ICMP reassembly, 256 packets at 10ms intervals
without loss/duplicates, and a one-MiB zero stream to a primary TCP server that
delays reads for one second and verifies byte count/digest. The qualifier also
requires stable endpoint identity, increased durable RX/TX counters without
new drops/errors, normal release, and the exact 19-counter audit. Local Bash
syntax and diff checks pass; it is mode `0755`, 9,551 bytes, SHA-256
`336e15ecd63c32cd1bd8984b5f81ebd89e81119449b31d566b5eef1554a137ca`.
The guest independently reports uploaded
`/tmp/test-runtime-network-stress-live-336e15ec.sh` with identical mode, size,
digest, and valid syntax. The first live run passes in
`g5-network-stress-live-first.log`: mode `0600`, 100,518 bytes, SHA-256
`31d18fcaae2ec20c2d539ae40022abfa6ee16aa6a7f7a905a19b6dc6cbf18e2e`,
exit 0, credential-pattern clean, terminal `G5_NETWORK_STRESS_LIVE_PASS`.
It retains 5/5 exact-1,400-byte ICMP, 3/3 3,000-byte fragmented/reassembled
ICMP, a 256/256 zero-loss/no-duplicate 10ms burst, and exactly 1,048,576 zero
bytes delivered through the one-second slow primary reader with matching
SHA-256 `30e14955ebf1…` and reply. Endpoint identity stays stable while counters
advance RX/TX 0/0→768/1,024 without drops/errors. Normal cleanup and the
eventually settled 19-counter audit pass. A separately invoked post-pass audit
also passes as `g5-network-stress-final-resource-audit.log`: mode `0600`,
15,686 bytes, SHA-256
`3f2c735794049ca274154de4305f69776e388121dd703b79771ff410ec11f27c`,
exit 0, credential-pattern clean, exact provenance, clean Kerf, all 19 counters
zero, and four active zero-restart services. Together with the exact-source VM
race suite for checksum/malformed/oversized opacity, disconnect loss/reconnect,
ordering, burst, and backpressure recovery, this closes the composite G5
MTU/fragment/load/slow-reader row. The post-closure repository gate passes
docs/links, schemas/current manifests, 97 OCI cases, all focused runtime suites,
qualifier syntax, and `git diff --check`.

The next open-row audit was recorded before another VM mutation. Retained
`g6-mkruntimed-restart-continuity-v2-pass.log` proves a changed mkruntimed PID
with the same live task, namespace-holder PID, child boot ID, durable recovery
record, attach stream, exec path, and events. Retained
`g6-forced-shim-reconnect-fifo-pass.log` proves shim-worker and namespace-holder
replacement with unchanged child boot identity, continuous attach, post-fault
exec, and normal teardown. These substantiate the daemon and shim-death
boundaries, but neither transcript surrounds the fault with a network exchange.
The newer packet-pump VM source run proves injected disconnect detection and
authenticated reconnect retry only inside the pump. There is still no focused
live proof of packet continuity across mknetd restart, relay/agent transport
process death, child replacement, or restart of the primary-side network
service. The G5 restart/reconnect row therefore remains open for those cases
and for packet assertions around the already-proved mkruntimed/shim cases.
Earlier reboot evidence is deliberately excluded: it exposed volatile bundle
and durable-orphan reconciliation failures rather than continuity.

The disposable VM is reachable and clean before the new restart work. GCE
reports `mklinux-g4-g6-final-20260905` `RUNNING`, start timestamp
`2026-10-05T17:58:35.542-07:00`, and the guest reports unchanged boot ID
`f93f21b6-21f1-4fc0-aebc-806347d6b43d`. mkruntimed, mknetd, containerd, and
Docker are active. Both containerd task namespaces, Docker containers,
Multikernel children, and mknetd endpoints are empty; anchored shim, relay, and
NBD workload processes are absent. This only establishes a safe preflight and
does not close any restart behavior.

`test-runtime-network-restart-live.sh` now implements the safe live portion of
the open matrix. It brackets mknetd restart, mkruntimed restart, and forced
shim-worker reconstruction with token-exact child-to-primary TCP exchanges,
requiring the same child boot and exact endpoint ownership/generations while
the expected daemon or worker/holder PID changes. It then normally deletes and
recreates the same task name, requires new child boot, sandbox generation, and
endpoint generation, and performs another exchange before exact cleanup and
the 19-counter audit. Bash syntax, ShellCheck when present, and diff checking
pass. The mode-0755, 10,568-byte script has SHA-256
`c4a3bb672fd5166ff5b7e5c4630ebfb3af079bf77162d68f3cae40ceb64d89ec`.
Direct relay-process replacement and primary-host reboot are intentionally not
claimed by this first harness.

The guest independently matches the uploaded qualifier's exact 10,568-byte
size and full SHA-256
`c4a3bb672fd5166ff5b7e5c4630ebfb3af079bf77162d68f3cae40ceb64d89ec`;
guest `bash -n` prints `GUEST_UPLOAD_VERIFY_PASS`. Its `stat` display contains
an operator-quoting artifact (`\755 ...'`), so the mode claim rests on the
successful `chmod 0755` plus direct execution rather than rewriting that raw
output. This is transfer/syntax proof only; the live run follows.

The first live attempt is preserved as
`g5-network-restart-live-first.log` (mode `0600`, 11,639 bytes, SHA-256
`a438da83021fe6af92b40012ebe936dbd5e79f106512a792f1402135898c4b3c`,
exit 1, credential-pattern clean). Clean preflight, provenance, child start,
and one rootfs/live-export/endpoint pass, but the first boot-ID exec is rejected
before any restart fault: `cat` is not an absolute guest executable and yields
`INVALID_ARGUMENT: invalid agent request`. The trap normally stops/removes the
task. All five sites now use `/bin/cat`; local syntax, ShellCheck when present,
and diff checks pass. Corrected mode/size/digest are `0755`, 10,593 bytes, and
`4f18aa1dea413825a9119691e546195ddb3c09e68eaea414dbe51e2035b2edf5`.

The immediate independent audit is retained rather than hidden:
`g5-network-restart-first-post-failure-audit.log` is mode `0600`, 406 bytes,
SHA-256 `391a4928b529361fbf9aad3f55c59ef3a07c4a43ed50e881923d508186ab6631`,
exit 1, credential-pattern clean. It stops at the still-configured idle-pool
preflight. Direct inventory proves no workload residue: all 16 GiB of the pool
is available, no Kerf instance exists, ctr tasks/containers, children, rootfs
records, and endpoints are zero, and all four services report success with zero
restarts. The 24 storage entries are historical export records rather than
live exports. The pool still requires an idle daemon restart before retry, and
no matrix member is claimed from this attempt.

Post-failure recovery is captured separately in
`g5-network-restart-first-recovered-audit.log`: mode `0600`, 2,266 bytes,
SHA-256 `0cd9d87e2bceeb14ec12f59057e2170ad6175ac123ebf6b0ff0003d3225e536f`,
exit 0, credential-pattern clean. An idle mkruntimed restart changes PID
44448→46406 and releases the pool. The independent auditor then reports clean
Kerf, all 19 resource counters zero, four active zero-restart services, and
terminal `G6_FINAL_RESOURCE_RETURN_PASS`; retry is safe from this recorded
baseline.

The corrected guest upload at
`/tmp/test-runtime-network-restart-live-4f18aa1d.sh` independently matches mode
`0755`, size 10,593, and full SHA-256
`4f18aa1dea413825a9119691e546195ddb3c09e68eaea414dbe51e2035b2edf5`;
guest syntax ends `GUEST_CORRECTED_VERIFY_PASS`. Behavioral retry remains a
separate result.

The second transcript is retained as
`g5-network-restart-live-second.log`: mode `0600`, 94,013 bytes, SHA-256
`0681e7151e11b6e1b80d28a2bc1f311d5081450544f84cf78d23fedae508245b`,
exit 1, credential-pattern clean. All live behavior succeeds before its final
audit. Five token-exact primary TCP observations see child peer `172.31.0.2`.
Mknetd PID changes 8326→47224, mkruntimed 46406→47323, and forced shim recovery
changes worker/holder 46897/47096→47426/47450 beneath unchanged supervisor
46892. Child boot `fb326a9b…`, address `172.31.0.2/30`, sandbox generation
`a904f85c…`, endpoint generation `afe9d4df…`, and zero drops/errors stay stable
through those continuity boundaries. After exact zero-count cleanup, same-name
replacement changes boot to `560753c9…`, sandbox generation to `2f731ec8…`,
and endpoint generation to `26889f19…`; its packet exchange and cleanup pass.
The only failure is sequencing: the strict auditor runs before the empty Kerf
pool is released and correctly refuses it for all 60 retries. These behaviors
are evidence, but this is not a closing transcript.

The qualifier now restarts idle mkruntimed after zero workload counts and
records its before/after PID before invoking the final auditor. Revised local
syntax, ShellCheck when present, and diff checks pass; mode/size/SHA-256 are
`0755`, 10,994 bytes, and
`f2f782a9b8c397e72dfc7e544bd45c64e05fa29e493cfdc6d9789e2ded38a855`.

The second post-attempt recovery independently passes as
`g5-network-restart-second-recovered-audit.log`: mode `0600`, 2,267 bytes,
SHA-256 `4d32cdbf0934c1b8240cafe6b183618722af2dbc2fd4da7ee2680d71e84a273d`,
exit 0, credential-pattern clean. Idle mkruntimed changes PID 47323→48889;
the auditor reports clean Kerf, all 19 counters zero, four active zero-restart
services, and `G6_FINAL_RESOURCE_RETURN_PASS`. The next retry has a proved
empty baseline.

Final guest transfer independently matches
`/tmp/test-runtime-network-restart-live-f2f782a9.sh` at mode `0755`, size
10,994, and full SHA-256
`f2f782a9b8c397e72dfc7e544bd45c64e05fa29e493cfdc6d9789e2ded38a855`;
guest syntax ends `GUEST_FINAL_VERIFY_PASS`. No behavioral result is inferred
from transfer verification.

The final corrected run passes as `g5-network-restart-live-third.log`: mode
`0600`, 76,778 bytes, SHA-256
`6bdd7e54a685385ef9612e9f714c9252992166c54c1b6abf73797b8261976d66`,
exit 0, credential-pattern clean, terminal
`G5_NETWORK_RESTART_LIVE_PASS`. Five exact child-to-primary exchanges are seen
from `172.31.0.2`: baseline, after mknetd 47224→49723, after mkruntimed
48889→49823, after shim worker/holder 49394/49594→49925/49949 under unchanged
supervisor 49389, and after same-name child replacement. Continuity retains
boot `1a6dc772…`, `172.31.0.2/30`, sandbox generation `b07bd996…`, endpoint
generation `7484d028…`, and zero drops/errors. Replacement changes boot to
`2a3e36c7…`, sandbox generation to `e31dd74d…`, and endpoint generation to
`1b5e9d2b…`; both generations clean to zero. Idle release changes mkruntimed
49823→50524 and the embedded strict audit passes clean Kerf, all 19 counters
zero, and four active zero-restart services. Thus mknetd, mkruntimed,
shim-worker, and child-replacement packet behavior is now live-proved. Direct
relay/agent transport-process replacement and primary-host restart remain open.

A separately invoked audit independently confirms the post-pass state:
`g5-network-restart-final-resource-audit.log` is mode `0600`, 1,910 bytes,
SHA-256 `f4e9c07596293bad835fd2a198efe0010709fee6fb92fbcee7700d49cebf42f3`,
exit 0, credential-pattern clean, with exact provenance, clean Kerf, all 19
counters zero, and four active zero-restart services. No tracked residue is
being hidden by the qualifier.

The next evidence strengthening is recorded before running it. Although shim
reconstruction necessarily replaced the worker-owned relay, the passing log
did not record relay identities. The qualifier now resolves exactly one direct
worker child named `mkvsock-relay` or `mk-agent-relay`, requires the old relay
dead and the replacement PID different before post-fault traffic, and records
both. It also records and requires a distinct primary TCP listener PID for each
of the five exchanges. Local syntax, ShellCheck when present, and diff checks
pass; the revised file is mode `0755`, 11,867 bytes, SHA-256
`2e998c178597319ea058044ee3ac23dcfa136d8b8b3e7722b3f220c88dbe96d6`.
No new live claim follows until this exact file runs.

The guest independently matches relay-hardened upload
`/tmp/test-runtime-network-restart-live-2e998c17.sh` at mode `0755`, size
11,867, and full SHA-256
`2e998c178597319ea058044ee3ac23dcfa136d8b8b3e7722b3f220c88dbe96d6`;
guest syntax ends `GUEST_RELAY_VERIFY_PASS`. This is transfer proof only.

The relay-identity replay is preserved as
`g5-network-restart-live-relay.log`: mode `0600`, 9,884 bytes, SHA-256
`40ed0cbd71c02eecdddc5630dccbc65f62c8bb8d833b4ff0d228d034782d0b62`,
exit 1, credential-pattern clean. Clean preflight and provenance pass, but
initial child Create returns `BACKEND_FAILURE` before relay discovery or any
fault. It provides no new restart evidence and does not replace the passing
`third` run. The bounded containerd journal records shim connection at
12:37:00Z, then disconnected-shim cleanup at 12:38:23Z; fallback cleanup cannot
execute absent public pathname
`/usr/local/bin/containerd-shim-multikernel-v2`. Direct inventory is clean:
Kerf has no pool/instance and tasks, containers, children, rootfs records, live
exports, and endpoints are zero; all 28 storage records are released history.
Following the instruction to skip the latest failed attempt, this replay is not
retried now. The public-shim-path issue remains explicit for later provenance
work while qualification moves to another row.

After the user's instance restart, GCE reports `RUNNING` with start timestamp
`2026-10-06T17:59:57.528-07:00`; the guest has new boot ID
`cc677283-6e80-4bef-a34d-aac8af61566a`. Exact installed selector
`0.1.0-dev-ca7d7d0…` and `/var/tmp/mksrc-e4d7c9c` survive. All four services
are active/running at PIDs 1446/1226/1468/1521, result success, zero restarts.
Both containerd namespaces, Docker, Kerf instances/pool, rootfs, endpoints, and
live exports are empty; 28 released export histories remain. The previously
missing public shim path is again the expected symlink into the selected
release, with an executable 14,034,425-byte target. This confines the prior
absence to the old boot but does not explain or pass that failed replay. The
new boot is a clean CNI qualification baseline.

CNI preflight finds `/opt/cni/bin/multikernel` linked to the selected-release
`mk-cni`; link and target share SHA-256
`d8dbaa8018554117a8eacd271d8eb8fb91a0b257ba594fa33b75de221bd201d5`,
and the target is mode 0755/4,791,080 bytes. The expected CNI config is not
installed under `/etc/cni/net.d`, while cache, named namespaces, `mkv*` and
`mkhost*` links, MK chains, and 172.31 NAT rules are empty. A focused qualifier
will provide and retain its exact config/private cache without claiming CRI
configuration, exercising the installed binary and live mknetd API only.

`test-runtime-cni-faults-live.sh` now implements the missing replacement-host
matrix without modifying CRI configuration. It supplies a private exact CNI
1.0 config/cache, executes seven exact-source rollback/reconcile/reuse tests 20
times with the race detector, and performs a real partial `ADD`: host veth
creation precedes a missing-netns move failure which must roll back completely.
It then exercises successful external-netns `ADD`, two `CHECK`s, deleting the
namespace before `DEL`, repeated `DEL`, and same-name reuse with the same
released address but a new generation. Each call retains config, env/argv,
stdout/stderr/status; live snapshots retain endpoint/cache and host/namespace
links, addresses, routes, filter/NAT rules, followed by the strict audit. Local
syntax, ShellCheck when present, and diff checks pass. The mode-0755,
8,369-byte qualifier hashes to
`4b6ce3783cdc947ffd3e515a4fcead1a3a61ce34efc630f8252db838d7ce894f`.
This is implementation evidence only until the exact file runs on the VM.

The guest independently reports the uploaded
`/tmp/test-runtime-cni-faults-live-4b6ce378.sh` at mode `0755`, size 8,369,
and full SHA-256
`4b6ce3783cdc947ffd3e515a4fcead1a3a61ce34efc630f8252db838d7ce894f`;
guest syntax prints `GUEST_CNI_VERIFY_PASS`. No live behavior is claimed from
transfer verification.

First CNI execution is retained as `g5-cni-faults-live-first.log`: mode `0600`,
30,713 bytes, SHA-256
`204da70e89c8c406dde6c4a75e07e18e3495b5df944252619a0ec6eca1299d54`,
exit 1, credential-pattern clean. Exact provenance/config pass, followed by 20
race-detector repetitions in which all seven named source cases pass: every
Linux ADD command boundary, allocation/final-persist rollback, allocating and
deleting reconciliation, repeated CHECK/DEL/name reuse, and quarantine
recovery. Before any live CNI call, Bash rejects expansion of `label` within
its own `local` declaration under `set -u`; the trap encounters the same issue.
Inspection proves only exact scratch `/var/tmp/mk-cni-faults.SCB9OC`, its
142-byte mode-0600 config, and an empty root-owned mode-0700 cache remain;
endpoints, test namespaces, links, MK rules, and 172.31 NAT rules are zero.
That verified directory is removed exactly and absence confirmed. Splitting
the declaration fixes the harness; syntax, ShellCheck when present, and diff
checks pass. Corrected mode/size/hash are `0755`, 8,376 bytes, and
`6d3372be3f421d28a1e919aa1b359d7a169526f6a00f2828207f11bfafb5b289`.
This closes source fault evidence only; live CNI behavior remains pending.

The guest independently verifies corrected upload
`/tmp/test-runtime-cni-faults-live-6d3372be.sh` at mode `0755`, size 8,376,
and full SHA-256
`6d3372be3f421d28a1e919aa1b359d7a169526f6a00f2828207f11bfafb5b289`;
syntax ends `GUEST_CNI_CORRECTED_VERIFY_PASS`. Live retry is separate.

The second CNI attempt is retained rather than overwritten as
`g5-cni-faults-live-second.log`: mode `0600`, 42,436 bytes, SHA-256
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

Independent post-failure audit `g5-cni-faults-second-post-failure-audit.log`
passes clean Kerf, all 19 counters zero, and four active zero-restart services:
mode `0600`, 1,907 bytes, SHA-256
`3008c6557765e177357da9f113d95ca5d7d1e73ed7cc3bfe56d0c52395319701`,
exit 0, credential-pattern clean. The evidence parser now interprets an
omitted `managed_namespace` as false, matching the schema's `omitempty`
encoding. Local syntax and diff checking pass; ShellCheck is unavailable. The
corrected qualifier is mode `0755`, 8,386 bytes, SHA-256
`7eda9ea25bf25bfb3a1098328efe96055ecebf9b4d752f71bf1479cd8a9d5721`.
Product behavior is unchanged; the complete live matrix remains pending.

Guest upload `/tmp/test-runtime-cni-faults-live-7eda9ea2.sh` independently
matches mode `0755`, size 8,386, and full SHA-256
`7eda9ea25bf25bfb3a1098328efe96055ecebf9b4d752f71bf1479cd8a9d5721`;
guest syntax ends `GUEST_CNI_FINAL_VERIFY_PASS`. This is transfer proof only.

`g5-cni-faults-live-third.log` is a preflight invocation error: the caller
omitted required `SOURCE_ROOT`, so the script exits before scratch creation or
any product operation. The retained mode-`0600`, 492-byte transcript has
SHA-256 `9f3ffb468c21ebf9c2a80db60d49723e2ba780e29a2cba0b10bcea7f8aad0732`,
exit 1, and an empty credential-pattern scan. It supplies no product claim;
the corrected invocation passes `/var/tmp/mksrc-e4d7c9c` explicitly.

Correct invocation passes as `g5-cni-faults-live-fourth.log`: mode `0600`,
105,747 bytes, SHA-256
`ef37e21c88e3b902d920b76290e45c06fbfc73b04d2534d4aee680dd6a6edccf`,
exit 0, credential-pattern clean, terminal `G5_CNI_FAULTS_LIVE_PASS`. All seven
exact-source cases pass 20 race-detector repetitions, including every Linux
ADD command boundary and allocating/deleting reconciliation. The installed
plugin's missing-netns ADD returns CNI code 100 and zeroes all seven focused
inventories. Generation `826bedc1…` then reaches READY at MTU 1400 and address
`172.31.0.2/30`; two CHECKs return empty stdout/stderr and exit 0. After the
namespace is deliberately removed, DEL and repeated DEL both return empty
stdout/stderr and exit 0, and all focused inventories are zero. Same-name reuse
obtains the same address in distinct READY generation `ee11cff6…`, its
CHECK/DEL pass, and cleanup is again all zero. The embedded strict audit
reports clean Kerf, all 19 counters zero, and four active zero-restart
services. This live execution plus exact-source boundary tests closes the CNI
partial-failure/repetition/stale-cleanup/reuse row.

Separately invoked `g5-cni-faults-final-resource-audit.log` independently
confirms the pass did not hide trap residue: mode `0600`, 1,907 bytes, SHA-256
`9998bad4799aec59f4fb1c8ec0e0fe138f8020fa3596a7d2ec53d138640a60bc`,
exit 0, credential-pattern clean. It reports the same exact selector/boot,
clean Kerf, all 19 counters zero, and four active zero-restart services.

Commit `561880f2e7b2a7972596a2f9f76edfae81dc06ee` (`test: qualify G5 CNI
fault recovery`) checkpoints the CNI qualifier, relay-identity evidence
hardening, and both continuously updated learning records. At this checkpoint
the checklist has 58 closed and 27 open rows.

The already-passing two-sandbox qualifier now targets the remaining live
source-spoof/route-injection/metadata/forwarding/sibling-bypass boundaries
without changing product policy. For both distinct children it records
per-generation iptables counters before/after; rejects HTTP to the metadata
address while leaving the already-proved DNS exception intact; installs
explicit sibling `/32` routes through each legitimate gateway and requires
reciprocal ping failure; adds distinct unallocated source `/32`s and attempts
source-bound TCP tokens to a primary listener, requiring both client failure
and a five-second no-accept observation. The original same-hostname/distinct-
identity, legitimate token routes, default sibling rejection, normal cleanup,
and strict audit remain unchanged. Local syntax and diff checks pass;
ShellCheck is unavailable. Revised mode/size/SHA-256 are `0755`, 13,331 bytes,
and `069e6d6c4b9aff5d806cb28a9437f3439c3f670a1400d75346a39c3f9211a917`.
This is implementation evidence only; the row remains open until the exact
file runs on the disposable VM.

Guest upload `/tmp/test-runtime-network-isolation-live-069e6d6c.sh`
independently matches mode `0755`, size 13,331, full SHA-256
`069e6d6c4b9aff5d806cb28a9437f3439c3f670a1400d75346a39c3f9211a917`,
and syntax marker `GUEST_NETWORK_POLICY_VERIFY_PASS`.

First policy replay is retained as
`g5-network-policy-bypass-live-first.log`: mode `0600`, 45,630 bytes, SHA-256
`15b95d337b94a473e10ac97f6d1f869f8e76e2ec53e621634f9ca38901c23c93`,
exit 2, credential-pattern clean. Exact provenance, distinct identities,
positive primary routes, default sibling rejection, and both metadata HTTP
rejections pass. The first explicit route mutation is rejected inside the
default OCI process with `RTNETLINK ... Operation not permitted`, before a
bypass packet is sent; this is a real least-privilege barrier but not proof of
the primary firewall under a privileged workload. Trap cleanup converges.
Post-failure audit plus CLI capability inspection is mode `0600`, 2,452 bytes,
SHA-256 `15e5b96a0157abafd38c6f026714f1d9e80391d71cc22abe847eac5f5bd44331`,
exit 0, credential-pattern clean: clean Kerf, all 19 counters zero, four healthy
services, and `ctr run` explicitly supports `--cap-add`. The retry grants only
`CAP_NET_ADMIN` to both disposable test workloads so mutations reach the
primary policy; it does not alter host or product configuration.
The capability-scoped qualifier passes local syntax/diff checks and is mode
`0755`, 13,379 bytes, SHA-256
`7104931bc239b3b9707f9e1b69a566548fd70427087161e94ef98d57f16a2b44`.

Guest upload `/tmp/test-runtime-network-isolation-live-7104931b.sh` matches mode
`0755`, size 13,379, full SHA-256
`7104931bc239b3b9707f9e1b69a566548fd70427087161e94ef98d57f16a2b44`,
and syntax marker `GUEST_NETWORK_POLICY_CAP_VERIFY_PASS`.

Capability-scoped second run passes as
`g5-network-policy-bypass-live-second.log`: mode `0600`, 77,112 bytes, SHA-256
`4df3969bbd7128b8700349ddc40635757a3dbfb12fdcca991c289f61c62812f4`,
exit 0, credential-pattern clean, terminal
`G5_NETWORK_ISOLATION_LIVE_PASS`. Both privileged guests successfully install
explicit sibling `/32` routes and distinct `198.18.0.1/32`/`.2/32` source
aliases. Reciprocal routed pings still lose 100%; each chain's sibling-drop
counter advances from one to two packets. Both metadata HTTP requests fail with
connection refused and each metadata REJECT counter advances 0→1. Both
source-bound TCP attempts exit 1 and the primary listener records
`{"accepted": false}`. Their primary anti-spoof chain counters remain zero,
showing those spoofed flows were rejected before that host-chain rule rather
than attributing an unobserved hit to it; exact-source tests separately prove
the `/32` rule installation/CHECK contract. Legitimate token routes from both
assigned sources still pass first. Cleanup reaches zero endpoints/roots/live
exports and the embedded strict audit passes all 19 counters after bounded pool
release. Independent `g5-network-policy-final-resource-audit.log` is mode
`0600`, 1,909 bytes, SHA-256
`6673daf0806a70e4722e2ffb4e5472ac30fef0ce3dea316d5d895df03b2c8c3b`,
exit 0, credential-pattern clean, again proving clean Kerf, all counters zero,
and four active zero-restart services. This closes the policy-bypass row
without claiming an anti-spoof counter hit that was not observed.

Commit `cd3e5c9ef6ff26229cdff122d1e4c8a15248dfef` (`test: qualify G5 policy
bypass rejection`) checkpoints the expanded qualifier and evidence narrative.
The checklist now has 60 closed and 25 open rows.

Evidence cross-audit closes the G5 response-detail row without a redundant VM
run. Retained flow transcript `a380aaf7…` contains tokenized TCP and UDP
request/reply pairs observed at primary `10.148.0.58`, the configured resolver
and returned A/AAAA answers, plus external HTTP details. Retained isolation
transcript `de553bbb…` contains both child identities and complete A→B/B→A ping
output with exit 1, one transmitted, zero received, and 100% loss. The newer
policy transcript `4df3969b…` independently repeats legitimate token routes and
both default and injected-route sibling failures. These are response-bearing
and raw observations, not summary markers.

Current-boot primary-health preflight identifies the exact stable objects the
remaining three-phase check must bind: active `ssh.service` and
`google-guest-agent.service`; two port-22 listeners; default route `via
10.148.0.1 dev ens4 ... src 10.148.0.58`; NIC `ens4` at PCI
`0000:00:04.0/virtio1` bound to `virtio_net`; root `/dev/sda1`, base disk
`sda`, beneath PCI `0000:00:03.0/virtio0/...` bound to the SCSI `sd` driver. A
primary metadata request returns HTTP 200, 19 bytes, and `Metadata-Flavor:
Google`. The ad-hoc probe's shell-escaped SHA extraction fails and is not
claimed; the qualifier will use direct file hashing and compare a normalized
snapshot before, while two children are active, and after cleanup.

The qualifier now implements that normalized snapshot with direct metadata
body SHA-256, response status/flavor/size, the live SSH session plus listener
count, guest-agent PID/executable, exact default route, NIC device/driver, and
root-source/base-disk device/driver. It requires byte-identical snapshots
before task creation, after all privileged policy attempts with two children
live, and after normal cleanup plus strict audit. Local syntax/diff checks pass;
mode/size/SHA-256 are `0755`, 15,560 bytes, and
`7cecd2cc5879ebfe4c41ad9c81cf0eadef9628522d19aefa28cd3d3e273f7520`.
Live execution remains required.

Guest upload `/tmp/test-runtime-network-isolation-live-7cecd2cc.sh`
independently matches mode `0755`, size 15,560, full SHA-256
`7cecd2cc5879ebfe4c41ad9c81cf0eadef9628522d19aefa28cd3d3e273f7520`,
and syntax marker `GUEST_PRIMARY_HEALTH_VERIFY_PASS`.

First three-phase attempt `g5-primary-health-live-first.log` is a
pre-observation harness failure: mode `0600`, 13,524 bytes, SHA-256
`1be67cf4d477fe5b09e8c4df0d8932e50dd4bfe3b707a90c2a82feaa58e4e27d`,
exit 1, credential-pattern clean. Exact provenance and zero-resource preflight
pass; Bash then expands `phase` inside its own `local` declaration under
`set -u`, before metadata access or task creation. The trap confirms both task
identities absent. Splitting the declaration fixes only the qualifier; no
product claim follows from this attempt.
Corrected mode/size/SHA-256 are `0755`, 15,575 bytes, and
`4e08b01af8f49af0cfb74610bfb3725a95ad4f8972b9bb3681f46fc5a9834923`;
local syntax and diff checks pass.

Guest upload `/tmp/test-runtime-network-isolation-live-4e08b01a.sh` matches mode
`0755`, size 15,575, that full digest, and syntax marker
`GUEST_PRIMARY_HEALTH_CORRECTED_VERIFY_PASS`. Corrected run passes as
`g5-primary-health-live-second.log`: mode `0600`, 103,165 bytes, SHA-256
`47db93520a48e0c71098b6f13342872289b73632e5980df05e8ee6be8939129d`,
exit 0, credential-pattern clean. Before, during two active privileged children
after every policy attempt, and after cleanup, the normalized snapshot is
byte-identical: current SSH connection/two listeners, guest-agent PID 1079,
primary metadata 200/Google/19-byte body SHA `2887acc3…`, exact default route,
NIC PCI/virtio path and driver, and root-disk controller path and driver.
Independent final audit is mode `0600`, 1,909 bytes, SHA-256
`425dd620b036de24dcb5086bad0ea79b70a4ae68231c0a41e34bdec25864d638`,
exit 0, credential-pattern clean, with all 19 counters zero. This closes the
three-phase host-health/controller-ownership row.

The passing transcript retains pre/during/post ancestry. The first dedicated
final-network wrapper is a quoting failure before observation (mode `0600`,
1,985 bytes, SHA `37e58a96…`, exit 1) and is not evidence. Shell-safe retry
`g5-primary-health-final-network-inventory-second.log` passes at mode `0600`,
2,303 bytes, SHA-256
`4d88750064f63370f1897bff1c4dc22790a6333af54c26501be7a22ad59cd67b`,
exit 0, credential-pattern clean: namespaces, managed/TUN/TAP links, routes,
iptables filter/NAT and nftables matches, shim/relay/NBD processes, endpoints,
and rootfs records are all zero; NIC/root controller identities remain exact.

Commit `dadb104f2df04878d4d675ce51276ba9eb7bfca0` (`test: qualify G5 primary
host continuity`) checkpoints the qualifier and both evidence narratives. The
checklist now has 63 closed and 22 open rows.

The operator-restarted disposable instance is reachable again. GCE identifies
`mklinux-g4-g6-final-20260905` as resource `6701540373796488780`, `RUNNING`,
last started `2026-10-07T06:21:49.238-07:00`, with internal/external addresses
`10.148.0.58`/`34.126.166.142`. The guest now has boot ID
`8ef29982-c227-45b8-83a6-08b7af3c419b`, kernel `7.0.0-mk2-gce-lab`, and the
selected `ca7d7d013485…` release. Initial post-restart sampling found active,
zero-restart mkruntimed/mknetd/containerd/Google guest agent at PIDs
1452/1235/1464/1083. Docker had PID 1533 but remained `activating`; both ctr
tasks and running Docker containers were zero. This deliberately records only
the restart baseline. It does not promote Docker readiness or any G6 claim
until the follow-up health and strict-resource observations pass.

The immediate readiness follow-up proves Docker reached `active/running` at
the same PID 1533. All four runtime services are therefore running with
unchanged PIDs and zero systemd restarts; both the durable storage-file listing
and Kerf inventory are empty. Its last exact-process command is deliberately
excluded: a double-quoted remote `awk $1` was expanded by the outer `set -u`
shell and aborted only that subcheck. The strict audit retry must use a
shell-safe process matcher before any workload claim.

The strict retry passes and is retained before further mutation. The local and
VM audit scripts share full SHA-256
`7010e3dae0e4fd6faafa0ff5c405d9874e05e02f6bad021bff83450ef5521821`.
`20261007-post-restart-final-resource-audit.log` is mode 0600, 13,364 bytes,
SHA-256 `585449523ecb3548f2dd1343c6d7a82cfeefb3ac0b60a8ed7979823ffa452f6e`,
exit 0, and credential-pattern clean. It records the new boot, exact selected
release and daemon executable hash, empty Kerf state, every one of the 19
resource counters at zero, and all four services active/running with zero
restarts. This is the clean pre-mutation boundary for continued G6 work.

The OCI capability audit is now bound to exact restarted-VM source. Local and
guest digests match for the validator `ddd77fb8…`, test matrix `181bd6ae…`,
live pre-allocation harness `46000f5f…`, agent server `8afdf699…`, and shim
`a1349318…`. The implementation/test map explicitly covers hooks, seccomp,
namespace, mount, resource/cgroup, rlimit, capability, read-only-root,
hostname, protected-path, and pathname policy, while capability negotiation
requires the same OCI feature set advertised by the agent. Retained
`20261007-g6-oci-capability-matrix-vm.log` is mode 0600, 510 bytes, SHA-256
`019608e701d60056ae1320532e292da83b32e0fe396b6dcbb1197efe7a285daf`,
exit 0, and credential-pattern clean. It reports 97 semantic cases plus the
namespace, file-identity, and outer-cleanup boundaries. This is exact-source
evidence; installed-service rejection and cleanup are still intentionally
unclaimed until the live harness completes.

Installed OCI behavior now passes on that exact source and selected release.
`20261007-g6-oci-preallocation-live-pass.log` is mode 0600, 133,211 bytes,
SHA-256 `379a03bc23aadfc9a9f96f5d8ef3be90ecb16ed3cbcf15e709b0cf94935e8aa0`,
exit 0, and credential-pattern clean. It records the exact identity tuple,
rejects unsupported AppArmor with status 1 at the pre-allocation validator,
shows no Kerf allocation or durable/runtime/network resources immediately
afterward apart from two bounded transient shim wrappers, then reaches the
complete zero inventory. The supported control prints
`MK_OCI_SUPPORTED_PASS`; pool release again reaches all 19 zeros and four
healthy zero-restart services. Independent
`20261007-g6-oci-final-resource-audit.log` is mode 0600, 13,364 bytes, SHA-256
`9e6005722e3e9e207302aa3602329636da600e9692c7ce41ae0963a77a30c475`,
exit 0, credential-pattern clean, and independently confirms empty Kerf plus
all counters zero. The 97-case VM matrix and these installed observations close
the OCI support/fail-closed scope without overclaiming the separate hostile
path-race matrix.

The stale-relay live test needs an explicit RPC barrier. The installed `ctr`
CLI provides `tasks start` but no separate task-create command; it moves from
container metadata through Task Create to Task Start without an operator
barrier. Because the relay pathname is derived from the port/generation fixed
during Create but consumed at Start, a polling watcher would only demonstrate
a lucky scheduling race. A small source-controlled qualification helper will
call containerd `NewTask`, emit a CREATED marker, wait for a continuation file,
and then call `Start`, allowing the harness to place and identify the stale
root-owned Unix socket deterministically before product code touches it.

The first helper check is a local pre-build failure, not product evidence. It
ran root-relative chmod/gofmt/syntax names from `runtime/`, so those paths did
not exist. The build itself found the helper through `../scripts`, then stopped
without a binary because containerd's high-level client brings in lazily
excluded transitive modules whose checksums are not yet in `go.sum`. Nothing
ran on the VM. The correction will use repository-root paths and retain the
approach only if standard module resolution yields a bounded, reviewable
checksum delta rather than an uncontrolled dependency expansion.

The corrected check validates the design. Root-level gofmt and Bash syntax
pass, the helper builds at SHA `c5fa47a7…` from source SHA `6efb947e…`, and the
mode-0755 live qualifier is 7,625 bytes/SHA `62dfe9bb…`. Standard module
resolution changes only 30 `go.sum` checksum lines, with no `go.mod` change,
because containerd is already a direct dependency. The complete local runtime
suite passes all 22 packages after that delta. These results qualify the helper
and preserve the code baseline; no stale-socket product claim is made before
the commit-bound VM run.

The barrier qualifier is frozen in commit
`df1d147566323bc40d553dc8e96a228b1a80005c`. Its exact 5,795,840-byte Git
archive hashes to `1c71938d3c6b8aa453bc2e82cff489b1fd429a1e7d3021e5318ae9ab285e552b`
both locally and after upload. A new `/var/tmp/mksrc-df1d147` contains 661
files/5,243,520 bytes; helper and qualifier digests independently match
`6efb947e…` and `62dfe9bb…`, with guest `go.sum` at `bc6646c9…`. The next VM
transcript is therefore source-bound, but no stale-relay behavior is claimed
from transfer and hashing alone.

The first exact-commit stale-relay attempt reaches Task Create but stops on a
case-sensitive harness assertion before placing a stale socket or calling
Start. Containerd returns lowercase `created` with holder PID 11948, while the
qualifier expected uppercase. The mode-0600, credential-clean failure is
101,206 bytes/SHA `b1e9b261…`, exit 1. Its trap also learned that
`ctr tasks rm -f` enforces a CLI precondition for CREATED tasks. The helper
now supports a
scoped start-existing recovery path and the trap uses it before kill/delete.
Verified recovery binary `10408c51…` starts only that retained task; SIGKILL
records 137 and deletion succeeds. The recovery transcript is mode 0600, 9,739
bytes/SHA `e33caf23…`, credential-pattern clean; its immediate audit exits 1
only while two deleted-task shim wrappers reap. Bounded follow-up
`20261007-g6-stale-relay-first-cleanup-final-audit.log` is mode 0600, 13,512
bytes/SHA `5f17ecd8…`, exit 0, credential-pattern clean, with empty Kerf, all 19
counters zero, and healthy services. A separate failed audit exposed the
ephemeral sandbox token through Kerf command-line output; that mode-0600
4,506-byte/SHA `c8385175…` raw file was quarantined outside evidence and is not
cited. This entire attempt supplies harness/cleanup learning, not stale-socket
product evidence.

After adding lowercase status handling and CREATED-task recovery, the helper
and qualifier hash to `c2d9c070…`/`0dd9d82f…`. Bash syntax, diff hygiene, and
the full 22-package local runtime suite pass. One intervening command repeated
the earlier subdirectory/root-relative path mistake and ran no checks; it is
excluded. The successful root-directory rerun is only a local baseline until a
new commit-bound VM attempt passes.

Commit `28d1a1023ed26b5bf23248041c35028e5bde31f0` freezes the corrected helper
and trap. Its exact 5,795,840-byte archive hashes to `fa43e9ee…` locally and on
the guest; new `/var/tmp/mksrc-28d1a10` contains 661 files/5,249,376 bytes and
independently matches helper/qualifier hashes `c2d9c070…`/`0dd9d82f…`. The
preceding `5f17ecd8…` audit establishes a zero-resource start for this
source-bound retry.

The second exact-commit attempt validates the deterministic create barrier and
recovery cleanup, but not stale-relay replacement. Task
`mk-stale-relay-live` reaches status `created` with holder PID 19868 at
`2026-10-07T13:50:02.618832079Z`. Before a stale socket is derived or placed,
the qualifier exits 1 because it assumes `/var/lib/mkruntimed/state.json`; the
installed release has no monolithic state file at that path. The trap then uses
the helper to start that exact CREATED task and kill/delete its task/container.
Retained `20261007-g6-stale-relay-live-second.log` is mode 0600, 107,582 bytes,
SHA-256 `db8b971af97f1afbd78c4a89fbe54ee51005e4b1d32d013ba358ceffe1cce184`,
exit 1, and credential-pattern clean. This is qualification-harness evidence
only. A corrected retry must discover the authoritative installed relay
coordinates without assuming obsolete state layout, and must first prove the
  VM has returned to the strict all-zero resource baseline.

Read-only installation discovery identifies the mismatch precisely.
`/etc/mkruntime/config.json` resolves to a strict release config whose decoded
`state_directory` is `/var/lib/mkruntime`; its mode-0600 `state.json` contains
the expected `results,sandboxes,sequence,version` schema and presently zero
sandboxes. `/var/lib/mkruntimed` is instead the parent of the independent
rootfs/storage stores. Task and container listings are empty after recovery.
The qualifier should decode the installed host config just as mkruntimed does,
then read `<state_directory>/state.json`; embedding either path would repeat
the same provenance error. A separate strict post-failure audit remains
required before the next mutation.

The first independent audit after that recovery is intentionally retained as
cleanup-progress evidence. The mode-0600, 2,667-byte transcript has SHA-256
`e6d2d2ba9dd924b95e933a42c4def1b8cdbbde3c2da61d6e51884e3ab97fcbe0`,
exit 1, and no credential-pattern match. Kerf reports no instances and zero
allocated bytes, but its completely available 16-GiB pool remains configured;
the strict audit therefore stops at the first lifecycle residue. No clean
baseline is claimed until a bounded follow-up proves that pool is released.

The pool remains configured through the full explicit 120-second follow-up
(`pool_release_timeout`). This is not ordinary short-lived wrapper reaping and
therefore blocks the next qualification mutation. Durable daemon state still
has zero sandboxes and containerd has no task/container. Read-only logs and
lifecycle-result metadata must now explain the incomplete cleanup; if they
match the daemon's documented empty-state idle-pool recovery condition, a
scoped mkruntimed restart may recover it, followed by a new strict audit. This
observation still says nothing about stale-relay replacement behavior.

The 120-second timeout is expected pool policy, not evidence of a lifecycle
cleanup defect. `lifecycle.Service` deliberately keeps the initialized pool
through ordinary zero-sandbox intervals to avoid repeatedly acquiring a large
contiguous allocation; `ReleaseIdlePool` releases it on graceful daemon
shutdown. The qualifier's successful epilogue already restarts mkruntimed
before strict audit, but the path-discovery failure occurred before that
epilogue. A scoped manual restart now exercises the identical documented
recovery path, with both daemon PID transition and the subsequent inventory
captured as evidence.

That scoped recovery succeeds. The mode-0600 transcript
`20261007-g6-stale-relay-second-cleanup-recovery.log` is 13,162 bytes, SHA-256
`a27b2fb4716a1d89ddccdb0e7a711e7d526c0e90ef5345724dd2adbba6d245e8`,
exit 0, and credential-pattern clean. It proves zero sandboxes before restart,
mkruntimed PID 12829 -> 21126, active state with `NRestarts=0`, and then a
strict `No memory pool configured`/no-instances/all-19-counters-zero inventory
with all four services active/running and zero restarts. The revised qualifier
will decode the installed `state_directory` and make its failure trap perform
the same idle-pool release only after durable sandbox state is empty.

The third qualifier revision now does that. Both relay-coordinate discovery
and guarded cleanup decode `/etc/mkruntime/config.json` and join its
`state_directory` to `state.json`. Failure cleanup restarts mkruntimed only
when the durable sandbox map is exactly empty and the pool remains configured;
the Kerf query feeds a quiet predicate directly so no instance command line is
captured. Bash syntax, available ShellCheck, and diff hygiene pass. The updated
script hashes to `2611d5c435b39dddeaf99fe85adfe6bfbc06b711b7349c1a5ee5d77e11248e3a`;
the helper stays `c2d9c070…`. These checks qualify the harness revision but do
not replace commit/archive binding or the live stale-socket observation.

Commit `49acc614a30d0316e2f46405c7f9b9bd80bc1038` now freezes that revision.
Its 5,816,320-byte Git archive hashes to
`0e778c73760cece17788a7157aa6b8a30ab3ffe8821121cf751ecde58e72fdf5`
locally and after upload. New `/var/tmp/mksrc-49acc61` has 661 files/5,260,859
bytes and independently matches qualifier/helper hashes
`2611d5c4…`/`c2d9c070…`. Guest syntax passes, and the pre-run strict audit again
shows no Kerf pool or instance, all 19 counters zero, and all four services
healthy with zero restarts. This binds a clean third attempt; it does not by
itself demonstrate relay replacement.

The third exact-source attempt reaches and mutates the actual stale relay path.
Task Create publishes holder PID 28145 and resolves port 7200, generation
`7fd9e5bac156cc97032cac0d2023fc7a`, and
`/run/mk-agent-7200-7fd9e5bac156.sock`. The pre-Start object is root-owned,
mode 0755, one link, device 28/inode 3321, and returns `ECONNREFUSED`. Start
succeeds; the post-Start identity logic confirms a socket owned by root with
one link and an inode different from 3321, then its additional second-client
connection gets `ECONNREFUSED` and aborts before recording the workload or
final observation. The mode-0600 transcript is 111,256 bytes, SHA-256
`d270af0eea9e6bd2ed5ea15c1483280e72343131c38f3caa88e773e1c982901e`,
exit 1, and credential-pattern clean. Its guarded trap removes task/container
and restarts mkruntimed after observing zero sandboxes. This is evidence that
the stale inode was replaced, not yet a complete live/workload/cleanup pass;
the relay's accept model must determine whether a second connection is a valid
health assertion.

The relay implementation makes that answer explicit. In
`tools/mkvsock-relay.c`, `userver()` binds/listens, accepts exactly the shim's
single agent connection, closes its listener, and pumps the accepted stream.
A later second client must therefore see `ECONNREFUSED`; requiring acceptance
misdiagnoses correct single-accept behavior. Replacement inode identity plus
successful workload/agent operations and the extant relay pump process are the
appropriate live assertions. The next harness revision removes only the
invalid extra connect and continues through those checks.

Cleanup after the third attempt is independently complete. Transcript
`20261007-g6-stale-relay-third-cleanup-final-audit.log` is mode 0600, 13,027
bytes, SHA-256 `cea8a319995ad7c5897951a8dbc19c41b814de07b34b5fa9ce1c2932f3375891`,
exit 0, and credential-pattern clean. It binds mkruntimed PID 28297, no Kerf
pool/instance, all 19 counters zero, and four active/running zero-restart
services. Thus guarded failure recovery itself is proven and supplies the
clean baseline for a revised run.

Qualification now moves to the separate hostile-input/path-race gap while the
next stale-relay live retry is deferred. New
`scripts/test-runtime-hostile-paths-vm.sh` makes the intended VM scope explicit:
five groups, 59 named tests, and 100 race-detector repetitions per test. The
groups cover 22 shim namespace/task/OCI/I/O/token/recovery/event/relay cases,
17 rootfs mount/bundle/artifact/cleanup cases, seven descriptor-safe file
publication cases, seven Unix-socket ownership/replacement cases, and six
storage identity/path cases. The harness proves its selected names before
execution, fails on any skip, uses VM-local ext4 scratch/cache, and requires the
strict 19-counter audit both before and after. This records planned evidence
scope, not a passing result. Because the VM was user-restarted, its new boot,
installed identities, and zero-resource baseline must be captured anew before
the matrix runs.

Local harness preflight passes: the executable qualifier is 5,526 bytes with
SHA-256 `eeffc269623f72f1bd62e2e62fc21e9b121198a9ee6ee8d40898d2b81c15d47a`;
Bash syntax, available ShellCheck, and diff hygiene are clean. That establishes
only qualifier form. The guest will independently use `go test -list` to prove
all 59 names exist before any repeated race run, preventing a stale regex from
creating a false pass.

The restarted VM now has a fresh authoritative baseline. GCE resource
`6701540373796488780` is RUNNING from
`2026-10-07T17:39:59.195-07:00`; boot ID is
`7db90e79-7bf7-4ff8-befd-a3e1a5aff949`, kernel is
`7.0.0-mk2-gce-lab`, and addresses are `10.148.0.58`/`34.142.184.77`.
Release selection remains `0.1.0-dev-ca7d…`, with live daemon SHA
`a4a91006…` at PID 1451. All four services are active/running at PIDs
1451/1232/1465/1513 with zero restarts. Retained mode-0600 preflight
`20261008-post-user-restart-hostile-preflight.log` is 13,795 bytes, SHA-256
`2ce224c2f7d569be1768b71e82ef923bd592df0554f86e2c73b40c2beb241d75`,
exit 0, and credential-pattern clean. Its strict audit proves no pool or
instances and every one of the 19 resource counters at zero.

The first exact-commit hostile-path run exposes a VM-sensitive test fixture,
not a product defect. Commit `252add21366ab4e07b17c00dc41057bb89087711`
has a 5,826,560-byte archive/SHA-256 `b6240462…` on both hosts; its new guest
extraction contains 662 files/5,274,915 bytes and qualifier SHA `eeffc269…`.
All 22 shim test names enumerate. Across 100 race repetitions, the other
selected cases pass, while `TestStaleRelayCleanupRemovesExactSafeSocket` fails
all 100 times because it uniquely uses raw `t.TempDir()`: under the VM's 002
umask that directory is 0775, and the production Unix-socket guard correctly
rejects the group-writable parent. The mode-0600 failure transcript is
1,038,905 bytes, SHA-256
`a23690c54ca8ec32370e3c0295d88809674d418c22333919f5b6cd2af79a9230`,
exit 1, with 100 failure lines, zero skips, no race report, and no credential
pattern. Set-e stops before the remaining four groups. The fix belongs in the
test—reuse `privateTestDirectory(t)` mode 0700—not in the security policy.

The independent post-failure audit is clean. Mode-0600 transcript
`20261008-g6-hostile-path-first-failure-final-audit.log` is 13,022 bytes,
SHA-256 `ec0730ce098dfc2703ee24f6b182a9c1d7260e6fc124c7654640199e21eb0905`,
exit 0, and credential-pattern clean. It retains the restarted boot/service
PIDs, shows no pool or instance, all 19 counters at zero, and four active
zero-restart services. The test now uses `privateTestDirectory(t)`, while the
qualifier sets umask 077 before creating its own scratch/cache hierarchy.
Verification and an exact-commit VM rerun remain pending.

The corrected fixture passes 100 local race-detector repetitions in 1.062
seconds. Repository-root Bash syntax, available ShellCheck, and diff hygiene
also pass; the updated qualifier SHA-256 is
`f5b2059e01bae4219fa829d75ef5b0e1aa48d303b36e19637b5c1376e204d31d`.
An initial syntax command was mistakenly launched from `runtime/` with a
root-relative script path and therefore checked nothing; it is explicitly
excluded. The repeated root-level checks are the valid result, and the full
commit-bound VM matrix is still required.

Commit `595ebbe7dabebbdb86862a7f29270beef7030485` freezes both corrections.
Its 5,836,800-byte archive hashes to `94b0b1d1…` locally and on the guest. New
`/var/tmp/mksrc-595ebbe` contains 662 files/5,281,042 bytes; independent guest
qualifier/test hashes are `f5b2059e…`/`32fa942b…`, syntax passes, and the strict
preflight again returns all 19 counters to zero on boot `7db90e79…`. This is
the exact clean starting point for the complete corrected matrix.

The `595ebbe` rerun confirms the socket fixture fix and reveals a separate
harness-environment error. Its global `umask 077` silently converts deliberately
permissive test objects to private modes. Thus exactly two tests fail on every
one of 100 repeats: the unsafe-I/O fixture's requested FIFO 0660 becomes 0600,
and the unsafe-token fixture's requested 0666 file becomes 0600; both are then
correctly accepted by production and the tests complain. Other selected cases,
including safe stale-socket removal, pass; there are zero skips and no race
report. Mode-0600 transcript
`20261008-g6-hostile-path-race-matrix-corrected.log` is 1,040,961 bytes,
SHA-256 `690fd371cff491a5c39645ce2a63d82ff35c12047638bc78bf3c2d466d8c3e9b`,
exit 1, with 200 top-level failure lines and no credential-pattern match. The
right correction is to preserve the test process umask and chmod only the
qualifier-owned scratch/cache directories to 0700.

The independent audit after this second harness failure passes. Transcript
`20261008-g6-hostile-path-second-failure-final-audit.log` is mode 0600, 13,022
bytes, SHA-256 `ddf835267d0202d2ed2cd9b0ef6bd9c683b8e680f1bec8ff9ede317a578de3f1`,
exit 0, and credential-pattern clean. It again shows no pool/instance, all 19
counters zero, and unchanged healthy zero-restart services. The qualifier now
preserves the process umask and chmods only its own scratch/cache/tmp
directories to 0700. Validation and a fresh commit-bound run remain pending.

The final correction passes Bash syntax, available ShellCheck, and diff checks.
With umask 002 explicitly reproduced, the corrected stale-socket fixture and
both permissive-mode I/O/token tests pass together through 100 race-detector
iterations in 1.241 seconds. The qualifier now hashes to
`214c801ef71e99d9afb6a824bc5a45535a828aaf8392bf02764fd6695718afc6`.
This resolves the observed local harness regressions, but the full five-group
VM result remains open until a newly bound commit passes.

Commit `27ffedc167404bda0b6067c82f9a8fb305029fe1` now freezes that qualifier.
Its 5,836,800-byte archive hashes to `527783a3…` locally and on the guest. New
`/var/tmp/mksrc-27ffedc` contains 662 files/5,286,177 bytes; qualifier and fixed
test-source hashes match `214c801e…`/`32fa942b…`, and guest syntax passes. This
binds the next complete run to exact source without claiming its outcome.

The active `27ffedc` run has now completed its exact 22-test shim and 17-test
rootfs groups through 100 race-detector repetitions apiece, with zero skips and
explicit pass markers. The previously affected socket/I/O/token fixtures pass
under the guest's native umask. The seven-test safefile group is still running;
no Unix-socket, storage, final-audit, or aggregate result is claimed yet.

That `27ffedc` run completes shim (22), rootfs (17), and safefile (7) groups at
100 race repetitions with zero skips, then fails exactly two Unix-socket tests
on every repeat. Their `bind: invalid argument` occurs before policy behavior:
the qualifier's `/var/tmp/mk-hostile-paths.XXXXXX/tmp` prefix plus Go's long
test directory exceeds Linux `sockaddr_un`. Storage and final audit do not run.
The retained mode-0600 transcript is 1,743,744 bytes, SHA-256
`f7a5de01e498d4245058fdec32d3343fa12e25bfdf11fb49fb50861fb40f4e1c`,
exit 1, with 200 top-level failures, zero skips/race reports, and no credential
pattern. A short ext4 root `/var/tmp/h.XXXXXX`, used directly for TMPDIR and
GOTMPDIR, preserves both filesystem provenance and Unix path capacity.

The independent post-path-length audit passes. Mode-0600 transcript
`20261008-g6-hostile-path-third-failure-final-audit.log` is 13,022 bytes,
SHA-256 `92cf27d895b030e753bcf727d748148ec46bc7c204bb0992ee504ac2a9c85f12`,
exit 0, credential-pattern clean, and again proves no pool/instance, all 19
counters zero, and unchanged healthy services. The qualifier now uses the
short ext4 directory `/var/tmp/h.XXXXXX` directly for TMPDIR/GOTMPDIR with
only a short `c` cache child. Validation and another commit-bound run remain.

With an equivalently short private local temp root, all seven selected
Unix-socket tests pass 100 race-detector repetitions in 1.179 seconds. Bash
syntax, available ShellCheck, and diff hygiene pass, and the revised qualifier
hashes to `33c62163f1f11c77755649091a895151e64ca4529c26908c5675c2e3f0c1d983`.
This verifies the path-capacity fix locally, not the still-open VM aggregate.

Commit `b750ec4a2027320f514267930b83fe8565bbf1f4` freezes the short-root
correction. Its 5,847,040-byte archive hashes to `7716916d…` on both hosts.
Fresh `/var/tmp/mksrc-b750ec4` contains 662 files/5,291,293 bytes; qualifier
and fixed test-source hashes match `33c62163…`/`32fa942b…`, and guest syntax
passes. The complete aggregate must still pass in one execution.

The active `b750ec4` run has passed four groups in sequence: shim 22, rootfs
17, safefile 7, and Unix-socket 7, all for 100 race-detector repetitions with
zero skips. The short temp root removes the earlier Unix bind failures. Storage
is still active, so aggregate and final-audit status remain unclaimed.

The final `b750ec4` aggregate passes. Retained mode-0600 transcript
`20261008-g6-hostile-path-race-matrix-pass.log` is 1,888,866 bytes, SHA-256
`e9b9a241b86baec3f56e3294a54956447978b82a5136d3b13478c0147f5270d9`,
exit 0, and credential-pattern clean. It enumerates and executes all 59 exact
tests—22 shim, 17 rootfs, seven safefile, seven Unix-socket, six storage—for
100 race repetitions each with zero failures, skips, or race reports. Strict
audits before and after show no pool/instance, all 19 counters zero, and four
unchanged active zero-restart services. Exact group/aggregate markers plus
`G6_FINAL_RESOURCE_RETURN_PASS` and `G6_HOSTILE_PATH_RACE_MATRIX_PASS` close
the broader source path-race matrix. The parent hostile-input row remains open
only for the deferred complete live stale-relay/workload observation.

Independent post-pass transcript
`20261008-g6-hostile-path-final-independent-audit.log` is mode 0600, 13,022
bytes, SHA-256 `b972d661960eb5fd0f636363ee01a1114a1812328fd502172cc366e6bc009b59`,
exit 0, and credential-pattern clean. It independently repeats no pool or
instances, all 19 counters zero, and unchanged active zero-restart service PIDs
1451/1232/1465/1513. The source path-race scope is therefore evidence-complete;
this does not substitute for the separately deferred complete live stale-relay
workload proof.

The next open automated scope is consolidated in new
`scripts/test-runtime-task-v2-exhaustive-vm.sh`: 87 exact focused tests split
into lifecycle 20, process/I/O 33, control/read 14, and delete/event/recovery
20. The selection covers every one of the 17 Task RPC entries, including
supported and explicitly excluded methods, state guards, retry/duplicate
semantics, ordered events, exact exits, and normal/fallback cleanup. The
qualifier proves every name with `go test -list`, requires 20 race-detector
repetitions with zero skips/race reports, and audits resources before/after.
Local Bash syntax, available ShellCheck, and diff hygiene pass; the mode-0755,
7,128-byte script hashes to
`4553faf1a0af607f55776f79cc641fe175fad34d28aad0b6ad2e8eb858ff48c7`.
This is design/preflight evidence only until the exact source passes on the VM.

A static pre-bind audit resolves exactly 87 unique selections against existing
shim test functions, with no missing name or duplicate. The guest will still
perform compiled `go test -list` enumeration before execution; this local check
prevents committing an obviously stale aggregate list.

Commit `fdb15d73cf2625174b3a06633b423121029707b9` freezes the aggregate.
Its exact 5,857,280-byte archive hashes to `fd2a4af4…` locally and on the guest.
Fresh `/var/tmp/mksrc-fdb15d7` has 663 files/5,305,432 bytes; qualifier and
test-source hashes match `4553faf1…`/`32fa942b…`, syntax passes, and the strict
preflight again reports no pool/instance and all 19 counters zero on boot
`7db90e79…`. Transfer/preflight bind the run but do not establish its result.

The active Task v2 run has passed its 20-test lifecycle group through 20 race
repetitions with zero skips or race reports. The 33-test process/I/O group is
now active and has exercised resize, kill reply-loss deduplication, guest-delete
reconnect, CloseIO, and FIFO cases without failure so far. The remaining groups,
aggregate, and final audit are not yet claimed.

The exact-source Task v2 aggregate completes successfully. Lifecycle 20,
process/I/O 33, control/read 14, and delete/event/recovery 20 each pass 20
race-detector repetitions. Its aggregate is `methods=17 groups=4
selected_tests=87 iterations=20 skips=0 races=0 status=pass`, followed by
`G6_TASK_V2_EXHAUSTIVE_MATRIX_PASS`. Retained transcript
`20261008-g6-task-v2-exhaustive-matrix.log` is mode 0600, 810,938 bytes,
SHA-256 `d2ab3c84ca4ee92abc5faf5015dc1eb343b88893bd082ff26d7b38a8e6b59b2d`,
and wrapper exit 0. Direct scans find zero top-level failures, skips, race
reports, or credential-pattern matches. Both strict audits emit
`G6_FINAL_RESOURCE_RETURN_PASS`; the final state has no Kerf pool/instances,
all 19 resource counters zero, and mkruntimed/mknetd/containerd/Docker active
at unchanged PIDs 1451/1232/1465/1513 with zero restarts. This closes the
fake-daemon every-method/state/duplicate/event/exit/cleanup automated row; it
does not stand in for any separately required live fault-injection evidence.

Independent post-aggregate audit
`20261008-g6-task-v2-final-independent-audit.log` is mode 0600, 13,358 bytes,
SHA-256 `bcba3c7fb4dc9c5406f1377f13725290834483c5cb8220ae4d56befba8d9361d`,
exit 0, and credential-pattern clean. A separate SSH command again observes no
Kerf pool or instances, all 19 resource counters zero, and the same active
zero-restart service PIDs 1451/1232/1465/1513 before emitting
`G6_FINAL_RESOURCE_RETURN_PASS`. This independently confirms post-suite cleanup.

The next live cancellation input is now source-controlled. Helper
`runtime-cancellation-blocker.sh` delegates validation and non-target Kerf calls
to the exact real executable, but holds either the rootfs-build or Kerf-load
boundary with a signal-resistant descendant. Qualifier
`test-runtime-cancellation-live.sh` temporarily starts the same mkruntimed argv
under a collected systemd unit, cancels a real ctr Create at each boundary,
requires both blocker PIDs to disappear, and runs the strict audit after each.
It then cancels live Wait and attach RPCs against a running child, requires the
same Task PID/state and a successful post-cancel exec, deletes normally, and
audits again. Its exit trap restores the official mkruntimed and removes all
qualification assets. Bash syntax, available ShellCheck, and diff hygiene pass.
The mode-0755 helper is 829 bytes/SHA-256 `60ee2998…`; the mode-0755 qualifier
is 6,708 bytes/SHA-256 `524f8d5b…`. No behavior is claimed before exact-source
VM execution and a separate post-run audit.

Commit `af132d8b75a8a08244ac39a45718a6087180cd6a` freezes those inputs. A clean
6,696,960-byte `git archive` at `/tmp/mksrc-af132d8.tar` has SHA-256
`4e91114549ad13ceb079558f9b1228d8e65a18c9c9659fa18281c4829fb9201d`.
This is source binding for the next transfer, not live cancellation evidence.

Guest preflight `20261008-g6-cancellation-source-preflight.log` is mode 0600,
17,196 bytes, SHA-256
`fda143bdfb4761e32076881facc101dd8fcbe1ddd05d4420a6977835e0b1ea71`, and
exit 0. The guest independently matches archive SHA `4e911145…`, extracts 667
files/6,145,091 bytes into fresh `/var/tmp/mksrc-af132d8`, matches both script
digests, and passes Bash syntax. The strict baseline again has no pool or
instances, all 19 counters zero, and four unchanged active zero-restart
services. This establishes transfer/baseline only, before fault execution.

The first exact-source cancellation run fails and is retained rather than
overwritten. `20261008-g6-cancellation-live-first.log` is mode 0600, 73,262
bytes, SHA-256 `1f59d9f4cd187d40c270c5d1cb6253075c16e18d3f4ee7d7524cc0f44d0e82c5`,
exit 1, and credential-pattern clean. It reaches the rootfs-build blocker and
cancels the real ctr Create, but PID 79086 remains present throughout the
60-second reap assertion. The restoration trap returns the official daemon and
removes temporary paths, but cannot make this a behavioral pass.

The separate `20261008-g6-cancellation-first-failure-audit.log` is mode 0600,
9,129 bytes/SHA-256 `94c5f2d274a2051ac513cff25525eb7e6882fc3857d890c0e2a47d9bb7c6e4cd`.
Its strict audit fails at two residual shim processes while every other one of
the 19 counters and Kerf pool/instance state is zero. Later diagnostic commands
mask the capture's aggregate exit to zero, so this transcript is classified
failed. Corrected process/journal evidence
`20261008-g6-cancellation-first-leak-diagnostic-corrected.log` is mode 0600,
8,435 bytes/SHA-256 `17e1300502db4c606d7a58316d31fb6925663c8e4e6df3a61324b0dcddf25f9d`,
exit 0. PIDs 79062/79067 are live sleeping exact-binary supervisor/worker
processes with parentage 1→79062→79067 and exact task argv, not zombies.
Containerd reports Delete deadline expiry, shim disconnect, and fallback delete
failure because the bundle working directory is already absent. A preliminary
diagnostic failed on nested awk expansion and is retained at mode 0600/823
bytes/SHA `38ce2b34…`. The live row stays open pending cleanup, source diagnosis,
and a corrected exact-source replay.

Source diagnosis finds a real supervisor ordering defect. The proxy ran in a
goroutine, but the supervisor immediately blocked on `cmd.Wait`; containerd EOF
therefore sat unread in `proxyDone` while the idle worker and supervisor stayed
alive. The fix races worker and proxy completion. Proxy-first completion closes
the private connection, kills/reaps any still-live worker, removes the held
PID-file identity, and exits without spending the forced-worker restart budget.
A process test starts a blocking real worker, closes the containerd side, and
requires bounded supervisor exit plus absent worker PID/PID file.

The first local test invocation stopped before compilation on the read-only
default Go cache. A private-cache retry compiled the changes, but abstract Unix
listener creation is forbidden locally with `EPERM`; the process test timed out
and its cleanup exposed a test-only directory-close race. It now capability-
probes and skips before any goroutine when abstract listeners are unavailable.
Neither local failure is pass evidence; corrected private-cache validation and
the VM's zero-skip execution remain pending.

The first cleanup command is retained as a mode-0600, 1,227-byte, SHA-256
`77457ff5…` harness failure: grep treated leading `-id` as an option and no
signal was sent. Corrected cleanup transcript
`20261008-g6-cancellation-first-leak-cleanup-corrected.log` is mode 0600,
14,853 bytes, SHA-256
`2f9ad273ccdd793df98e06241a2695c6695a39e3cb03e39a8fee4ed3e8a743e4`,
exit 0. It validates both exact task argvs, signals only supervisor 79062,
observes supervisor/worker 79062/79067 vanish, and passes the strict all-zero
audit with restored official mkruntimed PID 79465 and healthy services.

With the capability preflight, the private-cache focused race suite passes 20
repetitions in 24.042 seconds. The new process test skips cleanly only under the
local abstract-socket restriction; the existing bridge replay/replacement and
worker-restart tests pass. A zero-skip VM run is still required.

The final supervisor form closes private TTRPC first and grants the worker a
30-second production grace to finish canceled-Create rollback; it force-kills
and reaps only after that bound. The deliberately non-serving regression worker
uses an injected 100 ms grace. The corrected focused matrix passes 20 race
repetitions in 24.067 seconds, and the full shim package passes `-race` in
9.985 seconds, followed by package vet, qualifier Bash syntax, and diff hygiene.
This is local implementation evidence only; the VM must build/activate this
exact candidate before the live cancellation replay can support a claim.

Commit `385f019905d0309065f065288117df6ecf9914fe` freezes the supervisor
correction, regression, and first-run/cleanup evidence. Its clean source archive
`/tmp/mksrc-385f019.tar` is 6,850,560 bytes and hashes to
`99bb1941082a855e8cf895141d3d314ecb677b7c68b96479a1dc615d914386c0`.
No VM build or activation is yet claimed.

The first `385f019` VM build gate is retained at mode 0600, 14,177 bytes,
SHA-256 `df4b0ec6d6f081b89b014959f3f3ff65300c840c8847b86fb56187e0ed1cd16a`,
exit 1. It has zero skips/race reports and one top-level failure: 19
process-level disconnect repetitions pass, while one opens the marker between
exclusive file creation and PID write and attempts `Atoi("")`. Build/install
never run, leaving the active release unchanged. The test now waits for a
nonempty PID record before parsing.

Commit `cacec0db6fef21a5721bcdef558dc30703867d89` freezes the test-only
correction and its failed-gate record. Clean archive `/tmp/mksrc-cacec0d.tar`
is 6,860,800 bytes/SHA-256
`b16af3c929a1fe4d34163521fa055799e65fa6e31a155eae5e9ed720ec4b7f79`.
The corrected local focused matrix passes 20 repetitions in 24.059 seconds;
no transfer or VM result is yet claimed.

Strengthened guest transcript `20261008-g6-cancellation-build-cacec0d.log` is
mode 0600, 63,147 bytes, SHA-256
`1e16427487eab78a2021d7dce4ef1f09bd0cb9e23b8e4c157335f17476e7be8e`,
exit 1. The actual regression gate passes 100 race repetitions in 128.518
seconds—400 passing groups and zero failures, skips, or race reports—and all
seven version-stamped components build. `make runtime-manifest` creates its
exclusive manifest; a redundant second generator then correctly fails with
`EEXIST`, before installer invocation. The active release remains unchanged;
this is a harness-command failure, not candidate activation or live evidence.

Corrected activation transcript
`20261008-g6-cancellation-activate-cacec0d.log` is mode 0600, 17,873 bytes,
SHA-256 `91853bab6940d074e2c2c6c104fe9ef24e05d835b6557d8bc482d59f2f729315`,
exit 0, with a clean credential-value scan. It installs and selects exact
release `0.1.0-dev-cacec0db6fef21a5721bcdef558dc30703867d89`; installed and
source shims independently match SHA-256
`3d1b1b9ff843e7e9ac4f65ffb3a10a415165b4c707db223961c52fa1f866a3ed`.
On boot `0404118a-b8a8-4187-89fa-2e45529963c2`, mkruntimed and mknetd are
coherently restarted while containerd and Docker remain healthy; all four are
active/running with zero restarts. The strict post-activation audit reports no
Kerf pool or instances and all 19 resource counters zero. This establishes the
exact installed baseline, not yet live cancellation behavior.

Exact-source live transcript `20261008-g6-cancellation-live-cacec0d.log` is
mode 0600, 73,286 bytes, SHA-256
`2bfba8a852054ebd6884068c7c2a5e798fb0740baf58a22eec938e07d811e25b`,
exit 1, with a clean credential-value scan. The exact-release preflight is
clean and the real rootfs Create reaches blocker parent 16712 and deliberately
TERM-ignoring child 16715. Canceling the actual ctr client returns status 124,
but both blocker PIDs remain for the full 60-second observation bound. The
qualifier stops before child-boot and Task Wait/Attach, so this is a reproduced
live cancellation defect rather than qualifying evidence.

Immediate diagnostic
`20261008-g6-cancellation-cacec0d-failure-diagnostic.log` is mode 0600,
16,423 bytes, SHA-256
`503e92dc176e797cdfb988f60f76fafa2a4370eb16afec2336073015de07b9c9`,
exit 0, with a clean credential-value scan. The qualifier trap's transient
service stop has removed both blocker PIDs by collection time. Containerd logs
a Delete deadline at 13:28:56, shim disconnect at 13:29:01, then fallback
delete failure because the bundle CWD is already gone. Official exact-candidate
mkruntimed is restored as PID 17091; all four services are active/running with
zero restarts, Kerf is empty, and every one of the 19 resource counters is
zero. Restoration cleanup is therefore sound, while cancellation-induced
descendant termination before daemon teardown remains unproved and open.

The remaining root cause lies in the mkruntimed daemon protocol. Its client
formerly half-closed the write side to delimit JSON; the server necessarily
consumed that EOF before dispatch and therefore could not observe a later
caller disconnect to cancel backend work. Requests are now newline-framed with
the socket left open. The bounded server parses that frame, still accepts the
former EOF-delimited form, and derives a per-request dispatch context that is
canceled when the framed peer disconnects. Rootfs and Kerf pass this context to
the existing bounded command runner, whose cancellation kills the entire
private process group.

Direct framed-disconnect and actual `daemon.Client` cancellation regressions
both pass 100 race-detector repetitions. Full race suites for daemon, rootfs,
lifecycle, mkruntimed, and the containerd shim pass, followed by focused vet and
diff hygiene. This is local correction evidence only; exact-source guest build,
activation, and the complete live cancellation replay remain required.
The exact transcript `20261008-g6-daemon-disconnect-local-race.log` is mode
0600, 1,369 bytes, SHA-256
`d190575d88cc589f014864fc6d34d2a5b2a82e6eec4cee4b4d96ef78300e478e`,
exit 0, with a clean credential-value scan.
