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
