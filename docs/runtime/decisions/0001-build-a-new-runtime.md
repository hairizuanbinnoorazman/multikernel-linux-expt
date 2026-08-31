# ADR 0001: Build a new runtime around Kerf

- Status: accepted
- Date: 2026-08-30

## Decision

Build a new Multikernel container runtime. Reuse Kerf as the CPU, memory, and
child-kernel lifecycle backend. Use Firecracker and Kata Containers as design
references, not as the initial codebase.

## Reasons

Firecracker is organized around KVM virtual machines, `/dev/kvm`, vCPU
`KVM_RUN` loops, and virtio devices. Replacing those mechanisms would discard
most of the implementation while retaining a large upstream-divergent fork.
Calling the result Firecracker would also imply a hardware virtualization
security boundary that Multikernel does not currently provide.

Kata has the right high-level shape—containerd shim, sandbox manager, guest
agent, and one kernel per pod—but its runtime and device paths assume a
hypervisor. A Kata backend may become appropriate after the Multikernel daemon,
agent, storage, and network contracts have been proven independently.

Kerf already owns the correct low-level concepts. Container-specific behavior
does not belong in Kerf, but stable machine-readable lifecycle APIs may be
contributed upstream.

## Consequences

- The MVP gets a small, explicit codebase and an honest name.
- OCI/containerd behavior can be tested independently from Kerf internals.
- The project must implement its own recovery journal and child agent.
- A later Kata backend remains possible without making it the first critical
  path.
- A Kerf fork is permitted only for narrowly scoped missing backend APIs, with
  upstream contribution preferred.
