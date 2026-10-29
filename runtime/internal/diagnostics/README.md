# `diagnostics`

This package publishes a bounded snapshot from the existing validated doctor
APIs. It owns destination policy, byte limits, private exclusive publication,
receipt hashing and fixed publication errors. It does not read state files,
collect logs, run subprocesses, repair resources or decide service readiness.

The production CLI supplies the component and version-7 observers under one
deadline. Preserve that context and the reports' failure/unobserved semantics.
The inspector callback is internal trusted code, not an arbitrary JSON input.
See the [bundle contract](../../../docs/runtime/contracts/diagnostic-bundle.md).
