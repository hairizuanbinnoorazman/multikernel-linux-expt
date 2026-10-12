# Script index

Scripts are grouped here by artifact type so the root `Makefile` can sync them
to a stable remote path. For the thematic mapping, see
[`../docs/experiments/ASSET-MAP.md`](../docs/experiments/ASSET-MAP.md).

- Bring-up: `provision-kernel.sh`, `install-kerf.sh`, `verify-host.sh`,
  `build-initramfs.sh`, and `smoke-*.sh`.
- DAXFS: `daxfs-*.sh`, `build-daxfs-*.sh`, and `test-daxfs-*`.
- Direct ext4: `disk-roots-*.sh` and `test-ext4-*`.
- Mediated ext4: `mediated-*.sh`, `build-mediated-*.sh`, and
  `test-mediated-*`.
- Runtime gates: `test-runtime-g1.sh`, `test-runtime-g2.sh`,
  `test-runtime-g3.sh`, `test-runtime-g4-g6.sh`,
  `test-runtime-g4-g6-feature-matrix.sh`,
  `test-runtime-containerd-restart-live.sh`,
  `test-runtime-docker-restart-live.sh`,
  `test-runtime-shim-death-live.sh`,
  `test-runtime-mkruntimed-restart-live.sh`, and `test-runtime-recovery.sh`.
  Focused live Task event ordering and exit semantics are covered by
  `test-runtime-task-events-live.sh`.
- Clean shim shutdown and same-name recreation are covered by
  `test-runtime-clean-shim-restart-live.sh`.
- Concurrent ctr/Docker resource identity and churn are covered by
  `test-runtime-concurrent-churn-live.sh`.
- Repository checks: `check-docs.sh`.
- G7 security boundary assessment: `audit-runtime-isolation-source.py` verifies
  the pinned review files. `build-security-authority-initramfs.sh` and
  `test-runtime-security-authority-live.sh` provide explicit, disposable-host
  diagnostic fixtures; they are not included in default checks. See the
  [assessment](../docs/runtime/research/g7-security-boundary-assessment.md).
  `verify-g7-boundary-evidence.py` checks the scoped source archives and cloud
  cleanup ledger comparison without running privileged probes.
  `verify-g7-authority-evidence.py` checks the subsequent live bundle's exact
  fixture sources, observed authority profiles, and cleanup evidence.
  The remediation fixture requires `MK_SECURITY_LOAD`, built from
  `runtime/tools/g7-credential-load`, to exercise production Kerf credential
  delivery. It runs four authority profiles and two rejection cases.
  `verify-g7-remediation-evidence.py` checks those results offline. Direct G3
  initramfs builders require a private `MK_AGENT_TOKEN_FILE`; credentials must
  never be included in child kernel or agent command lines.
- Final live resource-return audit: `audit-runtime-final-resources-live.sh`.

Destructive and billable operations must remain explicit Makefile targets;
they must not be hidden inside default checks.
