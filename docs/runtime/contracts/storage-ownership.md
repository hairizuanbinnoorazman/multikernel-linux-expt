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
