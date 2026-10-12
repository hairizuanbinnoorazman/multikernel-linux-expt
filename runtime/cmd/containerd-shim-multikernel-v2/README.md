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
Current code validates configured stdio paths before Create/Exec mutation and
revalidates them on start and reconstruction. Linux `openat2` rejects symlinked
ancestors and magic links; stable device/inode, mode, owner, link count, and
ctime checks reject path replacement or metadata races. Stdin must be a
private caller-owned single-link FIFO, while output may also be a similarly
protected regular file. Current-revision live revalidation remains required.

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

## Source ownership

The shim remains one Go package so the Runtime v2 protocol surface and shared
state are unchanged, but operational subsystems have explicit file owners:

- `main.go` owns the executable entrypoint and runtime discovery;
- `service.go` owns shared service/process state, dependencies, construction,
  request locking, and capability requirements;
- `startup.go` owns worker launch, socket publication, and authentication of
  an existing shim;
- `cleanup.go` owns bundle/runtime directory identity and fallback cleanup;
- `recovery.go` owns persisted recovery validation, publication, and worker
  reconstruction;
- `state_files.go` owns token creation/loading and identity-bound state file
  publication/removal;
- `agent_transport.go` owns relay lifecycle, agent negotiation/reconnection,
  guest state validation, and guest control calls;
- `tasks_create.go` owns allocation, Create/Start orchestration, and rollback;
- `tasks_process.go` owns exec projection/validation, Exec, and Delete;
- `task_control.go` owns task reads, signals, shutdown, terminal control,
  pause/resume, and statistics;
- `process_io.go` owns validated stdio paths, FIFO keepers, and descriptor
  acquisition/closure;
- `process_streams.go` owns stdin replay, output delivery, process monitoring,
  and exit publication;
- `namespace_holder.go` owns the primary namespace-holder process;
- `network.go` owns endpoint validation, the packet pump, counter reporting,
  and bounded network shutdown;
- `rootfs.go` owns rootfs request normalization and daemon adapters;
- `supervisor.go` owns worker restart, containerd connection replay, private
  worker listeners, and supervisor PID publication; and
- `event_journal.go` owns durable event ordering, replay, and publication.

All files share the same generation-bound ownership rules: persisted identities
must be validated before mutation, recovery must not invent ownership, and
cleanup must remove only artifacts whose recorded identity still matches.

Aim for roughly 300–600 lines per implementation file. A cohesive responsibility
may extend to roughly 800 lines; small entrypoints and isolated helpers can be
shorter. Split by responsibility before reaching those bounds rather than
splitting a function or adding an artificial abstraction to meet a line count.
Tests follow the same responsibilities, with shared fakes in `*_fakes_test.go`.
The split preserves the existing service fields and locking behavior; changing
component ownership or lock ordering requires a separate behavioral review.
