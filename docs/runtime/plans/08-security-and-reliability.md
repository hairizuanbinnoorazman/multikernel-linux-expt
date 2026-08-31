# Plan 08: Security and reliability qualification

## Purpose

Define a defensible preview security posture and prove recovery under component
failure. This gate does not attempt to turn Multikernel into a KVM-equivalent
security boundary.

## Security work

- Threat-model primary daemon, shim, child agent, workload, image, control
  transport, storage transport, and network transport.
- Minimize daemon capabilities and filesystem access.
- Authenticate peers and bind credentials to sandbox ID and generation.
- Validate every length, offset, path, identity, and state transition.
- Fuzz protocol decoders, OCI conversion, state recovery, and transport
  framing.
- Audit unsafe code and privileged subprocess invocation.
- Produce dependency, license, and vulnerability inventories.
- Document why the preview is restricted to trusted workloads.

## Failure matrix

Inject failure into each of these while idle and under I/O:

- shim process;
- `mkruntimed`;
- `mk-agent`;
- child kernel;
- storage server and connection;
- network server and connection;
- containerd;
- primary userspace restart; and
- GCE stop/start and reset.

For each case record detection time, workload effect, data effect, remaining
resources, reconciliation action, and final cleanup.

## Long-running tests

- Repeated create/start/stop/delete cycles.
- Parallel sandbox churn.
- Resource exhaustion and recovery.
- Sustained storage and network load.
- Log pressure and slow/unread clients.
- Clock changes and timeout behavior.

## Gate G8

Pass when there are no silent resource leaks in the tested failure matrix,
protocol fuzzing finds no unresolved high-severity issue, privilege boundaries
are documented, and the release security statement is precise about trusted
single-tenant use.
