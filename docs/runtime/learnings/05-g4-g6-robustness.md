# G4-G6 robustness follow-up

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
