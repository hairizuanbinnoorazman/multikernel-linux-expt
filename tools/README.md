# Native experiment helpers

- `mkvsock-probe.c`: bounded Multikernel AF_VSOCK integrity proof.
- `mkvsock-nbd.c`: bounded NBD-to-AF_VSOCK adapter with export identity checks, used by the
  primary-mediated ext4 experiment.

Their initial experimental behavior and transport limitations are documented in the
[mediated ext4 report](../docs/experiments/ext4-mediated/report.md).

All three helpers use [checked argument parsing](native_args.h) before opening
images/devices or sockets. Decimal CID/port inputs reject signs, whitespace,
suffixes, overflow and the reserved wildcard value; ports must be nonzero.
NBD sizes must be positive 4096-byte multiples representable by the device ioctl;
image/generation identifiers use a bounded alphanumeric/dot/underscore/hyphen
syntax. The client placeholder must be the literal `reserved`. Standalone
helpers still require a trusted caller selecting the file/device and endpoint.
Run `python3 scripts/test-native-helper-arguments.py` for boundary checks and
malformed CLI requests with an unchanged synthetic host canary. Keep `native_args.h`
beside the C sources when building outside the repository.

`mkvsock-nbd` also supplies the runtime's primary-mediated storage backend.
It requires an exact canonical export hello, including zero status/reserved
fields and zero-padded identity strings. Request validation precedes payload
receipt and disk I/O: only READ, WRITE, FLUSH and DISC are accepted, only WRITE
may carry FUA, and flush/disconnect length and offset must be zero. Read/write
ranges use subtraction after validating the offset to prevent overflow and
remain bounded to 4 MiB. Zero-length read/write requests retain their prior
no-op behavior. Unsupported commands or flags terminate the session before
mutation rather than silently ignoring fields or guessing payload framing.
Each initial hello and its reply have independent absolute 15-second deadlines.
A rejected, truncated or stalled hello closes that connection and returns to
accepting; it cannot terminate the export server before the intended client
connects. Continuous invalid connection pressure still needs separate review.
Rejection replies contain a canonical error envelope with zero size and empty
identity fields, so a rejected peer cannot learn the expected export identity
and retry using values disclosed by the server.
The hello checks image identity, size and export generation. It has no MAC or
peer-CID authorization; its scope is the existing trusted-node transport boundary.
The transmission field rules follow the
[NBD protocol](https://github.com/NetworkBlockDevice/nbd/blob/master/doc/proto.md).

Run `bash scripts/test-mkvsock-nbd-timeouts.sh` for canonical hello, command/flag,
range/overflow regressions and socket-timeout checks. Restricted environments
may explicitly skip the socket-option check on EPERM; parser checks still run.
These portable checks do not qualify live Multikernel transport or the full
G7 native fuzzing campaign.

After accepting the export hello, the native storage server permits indefinite idle time
before the first request byte. Once that byte arrives, an absolute monotonic
15-second deadline covers the remaining header and WRITE payload. Response
header and READ data share a fresh 15-second delivery deadline. Short transfers,
interruptions, and trickled bytes never renew either allowance; nonblocking
socket I/O keeps readiness races within the same bound. Disk syscall duration
is outside these transport deadlines. The native regression includes idle,
trickle, partial-payload and unread-response cases. A restricted `send` syscall
produces an explicit EPERM skip; these cases must pass outside that sandbox.

The native storage server keeps only its validated imagefd3, established listener
and diagnosticfds1/2 before readiness. It closes other inherited descriptors,
drops capabilities and installs the [request syscall filter](native_server_sandbox.h).
Keep this header and native_privileges.h beside the C source when building.
Linux>=5.9 and seccomp filter support are required; startup fails if either
restriction cannot be installed. Client/device setup modes retain their own
behavior. The filter denies new files/connections/processes and host control,
while admitting existing image/stream/diagnostic operations and libc metadata.
See the [runtime privilege contract](../docs/runtime/contracts/primary-runtime-privileges.md)
for exact grants and limitations. Run
`bash scripts/test-native-server-sandbox.sh` for descriptor I/O and host-operation
refusal checks; this fixture requires working loopback socket permissions.

Server setup also applies64MiB address-space/16-descriptor hard ceilings,
retains stricter inherited limits, disables dumpability and sets core limit0.
Refused hellos produce at most16 detail lines, a suppression notice and a
terminal aggregate summary; recovery's ready/accepted/final-sync records remain.
Run `bash scripts/test-native-server-pressure.sh` for256 refused hellos followed
by intended-client read/write/FUA/flush/disconnect in the real server loop with
loopback transport substitution, plus enforced allocation/fd/core ceilings.
This is bounded normal-peer logging, not CPU fairness or a compromised-helper
log quota; production VSOCK compatibility is qualified separately.

`native_stream_proxy.h` forwards the guest NBD client's TCP/VSOCK streams with
two bounded 64 KiB FIFOs, nonblocking partial writes and independent half-close
draining. It avoids a blocking write in one direction starving the reply needed
by the other endpoint. `scripts/test-native-stream-proxy.py` retains the prior
blocking-loop deadlock control and verifies full-duplex backpressure, complete
half-close replies and broken-peer refusal. Export identity, limits and request
deadlines remain the same. Live Docker concurrency remains a separate gate.

The NBD client explicitly marks only its already-selected Multikernel VSOCK
endpoint for ring-full handling. Its IPI enqueue path returns `ENOSPC` before
claiming a slot when the ring is full. The proxy retains those unsent bytes,
backs off that write direction from 1 to 16 ms, and continues servicing the other
direction. Successful write progress resets the backoff.
An uninterrupted queue-full episode has a 15-second deadline; normal NBD and
absolute server frame deadlines are unchanged. Only the first ring-full event is
logged per proxy, preventing retry-driven diagnostic growth. Other endpoints and
other errors retain fatal-error behavior. `test-native-stream-backpressure.py`
checks injected transient exhaustion with exact duplex bytes/half-close replies,
permanent exhaustion with bounded retry count/timeout, and unmarked ENOSPC refusal.
This is a protocol retry correction, not a live pressure qualification or a change
to kernel queue sizing. The receive-readiness kernel patch is qualified separately.

`mkvsock-relay server-peer CID PORT UNIX` adds explicit host-side peer selection.
It checks accepted VSOCK address length/family and the expected nonzero CID before
publishing the Unix bridge or forwarding bytes. Wrong peers are closed; 64
refusals terminate the helper without publishing a bridge, and EINTR does not
consume that budget. Callers must still bound the helper's lifetime. The common
entrypoint protects dumpability before endpoint setup. CID matching does not
attest a guest process or replace the agent request authentication contract.

The existing `server PORT UNIX` and `client CID PORT UNIX` modes retain their
interfaces. The current shim's shared relay factory uses `server-peer` with the
sandbox's declared child CID for startup, recovery and replacement. It never
falls back to `server` for an older helper. Deploy the shim with a pinned helper
that supports this interface; older deployed binaries do not gain enforcement.
Live Docker lifecycle qualification of this source change remains separate from
the earlier capabilities-only diagnostic. These tools support direct
child kernels without nested virtualization and retain shared host trust after
child-kernel compromise.
