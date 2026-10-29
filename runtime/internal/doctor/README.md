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

`InspectUnified` (version 6, `--doctor-all-pool`) repeats the complete version-5
combined view and version-1 pool view twice under the original caller context.
It requires identical full lifecycle records, including sequence, across the
views, matching boot IDs, and equality of both complete rounds. Missing or
unsupported components and observed drift suppress both component payloads.
Complete degraded or inconclusive observations retain their findings, but all
pool candidates are suppressed unless the aggregate status is consistent.
The nested contracts retain their existing scopes. The top-level contract
observes capacity in addition to the version-5 scope; it still does not establish
atomicity, readiness, admission, allocator internals, current kernel incarnation,
Docker health or containment. The revision identifies only the CLI build.
Direct child kernels require no nested virtualization and share host trust.

`InspectComponents` separately repeats negotiated compiled daemon build labels
under the original caller deadline and boot bracket. It refuses partial or
drifting views, preserves explicit unknown revisions as inconclusive, and shows
known differing labels without treating equality as compatibility or artifact
attestation. The inspector revision is kept separate. This adds no backend,
process execution, file-state inspection, readiness or containment claim; see the
[contract](../../../docs/runtime/contracts/component-builds.md). Direct child
kernels still use no nested virtualization and share host trust.

`InspectOperational` (version 7) repeats both complete `InspectUnified` and
`InspectComponents` views under the original context and a common boot bracket.
It refuses incomplete or outer-round drift without partial payloads. Complete
inconclusive/degraded observations retain findings and suppress capacity
candidates in a copied pool report. Known differing labels do not establish
compatibility; admission and artifact/kernel verification remain false. See the
[operator contract](../../../docs/runtime/contracts/operational-doctor.md).
Direct child kernels use no nested virtualization and share host trust.
