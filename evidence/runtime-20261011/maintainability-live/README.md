# Maintainability refactor: live GCE checks

Validation uses the dirty workspace based on `d93d0edfe651b713be8379f694faff471c9c7101`.
The source archive and exact per-file hashes are retained in [source.tar.gz](source.tar.gz),
[source-identity.json](source-identity.json), and [source-hashes.json](source-hashes.json).
[source-files.json](source-files.json) lists the archived paths.
The archive was committed only in the disposable VM as snapshot
`49bba37bc4b1d584df6799219be20e26054cf126`; candidate binary revisions refer to this
snapshot, not to a commit in the user workspace. The original transfer archive hash matched on both hosts. Published archive
owner metadata is normalized; both archive hashes are recorded separately in
source-identity.json, and all file contents are unchanged.

The disposable host is `mklinux-maintainability-20261011`, an `n2-standard-16` in
`asia-southeast1-b`, running the pinned `7.0.0-mk2-gce-lab` kernel. A new 30 GB disk
`mklinux-maint-storage-20261011` supplies private container root storage. The
pre-existing experiment disk is not attached or modified.

The full VM race suite, vet, candidate builds, and binary installation passed in
[host-build-v3.log](host-build-v3.log). Candidate identities and deployed hashes
are in [release-manifest.json](release-manifest.json) and
[component-audit-v2.log](component-audit-v2.log). The guest agent hash matches the
candidate release; the guest initramfs was rebuilt for this run.

Earlier failed attempts are retained. The VM's initial Go test temporary paths
exceeded the Unix socket path limit; setting both TMPDIR and GOTMPDIR to `/var/tmp`
resolved this. Deployment inputs needed root ownership. The first live bootstrap
needed the kernel config next to the approved kernel artifact and the transport
module in the service's expected location. The first component-audit invocation
used an unsupported CLI subcommand; the retry used the manager's verifier.
These corrections affect the disposable test environment; runtime source was unchanged.

The first feature-matrix attempt passed through guest attachment and terminal
allocation, then failed cleanup while an unattended OS upgrade restarted host
services and the runtime lost its storage helper. This is retained as a failed
attempt, not a full matrix pass. Upgrade timers were subsequently disabled.
After reboot, the failed fixture state was archived on the disposable VM and a
fresh smoke test passed. A second attempt stopped at its initial inventory check
because orphan containerd metadata remained from the interrupted terminal case.
That exact record was removed after proving its task and child were absent.
The third matrix attempt passed in [feature-matrix-v3.log](feature-matrix-v3.log),
including both clients, live resizing, and a final zero-resource inventory.
All focused suites also passed: [containerd restart](containerd-restart.log),
[Docker live-restore restart](docker-restart.log), [forced shim death](shim-death.log),
[network isolation](network-isolation.log), and [repeated clean shim lifecycle](clean-shim-restart.log).

[shim-declaration-audit.json](shim-declaration-audit.json) confirms all 575
production/test declaration groups (583 symbols) are unchanged from the pre-split
shim. [audit-shim.go](audit-shim.go) is the reproducible audit helper; its input is
retained in [shim-baseline.tar.gz](shim-baseline.tar.gz). It compares
standalone gofmt-normalized declarations, including all attached and internal
comments, and ignores import layout.

A fresh [local race-suite and vet run](local-validation.log) passed.
[make docs-check](docs-check.log) also passed, including the evidence schema and
local links.
[final-audit.log](final-audit.log) verifies candidate and guest-agent hashes,
zero active children/tasks/roots/endpoints/shims/helpers, and all runtime services
active with no automatic restarts. These are scoped regression checks, not full
certification of every runtime acceptance gate or arbitrary OS-update recovery.

The disposable instance, boot disk, and 30 GB storage disk are deleted;
see [delete-instance.log](delete-instance.log) and
[delete-storage-disk.log](delete-storage-disk.log). The complete
[post-run resource ledger](resources-after.json) matches the
[pre-run ledger](resources-before.json), excluding capture timestamps. The
pre-existing experiment disk, snapshots, addresses, and firewall rules were
preserved. No resources created by this run remain. The archived failed fixture
state on the VM was disposed of with the temporary disks.

[manifest.json](manifest.json) records the scoped pass, assertions, component
hashes, all captured command outcomes (including failed attempts), and cleanup.
[shim-file-sizes.json](shim-file-sizes.json) records the final layout: largest
production file 617 lines, largest test file 509 lines, entrypoint 81 lines.

Publication sanitization follows the repository evidence index: project and
operator paths/names use placeholders, UUIDs use distinct anonymized values,
cloud/operator IPv4 addresses use documentation ranges, and authentication
tokens are redacted. Equal identities still compare equal and distinct identities
remain distinct. Test outcomes, timestamps, component hashes, source file hashes,
and cleanup inventory comparisons are preserved. Raw captures remain in a private
local backup outside the repository. Archive owner metadata is normalized while
preserving every archived file's bytes.
