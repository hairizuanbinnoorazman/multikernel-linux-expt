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

The current version-2 JSON scope is `durable-resource-ownership-and-kernel-presence`. `consistent` (exit 0)
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

A valid capability response without `ownership-v1` now yields the distinct code
`OWNERSHIP_UNSUPPORTED`; unavailable or invalid observations retain
`OWNERSHIP_UNAVAILABLE`. Both exit 1 without a legacy fallback. Failed reports
retain the initially observed `boot_id`; `boot_stable` becomes true only after
the final matching boot read, so early failures cannot claim boot stability.

`sudo mkruntimed --config=/etc/mkruntime/config.json --inspect-storage` negotiates
`storage-ownership-v1` and prints a bounded `durable-storage` metadata report.
It exits before opening local stores, initializing Kerf or reconciling state.
The same protected Unix socket and caller deadline apply. A missing capability,
malformed reply, busy store or oversized inventory fails without a legacy
serialization fallback or partial success.

The view records sandbox/export generations, bound CID, state, filesystem UUID,
size/quota, inode limit, port, device/inode identity and validated release check/
counters. It excludes backing paths, image names, source digests, credentials,
labels and raw errors. At most 256 sorted exports are returned. A busy persistence
lock yields retryable UNAVAILABLE immediately; no backend operation is performed.
Released historical records may carry CID zero from an unbound legacy release;
the view never invents that binding. These are recorded claims, not current
process/image/socket health or independently verified lifecycle ownership.
`--doctor` still lists storage exports as unobserved until independent
correlation/liveness checks qualify that broader scope. The deployment uses no
nested virtualization; this view does not establish hostile-kernel containment.

Doctor version 2 requires negotiated `resource-ownership-v1`. Its combined view
adds explicit per-sandbox storage expectations, recorded export generations and
opaque fingerprints of the complete typed storage descriptor. It reports missing
required leases, orphan live leases, generation/CID/export-generation/descriptor
mismatches and releases inconsistent with a live owner. Ordinary sandboxes with
no storage expectation are supported; released historical leases hold no live
authority and are not mistaken for orphans. Intermediate leases require recheck.
Fingerprints use domain-separated, length-delimited raw bytes and integers; they
neither expose backing paths nor authorize recovery. Busy stores fail promptly.
Repeated combined views must match across kernel and boot observations. The
view is not an atomic transaction or ABA/liveness proof. Native storage health,
network, capacity, readiness, Docker health and containment remain unobserved.
Older daemons produce RESOURCE_OWNERSHIP_UNSUPPORTED without a partial fallback;
the older separate ownership/storage inspection RPCs remain unchanged.


The separate `--doctor-network --network-socket=/run/mknetd.sock` mode negotiates
explicit lifecycle network expectations and mknetd ownership, samples both twice
around kernel/boot reads, and emits version 4. It reports recorded ownership
mismatches, not native network health or cleanup authority. Legacy unknown intent
and sampled drift remain inconclusive; no fallback guesses required networking.
The existing `--doctor` output remains version 3. See the
[network ownership contract](../../../docs/runtime/contracts/network-ownership.md)
and [GCE proof](../../../evidence/runtime-20261027/g8-network-doctor/README.md).

`sudo mkruntimed --config=/etc/mkruntime/config.json --doctor-all --network-socket=/run/mknetd.sock`
emits version 5 with storage/native-process and network ownership in one
read-only command. It requires exact lifecycle agreement and repeated complete
component samples under one deadline. Unsupported/unavailable/drifting views
cannot become partial success. Existing degraded storage findings remain when
network findings are added. This is sampled correlation, not atomic inspection,
complete health or recovery authority. See the
[combined inspection contract](../../../docs/runtime/contracts/resource-ownership.md#combined-operator-inspection).
The existing version-3 and version-4 modes are unchanged.

[Controls](../../../evidence/runtime-20261027/g8-combined-doctor-controls/README.md)
and [GCE CLI preflight](../../../evidence/runtime-20261027/g8-combined-doctor-cli/README.md)
qualify the new command and explicit legacy refusal. The separate
[child attempt fails before the live hold](../../../evidence/runtime-20261027/g8-combined-live-failure/README.md);
no version-5 live-build or complete-health pass is claimed.


`sudo mkruntimed --config=/etc/mkruntime/config.json --doctor-pool` separately
negotiates configured pool limits/durable claims and correlates them with two
independent kernel CPU-free-list and memory-map samples. It refuses missing
metadata, legacy peers, changed samples and policy/ownership mismatch; degraded
state suppresses all candidates. Memory candidates respect configured reserve
and the largest free registered extent. `admission_guaranteed` is always false.
The [pool-capacity contract](../../../docs/runtime/pool-capacity.md) and
[actual GCE reservation qualification](../../../evidence/runtime-20261028/g8-pool-capacity-gce/README.md)
define its scope. Version-5 `--doctor-all` is unchanged and still marks pool
capacity unobserved; service readiness and complete G8 diagnostics remain open.
Direct child kernels use no nested virtualization and share host trust.
