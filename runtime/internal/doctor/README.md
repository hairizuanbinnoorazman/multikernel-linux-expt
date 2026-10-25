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
