# Rootfs RPC failure replies

Source `e467a53b` replaces raw rootfs service errors at the daemon RPC boundary:
PrepareRootfs, ValidateRootfs and CleanupRootfs return fixed stage descriptions.
Backend command output, paths, wrapped filesystem errors and credentials are not
formatted into these failure replies. Failed preparation also omits its result
body. Successful preparation/validation/cleanup responses retain their shape.

Service failure code remains `FAILED_PRECONDITION`, retryable false. Unsupported
service and malformed request cases retain `UNSUPPORTED` / `INVALID_ARGUMENT`.
The service implementation still retains its original internal errors and rollback,
reconciliation and ownership semantics; this changes the RPC producer only. A
fixed external description does not establish that rollback completed. Inspect
recorded ownership before deciding recovery, particularly after uncertain unmount.

[Local proof](../../../evidence/runtime-20261104/g8-rootfs-rpc-controls/README.md)
exercises real daemon wire framing and concrete rootfs service/store behavior with
a controlled backend. Validate, mount, build, uncertain unmount and cleanup errors
carry repeated synthetic private data. Tests verify error/body omission, completed
rollback, retained MOUNTED/PREPARED ownership, reconciliation, exact-identity retry,
stale cleanup refusal, missing bundle paths and malformed/unsupported classification.
Daemon/rootfs race suites, full runtime race and required checks pass outside the
sandbox's synthetic permission view; its original refusal remains separately retained.
The separate [GCE v2 fixture](../../../evidence/runtime-20261104/g8-rootfs-rpc-gce/README.md)
also passes, preserving all 183 artifacts and its failed predecessor. These
fake-backend controls do not qualify actual Linux mount/child-Docker failures.

Task errors, other producers, aggregate diagnostic pressure/retention, provenance/
readiness and full G7/G8 retain separate scope. No nested virtualization is used;
direct child kernels share host trust after child-kernel compromise.
