# Docker service stop admission budget

Source `a2a99088` makes admission to `dockerengine.Service.Stop` respect the
caller's context. A stop queued behind another stop can return Canceled or
DeadlineExceeded while that owner is still active. The canceled waiter does not
send another signal, stop a scope, close a pidfd, release a mount or publish
completion. Cancellation does not revoke or cancel the current owner.

A zero-value gate owns one lazily initialized capacity-one channel. Waiters have
no helper goroutine or polling timer. Cancellation racing token acquisition
returns the token before teardown starts. Teardown remains serialized across
pidfd signaling, cgroup stop, launcher/log reaping and prepared mount release.
Completed release is published atomically only after those existing steps succeed.
An already observed completed release remains idempotent, including with a canceled
non-nil context; a nil context is still refused.

This covers queue admission, not all kernel, cgroup, filesystem or diagnostic I/O.
It adds no fairness or admission-count bound. Callers still need a bounded context,
and unknown/failed release retains its exact cleanup ownership. The active stop's
two-second launcher grace period, original context checks, scope identity and mount
release contracts remain. No numeric-PID signal fallback is added.

[Exact-parent/local controls](../../../evidence/runtime-20261105/g8-docker-stop-queue-controls/README.md)
hold a real stop in its grace period using a clone-bound pidfd for a fresh fixture
process. A zero-authority scope prevents any real cgroup or mount release. Both
queued cancel/deadline cases fail against the exact parent and pass after the fix.
The owner returns its own cancellation and retains the live pidfd; a later admitted
attempt keeps that same ownership. Separate controls check completed-release
idempotence, 128 deadline waiters, 512 serialized admissions and 256 cancellation
races without a lost token. Full runtime race and required checks pass.

This does not establish the cause of the retained Docker timeout or recover that
workload. Actual source-qualified child-Docker execution, stop/release and full
G7/G8 remain separate. No nested virtualization is used; direct child kernels
share host trust after compromise.
