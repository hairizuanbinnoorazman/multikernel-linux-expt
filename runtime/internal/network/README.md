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
