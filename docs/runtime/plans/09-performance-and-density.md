# Plan 09: Performance and density

## Purpose

Measure whether the runtime offers enough benefit to justify its dedicated CPU
and memory model. Performance work begins only after correctness gates.

## Baselines

Compare the same OCI workload under:

- `runc`;
- the Multikernel runtime;
- Kata with an available KVM VMM; and
- Firecracker where nested virtualization is intentionally available and the
  comparison is operationally reasonable.

Results must label GCE virtualization effects and must not be generalized to
bare metal.

## Measurements

- Cold and warm sandbox creation latency.
- Kernel boot, agent-ready, process-start, stop, and cleanup breakdowns.
- Minimum and steady-state memory overhead.
- CPU reservation efficiency and SMT/NUMA effects.
- Maximum concurrent sandboxes under explicit primary headroom.
- Storage throughput, latency, flush cost, and CPU consumption.
- Network throughput, latency, packet rate, loss, and CPU consumption.
- Application benchmarks relevant to dedicated-core and kernel-specialized
  workloads.
- Failure-detection and recovery latency.

## Method

- Pin versions and machine shape.
- Use multiple runs, report distributions, and retain raw data.
- Separate cached and uncached storage results.
- Separate initialization from steady-state work.
- Record primary overhead as well as child workload results.
- Publish negative results and capacity cliffs.

## Gate G9

Pass when results are repeatable, raw evidence is retained, comparisons are
like-for-like, and the project can identify at least one workload class where
Multikernel offers a meaningful advantage or honestly conclude that it does
not yet do so.
