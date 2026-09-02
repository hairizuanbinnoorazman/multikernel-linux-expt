# Runtime workspace

This directory contains the G0-G6 Multikernel runtime MVP described
in [`../docs/runtime/architecture.md`](../docs/runtime/architecture.md). Host
qualification, the Kerf control-plane happy path, and the minimal child OCI
agent reached provisional local and GCE milestones. A private OCI-root initramfs, authenticated
primary-mediated TUN networking, and the containerd Runtime v2 shim also passed
the G4-G6 happy-path proof through both `ctr` and Docker. The broader gate
failure/recovery matrices remain open.

Guest stdin, detach/reattach, terminal execution, and Runtime v2 resize are
implemented. Stdin/attach, PTY execution, and initial terminal-size propagation
passed through both `ctr` and Docker on a qualified disposable GCE host; a
deliberate post-start resize has local agent coverage but still needs live
revalidation. The FIFO/PTY paths also pass the current local unit suite and race
detector, with broader backpressure/failure coverage still open.

None of G0 through G6 is a closed normative gate. See the
[G0-G3](../docs/runtime/learnings/g0-g3-remediation-checklist.md) and
[G4-G6](../docs/runtime/learnings/g4-g6-remediation-checklist.md) remediation
audits for the demonstrated boundary and remaining contract, implementation,
test, and evidence work.

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
sudo ../scripts/test-runtime-g4-g6.sh # qualified, configured GCE host only
sudo ../scripts/test-runtime-g4-g6-feature-matrix.sh # disposable GCE host only
```

The GCE scripts are explicit, billable tests under `../scripts/test-runtime-g*.sh`.
The G3 AF_VSOCK compatibility path uses `../tools/mkvsock-relay.c`; direct Go
AF_VSOCK endpoints are prohibited for the pinned transport because live tests
reset the primary.
