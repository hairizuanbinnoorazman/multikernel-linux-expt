# Observed Multikernel VSOCK source fixture

`mk_transport.c` was read from `/home/hairizuan-tw/src/linux/net/vmw_vsock/mk_transport.c`
on the GCE laboratory instance, boot `1bcdcae0-428b-435e-aff8-02d7637dc7ba`.
SHA-256: `a877cbe5f1306c3068b80074750045ca4c362ea18adb1ac0493e78ece317707e`.
The original SPDX/copyright header is retained. This fixture identifies the
observed source bytes, not their binding to a particular loaded module build.

`test-kernel-vsock-rx-accounting.py` applies the candidate patch to a temporary
copy, compiles the actual structure/queue functions with userspace kernel stubs,
and compares their results against an independent queue traversal. Its negative
control compiles the original readiness function and counts actual node visits.
It does not model kernel locking, allocation failures, IPI delivery or live
throughput. The source fixture is never installed or compiled into the runtime.
