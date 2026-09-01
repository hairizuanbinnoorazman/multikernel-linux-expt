# Script index

Scripts are grouped here by artifact type so the root `Makefile` can sync them
to a stable remote path. For the thematic mapping, see
[`../docs/experiments/ASSET-MAP.md`](../docs/experiments/ASSET-MAP.md).

- Bring-up: `provision-kernel.sh`, `install-kerf.sh`, `verify-host.sh`,
  `build-initramfs.sh`, and `smoke-*.sh`.
- DAXFS: `daxfs-*.sh`, `build-daxfs-*.sh`, and `test-daxfs-*`.
- Direct ext4: `disk-roots-*.sh` and `test-ext4-*`.
- Mediated ext4: `mediated-*.sh`, `build-mediated-*.sh`, and
  `test-mediated-*`.
- Runtime gates: `test-runtime-g1.sh`, `test-runtime-g2.sh`,
  `test-runtime-g3.sh`, `test-runtime-g4-g6.sh`,
  `test-runtime-g4-g6-feature-matrix.sh`, and `test-runtime-recovery.sh`.
- Repository checks: `check-docs.sh`.

Destructive and billable operations must remain explicit Makefile targets;
they must not be hidden inside default checks.
