# Child guest artifacts

- `init`: minimal CPU, memory, initramfs, and MKTTY bring-up.
- `daxfs-*` and `docker-proof.sh`: DAXFS and Docker-derived root proofs.
- `ext4-bootstrap-init`: bounded rejection of a missing direct ext4 root.
- `mediated-*`: AF_VSOCK transport and primary-mediated ext4 roots.

See the [experiment asset map](../docs/experiments/ASSET-MAP.md) for the
matching builders, tests, reports, and evidence.
