# Runtime workspace

This directory is reserved for the Multikernel container runtime described in
[`../docs/architecture/TARGET.md`](../docs/architecture/TARGET.md). It does not
yet contain a runtime implementation.

The first code should be added only as the corresponding contracts in
[`../docs/plans/`](../docs/plans/README.md) are accepted.

## Intended layout

```text
runtime/
├── cmd/
│   ├── mk-host-check/
│   ├── mkruntimed/
│   └── containerd-shim-multikernel-v2/
├── agent/
├── protocol/
├── internal/
│   ├── kerf/
│   ├── lifecycle/
│   ├── state/
│   ├── storage/
│   └── network/
└── tests/
```

## Dependency direction

```text
containerd shim -> daemon client -> versioned protocol
mkruntimed       -> lifecycle/state -> Kerf/storage/network adapters
mk-agent         -> agent protocol  -> child-local process manager
```

The shim must not import or invoke Kerf. Storage and network adapters must not
own global lifecycle policy. Protocol packages must not depend on executable
packages.

## Build policy

- The default local build and unit tests must not require root, GCE, Kerf, or a
  Multikernel kernel.
- Privileged integration tests must be separately selected.
- No installation target may silently modify containerd or make this runtime
  the node default.
- Generated files must be reproducible and identify their source schema.
