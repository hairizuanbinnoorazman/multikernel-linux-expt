# Plan 07: Security and reliability qualification

## Purpose

Define a defensible preview security posture and prove recovery under component
failure. This gate does not attempt to turn Multikernel into a KVM-equivalent
security boundary.

## Threat-model method and security backlog

Use [STRIDE across data flows and trust boundaries](../research/stride-threat-model.md)
and execute the [G7 security task checklist](g7-security-checklist.md).
The model includes the project's hypothesis that separate Linux kernels could
make workload escape harder than gVisor. That comparative claim is unproven;
child-kernel compromise must be evaluated separately from process compromise,
and the existing lack of a demonstrated hostile-child hardware boundary remains
an architectural limitation. Closing G7 qualifies the documented trusted preview,
not a claim of superiority to gVisor or hostile multi-tenant isolation.

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

## Gate G7

Pass when there are no silent resource leaks in the tested failure matrix,
protocol fuzzing finds no unresolved high-severity issue, privilege boundaries
are documented, and the release security statement is precise about trusted
single-tenant use.
