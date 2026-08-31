# Kernel and image policy (v1)

The initial implementation is Go on `linux/amd64`. The runtime supplies an
approved `vmlinux`, matching bootstrap initramfs, modules, and static
`mk-agent`. Containerd supplies a validated OCI bundle and unpacked root. OCI
content is never treated as a VM disk, ISO, kernel, bootloader, or initramfs.

An approved manifest records schema version, architecture, kernel release,
Multikernel and Kerf compatibility, SHA-256 hashes, required config symbols,
modules, agent protocol range, and supported OCI features. The daemon verifies
regular-file type, ownership policy, architecture, release, and every digest
before allocation. Paths must be absolute configuration values, resolved
without following a caller-controlled final symlink.

The default pin is Multikernel `v7.0-mk2`
(`3bdd35b64413da0b4e089ce931bfc2e8b031cbf7`) with Kerf `v0.2.0`
(`8b72b3e9b266f8d32e707e2c1743ad7afc50b1ec`). Alternate kernels require a
separate approved manifest; workload requests may choose a manifest name, not
an arbitrary kernel path.

Required OCI fields not listed as supported are rejected before allocation.
For G3, supported process fields are argv, environment, cwd, UID/GID,
supplementary groups, terminal=false, and explicit signals.
Namespaces, mounts, cgroups, capabilities, masked/read-only paths,
`noNewPrivileges`, terminal I/O, and seccomp remain fail-closed until their
implementation and tests land. Wrong architecture always fails before the
process starts.
