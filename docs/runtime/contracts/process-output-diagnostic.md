# Bounded running/stopped process-output observation

Source `255c3908` adds explicit `--process-output-summary --process-id=ID` to
`mk-agent-capabilities-probe`. Version 5 uses one deadline for Capabilities,
StateProcess, ReadProcessOutput and final StateProcess. Capabilities, exact local
relay PID/UID/GID and caller-selected sandbox/generation/endpoint must match before
the query. Credentials arrive only through the existing sealed anonymous descriptor.
No reconnect, replay, arbitrary execution, signal, quiesce or shutdown is provided.
Authentication sequence may advance even when a reply is unavailable. The guest's
anti-replay counter is generation-wide; a separate operator client can make an
older controller's sequences stale. Use this tool only for a retained/quarantined
generation or a coordinated control handoff. Unchanged file/ledger snapshots do
not establish unchanged control-session usability.

A valid RUNNING or STOPPED process can be sampled. CREATED tasks are not output-ready.
The sample status must match the first validated state; the final ID/status/PID/exit
must match it again. A transition or malformed response makes the summary inconclusive.
This brackets reported fields, not a process incarnation or an atomic output history.
Guest replies are not cryptographic guest-process attestation.

Both output offsets are zero, preserving the retained buffer acknowledgement base.
Each decoded stream is bounded to 64 KiB. Public output contains counts/offsets,
truncation flags, sampled process state and fixed complete stdout markers; no raw
stdout/stderr, errors, output hashes, command, environment or credential is projected.
Marker order is the host allowlist order, not observed chronology. Four additional
constant lines identify coarse write failure and coarse/fine sync completion:
`G8_STORAGE_PRESSURE_COARSE_WRITE_EXIT=1`,
`G8_STORAGE_PRESSURE_COARSE_SYNC_EXIT=0`,
`G8_STORAGE_PRESSURE_WRITE_EXIT=1`, `G8_STORAGE_PRESSURE_SYNC_EXIT=0`.
Different values, prefixes/suffixes, stderr, partial retained first lines and
unterminated final lines are excluded. Absence can reflect truncation or delivery;
marker membership does not attest ENOSPC, successful sync or a build.

The new mode requires an explicit process ID and excludes both
`--stopped-output-summary` and `--private-bundle-dir`. Conflicts are refused before
credential read/authentication. Existing versions 1–4 keep their request/state/private
publication contracts; their marker allowlist gains the same four constant lines.
Stopped-only mode still refuses a RUNNING task. Public process observations do not
authorize graceful stop, storage release, kernel unload or pool return.

[Local wire/privacy/race/build proof](../../../evidence/runtime-20261105/g8-process-output-controls/README.md)
retains transition/deadline/refusal and legacy-mode controls. Operator use additionally
requires current boot, exact generation/CID, protected bundle and native backing/process
identity checks, fresh owned relay, and before/after retained ownership evidence.
Full G7/G8, Docker readiness, recovery and containment remain separate. No nested
virtualization is used; direct child kernels share host trust after compromise.

The separate [retained GCE observation](../../../evidence/runtime-20261105/g8-retained-process-output-gce/README.md)
reports RUNNING PID 613 and full-pressure/fine-sync markers beyond the host's
persisted/captured offsets. Its failed ownership is unchanged, with no guest mutation.
Later retained markers do not establish original-deadline chronology or prove the
pending signal was undelivered. The failed Docker/cleanup gate remains open.
