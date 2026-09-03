# G0-G3 remediation proof, 2026-09-03

## Verdict

The current implementation's **narrow, explicitly scoped G0-G3 claims are
substantiated** by this run. The normative gates are **not closed**. Each gate
manifest is therefore `provisional`, not `pass`: the open implementation and
fault-matrix requirements in the remediation checklist cannot be converted
into success merely by rerunning the existing happy paths.

The disposable GCE instance `mklinux-g0-g3-proof-20260903` was an
`n2-standard-16` in `asia-southeast1-b`, restored from the known custom-kernel
snapshot. It ran Multikernel `7.0.0-mk2-gce-lab` and Kerf 0.2.0. The instance,
its 100 GiB auto-delete boot disk, and its ephemeral address were deleted.
Only the documented pre-existing disk and snapshots remain.

## Claim-to-proof matrix

| Gate | Claim supported by this run | Decisive proof | Result boundary |
| --- | --- | --- | --- |
| G0 | Current schemas/fixtures, Go race tests, vet, and shell syntax checks pass together. | [`g0-local-verification.log`](g0-local-verification.log), [`manifest-g0.json`](manifest-g0.json) | Contract drift and end-to-end fail-closed OCI handling remain open. |
| G1 | The narrow host report qualifies this pinned host; an intentional child PID-1 crash is reclaimed without losing host services or resources. | [`g1-host-report.json`](g1-host-report.json), [`g1-live.log`](g1-live.log), [`final-host-state.log`](final-host-state.log), [`manifest-g1.json`](manifest-g1.json) | No hostile-kernel isolation claim; allocation-policy and topology/controller gaps remain. |
| G2 | A committed running sandbox survives daemon `SIGKILL`; restart can stop/delete it and restore resources from durable state. | [`g2-live.log`](g2-live.log), [`state-after-sigkill`](g2/state-after-sigkill/state.json), [`state-after-cleanup`](g2/state-after-cleanup/state.json), [`manifest-g2.json`](manifest-g2.json) | This is one crash boundary, not orphan/pre-snapshot or host-reset recovery proof. |
| G3 | A freshly authenticated agent runs one OCI process with split output, exact exit 23, child kernel identity, quiescence, and cleanup. | [`lifecycle.json`](g3/lifecycle.json), [`console.log`](g3/console.log), [`g3-live.log`](g3-live.log), [`manifest-g3.json`](manifest-g3.json) | Broader OCI, process, framing, reconnect, backpressure, and poweroff requirements remain open. |

## Remediation performed during the run

1. The first G1 attempt failed before mutation because `make sync` omitted
   `guest/runtime-crash-init`. The failure is retained in
   [`g1-attempt1-missing-asset.log`](g1-attempt1-missing-asset.log), and the
   canonical sync target now includes the asset.
2. The remote wrapper was changed to strict error propagation for the decisive
   rerun so a later guest-agent check could not mask a scenario failure.
3. G2 now accepts explicit durable state/evidence directories and retains every
   API response plus journal/snapshot copies immediately after `SIGKILL` and
   after clean deletion. The retained journal grew from six entries at crash
   to ten complete lifecycle entries after recovery.
4. G3 now generates a random 128-bit generation and random 256-bit token per
   run. Cleanup redacts the token from console evidence on success or failure;
   the collected tree was scanned for an unredacted command-line token.
5. Four separate schema-valid gate manifests replace the historical combined
   G3-labelled manifest pattern for this run.

## Learnings and remaining claim boundary

- Reproduction automation is part of the claim. A correct runtime path with a
  missing synced fixture is not independently reproducible.
- Pass markers are insufficient for recovery claims. The durable state proves
  `RUNNING` at daemon death and `ABSENT` after restart-driven cleanup, while the
  journal shows intent/complete pairs for create, load, start, stop, and delete.
- Secret-safe capture must be automatic. Fresh randomness without failure-path
  redaction would improve authentication while making retained evidence unsafe.
- The host has no `/sys/devices/system/memory/online`; portable cleanup evidence
  should use `/proc/meminfo` plus the Kerf pool/instance ledger on this kernel.
- Immediately after the delete request, GCE temporarily returned the instance
  as `STOPPING` although the boot disk was already absent. Final cleanup was not
  asserted until `instances describe` returned absent and all inventories were
  rechecked.

The strongest correct conclusion is therefore: **G0-G3's current narrow
milestones reproduced and their evidence quality materially improved; the
claim that all four normative gates are complete remains false.**
