# ext4 disk experiment evidence index — 2026-08-30

The observed execution record is consolidated in
[the execution matrix](../../docs/experiments/ext4-direct/execution.md) and
[experiment learnings](../../docs/experiments/ext4-direct/learnings.md). The decisive evidence
was gathered with read-only commands and is summarized here for quick audit.

| Gate | Evidence | Result |
| --- | --- | --- |
| Cloud inventory | VM `mklinux-lab`; boot disk 100 GiB; two 10 GiB child disks | Created |
| Restored baseline | custom kernel, 16 vCPUs, guest agent, ext4 root, Kerf present | Pass |
| Blank-disk identity | Google by-id -> `sdb`; serial `mk-child-a-root`; no signatures | Pass |
| Live topology | `sda` HCTL `0:0:1:0`; `sdb` HCTL `0:0:2:0`; shared PCI `0000:00:03.0` | Hard stop |
| Pinned Kerf report | only PCI BDF `0000:00:03.0` accepted as the device | Hard stop |
| Pinned kernel source | device pool/transfer key is PCI domain/bus/devfn | Hard stop |
| C3/NVMe alternative | boot and child are `nvme0n1`/`nvme0n2` under PCI `0000:00:05.0` | Hard stop |
| Restored no-device regression | two four-vCPU children concurrent; host remained healthy | Pass |
| Primary absent-root bootstrap | bounded `root-not-found` diagnostic failure | Pass |
| Alternate kernel build/boot | distinct release `7.0.0-mk2-gce-lab-alt` | Pass |
| Distinct-kernel concurrency | both releases and separate absent UUIDs verified concurrently | Pass |
| Mutation check | no pool, instances, formatting, mount, or handoff | Pass |
| Final cloud cleanup | no instances or disks; two recovery snapshots `READY` | Pass |

The final resource disposition is recorded in
[`final-cloud-cleanup.txt`](final-cloud-cleanup.txt).

Reproduce the decisive guest check with:

```bash
sudo ./scripts/disk-roots-audit.sh \
  /dev/disk/by-id/google-mk-child-a-root
```

Exit status 2 is the expected result on the recorded N2/SCSI topology.
