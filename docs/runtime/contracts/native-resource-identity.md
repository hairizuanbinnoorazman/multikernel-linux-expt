# Native resource identity inspection

`mkruntimed --doctor` version 3 requires negotiated `native-resource-identity-v1`
and `InspectNativeResources`. An older daemon yields an explicit unsupported
finding; there is no partial legacy fallback. Versions 1 and 2 remain historical
inspection helpers and retained evidence, not the current CLI acceptance contract.

The version-1 native report contains source
`durable-resources-and-native-process-identity`, the existing combined resource
projection and one ordered process observation per storage export. Each process
observation carries sandbox ID, sandbox generation, export generation and a
fixed status. No PID, path, command, image name, credentials or raw backend error
is serialized. Limits and validation from the combined view still apply.

The service copies validated private leases under a bounded persistence lock,
releases it before process inspection, and compares the complete private leases
afterwards. Lifecycle snapshots bracket the storage/process sample. Busy stores,
invalid/oversized inventories, cancellation and sampled ownership drift refuse
without partial output. No lifecycle mutation/global or storage-operation lock
is acquired, and inspection never calls the recovery observer.

Each non-released lease uses the exact read-only native process primitive:
protected record, generation/CID, pidfd/proc start time, exact command, launched
executable inode, inherited backing file and current named backing inode/size.
`process-matched` describes that sampled identity predicate. `record-missing` and
`process-absent` are observations of absence; `unavailable` covers refusals,
ambiguous identities and private backend failures. Released history is explicitly
`not-probed-released`; neither process absence nor current backing-file absence
is asserted for it. No cleanup or recovery authority follows from any status.

Doctor v3 repeats the complete negotiated view around its kernel and boot reads.
Different samples are inconclusive and omit partial resource/process inventories.
An ACTIVE lease with a stable missing record or process yields a degraded finding;
unavailable identity is inconclusive. Transitional leases require rechecking.
Existing metadata mismatches remain degraded even if native identity is unavailable.
Success includes `native_processes` in the report. Existing kernel-incarnation,
native-storage-health, network, pool, readiness, Docker and containment gaps remain
explicit in `unobserved`: process identity does not prove socket/session health,
I/O progress, storage contents or approval of a replaced executable pathname.
Sampling is not an atomic snapshot or proof against an unobserved ABA change.

The primitive creates no directories or records, deletes/quarantines nothing,
initializes no backend defaults, takes no image lock, reads no mutable image
contents/server logs, runs no filesystem checker and uses only signal zero.
Ordinary filesystem access-time changes remain possible. Private local metadata
is inspected through bounded descriptor-anchored reads; the caller deadline is
shared by negotiation and repeated observations.

[RPC controls](../../../evidence/runtime-20261025/g8-native-resource-controls/README.md)
and the [process primitive controls](../../../evidence/runtime-20261025/g8-native-process-inspection/README.md)
are separate from the [live child Docker qualification](../../../evidence/runtime-20261025/g8-live-native-resource-identity/README.md),
which matches active export identity at ENOSPC and built-image hold, then records
released history and independently qualified normal cleanup. Full G7/G8 remains open.
The [deployment constraint](../architecture.md#deployment-and-containment-constraint)
uses direct child kernels without nested virtualization. Children share the
primary's trust domain after arbitrary child-kernel compromise.

The private GCE harness has an explicit `--storage-record-loss` scenario;
`MK_MODULE_TRIAL_RECORD_FAULT=1` selects it through the pinned module trial.
It temporarily holds the exact active process record outside its normal name,
checks two generation-bound missing-record findings with unchanged private
ownership ledgers, then exercises the child-local engine while the record is
hidden. The same inode/bytes are restored before continuing normal cleanup.
The [fault controls](../../../evidence/runtime-20261025/g8-native-record-loss-controls/README.md)
cover refusal, client exceptions and conflicting restoration targets. The
[live GCE proof](../../../evidence/runtime-20261025/g8-live-native-record-loss/README.md)
qualifies both negative diagnostics, child Docker continuity, exact restoration
and later normal cleanup with unchanged primary inventories.

This is a metadata-loss test, not a native process kill or host-power-loss test.
A missing record does not establish that its process has died. Diagnostics must
not stop the export, clear its lease or reconstruct authority from a guessed PID.
The lab helper knows the exact held inode; it is not a production repair procedure.
An occupied restore target or modified held record remains preserved for operator
diagnosis. A killed injector can leave its held record behind. No daemon restart
or reconciliation is permitted while this deliberate fault is present. Follow
the recovery guide before acting on a real missing-record finding.
