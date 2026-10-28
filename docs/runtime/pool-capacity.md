# Read-only pool capacity diagnostics

`mkruntimed --doctor-pool --config=/etc/mkruntime/config.json` reports configured
CPU/memory limits and durable lifecycle claims alongside independent kernel
CPU-pool and memory-map observations. It negotiates `pool-ownership-v1` on the
existing protected daemon Unix socket; an installed legacy daemon refuses
explicitly. Version-5 `--doctor-all` remains unchanged and still lists pool
capacity as unobserved. Inspection never initializes, reconciles or repairs a
pool. All inspection modes are mutually exclusive.

The version-1 pool report brackets two complete ownership/kernel samples with
boot reads under one context budget. It refuses changed samples, canceled
requests, malformed or obscured kernel metadata and unsupported interfaces.
Configured limits and daemon pool-init status are sampled under the same
lifecycle lock as mutations; waiting for that lock honors cancellation. The
projection excludes bundle paths, arbitrary labels, credentials and raw errors.

The CPU reader requires the root kernel's explicit `cpus-available` property,
64-bit physical IDs, unique members and a valid free subset. It never assumes
all pool members are free. With no initialized pool, the current root tree
contains CPU 0 but no free-list property: the command reports
`KERNEL_CPU_FREE_LIST_UNAVAILABLE`, not zero verified capacity or a free child
CPU. CPU 0 is never a candidate. Incompatible older kernel trees also remain
inconclusive; no legacy inference is made.

The memory reader parses bounded `/proc/iomem` hierarchy, matches the canonical
pool resource name, rejects redacted address maps, merges nested/overlapping
allocation intervals without double counting, and reports total, allocated,
available bytes and the largest free registered extent. It does not expose
physical ranges or arbitrary resource names. Reads refuse symlinks, FIFOs,
unprotected files, changed file identities and oversized input.

For stable complete observations, the command checks configured policy,
duplicate/absent/free CPU claims, unowned kernel allocations, initialization and
memory overcommit. Any degraded observation suppresses all candidates.
`cpu_candidates` intersects configured, unclaimed and kernel-free CPU sets.
`memory_candidate_upper_bound_bytes` is the minimum of remaining configured
budget, kernel-free bytes minus reserve and largest registered free extent.
Configuration quantities use the daemon's parsed bytes; they are not replaced by
human Kerf quantities. Aggregate registered memory bounds do not attest allocator
internals, alignment, a reservation or success of the next allocation.

Only `status=consistent` exits zero. `inconclusive` or `degraded` exits one;
`admission_guaranteed` remains false in every report. A complete matching sample
is not an atomic snapshot or proof against ABA transitions. Service readiness,
CPU-transition health, kernel incarnation, native I/O health, Docker-engine
health and containment remain explicitly unobserved. These requirements remain
part of full G7/G8 acceptance.

Local controls cover sparse IDs, malformed/truncated FDT blocks, redacted maps,
allocation unions, fragmentation, ownership/policy drift, cancellation, legacy
RPC refusal and private metadata/error redaction. Fuzz runs are bounded controls,
not the full G7 fuzz campaign. The opt-in GCE driver reserves only a CREATED,
unloaded instance through production lifecycle code, observes capacity through
the production RPC and CLI, deletes that reservation and returns its pool.
It never loads/boots a child or invokes Docker. Its live qualification and a
fresh Docker build/pressure capacity checkpoint remain separate gates until
actual evidence is linked here.

Direct child kernels require no nested virtualization. They share host trust
after child-kernel compromise; capacity inspection adds no containment boundary.
