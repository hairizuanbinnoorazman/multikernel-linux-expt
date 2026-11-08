# Read-only child engine inspection through the owning shim

The owning shim keeps the child agent's authenticated control connection.
`multikernel.inspection.v1.Inspection/InspectDockerEngine` exposes its
[engine observation](docker-engine-observation.md) on the existing containerd
shim TTRPC socket. A standard shim plugin registers the extension; the existing
same-effective-UID/GID peer handshake protects it. No listener or token is added.

The protobuf request and response use `google.protobuf.BytesValue` containing
strict bounded JSON. The wire definition is `runtime/inspection/inspection.proto`.
The request has exactly `version=1`, `namespace`, `container_id`, `sandbox_id`
and `generation`. Unknown/duplicate/case-variant fields, null required fields,
unsafe identities and requests over 1024 bytes refuse. A report has exactly
`version=1`, `owner` (the complete matched request), and `observation` (the strict
version-1 child API/launcher result), with a 2048-byte bound.

One five-second context includes waiting for the shim state lock and the existing
agent queue, capability negotiation and observation. The shim matches all four
owner fields, requires its init process RUNNING, and holds the state lock through
the observation so deletion cannot transfer ownership. It invokes only
Capabilities and InspectDockerEngine through its existing client. It does not
reconnect, replace a relay, replay, execute a workload, signal or repair anything.
Unsupported agents return Unimplemented. Other failures retain fixed codes and
messages without diagnostic output, details, paths or credentials.

`mk-shimctl` captures the existing protected caller-owned socket inode, verifies
it while dialing and checks the connected Unix peer UID before sending the query.
Production shims run as root. It requires the expected owner tuple and validates
that the response exactly matches it. It has one five-second dial/RPC budget,
no connection retry or fallback, and no credential input:

```sh
mk-shimctl --socket="$SHIM_SOCKET" --namespace=moby \
  --container-id="$CONTAINER_ID" --sandbox-id="$SANDBOX_ID" \
  --generation="$SANDBOX_GENERATION"
```

It emits one JSON report and exits zero only for responding; other observed
states exit one with their report. Failed/unsupported/foreign observations emit
no success JSON and a fixed stderr message. Invalid arguments exit two.
The operator must obtain the socket and owner from trusted current runtime
state; no old PID or guessed socket grants ownership.

This is a sampled engine/launcher observation, not workload admission, reserved
storage, build progress, child-kernel integrity or hostile tenant containment.
Doctor/bundle formats and ordinary Docker workload paths stay unchanged.
Direct child kernels use no nested virtualization and share primary trust after
arbitrary child-kernel compromise. Actual host Docker/child execution and
complete G7/G8 require separate qualification.

The optional disposable pressure qualifier sets
`MK_MODULE_TRIAL_ENGINE_OBSERVATION=1` on its parent and uses
`--engine-observation` on the host fixture. It stages the committed agent and a
new privately approved base archive, with full original/new bootstrap validation.
Its guest pressure/build mode includes a finite original RUN window for sampling.
Samples at full filesystem, unfinished build and built image require responding;
removal must refuse the old owner. The independent audit must explicitly select
`--require-engine-observation` to claim this subcase. Existing pressure/build/
health/network/native-release/resource gates remain required. Doctor v8 and
bundle v2 scopes remain unchanged.

[Shim/client race and source-stage controls](../../../evidence/runtime-20261108/g8-shim-engine-inspection-controls/README.md)
and [receipt/profile policy and prefix mutation controls](../../../evidence/runtime-20261108/g8-live-engine-observation-controls/README.md)
pass locally. Actual real-child qualification is still required.

The binary registers its inspection plugin once, then binds the concrete owner
returned by its initializer before loading the plugin graph. Reusable service
construction does not mutate containerd's process-global plugin registry. An
unbound plugin or a second initialization refuses. [Repeated-constructor and
registration-factory proof](../../../evidence/runtime-20261108/g8-shim-inspection-initializer/README.md)
retains the original duplicate-ID panic and the corrected race-enabled controls.
