# G1 learnings: host qualification and isolation

## Result

Gate G1 passed on the restored `n2-standard-16` GCE host. The read-only
`mk-host-check` reported `qualified: true` with:

- `7.0.0-mk2-gce-lab`, Kerf `0.2.0`, and all four required kernel options;
- 16 online CPUs and the logical/APIC/core map;
- 67,416,367,104 bytes of primary memory;
- Secure Boot disabled, lockdown inactive, and the guest agent active; and
- no existing Multikernel instance.

Local fixture tests rejected a stock kernel, APIC ID 0, duplicate/offline CPU
IDs, insufficient primary CPU headroom, and an unknown existing instance.
The command has no mutation path and reports only the required kernel config
states after the initial live report showed that a complete Ubuntu config made
evidence needlessly large.

The destructive negative test intentionally exited child PID 1 with status 71.
The child panicked, transitioned itself from `active` back to `loaded`, and the
primary remained reachable. The harness unloaded and deleted it, returned the
pool, observed CPUs `0-15`, and reconfirmed the Google guest agent.

## Isolation boundary

The result is a resource-lifecycle proof, not hostile-kernel isolation.
APIC/core allocation and memory ownership are software coordinated. The
primary retains the boot disk, NIC, and their shared controllers; no device was
assigned to the child. A sibling kernel is not claimed to have KVM/EPT-grade
memory, interrupt, MSR, I/O-port, or denial-of-service isolation.

Raw evidence: [`../../../evidence/runtime-20260831/g0-g3-gce/g1-host-report.json`](../../../evidence/runtime-20260831/g0-g3-gce/g1-host-report.json)
and [`g1-crash-reclaim.log`](../../../evidence/runtime-20260831/g0-g3-gce/g1-crash-reclaim.log).
