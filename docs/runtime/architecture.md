# Target Multikernel container-runtime architecture

## Objective

Expose Multikernel child kernels as OCI pod sandboxes through containerd and,
eventually, Kubernetes. The primary kernel retains ownership of physical and
cloud devices. Kerf allocates CPUs and memory and performs child-kernel
lifecycle operations. Workload processes execute inside the child kernel.

This is not a Firecracker derivative and it is not a KVM virtual machine.

## Boot and image ownership

A Multikernel child does not boot an ISO, firmware image, bootloader, or guest
distribution installer. Kerf loads three runtime-owned inputs directly into
the CPUs and memory allocated to the child:

- an approved Linux `vmlinux` selected from the runtime's kernel manifest;
- a small bootstrap initramfs containing `mk-agent`, transport modules, and
  only the tools needed to reach the sandbox root; and
- a generated kernel command line containing authenticated sandbox and
  endpoint identity.

The OCI image supplies the workload userspace: executables, libraries,
configuration files, filesystem metadata, and image execution defaults. It
does not normally supply the child kernel, bootloader, or bootstrap initramfs.
Containerd owns image acquisition and unpacking and supplies the OCI bundle and
snapshot/rootfs mounts. The Multikernel runtime must not become another image
registry client or layer downloader.

The bootstrap mounts the supplied root through the selected DAXFS or mediated
ext4 mechanism, verifies its identity, and hands process management to
`mk-agent`. Runtime bootstrap files stay outside the caller's OCI snapshot.
An OCI image must match the child kernel architecture, and configuration that
requires an unsupported kernel or OCI feature must fail before the workload is
started.

The normal policy is a small, pinned set of trusted child kernels, not a newly
built kernel for each container image. An explicitly selected alternate kernel
is allowed only when its architecture, modules, initramfs, features, hashes,
and compatibility have been qualified together.

## Target topology

```text
kubelet
  -> containerd CRI plugin
    -> containerd-shim-multikernel-v2 (one per pod sandbox)
      -> mkruntimed (one privileged node daemon)
        -> Kerf and /sys/fs/multikernel
          -> child kernel
            -> mk-agent
              -> pod containers

primary-owned services
  -> rootfs/image service -> Multikernel VSOCK -> child storage adapter
  -> CNI/TUN service      -> Multikernel VSOCK -> child network adapter
  -> log/exec/metrics     <-> Multikernel VSOCK <-> mk-agent
```

`mknetd` is the sole owner of endpoint allocation, primary veth/TUN topology,
routes, NAT, named namespaces, and per-generation firewall chains. An external
CNI caller can create the endpoint before the Runtime v2 task, in which case
`mknetd` binds that exact endpoint to the `mkruntimed` sandbox. For standalone
`ctr`, `mknetd` creates a runtime-owned generation-named namespace through the
same contract. The unprivileged shim requests `PROVISION` and `ATTACH`, receives
the already-open TUN descriptor over the authenticated Unix socket, and pumps
one negotiated-MTU frame at a time over the authenticated agent session.
Neither the child nor the shim moves, configures, or opens the GCE NIC or a
primary namespace. Durable endpoint and counter state lets `mknetd` reconcile
independently of a client restart, while the shim recovery record binds the same
endpoint and sandbox generations before requesting a fresh descriptor.

## Component boundaries

### `containerd-shim-multikernel-v2`

The shim implements containerd Runtime v2 task behavior. It must not directly
hot-unplug CPUs, allocate Multikernel memory, or manipulate global Kerf state.
It translates containerd requests into sandbox requests and preserves stdio,
exit status, and task event semantics. It also must not mount caller snapshots
or construct filesystem images: it submits the exact bundle, mounts, task
identity, and storage port to the authenticated rootfs-preparation API.

### `mkruntimed`

The daemon is the only runtime component permitted to mutate Kerf or
`/sys/fs/multikernel`. It owns resource allocation, a durable operation
journal, per-sandbox locks, reconciliation after restart, and cleanup. Its API
must be versioned and usable without containerd so lower layers can be tested
independently. Its journaled rootfs-preparation service mounts the containerd
snapshot read-only, builds and verifies the mediated ext4 image on the retained
storage filesystem, unmounts the snapshot before allocation, and removes the
image only after the matching sandbox generation has released it.

### Kerf adapter

The first adapter may invoke the pinned Kerf CLI with structured parsing and
strict timeouts. The target is a stable machine-readable Kerf API. Container
semantics must remain outside Kerf.

### `mk-agent`

The child agent receives authenticated, versioned requests over Multikernel
AF_VSOCK. It creates namespaces and cgroups inside the child, mounts the OCI
root filesystem, launches processes, forwards stdio and signals, reports exit
status, and performs an orderly storage shutdown before child termination.

### Storage and networking services

The primary owns backing storage and the external NIC. Children receive only
mediated endpoints. The initial root path may reuse the proven DAXFS or
primary-mediated NBD mechanisms. The network path must use a primary-owned
network namespace and a packet transport; the GCE NIC must never be assigned
to a child.

## Sandbox granularity

One Multikernel child represents one Kubernetes pod sandbox. Multiple OCI
containers may run inside that child through one agent. One child per
container is allowed for direct containerd testing but is not the target
density model.

## Required lifecycle

```text
ABSENT
  -> ALLOCATING
  -> CREATED
  -> LOADED
  -> RUNNING
  -> STOPPING
  -> STOPPED
  -> RELEASING
  -> ABSENT
```

Every transition needs an idempotency key, timeout, observable result, and
rollback rule. `ERROR` is an annotation on a durable last-known state, not a
license to guess which resources are safe to release.

## Initial trust boundary

The first runtime is for trusted, single-tenant workloads. Multikernel does not
provide KVM/EPT-style hardware isolation between sibling kernels. No document,
API name, or benchmark may describe the MVP as a safe hostile multi-tenant
sandbox. A stronger claim requires a separately reviewed threat model and
negative isolation evidence.

## Non-negotiable invariants

- Never allocate APIC ID 0.
- Keep management CPUs and sufficient memory in the primary.
- Do not assign the primary boot disk, GCE NIC, or their shared controllers.
- Give each writable filesystem exactly one child owner.
- Authenticate sandbox identity, generation, and endpoint on every transport.
- Persist enough state to reconcile after daemon, shim, agent, or child crash.
- Prove resource return after every destructive or failure-injection test.
- Preserve the stock recovery kernel and serial-console access in cloud tests.
