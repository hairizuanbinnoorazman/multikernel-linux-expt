# `containerd-shim-multikernel-v2`

Containerd Runtime v2 shim planned by
[`../../../docs/runtime/plans/06-containerd-shim.md`](../../../docs/runtime/plans/06-containerd-shim.md).

The shim talks to `mkruntimed`. It must not invoke Kerf or write to the
Multikernel filesystem directly.
