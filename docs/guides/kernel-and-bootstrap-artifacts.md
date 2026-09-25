# Kernel and bootstrap artifacts

The runtime boots only administrator-approved artifact sets. A workload may
select a manifest name; it may not supply arbitrary kernel, initramfs, module,
agent, or relay paths.

This guide explains how to assemble and validate one artifact set. The
normative rules are in the
[kernel and image policy](../runtime/contracts/kernel-and-image-policy.md) and
[`kernel-manifest-v1.schema.json`](../runtime/contracts/schemas/kernel-manifest-v1.schema.json).

## Artifact set

Each manifest names and hashes:

- an uncompressed x86-64 ELF `vmlinux`;
- the matching bootstrap initramfs;
- the static `mk-agent` executable;
- the static VSOCK relay executable;
- the `mk_transport` module built for the child kernel release;
- any additional required modules;
- the exact Multikernel and Kerf compatibility pins;
- the supported agent protocol range; and
- the OCI features this artifact set is approved to accept.

The current compatibility pins are Multikernel `v7.0-mk2` at
`3bdd35b64413da0b4e089ce931bfc2e8b031cbf7` and Kerf `v0.2.0` at
`8b72b3e9b266f8d32e707e2c1743ad7afc50b1ec`.

## 1. Build from pinned inputs

Build the child kernel and every kernel module from the same pinned kernel
tree and configuration. Build `mk-agent` from the same clean runtime revision
used for the installed host components. The relevant repository entry points
are:

- `make runtime-build` for `mk-agent` and the other Go components;
- `scripts/build-runtime-vsock.sh` for the pinned `mk_transport` module;
- the static relay build used by `scripts/test-runtime-g3.sh`; and
- `scripts/build-agent-initramfs.sh`,
  `scripts/build-mediated-root-initramfs.sh`, or
  `scripts/build-runtime-container-initramfs.sh` for the applicable boot path.

Read each builder's required arguments before running it. The exact initramfs
builder depends on the experiment or runtime storage path. Do not mix
artifacts from different child kernel releases.

## 2. Stage under administrator-owned paths

Place artifacts under an absolute, root-owned hierarchy such as
`/opt/mkruntime/artifacts/RELEASE/`. Every path component must be a real
directory, not a symlink, and must not be group- or world-writable. Artifact
files must be regular, root-owned, and not group- or world-writable.

Install the manifest under the strict directory selected by
`kernel_manifest_directory` in `/etc/mkruntime/config.json`, normally
`/etc/mkruntime/kernels`.

## 3. Verify identity before writing the manifest

Record the child kernel release and module identity:

```bash
file /absolute/path/to/vmlinux
modinfo -F name /absolute/path/to/mk_transport.ko
modinfo -F vermagic /absolute/path/to/mk_transport.ko
sha256sum \
  /absolute/path/to/vmlinux \
  /absolute/path/to/initramfs \
  /absolute/path/to/mk-agent \
  /absolute/path/to/mkvsock-relay \
  /absolute/path/to/mk_transport.ko
```

`vmlinux`, `mk-agent`, and the relay must be x86-64 ELF files. The module name
must be `mk_transport`, and its vermagic must begin with the manifest's exact
`kernel_release`.

## 4. Create the strict manifest

Start from
[`kernel-manifest.valid.json`](../runtime/contracts/fixtures/kernel-manifest.valid.json)
for structure only. Replace every placeholder path and digest, and set a safe
manifest name matching `^[a-z][a-z0-9.-]{0,62}$`.

Do not copy the fixture's repeated example digests. Do not advertise an OCI
feature simply because code exists for it: `oci_features` is an approval and
must reflect the features validated for this artifact set. Unknown manifest
fields are rejected.

Required transport values are fixed:

| Field | Required value |
| --- | --- |
| `module_name` | `mk_transport` |
| `socket_option` | `9` |
| `transport_id` | `1` |
| `primary_role` | `server` |
| `child_role` | `client` |

The protocol interval must include version 1. Required kernel configuration
entries use forms such as `CONFIG_MULTIKERNEL=y`.

## 5. Validate the installed artifact set

Run the bootstrap validator as root against the final installed paths:

```bash
sudo ./scripts/validate-runtime-bootstrap.py \
  /etc/mkruntime/kernels/gce-mk2.json gce-mk2
```

The validator checks strict fields, compatibility pins, protocol coverage,
absolute safe paths, ownership and modes, regular-file type, SHA-256 digests,
x86-64 ELF identity, and transport-module name/vermagic.

`--skip-modinfo` and `--skip-parent-safety` exist for isolated tests. They must
not be used to qualify a production or evidence artifact set.

## 6. Connect the manifest to host configuration

Set `kernel_manifest_directory` in the strict host configuration to the
manifest directory. Sandbox requests select the manifest by its `name`, never
by a filesystem path. Restart `mkruntimed` only after validation succeeds and
when doing so will not disrupt unrelated tasks.

For a second kernel, create a completely separate artifact directory and
manifest. Reusing an initramfs or transport module across releases is allowed
only if its recorded digest, kernel compatibility, and validation genuinely
match; never infer compatibility from a similar filename.

## Update checklist

- Are all source revisions pinned and recorded?
- Do the kernel and modules have the exact same release?
- Was `mk-agent` built from the installed runtime revision?
- Are paths absolute, root-owned, non-symlinked, and non-writable by others?
- Do all hashes match the final installed bytes?
- Does `oci_features` claim only validated behavior?
- Does the bootstrap validator pass without skip options?
- Was the validated manifest retained with the run evidence?
