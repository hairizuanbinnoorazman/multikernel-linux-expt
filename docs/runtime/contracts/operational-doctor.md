# Version-7 resource and compiled-component observations

With qualified runtime/network daemons, select the combined operator format:

```bash
sudo mkruntimed --config=/etc/mkruntime/config.json --doctor-all \
  --doctor-format=7 --network-socket=/run/mknetd.sock
```

`--doctor-all` defaults to version 5. Explicit format 5 preserves that contract;
format 6 selects the same contract as `--doctor-all-pool`. Format 7 adds the
version-1 [compiled build views](component-builds.md) to the version-6
[resource/network/pool view](../pool-capacity.md#unified-inspection).
The format option requires `--doctor-all`; it cannot modify another inspection
mode or silently start a daemon. Unsupported formats and relative network
sockets are rejected before loading configuration. Inspection exits before
state locking, reconciliation, Kerf initialization or resource allocation.

The new scope is `resource-network-pool-and-compiled-component-observations`.
One original caller deadline bounds two complete rounds of both nested
inspections, including each nested inspector's own repeat and boot checks.
Every complete report must identify the same boot. Full resource/lifecycle/
sequence and kernel observations and both build reports must remain equal across
rounds. Missing, unsupported, invalid or observed drifting views produce an
inconclusive result with neither top-level component payload. Findings use
stable codes and bounded ownership identities, never raw errors or credentials.

Complete degraded/inconclusive views retain their findings, with degraded
severity taking precedence. Aggregate capacity candidates are suppressed unless
the overall status is consistent, including when a compiled revision is unknown.
Nested candidate fields can therefore be suppressed relative to a standalone
pool query. Stable known differing compiled labels remain visible; compatibility
is unobserved, and label equality does not attest an executable. The revision
field identifies the CLI, while the nested component reports identify only
compiled daemon labels.

`admission_guaranteed`, `artifact_verified` and `kernel_verified` always remain
false. Consistency and exit zero mean scoped observations, not health or
readiness. Executable/process/other component identity, compatibility, kernel
identity/incarnation, CPU transition health, allocator internals, native I/O
health, descriptor intent, service readiness, Docker health and containment
remain unobserved. Repeated equality cannot exclude ABA changes or a same-build
daemon restart. A released idle pool can have no free-list metadata and return
inconclusive; never create resources to make diagnostics green.

Local controls cover complete and degraded views, unknown/mixed labels, drift
between the outer rounds, private/legacy failures, boot mismatch and cancellation.
Actual version-7 GCE/Docker pressure/build/removal qualification remains pending;
prior version-6 Docker and idle component proofs keep their separate scopes.
Direct child kernels use no nested virtualization and share host trust after
compromise. This interface grants no repair or cleanup authority.
