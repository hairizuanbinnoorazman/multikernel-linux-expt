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

The [source-bound regression and GCE proof](../../../evidence/runtime-20261109/g8-create-intent-marker/README.md)
qualify four fresh entry paths, real owned-process exit, private publication,
quarantine/inventory refusal and post-publication task retention. The full shim
and safefile race suites and required checks pass for the corrected source.

See the [ownership and rollback contract](namespace-holder-ownership.md).

## Read-only inspection

Use the bundle path from the existing host ownership records:

```sh
sudo containerd-shim-multikernel-v2 --inspect-create-intent \
  --bundle /absolute/path/to/the/host-bundle
```

This standalone mode runs before the shim launcher and observes protected
directory/file identities and names without reading marker contents, contacting
daemons or creating a child. Its fixed version-1 JSON reports `pending`, `absent`
or `inconclusive`, with a fixed code. Paths, inode values, credentials, marker
data and raw errors are excluded. `contents_observed`, `cleanup_authorized`,
`admission_guaranteed` and `recovery_complete` are always false.

`pending` and `absent` return 0 for a completed observation; neither grants
permission to remove a marker or allocate resources. `inconclusive` returns 1;
invalid arguments return 2. A missing marker only means it was not observed in
this snapshot. Marker/bundle changes during observation produce an inconclusive
result. The existing 256-name bound applies, including removal quarantines.

Observation has a two-second budget with at most one filesystem worker. Output
has one at-most-1024-byte attempt with a separate 50 ms budget and no stderr
fallback or retry. A blocked worker can remain until this standalone process
exits. Closed/full stdout preserves exit 1 through a private CLOEXEC duplicate;
inherited descriptor flags and signal policy are unchanged. Discard incomplete
output or output from a nonzero exit; the inconclusive JSON, when delivered,
still gives a fixed refusal reason. The helper is not for resident polling loops.

Direct child kernels need no nested virtualization and share primary trust
after child-kernel compromise.
