# G1 pinned Multikernel isolation audit

Audit date: 2026-09-03

## Scope and method

This audit covers Multikernel Linux `v7.0-mk2`, exact commit
`3bdd35b64413da0b4e089ce931bfc2e8b031cbf7`. The source was fetched from the
upstream `multikernel/linux` repository into a detached sparse checkout. The
review covered the x86 boot handoff, E820 construction, memory pools, CPU and
PCI hotplug, PCI discovery filtering, shared IPI transport, and DMA heap.

The decisive files and their SHA-256 digests at the audited commit are:

| File | SHA-256 |
| --- | --- |
| `arch/x86/multikernel/e820.c` | `ca374ca55792a8e97500144f8bd5eee82c22fb3af9fb2005aadb0ce3be6203e2` |
| `arch/x86/multikernel/spawn.c` | `6a7f5412afa43f36fb645f3c457e6135b7cbd741623707ec5b8880b8cc48c883` |
| `arch/x86/multikernel/direct_boot.S` | `7b0b5ccda22591986ea73baf8cb2166f4a1513663cca13b39e5f924a8472e89e` |
| `kernel/multikernel/mem.c` | `07c0436128eb7a3d91437589b682d3dd4d65e51e75f727edf2d5d8030b8f68cb` |
| `kernel/multikernel/hotplug.c` | `de0633bb71bb318aab8278db4ac9cf4418cd7f8bb494f0537b75b8f04784c7eb` |
| `kernel/multikernel/instance_dt.c` | `bd598e89297926a602c7aa893c086c2f9ff407737de151c98bdde321c31dc0f8` |
| `kernel/multikernel/ipi.c` | `4154dc981fb3c4cc57c1c383e73fdd2a2d0231089f5df5bda23fb1add02d0fe1` |
| `kernel/multikernel/dma_heap.c` | `5b87403b1672efcaa8b21b3ed287b1ef174b26de6a472e77a0763d9f90aadfcb` |

The classifications below distinguish allocation correctness for trusted
kernels from a security boundary against a malicious child kernel.

## Source findings

### Memory

The primary allocates physically contiguous pages, removes them from ordinary
allocation, and tracks disjoint instance regions. The child E820 table contains
only its assigned regions, and the initial identity page table maps those
regions. These are meaningful software ownership and accidental-access
controls for a trusted child:

- [`contig.c`](https://github.com/multikernel/linux/blob/3bdd35b64413da0b4e089ce931bfc2e8b031cbf7/kernel/multikernel/contig.c#L14-L67)
  obtains and releases contiguous ranges.
- [`mem.c`](https://github.com/multikernel/linux/blob/3bdd35b64413da0b4e089ce931bfc2e8b031cbf7/kernel/multikernel/mem.c#L416-L469)
  allocates and records an instance region.
- [`e820.c`](https://github.com/multikernel/linux/blob/3bdd35b64413da0b4e089ce931bfc2e8b031cbf7/arch/x86/multikernel/e820.c#L23-L60)
  advertises only the recorded regions as RAM.
- [`spawn.c`](https://github.com/multikernel/linux/blob/3bdd35b64413da0b4e089ce931bfc2e8b031cbf7/arch/x86/multikernel/spawn.c#L374-L393)
  builds initial identity mappings from the same region list.

They are not a hostile-kernel memory boundary. The child is entered directly
in x86 kernel mode by switching CR3 and jumping to its kernel entry
([`direct_boot.S`](https://github.com/multikernel/linux/blob/3bdd35b64413da0b4e089ce931bfc2e8b031cbf7/arch/x86/multikernel/direct_boot.S#L31-L60));
there is no hypervisor second-level translation such as EPT. E820 and the
initial page tables constrain discovery and normal boot behavior, but a
malicious ring-0 child can construct new page tables for physical addresses it
can discover or guess. Therefore memory confidentiality and integrity across
hostile siblings are **not hardware-enforced and must not be claimed**.

The DMA heap intentionally maps pool physical pages to userspace and devices
([`dma_heap.c`](https://github.com/multikernel/linux/blob/3bdd35b64413da0b4e089ce931bfc2e8b031cbf7/kernel/multikernel/dma_heap.c#L46-L68),
[`dma_heap.c`](https://github.com/multikernel/linux/blob/3bdd35b64413da0b4e089ce931bfc2e8b031cbf7/kernel/multikernel/dma_heap.c#L120-L148)).
That is an explicit sharing mechanism, not isolation.

### CPUs and interrupts

Pool CPUs are taken offline and parked before assignment; the implementation
tracks membership and refuses to return a CPU not recorded as free
([`hotplug.c`](https://github.com/multikernel/linux/blob/3bdd35b64413da0b4e089ce931bfc2e8b031cbf7/kernel/multikernel/hotplug.c#L1055-L1128)).
This provides exclusive execution of a logical CPU after a successful
software-coordinated handoff. It does not isolate SMT-shared core resources,
last-level cache, memory bandwidth, power controls, or the APIC fabric.

Inter-kernel messages use shared memory plus IPIs. Receiver-side ring indices
are masked to prevent a corrupt peer index from walking outside the ring, and
abandoned entries are dropped on respawn
([`ipi.c`](https://github.com/multikernel/linux/blob/3bdd35b64413da0b4e089ce931bfc2e8b031cbf7/kernel/multikernel/ipi.c#L23-L58)).
This is useful defensive validation, but a bare-metal ring-0 child can program
its local APIC. The reviewed Multikernel path does not interpose arbitrary
interrupt generation, MSR access, or I/O-port instructions. Those facilities
are consequently outside the hostile-workload boundary.

### PCI and DMA

Device transfer unbinds the current driver and records the PCI function in a
software pool
([`hotplug.c`](https://github.com/multikernel/linux/blob/3bdd35b64413da0b4e089ce931bfc2e8b031cbf7/kernel/multikernel/hotplug.c#L758-L805),
[`hotplug.c`](https://github.com/multikernel/linux/blob/3bdd35b64413da0b4e089ce931bfc2e8b031cbf7/kernel/multikernel/hotplug.c#L1131-L1192)).
The child filters PCI discovery against its manifest before config-space reads,
while permitting bridges on a path to an allowed function
([`instance_dt.c`](https://github.com/multikernel/linux/blob/3bdd35b64413da0b4e089ce931bfc2e8b031cbf7/kernel/multikernel/instance_dt.c#L831-L912)).

This is a software allowlist, not proof of IOMMU isolation, interrupt
remapping, reset isolation, ACS isolation, or bridge/function independence.
A controller or bridge can cover primary-owned devices, as observed on the GCE
N2 topology. Runtime v1 therefore prohibits physical-device allocation and
keeps the boot disk, NIC, and every shared ancestor with the primary.

### Denial of service and recovery

The source contains bounded waits and recovery paths for resource handoff, and
the retained G1 live test proves the primary can reclaim resources after one
cooperative child PID 1 crash. Those facts do not bound attacks through shared
cache/memory bandwidth, APIC traffic, machine-check or power-control MSRs,
firmware, interconnect, or a deliberately wedged kernel. There is no
hypervisor scheduler or virtual hardware boundary. Denial-of-service resistance
against a malicious child is **unverified and not claimed**.

The same audit confirms that child-initiated poweroff is safe at the pinned
revision: spawn boot replaces the native halt, poweroff, restart, emergency-
restart, and stop-CPU operations with Multikernel handlers
([`spawn.c`](https://github.com/multikernel/linux/blob/3bdd35b64413da0b4e089ce931bfc2e8b031cbf7/arch/x86/multikernel/spawn.c#L1680-L1730)).
The handler notifies instance 0 and parks every child CPU, and the primary
settles the instance back to `LOADED`
([`core.c`](https://github.com/multikernel/linux/blob/3bdd35b64413da0b4e089ce931bfc2e8b031cbf7/kernel/multikernel/core.c#L1321-L1407)).
This source property must be re-audited for any alternate kernel manifest.

## Per-resource classification

| Resource | Classification | Runtime v1 policy and claim boundary |
| --- | --- | --- |
| Logical CPUs | software-coordinated; hardware-exclusive after handoff | Allocate whole cores only, retain primary headroom, and reject duplicate/offline/APIC-0 input. This is not protection from shared-core or platform DoS. |
| Physical memory capacity | software-coordinated / accidental-access protection | Contiguous ranges, E820, and initial page tables keep trusted kernels in their grant. No EPT means no hostile-child confidentiality or integrity claim. |
| Shared control/IPI memory | software-coordinated, intentionally shared | Bounded receiver parsing reduces accidental corruption; authentication is for stale/accidental peers, not a malicious sibling kernel. |
| Interrupt/APIC fabric | unverified for hostile isolation | Use only the approved child kernel; no hostile-tenant claim. |
| MSRs and I/O ports | unverified and effectively unrestricted to child ring 0 | Prohibited as a security boundary; do not run hostile child kernels. |
| PCI functions | software allowlist; DMA isolation unverified | Physical pass-through is prohibited in runtime v1. |
| Boot disk, NIC, shared PCI ancestors | prohibited | Always primary-owned; daemon preflight rejects them before mutation. |
| DAXFS/shared memory | software-coordinated, intentionally shared | Read-only sharing or exactly one writer; no cross-kernel coherent writable-root claim. |
| Mediated storage/network | software-coordinated by primary | Primary retains hardware ownership and validates bounded protocol operations. A compromised primary controls children. |
| Availability/DoS | unverified | Single-tenant trusted-workload use only; recovery tests prove specific failures, not adversarial availability. |

## G1 conclusion

The pinned implementation supports deterministic allocation and useful fault
containment for approved, trusted child kernels. It does not provide a KVM/EPT-
grade boundary. Runtime v1 is therefore restricted to trusted workloads on a
single-tenant node, prohibits physical-device assignment, uses whole-core CPU
allocation, and treats the primary as trusted. Any hostile multi-tenant claim
requires a different architecture or separate hardware-backed isolation proof.
