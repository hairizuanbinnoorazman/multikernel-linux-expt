# Cgroup scope stop admission budget

Source `65f0ffa5` makes `guestscope.Scope.Stop` respect caller cancellation and
its existing 30-second ceiling while waiting for the scope's current owner.
Admission, Empty, peer observation and release still use the same zero-value
serial gate. Start retains that gate through clone/exec admission, so Stop cannot
close or reuse its cgroup descriptor beneath an active clone.

The gate has one lazily initialized capacity-one channel, no waiter worker or
polling loop. Cancellation observed during acquisition returns the token before
any stop work. A canceled waiter sends no signal, writes no cgroup control,
changes no closing/closed state, and closes no descriptor. It leaves the owner
active; it does not revoke an admission or repair a stuck syscall.

Cancellation before acquisition reports only the context result, without a
cached-completion or backend-identity observation. This includes already-canceled
requests for a closed or zero-value scope. A fresh live request still reports
ErrIdentity for a zero-value scope and nil for completed release. Nil contexts
remain refused. This explicitly gives admission cancellation priority over the
previous lock-then-state inspection order.

Legacy Start/Empty/ContainsLivePeer retain their blocking contracts and common
serialization. This is a Stop queue budget, not a real-time bound on kernel I/O,
clone/exec or all inspection operations. Scope kill, population-zero, directory
identity and exact removal behavior are unchanged. No numeric-PID fallback or
foreign-tree adoption/repair is introduced.

[Local exact-parent proof](../../../evidence/runtime-20261106/g8-scope-admission-controls/README.md)
passes held-owner cancel/deadline, unchanged authority, mixed legacy/context
serialization, cancellation races, full runtime race and required checks.
The installed test added by `af4cd9a0` is opt-in: it creates only fresh bounded
leaves below a dedicated delegated test unit, freezes its own leaf, starts a
fixed clone there, checks cancellation while admission remains blocked, then
thaws and normally releases it. An outside process and clone-bound pidfds check
release scope. Native execution is a separate evidence requirement.

The test mechanism follows [Linux clone semantics](https://man7.org/linux/man-pages/man2/clone.2.html)
and [cgroup freezer/kill semantics](https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html).
No production freeze/repair API is added. A native primary-kernel pass would not
qualify actual child-Docker builds/recovery or containment after child-kernel
compromise. Those and full G7/G8 remain open. No nested virtualization is used.

The [native GCE fault proof](../../../evidence/runtime-20261106/g8-scope-admission-gce/README.md)
now passes success/zero (`96fdd0b5`) for `af4cd9a0`: exact-parent timing failures
and candidate passes use identical tests; all four owned scopes are normally
released, children reaped and outside processes preserved. A post-run audit
confirms the delegated unit cgroup is absent and original controller/retained
ownership state is unchanged. This is primary-kernel primitive qualification;
actual new-source child-Docker recovery and full G7/G8 remain separate.
