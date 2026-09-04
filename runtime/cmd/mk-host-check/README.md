# `mk-host-check`

Read-only node qualification command planned by
[`../../../docs/runtime/plans/01-host-and-isolation.md`](../../../docs/runtime/plans/01-host-and-isolation.md).

It must emit a structured report and must never initialize a Kerf pool, park a
CPU, allocate child memory, load a kernel, or change a device assignment.

Run it with the exact proposed Kerf pool so qualification includes the
read-only allocation dry-run:

```bash
mk-host-check --kerf=/absolute/path/to/kerf \
  --probe-pool-cpus=8,9,10,11 --probe-pool-memory=16GB
```

The APIC set must contain whole SMT cores. The report includes offline CPUs,
Kerf state and dry-run output, `/proc/kimage`, instance status and assigned
devices, stale-resource findings, serial recovery, and resolved boot-disk and
primary-NIC PCI ancestry. Unknown critical state fails closed.
