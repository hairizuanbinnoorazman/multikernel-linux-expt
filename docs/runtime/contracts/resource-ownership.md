# Correlated recorded resource ownership

The retained doctor report version 2 uses negotiated `resource-ownership-v1` and
InspectResourceOwnership through the existing protected daemon socket. Older
separate ownership and storage RPCs retain their existing formats. The new
client refuses unsupported or malformed combined views without substituting a
partial legacy report. Capability/report reads share the caller deadline.

The combined view contains version 1, source `durable-lifecycle-storage`, the
existing lifecycle and storage projections, explicit lifecycle storage bindings
and export descriptor fingerprints. Each lifecycle binding says whether storage
is required and records its export generation when known. Ordinary sandboxes
without mediated storage are valid. Released historical leases hold no live
authority and do not become orphan candidates solely because their owner exited.

A descriptor fingerprint covers path, image ID, filesystem UUID, source digest,
size, quota, inode limit and port. SHA256 uses the versioned domain
`mkruntime-storage-descriptor-v1` followed by a NUL, four strings in the listed
string-field order with unsigned 64-bit big-endian byte lengths, then the four
integer fields as unsigned 64-bit big-endian values. Raw string bytes are hashed;
invalid UTF-8 is not normalized and field boundaries are unambiguous. Only the
fingerprint is returned; private paths, image names, source digests, labels,
credentials and free-form errors are not serialized. A fingerprint is neither
a capability nor authority to repair, release or reuse a lease.

Lifecycle and storage projections fail promptly on busy persistence locks and
refuse oversized or invalid inventories. The lifecycle projection copies only
allowlisted metadata, not private result/label objects. The service brackets its
storage snapshot with lifecycle projections. The doctor requires two equal
combined views around its kernel and boot observations; observed drift is
inconclusive. This detects sampled changes, not an atomic cross-store transaction
or unobserved ABA. No backend, reconciliation or mutation lock is invoked.

Stable metadata mismatches yield degraded findings for missing required leases,
orphan live leases, sandbox/export generation, CID or descriptor mismatches,
unexpected leases and releases inconsistent with a live owner. Transitional
lifecycle/lease states require rechecking. Findings preserve claimed sandbox and
export generations; a sandbox-generation mismatch also records the expected
current generation. They are inspection hints, never cleanup authorization.

`consistent` means sampled recorded claims agree and kernel names match. It does
not probe native processes, current files/inodes, socket sessions, Docker health
or contents. The report explicitly leaves `native-storage-health`, kernel
incarnation, network, capacity, readiness, Docker and containment unobserved.
The [deployment constraint](../architecture.md#deployment-and-containment-constraint)
uses direct children without nested virtualization; hostile child-kernel
containment and complete G7/G8 remain open.

An internal LinuxBackend.InspectProcess primitive now exists separately from
Observe, which can remove stale records during recovery. It checks exact private
record and generation/CID identity, pidfd/proc incarnation, command, launched
executable inode and live backing-fd/current pathname identity. Record/runtime
identity checks bracket the sample. It creates no directories, initializes no
backend state, removes no records, takes no image lock, reads no mutable image
contents or logs, and runs no recovery/filesystem check. Only signal zero is
used; ordinary filesystem access-time changes remain possible. Missing records
or processes are inconclusive; a match grants no recovery authority and says
nothing about VSOCK sessions, progress, file contents or current executable-path
approval. The launched executable inode is the identity being checked.

The [GCE controls](../../../evidence/runtime-20261025/g8-native-process-inspection/README.md)
use a synthetic helper with real pidfds/proc and inherited backing file, retain
an initial fixture allocation failure and its correction, and prove unchanged
primary state. The [current version-3 contract](native-resource-identity.md) exposes this primitive
through a separately negotiated RPC and doctor report. Version 2 remains the
recorded-metadata-only historical view; full native-storage-health remains
unobserved in version 3 too. Live
native export qualification and complete G7/G8 remain open under the same direct
child, no-nested-virtualization deployment constraint.
