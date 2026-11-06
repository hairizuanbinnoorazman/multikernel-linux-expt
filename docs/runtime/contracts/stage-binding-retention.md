# Retaining a private lab stage alias

A storage-backed disposable stage may be bound at an `/opt/mkruntime/g8-host-*`
alias while its files live under `g8-stage-*` on the dedicated ext4 volume.
The production storage validator deliberately permits one device mount only.
A retained stage bind is therefore incompatible with that startup guard until
released. Do not weaken the guard or restart services while the alias is bound.
Reboot removes this temporary alias; that alone is not artifact loss or cleanup.

`manage-runtime-stage-binding.py` records and manages exactly one such alias.
It is a root-only lab tool, not a general mount manager or guest recovery tool.
Capture an existing binding, with a known filesystem UUID, into a new mode-0600
record in an existing root-owned mode-0700 directory outside the alias:

```bash
sudo python3 scripts/manage-runtime-stage-binding.py capture \
  --record /PRIVATE-PARENT/binding.json \
  --volume /srv/multikernel-storage --uuid EXPECTED_FILESYSTEM_UUID \
  --backing /srv/multikernel-storage/g8-stage-SOURCE7 \
  --alias /opt/mkruntime/g8-host-SOURCE7
```

Retain its SHA-256 through a trusted private channel. Subsequent commands require
that independent pin; a digest computed from untrusted input grants no authority:

```bash
sudo python3 scripts/manage-runtime-stage-binding.py check \
  --record /PRIVATE-PARENT/binding.json --record-sha256 TRUSTED_SHA256
sudo python3 scripts/manage-runtime-stage-binding.py release \
  --record /PRIVATE-PARENT/binding.json --record-sha256 TRUSTED_SHA256
sudo python3 scripts/manage-runtime-stage-binding.py restore \
  --record /PRIVATE-PARENT/binding.json --record-sha256 TRUSTED_SHA256
```

Capture validates current topology before any namespace operation. For a live
alias, it inspects the covered empty directory in a forked private mount
namespace and verifies that the parent's binding remains. The private record
binds the source filesystem UUID, persistent backing/underlay/parent inodes,
paths and historical capture boot. Device numbers are checked against the UUID
link in the current boot, rather than carried forward as reboot-stable identity.
No backing contents, guest output, configuration or credentials are opened.

Before each operation the tool checks protected canonical directories, exact
root-only modes, source and target identity, ext4 UUID/device/type/options and
mount topology. It permits only the base volume plus this exact alias, with no
nested aliases, unexpected duplicate device mounts or foreign target mounts.
A release rechecks the underlay in a private namespace before detaching the
real alias. A busy mount refuses: there is no lazy/forced unmount, signaling or
repair. Restore requires the original empty underlay and binds descriptor-pinned
source/target objects. Existing correct state is idempotent; replaced, occupied,
aliased, malformed, shared, unpinned or mismatched records/paths refuse.

The namespace child has a five-second observation wait and is reaped; kernel
metadata/mount calls are not a hard wall-clock guarantee under uninterruptible
I/O. The tool samples pathname identity and trusts root administration; it does
not exclude unobserved ABA changes or root/child-kernel compromise. Operation
failure can leave state to inspect; do not overwrite the private record, force
unmount, or infer completion from a missing alias. The helper does not alter
production mount policy, fstab, services, transport or backing files.

Success emits fixed status flags, never the private record, UUID, inode/path
inventory or record digest. Capture/check/release/restore never qualify guest
cleanup or cold restoration. After release, independently run the existing
production storage-mount validator with its approved deployment parameters,
then follow the normal idle maintenance/restart procedure. After a reboot,
requalify the mounted volume and record before any alias restore; capture boot
is historical, not current process/session authority. Full G7/G8 remain open.
Direct child kernels require no nested virtualization and share host trust
after arbitrary child-kernel compromise.
