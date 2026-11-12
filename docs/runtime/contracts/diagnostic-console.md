# Disposable console diagnostic windows

`scripts/runtime_console_capture.py` observes only the exact Docker sandbox
selected by the disposable live harness. It is a bounded diagnostic source, not
an interactive console or a complete operator diagnostic bundle. The
[deployment constraint](../architecture.md#deployment-and-containment-constraint)
uses direct child kernels without nested virtualization. Console observation
does not establish containment after child-kernel compromise.

The total retained raw-data budget remains 1 MiB. Startup receives at most
768 KiB; 256 KiB is reserved for a fresh connection before the held workload is
released.
Both windows share the 600-second capture deadline after initial attachment.
Initial instance discovery remains bounded at 300 seconds; teardown attachment
and thread stop each wait at most two seconds. A saturated startup connection
is closed, and the observer waits without draining arbitrary workload output.

While the built-image hold still pins the live workload, the observer checks
the same sysfs device/inode and CID, opens a
new protected character device and performs only its first-write CID selection.
It acknowledges attachment before the harness releases USR2. Docker can delete
the Runtime v2 task and child at workload exit, before outer `docker rm`, so
waiting until that later command is unsafe. There is no guest
command/input write. Replacement refuses attachment. This binds the observation
to the held kernel name/inode/CID; it does not independently verify the kernel's
incarnation against a durable sandbox generation.

Every record declares `coverage: phase-windows` and `complete_stream: false`.
The interval between connections is unobserved. Each phase carries its byte
count, cap, hash and stop reason. `truncated` reports a phase reaching its output
limit, and `teardown_attached` reports successful second attachment. A teardown
flood can exhaust its own budget; attachment alone is not proof that shutdown
markers were captured. The integration receipt retains these coverage fields
with the console metadata hash.

The private raw file concatenates the two segments; phase lengths/hashes identify
the discontinuity. It stays mode 0600 and is excluded from evidence exports.
Filtering happens independently per segment and drops incomplete final lines,
so a cut boundary cannot fabricate a diagnostic event. The filtered file is now
a closed vocabulary of fixed ASCII event codes, with
`filtered_format: fixed-console-event-codes-v1` in the metadata. Known startup
and shutdown markers keep only their marker name; storage, transport, filesystem,
memory-pressure and I/O text matches project to fixed `MK_CONSOLE_*` codes.
No guest line, suffix, credential spelling, path, free-form reason or dynamic
number is copied. Only complete LF-terminated lines are considered; CRLF marker
lines are accepted. Whole-code suffix retention bounds the output at 64 KiB.

Codes describe matching guest-controlled text, not authenticated kernel facts,
readiness or cleanup. A guest can forge codes and influence their order/count;
this does not establish covert-channel isolation or a complete diagnostic bundle.
Historical runs using the earlier pattern redactor keep their original scope.
Raw, code and metadata files are exclusive, synced and mode 0600 independently
of umask; existing console evidence is never overwritten.

Normal removal and forced cleanup both precede observer shutdown. Attachment or
stop failure remains visible while owned cleanup is still attempted. Independent
synced native close, offline checking and resource observations remain necessary
for storage-cleanup qualification. Console markers alone do not prove release.

[Historical eight controls](../../../evidence/runtime-20261025/g8-console-window-controls/README.md)
cover floods, stop, redaction, replacement and byte limits. Live child handoff is
qualified separately. The [lifetime regression](../../../evidence/runtime-20261025/g8-console-hold-regression/README.md)
also checks late output after instance-path removal and idempotent selection;
the [failed late attempt](../../../evidence/runtime-20261025/g8-console-window-late-attachment-failure/README.md)
remains explicitly failed. Complete G7/G8 remains open.

The [fixed live handoff](../../../evidence/runtime-20261025/g8-console-window-qualified/README.md)
subsequently captures both shutdown markers exactly once in its teardown
segment. That run retains 278,945 startup bytes and 370 teardown bytes, below
both caps; actual flood bounds remain supported by the separate pipe controls.
It verifies controlled window handoff, not a continuous stream or an arbitrary
hostile-console diagnostic bundle.


The candidate native proxy now emits `MKNBD_PROXY_EVENT
stage=mk-queue-resumed endpoint=N detail=B` once, on its first positive send
after marked MK ENOSPC retries; `B` is accepted bytes, bounded by its 65536-byte
buffer. It does not mean peer consumption, NBD completion or a healthy session.
The corresponding `mk-queue-full` remains once per invocation, with errno as its
detail. Repeated stalls do not produce more full/resumed pairs. Timeout/error
handling and data forwarding remain unchanged. The
[controls](../../../evidence/runtime-20261027/g8-queue-resume-diagnostics/README.md)
use real socketpairs with injected errors and retain the prior binary as a
negative control. This candidate is not deployed in the pinned live profile.

Check helper identity before interpreting absence: older helpers emit no resume
line, and bounded console windows can miss events. Neither a missing resume nor
an observed first resume determines the cause or duration of a later stall.

The [fixed-code projection proof](../../../evidence/runtime-20261112/g8-console-fixed-codes/README.md)
reproduces ten free-text disclosures plus permissive metadata modes and regular/
symlink overwrite on prior source. Thirteen committed-source native controls and
required checks pass, including synthetic payloads, actual pipe windows and
metadata preservation. GCE independently verifies primary/retention/resource
parity and successful own-unit/cgroup cleanup. This closes the native free-text
projection and metadata boundary; actual child use, covert-channel containment
and complete G7/G8 remain open.
