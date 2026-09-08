# Plan 01: Host qualification and isolation boundary

## Purpose

Prove that a node is safe to use for runtime development and determine what
isolation the current Multikernel implementation actually enforces.

## Minimum implementation

Create a read-only `mk-host-check` command or script that emits JSON containing:

- running kernel release and required config states;
- Kerf version and Multikernel filesystem availability;
- logical CPU, APIC, core, NUMA, online, and offline mappings;
- memory and contiguous-allocation readiness;
- Secure Boot, lockdown, kexec, serial recovery, and GCE guest-agent state;
- existing pools, instances, assigned devices, and stale resources; and
- storage/NIC controller topology with explicit forbidden-device findings.

It must not initialize a pool or modify the host.
All external qualification probes (`kerf --version`, `kerf show`, Kerf
allocation dry-run, and guest-agent service status) use a sanitized environment,
the earlier of caller cancellation and a five-second default, complete
process-group termination, and at most 64 KiB of combined output. Timeout or
overflow makes the corresponding critical signal unknown or unavailable and
therefore cannot qualify the host.

## Tests

1. Clean stock kernel: fail with an actionable unsupported-kernel result.
2. Qualified Multikernel primary: pass without changing any state.
3. APIC ID 0 requested: reject.
4. SMT sibling split: warn or reject according to declared policy.
5. Insufficient primary CPUs or memory: reject.
6. Existing unknown instance: refuse global pool reconciliation.
7. GCE boot disk/NIC controller requested: reject.
8. Child reads only its assigned CPU and approximate memory view.
9. Child crash followed by controlled resource reclamation.
10. Primary remains reachable and the guest agent remains healthy throughout.
11. External-probe timeout kills descendants and output overflow fails closed
    without growing the report beyond its retention bound.

## Isolation investigation

- Inspect physical-memory mapping and access checks in the pinned kernel.
- Test only safe, disposable negative cases; do not write outside assigned
  memory merely to demonstrate a suspected weakness.
- Document interrupt, DMA, MSR, I/O-port, and device access assumptions.
- State whether isolation is accidental, software-enforced, or
  hardware-enforced for each resource class.

## Evidence

Retain the JSON report, Kerf state, device tree, APIC map, primary and child
logs, boot ID, GCE configuration, and final resource-return proof.

## Gate G1

Pass when host qualification is deterministic, dangerous allocations are
rejected before mutation, one child can crash without losing primary control,
and the trusted-workload security boundary is documented without unsupported
claims.
