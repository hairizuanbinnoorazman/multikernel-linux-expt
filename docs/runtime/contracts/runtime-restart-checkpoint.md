# Private runtime restart and child Docker continuity

The disposable module-trial driver accepts only the named `pressure`,
`runtime-restart` or `runtime-crash` scenario through `MK_MODULE_TRIAL_SCENARIO`.
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
scope. Legacy and crash-during-build checkpoints remain excluded.

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
receipt flag check is detected. Its actual live qualification is pending.
