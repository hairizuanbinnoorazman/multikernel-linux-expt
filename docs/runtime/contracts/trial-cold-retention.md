# Private trial artifacts across a cold boot

`runtime_trial_retention.py` records an explicit set of private control artifacts
before a reboot and verifies their bytes, inode and metadata on a different
caller-pinned boot. Explicit absent paths must remain absent, including failed
phase checkpoints and missing completion receipts. It never discovers artifacts,
inspects a backing image, signals a process, changes a module or repairs evidence.

The operator separately verifies the actual boot ID, dedicated storage mount,
original transport and idle resources. The retention result alone does not
qualify cold restoration, workload completion, recovery or reuse. Historical
units/PIDs never become live authority. A same-boot verification refuses.

The explicit existing and absent set has at most 256 entries. Files must be
caller-owned regular single-link files without group/other writes or special
mode bits, no larger than 128 MiB each. Parent traversal is anchored to directory
file descriptors and rejects symlinks and unsafe owners/writes (root-owned sticky
ancestors such as `/tmp` are permitted). Reads are bounded, no-follow and
nonblocking, and recheck open/named identity and metadata. The complete existing
and absent set is observed twice. These samples do not establish atomicity or
exclude every ABA change.

Manifests contain private paths, byte hashes and metadata; keep them on the
qualified host. Publication requires a caller-owned mode-0700 parent, creates a
new mode-0600 file exclusively, and syncs file and directory. Existing files are
never replaced. A publication failure may retain an incomplete file; explicitly
refuse that generation instead of overwriting it. Loading rejects duplicate JSON
keys, extra fields, malformed metadata and non-private manifest modes. Only fixed
booleans, pinned boot identities and explicit-set counts need to leave the host.

```python
# Trusted operator code supplies fixed absolute artifact names and observed boot.
manifest = capture(existing_paths, old_boot, absent_paths)
publish(private_manifest_path, manifest)
# After an externally requested reboot and independent mount/module checks:
flags = verify_after_boot(load(private_manifest_path), observed_new_boot)
```

[Real-file controls](../../../evidence/runtime-20261108/g8-trial-retention-controls/README.md)
cover positive publication/verification and adversarial bytes, inode, modes,
symlinks, hardlinks, oversized files, FIFOs, duplicate JSON, absent-path creation,
unsafe publication parents and whole-set drift. Actual cold-restored trial proof
is separate. Direct child kernels use no nested virtualization; arbitrary
child-kernel compromise shares primary trust. Complete G7/G8 remain open.
