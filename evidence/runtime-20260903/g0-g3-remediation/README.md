# G0-G3 continuing remediation evidence — 2026-09-03

This directory indexes the second disposable GCE run on
`mklinux-g0-g3-remediation-20260903` (`n2-standard-16`, boot ID
`a0649bbe-06e5-4f61-ba93-36b473cf0b69`). The guest scenarios cleaned up their
child kernels, pools, and daemon processes. Final GCE inventories confirm the
instance, auto-delete boot disk, and any same-named static address are absent.

## Current verdict

The evidence substantiates additional narrow G1-G3 remediation claims, but the
gates remain `provisional` because their unchecked exit criteria still apply.

| Claim | Verdict | Direct proof |
| --- | --- | --- |
| G0 agent method/reply schema and terminal/architecture policy match the implemented v1 behavior; the complete local suite passes | proved for the synchronized artifacts | `manifest-g0.json`, `local-validation.log` |
| G1 preserves zero topology/NUMA identifiers and still reclaims an intentional child PID 1 crash | proved for this host/run | `manifest-g1.json`, `g1-host-report.json`, `g1-topology-live.log`, `g1-live.log` |
| G2 retains two disjoint running sandboxes across daemon `SIGKILL` and recovers both after restart | proved for this operation point | `manifest-g2.json`, `g2/state-after-sigkill/`, both sets of API replies, `g2/state-after-cleanup/` |
| G2 rejects an 8 GB pool's second 4 GiB request before Kerf and admits both with a 12 GB pool | live-proved with production preflight remediation | `preflight-8gb/`, `g2-memory-preflight-live.log`, `g2-memory-admission/`, `g2-memory-admission-live.log` |
| G3 performs the authenticated direct-bundle lifecycle with split output, exit 23, quiescence, and exact generated-token absence after cleanup | proved for this lifecycle | `manifest-g3.json`, `g3/lifecycle.json`, `g3/token-redaction-check.txt`, `g3/console.log` |
| Guest resources return after each scenario | proved | pass logs and `resources-current.json` |
| Full G0, G1, G2, or G3 gate completion | **not proved** | unchecked items in `docs/runtime/learnings/g0-g3-remediation-checklist.md` |

## Failed attempts retained as learning

- `failed-8gb/` records the fail-closed `BACKEND_FAILURE` on the second child
  before admission accounting existed. The earlier harness also tracked its
  `sudo` supervisor rather than the real privileged daemon and left the
  restarted daemon orphaned. PID 5686 was explicitly removed; the corrected
  `preflight-8gb/` and `g2-memory-admission/` runs supersede its cleanup claim.
- The first G3 evidence-validation scan looked for any 64-hex string and found
  the initramfs SHA-256 checksum. It was not a credential leak. The corrected
  harness checks the exact generated token held in memory and emits
  `g3/token-redaction-check.txt` only after that token is absent.

## Resource state

- `resources-before.json` describes the clean guest baseline on the retained VM.
- `resources-current.json` is the last pre-deletion snapshot: the VM was
  `RUNNING`, with CPUs `0-15`, no Multikernel child instances or daemon, and the
  Google guest agent active.
- `resources-after.json` records empty instance, disk, and named-address
  inventories after termination.
