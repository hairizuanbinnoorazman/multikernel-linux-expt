# Evidence contract (v1)

Each gate run creates `evidence/runtime-YYYYMMDD/<run-id>/manifest.json` plus
raw, immutable command output. The manifest records schema version, gate,
result, UTC start/end, repository commit and dirty state, component revisions,
host and GCE identity, commands with exit status and output path, assertions,
known limitations, and cleanup result.

Cloud runs also contain `resources-before.json` and `resources-after.json`
conforming to
[`gce-resource-ledger-v1.schema.json`](schemas/gce-resource-ledger-v1.schema.json).
The ledgers cover all project instances, disks, snapshots, reserved addresses,
and firewall rules, including instance-to-disk auto-delete relationships,
labels, location, users, and final state. This project-wide scope makes both
resources created by the run and pre-existing retained resources visible. A
retained resource requires an explicit reason and operator acknowledgment.

Secrets, metadata tokens, private keys, full environments, OCI registry
credentials, and agent authentication tokens must be redacted before capture.
Raw historical transcripts are not reformatted after indexing.

Run a live harness through `scripts/capture-evidence-command.py OUTPUT --
COMMAND ...`. The runner exclusive-creates a mode-0600 transcript, streams the
same combined stdout/stderr to the operator, records UTC boundaries and argv,
redacts values of visibly sensitive argv options, preserves the command's exit
status, and fsyncs the completed transcript. It does not attempt to recognize
secrets printed by the child; evidence harnesses must continue to suppress
credentials, authentication tokens, and full environments at their source.

The strict machine-readable form is
[`evidence-manifest-v1.schema.json`](schemas/evidence-manifest-v1.schema.json).
It supports G0 through G10 and requires structured repository, component,
host, command, assertion, limitation, and cleanup records. In particular,
every command records argv, exit status, and an output path, and every
assertion names its evidence paths.
