# Correlated operational health observations

Run `mkruntimed --config=/etc/mkruntime/config.json --doctor-all --doctor-format=8
--network-socket=/run/mknetd.sock` as one command. Version 8 explicitly adds the
negotiated lifecycle journal status and recorded host-network CHECK report to the
version-7 resource/pool/component observations. Default version 5, versions 6/7
and the version-1 diagnostic bundle retain their existing contracts.

All callbacks share the original configured backend timeout. One complete v7
view precedes two journal/network samples, followed by a second complete v7 view.
No child output, raw logs, credentials, local-state fallback, repair or new
filesystem-writability operation is used. Existing protected sockets and server
peer authorization apply. Network CHECK may invoke its existing bounded read-only
host backend; this is not an end-to-end network probe.

Both operational views and both health samples must match. Health reports must
validate, have the same boot as operational ownership and cover exactly the
recorded endpoint generations and descriptor digests, including empty inventories.
The journal sequence must equal the operational lifecycle sequence. Missing,
unsupported, invalid, unavailable, cross-bound or drifting observations return
`inconclusive` with a fixed finding and no nested payloads. Raw callback/RPC
errors are excluded. Cancellation stops further observations and retains no
partial payload; callers do not receive a new timeout for later sources.

A correlated journal failure or degraded recorded network CHECK makes the report
`degraded`. Pending journal completions make it `inconclusive`; degraded severity
takes precedence. Existing operational findings/severity remain. Every aggregate
result other than `consistent` suppresses CPU candidates and the memory candidate
upper bound in its nested pool view. Nested component status describes its own
scope; only the top-level status describes the entire v8 observation.

The CLI emits one JSON report. Exit 0 requires `consistent`; all other report
statuses exit 1. Invalid format/mode arguments exit 2 before configuration load.
All admission guarantees remain false. A consistent observation is not mutation
readiness, current journal writability, journal-byte attestation, process/kernel
incarnation, end-to-end health, Docker readiness or containment. Repeated equality
cannot exclude intervening ABA changes or same-sequence service restarts. Disk
sync/probe behavior is not inferred from a responsive metadata socket.

This advances G8 health correlation; it does not close full G7/G8 or qualify a
new live child-Docker lifecycle. Direct child kernels require no nested
virtualization and share host trust after child-kernel compromise.

[Local race/required, Unix-wire and four guard mutations](../../../evidence/runtime-20261106/g8-operational-health-controls/README.md)
and [GCE wire/CLI refusal with independent cleanup audit](../../../evidence/runtime-20261106/g8-operational-health-gce/README.md)
pass. Positive CLI execution against production daemons and new live Docker
qualification remain separate requirements; these controls do not close full G8.
