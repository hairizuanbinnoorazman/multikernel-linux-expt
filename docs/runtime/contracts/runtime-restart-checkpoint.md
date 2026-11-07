# Private runtime restart and child Docker continuity

The disposable module-trial driver accepts only the named `pressure`,
`runtime-restart`, `runtime-crash` or `runtime-crash-during-build` scenario through
`MK_MODULE_TRIAL_SCENARIO`.
The recovery scenarios select normal SIGTERM or forced SIGKILL private-daemon
replacement after the first complete child build. They do not restart
the installed primary services. Record-fault modes cannot be mixed into this
scenario, and unsupported scenarios/paths refuse before trial mutation.

The workload completes its original child-local build matrix and holds the
child alive. Before and after the private runtime restart it samples child boot,
Docker engine, exact running owner and saved-archive identity. The replacement
runtime must have a different PID in the same boot and adopt the original child.
A second build matrix executes through the recovered control plane, while the
original saved archive remains byte-identical. Caller-selected recovery policy
requires the exact signal/status and matching lifecycle crash boolean in the
version-2 checkpoint. Normal and crash evidence cannot inherit one another's
scope. Legacy checkpoints remain excluded. The in-flight case uses its own
version-3 contract described below.

Same-boot transport reuse accepts an explicitly named, pinned pressure or
recovery predecessor. Recovery predecessors bind their exact normalized view,
including boolean types, to their scenario. The driver still requires current
completed unit epochs and empty cgroups, unchanged primary state and idle
resources. Before a recovery predecessor can authorize a new trial, its original
source archive and actual recovery files are requalified. This does not unload
the candidate transport, recover an older failed generation or claim current
admission from an archived receipt alone.

The build probe emits an exact image-ID marker before removing its build
resources. Primary Docker observations around restart and after rebuild require
both exact built-image absence and unchanged engine/inventories. The child
engine must differ from the primary. The recovery checkpoint binds these
observations, the runtime exit tuple and both bounded post-recovery streams.
The parent reads the exact root lifecycle receipt and independently qualifies
continuity, original archive preservation, successful rebuild and isolation;
matching recomputed digests cannot mask a changed child/owner or primary engine.

The postrun audit selects its scenario explicitly and repeats receipt/reference
checks plus normal native release/offline checking, current resource return and
primary file/service/Docker/network parity. Recovery snapshots themselves always
leave cleanup unqualified. This scenario collects no ENOSPC/v8/v2 hold sequences;
pressure qualification and readiness remain separate. Current source uses direct
storage staging with private unit scratch, without a host-wide stage alias.

Local controls pass canonical recovery, wrong boot/invocation/signal/PID,
child/engine/owner/archive drift, duplicate/oversize input, missing build markers
and forged image absence even with recomputed receipt hashes. The
[actual GCE run on boot 5ad](../../../evidence/runtime-20261107/g8-live-runtime-restart-d3d2074/README.md)
passes source `d3d20749`, both exact unit invocations and the separate postrun
audit, including native release/offline checking and current resource return.
The five recovery/four audit/four layout control groups also pass as root.
Completed units retain empty leaf cgroups; candidate transport remains loaded
and idle pending cold restoration. Full G7/G8 remain open. Direct
child kernels use no nested virtualization and share primary trust after
arbitrary child-kernel compromise.

The forced-crash extension passes local signal/status/scenario, receipt drift,
engine drift and predecessor scope/type controls; disabling only the crash
receipt flag check is detected. [Actual GCE forced-crash proof](../../../evidence/runtime-20261107/g8-live-runtime-crash-bcd1139/README.md)
passes source `bcd11391`, both exact unit invocations and independent postrun
audit. It preserves and requalifies the original normal-restart predecessor
without transport reload, completes a second child build and normal release,
and passes eleven actual-input refusal controls plus default-normal-policy
refusal. Seven recovery/four reuse/four audit/four layout groups pass as root.
The fault occurs after the first build while its child stays alive. In-flight
build crash, other components, wider recovery/concurrency, cold restoration and
full G7/G8 remain open.

## Crash during the original child build

`runtime-crash-during-build` selects the existing 45-second BuildKit RUN overlap
probe. Version 3 binds raw stderr snapshots before kill, after kill and after
adoption. Each must contain exactly one executed BEGIN marker and no END, be an
exact prefix of the original final stderr, and bind the corresponding old/new
runtime PID. Strict monotonic timestamps must bracket the observed SIGKILL/-9
exit, order adoption after exit, and fit inside the 45-second probe window.
Echoed Dockerfile commands do not count as executed markers. Matching hashes
alone cannot qualify an unrelated or already finished RUN snapshot.

The original final stderr must contain executed BEGIN then END, and the original
stdout must contain the complete build matrix and exact first image ID. These
streams bind through the original workload checkpoint and lifecycle receipt.
Before/after-adoption child snapshots intentionally have no saved-image digest:
the first archive does not exist yet. Completed-first and post-rebuild snapshots
must keep the original child/engine/owner, and the saved first archive must be
preserved after the second build. Primary engine/inventory is sampled around
kill/adoption; exact image absence is checked after original completion and
after rebuild. The new scope cannot inherit normal or after-build crash evidence.

Nine recovery/four reuse/four layout control groups pass locally, including
recomputed-hash finished/unbound/reordered/replaced RUN refusals. Actual in-flight
crash execution and independent postrun qualification remain pending. No nested
virtualization or stronger child-kernel containment is introduced; full G7/G8
remain open.
