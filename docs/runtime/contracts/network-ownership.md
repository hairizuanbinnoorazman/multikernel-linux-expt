# Recorded network ownership

`mknetd --inspect-ownership --socket=/run/mknetd.sock` reads a bounded metadata
projection through the existing protected Unix RPC socket. It does not start a
server, open the inspector's `--state-dir`, invoke a backend or reconcile state.
The inspector requires only an absolute socket path, not an egress interface.
Existing socket modes and server peer-UID authorization are unchanged.

The client first sends `CAPABILITIES`, requiring `network-ownership-v1`, then
`INSPECT_OWNERSHIP`. Both use the same caller deadline (ten seconds by default).
Cancellation closes a pending read. Older daemons return unsupported; malformed
or unadvertised views are refused, with no raw-state/endpoint fallback. Inspection
requests accept neither an endpoint nor a generation argument.

Report version 1 uses source `durable-network`. Its top-level `primary_boot_id`
is the service's startup boot identity. `endpoints` is a non-null array sorted by
unique endpoint generation, limited to 1024 entries. Each entry carries:

- Endpoint generation, optional bound sandbox ID/generation and optional recorded
  endpoint boot identity, which may be older or missing in legacy records.
- Recorded owner (`runtime` or `cni`), managed-namespace flag and state
  (`ALLOCATING`, `READY`, `DELETING`, `DEGRADED`, `DISCONNECTED`).
- A descriptor fingerprint and recorded RX/TX packet/drop/error counters.

Raw container/network/interface names, namespace paths, addresses and DNS are
not serialized. The SHA256 fingerprint uses domain
`mkruntime-network-descriptor-v1` followed by NUL. It hashes ContainerID,
NetworkName, IfName, NetNS, Address and Gateway as raw byte strings with unsigned
64-bit big-endian length prefixes; then MTU as unsigned 64-bit big-endian; then
DNS Domain as a length-prefixed string; then DNS Nameservers, Search and Options
in that order, each with a 64-bit list count and length-prefixed strings. Each
string is capped at 4096 bytes and aggregate string bytes at 65536. Raw invalid
UTF-8 is not normalized; field and list boundaries are unambiguous. This is an
opaque comparison fingerprint, not repair/release authority.

The projection uses a try-lock on the store, independent of service mutation
locks. Busy persistence returns retryable `UNAVAILABLE`; too many records return
`RESOURCE_EXHAUSTED`; invalid metadata returns sanitized `OPERATOR_ACTION`.
No partial inventory is substituted. Projection neither rereads mutable state
files nor attests current links, rules, namespace/process incarnations, sockets,
reachability or packet isolation. `READY` is a recorded claim, not a health pass.
Separate reads are not an atomic cross-daemon snapshot and can observe drift.

[GCE proof](../../../evidence/runtime-20261027/g8-network-ownership/README.md)
covers the real private daemon/CLI's idle report, installed older-daemon refusal,
and unchanged primary files/PIDs. A separate explicit test projects an original
failed ledger without invoking reconciliation; its DISCONNECTED prior-boot
endpoint remains visible and unchanged. This fixture does not change production
startup reconciliation, which retains its existing prior-boot retirement policy.
Populated live endpoint/doctor correlation and full G8 acceptance remain open.

The [deployment constraint](../architecture.md#deployment-and-containment-constraint)
uses direct child kernels without nested virtualization. These observations do
not establish containment after child-kernel compromise; host trust remains
shared and full G7/G8 remains open.

## Explicit lifecycle intent and correlated doctor

New shim allocations reserve label `io.multikernel.network-expectation-v1` with
value `required` in immutable SandboxConfig labels. Direct lifecycle clients may
explicitly declare `none`; omission remains `unknown` for legacy compatibility.
Other values are rejected by creation validation. This declaration is intent,
not a permission change or a network allocation. Diagnostic projection copies
only the parsed enum, never the labels map or arbitrary values.

The runtime advertises `network-expectations-v1` and accepts
`InspectNetworkExpectations` with an absent or empty-object body. Its version-1
`durable-lifecycle-network` report contains the lifecycle projection and one
aligned sandbox/generation/expectation tuple per record, including historical
ABSENT owners. The store try-lock and existing lifecycle record/CPU limits apply;
busy, oversized and invalid inventories refuse complete inspection rather than
substituting partial metadata. The label is part of the existing immutable
configuration/idempotency binding; no existing RPC shape is extended.

`mkruntimed --doctor-network --network-socket=/run/mknetd.sock` selects the separate
version-4 `durable-lifecycle-network-ownership-and-kernel-presence` report. One
caller budget covers negotiated expectations and endpoint reads. Two complete
pairs bracket kernel-name and boot observations. Any sampled intent, endpoint,
counter, lifecycle or kernel drift is inconclusive; no atomicity or unobserved
ABA protection is claimed. The existing `--doctor` stays version 3 and retains
its prior network-unobserved scope.

The new mode identifies stable recorded mismatches with generation-bound hints:
missing required RUNNING endpoints, orphan candidates, sandbox generation
mismatch, endpoints after owner release, unexpected or multiple bindings,
startup/endpoint boot mismatch and recorded DISCONNECTED/DEGRADED state. Legacy
unknown intent, incomplete creation/deletion and missing endpoint boot identity
require rechecking. A required endpoint absent before provision is inconclusive;
a missing declaration is never interpreted as required or none. Unbound CNI
endpoints outside runtime ownership are not guessed to be orphans; unbound
runtime endpoints require rechecking.

`consistent` still means sampled claims and kernel names agree. Expected endpoint
descriptor/incarnation, native namespace/rule/session health, storage exports,
pool/readiness, Docker health and containment remain explicitly unobserved.
Findings confer no cleanup authority. In particular, a matching descriptor
fingerprint is not a lifecycle-declared descriptor expectation.

[Correlated GCE proof](../../../evidence/runtime-20261027/g8-network-doctor/README.md)
qualifies actual private idle CLI/daemons, installed legacy refusal, ordinary
doctor compatibility and populated injected metadata controls. It does not
qualify active child endpoints or a new Docker build. Full G7/G8 remains open
under the same direct-child, no-nested-virtualization/shared-trust constraint.

## Exact sampled CHECK values

[Endpoint state-value qualification](../../../evidence/runtime-20261027/g8-endpoint-state-values/README.md)
proves CHECK rejects MTU, gateway, device and address substring lookalikes.
Administrative UP is read from the link flags rather than LOWER_UP or state text;
IPv4 forwarding must equal 1. Links and routes require one unambiguous output
line. Real disposable GCE namespace controls cover both MTUs and both gateways,
with successful restoration after each rejection. This remains a sampled CHECK;
it does not establish endpoint incarnation or active child Docker health.
