# G7 credential and workload authority remediation

**Scoped verification passed: four workload profiles and two rejected requests.**
Full G7 remains partial. Executor/source reviewer: Codex; independent review is
pending. This bundle preserves the new sources and supplements the immutable
[baseline](../g7-authority-live/README.md), whose confirmed failures remain intact.

| Observation | Non-root | Root, empty capabilities | Root, omitted capabilities | Root, 14 allowed capabilities |
| --- | --- | --- | --- | --- |
| Kernel/agent command-line token visible | No / No | No / No | No / No | No / No |
| Child-bootstrap canary read/write | No / No | No / No | No / No | No / No |
| Effective capabilities | Zero | Zero | Zero | `00000000a80425fb` |
| NoNewPrivs | 1 | 1 | 1 | 1 |
| Memory, I/O, kernel-log, bootstrap disk devices exported | No | No | No | No |

Explicit `CAP_SYS_PTRACE` and `noNewPrivileges: false` requests were rejected
before workload start. The ordinary capability profile requests the entire
supported 14-capability set, including DAC override and chroot, and still cannot
read/write the exact bootstrap canary. This is evidence for those profiles and
that route, not universal confinement or a primary-host escape demonstration.

## Fixes exercised

The production Kerf loader appends a private root-owned credential cpio archive
to the approved image using a sealed memory file. All six boots use this loader;
`g7-credential-load` is a bounded driver of the production `kerf.CLI.Load` method.
The original approved initramfs is not modified. Credentials are absent from
kernel and agent command lines. The agent validates and deletes the one-shot
private file and disables dumpability before accepting authenticated requests.

Omitted root capabilities default to an empty set, no-new-privileges is mandatory,
and excluded capabilities and unequal root bounding/permitted/effective contracts
fail closed. The bootstrap exports selected ordinary workload devices instead
of its entire devtmpfs. The mediated bootstrap also transfers the token through
private tmpfs before switching roots, keeping it off the mediated storage image;
**that mediated path and Docker compatibility were not live-requalified here**.
See the [authority contract](../../../docs/runtime/contracts/workload-authority.md).

## Retained evidence

- [Manifest](manifest.json), [profile observations](observations.json), and
  `host/remediation-rerun/`: production agent replies, console, control, relay,
  Kerf, cleanup, rejection, actual loaded-initramfs digests, and final inventory.
- `source-identity.json`, `source-hashes.json`, `tested-source.tar.gz`,
  `fixtures.tar.gz`, and `workspace.diff`: exact new Go/bootstrap/fixture sources,
  tests, module pins, and the uploaded diagnostic payload. Remote binary/payload
  digests appear in `pre-delete-audit.log`.
- `runtime-tests-final.log`: full `go test -race -count=1 ./...` passed.
  `oci-validation.log`: 103 semantic cases plus existing boundaries passed.
  Portable tests independently decode the cpio credential archive and reject
  unsafe credential files and excluded capability contracts.
- `host/kernel.config`, `host/kernel-build.diff`, and `prepare-host.log`: full
  transport-module build configuration, local build changes, and pinned identities.
  The kernel binary was not rebuilt; its digest matches prior qualification.
- [Attempt history](attempt-history.json), `tests-attempt-1.log`, and
  `host/remediation/`: retained failed attempts. The snapshot kernel's mode 0775
  correctly failed production artifact validation. `kernel-policy-copy.log`
  proves the protected root-owned 0644 copy has the same approved binary digest.
- `pre-delete-audit.log`, `delete-instance.log`, exact-name resource queries,
  before/after cloud ledgers, and `cleanup-verification.json`: six children
  removed, CPUs/pool/helpers returned, VM and auto-delete boot disk deleted, and
  project-wide resource baseline unchanged. Update services stayed enabled.

The credential-bearing loaded images were not published; their digests and
construction inputs are retained. Command outputs lack independent signatures.
Project/operator/host boot identifiers are pseudonymized and credentials are
redacted. Intermediate failed test source was not separately archived.

Verify offline:

```sh
python3 scripts/verify-g7-remediation-evidence.py evidence/runtime-20261012/g7-authority-remediation
python3 scripts/check-runtime-evidence.py
```

The remaining limits include shared child procfs and root signal/DoS authority,
production mediated-root/Docker qualification, independent review, remaining G7
checks, and the missing independently enforced boundary after child-kernel
compromise. These fixes do not establish stronger escape resistance than gVisor.
The [current assessment](../../../docs/runtime/research/g7-security-boundary-assessment.md)
keeps confidence attached to the measured claims.
