# Read-only observation of storage growth

`scripts/observe-runtime-storage-geometry.py` supplements the unchanged mounted
storage identity guard for the reviewed whole-disk ext4 maintenance. Raw-device
growth alone must not qualify filesystem growth. The observer has no resize,
filesystem write, deployment activation or service-control operation.

It pins the existing mount guard, verifies the approved device/serial/UUID/label
and mounted ext4 identity, holds the matching block device read-only, and reads
only the 1024-byte primary superblock at offset 1024. Geometry is sampled twice
around a second mount-identity check. The parser uses the kernel's documented
[ext4 superblock fields](https://www.kernel.org/doc/html/latest/filesystems/ext4/super.html),
including high counts only with the 64-bit feature. It checks geometry/identity
and the five-percent reserved-block policy with less than one block of rounding.
It is not a checksum verifier, fsck, repair or hostile-filesystem attestation.

The caller pins old/new byte sizes and chooses exactly one phase:

| Phase | Raw device | Filesystem |
| --- | --- | --- |
| `before` | Old size | Old size |
| `disk-grown` | New size | Old size |
| `filesystem-grown` | New size | New size |

The existing binding's identity fields remain required during the transition;
the observer's expected raw bytes follow the selected phase. This does not
activate a new binding or prove that the running daemon loaded it. After an
approved grow, run the corresponding read-only phase check before proceeding
to the next maintenance step. Refusal does not authorize repeating resize2fs,
reducing reserves or modifying a historical baseline.

The previous approved 30-to-50 GiB grow required a separate reserve correction;
its [retained proof](../../../evidence/runtime-20261027/g8-data-disk-expansion/README.md)
shows why the ratio must be checked after growth. The proposed 50-to-58 GiB
maintenance still needs its separate approval. Native preparation must preserve
existing primary state and failed-image metadata without reading image contents.
The observer exports fixed flags and numeric geometry only; UUID/serial/label
and device paths stay private. CLI refusal is fixed and omits exception text.

This is a maintenance pre/postcondition, not staging admission or full G7/G8.
Direct children use no nested virtualization and share primary trust after
arbitrary child-kernel compromise.
