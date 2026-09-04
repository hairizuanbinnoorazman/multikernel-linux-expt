# G0-G3 final remediation evidence

This directory retains the 2026-09-04 final G0-G3 run on disposable GCE
instance `mklinux-g0-g3-final-20260904`, restored from the approved pre-DAXFS
snapshot. The archive upload was explicitly authorized after the initial
security-review pause. Remote checksums were verified after download. A staged
secret scan then found the generated G3 credential in primary-kernel kexec
command-line traces outside the initially scanned G3 directory; those five
occurrences were capture-redacted before commit. `redaction.log` records the
transformation, `remote-evidence-sha256.txt` preserves the downloaded checksum
ledger, and `evidence-sha256.txt` covers the sanitized repository copy.

## Claim-to-proof index

| Gate | Decisive proof |
| --- | --- |
| G0 | `provenance.log` pins Multikernel/Kerf and module identity; `component-sha256.txt`, `g0/kernel-manifest.json`, `g0/g2-config.json`, and the transport patch retain the approved artifact boundary; `local-validation.log` records schemas, OCI/bootstrap validators, race tests, vet, docs, shell syntax, and diff checks. |
| G1 | `g1/host-report.json` is the qualified host report; `g1/live.log` proves the child CPU/memory view, intentional PID 1 exit, and reclaim; APIC, device, dmesg, boot-ID, GCE configuration, Kerf, CPU-return, and guest-agent files retain the full planned set. |
| G2 | `g2/*failure.json` records classified load/start/stop failures; `g2/g2-client-disappearance.txt` records client loss with an active child; state/journal copies before and after daemon `SIGKILL` prove two-child recovery; `g2-admission/g2b-create.json` proves pre-backend `RESOURCE_EXHAUSTED`; `g2-ready-discovery/observation.txt` retains raw `ready` and runtime `CREATED`. |
| G3 | `g3/lifecycle.json` records the safe authentication/framing matrix, non-root UID/GID/groups, exact argv/env/cwd, measured capabilities, exit 23, oversized-frame closure, reconnect, and quiesced shutdown; `g3/shutdown-poweroff.txt` proves the reply/poweroff/loaded transition; the redaction check and empty error logs bound credential/error leakage. |

## Completion accounting

| Scope | Complete | Percent |
| --- | ---: | ---: |
| G0 gate-specific rows | 14 / 14 | 100% |
| G1 gate-specific rows | 15 / 15 | 100% |
| G2 gate-specific rows | 30 / 30 | 100% |
| G3 gate-specific rows | 31 / 31 | 100% |
| Gate-specific total | 90 / 90 | 100% |
| Closing rules, P0 reconciliation, and evidence repair | 18 / 18 | 100% |
| Entire remediation checklist | 108 / 108 | 100% |

These figures measure implemented and evidence-backed checklist work. The
disposable instance and its auto-delete boot disk were deleted, the final cloud
inventory is retained in `resources-after.json`, and all four master gates are
now `pass`.

The per-gate `manifest-g*.json` files map every pass assertion to these raw
files. `final-host-state.log` proves CPUs `0-15`, no Kerf pool/instance, and an
active guest agent before cloud teardown. `resources-before.json` and
`resources-after.json` are the cloud ledger. The pre-existing unattached
`mk-mediated-storage-20260830` disk and three pre-existing snapshots are not
test resources and must remain untouched.

## Findings remediated during the run

- The pinned Kerf/sysfs vocabulary reports a freshly created instance as
  `ready`; the adapter now maps both `ready` and `created` to runtime `CREATED`.
- A controller-owned relay can return to its listen loop after child poweroff;
  `mk-agentctl` now explicitly terminates and reaps every owned relay.
- Evidence initially placed in `/tmp` does not survive a GCE stop/start; G1 and
  G2 were rerun into `$HOME/g0-g3-final-evidence` before download.

## Scope boundary

This evidence closes G0-G3 for the trusted, single-tenant, direct-bundle
boundary. It does not establish hostile-kernel isolation or imply G4 storage,
G5 networking, G6 containerd/`ctr`, Kubernetes, production hardening, or
release readiness.
