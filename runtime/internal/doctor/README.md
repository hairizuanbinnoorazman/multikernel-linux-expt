# Runtime presence diagnostics

`doctor.Inspect` compares bounded, validated `ownership-v1` reports with kernel
instance names. It uses stable failure codes and emits no raw daemon errors,
paths, environments, labels or credentials. The command's shared context bounds
all daemon calls. Kernel enumeration reads at most 257 entries and refuses more
than 256, absent inventory, aliases, unsafe names and non-directories.

This is one part of [G8](../../../docs/runtime/plans/08-operability-and-diagnostics.md).
Generation-bound durable records are preserved, but a kernel name alone does not
prove incarnation. The scope and unobserved checks are explicit in every report.
No mutation, reconciliation, repair, service restart or child execution occurs.

`InspectCombined` (version 5) runs native resource, network expectation and
endpoint reads in the same two-sample kernel/boot bracket. Each sample must use
identical lifecycle metadata across native and network-expectation views. A
single context bounds all reads; cancellation stops before later callbacks.
Component refusals and drift cannot expose a partial combined success. Complete
claims preserve existing storage/native findings while adding network findings,
with degraded severity taking precedence. Existing v3/v4 inspection stays separate.
The combined report still declares native health, incarnation, descriptor intent,
capacity, readiness, Docker health and containment unobserved.
