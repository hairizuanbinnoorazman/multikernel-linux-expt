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

The generation-evolution lifecycle passed 100 repetitions, followed by clean
full Go race/vet and documentation/schema/evidence, OCI, rootfs/storage/image,
deployment, containerd, and final-audit gates. The expected permission-gated
socket subcase remains a live-host check. Exact-source deployment and service
recovery remain pending.

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

The exclusive publication, raced replacement, and partial-launch cleanup
group passed 100 race-detector repetitions. Full-tree verification remains
pending.

The subsequent complete repository race suite, `go vet ./...`, the complete
documentation/schema/evidence/deployment chain, and `git diff --check` passed
on 2026-09-18. The local exclusive launch-metadata checkpoint is complete;
current-source disposable-host proof remains unauthorized.

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
