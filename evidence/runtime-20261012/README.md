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
