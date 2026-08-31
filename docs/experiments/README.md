# Experiment index

[The asset map](ASSET-MAP.md) connects each experiment below to its scripts,
guest programs, patches, Docker inputs, and native helpers.

## Primary and child-kernel bring-up

- [Complete GCE runbook](../guides/gce-lab-runbook.md)
- [Chronological field notes](field-notes-20260828.md)
- Raw bring-up evidence is retained with the relevant later experiment
  bundles under [`../../evidence/`](../../evidence/).

## DAXFS and Docker-derived roots

- [Plan](daxfs/plan.md)
- [Implementation and GCE report](daxfs/report.md)
- [Evidence index](../../evidence/daxfs-20260828/README.md)

Headline: single-child and shared-read-only roots passed. Shared writable
DAXFS coherence failed and must not be treated as supported.

## Direct ext4 device assignment

- [Experiment overview](ext4-direct/README.md)
- [Plan](ext4-direct/plan.md)
- [Execution matrix](ext4-direct/execution.md)
- [Learnings](ext4-direct/learnings.md)
- [Evidence index](../../evidence/ext4-disk-20260830/README.md)

Headline: the safe path was rejected because the boot disk and candidate child
disks shared one allocatable PCI controller.

## Primary-mediated ext4 roots

- [Implementation and GCE report](ext4-mediated/report.md)
- [Evidence index](../../evidence/ext4-mediated-20260830/README.md)

Headline: two persistent, isolated ext4 roots passed concurrently while the
primary retained the storage controller. Production hardening remains open.
