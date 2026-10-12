# G7 live authority assessment — 2026-10-12

Result: **partial G7 assessment with confirmed security failures**. All three
workload profiles completed; this is not a successful confinement qualification.
Executor/source reviewer: Codex. Independent review remains pending.

| Observation | Non-root 1234 | Root with empty capabilities + NNP | Root with omitted capabilities |
| --- | --- | --- | --- |
| Effective capabilities | Zero | Zero | All 41 capability bits |
| Kernel command-line credential visible | Yes | Yes | Yes |
| Agent argument credential visible | Yes | Yes | Yes |
| Child-bootstrap canary read/write | No / No | No / No | Yes / Yes |
| NoNewPrivs / Seccomp | 0 / 0 | 1 / 0 | 0 / 0 |

The canary belongs to the child bootstrap, outside the workload OCI chroot.
Its access does **not** demonstrate primary-host or sibling escape. Credential
values were never emitted by the workload; console tokens are redacted. No
stolen-credential RPC, arbitrary memory access, malicious module load, privileged
I/O operation, or destructive primary-host probe was performed. Device permission
predicates do not prove that kernel operations would succeed.

## Evidence and reproducibility

- [Manifest](manifest.json) records claim assertions and their failures.
- [Observed profiles](observations.json) summarize the raw production-agent
  responses under `host/authority-completed/`, which also contain console,
  relay, control, Kerf, cleanup, and final inventory records.
- `host/kernel.config`, `host/kernel-build.diff`, and `prepare-host.log` retain
  the full configuration used to build the transport module, local build changes,
  pinned revisions and hashes. The original full configuration embedded in the
  restored `vmlinux` was not separately extracted; its binary digest matches
  the prior qualification. The kernel was not rebuilt for this assessment.
- `source-identity.json`, `source-hashes.json`, `tested-source.tar.gz`, and
  `workspace.diff` freeze reviewed sources. `fixtures-v4.tar.gz` is the exact
  successful upload, and its remote digest appears in `pre-delete-audit.log`.
  Prior fixture uploads are retained separately. Agent binaries are unchanged
  from the [earlier source/race-test bundle](../g7-security-boundary/README.md);
  its immutable kernel source archive also supplies the source reference.
- [Attempt history](attempt-history.json) explains four failed attempts: bootstrap
  directory hiding and relay readiness, unsafe fixture config permissions,
  bounded bundle metadata diagnosis, and a parser error for omitted zero exit
  status. Earlier failure traps did not retain result JSON; those attempts are
  not counted as completed authority measurements.
- `pre-delete-audit-v2.log` asserts all CPUs returned, no children/helpers, and
  no configured pool. The original audit's invalid Kerf command is retained.
- `delete-instance.log`, exact-name empty resource queries, before/after
  project-wide ledgers, and `cleanup-verification.json` establish removal of the
  disposable VM and auto-delete boot disk. Pre-existing resources were unchanged.
  No test-created cloud resource is retained.

The bootstrap now creates `/dev/pts`, `/dev/shm`, and `/dev/mqueue` after mounting
devtmpfs. The completed boots exercise this correction. The fixture builder
sets private config permissions; the harness awaits relay readiness and handles
the production API's omitted zero exit code. Production credential and default
root policy behavior was not changed by this assessment.

Verify the source bytes, profile observations, and cleanup without launching probes:

```sh
python3 scripts/verify-g7-authority-evidence.py evidence/runtime-20261012/g7-authority-live
python3 scripts/check-runtime-evidence.py
```

Project, operator-home, and host boot identifiers are pseudonymized. Tokens are
redacted. Command transcripts are captured outputs, with command outcomes bound
by the manifest and attempt history; they are not independently signed attestations.
The diagnostic initramfs does not qualify mediated roots or Docker policy. Full
G7, credential remediation, independent review, and a pinned gVisor comparison
remain open. See the [current confidence assessment](../../../docs/runtime/research/g7-security-boundary-assessment.md).
