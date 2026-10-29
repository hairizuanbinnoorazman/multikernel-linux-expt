# Bounded structured diagnostic bundle

`mkruntimed --diagnostic-bundle=/absolute/private/new.json` saves a version-1
JSON bundle with scope `operational-and-component-observation-snapshot`. It
collects the separate compiled-component report first, then the version-7
[operational report](operational-doctor.md), under one configured backend timeout.
This preserves build-label diagnostics when resource inspection is unsupported.
The reports are separate samples; no equality between them, atomicity, process
incarnation or readiness is inferred.

Create an owner-only directory explicitly before collecting:

```sh
sudo install -d -m 0700 /var/lib/mkruntime-diagnostics
sudo mkruntimed --config=/etc/mkruntime/config.json \
  --network-socket=/run/mknetd.sock \
  --diagnostic-bundle=/var/lib/mkruntime-diagnostics/incident-001.json
```

Use a fresh filename for each attempt. The parent must already exist, be owned
by the effective caller and have mode 0700. The command never creates parents or
adjusts permissions. All path components must be free of symlinks. The canonical
absolute destination must have a simple ASCII basename ending in `.json` (at
most 128 bytes). Relative paths, traversal, an explicitly empty destination and
conflicting inspection/format modes exit 2. The network socket must be absolute.
These argument checks precede configuration load.

The bundle contains UTC start/finish times, the two structured reports and an
explicit exclusion list. It does not collect raw logs, guest stdout/stderr,
kernel console, environments, process command lines, configuration or durable
state files. The doctor APIs negotiate and validate responses through existing
protected sockets and retain fixed failure codes rather than raw errors. Existing
bounded kernel readers remain resource observation sources. Identifiers, boot
IDs, capacity, build labels and opaque descriptor fingerprints can be sensitive:
this is protected operator material, not an automatic public export or an
anonymization service.

The entire encoded bundle, including the newline, is capped at 8 MiB. Oversize
encoding publishes no truncated artifact. Publication uses a held parent
descriptor, mode 0600, file synchronization, exclusive rename and directory
synchronization. Existing files, symlinks, directories and competing collectors
are never overwritten. Observed parent substitution is refused before publication
or reported afterward; descriptor anchoring prevents write redirection. A sync
error or observed post-publication substitution can leave a completed artifact:
preserve it and inspect the original directory before retrying with a new name.
The command never recursively cleans up on failure. Filesystem synchronization
may outlast the inspection deadline; no filesystem I/O latency, constant-memory
or atomic cross-daemon snapshot guarantee is made.

Unsupported, unavailable, canceled and degraded observations are saved, including
after the inspection deadline expires. They never become successful empty
inventories. Standard output contains a version-1 receipt with scope,
`collection_status: captured`, exact byte count, SHA-256 and aggregate
`observation_status`. Degraded severity takes precedence over inconclusive, then
consistent. Exit 0 requires publication and both observations consistent;
otherwise exit 1. Publication failure produces a fixed stderr error and no
receipt. A captured receipt proves publication of the reported bytes, not health.
Compare `sha256sum` of the artifact to the receipt before sharing or archiving.

All report-level unobserved fields and false attestation/admission flags remain.
This does not establish native storage/network health, service readiness,
executable/kernel provenance, Docker readiness or containment. Full G8 still
requires health/readiness, additional safe diagnostic sources, log retention and
pressure, and recovery/failure qualification. Direct child kernels require no
nested virtualization and retain shared host trust after child-kernel compromise.

[Local controls](../../../evidence/runtime-20261029/g8-diagnostic-bundle-controls/README.md)
pass the full runtime race suite and bounded publication/redaction controls.
[Actual GCE proof](../../../evidence/runtime-20261029/g8-diagnostic-bundle-gce/README.md)
passes idle collection with installed legacy services and actual current private
daemons, exact original-artifact identities/modes, refusal, shutdown and primary
preservation. These idle bundles are correctly inconclusive after pool release;
[Actual active Docker collection](../../../evidence/runtime-20261029/g8-live-diagnostic-bundles/README.md)
subsequently passes ENOSPC/build/removal capture, exact receipt/artifact checks,
engine/image isolation and normal cleanup. Its pinned budget scope follows below.
Its [cold restoration](../../../evidence/runtime-20261029/g8-live-diagnostic-bundles-cold-restoration/README.md)
also verifies original transport, retained files/images and exact live bundle
artifact preservation. Neither result closes the full G8 health/diagnostics gate.

The Docker pressure fixture now collects a bundle during ENOSPC, the built-image
hold and after removal. The strict bundle checkpoint requalifies complete resource/pool/
component references before comparing nested reports, receipt digest/size and
exclusion fields. It rejects forged internally consistent references, partial
payloads, numeric aliases, altered capacity/ownership/builds and health claims.
Local active/released fixture controls pass. Initial source `5412c4be` preserves
the inherited ten-second ENOSPC/removal budgets, but its built-image doctor shares
the remaining 180-second image-hold budget. Its evidence must retain that scope.
The subsequent common checkpoint explicitly clamps every phase to ten seconds
and retains monotonic start/deadline/finish/elapsed metadata, eight-report count
and a receipt digest for each timing artifact. Local timing controls reject
extended deadlines, elapsed-time mismatch and numeric aliases. Actual Docker
collection now passes for source 5412c4be with the scope above; corrected
shared-budget qualification remains pending. Its
[focused timing controls](../../../evidence/runtime-20261029/g8-diagnostic-checkpoint-timing-controls/README.md)
are separate from actual timed execution.
