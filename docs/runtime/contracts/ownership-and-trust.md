# Ownership and trust boundary (v1)

| Resource or action | Owner | Other components |
| --- | --- | --- |
| Registry content, pull, unpack, OCI bundle/snapshot | containerd | Shim passes references; runtime reads validated paths |
| Runtime v2 request and stdio/event mapping | shim | No Kerf or sysfs mutation |
| Allocation policy, journal, generations, cleanup | `mkruntimed` | Sole global lifecycle authority |
| Multikernel pool and child mutation | Kerf adapter | Only daemon invokes it |
| Child kernel, initramfs, `mk-agent` | runtime/operator | Selected only from approved manifest |
| Container process, namespaces, child cgroups | `mk-agent` | Daemon sends authenticated requests |
| OCI root contents | containerd snapshot owner | Agent does not mutate runtime bootstrap |
| Writable root/volume | exactly one sandbox | Primary service retains backing-device ownership |
| GCE boot disk, NIC, shared controllers | primary kernel | Never assign to a child |
| Cloud VM, disk, snapshot lifecycle | operator/test harness | Every run records before/after ledger |

## Threat statement

The operator, primary kernel, `mkruntimed`, approved Kerf binary, approved
child kernel/initramfs, and node configuration are trusted. OCI metadata,
bundle filesystem content, protocol bytes, sandbox identifiers, workload
processes, and stale state are untrusted inputs and are validated.

The MVP is restricted to trusted workloads on a single-tenant node.
Multikernel sibling kernels do not have a demonstrated KVM/EPT isolation
boundary. CPU ownership is software coordinated; child memory access limits,
interrupt/MSR/I/O-port behavior, and denial-of-service resistance are not
claimed as hostile-tenant boundaries. Devices use primary ownership and
mediation because a passed-through controller can include the primary boot
disk or NIC. A compromised primary controls every child.

Authentication binds each daemon-agent session to sandbox ID, generation,
protocol major version, endpoint, and a random 256-bit token stored in a
root-only state directory and injected through runtime-owned bootstrap data.
It prevents accidental/stale peers; it does not defend against a compromised
primary or child kernel that can read shared memory.
