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
- Guest image builds: `build-buildkit-probe-initramfs.sh` constructs the pinned
  builder fixture; `test-runtime-guest-build-live.sh` compares ordinary rootless
  startup with automatic guest bootstrap, tests Dockerfile execution/cache/export,
  and reruns authority checks. `verify-g7-guest-build-evidence.py` verifies the
  OCI image contents and retained security/cleanup evidence offline. See the
  [builder design and scope](../docs/runtime/research/guest-image-builds.md).
- Docker-in-Docker: `build-docker-probe-initramfs.sh` and
  `test-runtime-dind-live.sh` reuse the approved builder fixture machinery for
  guest-local `dockerd`, ordinary `docker build`/`docker run`, export and security
  observations. `verify-g7-dind-evidence.py` independently checks saved image
  contents and recorded authority/cleanup. See the
  [Docker guest scope](../docs/runtime/research/docker-in-docker-guests.md).

Destructive and billable operations must remain explicit Makefile targets;
they must not be hidden inside default checks.

- `build-docker-online-probe-initramfs.sh` prepares the approved online Docker
  guest diagnostic, including pinned network modules and the Python example.
- `test-runtime-dind-online-live.sh` runs the online guest Docker Hub/PyPI
  prototype on a disposable pinned lab, with scoped forwarding and cleanup.
- `probes/docker-online-controller.go` is a diagnostic primary packet pump
  using the unchanged production agent's authenticated network RPCs. Build it
  from `runtime/` with `CGO_ENABLED=0 go build -o /tmp/docker-online-controller
  ../scripts/probes/docker-online-controller.go`; credentials are read from a
  private file rather than command-line arguments.
- `verify-g7-dind-online-evidence.py` verifies Docker Hub image identity,
  PyPI download receipts, runtime HTTPS, authority observations and cleanup
  from the retained online prototype evidence.

- `collect-runtime-dependencies.py` collects private installed artifact/package/
  license metadata using the non-executing `tools/mk-dependency-buildinfo.go`
  inspector. `summarize-runtime-dependencies.py` exports only aggregate review
  facts. `harden-kerf-installation.py` protects the root-owned imported source
  tree; root `install-kerf.sh` runs it before make/pip. See the
  [dependency and imported-code guide](../docs/guides/runtime-dependency-review.md).
