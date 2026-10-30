# Lifecycle journal status

`mkruntimed --inspect-journal --config=/etc/mkruntime/config.json` negotiates
`journal-status-v1` over the existing authenticated runtime socket. It returns a
version-1 `in-process-lifecycle-journal` report with only status, last acknowledged
sequence, pending-completion count and the store's observed failure flag. Every
field is required on the wire; omitted/null flags and counters cannot become an
idle report through zero-value decoding. Unknown fields, duplicate keys and
contradictory status/count/failure combinations refuse inspection.

`idle` means no observed journal failure and no outstanding acknowledged intents.
`pending` means acknowledged intents still require completions. `failed` means
the store has entered its fail-closed state, including explicit close or a
previously detected append/sync/path-identity failure. Failed takes precedence
over pending. The sequence is the last acknowledged sequence; a failed append
may have left additional unacknowledged bytes. A pending intent can be normal
in-flight work or retained recovery work: the report cannot distinguish them.

The producer takes a nonblocking store snapshot and performs no filesystem I/O,
journal replay, configuration cloning, backend call, reconciliation or repair.
Busy/canceled snapshots fail with a fixed unavailable error. Invalid accounting
fails with an operator-action error. The client shares one five-second maximum
across capability negotiation and inspection, shortened by the caller's deadline.
Unsupported servers refuse without reading local state or falling back to a raw
journal stream. Raw operation identities, errors, configuration, paths, command
lines, labels, credentials and journal payloads are excluded.

The CLI prints a validated report. Idle exits 0; pending/failed exit 1. RPC,
compatibility or malformed-response failure returns a fixed error without a
report. Existing metadata RPCs can remain responsive after a journal failure;
operators must not infer mutation readiness from a responding socket or a
consistent ownership report alone.

This is an in-process observation, not a filesystem writability test, space
reservation, recovery authorization or full readiness check. A replaced journal
path can remain undetected until an actual mutation validates it; inspection does
not touch that path. An idle result does not attest current storage, journal bytes
or future durability. Full G8 still requires service readiness, native health,
recovery, log pressure and the other plan requirements. Direct child kernels
require no nested virtualization and share host trust after child-kernel compromise.

The Docker pressure fixture additionally requires an idle journal report at
ENOSPC, built-image and post-removal holds. It requalifies the independent
storage/native ownership report, then requires exact sequence equality, zero
pending completions and no observed failure. Each observation has a separate
five-second maximum shortened by the original caller deadline; it does not
extend the existing eight-report diagnostic timing scope. All three artifact
hashes enter the lifecycle receipt. Matching sequence numbers do not exclude
same-sequence restarts or other ABA; existing incarnation limitations remain.
This integration requires separate actual child Docker qualification.
