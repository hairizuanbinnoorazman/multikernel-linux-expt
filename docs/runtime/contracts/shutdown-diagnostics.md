# Bounded Docker shutdown evidence

Source `4adc7f6f` protects Docker stop and mediated storage shutdown from blocked
or closed diagnostic stdout/stderr. The normal agent uses a process-wide reporter
for its six existing marker stages. It accepts a typed stage and optional errno;
no error text, path, environment, command output or workload identifier enters it.
Only errno 1..4095 is formatted. Existing healthy marker bytes are preserved.

| Stage | Stream | Fixed marker |
| --- | --- | --- |
| Docker stop failure | stderr | `MK_DOCKER_STOP_FAIL` with optional errno |
| Docker storage-quiesce failure | stderr | `MK_DOCKER_STORAGE_QUIESCE_FAIL` |
| Root remount failure | stdout | `MK_STORAGE_ROOT_QUIESCE_FAIL stage=remount-ro` with optional errno |
| Root quiesce success | stdout | `MK_STORAGE_ROOT_QUIESCE_PASS stages=sync,remount-ro,sync device=/dev/nbd0` |
| NBD disconnect failure | stdout | `MK_STORAGE_NBD_DISCONNECT_FAIL device=/dev/nbd0` |
| NBD disconnect success | stdout | `MK_STORAGE_NBD_DISCONNECT_PASS device=/dev/nbd0` |

Each stage consumes one attempt for the process lifetime, including busy, failed,
short or unavailable delivery. Each stream has at most one active writer, one
record of at most 128 bytes and one private descriptor. There is no queue, retry
or per-call worker behind a busy stream. A claimed free stream waits at most a
50 ms timer budget; ordinary encoding/scheduling are not real-time guarantees.
At most six records can be attempted across both streams in a process lifetime.

The worker duplicates descriptor 1 or 2 to a private CLOEXEC descriptor at least
3, preserving inherited flags and process signal policy. Closed readers yield
EPIPE rather than Go's special SIGPIPE termination. A timed-out write can remain
blocked until the reader resumes or the process exits, but later calls cannot
add writers to that stream. Other distinct stages can still use a free stream.
A busy stage is dropped permanently, even after the stream becomes free.

The production root-quiesce/disconnect calls continue to supply process stdout;
the helper selects bounded delivery for that writer. Default Docker diagnostics
also use this reporter. Explicit other io.Writer collectors retain synchronous
in-process test/capture behavior and must be trusted. They are not a production
RPC-selected output destination.

Stop failure still prevents root quiescence. Remount failure still prevents the
second sync. Disconnect failure still prevents poweroff. Original errors and
Docker stop timeout/ordering are preserved. These syscall/engine operations have
their own timing and failure contracts; this only bounds diagnostic delivery.
The separate [terminal reporter](daemon-terminal-diagnostics.md) can retain its
own one blocked writer, so its limit is additional to these two streams.

Markers are best-effort observations, not protocol acknowledgements, full event
history, proof of current state or resource-release authority. Delivery failure
never changes a shutdown success into a success receipt or permits resource
release. Existing gates requiring marker bytes remain incomplete when they are
missing. Native synced-close, ownership and independent offline checks remain
required under the [recovery guide](../../guides/runtime-recovery-and-cleanup.md).

[Local controls](../../../evidence/runtime-20261105/g8-shutdown-diagnostics-controls/README.md)
retain seven exact-parent marker passes and fourteen full/closed-reader failures,
then 21 green descriptor cases, repeat/quota/CLOEXEC/failure-order checks and full
runtime race/required checks. Actual source-qualified Docker execution, live
shutdown, aggregate retention/pressure and full G7/G8 remain separate. No nested
virtualization is used; direct child kernels share host trust after compromise.
