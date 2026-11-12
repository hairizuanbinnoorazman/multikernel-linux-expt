# Read-only audit of a disposable child Docker trial

`scripts/audit-runtime-docker-trial.py` audits a completed, source-bound GCE lab
storage-pressure or explicitly selected private-runtime recovery trial without
signaling processes, changing ownership, probing
writability or restoring transport. It is specifically for the existing pinned
`test-runtime-vsock-module-trial.py` and Docker-profile harness layout, CPU set
and candidate transport. It is not a general production readiness command.

From a fresh checkout containing the audit tool, run on the qualified lab host:

```bash
sudo python3 scripts/audit-runtime-docker-trial.py \
  --scenario pressure \
  --work /srv/multikernel-storage/stages/g8-host-SOURCE7 \
  --source-commit SOURCE40 \
  --build-sha256 TRUSTED_BUILD_JSON_SHA256 \
  --boot-id EXPECTED_BOOT_UUID \
  --parent-invocation EXPECTED_PARENT_INVOCATION32 \
  --workload-invocation EXPECTED_WORKLOAD_INVOCATION32
```

Replace every placeholder with independently retained launch/build identities.
Do not obtain the build pin solely from an untrusted destination. The tool
requires root-owned protected directories, a caller-pinned build manifest,
matching source archive/commit and programs, and archive-matching Python
acceptance sources before importing fixture-specific qualifiers. It accepts
only the exact same-boot completed parent/workload invocations. Refusals emit a
fixed message without exception text, input values or partial success output.

The audit checks three original private bundle identities and receipts, archived
component/storage/pool/operational references, journal/network health bindings,
shared timing evidence and its parent receipt binding. These are archived
observations accepted during the actual holds; the audit does not recreate a
deleted private endpoint or repeat a live native network CHECK after removal.
It also checks the build matrix, guest engine separation and exact child-image
absence, current primary watched files/service PIDs/Docker/firewall/link/route
parity, declared service capability/NNP observations, workload authority,
recorded native release/offline check and current private ownership/resource
return. Backing contents of older retained failures are outside its scope.

Fixture evidence inputs reject symlinks. Installed primary configuration may
use root-controlled immutable-deployment symlinks; their original/resolved paths
must remain protected and the sampled target must remain stable. This exception
does not apply to fixture inputs. Root compromise or unobserved ABA changes are
not excluded by sampled file identity/hash checks.

An exited `RemainAfterExit` unit can retain an empty leaf cgroup. The tool
requires `populated=0`, no PIDs and no descendant groups and reports
`retained_empty_systemd_cgroup=true` separately from absence. A populated,
aliased, replaced or nested cgroup refuses. It also checks for remaining owned
processes through cgroup/executable observations. It neither stops the completed
unit nor relabels the retained empty directory as absent.

JSON output contains only fixed outcome flags, caller-pinned identities and the
maximum diagnostic sequence duration. Raw receipts, state, logs, guest streams,
backing paths, inventories and internal fingerprints stay on the host.
Successful audit explicitly leaves cold restoration and full G7/G8 unqualified.
The audit independently rechecks the retained private `docker.json` logging
defaults, `docker-info.json` driver and final container `LogConfig` against the
[trial output policy](trial-output.md). Concurrent checkpoints also recheck
both containers' log configurations. Missing or altered policy refuses even
when workload markers and receipt hashes otherwise match. Attached streams
at the 131072-byte cap refuse in both ordinary and concurrent acceptance.
These archived observations do not establish actual rotation behavior or
cryptographically attest an honest Docker daemon.
The [committed-source and native controls](../../../evidence/runtime-20261112/g8-log-policy-audit-corrected/README.md)
reproduce ten baseline acceptances of altered policy/exact-cap evidence and
verify candidate refusal while preserving valid cases. All ten native tests
pass without skips after an explicitly constructed writable-parent fixture
replaces its `/tmp` assumption. The failed predecessor remains preserved.
This qualifies audit acceptance controls, not a new actual child trial.
Select `runtime-restart` or `runtime-crash` explicitly for a recovery trial;
these bind the corresponding exact signal/status, actual recovery inputs and
parent-normalized view rather than claiming pressure/health hold evidence.
Their output distinguishes normal restart and forced-crash build continuity.
Select `runtime-crash-during-build` for the version-3 in-flight contract. It
additionally binds the three unfinished RUN snapshots to the original completed
build streams and exact kill/adoption ordering; its output has a separate
`runtime_inflight_crash_build_continuity_qualified` flag. Original workload
stdout/stderr hashes are verified for every scenario. Reuse and audit refuse
unannounced cgroup paths, including dangling aliases, before claiming absence.
`runtime-crash-during-io` selects the [version-4 synced-write contract](synced-write-runtime-crash.md).
Its distinct `runtime_synced_io_crash_continuity_qualified` flag requires a
confirmed-dead progress baseline, new completed synced writes during the outage,
post-adoption progress, known-data readback and native written-byte coverage.
The guest probe and write-loop inputs also bind to their committed archive bytes.
Direct child kernels need no nested virtualization; arbitrary child-kernel
compromise still shares primary trust. This is not a hostile-kernel containment
proof or proof of general Docker build compatibility.

[Actual GCE proof and negative controls](../../../evidence/runtime-20261106/g8-live-docker-boot6349/README.md)
retain the successful current-source lifecycle, all four root controls, wrong
boot/invocation/build-pin refusal and detection of a separately disabled epoch
guard. Earlier failed ownership remains preserved and unrecovered.

For new health-enabled receipts the audit additionally requalifies v8 health/v2
bundles, private endpoint snapshots, original product identities and shared
health timings against the exact parent record. `live_health_pair_qualified`
reports this scope; legacy receipts cannot inherit it. The private trial scratch
identity check is reported separately as `trial_scratch_private_mount_qualified`.
[Actual warm proof](../../../evidence/runtime-20261106/g8-live-health-holds/README.md)
passes both with root controls and a forged-receipt mutation comparison.
