# Plan 02: Privileged control plane and Kerf adapter

## Purpose

Replace ad hoc shell orchestration with one recoverable node daemon while
keeping Kerf as the low-level resource manager.

## Minimum implementation

Implement `mkruntimed` with a versioned Unix-domain API:

- `NodeInfo` and `ListSandboxes`;
- `CreateSandbox`;
- exact-input `CancelCreateSandbox` for ambiguous create rollback;
- `LoadSandbox`;
- `StartSandbox`;
- `StopSandbox`;
- `DeleteSandbox`;
- `SandboxState`; and
- an event stream.

Add a Kerf adapter with explicit argv construction, sanitized environment,
bounded execution time, captured structured output, and no shell expansion.
Persist a write-ahead operation journal under a configurable state directory.
Use one global allocation lock and one lock per sandbox.

## Tests without Multikernel

- Fake-Kerf happy path and every nonzero exit.
- Timeout, truncated output, malformed state, and killed daemon.
- Duplicate create/start/stop/delete requests.
- Two concurrent allocations racing for the same resource.
- Restart at every lifecycle transition and deterministic reconciliation.
- Refusal to delete resources owned by an unknown operator.
- Filesystem permissions on the socket, journal, logs, and artifacts.

## Privileged tests

- One real child through the daemon only—no parallel manual Kerf mutation.
- Two children with disjoint resources.
- Daemon restart while children run.
- Shim/client disappearance while a child runs.
- Child failure during load, boot, and stop.
- Complete cleanup with CPUs and memory returned.

## Kerf upstream decision

After the CLI adapter works, list every parsing or atomicity weakness. Prefer a
machine-readable Kerf API contribution. Create a narrow fork only when a
required change cannot be accepted or tested upstream in time.

## Gate G2

Pass when all lifecycle operations are idempotent, daemon restart reconciles
known states, concurrent allocation cannot overlap resources, and every tested
failure either completes rollback or leaves a precise operator-action state.
