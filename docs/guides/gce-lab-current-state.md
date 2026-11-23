# Current verified lab state

Latest recovery preparation `acb53c42` adds [stopped-drain provenance](../../evidence/runtime-20261123/g8-recovery-drain-provenance/README.md)
and separate counter validation for a future real-child worker fault. Local
controls pass; that candidate has not executed on GCE. Its read-only lab recheck
preserves all current and historical sets and the idle primary baseline.

The latest qualified native GCE code is the [exited-guest worker-recovery fix](../../evidence/runtime-20261123/g8-recovered-output/README.md)
at `2f4ba5a2`. Its committed-source native GCE controls kill the monitor at three
write boundaries, then verify remaining output drains before completion: 8,192
stdout bytes, 17 stderr bytes, exit 37 and a conservative 4,096-byte stdout replay bound.
Missing stdin does not prevent drain; replaced output identity and mismatched
wait PID refuse. All six cases and four existing recovery groups pass without
skips. The successful unit is stopped, scratch/cgroup removed, and 22 artifacts/
two absences are sealed. Two collector refusals are preserved without rerunning
the native workload. A [fresh read-only observation](../../evidence/runtime-20261123/g8-recovered-output/current-status.json)
rechecks this seal, the inactive unit, original idle transport/resources, primary
baseline, 200 GiB/5% reserve and all prior seals/metadata-only history.
This is native fault proof;
actual-child fault qualification of this source remains open. The latest actual-
child lifecycle remains the separately scoped `6a735215` proof below.

The [real-child finite-exec accounting proof](../../evidence/runtime-20261122/g8-child-output-accounting/README.md)
qualifies source `6a735215`. Host Docker selects the direct child; the complete
nested Docker build matrix passes. A held-child exec produces and delivers
8,192 stdout bytes and 17 stderr bytes, exits 37, and has zero guest drops, host
pressure drops and replay exposure. Before/after snapshots bind the source,
worker, bundle, unchanged tail prefix and one newly deleted exec. Five identity
refusals and ordinary-scope refusal pass. The counter-only recovery tail retains
at most 16 completed execs; missing or exhausted history cannot qualify zero loss.

Independent cold restoration passes after one authorized reset of the same
instance: 379 artifacts and 140 absences in four bounded manifests, plus two
approved static inputs, remain preserved. Four native daemon logs and historical
native backing trees use metadata-only verification. Earlier actual-child and
native/refused seals also survive, including the 329-artifact/118-absence
instrumented case, the 17-artifact guest-buffer case, both 14-artifact final-
accounting generations and the 14-artifact disposition case. Archived acceptance
uses the original boot; old completion refuses the new boot and grants no PID,
cleanup or reuse authority.

The original transport is idle, all CPUs/pool/children/refs/VSOCK resources are
returned, and a fresh primary baseline is stable. Primary files and persistent
Docker state are preserved. The 200 GiB filesystem retains its 5% reserve and
running-daemon binding. A [current read-only observation](../../evidence/runtime-20261122/g8-child-output-accounting/current-status.json)
rechecks the fresh baseline, seals, metadata-only logs, approved inputs, full
packet/eight binary hashes and capacity. The unchanged 8 GiB staging floor has
space available; full builder-peak admission remains separate. Source identities,
private paths, invocation/boot IDs and authority records remain in the lab.

The first native preflight refused its sandbox's read-only mount view before
running any test. Nine artifacts/five absences preserve that refusal. The
corrected generation retains sandbox protections, checks immutable inputs inside
it, and performs full host-storage/primary/history admission outside its mount
namespace. Twelve staged Python programs and six native Go groups pass without
skips, including the actual compiled inspector after a real native exec deletion.
Local full shim races and required checks pass, with five environment-dependent
repository cases skipped.

The separate 16 MiB log writer exits 0; two retained log files total 1,797,083
bytes within the 2,131,072-byte metadata bound. This does not qualify whole-writer
or Docker logger loss accounting. Real worker faults, broader producer coverage,
readiness, peak RSS and full G7/G8 remain open. Earlier proof keeps its own source
identity: [instrumented exhaustion](../../evidence/runtime-20261122/g8-diagnostic-child-binding/README.md),
[native dispositions](../../evidence/runtime-20261122/g8-host-output-disposition/README.md),
[final guest counters](../../evidence/runtime-20261122/g8-host-final-output-accounting/README.md)
and [guest buffers](../../evidence/runtime-20261122/g8-output-byte-accounting/README.md).

Authorized operators must revalidate the current private baseline, exact source/
binary identities, retained sets and idle resources before admission. Restart or
state change requires fresh verification. Direct children require no nested
virtualization and share primary trust after arbitrary kernel compromise. See
the [lab runbook](gce-lab-runbook.md) and [runtime plans](../runtime/plans/README.md).
