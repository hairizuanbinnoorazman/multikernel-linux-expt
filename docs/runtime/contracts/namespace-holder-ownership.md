# Namespace-holder failure ownership

A namespace helper that fails startup or validation is not a valid host task
representative. If stopping it also fails, retain the exact helper handle and
both error causes. Its public PID is zero and validation always refuses, even
if its namespace later appears correct. Only a successful stop releases this
rejected handle before another factory invocation can create a replacement.

A factory may return both a holder and a startup error when cleanup failed.
The service must retain this cleanup ownership. A successful startup still
returns the ordinary holder; a failure with successful cleanup returns no holder.
Existing stop/reap semantics and parent-death signaling remain unchanged.

Create rollback stops at a failed holder stop. It must preserve the network
endpoint, sandbox generation, prepared root, token and process record rather
than releasing them around a possibly live namespace owner. Once that exact
stop succeeds, an explicit rollback retry proceeds through the existing network,
sandbox cancellation/deletion and prepared-root cleanup order. A repeated stop
after completed cleanup is a no-op. This does not authorize signaling a historic
PID, deleting an uncertain generation or a broad destructive repair.

The fixed [diagnostic boundary](process-diagnostics.md) carries no underlying
error or namespace path. These rules preserve in-process cleanup ownership;
they do not add a durable holder identity or establish restart reconciliation,
Docker health, successful namespace entry or complete G7/G8 acceptance. Direct
child kernels require no nested virtualization and share primary trust after
child-kernel compromise.

The [committed-source comparison and GCE proof](../../../evidence/runtime-20261109/g8-namespace-holder-retention/README.md)
reproduce prior rejected-handle loss and rollback release, and qualify factory,
validation, startup cleanup, rollback barriers and explicit retries. A real
owned process stays live during an injected stop refusal, then is killed/reaped
through its exact retained handle. Actual signal syscall failure is not induced.
