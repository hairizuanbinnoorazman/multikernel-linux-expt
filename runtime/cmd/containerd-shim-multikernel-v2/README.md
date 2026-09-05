# `containerd-shim-multikernel-v2`

Containerd Runtime v2 shim planned by
[`../../../docs/runtime/plans/06-containerd-shim.md`](../../../docs/runtime/plans/06-containerd-shim.md).

The shim talks to `mkruntimed`. It must not invoke Kerf or write to the
Multikernel filesystem directly.

Guest stdin and output are bridged in bounded authenticated chunks. The shim
keeps containerd's FIFO endpoints reusable after detach, drains buffered stdin
before forwarding `CloseIO`, and maps terminal processes to the child PTY and
`ResizePty` path. These paths passed through both `ctr` and Docker on the
[qualified disposable-host run](../../../evidence/runtime-20260902/g4-g6-io-live/README.md).

The Task PID is a versioned guest PID (`multikernel-v1-guest-pid`); the shim
supervisor PID remains the pre-start Create identity. The serving worker is
supervised behind the inherited Runtime v2 listener. Its PID is retained in
`.multikernel-worker.pid`, while the versioned `.multikernel/sandbox.json`
record contains the sandbox generation, process/FIFO/terminal state, guest
PIDs, exits, output offsets, and network identity needed by a replacement
worker. Agent calls have bounded deadlines and honor request cancellation.

`Stats`, `Pause`, and `Resume` are supported. Dynamic resource `Update` and
`Checkpoint` remain explicit `NotImplemented` operations: Kerf does not yet
offer safe live CPU/memory reassignment or whole-child checkpoint/restore, and
the minimum G6 surface does not advertise either operation.
