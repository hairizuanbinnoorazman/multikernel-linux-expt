# `diagnostics`

This package publishes a bounded snapshot from the existing validated doctor
APIs. It owns destination policy, byte limits, private exclusive publication,
receipt hashing and fixed publication errors. It does not read state files,
collect logs, run subprocesses, repair resources or decide service readiness.

The production CLI supplies the component and version-7 observers under one
deadline. Preserve that context and the reports' failure/unobserved semantics.
The inspector callback is internal trusted code, not an arbitrary JSON input.
See the [bundle contract](../../../docs/runtime/contracts/diagnostic-bundle.md).

`CollectWithBudget` adds optional per-directory logical-byte and file-count
limits, metadata-only inventory checks and drift refusal. All current collectors
serialize through a nonblocking directory-inode lock. No capture deletes prior
evidence, persists a quota policy or reserves filesystem capacity.
