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

The final I/O transcript is also marker-oriented: it does not retain the
asserted stdin, attach, or `37 91` guest output. Its exact harness hash and exit
status support the scoped assertions, but the dirty source diff is absent. Its
manifest is not schema-valid because component versions are missing and
`${HOST_BOOT_ID}` is invalid, and `gate: G6` plus `result: pass` can be mistaken
for a full-gate result.

## Current audited verdict

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
packet path is negotiated-MTU bounded and
single-flight, detects a 250 ms stalled exchange, reconnects without resetting
the authenticated sequence, and reports monotonic packet/drop/error counters.
DNS configuration and regular-file/symlink/absent restoration are tested.
Linux command-order tests inject failure at every partial-`ADD` boundary and
assert reverse cleanup, source-spoof, sibling, metadata, and default-drop
rules. These are implementation observations from the local test suite, not
G5 gate evidence: privileged namespace traffic, policy bypass, restart/load,
and final cleanup still require the disposable-instance matrix and raw bundle.
