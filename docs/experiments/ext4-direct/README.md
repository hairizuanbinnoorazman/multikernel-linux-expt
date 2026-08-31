# Direct ext4 device-assignment experiment

This experiment tested whether separate GCE Persistent Disks could be assigned
directly to separate Multikernel children. It stopped at the mandatory topology
gate because the child disk and primary boot disk shared the same allocatable
controller. No controller was handed to a child.

- [`plan.md`](plan.md): original staged plan, including the mediated fallback.
- [`execution.md`](execution.md): pass, fail, blocked, and cleanup matrix.
- [`learnings.md`](learnings.md): topology analysis and subsequent findings.
- [`../../../evidence/ext4-disk-20260830/README.md`](../../../evidence/ext4-disk-20260830/README.md):
  raw evidence index.

The successful fallback is documented separately in the
[primary-mediated ext4 report](../ext4-mediated/report.md).
