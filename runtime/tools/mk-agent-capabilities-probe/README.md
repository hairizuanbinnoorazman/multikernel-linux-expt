# Bounded retained-agent diagnostic

This Linux operator tool sends one `Capabilities` request, using the production
agent client. An explicit `--process-id` also requests one `StateProcess` after
capabilities match. It has no process execution, signal, quiesce, shutdown or recovery
operation. Build from `runtime` with `go build ./tools/mk-agent-capabilities-probe`.
The executable requires root and disables process dumpability before reading its
credential. Supply a 32-byte credential through an inherited anonymous memfd,
mode 0600, owned by the caller, sealed against writes/growth/shrink and further
seal changes. Named files, unsealed descriptors and malformed input are refused.
Never put the credential in arguments, environment, logs or published receipts.

Use a fresh protected Unix socket served by the explicitly selected native
`mkvsock-relay server-peer CID PORT SOCKET`. That mode checks the kernel-provided
remote CID before publishing the socket. Pass the PID of that owned relay as
`--relay-pid`. `agent.DialPeerContext` checks captured socket ownership and exact
local PID/UID/GID before writing; ordinary reconnect preserves this same check.
For production helper replacement, a trusted owner may explicitly select its
new owned PID with `ReconnectPeerContext`; this retains the initially captured
UID/GID and authentication sequence and pins later ordinary reconnects to that
replacement. Failed verification changes neither policy nor sequence.
The current shim also selects this native mode through its shared production
factory. This diagnostic alone does not qualify shim deployment, recovery or a
complete Docker lifecycle, and older deployed helpers remain unchanged.

Required flags are `--socket`, `--sandbox-id`, `--generation`, `--endpoint`,
`--expected-release`, `--relay-pid` and `--token-fd`. `--timeout` defaults to five
seconds and cannot exceed fifteen for the whole probe, including both calls in
process mode. The caller must also bound relay acceptance
and process lifetime, validate the recorded generation/boot/resources before
launch, and retain before/after observations. Do not reconnect or replay merely
because an observation times out. A request can advance the guest authentication
sequence even when its reply is unavailable.

Output is a fixed JSON report, never raw replies or error strings. A `matched`
result requires protocol 1, the expected amd64 kernel release and explicitly
present root UID/GID claims. It checks a response's version/sequence through the
existing protocol. Requests have MAC authentication; replies do not provide
cryptographic guest process attestation. Matching a release does not establish
kernel incarnation, Docker/storage health, lifecycle recovery or containment.

Without `--process-id`, the version-1 capabilities report and single-request
behavior are preserved. With a valid explicit process ID, version 2 reports
`observed` only after a bound process state or typed `NOT_FOUND` reply. The
reported target is a SHA-256 of the requested process ID, not its raw name and
never a hash of the credential. A valid state must match the exact requested ID,
known CREATED/RUNNING/STOPPED state, valid guest PID and exit-code range. Output
contains only that validated state/PID/exit code, excluding stdout/stderr, private
error text and untrusted reply identifiers. Capability mismatch prevents the
process query. There is no reconnect or replay.

Exit zero for `observed` means the observation succeeded, including an absent
process; it does not mean the process is running or Docker is ready. A current
absence does not establish whether a process existed earlier. The production
shim maps its initial task to guest process `init`; its host namespace-holder
PID is a different identity. Keep that distinction when diagnosing startup.

Direct child kernels require no nested virtualization. Primary and child kernels
share host trust after compromise. This diagnostic cannot close full G7/G8 gates.
