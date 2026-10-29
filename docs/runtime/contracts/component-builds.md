# Compiled daemon build reports

`mkruntimed --inspect-components --config=/etc/mkruntime/config.json
--network-socket=/run/mknetd.sock` inspects the compiled labels reported by
mkruntimed and mknetd through their existing authenticated Unix sockets. Use a
qualified build implementing `component-build-v1`; installed older daemons are
explicitly unsupported, never assigned the inspector's revision as a fallback.

The [component schema](schemas/component-build-v1.schema.json) allows only version,
source, component, compiled build version/revision, and explicit artifact/kernel
verification flags that must remain false. Component is mkruntimed or mknetd;
version labels follow the schema pattern and are limited to 64 characters; revision is 7–40 lowercase
hexadecimal characters or explicit `unknown`. No paths, configuration, process
arguments, environment, endpoint descriptions, credentials or raw errors appear.
The RPCs read compiled labels only and take no lifecycle/network/store mutation
locks, initialize no resources and invoke no backend or external command.

The runtime method `InspectComponentBuild` accepts no selectors and only an
absent or empty-object body. The network method `INSPECT_COMPONENT_BUILD` accepts
no endpoint or generation. Clients first negotiate the capability and reject
malformed capabilities, component swaps, mixed response payloads or false
attestation claims. All errors are sanitized; unsupported peers do not trigger
raw state-file reads or version-command execution.

The operator report has its own version 1 and scope
`compiled-runtime-network-build-self-reports`. Under one caller deadline it
reads both complete component reports twice and brackets them with boot IDs.
Unsupported, unavailable, invalid or observed drifting views expose neither
component payload. Stable unknown revisions remain explicit with inconclusive
status. Stable known different revisions are both shown, with
`compiled_revisions_match: false`; matching labels are not compatibility proof.
The top-level revision identifies the inspector binary only. Exit zero means
consistent known self-reports, not readiness, artifact attestation or health.

Executable digests, daemon process incarnation, component compatibility, other
component builds, kernel identity/incarnation, service/native I/O health, Docker
health and containment remain unobserved. A restart into the same build between
samples can escape detection. Actual artifact provenance needs independent
release/digest/process evidence; these labels cannot provide it themselves.
The existing version-6 unified resource/pool report is unchanged and is not
silently upgraded by this separate inspection mode. [Local race and shared
schema/Go controls](../../../evidence/runtime-20261029/g8-component-build-controls/README.md) pass.
[Actual GCE daemon/CLI proof](../../../evidence/runtime-20261029/g8-component-build-gce/README.md)
qualifies repeated idle queries, both installed legacy refusals, executable
hashes and declared capability/NNP compatibility for the specific private
processes, clean shutdown and primary preservation. Build reporting during
actual Docker holds remains pending. The API itself still cannot attest
executable identity, process incarnation, kernel provenance or health.

Direct child kernels use no nested virtualization and share host trust after
child-kernel compromise. Build inspection introduces no containment boundary.
