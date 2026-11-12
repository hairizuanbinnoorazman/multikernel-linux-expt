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

The [committed-source and native proof](../../../evidence/runtime-20261112/g8-storage-geometry-whitelist/README.md)
passes six controls plus the actual before-phase observation: raw device and
ext4 both span 50 GiB, with 4096-byte blocks and exactly five-percent reserve.
An installed independent ext4 header reader agrees. Both not-yet-performed
post-grow phases refuse on the actual old device before metadata reads.
Independent verification preserves prior state/refused attempts, stops only
the successful own unit and requires its cgroup absent. Post-grow phases remain
unqualified until approved maintenance actually produces those states.

Before irreversible maintenance, `scripts/plan-runtime-storage-binding.py`
checks the selected immutable deployment and prepares the proposed byte-size
change through the pinned manager's existing validation path. It intercepts
the persistence entry point before directory creation or selection. Only
`runtime.env` may differ, all asset modes must match, and an already existing
candidate directory (including a dangling alias) refuses. The selected
deployment is verified again after preparation. No disk or service operation
is invoked. CLI output contains fixed flags and old/new byte counts; deployment
IDs, file inventory and environment values remain private.

Six controls compare the prediction with a real update in a disposable copied
root, require unchanged source-root content and metadata, and exercise stale
size, shrink, alignment, writable assets, candidate collisions, manager pinning
and private CLI refusals. This is a point-in-time plan, not an activation or a
guarantee against later changes. The real manager must revalidate at execution,
after separate maintenance approval and geometry qualification.

The [native installed-binding proof](../../../evidence/runtime-20261112/g8-storage-binding-plan-corrected/README.md)
now verifies this plan on the actual GCE deployment. Independent verification
recomputes the private plan identically and checks unchanged installed graph
metadata, prior proofs/refusal metadata, primary state and before-phase geometry.
This qualifies preparation only; the proposed increase and actual activation
remain unperformed.
