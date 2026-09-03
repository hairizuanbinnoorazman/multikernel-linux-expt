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
For the provisional G3 implementation, accepted process fields are argv,
environment, cwd, UID/GID, supplementary groups, terminal mode, stdin,
incremental split output, terminal resize, and explicit process-group signals.
The agent advertises only the subset with focused positive coverage; UID/GID,
supplementary groups, and signals remain implemented-unproven and are not
advertised as supported until their nontrivial live cases pass. Per-request
stdin and output chunks are bounded, but aggregate output retention remains an
open G3 requirement.

Namespaces, mounts, cgroups/resources, capabilities, hooks, rlimits,
masked/read-only paths, read-only roots, `noNewPrivileges`, hostname, and
seccomp remain fail-closed. Static primary-mediated network methods are part of
the additive v1 agent protocol used by G5; they do not broaden the G3 OCI field
set. Wrong architecture always fails before the process starts.
