# Workload authority policy

This policy tightens workload execution following the October 12 G7 findings.
It applies to production chroot execution, including init and exec processes.
The explicit local `test-no-chroot` mode does not qualify security behavior.
Historical G4-G6 evidence applies to its recorded sources; changes here require
new compatibility qualification before a release claim.

## Credentials and bootstrap access

The primary supplies the per-sandbox agent token in a private cpio archive
appended to the approved initramfs through a sealed memory file. The approved
image remains unchanged. The token is not passed in the kernel command line,
agent arguments, or workload environment. Kernel command lines containing
`mk.token=` are rejected by the production loader and bootstrap.

The archive creates `/.mk-agent` with mode 0700 and `token` with mode 0600,
owned by bootstrap root. For mediated roots, the bootstrap transfers the token
into a private tmpfs at `/.mk-agent` before switching roots; it does not write
the token into the mediated disk. The agent validates the private file's owner,
mode, type, link count, size and identity, then consumes and deletes it before
accepting requests. Symlinks and hardlinks fail closed. The agent disables
dumpability before reading the credential. Administrative primary access and
arbitrary child-kernel execution remain outside this protection.

The primary lab `mk-agentctl` reads `--token-file` from a caller-owned0700
no-symlink directory. The0600single-link regular file must contain exactly64
hexadecimal digits and a newline; reads are bounded and identity-checked.
The client disables dumpability before reading. The plaintext `--token-hex`
flag is rejected. Harness redaction helpers also receive file paths rather
than token values. The harness retains and removes this primary credential;
the child's separate one-shot credential is consumed during bootstrap.
Administrative primary access remains outside this protection.

## Process privileges

All production workloads run with `noNewPrivileges` enabled. Explicit requests
to disable it are rejected by the primary OCI validator and child agent.
Omitted root capabilities default to empty bounding, permitted, effective,
inheritable and ambient sets. A non-root workload receives no additional
capability contract; its bounding set may remain populated, while effective
capabilities are empty and no-new-privileges prevents exec privilege gains.

Root workloads may request only these ordinary capabilities:

`CAP_CHOWN`, `CAP_DAC_OVERRIDE`, `CAP_FOWNER`, `CAP_FSETID`, `CAP_KILL`,
`CAP_SETGID`, `CAP_SETUID`, `CAP_SETPCAP`, `CAP_NET_BIND_SERVICE`, `CAP_NET_RAW`,
`CAP_SYS_CHROOT`, `CAP_MKNOD`, `CAP_AUDIT_WRITE`, and `CAP_SETFCAP`.

Bounding, permitted, and effective sets must match. UID 0 exec would otherwise
restore grants from the bounding set, so unequal contracts are rejected rather
than silently broadening effective privileges. Inheritable and ambient sets
retain their containment checks. Unknown, duplicate, non-root explicit, or
other capability requests fail closed. Kernel administration, raw I/O, ptrace,
and filesystem-handle bypass capabilities are excluded from every set.

This changes compatibility: clients that request `noNewPrivileges: false`,
unequal root capability sets, or excluded capabilities receive rejection.
Applications needing an ordinary capability must request it intentionally.
Approved agent hashes and deployment artifacts must be regenerated for these
sources. Earlier binaries cannot consume the new private token handoff.

## Devices and remaining limits

Workload `/dev` contains null, zero, full, random, urandom, tty, PTY access,
shared-memory and message-queue mounts, and standard descriptor links. It does
not export the bootstrap's memory, I/O, kernel-log, or storage devices. An
explicit `CAP_MKNOD` grant can create nodes; restricted kernel capabilities
remain necessary to protect privileged device operations.

Child procfs remains shared: workload processes can observe child process
identities, and root signal/denial-of-service behavior requires further review.
Seccomp and child namespace/resource projection remain unsupported. The tested
`/proc/1/root` canary route must be denied under supported privilege profiles;
denying that route is not a proof of universal confinement.

These controls reduce process-level exposure. They do not supply independent
containment after child-kernel compromise or establish a security advantage
over gVisor. See the [G7 assessment](../research/g7-security-boundary-assessment.md).
