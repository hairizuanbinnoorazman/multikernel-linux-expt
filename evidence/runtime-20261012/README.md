# Runtime security assessment started 2026-10-12

- [G7 boundary assessment](g7-security-boundary/README.md): reproduced pinned
  kernel source, current agent race suite, proposed live authority fixtures,
  source findings, approval limitations, and disposable cloud cleanup.
- [G7 live authority assessment](g7-authority-live/README.md): all three profiles
  collected after explicit upload authorization; credential exposure confirmed,
  default-root child-bootstrap canary access, retained failed attempts, and
  verified disposable host cleanup. G7 remains open.
- [G7 authority remediation](g7-authority-remediation/README.md): private
  credential handoff and privilege/device fixes; four workload profiles plus
  two rejected requests, full Go race-suite results, source hashes and cleanup.
  These bounded passes do not close full G7 or qualify hostile-child containment.
- [G7 guest image builds](g7-guest-builds/README.md): automatically prepared
  rootless BuildKit inside a dedicated guest; multi-stage RUN/COPY, cache, OCI
  export/content verification, failure propagation, authority checks and cleanup.
  This remains an offline initramfs-backed prototype, not default Docker support.
- [G7 Docker-in-Docker](g7-dind/README.md): guest-local rootless Docker Engine;
  ordinary Docker builds and nested runs, cache and failure checks, independently
  verified saved image, authority observations and disposable cloud cleanup.
  Registry networking and production profile integration remain unqualified.
- [G7 online Docker Python builds](g7-dind-online/README.md): fresh guest pulls
  Python from Docker Hub, installs five packages from HTTPS PyPI, runs an HTTPS
  app, reuses build cache, and retains package hashes, authority and cleanup.
  Production CNI/storage/profile integration and resource enforcement remain open.
