# G3 learnings: child agent and OCI lifecycle

## Result

Gate G3 passed on GCE through the compatible transport path. A static
`mk-agent` booted from the runtime initramfs, authenticated the sandbox ID,
generation, endpoint, sequence, and HMAC, then managed a BusyBox OCI bundle
whose userspace was separate from the selected child kernel.

The live process printed distinct stdout/stderr, reported
`7.0.0-mk2-gce-lab`, and preserved exit code 23. `CreateProcess`,
`StartProcess`, `WaitProcess`, `DeleteProcess`, and `Shutdown` completed; the
child stopped and the primary returned to CPUs `0-15` with no instance or
pool. The exact result is in
[`g3-lifecycle.json`](../../../evidence/runtime-20260831/g0-g3-gce/g3-lifecycle.json).

## Transport compatibility finding

The recovery snapshot had `CONFIG_MULTIKERNEL_VSOCKETS` disabled. Rebuilding
the previously proved patched module produced the same SHA-256
`f7bcaf7f…b23495f` as the mediated-storage experiment.

Direct Go AF_VSOCK endpoints were not compatible with this pinned transport:
the first wrapper failed because Go could not interpret AF_VSOCK in
`getsockname`; subsequent data exchange through raw Go endpoints reset the
primary twice, once in each connection direction. Both resets returned with no
Multikernel resource leak, but this path is prohibited.

The passing design uses `mkvsock-relay`, a small static C bidirectional relay
derived from the already-proved socket setup. The primary listens, the child
connects to CID 0, and Go uses Unix sockets on both sides. Frames use an
explicit big-endian 32-bit length and a 1 MiB bound; no EOF or half-close is
used as a message delimiter.

## Supported OCI subset

The tested subset is exact argv without shell interpretation, environment,
cwd, UID/GID and supplementary-group plumbing, non-terminal split stdio,
signals, wait, delete, and exit status. Unsupported mounts, hooks,
capabilities, namespaces/resources, seccomp, masked/read-only paths,
`noNewPrivileges`, and terminal mode fail closed. `ExecProcess`, rlimit/umask
application, reconnect, and clean filesystem quiescence need expansion before
containerd integration; they do not invalidate the minimal direct-bundle G3
proof but remain required follow-up work.
