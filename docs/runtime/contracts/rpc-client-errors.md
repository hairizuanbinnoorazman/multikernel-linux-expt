# Local RPC client failures

Source `7e973e47` prevents local daemon/network RPC failures from reflecting Unix
socket paths, wrapped transport text or private request-encoding diagnostics into
Docker-facing error strings. `protocol.ClientFailure` accepts a typed stage and
returns only fixed metadata or a standard context sentinel. It discards the cause:
formatting and unwrapping the result cannot recover private transport payloads.
This is a producer rule, without pattern-based sanitization.

Daemon connection/deadline/write/read failures retain `UNAVAILABLE` and retryable
true. Request encoding retains `INVALID_ARGUMENT` and retryable false. The network
CALL and ATTACH clients return fixed stage errors; `errors.Is` still recognizes
context cancellation/deadline sentinels. Authenticated server API errors, codes,
retry flags and response bodies retain their separate producer contracts.
Strict JSON decoding already supplies bounded redacted parse failures.

Both network methods refuse a pre-cancelled context before dialing and fail closed
if connection deadline setup fails. ATTACH now registers the same joined
close-on-context callback as ordinary network CALL, interrupting a blocked real
Unix descriptor receive promptly. Cleanup unregisters or joins that callback
before returning; successful descriptor transfer remains independent of closing
the control socket. Existing descriptor rejection/closure checks remain in force.

The classifier gives the caller's cancellation/deadline priority over a transport
cause, including the socket/context timer race at a deadline. Its fallback stage
contains neither the cause nor its unwrap chain. Local socket errno detail is no
longer exposed by these client errors; operation stage and standard context
identity remain available. Other RPC producers, Task/flag/inspector errors,
aggregate diagnostic pressure/retention and full G7/G8 remain separate work.

[Local controls](../../../evidence/runtime-20261104/g8-rpc-client-controls/README.md)
retain failing-before/fixed-after tests, race tests, complete runtime race and
required checks. The [GCE private fixture](../../../evidence/runtime-20261104/g8-rpc-client-gce/README.md)
also passes for this source, preserving primary state and all 165 diagnostic
artifacts; installed daemons and kernel backends are not changed. The separate
[actual Docker regression](../../../evidence/runtime-20261104/g8-live-docker-rpc-client/README.md)
and [cold restoration](../../../evidence/runtime-20261104/g8-live-docker-rpc-client-cold-restoration/README.md)
also pass for this source. The private RPC cancellation/transport faults were not
injected during Docker; they retain their separate fixture scope. No nested virtualization or new containment boundary is added;
direct child kernels share host trust after child-kernel compromise.
