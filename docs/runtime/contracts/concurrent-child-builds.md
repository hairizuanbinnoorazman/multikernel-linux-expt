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
recomputed hashes. Actual current-source concurrent execution remains pending.
Cold restoration and full G7/G8 remain open. No nested virtualization is used;
arbitrary child-kernel compromise shares primary trust.
