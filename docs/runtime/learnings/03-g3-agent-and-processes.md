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
| Authentication identity, HMAC, and replay rejection | `unit-tested`; happy path live | `agent.TestAuthenticationAndReplay`; live lifecycle succeeded with one authenticated session. |
| Exact argv, environment, cwd, UID/GID plumbing | `unit-tested` or implemented-unproven live | `agent.TestLifecycle` exercises argv/environment and current UID/GID locally. The retained live result does not expose its config, so non-default cwd and non-root identity are not live-proven. |
| Supplementary groups and signal delivery | `implemented-unproven` | Code passes groups and accepts signals, but neither non-empty groups nor delivery is proved by retained live evidence. Advertising them in the historical `Capabilities` reply was over-broad; the current response omits UID, GID, groups, and signals until their nontrivial live cases pass, enforced by `agent.TestAuthenticationAndReplay`. |
| Wait before start | `unit-tested rejected` | `agent.TestWaitRejectsUnstartedProcess` proves an unstarted process returns an error instead of blocking forever. |
| Unsupported mounts | `unit-tested rejected` | `agent.TestUnsupportedFailsClosed`. |
| Hooks, capabilities, namespaces/resources, seccomp, masked/read-only paths, read-only root, `noNewPrivileges`, rlimits, hostname, terminal | `implemented rejection; not fully tested` | `LoadBundle` rejects non-empty values, but the required field-by-field test matrix is absent. |
| `ExecProcess`, bounded streaming/backpressure, process/output retention limits, reconnect, structured agent errors, reply integrity | `not implemented` | No implementation or test exists. |
| Real quiescence, session termination, filesystem flush/poweroff | `not implemented` | `Shutdown` returns a constant `quiesced` body; the controller closing the transport drives the historical shutdown. |

The plan's full minimum implementation remains the G3 exit criterion. The
frozen policy's smaller accepted/rejected OCI set governs the provisional
prototype and does not waive namespaces, capabilities, rlimits, cgroups,
`ExecProcess`, independent bounded stdio, tested signals, or verified clean
quiescence required by the plan.
