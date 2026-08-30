# `mk-host-check`

Read-only node qualification command planned by
[`../../../docs/plans/01-host-and-isolation.md`](../../../docs/plans/01-host-and-isolation.md).

It must emit a structured report and must never initialize a Kerf pool, park a
CPU, allocate child memory, load a kernel, or change a device assignment.
