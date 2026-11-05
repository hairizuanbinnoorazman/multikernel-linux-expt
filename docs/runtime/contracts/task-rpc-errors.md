# Task RPC failure replies

Source `18c97b4f` returns an explicit Task facade from the production shim factory.
All 17 Task methods delegate once; failure replies contain a fixed method-specific
description, no gRPC details and no partial response body. The outward error holds
only a method identifier and code, without the private cause or unwrap chain.
Success responses and request/context identity retain their delegate contract.

Code classification matches the pinned ttrpc v1.2.7 transport exactly. In particular,
raw containerd errdefs and wrapped context sentinels can map to Unknown; this change
does not reinterpret them. Direct context/I/O sentinels, filesystem predicates,
wrapped gRPC statuses and malformed nil/OK/nonstandard status carriers are covered
by a real Unix-wire comparison with raw registration. Private causes are not
formatted to determine a code. Classification may still invoke their GRPCStatus
or filesystem predicate methods, as the transport does.

The explicit facade satisfies shim.Shim without embedding its delegate, so a new
Task API method requires implementation. shim.Run embeds the returned TaskService
inside its own wrapper: relying on an optional service interceptor alone would not
install this boundary. The constructor test confirms the production factory uses
it. Manager StartShim/Cleanup forward unchanged; constructor/startup errors,
runtime-info/flag output, other producers and successful Task payloads remain
separate from this error contract. Internal service calls and recovery retain their
original errors and behavior.

[Local proof](../../../evidence/runtime-20261105/g8-task-rpc-controls/README.md)
retains the raw-registration negative control, all-method privacy tests, wire-code
oracle, exact delegation, panic-on-format cause, production constructor and Manager
checks. It also retains the initial group-writable bundle fixture refusal; only
the fixture permission was corrected. Focused/full runtime race and required checks
pass. The separate [GCE fixture](../../../evidence/runtime-20261105/g8-task-rpc-gce/README.md)
also passes success/zero, preserving all 201 artifacts and prior failures with
controlled delegates and no installed-service or kernel backend changes.
The separate [actual Docker regression](../../../evidence/runtime-20261105/g8-live-docker-task-rpc/README.md)
also passes normal build/recovery, separation, diagnostics and cleanup for this
source, without injecting the controlled private faults. Cold restoration remains
required. Aggregate diagnostic pressure/retention,
provenance/readiness and full G7/G8 remain separate.

No nested virtualization is used; direct child kernels share host trust after
child-kernel compromise.
