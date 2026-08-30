# Plan 07: Kubernetes and CRI integration

## Purpose

Run a Kubernetes pod sandbox in one Multikernel child without bypassing normal
containerd CRI behavior.

## Minimum implementation

- Installable containerd runtime configuration.
- A `RuntimeClass` named `multikernel` or versioned equivalent.
- Pod-sandbox metadata mapping and pause-container handling.
- Multiple containers inside one child sandbox.
- CNI integration from Plan 05.
- Kubernetes log, exec, attach, stop, restart-policy, and termination-grace
  behavior.
- Node labels/taints and an extended resource or admission rule preventing the
  scheduler from overcommitting Multikernel sandboxes.

## Tests

- One pod, two containers, shared pod network, and separate container roots.
- Init container followed by application containers.
- ConfigMap, Secret, projected service-account, `emptyDir`, and read-only
  volume behavior.
- Liveness/readiness/startup probes.
- Graceful deletion and forced deletion.
- Kubelet and containerd restart while the pod runs.
- Node reboot reconciliation.
- Resource requests that cannot be mapped to whole CPUs or contiguous memory.
- Concurrent ordinary `runc` and Multikernel `RuntimeClass` pods.
- `critest` subset with every skipped case explained.

## Scheduling boundary

Kubernetes millicpu requests do not map directly to dedicated Multikernel
CPUs. The preview runtime must require an explicit sandbox resource policy and
must not silently round allocations in a way that permits node overcommit.

## Gate G7

Pass when a normal pod manifest using `runtimeClassName` completes its full
lifecycle, two containers share one child correctly, node services survive,
and scheduling cannot allocate overlapping or unavailable Multikernel
resources.
