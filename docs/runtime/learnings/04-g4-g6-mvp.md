# G4-G6 executable MVP

## Result

The executable path from a stock BusyBox OCI image to a dedicated
Multikernel child passed on the disposable GCE instance
`mklinux-g6-20260831`. `ctr` and Docker ran concurrently through
`io.containerd.multikernel.v2`; their containers reported different child
kernel boot IDs, and the primary boot ID remained unchanged. The retained
child output did not record `uname -r`, so the selected
`7.0.0-mk2-gce-lab` child release is expected from the configured kernel
manifest but is not independently established by this run.

This is an MVP milestone, not closure of every acceptance item in the G4-G6
plans. The master gate rows remain unchecked until their complete storage,
CNI, failure, and restart matrices pass.

## Implemented path

- Containerd or Docker remains responsible for pulling and unpacking the OCI
  image. The shim mounts the prepared root and builds a private, sorted,
  timestamp-free-gzip initramfs. The builder reads and copies the prepared
  root, but the run did not retain before/after snapshot digests or mount
  provenance. Snapshot non-mutation and reproducibility therefore remain
  remediation items rather than proved properties.
- Every Task v2 `Create` allocates a disjoint CPU pair and 3 GiB from
  `mkruntimed`, boots a child, authenticates its agent, and exposes the basic
  `Create`, `Start`, `State`, `Wait`, `Kill`, `Delete`, and `Exec` lifecycle.
- Each child receives a private static `/30` TUN interface. Framed packets are
  multiplexed over the authenticated agent stream to a primary-owned TUN;
  only the primary performs forwarding and NAT through the GCE NIC.
- Docker is invoked with `--network none` because the Multikernel runtime owns
  the child link. Docker's bridge must not also attach a veth to the shim PID.
- Generation-qualified daemon idempotency keys allow safe container-name
  reuse. The installed control-plane service uses a 90-second Kerf deadline
  because returning and reallocating the 16 GiB pool can exceed 30 seconds on
  this GCE machine.

## Repeatable proof

Run the privileged proof on an already qualified and configured host:

```bash
scripts/test-runtime-g4-g6.sh
```

The script starts a `ctr` task and a Docker container concurrently, checks
distinct child boot IDs and disjoint addresses, performs outbound DNS/HTTP,
rejects direct cross-sandbox traffic, executes a second process in each,
sends SIGKILL, checks Docker's exit status, removes both containers, and
asserts that no Kerf instance, TUN, iptables rule, containerd task, or Docker
container remains. It does not verify the `ctr` SIGKILL exit code, caller
snapshot immutability, rootfs mounts and artifacts, relay/shim/FIFO cleanup,
or the complete CPU, memory, storage, and recovery-record inventory.

Two consecutive clean-pool runs passed:

- [`g4-g6-proof-clean-pool.log`](../../../evidence/runtime-20260901/g4-g6-gce/g4-g6-proof-clean-pool.log)
- [`g4-g6-proof-repeat.log`](../../../evidence/runtime-20260901/g4-g6-gce/g4-g6-proof-repeat.log)
- [`g4-g6-environment.log`](../../../evidence/runtime-20260901/g4-g6-gce/g4-g6-environment.log)

These files support the narrow executable-MVP assertions. They retain observed
boot IDs and addresses, but not child kernel releases, expanded commands, DNS
answers, HTTP responses, live network rules, or source-snapshot digests. The
manifest is schema-valid, but the run used a dirty repository without a
retained diff and indexes G4/G5 assertions under `gate: G6`.

## Remaining full-gate work

- G4: the complete metadata, malformed-image, quota/ENOSPC, persistence,
  failure, and recovery matrix.
- G5: a normal CNI `ADD`/`CHECK`/`DEL` adapter, MTU/load/fault tests, and
  network-policy bypass tests.
- G6: shim crash task preservation/reconnect, event ordering, cancellation,
  faithful PID reporting, OCI fail-closed validation, and FIFO edge-case
  tests. A later follow-up added PTYs, stdin/attach, and resize support and
  exercised initial terminal-size propagation live. Containerd restart and
  shim-reclaim results remain retained operator observations rather than
  evidence-contract-quality proof; see
  [`05-g4-g6-robustness.md`](05-g4-g6-robustness.md).

The full implementation and replacement-instance evidence handoff is tracked
in the
[`G4-G6 remediation checklist`](g4-g6-remediation-checklist.md).
