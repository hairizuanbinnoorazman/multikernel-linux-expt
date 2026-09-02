# G4-G6 robustness follow-up

## Result

The 2026-09-01 follow-up on disposable GCE instance
`mklinux-gates-20260901` remains **provisional**, but closes several concrete
fresh-host and recovery gaps. The host used the qualified
`7.0.0-mk2-gce-lab` kernel, an `n2-standard-16` machine, and an auto-delete
100 GB boot disk restored from `mklinux-lab-pre-daxfs-20260828-2030`.

Clean isolated reruns proved:

- containerd restart reconnected to a running shim and retained the same child
  boot ID;
- `mkruntimed` restart retained the same running child and boot ID;
- an injected initramfs-builder ENOSPC error failed creation before allocating
  a child or network resource;
- forced shim death safely reclaimed the Kerf child, memory pool, TUN device,
  NAT rule, and forwarding rules after network recovery identity was persisted.

## Fresh-host fixes

The snapshot deliberately lacked runtime packages and image caches. Recreating
the service exposed bootstrap assumptions that the earlier warm-host MVP did
not catch:

- `make sync` omitted `guest/mk-agent-init`;
- `/sys/fs/multikernel` was not mounted by a persistent unit;
- Docker needed the Runtime v2 `runtimeType` registration rather than a
  runc-compatible `path` registration;
- the baseline proof assumed BusyBox was already present in both image stores.

The sync list, systemd mount dependency, Docker example configuration, and
explicit image pulls now cover these conditions.

## Remaining formal failures

- On the robustness-test build, terminal task creation was rejected before
  child allocation. Consequently terminal resize could not be exercised in
  that retained live run.
- Forced shim death now has bounded, leak-free safe reclaim, but the running
  task is not reconstructed or reconnected. Safe reclaim is not equivalent to
  the G6 reconnect requirement.
- No CNI configuration or Multikernel CNI binary was installed; normal CNI
  `ADD`, `CHECK`, and `DEL` remain unimplemented.
- The injected ENOSPC rollback covers only one storage failure point. Metadata,
  malformed-image variants, inode/block exhaustion, high-water refusal,
  persistence, corruption, and recovery still require the full G4 matrix.

The final inventory contained no Multikernel instances, containerd tasks or
containers, Docker containers, `mkn*` links, Multikernel NAT/forwarding rules,
or temporary systemd environment overrides.

## Shared `ctr` and Docker feature matrix

On 2026-09-01, a new disposable `n2-standard-16` instance,
`mklinux-g4-g6-matrix-20260901`, was restored from the qualified pre-DAXFS
snapshot and rebuilt from repository commit
`5e9fe33b7d54273c987fd21673f34ccde2121018` plus the working-tree matrix
harness. The host qualification report passed before the runtime was used.

The executable matrix then passed through both clients:

- image pull/inspection, combined foreground run, and split create/start;
- state inspection, exec, stdout/stderr, blocking wait, and nonzero exit;
- TERM, exit observation, deletion, full resource return, and two clean
  same-name reuse cycles;
- distinct child-kernel identity, private writable roots, outbound DNS/HTTP,
  and bidirectional sibling-link isolation; and
- `mkruntimed` restart while both clients' workloads remained live, preserving
  both child boot IDs.

Terminal/resize and pause/resume were also invoked through both clients and
rejected while leaving no resources behind. Those are confirmed unsupported
paths, not passes. Task `Stats`, `Update`, and `Checkpoint`, guest stdin/attach,
faithful guest PIDs, full OCI controls, CNI, and shim-crash task reconnection
remain unimplemented or partial. Containerd 2.2.2 does not expose a standalone
`ctr tasks wait`; the blocking `ctr run` path exercised Task `Wait`, while
Docker was additionally checked with `docker wait`.

The exact command mapping is kept in the [top-level feature matrix](../../../README.md#g4-g6-ctr-and-docker-feature-matrix).
Raw proof, component hashes, image digest, qualified-host report, and the final
clean inventory are indexed in the
[feature-matrix evidence](../../../evidence/runtime-20260901/g4-g6-feature-matrix/README.md).
This expands the proved G4-G6 surface but does not close the full gates.

## Post-matrix terminal implementation

After the disposable matrix host was deleted, the child agent gained a real
PTY path for init and exec processes and a validated `ResizeProcess` operation.
The Runtime v2 shim now accepts terminal tasks, forwards `ResizePty`, and
retains an initial window size sent before process start. Terminal execution,
combined terminal output, live resize, invalid-size rejection, and pre-start
resize retention pass local unit tests and the Go race detector.

## Guest I/O and terminal live revalidation

On 2026-09-02, disposable instance `mklinux-g4-g6-io-20260901` was restored
from the qualified snapshot and ran the updated shared matrix to completion.
Both `ctr` and Docker passed foreground guest stdin, detach followed by live
reattachment, terminal execution, and a live resize to 91 columns by 37 rows.

The run exposed and fixed two fresh-host-only gaps before the retained pass:

- Task `CloseIO` could overtake bytes already buffered in the stdin FIFO. The
  shim now drains those bytes before delivering guest EOF.
- The guest initramfs mounted `devtmpfs` but not `devpts`, so accepting
  `terminal=true` still left PTY allocation unusable. The guest now mounts
  `devpts` before binding `/dev` into the OCI root, and the private OCI config
  preserves the caller's terminal bit.

The final qualified-host report passed, the matrix ended with
`G4_G6_CTR_DOCKER_FEATURE_MATRIX_PASS`, all child/container/network inventories
were empty, and the disposable VM and auto-delete disk were removed. Raw proof
is indexed in the [2026-09-02 guest-I/O evidence](../../../evidence/runtime-20260902/g4-g6-io-live/README.md).

This closes the earlier implemented-but-not-live-revalidated terminal status
and the guest stdin/attach implementation gap. It does not close pause/resume,
`Stats`, `Update`, `Checkpoint`, faithful guest PIDs, CNI, the full G4 storage
matrix, or shim-crash task reconnection.
