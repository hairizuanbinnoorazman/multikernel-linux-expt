# G1 learnings: host qualification and isolation

## Result and current status

The 2026-08-31 run is a **provisional G1 milestone**, not a closed gate. On the
restored `n2-standard-16` GCE host, the read-only
`mk-host-check` reported `qualified: true` with:

- `7.0.0-mk2-gce-lab`, Kerf `0.2.0`, and all four required kernel options;
- 16 online CPUs and the logical/APIC/core map;
- 67,416,367,104 bytes of primary memory;
- Secure Boot disabled, lockdown inactive, and the guest agent active; and
- no existing Multikernel instance.

Local fixture tests rejected a stock kernel, APIC ID 0, insufficient primary
CPU headroom, and an unknown existing instance. Duplicate and offline APIC IDs
are rejected by `ValidateRequestedAPICs`, but those branches do not yet have
focused tests.
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

## Claim-to-proof audit

| Retained claim | Proof | Classification |
| --- | --- | --- |
| Qualified kernel/config, Kerf 0.2.0, 16 online CPUs, 67,416,367,104 bytes, Secure Boot disabled, lockdown inactive, guest agent active, and no instance | [`g1-host-report.json`](../../../evidence/runtime-20260831/g0-g3-gce/g1-host-report.json) contains those exact fields. | Live tested, narrow report only. |
| Intentional child crash was reclaimed | [`g1-crash-reclaim.log`](../../../evidence/runtime-20260831/g0-g3-gce/g1-crash-reclaim.log) records create, load, unload, delete, pool return, and the pass marker. | Live tested. |
| Child saw two assigned CPUs and approximately 4 GiB | [`runtime-g3-console.log`](../../../evidence/runtime-20260831/g0-g3-gce/runtime-g3-console.log) records the parsed `8,10` assignment, restriction to two CPUs, and 4,194,304 KiB. | Live observation from the later G3 child, not asserted in a G1 manifest. |
| APIC 0, headroom, stock kernel, and existing instance are rejected | `runtime/internal/hostcheck` tests and `go test ./...` pass. Duplicate/offline rejection exists in `ValidateRequestedAPICs` but lacks a focused test. | Unit tested only for the named covered cases; the historical manifest does not index this output. |

The report does not contain contiguous-allocation readiness, `/proc/kimage`,
pool/device/stale-resource details, controller ancestry, serial recovery, or a
complete online/offline/topology assessment. The combined manifest is labelled
G3 and lacks a G1 assertion for child CPU/memory visibility. Those checklist
items therefore remain open despite the useful raw observations above.
