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
can contain CPU 0 or omit both CPU properties after release, with no free-list property: the command reports
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
is not an atomic snapshot or proof against ABA transitions. The revision field
identifies the CLI build, not negotiated daemon/kernel build provenance. An idle
unavailable free list is not itself a service-health classification. Service readiness,
CPU-transition health, kernel incarnation, native I/O health, Docker-engine
health and containment remain explicitly unobserved. These requirements remain
part of full G7/G8 acceptance.

Local controls cover sparse IDs, malformed/truncated FDT blocks, redacted maps,
allocation unions, fragmentation, ownership/policy drift, cancellation, legacy
RPC refusal and private metadata/error redaction. Fuzz runs are bounded controls,
not the full G7 fuzz campaign. The opt-in GCE driver reserves only a CREATED,
unloaded instance through production lifecycle code, observes capacity through
the production RPC and CLI, deletes that reservation and returns its pool.
It never loads/boots a child or invokes Docker. [Actual GCE qualification](../../evidence/runtime-20261028/g8-pool-capacity-gce/README.md)
passes configured/durable/kernel correlation, 4 GiB and 7 GiB candidate bounds,
exact CREATED deletion/pool return and unchanged primary/failed state. The [actual Docker pressure/build checkpoint](../../evidence/runtime-20261028/g8-live-docker-pool-capacity/README.md)
also passes with matching live ownership/kernel samples and normal cleanup.
Its [separate cold restoration](../../evidence/runtime-20261028/g8-live-docker-pool-capacity-cold-restoration/README.md) passes,
restoring the original transport and verifying all 93 retained files / 12 images,
primary persistent Docker inventories and retained failed-fixture state. The retained
[first status fixture](../../evidence/runtime-20261028/g8-pool-capacity-first-fixture-return/README.md)
and [released-root refusal](../../evidence/runtime-20261028/g8-pool-capacity-released-root-refusal/README.md)
explain corrections without relabeling those attempts successful.

Direct child kernels require no nested virtualization. They share host trust
after child-kernel compromise; capacity inspection adds no containment boundary.

## Unified inspection

`sudo mkruntimed --config=/etc/mkruntime/config.json --doctor-all-pool`
adds an explicit version-6 report that nests the qualified version-5 combined
and version-1 pool contracts. Both complete inspections run twice with one
absolute caller deadline. Their boot IDs and full lifecycle records (including
sequence and claims) must match, and both rounds must be equal. Unavailable,
legacy or drifting views produce an inconclusive result without partial payloads.
Complete degraded/inconclusive component observations retain their findings and
suppress pool CPU/memory candidates. Only a consistent aggregate can expose
candidates; admission remains false. Exit zero means consistent observations,
not readiness. The separate `--doctor-all` and `--doctor-pool` contracts remain
unchanged. Matching samples do not exclude ABA changes or prove atomicity.

[Retained local race/CLI controls](../../evidence/runtime-20261029/g8-unified-doctor-controls/README.md)
cover correlation, cross-view and between-round drift,
boot mismatch, legacy refusal, private errors, severity and cancellation.
[Actual version-6 Docker qualification](../../evidence/runtime-20261029/g8-live-unified-doctor/README.md)
now passes ENOSPC, built-image and post-removal holds with exact nested component
correlation and normal cleanup. The preceding separate-command proofs retain
their own scope. [Separate cold restoration](../../evidence/runtime-20261029/g8-live-unified-doctor-cold-restoration/README.md)
also passes and verifies the original transport, all retained contents and primary
persistent Docker state. Complete G7/G8 stays open.
