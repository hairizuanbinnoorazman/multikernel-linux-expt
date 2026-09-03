# G3 learnings: child agent and OCI lifecycle

## Result and current status

The 2026-08-31 run is a **provisional direct-bundle G3 milestone**, not a closed
gate. On GCE, a static
`mk-agent` booted from the runtime initramfs, authenticated the sandbox ID,
generation, endpoint, sequence, and HMAC, then managed a BusyBox OCI bundle
whose userspace was separate from the selected child kernel.

The live process printed distinct stdout/stderr, reported
`7.0.0-mk2-gce-lab`, and preserved exit code 23. `CreateProcess`,
`StartProcess`, `WaitProcess`, `DeleteProcess`, and `Shutdown` completed; the
child stopped and the primary returned to CPUs `0-15` with no instance or
pool. The exact result is in
[`g3-lifecycle.json`](../../../evidence/runtime-20260831/g0-g3-gce/g3-lifecycle.json).

## Transport compatibility finding

The recovery snapshot had `CONFIG_MULTIKERNEL_VSOCKETS` disabled. Rebuilding
the previously proved patched module produced the same SHA-256
`f7bcaf7f…b23495f` as the mediated-storage experiment.

Direct Go AF_VSOCK endpoints were not compatible with this pinned transport:
the first wrapper failed because Go could not interpret AF_VSOCK in
`getsockname`; subsequent data exchange through raw Go endpoints reset the
primary twice, once in each connection direction. Both resets returned with no
Multikernel resource leak, but this path is prohibited.

No raw output from those failed attempts or the two resets was retained. They
are therefore classified as **unretained operator observations**, not evidence
claims. The prohibition remains conservative and is independently consistent
with the retained passing C-relay path; a later session must not cite the reset
details as reproduced proof without a new safe capture.

The passing design uses `mkvsock-relay`, a small static C bidirectional relay
derived from the already-proved socket setup. The primary listens, the child
connects to CID 0, and Go uses Unix sockets on both sides. Frames use an
explicit big-endian 32-bit length and a 1 MiB bound; no EOF or half-close is
used as a message delimiter.

## Replacement live evidence (2026-09-03)

The direct-bundle lifecycle reproduced through a newly built pinned transport
module. The harness now generates a fresh random 128-bit generation and
256-bit token for every run and redacts the token from the child console on
success or failure. The retained result again proves split stdout/stderr,
child release `7.0.0-mk2-gce-lab`, exit 23, quiescence, and complete resource
return; empty control/relay logs indicate no emitted error. The [G3 manifest](../../../evidence/runtime-20260903/g0-g3-proof/manifest-g3.json)
remains `provisional`: the negative authentication/framing matrix, broader OCI
semantics, bounded output, reconnect, and agent-driven poweroff are still open.

The continuing remediation run deployed the updated agent and repeated that
live lifecycle with a fresh generation/token. Its harness now places all
artifacts in one evidence directory and checks the exact generated token after
cleanup; `token-redaction-check.txt` records that it is absent. Local focused
tests additionally cover field-by-field OCI rejection, OCI version and x86-64
executable checks, process-group signal delivery, bounded retained output and
process count, terminal-size rejection, shutdown while running/quiescent,
framing failures, structured secret-safe errors, and reply version/sequence
binding. These additions narrow the open work but do not prove the full live
negative matrix or independent streaming/backpressure.

## OCI and process feature matrix

“Advertised” is not treated as “tested.” The retained live result proves only
what appears in [`g3-lifecycle.json`](../../../evidence/runtime-20260831/g0-g3-gce/g3-lifecycle.json)
and the indexed child console.

| Feature | Status | Proof or limitation |
| --- | --- | --- |
| Create/start/wait/delete one process; split stdout/stderr; exit 23; child kernel release | `live-tested` | `g3-lifecycle.json` records every response and exact output; `runtime-g3-console.log` records the selected child kernel and assigned CPU/memory view. |
| Authentication identity, HMAC, and replay rejection | HMAC/replay `unit-tested`; identity mismatch `implemented-unproven`; happy path live | `agent.TestAuthenticationAndReplay` covers a valid identity, invalid MAC, and replay, but not wrong sandbox ID, generation, or endpoint. The live lifecycle succeeded with one authenticated session. |
| Exact argv, environment, cwd, UID/GID plumbing | `unit-tested` or implemented-unproven live | `agent.TestLifecycle` exercises argv/environment and current UID/GID locally. The retained live result does not expose its config, so non-default cwd and non-root identity are not live-proven. |
| Supplementary groups and signal delivery | groups `implemented-unproven`; signal scope `unit-tested` | A focused process-group test proves that the leader and descendant receive `SIGTERM`. Non-empty supplementary groups remain without a nontrivial live test, so the current response does not advertise them. |
| Wait before start | `unit-tested rejected` | `agent.TestWaitRejectsUnstartedProcess` proves an unstarted process returns an error instead of blocking forever. |
| Unsupported mounts | `unit-tested rejected` | `agent.TestUnsupportedFailsClosed`. |
| Hooks, capabilities, namespaces/resources, seccomp, masked/read-only paths, read-only root, `noNewPrivileges`, rlimits, and hostname | direct-agent rejection `unit-tested` | Field-by-field direct-agent tests prove rejection. The later G6 builder can still discard these fields before `LoadBundle`, so fail-closed behavior is not true end to end. |
| `ExecProcess` | implemented later; G6 live-tested, focused G3 unit coverage incomplete | The later shared `ctr`/Docker matrix exercises exec through the shim and agent, but the historical G3 run did not and the agent suite lacks a focused exec lifecycle/failure matrix. |
| Terminal and resize | implemented later; unit-tested; PTY and initial size live-tested | Local agent tests exercise post-start resize. The retained G6 live harness proves PTY operation and initial-size propagation, not a deliberate post-start size change. |
| Stdin, `CloseProcessStdin`, incremental output, and attach | implemented later; unit- and G6 live-tested narrowly | Requests are chunk-bounded and retained stdout/stderr are capped at 4 MiB each, but independent streaming and slow-reader/backpressure behavior remain open. |
| Process/output retention limits | `unit-tested` | The manager caps processes at 1024 and each retained stdout/stderr stream at 4 MiB with explicit truncation flags. |
| Reconnect | `not implemented` | The declared reconnect behavior still lacks implementation and tests. |
| Structured agent errors and reply binding | `unit-tested` | Replies use fixed secret-safe structured errors; the client validates protocol version and request sequence. V1 deliberately has no reply MAC on its trusted point-to-point session, as documented. |
| Quiescence check, session termination, filesystem flush/poweroff | quiescence rejection implemented; shutdown incomplete | `Shutdown` now rejects while a managed process is running and reports `quiesced` only after processes stop. It still does not terminate the agent session, perform storage quiescence, or power off independently; the controller closes the historical transport. |

The plan's full minimum implementation remains the G3 exit criterion. The
frozen policy's smaller accepted/rejected OCI set governs the provisional
prototype and does not waive namespaces, capabilities, rlimits, cgroups,
independent bounded stdio, complete exec/signal semantics, or verified clean
quiescence required by the plan.

## Current audited verdict

Later G4-G6 work materially expanded the agent beyond the 2026-08-31 direct
bundle milestone: exec, process state, process-group signals, stdin/EOF,
incremental reads, attach support, PTYs, resize, and static network transport
now exist. Those additions do not retroactively strengthen the historical G3
manifest, and several are covered only by narrow unit or shared-client tests.

G3 remains open because configured namespaces, capabilities, rlimits, cgroups,
independent streaming/backpressure, complete failure/race coverage, reconnect,
and agent-driven filesystem quiescence/poweroff remain absent. Executable
architecture, bounded retention, structured errors, reply binding, the expanded
agent method schema, and terminal policy are now implemented/tested; the G6 OCI
adapter still violates the frozen fail-closed rule by discarding unsupported
fields.
