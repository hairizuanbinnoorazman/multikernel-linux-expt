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

Explicit `--stopped-output-summary --process-id=...` selects version 3. It
requires a valid STOPPED state, reads at most 64 KiB per stream with fixed zero
offsets, then checks the same process ID/status/PID/exit again. The four-call
budget is Capabilities, StateProcess, ReadProcessOutput, StateProcess, within the
same absolute timeout; there is no reconnect or replay. Positive output offsets
can acknowledge/discard buffered data in the existing API. This mode always
uses zero so it does not consume retained bytes, including after a prior reader
has advanced the retained base.

The summary includes sample byte counts, retained start/end offsets, upstream
truncation flags and a fixed allowlist of complete newline-terminated Docker
probe stdout markers. Missing fields, oversized data, offset underflow, wrong
output state or a changed final process identity refuse the summary. The first
partial line at a nonzero retained base and an unterminated last line are never
counted as markers. Stderr text, arbitrary lines, output hashes and payloads
are excluded; decoded samples are cleared before return.

Markers are guest-output clues. They do not independently attest a build,
ENOSPC, Docker health or host isolation. A missing marker may have been outside
the sampled/retained window; complete history and atomic output stability are
unproven. Only the sampled process-state identity is bracketed. The default
version-1 and optional process-only version-2 behavior remain unchanged.

For explicit private evidence preservation before a reviewed recovery,
`--private-bundle-dir=/absolute/existing/private-directory` additionally selects
version 4 and requires stopped-output-summary mode. The directory must already
be caller-owned, mode 0700, with no symlink components. After the stopped state
bracket succeeds, the probe durably creates `stopped-output.json`, mode 0600,
without replacing any existing entry. The file holds only the bounded sampled
stdout/stderr window and its validated ownership/process metadata. It includes
no caller credential, descriptor, client or environment. Workload output remains
private data; do not put this artifact in a public evidence packet. It is not a
complete history or a general diagnostic bundle.

The public report still excludes payloads, paths, output hashes and private
errors, and adds only confirmed-write and byte-count fields. Publication errors
or uncertainty are inconclusive and do not authorize recovery. Existing entries
are retained; reruns require a fresh private target. Default modes never persist
raw output. This capture does not quiesce, stop or otherwise mutate the guest.

Direct child kernels require no nested virtualization. Primary and child kernels
share host trust after compromise. This diagnostic cannot close full G7/G8 gates.
