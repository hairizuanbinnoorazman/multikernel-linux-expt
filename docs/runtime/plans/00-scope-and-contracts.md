# Plan 00: Scope, contracts, and repository baseline

## Purpose

Prevent architecture drift before implementation begins. Convert the existing
experiments into explicit inputs and freeze what the MVP does and does not
promise.

## Checks

- Inventory every existing script, patch, guest artifact, and evidence bundle.
- Verify the pinned Multikernel and Kerf revisions remain obtainable.
- Re-run link and shell syntax checks locally without touching GCE.
- Record which mediated-storage requirements are passed, incomplete, or known
  unsafe.
- Confirm licenses for Kerf, containerd libraries, Kata Agent concepts, and any
  code considered for reuse.
- Choose the initial implementation language. The default proposal is Go for
  the daemon, shim, and agent because containerd APIs and static binaries are
  readily available; keep the existing C transport helpers until replaced by
  tested equivalents.

## Minimum artifacts

- Versioned daemon/agent protocol schema.
- Sandbox identity and generation format.
- Lifecycle state machine and error taxonomy.
- Evidence manifest schema.
- Configuration schema separating host-global and per-sandbox settings.
- Threat-model statement naming trusted and untrusted actors.

## Contract tests to define before code

- Repeating `Create`, `Start`, `Stop`, and `Delete` is deterministic.
- A stale generation cannot connect to a reused sandbox identifier.
- Two requests cannot allocate the same CPU, memory range, port, or image.
- An interrupted transition is distinguishable from a completed transition.
- Unknown protocol fields are rejected or safely ignored according to version.
- Logs never include secrets or raw OCI credentials.

## Gate G0

Pass when the architecture, state machine, API methods, error classes, trust
boundary, version pins, and evidence format have reviewable documents and no
open decision changes the shape of the MVP.
