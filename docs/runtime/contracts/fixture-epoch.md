# Fixture evidence across a reboot

`scripts/runtime_fixture_epoch.py` qualifies only a systemd unit status observed
in the same boot as its pinned receipt and exact invocation. It parses exactly
six requested properties, rejects missing/duplicate/aliased or malformed input,
and returns fixed codes without echoing private values.

A new boot takes precedence over a matching invocation string or successful
saved status. Missing transient units commonly report MainPID 0 and default
success/zero fields; those defaults are not evidence that an old unit completed
in the current boot. A changed or absent invocation also cannot inherit history.

`unit_completion_qualified` is process-status evidence only. It does not prove
cgroup/process census, namespace release, resource return, backing durability,
normal Docker cleanup or recovery. Those require their own same-boot checks.
Historical logs/receipts and pre-reboot observations stay attributed to their
original boot. A missing post-run audit stays missing after restart; current
absence must not reconstruct a pre-reboot cleanup result.

After a user restart, retain prior failed ledgers and source artifacts. Treat
old numeric PIDs, socket peers and mutation intents as prior-boot data, not live
signaling/reuse authority. Establish a fresh primary/resource baseline before a
new trial. Reboot termination does not convert a failed workload or incomplete
recovery into a passing normal-cleanup gate.

Direct child kernels require no nested virtualization and retain shared host
trust after child-kernel compromise. Full G7/G8 require separate qualification.

[Local epoch controls](../../../evidence/runtime-20261106/g8-fixture-epoch-controls/README.md)
reject a copied old success after removing only the boot guard. [Actual user
restart evidence](../../../evidence/runtime-20261106/g8-user-restart-6349/README.md)
records the old audit gap and a separately checked fresh baseline. Subsequent
[health-bundle qualification](../../../evidence/runtime-20261106/g8-health-bundle-gce/README.md)
passes its own current-epoch independent audit without rewriting history.
