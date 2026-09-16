# Plan 05: Mediated pod networking

## Purpose

Provide transparent container networking without assigning the GCE NIC or its
controller to a child.

## Proposed architecture

```text
CNI-created primary network namespace
  <-> primary TUN/TAP endpoint
  <-> framed packet transport over Multikernel AF_VSOCK
  <-> child TUN endpoint
  <-> pod bridge/veth and container namespace
```

The primary performs routing, firewalling, NAT, and CNI interaction. The child
owns only its virtual interface and container namespaces.

## Minimum implementation

- `mknetd` primary service with one authenticated endpoint per sandbox.
- Child packet adapter controlled by `mk-agent`.
- MTU negotiation, bounded frames, backpressure, counters, and link state.
- Static addressing for the first proof.
- DNS configuration injection.
- CNI `ADD`, `CHECK`, and `DEL` adapter after the static proof.

The CNI adapter treats an `ADD` response as untrusted. Before caching it, the
adapter requires the exact requested container, network, interface, and
namespace identity; a CNI-owned generation; a valid IPv4 `/30`, gateway, MTU,
DNS policy, and READY state; and no premature sandbox binding. Any identifiable
post-allocation validation or cache-persistence failure issues a generation-
bound `DEL` under a five-second rollback deadline. The generation cache is a
private caller-owned real directory; reads use `O_NOFOLLOW`, bounded input, and
pre/post-open inode identity checks.

`mknetd` journals an `ALLOCATING` endpoint generation, including the
deterministic managed-namespace path, before creating a namespace, link, route,
or firewall rule. It changes the record to `READY` only after the complete
backend succeeds. Synchronous failure rolls external state back under a bounded
context and removes the record only after cleanup succeeds; restart
reconciliation performs the same cleanup for any retained `ALLOCATING` record.
Teardown first changes an unbound endpoint to `DELETING`; only then does it
remove backend resources, a managed namespace, and finally the record. Restart
reconciliation completes either transitional phase, so a host/service failure
cannot leave a READY record for resources already being removed.
For a runtime-owned endpoint, the sandbox ID and generation remain attached to
the `DELETING` record until cleanup finishes. A repeated RELEASE can therefore
find and resume a failed teardown without waiting for an mknetd restart.
On startup, the durable store validates every map key, workload/generation,
owner, namespace path, address/gateway, MTU, DNS policy, state, and sandbox
binding before reconciliation can act. The state file is opened no-follow with
a bounded read and matching pre/post-open inode identity.

Every privileged primary or guest `ip`, `iptables`, `nsenter`, and sysctl
operation, including the `mknetd` egress preflight, runs through the shared
bounded process-group runner. Primary calls observe caller cancellation; guest
calls observe agent-server cancellation, and every call has a 30-second
default that terminates the complete command group. Combined stdout/stderr
retention is limited to one MiB and returned failure diagnostics to 16 KiB.
`mknetd` service cancellation closes the listener and every accepted request
socket, including a peer stalled before its newline; accept-loop failure
cancels the derived handler context as well.
The concurrent handler budget defaults to 128 and is hard-capped at 1,024.
Connections above the configured budget are closed without spawning a
goroutine, and listener return waits for all admitted handlers to finish.
The shim owns one joined network-report worker with a one-entry latest-state
queue. Failed `READY`, `DISCONNECTED`, `DEGRADED`, or monotonic-counter reports
retry every 100 milliseconds; newer state replaces stale pending state, and
each `mknetd` call remains bounded to two seconds. Network teardown cancels and
joins that worker before its final synchronous counter report.
Guest `CloseNetwork` has an independent five-second deadline. Timeout is
returned but does not skip local TUN closure or the final bounded counter
report, so an unresponsive child cannot retain the shim's network descriptor
or block Task deletion indefinitely.
Successful guest configuration retains the complete name, address, gateway,
MTU, and ordered DNS identity. `ConfigureNetwork` accepts only an exact replay
of that completed identity; a different request or any partial-cleanup state
fails closed. The shim can therefore reconnect and retry a lost response within
one bounded exchange without configuring the child twice. `CloseNetwork`
clears the replay identity only after every close step succeeds and retains it
across a failed close for the next cleanup retry.
Guest DNS restoration retains cleanup ownership after failure and becomes a
no-op after success, so repeated network close cannot remove restored state.
`ATTACH` transfers exactly one generation-bound TUN descriptor with one
complete newline-terminated response. The server accepts only a complete
payload and control-message send. The client marks received descriptors
close-on-exec immediately and closes every received descriptor when payload,
binding, error, truncation, or descriptor-count validation fails.
PROVISION, recovery ATTACH, REPORT, and RELEASE all apply mknetd's complete
durable endpoint validator before retaining state, slicing generation-derived
request IDs, or issuing a mutation. Workload/sandbox identity and any requested
namespace or endpoint generation must match exactly; malformed generations
therefore return errors instead of panicking, and invalid ATTACH descriptors
are closed before use.
Synthetic send-boundary coverage is local; the real SCM_RIGHTS success and
rejected-payload leak tests are permission-gated and must execute without a
skip in the privileged disposable-host run.

## Tests

- Child-to-primary, outbound TCP/UDP, DNS, and return traffic.
- Two sandboxes with overlapping internal names but distinct network identity.
- No cross-sandbox traffic without an explicit route/policy.
- MTU boundary, fragmentation, checksum, loss, reordering, and sustained load.
- Transport disconnect/reconnect and child/daemon restart.
- CNI failure after partial `ADD`, repeated `DEL`, and stale namespace cleanup.
- Privileged-command timeout with descendants, combined-output capture, and
  output-limit failure without an unbounded diagnostic.
- Repeated guest-network close and failed DNS-restoration retry ownership.
- Network-policy enforcement location and bypass attempts.
- Primary SSH, metadata, and guest-agent connectivity remain healthy.

## Deferred items

Service meshes, SR-IOV, direct device assignment, eBPF acceleration, and
multi-queue performance are outside the first networking gate.

## Gate G5

Pass when a child container has transparent IP connectivity through a
primary-owned CNI namespace, two sandboxes are isolated by default, teardown
leaves no namespace/interface/rule leaks, and the primary NIC is never moved.
