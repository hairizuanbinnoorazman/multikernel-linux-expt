# Docker readiness peer-check budget

The child-local Docker supervisor bounds each startup probe to 750 ms under the
existing 30-second startup context. Its transport uses that explicit probe
context for both Unix dialing and scope peer admission, even if HTTP supplies a
detached dial context. No proxy, TCP endpoint or host Docker socket is selected.

`Scope.ContainsLivePeerContext` shares the existing scope admission gate with
Start, Stop and observations. A canceled/deadline waiter returns false with the
context error before observing scope state, polling a pidfd or reading membership.
It does not close the caller's peer pidfd, signal processes, mutate ownership or
release resources. A nil context refuses. The original `ContainsLivePeer` keeps
its blocking contract through a background context.

After admission, existing requirements remain: owned open scope, valid pinned
peer, live pidfd before/after a bounded cgroup member read, exact membership and
no successful observation after cancellation is observed at the final check.
The Docker socket additionally requires UID 1000 and `SO_PEERPIDFD`; there is no
numeric-PID fallback. This changes queue waiting, not synchronous kernel read
latency or process-start cancellation. A full launch can still wait inside clone
admission; the probe budget does not claim an end-to-end startup syscall bound.

Local controls hold the actual scope gate and exercise cancel/deadline return
before owner release, real caller-pidfd preservation, cached-state refusal and
subsequent lock use. Real Unix listeners verify expired probes make no connection
when their dial context remains live. Installed opt-in frozen-admission tests
reuse the scope lifecycle fixture, with exact owned release and outside-process
survival. Installed tests must run separately; local skips do not qualify them.

These checks do not establish ongoing Docker health, child-kernel containment,
normal retained-workload recovery or full G7/G8. Direct child kernels require no
nested virtualization and share host trust after child-kernel compromise.
