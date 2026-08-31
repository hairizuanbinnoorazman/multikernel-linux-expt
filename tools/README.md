# Native experiment helpers

- `mkvsock-probe.c`: bounded Multikernel AF_VSOCK integrity proof.
- `mkvsock-nbd.c`: authenticated, bounded NBD-to-AF_VSOCK adapter used by the
  primary-mediated ext4 experiment.

These are experimental helpers, not production runtime services. Their tested
behavior and remaining transport limitations are documented in the
[mediated ext4 report](../docs/experiments/ext4-mediated/report.md).
