# Durable incomplete-Create refusal

Before allocation, the shim publishes `.multikernel-create.json` exclusively in
the held bundle directory, with mode 0600 and file/directory synchronization.
The bounded record contains only version, namespace, task and bundle identity.
It contains no credential, environment, configuration, guest output, process ID
or cleanup command. It is an uncertainty marker, never cleanup authority.

Any surviving marker blocks a fresh worker (including delete mode), Create and
manager Cleanup before recovery, event replay, backend validation or allocation.
Entries are observed without following links or reading their contents. An
associated removal quarantine also blocks admission; the directory inventory
is bounded to 256 names. Malformed, foreign, changed and oversized inventories
fail closed and are preserved. A new worker never adopts the marker for deletion.

Only the creating operation's captured file identity can remove the marker.
Successful Create removes it after durable task state and create-event acceptance;
completed rollback removes it after all cleanup barriers. A post-publication
removal failure retains the committed task instead of running destructive
rollback. Incomplete cleanup also refuses Start and Task Delete in the live
worker. Changed marker or bundle identities require inspection.

This detects incomplete Create across worker exit without inventing a task
record or treating an empty bundle as proof that resources were released. It
does not persist the full cleanup configuration, adopt historic process IDs,
automatically reconcile a failed operation or add an operator repair API. Do not
remove a marker merely to reopen admission: inspect the original daemon/rootfs/
network journals and generation ownership first. Durable recovery and explicit
operator completion remain open G7/G8 work. Process-exit tests do not qualify
power-loss durability or an actual child-Docker fault/recovery lifecycle.

See the [ownership and rollback contract](namespace-holder-ownership.md).
Direct child kernels need no nested virtualization and share primary trust
after child-kernel compromise.
