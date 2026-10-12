# Network adapter

Own the primary-side namespace/TUN endpoint and authenticated packet transport
to the child. It must never assign the primary NIC or its shared controller to
a child.

The store copies endpoint DNS slices on reads and writes. Callers may modify
their endpoint values without changing durable ownership outside `Put` or
bypassing the store lock and validation.
