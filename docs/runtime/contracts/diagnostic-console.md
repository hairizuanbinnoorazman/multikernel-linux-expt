# Disposable console diagnostic windows

`scripts/runtime_console_capture.py` observes only the exact Docker sandbox
selected by the disposable live harness. It is a bounded diagnostic source, not
an interactive console or a complete operator diagnostic bundle. The
[deployment constraint](../architecture.md#deployment-and-containment-constraint)
uses direct child kernels without nested virtualization. Console observation
does not establish containment after child-kernel compromise.

The total retained raw-data budget remains 1 MiB. Startup receives at most
768 KiB; 256 KiB is reserved for a fresh connection immediately before removal.
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
so a cut boundary cannot fabricate a diagnostic event. The filtered export is
at most 64 KiB. Existing pattern filtering/redaction does not constitute a
complete secret-safe diagnostic bundle for arbitrary hostile console text.

Normal removal and forced cleanup both precede observer shutdown. Attachment or
stop failure remains visible while owned cleanup is still attempted. Independent
synced native close, offline checking and resource observations remain necessary
for storage-cleanup qualification. Console markers alone do not prove release.

[Eight controls](../../../evidence/runtime-20261025/g8-console-window-controls/README.md)
cover floods, stop, redaction, replacement and byte limits. Live child handoff is
qualified separately; complete G7/G8 remains open.
