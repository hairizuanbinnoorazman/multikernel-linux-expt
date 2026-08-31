# Experiment asset map

The executable assets remain grouped by artifact type because the root
`Makefile` and GCE sync workflow depend on those stable paths. This map groups
the same files by experiment theme.

## Shared GCE and Multikernel bring-up

| Role | Assets |
| --- | --- |
| Cloud entry point | [`../../Makefile`](../../Makefile) |
| Project discovery | [`../../scripts/detect-gcp-project.sh`](../../scripts/detect-gcp-project.sh) |
| Kernel provisioning | [`../../scripts/provision-kernel.sh`](../../scripts/provision-kernel.sh) |
| Kerf installation | [`../../scripts/install-kerf.sh`](../../scripts/install-kerf.sh) |
| Host qualification | [`../../scripts/verify-host.sh`](../../scripts/verify-host.sh) |
| Minimal child root | [`../../guest/init`](../../guest/init), [`../../scripts/build-initramfs.sh`](../../scripts/build-initramfs.sh) |
| Two-child lifecycle | [`../../scripts/smoke-up.sh`](../../scripts/smoke-up.sh), [`../../scripts/smoke-down.sh`](../../scripts/smoke-down.sh) |

## DAXFS and Docker-derived roots

| Role | Assets |
| --- | --- |
| Build and lifecycle | [`../../scripts/daxfs-build.sh`](../../scripts/daxfs-build.sh), [`../../scripts/daxfs-up.sh`](../../scripts/daxfs-up.sh), [`../../scripts/daxfs-status.sh`](../../scripts/daxfs-status.sh), [`../../scripts/daxfs-down.sh`](../../scripts/daxfs-down.sh) |
| Root builders | [`../../scripts/build-daxfs-root.sh`](../../scripts/build-daxfs-root.sh), [`../../scripts/build-daxfs-initramfs.sh`](../../scripts/build-daxfs-initramfs.sh) |
| Child proofs | [`../../guest/daxfs-bootstrap-init`](../../guest/daxfs-bootstrap-init), [`../../guest/daxfs-proof.sh`](../../guest/daxfs-proof.sh), [`../../guest/docker-proof.sh`](../../guest/docker-proof.sh) |
| Docker input | [`../../docker/daxfs-proof.Dockerfile`](../../docker/daxfs-proof.Dockerfile) |
| Compatibility patches | [`../../patches/kerf-v0.2.0-skip-special-files.patch`](../../patches/kerf-v0.2.0-skip-special-files.patch), [`../../patches/kerf-v0.2.0-hardlink-inode-count.patch`](../../patches/kerf-v0.2.0-hardlink-inode-count.patch) |
| Extended tests | [`../../scripts/test-daxfs-dual-kernel.sh`](../../scripts/test-daxfs-dual-kernel.sh), [`../../scripts/test-daxfs-corruption.py`](../../scripts/test-daxfs-corruption.py) |

## Direct ext4 feasibility

| Role | Assets |
| --- | --- |
| VM/disk preparation | [`../../scripts/disk-roots-create.sh`](../../scripts/disk-roots-create.sh) |
| Read-only topology gate | [`../../scripts/disk-roots-audit.sh`](../../scripts/disk-roots-audit.sh) |
| Safe missing-root bootstrap | [`../../guest/ext4-bootstrap-init`](../../guest/ext4-bootstrap-init), [`../../scripts/test-ext4-bootstrap-no-disk.sh`](../../scripts/test-ext4-bootstrap-no-disk.sh) |
| Distinct-kernel regression | [`../../scripts/test-ext4-dual-kernel-no-disk.sh`](../../scripts/test-ext4-dual-kernel-no-disk.sh) |

## Primary-mediated ext4

| Role | Assets |
| --- | --- |
| Transport proof | [`../../tools/mkvsock-probe.c`](../../tools/mkvsock-probe.c), [`../../guest/mediated-transport-init`](../../guest/mediated-transport-init), [`../../scripts/test-mediated-transport.sh`](../../scripts/test-mediated-transport.sh) |
| Block transport | [`../../tools/mkvsock-nbd.c`](../../tools/mkvsock-nbd.c), [`../../guest/mediated-root-bootstrap-init`](../../guest/mediated-root-bootstrap-init), [`../../guest/mediated-disk-root-init`](../../guest/mediated-disk-root-init) |
| Image preparation | [`../../scripts/mediated-storage-prepare.sh`](../../scripts/mediated-storage-prepare.sh), [`../../scripts/mediated-image-prepare.sh`](../../scripts/mediated-image-prepare.sh) |
| Root builders | [`../../scripts/build-mediated-transport-initramfs.sh`](../../scripts/build-mediated-transport-initramfs.sh), [`../../scripts/build-mediated-root-initramfs.sh`](../../scripts/build-mediated-root-initramfs.sh) |
| Lifecycle tests | [`../../scripts/test-mediated-root-a.sh`](../../scripts/test-mediated-root-a.sh), [`../../scripts/test-mediated-dual-root.sh`](../../scripts/test-mediated-dual-root.sh) |
| Kernel compatibility | [`../../patches/linux-v7.0-mk2-vsock-build-safety.patch`](../../patches/linux-v7.0-mk2-vsock-build-safety.patch) |

## Runtime development

The future container-runtime source belongs under [`../../runtime/`](../../runtime/README.md).
Experiment helpers should move into runtime packages only after their behavior
is covered by the corresponding G0–G10 contract tests.
