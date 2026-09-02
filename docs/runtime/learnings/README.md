# Runtime gate learnings

- [`00-g0-scope-and-contracts.md`](00-g0-scope-and-contracts.md): provisional
  contract milestone, normative coverage, and later protocol/OCI contract
  drift.
- [`01-g1-host-and-isolation.md`](01-g1-host-and-isolation.md): narrow live host
  and crash/reclaim observations plus the current qualification/isolation gaps.
- [`02-g2-control-plane.md`](02-g2-control-plane.md): happy-path daemon and
  restart milestone, evidence limits, and current recovery/contract gaps.
- [`03-g3-agent-and-processes.md`](03-g3-agent-and-processes.md): historical
  direct-bundle proof reconciled with later exec, I/O, PTY, signal, and shutdown
  implementation changes and the remaining G3 gate work.
- [`04-g4-g6-mvp.md`](04-g4-g6-mvp.md): containerd/Docker, private roots,
  mediated networking, evidence qualifications, and the remaining full-gate
  work.
- [`05-g4-g6-robustness.md`](05-g4-g6-robustness.md): fresh-host bootstrap,
  restart/reconnect, forced shim death, terminal, injected ENOSPC, and the
  shared `ctr`/Docker feature matrix, updated with the remediation audit's
  current verdict, corrected claims, implementation defects, and evidence
  requirements.
- [`g0-g3-remediation-checklist.md`](g0-g3-remediation-checklist.md): audit
  findings and handoff checklist required before treating all four gates as
  fully closed.
- [`g4-g6-remediation-checklist.md`](g4-g6-remediation-checklist.md): complete
  implementation, test, replacement-instance, and evidence repair required
  before treating the G4-G6 MVP claims as full gate passes.
