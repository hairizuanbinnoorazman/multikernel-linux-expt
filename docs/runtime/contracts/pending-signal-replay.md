# Explicit replay of a persisted pending signal

`runtime/tools/mk-agent-signal-replay` is an operator recovery primitive for an
already persisted, unacknowledged shim signal. It requires a quarantined controller
handoff: no competing controller may acknowledge or retire the agent's cached
operation result. Current record equality cannot attest that history. Separate
operator sessions also advance the generation-wide authentication sequence and
can make an older controller stale.

The root caller disables core dumps and dumpability. Credential and strict intent
arrive in distinct sealed anonymous private descriptors. Intent binds current boot,
sandbox/generation, endpoint/release, exact process ID and guest PID, original signal
and operation ID, and original protected sandbox.json bytes, object timestamps and
file/directory identities. The source must still contain that unacknowledged intent;
checks repeat before authentication and immediately before the signal. The tool
verifies the local relay PID/UID/GID and guest capabilities, including keyed signals.
One bounded context covers all calls, including queue admission.

For a RUNNING task, an exclusive synced marker `pending-signal-replay.json` in a
caller-designated private attempt directory is published before one SignalProcess
request using the existing operation ID. Reuse that case-wide directory; never
choose another directory or delete the marker to retry an unknown result. An
existing/uncertain marker refuses a fresh invocation before key read or dial.
An already STOPPED task receives no signal and creates no attempt marker.

A missing/malformed reply remains unknown. A valid remote refusal is a known
failure with private text omitted. A successful RPC means the keyed call returned
success; it does not prove new delivery. The final exact process observation may
still be RUNNING. No reconnect, retry, signal acknowledgement, exec, quiesce,
shutdown, ledger edit or host resource release is performed. Public reports omit
credentials, operation ID, protected record hashes and output payloads.

[Local controls](../../../evidence/runtime-20261105/g8-signal-replay-controls/README.md)
pass, including full runtime race and required checks. This does not establish
incident causality or authorize cleanup. Preserve private state/output and cloud
serial/resource captures before a scoped stop, then independently prove native
closure and safe ownership release using the
[recovery guide](../../guides/runtime-recovery-and-cleanup.md).
Full G7/G8 remain open. No nested virtualization is used; direct child kernels
share host trust after compromise.
