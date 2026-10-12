# G7 boundary assessment: source and portable evidence

This is a **partial G7 assessment**, not a gate pass. SEC-01 and SEC-11 have
completed pinned-source assessments identifying an architectural limitation.
SEC-02 has a source review and fresh portable agent tests; the live workload
inventory and credential remediation remain open.

The conclusion is in the
[security boundary assessment](../../../docs/runtime/research/g7-security-boundary-assessment.md).

## Captured evidence

The [machine-readable review summary](review-summary.json) distinguishes task
completion, findings, pending live collection, and unsupported security claims.

- [Source audit](source-audit.log): nine pinned upstream files, SHA-256 hashes,
  immutable URLs, and source anchors. The first seven hashes match the original
  G1 audit and the restored host copies.
- [Kernel source archive](kernel-reviewed-source.tar.gz): complete reviewed
  upstream files with their GPL license; paths are relative to the kernel root.
- [Agent race suite](agent-tests.log): fresh `go test -race -count=1 -v ./agent`.
  Tests include unsupported-feature rejection, root-path validation,
  `noNewPrivileges`/rlimit application, secret-safe wire errors, and framing.
  No-chroot lifecycle fixtures do not prove workload confinement.
- [Fixture syntax check](fixture-syntax.log): shell parsing only; no child boot
  or authority result is implied.
- [Source identity](source-identity.json), [per-file hashes](source-hashes.json),
  and [tested-source archive](tested-source.tar.gz): exact production Go sources,
  fixtures, bootstrap, and build inputs. Current agent/ctl binaries were built
  locally and hashed; they were not deployed. Tar owner metadata is normalized.
  [Agent build](build-agent.log), [controller build](build-agentctl.log), and
  [binary identities](agent-build-identity-v2.log) retain compilation commands
  and digests. These are development binaries with `revision=unknown`; source
  hashes bind provenance. The first identity capture assumed JSON version output
  and failed; [that attempt](agent-build-identity.log) is retained, followed by
  the corrected plain-text identity capture. This was a collector error.
- [Host source/configuration audit](host-source-audit.log): seven source hashes,
  relevant kernel configuration, pinned Kerf revision, and online CPU inventory.
  [Host artifact identity](host-artifact-identity.json) transcribes the observed
  kernel digest from the read-only tool result and compares it with the previous
  qualified manifest. The configuration capture contains selected symbols,
  not a complete new release manifest. The source audit's `find` reported the
  unmounted control filesystem; its zero overall status comes from the final
  CPU inventory command, not a successful child-directory inspection.
- [Approval events](approval-events.json): review rejections and dispositions.
  A rejected command is not a command execution or a failed security probe.
- [Bundle verification](bundle-verification-v2.log): both archives match their
  digests, 134 source files and nine kernel files match recorded byte hashes,
  and project-wide cleanup inventories match. The earlier verification covers
  the source snapshot before the verifier was added to its own archived inputs.

## Reproduce the checks

From the repository root:

```bash
python3 scripts/verify-g7-boundary-evidence.py evidence/runtime-20261012/g7-security-boundary
mkdir -p /tmp/mk-g7-reviewed-source
tar -xzf evidence/runtime-20261012/g7-security-boundary/kernel-reviewed-source.tar.gz -C /tmp/mk-g7-reviewed-source
python3 scripts/audit-runtime-isolation-source.py /tmp/mk-g7-reviewed-source
cd runtime
GOCACHE=/tmp/mk-go-cache go test -race -count=1 ./agent
```

Use `tested-source.tar.gz` and the recorded Go toolchain for the exact reviewed
source snapshot. Neither reproduction path executes privileged live probes.

## Cloud scope and cleanup

A disposable `n2-standard-16` VM with a 100 GB auto-delete boot disk was restored
from the pre-DAXFS recovery snapshot to verify source/artifact identity and
prepare for the three authority fixtures. It used no service account or API
scopes. OS update services remained enabled. Source and kernel hashes matched
the retained qualification, but fresh full host qualification was not run.
The Multikernel control filesystem was not mounted and no child or pool was
created by this run.

Automatic approval review rejected transfer of repository-derived scripts and
binaries to the VM pending explicit authorization. No fixtures were uploaded,
and **none of the three live authority tests ran**. The host was deleted rather
than left running while authorization was pending. Recreating a disposable host
and authorizing transfer is necessary to finish SEC-02.

[Before](resources-before.json) and [after](resources-after.json) project-wide
ledgers record all resources, including pre-existing disks/snapshots. Their
comparison and resource disposition are in [cleanup verification](cleanup-verification.json).
The [instance attachment record](instance-before-delete-v2.json) records the
boot disk's auto-delete setting. [Pre-delete audit](pre-delete-audit.log) records
host health and confirms that no fixture upload or child start occurred.
[Deletion output](delete-instance.log) records the instance removal.

This run incurred VM/disk usage; the resource ledgers and instance timestamps
are the usage record, not a calculated invoice. Existing recovery snapshots and
the experiment disk were preserved; they were not used as writable test disks.

## Interpretation limits

The audit does not demonstrate a primary-host escape or run arbitrary
child-kernel code. It identifies that a child at kernel privilege controls its
first-level mappings without a separately established child-specific hardware
barrier. Finite passing tests do not prove absence of vulnerabilities.
No gVisor comparison, vulnerability inventory, complete G7 fault/fuzz/soak
matrix, independently reviewed security certification, or credential fix is
claimed. The source finding about credential visibility requires live
measurement and remediation before SEC-02 can close.

Project/operator identities and the host boot UUID are replaced with
placeholders/anonymized values. Component/source hashes, source excerpts,
command outcomes, and resource equality are preserved.
