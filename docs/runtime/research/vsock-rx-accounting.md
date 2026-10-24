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
are still required. The patch has not been applied to the instance source tree,
loaded into a kernel or integrated into the existing module build script.

The experiment uses direct child kernels without nested virtualization. It does
not establish containment after child-kernel compromise; see the
[deployment constraint](../architecture.md#deployment-and-containment-constraint).
Full G7/G8 remain open.
