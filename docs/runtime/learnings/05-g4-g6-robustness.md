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
An errored `StartProcess` call now reconciles with an independent five-second
state read. Exact `CREATED` preserves retryable local state; exact `RUNNING` and
rapid `STOPPED` prove the start applied and return success; unresolved transport
failure retains unverified ownership, monitoring, durable state, and bounded
cleanup. All four boundaries pass 100 race-detector repetitions.
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
resources are reaped. Live forced-death continuity for a running process is
still required.

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

Rootfs recovery records now bind both the original containerd bundle and the
configured storage root to their device, inode, and owner. A same-owner
whole-directory substitution made before replay or cleanup is rejected before
backend verification or unmount, and cleanup remains descriptor-anchored after
opening the recorded roots. Focused bundle and storage-root replacement tests
prove the substitute remains untouched and the durable cleanup record remains
available for retry. The identity-bearing store is explicitly version 2. An
empty version-1 store upgrades atomically; nonempty legacy ownership is
rejected because its original directory identity cannot be reconstructed
safely from a pathname.
