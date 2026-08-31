# Evidence contract (v1)

Each gate run creates `evidence/runtime-YYYYMMDD/<run-id>/manifest.json` plus
raw, immutable command output. The manifest records schema version, gate,
result, UTC start/end, repository commit and dirty state, component revisions,
host and GCE identity, commands with exit status and output path, assertions,
known limitations, and cleanup result.

Cloud runs also contain `resources-before.json` and `resources-after.json`
covering instances, disks, snapshots created by the run, addresses, and any
other billable resource. Each entry records project, zone/region, name, type,
purpose, ownership labels, auto-delete/retention policy, and final state. A
retained resource requires an explicit reason and operator acknowledgment.

Secrets, metadata tokens, private keys, full environments, OCI registry
credentials, and agent authentication tokens must be redacted before capture.
Raw historical transcripts are not reformatted after indexing.
