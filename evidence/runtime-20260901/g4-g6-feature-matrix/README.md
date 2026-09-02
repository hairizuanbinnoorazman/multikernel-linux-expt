# G4-G6 `ctr` and Docker feature-matrix evidence

Disposable GCE instance `mklinux-g4-g6-matrix-20260901` ran in
`asia-southeast1-b` on `n2-standard-16`, booted from a 100 GB auto-delete disk
restored from `mklinux-lab-pre-daxfs-20260828-2030`. Secure Boot was disabled.

The host qualification report passed on `7.0.0-mk2-gce-lab`. Containerd
2.2.2 and Docker 29.1.3 used `docker.io/library/busybox:1.36` at digest
`sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662`.

Evidence:

- [`g4-g6-feature-matrix.log`](g4-g6-feature-matrix.log) is the successful
  marker transcript. It contains 13 shared pass rows, two explicit unsupported
  rows, and the terminal `G4_G6_CTR_DOCKER_FEATURE_MATRIX_PASS`; expanded
  commands and observed assertion values were not retained.
- [`g4-g6-feature-matrix-environment.log`](g4-g6-feature-matrix-environment.log)
  records client/server versions, pinned source revisions, installed component
  hashes, image digest, five active services, all 16 CPUs returned, and empty
  Multikernel/containerd/Docker/network inventories.
- [`g4-g6-host-report.json`](g4-g6-host-report.json) is the pre-run qualified
  host report.
- [`cloud-cleanup.txt`](cloud-cleanup.txt) records deletion of the disposable
  instance and auto-delete boot disk and the final absence check.
- [`manifest.json`](manifest.json) maps assertions to those raw files.

The manifest is a non-conforming historical index because its redacted boot ID
does not satisfy the current schema, the run was dirty without a retained diff,
and its G6 label also indexes G4/G5 assertions. The pass rows remain useful when
read with the retained harness hash, but are not full-gate evidence.

The pass covers the currently supported shared client surface. It does not
turn expected TTY/resize or pause/resume rejection into support, and it does
not close the remaining G4-G6 full-gate items listed in the learning document.
