# Read-only durable storage ownership

`mkruntimed --config=/etc/mkruntime/config.json --inspect-storage` queries the
running daemon through the existing protected Unix socket. It negotiates
`storage-ownership-v1`, then calls `InspectStorageOwnership` with an absent body
or empty object. Unknown/duplicate fields, null and non-object bodies refuse.
The capability is advertised only when a storage service is configured.
Unsupported peers fail explicitly; there is no fallback to serializing private
exports. The CLI exits before opening stores, initializing Kerf or reconciling.
Its capability and report calls share the configured caller deadline.

A successful report has version 1, source `durable-storage` and a sorted `exports`
array. Each entry contains only:

- Sandbox ID and sandbox/export generations, recorded peer CID and state.
- Filesystem UUID, size/quota, inode limit, port and recorded device/inode identity.
- A validated offline-check digest and recorded read/write/flush close counters.

Backing paths, image names, source-image digests, timestamps, labels, credentials
and free-form errors are excluded. Records sort by sandbox ID and generation;
duplicates, invalid metadata and conflicting live claims refuse. A RELEASED
legacy record can carry peer CID zero, meaning its historical binding is unknown.
The view never invents that binding or turns it into live authority.

The store checks its 256-record cap before copying records. It uses a nonblocking
store lock: busy persistence returns retryable UNAVAILABLE, and oversize state
returns RESOURCE_EXHAUSTED rather than a partial inventory. Invalid durable
metadata returns OPERATOR_ACTION without reflecting the malformed value.
The projection does not acquire the service-operation lock, call a backend,
reopen state or mutate it. Existing socket authentication and frame limits apply.

These are recorded leases, not current native-process, image, socket or lifecycle
observations. Live-state close counters remain zero until release; zero does not
mean that an active export has performed no I/O. A clean recorded release does
not recheck the filesystem. There is no cross-subsystem atomicity or independent
orphan/liveness attestation in this metadata view. The retained
[version-2 doctor](resource-ownership.md) correlates recorded storage/lifecycle expectations, generations, CID and complete
descriptor fingerprints. It explicitly leaves native storage health unobserved.
The separately negotiated [version-3 doctor](native-resource-identity.md) adds
sampled native process/backing identity; full session/content health remains
unobserved. History retention/pagination beyond this bounded view remains a G8 requirement.

The [deployment constraint](../architecture.md#deployment-and-containment-constraint)
uses direct children without nested virtualization. This view helps an operator
inspect ownership; it does not establish containment after child-kernel compromise
or close complete G7/G8.

## Disposable Docker trial admission

Before switching the test transport or creating private Docker integration
state, the current cold-trial driver and Docker harness record a descriptor-based
minimum-capacity observation. One image needs at least 2147483648 image bytes
plus the existing 1073741824-byte reserve and 1028 available inodes; the two-image
scenario needs 4294967296 image bytes plus that reserve and 1032 inodes. Failure
writes `MINIMUM_FREE_BYTES` and/or `MINIMUM_FREE_INODES` before refusal. Symlinked
storage path components are rejected. Passing this observation grants no storage
lease, reserves no capacity, and does not account for source staging or concurrent
consumption. Full builder admission remains mandatory.

[Actual GCE proof](../../../evidence/runtime-20261027/g8-trial-capacity-admission/README.md)
qualifies refusal before transport or Docker mutation at the current shortage.
This is an early-refusal subcase, not a live build, storage recovery, normal
cleanup or complete G7/G8 acceptance. Failed images remain retained.
