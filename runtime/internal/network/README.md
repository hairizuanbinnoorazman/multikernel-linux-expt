# Network adapter

Own the primary-side namespace/TUN endpoint and authenticated packet transport
to the child. It must never assign the primary NIC or its shared controller to
a child.

Each endpoint blocks IPv4 traffic addressed to the primary in `INPUT`, separate
from its source, metadata, sibling, and egress rules in `FORWARD`. IPv6 ingress
is dropped in both chains because this adapter supports only IPv4. Guards are
installed before link activation, required by CHECK, and removed only after the
endpoint link is gone. Failed link removal retains the guards. Hosts must provide
both `/usr/sbin/iptables` and `/usr/sbin/ip6tables`; missing policy support fails
endpoint creation rather than leaving an unfiltered path.

This tightens compatibility: guest connections to primary-local TCP services,
including diagnostic listeners used by historical isolation fixtures, are now
blocked. Historical passes describe their recorded policy and revisions.

The store copies endpoint DNS slices on reads and writes. Callers may modify
their endpoint values without changing durable ownership outside `Put` or
bypassing the store lock and validation.

Linux CHECK also requires the complete ordered private IPv4 endpoint chain to
match canonical iptables `-S` output. Required-rule presence alone cannot detect
an earlier ACCEPT/RETURN bypass. Unknown output, reordering or extra rules fails
closed without repair. The [GCE proof](../../../evidence/runtime-20261026/g8-ordered-endpoint-firewall/README.md)
uses an ephemeral network namespace and preserves primary policy. That private-chain check alone does not attest global INPUT/FORWARD precedence
or prevent concurrent drift; the additional precedence check is described below.

CHECK now also verifies sampled IPv4/IPv6 INPUT/FORWARD guard precedence.
Every preceding rule must constrain one positive literal ingress interface
different from the checked endpoint. Unknown, broad, negated, wildcard or
same-interface prefixes fail closed. Disjoint sibling and egress rules are
accepted. [GCE packet controls](../../../evidence/runtime-20261026/g8-ingress-guard-precedence/README.md)
prove that a shadowing ACCEPT can permit simulated primary TCP access even while
DROP remains present; CHECK detects it and accepts the restored policy.
This remains sampled filter-table inspection, with no continuous enforcement,
other-table attestation or automatic rule repair. Reconciliation uses this CHECK
and refuses startup on unresolved policy drift.


`mknetd --inspect-ownership --socket=/run/mknetd.sock` negotiates a bounded
recorded ownership projection without opening state or running reconciliation.
The existing protected socket/UID policy is unchanged. Generations, boot claims,
states, counters and descriptor fingerprints are exposed; private names, paths,
addresses and DNS are omitted. Busy snapshots and unsupported/malformed servers
are explicit failures. One deadline covers negotiation and inspection, and
cancellation closes pending reads. This is metadata inspection, not backend
CHECK, native health, isolation or repair authority. See the
[contract](../../../docs/runtime/contracts/network-ownership.md) and
[GCE proof](../../../evidence/runtime-20261027/g8-network-ownership/README.md).
