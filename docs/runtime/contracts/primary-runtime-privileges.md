# Primary runtime privilege contract

The managed mkruntimed service runs as root with NoNewPrivileges=yes, empty
ambient capabilities and a bound of eight capabilities (mask0x868001b).
Builders, Kerf and newly spawned storage helpers inherit this bound. Existing
helpers can outlive a daemon restart; migration/recovery must verify their
process identity and record their actual privileges separately.

The native storage server now drops effective/permitted/inheritable/ambient
capabilities after image/lock/listener setup and before readiness or request
acceptance. It sets and verifies NoNewPrivileges first. The inherited bounding
set may remain populated; a root exec fixture confirms it cannot restore
capabilities after the permitted set is cleared. UID0, root-owned filesystem
access and root-authorized IPC remain risks in other components. The native
server request stage additionally keeps only imagefd3, listener and diagnostic
fds1/2, closes inherited executable/stdin/other descriptors using close_range,
and installs an architecture-checked seccomp allowlist before readiness.
Setup/filter failure aborts. Linux>=5.9 with seccomp filter support is required;
x86-64 and little-endian aarch64 syscall tables are admitted at compilation,
with live qualification recorded per architecture.

The request filter denies new path opens/mutations, socket/connect creation,
exec/process creation, signals/ptrace, mounts, ioctl, capability/credential
changes and executable/file-backed mappings. Image positional I/O/sync is
restricted to fd3; accept to the established listener; socket options to
SOL_SOCKET receive/send timeouts. Existing descriptor reads/writes, stream
send/receive/shutdown, poll/clock, signal return/mask, anonymous non-executable
allocation and exit remain available. Libc metadata lookup remains admitted
for stdio. Existing diagnostic-file writes and the image/connected peer remain
authority. This does not constrain protocol information flow, kernel bugs,
existing-descriptor abuse or retained daemon authority. The
[seccomp kernel documentation](https://docs.kernel.org/userspace-api/seccomp_filter.html)
describes syscall filtering and its limits. This is not whole-system containment.
Device
client/setup modes retain their separate required privilege behavior.

| Capability | Supported operation |
| --- | --- |
| CAP_CHOWN | Preserve admitted OCI uid/gid metadata during private root staging |
| CAP_DAC_OVERRIDE | Read admitted snapshots and stage/copy trees whose preserved modes/owners deny ordinary root-owner access |
| CAP_FOWNER | Preserve timestamps/modes and inspect foreign-owned input metadata with O_NOATIME |
| CAP_FSETID | Preserve admitted file mode semantics across ownership/content changes |
| CAP_MKNOD | Preserve supported device nodes during archive-semantic private copies |
| CAP_SYS_ADMIN | Create/seal recursive snapshot views, overlay/bind mounts and unmount owned views |
| CAP_SYS_BOOT | Kerf's kexec_file_load/unload kernel-image operations |
| CAP_SYS_PTRACE | Inspect exact storage-helper proc descriptors/executable identity, including helpers predating the reduced bound |

The installed pinned Kerf code uses kexec_file_load; its runtime load path does
not use /dev/mem, mlock or explicit rlimit adjustment. The DAX/console tooling
is a separate interface. RAWIO, IPC_LOCK, SYS_RESOURCE, network administration,
identity switching, module loading and unrelated capabilities are excluded from
the long-lived runtime profile. Snapshot xattrs are rejected except realized
overlay opacity; this profile does not add SETFCAP for unsupported file xattrs.

Module setup now belongs to multikernel-modules.service, a root oneshot bounded
only to CAP_SYS_MODULE (mask0x10000), with NoNewPrivileges and empty ambient
capabilities. It uses modprobe for vsock and mk_transport, selecting modules
from the running kernel's installed module tree, then verifies transport module
presence. Install the qualified transport into each supported host kernel's
module tree and run depmod before booting it. This preserves original-kernel
fallback without reusing a candidate's incompatible module. The daemon requires
this service and orders after it, then keeps its own presence/storage-mount
checks. Module setup errors fail the service; an already-loaded module is
handled by modprobe. No privileged '+' command bypass is used.

[Linux capabilities](https://www.man7.org/linux/man-pages/man7/capabilities.7.html)
define these operations; [systemd execution settings](https://github.com/systemd/systemd/blob/main/man/systemd.exec.xml)
enforce the process bound. This is an operation-derived profile to qualify,
not a universal minimum or compromised-runtime containment proof. SYS_ADMIN,
SYS_BOOT, SYS_PTRACE and root-owned filesystem writes remain broad authority.
Further helper separation, caller/kernel trust, dependency integrity, privilege
inventory and independent review remain required for full G7/G8 acceptance.

The native server lowers soft/hard address-space limits to64MiB and descriptor
limits to16 before setup, retaining any stricter inherited ceiling. It lowers
RLIMIT_CORE to0 and sets/verifies non-dumpable after capability removal and
before filtering. Setup failure aborts; the request filter denies later limit
or dumpability changes. [Linux resource-limit semantics](https://man7.org/linux/man-pages/man2/getrlimit.2.html)
define the address/fd ceilings; RLIMIT_AS bounds virtual memory, not aggregate
host/kernel memory. File-size/CPU lifetime limits are not used for the long-lived
export. Peer-induced refusal diagnostics retain sixteen detail lines, one
suppression notice and a saturating aggregate summary before the canonical
terminal sync/counter record. Ready/accepted/closed records remain available.
Repeated refused peers cannot grow detail logs without bound, but CPU fairness,
connection starvation, host log I/O latency and whole-host pressure require
separate controls and qualification. A compromised server can still write its
existing diagnostic descriptors; the normal-peer logging policy is not a
compromised-process write quota.

The privilege contract includes code imported by the selected interpreter.
The held Kerf entrypoint/interpreter checks do not by themselves protect an
editable source checkout. Production Kerf source and its selected virtualenv
must be root-owned and exclude group/other writes. The root installer protects
its tree before make/pip and checks the result; the
[dependency/imported-code guide](../../guides/runtime-dependency-review.md)
provides read-only checks for both separately selected trees. Primary root
remains trusted to maintain code, and this does not attest every system Python
import or close full dependency/source-provenance review.
