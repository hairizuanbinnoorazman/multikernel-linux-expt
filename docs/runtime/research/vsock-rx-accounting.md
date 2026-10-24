# VSOCK receive readiness accounting candidate

Pressure tests observed increasing WRITE-payload receive time and NBD timeouts.
The source tree available on the GCE instance implements `stream_has_data` by
walking every queued packet under its receive lock. Repeated readiness queries
therefore scale with queue depth. This is a concrete amplification mechanism;
its contribution to the live pressure failure still requires module-bound proof.

The [candidate patch](../../../patches/linux/linux-v7.0-mk2-vsock-rx-accounting.patch)
tracks queued unread bytes in a 64-bit counter under the existing receive lock.
It accounts for enqueue, temporary removal during copy, partial requeue, copy
failure rollback and cancellation. Readiness becomes a constant-time snapshot.
It preserves data order, forwarding count rollback and existing stream behavior.
It does not change queue admission, credit signaling, request limits or timeouts.
Receive-buffer admission remains a separate resource-hardening concern.

The [fixture](../../../scripts/fixtures/mk-transport-rx/README.md) binds the observed
source bytes. The required local control applies the patch in temporary scratch
and compiles its actual structure and queue functions. Independent traversal
checks partial reads, failed copies, an arrival while copying, cancellation and
a pending-byte count above 4 GiB. A deep-queue negative control observes 1,048,576
node visits for 64 polls of 16,384 packets; the candidate observes zero visits.
These are operation counts, not a live throughput benchmark.

Run `python3 scripts/test-kernel-vsock-rx-accounting.py`. Full kernel compilation,
module/source binding, real concurrency, live pressure and export-release checks
are still required. The canonical instance source tree and installed module files remain unchanged.
The private combined module is now undergoing a guarded live trial; pressure
qualification remains pending. The existing build script does not yet integrate
these patches.

The experiment uses direct child kernels without nested virtualization. It does
not establish containment after child-kernel compromise; see the
[deployment constraint](../architecture.md#deployment-and-containment-constraint).
Full G7/G8 remain open.

The first guarded module trial subsequently refuses before unloading anything:
four established VSOCK entries and four module references remain without process
owners after two failed cases, despite no child kernels or fixture cgroups.
The observed transport release callback sends shutdown but omits the core table
unlink operation. The [release patch](../../../patches/linux/linux-v7.0-mk2-vsock-release-sockets.patch)
calls `vsock_remove_sock`, which removes connected-table references and uses the
core's SOCK_DEAD rule to preserve bindings during transport reassignment.
The actual-callback control checks close states, allocation/send failure and
reassignment binding preservation against that inspected core API contract.
Its original-callback negative control retains a connected-table reference.
Kernel socket lifetime and module-reference return still need live evidence.
Forced module unload is excluded; existing orphaned state is not repaired by
changing source files or by the local fixture.

[Isolated combined compilation](../../../evidence/runtime-20261024/g8-vsock-release-module-build/README.md)
and the [busy-module refusal](../../../evidence/runtime-20261024/g8-vsock-orphan-guard/README.md)
are retained. A [clean administrative boot](../../../evidence/runtime-20261024/g8-clean-boot-observation/README.md)
clears the old orphan state without altering failed disk/ledger bytes. The lab
trial swaps only an unused fresh-boot transport, checks complete workload cleanup
before restoration, and restores through a separate idle reboot after traffic.
Concurrent callback/unregistration lifetime and general hot upgrade are not
qualified by this experiment. A reboot cannot substitute for the trial's zero
socket/reference, filesystem and resource cleanup requirements.
