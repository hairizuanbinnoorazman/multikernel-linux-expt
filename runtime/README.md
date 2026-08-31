# Runtime workspace

This directory contains the G0-G3 Multikernel runtime implementation described
in [`../docs/runtime/architecture.md`](../docs/runtime/architecture.md). Host
qualification, the recoverable Kerf control plane, and the minimal child OCI
agent passed locally and on GCE. Storage, networking, and containerd integration
remain G4-G6 work.

## Intended layout

```text
runtime/
├── cmd/
│   ├── mk-host-check/
│   ├── mkruntimed/
│   └── containerd-shim-multikernel-v2/
├── agent/                 process manager and authenticated control service
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

## Local validation

```bash
GOCACHE=/tmp/mk-go-cache go test ./...
GOCACHE=/tmp/mk-go-cache go vet ./...
CGO_ENABLED=0 go build ./cmd/mk-agent
```

The GCE scripts are explicit, billable tests under `../scripts/test-runtime-g*.sh`.
The G3 AF_VSOCK compatibility path uses `../tools/mkvsock-relay.c`; direct Go
AF_VSOCK endpoints are prohibited for the pinned transport because live tests
reset the primary.
