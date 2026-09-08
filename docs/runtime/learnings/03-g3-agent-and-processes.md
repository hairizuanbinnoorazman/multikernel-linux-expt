# G3 learnings: child agent and OCI lifecycle

## Result and current status

The 2026-08-31 run remains a **provisional historical direct-bundle G3
milestone**. The corrected 2026-09-04 live matrix and post-deletion cloud ledger
close G3. On GCE, a static
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
remains `provisional`: it predates the negative authentication/framing matrix,
non-root live identity, reconnect, and agent-driven poweroff proof.

The continuing remediation run deployed the updated agent and repeated that
live lifecycle with a fresh generation/token. Its harness now places all
artifacts in one evidence directory and checks the exact generated token after
cleanup; `token-redaction-check.txt` records that it is absent. Local focused
tests additionally cover field-by-field OCI rejection, OCI version and x86-64
executable checks, process-group signal delivery, terminal-size rejection,
structured secret-safe errors, and reply binding. The 2026-09-04 closure pass
adds the complete exec/exit/race matrix, independent bounded streaming with
backpressure/truncation, runtime capability reporting, fixed bundle-root
confinement, reconnect with sequence continuity, final-reply shutdown, and
child-scoped poweroff wiring. Those mechanisms pass the race suite; the fresh
live mechanisms are retained in the final disposable-host transcript.

## OCI and process feature matrix

“Advertised” is not treated as “tested.” The retained live result proves only
what appears in [`g3-lifecycle.json`](../../../evidence/runtime-20260831/g0-g3-gce/g3-lifecycle.json)
and the indexed child console.

| Feature | Status | Proof or limitation |
| --- | --- | --- |
| Create/start/wait/delete one process; split stdout/stderr; exit 23; child kernel release | `live-tested` | `g3-lifecycle.json` records every response and exact output; `runtime-g3-console.log` records the selected child kernel and assigned CPU/memory view. |
| Authentication identity, HMAC, replay, ordering, and malformed input | `live-tested` safe matrix; broader hostile campaigns deferred | The final lifecycle records wrong identity fields, protocol, invalid MAC, replay, out-of-order, malformed input, and oversized-frame closure; focused tests add concurrent and truncated-frame coverage. |
| Exact argv, environment, cwd, UID/GID plumbing | `live-tested` | The retained child ran literal argv/environment from `/work` as UID 1234, GID 2345 and groups 2345,3456. |
| Supplementary groups and signal delivery | groups `live-tested`; signal scope `unit-tested` | The final lifecycle retains non-empty groups; focused tests prove both leader and descendant receive process-group signals, including ignored TERM followed by KILL. |
| Wait before start | `unit-tested rejected` | `agent.TestWaitRejectsUnstartedProcess` proves an unstarted process returns an error instead of blocking forever. |
| Unsupported mounts | `unit-tested rejected` | `agent.TestUnsupportedFailsClosed`. |
| Hooks, capabilities, namespaces/resources, seccomp, masked/read-only paths, read-only root, `noNewPrivileges`, rlimits, hostname, and annotations | adapter and direct-agent rejection `unit-tested` | Field-by-field tests prove rejection, including empty objects and false-valued unsupported switches. The builder validates before allocation and rewrites only `root.path`; exec translation separately rejects unsupported process controls. |
| `ExecProcess` | `unit-tested`; later G6 live-tested | Focused tests cover invalid parent/spec/ID, duplicate, failed start cleanup, exact root inheritance, concurrency, signal, wait, and delete. |
| Terminal and resize | implemented later; unit-tested; PTY and initial size live-tested | Local agent tests exercise post-start resize. The retained G6 live harness proves PTY operation and initial-size propagation, not a deliberate post-start size change. |
| Stdin, `CloseProcessStdin`, incremental output, and attach | `unit-tested`; later G6 live-tested narrowly | Requests are chunk-bounded. Reads independently advance stdout/stderr windows; slow and absent readers exercise bounded backpressure and explicit truncation. |
| Process/output retention limits | `unit-tested` | The manager caps processes at 1024 and each stream at 4 MiB; state/wait replies exclude output and cannot exceed the frame through retention. |
| Reconnect | `live-tested` | Relay loss preserved the managed process and sequence; the client reconnected at the next sequence, with focused replay rejection coverage. |
| Structured agent errors and reply binding | `unit-tested` | Replies use fixed secret-safe structured errors; the client validates protocol version and request sequence. V1 deliberately has no reply MAC on its trusted point-to-point session, as documented. |
| Actual child/agent capabilities | `live-tested` | The response retains child release/architecture, Multikernel, cgroup v2 and transport presence, plus agent UID/GID, capability masks and `NoNewPrivs`; it does not substitute advertised constants for measured facts. |
| Quiescence check, session termination, filesystem flush/poweroff | `live-tested` | Shutdown returned `quiesced`, completed its reply, ended the session, invoked child poweroff, and returned Kerf to `loaded`; focused tests cover live-process rejection and races. |

The synchronized plan and policy define G3 as the direct-bundle process and
transport boundary. Namespaces, capability application, rlimits, and cgroups
remain rejected—not discarded—at this gate and remain mandatory for G6
production container containment. This records the real boundary without
weakening the later container requirement.

## Current audited verdict

Later G4-G6 work materially expanded the agent beyond the 2026-08-31 direct
bundle milestone: exec, process state, process-group signals, stdin/EOF,
incremental reads, attach support, PTYs, resize, and static network transport
now exist. Those additions do not retroactively strengthen the historical G3
manifest, and several are covered only by narrow unit or shared-client tests.

The 2026-09-04 implementation audit closes the previously missing local work:
bounded streaming/backpressure, exec and exit/failure races, reconnect, measured
capabilities, fixed bundle resolution, strict relay/transport qualification,
and explicit final-reply poweroff are implemented and race-tested. The final
child proves non-root supplementary identity, the safe authentication/framing
matrix, reconnect across actual relay loss, measured capabilities, and the
pinned child-poweroff transition. Mediated writable-root shutdown belongs to
G4 and is not implied by G3.

The first 2026-09-04 live attempt completed authenticated process work and
caused the child to report shutdown and return to Kerf `loaded`, but exposed a
primary-side cleanup defect: the final controller-owned C relay returned to its
listen loop after the child endpoint disappeared, so `mk-agentctl` blocked in
`Wait`. The harness was safely interrupted and reclaimed the child. The
controller now explicitly kills and reaps each owned reconnect relay. Current
code places the relay in a dedicated process group, bounds every complete
controller exchange to 30 seconds, and has focused blocked-write, blocked-read,
and descendant-reap tests. The corrected historical rerun then completed, and
its
[manifest and lifecycle JSON](../../../evidence/runtime-20260904/g0-g3-final/README.md)
retain the negative matrix, exact process result, reconnect, final Shutdown
reply, child poweroff/`loaded` transition, resource return, and token-redaction
check. This closes G3 only at the documented trusted direct-bundle boundary.

The final staged secret scan exposed a capture-scope mistake: the G3-local
exact-token check passed, but primary-kernel dmesg files elsewhere in the run
still contained five `mk.token` command-line occurrences across four files.
Those values were replaced with `[REDACTED]` before commit. The evidence set
retains the original remote checksum ledger, a redaction log, and a new checksum
ledger for the sanitized copy. Future capture review must scan the entire run
root, including primary dmesg, rather than treating the G3 subdirectory as the
credential boundary.
