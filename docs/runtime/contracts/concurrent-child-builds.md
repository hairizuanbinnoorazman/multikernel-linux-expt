# Concurrent child builds and survivor continuity

`MK_MODULE_TRIAL_SCENARIO=concurrent` selects two direct child kernels through
the private host Docker engine. The parent and workload both require the
two-image capacity floor. Installed services and the default Docker runtime
are unchanged.

Each child seeds a private canary file and sentinel image. Recorded child boot,
engine, canary and sentinel identities must differ. Container IDs map uniquely
to durable generation-bound owners with disjoint CPU sets/CIDs. Native backing
inodes, filesystem UUIDs, ports, path fingerprints and export generations must
be separate and match the same owners.

Both original BuildKit RUNs must be unfinished around one bounded primary Docker
observation. Before/after raw snapshots bind to the original final stderr,
contain executed BEGIN but no END and fit a ten-second observation budget.
Echoed Dockerfile text or an archived boolean cannot establish overlap.
Each original build must finish its complete build/cache/run/save/failure matrix
under the declared outer and nested authority. Exact image absence and primary
engine/inventory parity are sampled after both builds.

The sibling is released and removed first. The remaining owner, kernel instance,
engine, canary, native backing and saved archive must survive unchanged. A new
complete build matrix then runs on that survivor; the original archive is read
back unchanged. Separate primary observation checks the new built image. Final
native release/offline records must match both original export generations and
owners. The postrun audit additionally checks current resource return and
primary file/service/Docker/network parity.

Version-2 concurrency evidence binds source/boot/invocation, both original
streams, overlap captures, primary observations and survivor rebuild. The
inspection helper grants neither cleanup nor process/signal authority. Archived
predecessor success still requires current exact unit epochs, cgroups, source
archive, actual checkpoint requalification and idle resources before reuse.

Local controls pass shared identity/backing/CPU refusals, owner/generation drift,
survivor/archive drift, missing builds, authority mismatch, primary image leakage,
finished/unbound overlap, wrong epoch/scope and timing limits, including
recomputed hashes. [Actual GCE proof on restarted boot 509d](../../../evidence/runtime-20261107/g8-live-concurrent-builds-1030d6f/README.md)
passes source `1030d6f3`, both exact invocations and separate postrun audit.
Both original matrices and the additional survivor matrix pass; overlap spans
116.523 ms around primary observation. Root controls, seventeen live refusals
and a live-data in-memory prefix-guard mutation pass. Retained prior-boot data
and the new-boot preservation receipt stay unchanged.

This closes the named two-child build-separation/sibling-release/survivor-rebuild
subcase. Concurrency churn, broader fault combinations, readiness, cold
restoration and full G7/G8 remain open. No nested virtualization is used;
arbitrary child-kernel compromise shares primary trust.
## Private command collection

The two-child trial's `invoke` path now enforces its existing strict
less-than-131072-byte stdout/stderr policy during collection, including nested
`docker exec`, survivor rebuild and failure diagnostics. The shared collector
uses a fixed trial profile; ordinary checkpoint callers keep their previous
stderr limits and option validation. Collection, EOF handling and exit wait use
one absolute deadline. On refusal the owned private process group is terminated
before its leader can be reaped and its PID reused; this is not arbitrary
descendant containment or peak-RSS qualification.

Each command reserves exclusive mode-0600 private stdout/stderr files, preserves
bounded partial streams and a fixed failure record, and publishes exit status
only after successful collection. Existing or aliased evidence refuses before
execution. Diagnostic timeout/overflow handling retains the original files
without appending markers or overwriting them. Terminal exceptions contain no
guest bytes or original command arguments. Expected nonzero queries still
return their private result; an unexpected failure emits a fixed error.

These controls harden the host-facing collection boundary. Actual new-source
two-child execution, aggregate lifecycle and full G7/G8 still require their
live proof; direct children use no nested virtualization and share primary
trust after arbitrary child-kernel compromise.
