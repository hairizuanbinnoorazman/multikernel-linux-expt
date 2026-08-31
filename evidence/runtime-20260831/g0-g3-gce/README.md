# G0-G3 GCE evidence

## Run identity

- Project: `new-demo-project-462517`
- Instance: `mklinux-runtime-g0-g3`
- Zone/type: `asia-southeast1-b`, `n2-standard-16`
- Boot source: `mklinux-lab-pre-daxfs-20260828-2030`
- Source revision before changes: `059e612`
- Kernel: Multikernel `3bdd35b64413da0b4e089ce931bfc2e8b031cbf7`
- Kerf: `v0.2.0`
- Result: G0, G1, G2, and the minimal G3 gate passed

## Files

- `g1-host-report.json`: concise read-only qualification result.
- `g1-crash-reclaim.log`: intentional child PID-1 crash and cleanup pass.
- `g2-control-plane.log`: daemon SIGKILL/restart/reconcile/cleanup pass.
- `g3-vsock-build.log`: patched transport module build and hash.
- `g3-lifecycle.json`: authenticated OCI process result and capabilities.
- `runtime-g3-console.log`: decisive passing child console.
- `runtime-host-final.txt`: final host, boot ID, guest agent, CPU, pool, and
  instance state.

Empty `g3-control.err` and `g3-relay.log` files indicate that the decisive
control and relay paths emitted no error.

The disposable fixed G3 authentication fixture was redacted from the two
kernel-command-line records in `runtime-g3-console.log`. The downloaded source
archive was removed after extraction because it retained the unredacted copy;
all indexed evidence files remain available.

## Resource ledger

Before: no GCE instances; retained snapshots and the pre-existing detached
`mk-mediated-storage-20260830` disk. The run created one labeled instance and
one 100 GiB `pd-balanced` boot disk with auto-delete enabled.

After: `mklinux-runtime-g0-g3` was deleted and its boot disk no longer
appeared. No GCE instances remain. The only listed disk is the pre-existing
20 GiB `mk-mediated-storage-20260830`, intentionally retained by the earlier
experiment and not modified by this run.

## Failed attempts retained as learnings

The live sequence exposed Kerf byte syntax, Kerf's committed-create/nonzero
exit, missing transport qualification, Go AF_VSOCK incompatibility, and two
primary resets. See the per-gate learning documents. The final host report was
captured after the decisive pass and proves no child/pool resource leak.
