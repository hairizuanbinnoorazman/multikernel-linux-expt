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

Local fixture tests reject a stock kernel, APIC ID 0, insufficient primary CPU
headroom, an unknown existing instance, duplicate logical/APIC topology
mappings, duplicate requested APIC IDs, and offline requested APIC IDs.
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

## Replacement live evidence (2026-09-03)

The current narrow host/crash path reproduced on disposable instance
`mklinux-g0-g3-proof-20260903`. The first attempt exposed that `make sync`
omitted `guest/runtime-crash-init`; the failed non-mutating attempt was retained
and the Makefile was corrected. Under strict error propagation, the decisive
rerun qualified the pinned host, intentionally crashed child PID 1, reclaimed
the child, returned CPUs `0-15`, left no pool or instance, and kept the Google
guest agent active. The [G1 manifest and raw logs](../../../evidence/runtime-20260903/g0-g3-proof/README.md)
substantiate that narrow claim but do not change the isolation boundary or the
current implementation gaps below.

The continuing remediation run on retained instance
`mklinux-g0-g3-remediation-20260903` deployed the corrected checker and
requalified the same host class. Its focused live assertion proves that CPU 0's
physical, core, and NUMA identifiers are all present as numeric zero rather
than disappearing during JSON encoding. The raw report and assertion are in
[`g0-g3-remediation`](../../../evidence/runtime-20260903/g0-g3-remediation/README.md).

Raw evidence: [`../../../evidence/runtime-20260831/g0-g3-gce/g1-host-report.json`](../../../evidence/runtime-20260831/g0-g3-gce/g1-host-report.json)
and [`g1-crash-reclaim.log`](../../../evidence/runtime-20260831/g0-g3-gce/g1-crash-reclaim.log).

## Claim-to-proof audit

| Retained claim | Proof | Classification |
| --- | --- | --- |
| Qualified kernel/config, Kerf 0.2.0, 16 online CPUs, 67,416,367,104 bytes, Secure Boot disabled, lockdown inactive, guest agent active, and no instance | [`g1-host-report.json`](../../../evidence/runtime-20260831/g0-g3-gce/g1-host-report.json) contains those exact fields. | Live tested, narrow report only. |
| Intentional child crash was reclaimed | [`g1-crash-reclaim.log`](../../../evidence/runtime-20260831/g0-g3-gce/g1-crash-reclaim.log) records create, load, unload, delete, pool return, and the pass marker. | Live tested. |
| Child saw two assigned CPUs and approximately 4 GiB | [`runtime-g3-console.log`](../../../evidence/runtime-20260831/g0-g3-gce/runtime-g3-console.log) records the parsed `8,10` assignment, restriction to two CPUs, and 4,194,304 KiB. | Live observation from the later G3 child, not asserted in a G1 manifest. |
| APIC 0, headroom, stock kernel, existing instance, duplicate topology/APIC mappings, and offline APIC requests are rejected | Focused `runtime/internal/hostcheck` tests and `go test -race ./...` pass. | Unit tested; the replacement report separately live-proves zero-valued topology serialization. |

The report does not contain contiguous-allocation readiness, `/proc/kimage`,
pool/device/stale-resource details, controller ancestry, serial recovery, or a
complete online/offline/topology assessment. The combined manifest is labelled
G3 and lacks a G1 assertion for child CPU/memory visibility. Those checklist
items therefore remain open despite the useful raw observations above.

## Current implementation audit

The remaining remediation findings match the current host checker:

- primary CPU headroom now counts only online CPUs, and duplicate/offline
  topology cases have focused tests;
- unknown Kerf, Secure Boot, lockdown, kexec, and guest-agent states do not all
  fail closed;
- physical/core/NUMA zero values are retained in JSON and were observed live;
- pool state, `/proc/kimage`, device ownership, stale resources, SMT policy,
  contiguous-memory readiness, controller ancestry, and serial recovery are
  not reported; and
- `ValidateRequestedAPICs` rejects APIC 0, duplicates, offline IDs, and
  headroom violations, but the daemon allocation path does not consume a live
  qualified report or enforce that validation.

Therefore G1 currently demonstrates a useful read-only host snapshot and two
narrow live lifecycle observations. It does not yet establish deterministic
dangerous-allocation rejection or the plan's complete isolation matrix.
