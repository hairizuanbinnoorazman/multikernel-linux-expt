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
