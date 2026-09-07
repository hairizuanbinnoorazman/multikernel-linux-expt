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

## Tests

- Child-to-primary, outbound TCP/UDP, DNS, and return traffic.
- Two sandboxes with overlapping internal names but distinct network identity.
- No cross-sandbox traffic without an explicit route/policy.
- MTU boundary, fragmentation, checksum, loss, reordering, and sustained load.
- Transport disconnect/reconnect and child/daemon restart.
- CNI failure after partial `ADD`, repeated `DEL`, and stale namespace cleanup.
- Network-policy enforcement location and bypass attempts.
- Primary SSH, metadata, and guest-agent connectivity remain healthy.

## Deferred items

Service meshes, SR-IOV, direct device assignment, eBPF acceleration, and
multi-queue performance are outside the first networking gate.

## Gate G5

Pass when a child container has transparent IP connectivity through a
primary-owned CNI namespace, two sandboxes are isolated by default, teardown
leaves no namespace/interface/rule leaks, and the primary NIC is never moved.
