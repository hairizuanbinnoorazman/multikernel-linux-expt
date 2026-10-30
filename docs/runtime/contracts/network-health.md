# Recorded endpoint backend health

`mknetd --inspect-health --socket=/run/mknetd.sock` negotiates
`network-health-v1` through the existing authenticated Unix socket. It reads a
version-1 `host-network-backend-check` report. No state files, namespace paths,
addresses, DNS, backend output or raw errors are returned. Endpoints are sorted
by generation and carry their generation, opaque descriptor SHA-256 and a fixed
result code.

The server checks at most 32 recorded endpoints under one five-second context.
The client shares one five-second budget across negotiation and inspection,
shortened by an earlier caller deadline or configured client timeout. The
production backend uses bounded context-aware commands. Busy orchestration,
mutation or store locks refuse immediately. Invalid or oversized metadata refuses
before any backend check. Missing or prior-boot identities and non-READY records
are degraded without probing potentially stale namespace paths. For eligible
records, the existing Linux backend CHECK verifies host/namespace link, route,
address, forwarding, IPv4/IPv6 isolation and firewall rule precedence observations.
The report is discarded if cancellation or a changed durable snapshot is observed.

`checked` requires a nonempty inventory with every endpoint returning
`CHECK_PASSED`. `degraded` includes `ENDPOINT_BOOT_MISMATCH`,
`ENDPOINT_NOT_READY` or `BACKEND_CHECK_FAILED`. Empty inventory is `idle`, never
`checked`. The CLI prints a validated report; degraded observations exit 1,
checked/idle exit 0. Transport, compatibility and malformed response failures
return a fixed error without a report. Legacy daemons refuse without falling
back to raw CHECK requests or local durable-state reads.

Inspection takes existing orchestration and mutation locks for its bounded
operation. It calls only backend CHECK: no ADD, DEL, reconciliation, counter
updates, repair or persistence. It can delay concurrent lifecycle operations by
its remaining check budget. It samples recorded resources; it does not find
unrecorded orphans or prove absence of ABA or external rule changes between
observations. It does not probe child traffic, guest Docker, storage, service
readiness or allocation admission. Existing doctor formats and diagnostic bundle
schemas retain their scopes until explicitly integrated and qualified.

Direct child kernels require no nested virtualization and share host trust after
child-kernel compromise. This host network inspection does not establish
containment against a compromised child kernel. Live Docker qualification and
fault injection are required before closing the associated G8 checkpoint; the
complete G7/G8 gates remain open.

[Local controls](../../../evidence/runtime-20261030/g8-network-health-controls/README.md)
and [GCE native fault/CLI proof](../../../evidence/runtime-20261030/g8-network-health-gce/README.md)
pass. The Docker pressure fixture now requires a health report at ENOSPC,
built-image and post-removal holds, with a separate five-second maximum shortened
by its original caller deadline. It requalifies the complete independent
storage/network ownership reference chain before matching endpoint generation,
descriptor and CHECK_PASSED. Released inventory requires idle and no endpoints.
All three exact artifact hashes enter the lifecycle receipt. [Local binding controls](../../../evidence/runtime-20261030/g8-docker-network-health-binding-controls/README.md)
and [actual child Docker qualification](../../../evidence/runtime-20261030/g8-live-docker-network-health/README.md)
now pass checked observations at both live holds and idle after removal, with exact
receipt hashes and independently retained public generation/descriptor joins.
It does not add a ninth report to the existing eight-report timing measurement.

The actual Docker health trial's [separate cold restoration](../../../evidence/runtime-20261030/g8-live-docker-network-health-cold-restoration/README.md)
also passes with original transport, fifteen current diagnostic artifacts,
both prior cases and 93 retained files / 12 images preserved. Complete service
readiness and G7/G8 remain open.
