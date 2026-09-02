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

## OCI and process feature matrix

“Advertised” is not treated as “tested.” The retained live result proves only
what appears in [`g3-lifecycle.json`](../../../evidence/runtime-20260831/g0-g3-gce/g3-lifecycle.json)
and the indexed child console.

| Feature | Status | Proof or limitation |
| --- | --- | --- |
| Create/start/wait/delete one process; split stdout/stderr; exit 23; child kernel release | `live-tested` | `g3-lifecycle.json` records every response and exact output; `runtime-g3-console.log` records the selected child kernel and assigned CPU/memory view. |
| Authentication identity, HMAC, and replay rejection | HMAC/replay `unit-tested`; identity mismatch `implemented-unproven`; happy path live | `agent.TestAuthenticationAndReplay` covers a valid identity, invalid MAC, and replay, but not wrong sandbox ID, generation, or endpoint. The live lifecycle succeeded with one authenticated session. |
| Exact argv, environment, cwd, UID/GID plumbing | `unit-tested` or implemented-unproven live | `agent.TestLifecycle` exercises argv/environment and current UID/GID locally. The retained live result does not expose its config, so non-default cwd and non-root identity are not live-proven. |
| Supplementary groups and signal delivery | `implemented-unproven` | Code passes groups and accepts signals, but neither non-empty groups nor delivery is proved by retained live evidence. Advertising them in the historical `Capabilities` reply was over-broad; the current response omits UID, GID, groups, and signals until their nontrivial live cases pass, enforced by `agent.TestAuthenticationAndReplay`. |
| Wait before start | `unit-tested rejected` | `agent.TestWaitRejectsUnstartedProcess` proves an unstarted process returns an error instead of blocking forever. |
| Unsupported mounts | `unit-tested rejected` | `agent.TestUnsupportedFailsClosed`. |
| Hooks, capabilities, namespaces/resources, seccomp, masked/read-only paths, read-only root, `noNewPrivileges`, rlimits, and hostname | direct-agent rejection implemented; not fully tested | `LoadBundle` rejects non-empty values, but the required field-by-field test matrix is absent. The later G6 builder can discard these fields before `LoadBundle`, so fail-closed behavior is not true end to end. |
| `ExecProcess` | implemented later; G6 live-tested, focused G3 unit coverage incomplete | The later shared `ctr`/Docker matrix exercises exec through the shim and agent, but the historical G3 run did not and the agent suite lacks a focused exec lifecycle/failure matrix. |
| Terminal and resize | implemented later; unit-tested; PTY and initial size live-tested | Local agent tests exercise post-start resize. The retained G6 live harness proves PTY operation and initial-size propagation, not a deliberate post-start size change. |
| Stdin, `CloseProcessStdin`, incremental output, and attach | implemented later; unit- and G6 live-tested narrowly | Requests are chunk-bounded, but complete process output remains in unbounded in-memory buffers and slow-reader/backpressure behavior is open. |
| Process/output retention limits, reconnect, structured agent errors, reply integrity | `not implemented` | No complete implementation or test exists. |
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
architecture validation, bounded output/backpressure, complete failure/race
coverage, reply integrity, reconnect, structured secret-safe errors, and
agent-driven filesystem quiescence/poweroff remain absent. The current agent
protocol schema and frozen kernel/image policy also lag the added methods and
terminal capability, while the G6 OCI adapter violates the frozen fail-closed
rule by discarding unsupported fields.
