# `mkruntimed`

Privileged node daemon planned by
[`../../../docs/runtime/plans/02-control-plane.md`](../../../docs/runtime/plans/02-control-plane.md).

This command is the sole runtime owner of Kerf mutations. Keep command-line and
service setup here; lifecycle and adapter logic belongs under `internal/`.

`sudo mkruntimed --config=/etc/mkruntime/config.json --inspect-ownership`
queries the running daemon through its existing protected Unix socket and exits
before opening durable state, initializing Kerf, or reconciling anything. The
negotiated `ownership-v1` view returns only schema version, snapshot sequence,
sandbox ID/generation/state, assigned APIC IDs, memory, child CID and agent port.
It excludes labels, paths, credentials and free-form errors. It refuses malformed
metadata and inventories above 256 sandboxes or 4,096 total CPU entries instead
of presenting a partial result as complete. Older daemons fail explicitly; the
client never falls back to unrestricted sandbox serialization.

The source is explicitly `durable-lifecycle`: these are recorded assignments,
not current child liveness, available capacity, complete storage/network leases,
or a no-leakage attestation. A read during a transition may show its intermediate
state. G8's unified doctor, service readiness and independent orphan detection
remain open. Apply the [deployment constraint](../../../docs/runtime/architecture.md#deployment-and-containment-constraint):
nested virtualization is unavailable and ownership output does not establish
containment after arbitrary child-kernel compromise.

`sudo mkruntimed --config=/etc/mkruntime/config.json --doctor` adds a read-only
comparison of negotiated durable ownership with bounded kernel instance names.
It exits before state locking, Kerf initialization or reconciliation. Two
ownership reads and two kernel inventories share one configured timeout; changed
sequence, generation, inventory or boot identity produces `inconclusive`.
Unavailable/oversized/malformed observations return stable codes without raw
remote errors or a partial ownership inventory.

The JSON scope is `durable-ownership-and-kernel-presence`. `consistent` (exit 0)
means those sampled names agree; `degraded` or `inconclusive` exits 1. Conflicting
CPU/CID assignments, missing owned names, released names still present and
unrecorded names are findings. Intermediate lifecycle states require rechecking.
Orphan candidates are inspection hints, never deletion authorization. Findings
retain the durable generation when one exists, but kernel names cannot verify
that generation: `kernel_generation_verified` always remains false. Sampling
cannot exclude an unobserved ABA transition. `unobserved` explicitly lists
storage exports, network endpoints, pool capacity, service readiness, child
Docker health and containment; `consistent` does not establish full runtime
health or no leakage. Use the existing protected socket; no repair is attempted.
