# G4-G6 GCE robustness evidence

This follow-up used disposable instance `mklinux-gates-20260901` in
`asia-southeast1-b`, project `${MK_PROJECT}`. The instance ran the
qualified `7.0.0-mk2-gce-lab` kernel on `n2-standard-16` with an auto-delete
100 GB boot disk restored from the retained pre-DAXFS snapshot.

Observed isolated results:

- `CONTAINERD_RESTART_RECONNECT_PASS`: boot ID
  `87bc230e-c64a-43e7-a8fa-bafc7dd38fff` before and after restart.
- `MKRUNTIMED_RESTART_RECONNECT_PASS`: boot ID
  `8a93d586-a542-44fb-bfe7-6adc1744fe49` before and after restart.
- `SHIM_CRASH_SAFE_RECLAIM_PASS`: forced PID 23004 death reclaimed the child,
  pool, `mkn0`, and matching iptables rules with the patched shim.
- `STORAGE_ENOSPC_ROLLBACK_PASS`: injected builder failure returned
  `No space left on device` and allocated no child or network resources.
- `TERMINAL_RESIZE_UNSUPPORTED_CONFIRMED`: terminal create returned nonzero
  before allocating a child.

The final clean inventory showed all four services active and no runtime,
network, or container resources. The formal result remains provisional because
shim task preservation, terminal resize, CNI operations, and the full storage
matrix remain open.
