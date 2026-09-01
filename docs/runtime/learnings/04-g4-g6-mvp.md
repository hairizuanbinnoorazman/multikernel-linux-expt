# G4-G6 executable MVP

## Result

The executable path from a stock BusyBox OCI image to a dedicated
Multikernel child passed on the disposable GCE instance
`mklinux-g6-20260831`. `ctr` and Docker ran concurrently through
`io.containerd.multikernel.v2`; their containers reported different child
kernel boot IDs and the selected `7.0.0-mk2-gce-lab` release. The primary boot
ID remained unchanged.

This is an MVP milestone, not closure of every acceptance item in the G4-G6
plans. The master gate rows remain unchecked until their complete storage,
CNI, failure, and restart matrices pass.

## Implemented path

- Containerd or Docker remains responsible for pulling and unpacking the OCI
  image. The shim mounts the prepared root and builds a private, sorted,
  timestamp-free-gzip initramfs without modifying the caller's snapshot.
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
container remains.

Two consecutive clean-pool runs passed:

- [`g4-g6-proof-clean-pool.log`](../../../evidence/runtime-20260901/g4-g6-gce/g4-g6-proof-clean-pool.log)
- [`g4-g6-proof-repeat.log`](../../../evidence/runtime-20260901/g4-g6-gce/g4-g6-proof-repeat.log)
- [`g4-g6-environment.log`](../../../evidence/runtime-20260901/g4-g6-gce/g4-g6-environment.log)

## Remaining full-gate work

- G4: the complete metadata, malformed-image, quota/ENOSPC, persistence,
  failure, and recovery matrix.
- G5: a normal CNI `ADD`/`CHECK`/`DEL` adapter, MTU/load/fault tests, and
  network-policy bypass tests.
- G6: terminal resize, shim crash task preservation/reconnect, event ordering,
  cancellation, and FIFO edge-case tests. A later follow-up passed containerd
  and daemon restart reconnect plus leak-free shim-crash reclaim; see
  [`05-g4-g6-robustness.md`](05-g4-g6-robustness.md).
