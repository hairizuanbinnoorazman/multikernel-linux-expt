# G0 learnings: scope and contracts

## Result and current status

The 2026-08-31 work remains a **provisional historical G0 milestone**. The
2026-09-04 implementation and evidence pass closes G0, including the
post-deletion cloud ledger. The v1 contract set is under
[`../contracts/`](../contracts/README.md), with strict schemas for sandbox
and host configuration, approved kernel artifacts, daemon and agent envelopes,
and evidence manifests. The 2026-08-31 remediation added the missing schema
coverage and executable fixtures. The 2026-09-04 implementation pass completes
the G0 contract mappings, strict OCI boundary, approved bootstrap manifest, and
transport qualification. The schema-valid
[final G0 manifest](../../../evidence/runtime-20260904/g0-g3-final/manifest-g0.json)
supersedes the historical manifest without rewriting its result.

Go was selected for `mk-host-check`, `mkruntimed`, and `mk-agent`. Existing C
transport code remains appropriate at the AF_VSOCK boundary; the G3 run later
made that boundary a compatibility requirement rather than merely a migration
convenience.

The frozen MVP uses a runtime-owned kernel/initramfs/agent and a
containerd-owned OCI bundle/rootfs. It is `linux/amd64`, trusted-workload only,
and requires fail-closed handling for OCI fields not in the advertised feature
set. Sandbox identity is `(id, random generation)` and every mutation has an
idempotency key. Error, ownership, lifecycle, configuration, authentication,
kernel selection, and cloud evidence rules are explicit. The later G4-G6
field-loss defect has now been removed: bundle input is validated before
allocation and exec input is validated before translation, so neither adapter
can silently discard unsupported OCI controls.

## License audit

No upstream code was copied into the Go implementation. Linux-derived kernel
and module work remains GPL-2.0; Kerf v0.2.0 is Apache-2.0; containerd and Kata
concepts reviewed for interface design are Apache-2.0. The new C relay carries
GPL-2.0-or-later consistently with the existing transport helpers. A future
release still needs a generated dependency/SBOM audit.

That deferred work is explicitly assigned to G8's dependency/license inventory
and G10's release SBOM, license, and provenance artifacts. It has not been
counted as completed by G0.

## Revalidation proof (2026-08-31)

The following commands were rerun from the repository root in this remediation
session:

The complete local command was rerun again on 2026-09-03 against commit
`4cfca4e` plus the documented dirty evidence-harness changes. Schema fixtures,
documentation links, the Go race suite, `go vet`, and shell syntax all passed.
The replacement [G0 manifest](../../../evidence/runtime-20260903/g0-g3-proof/manifest-g0.json)
is schema-valid and deliberately remains `provisional`. The continuing
[G0 remediation manifest](../../../evidence/runtime-20260903/g0-g3-remediation/manifest-g0.json)
indexes the synchronized agent schema/reply policy and the expanded 20-case
fixture plus full race/vet/docs/shell verification. The later strict 25-case
OCI validation and approved-bootstrap validator close the adapter and artifact
qualification gaps locally.

The 2026-09-04 final run retained the exact approved host/kernel manifests,
component hashes, pinned Multikernel and Kerf commits, transport patch, module
name/vermagic, and complete local validation transcript in the
[final evidence index](../../../evidence/runtime-20260904/g0-g3-final/README.md).

| Command | Result | What it proves |
| --- | --- | --- |
| `python3 scripts/check-runtime-schemas.py` | `PASS (6 schemas, 20 cases)` | Every schema is a valid Draft 2020-12 schema; positive fixtures pass; unknown fields/methods, duplicate fields, overflow, relative/unsafe paths, malformed digests, missing pins, invalid labels, invalid reply authentication fields, and missing evidence exit status fail. |
| `make docs-check` | `PASS` | Repository verification now includes schema fixtures as well as required paths and local links. |
| `cd runtime && GOCACHE=/tmp/mk-go-cache go test ./...` | `PASS` | All runtime packages pass, including `protocol.TestStrictDecode` for unknown fields, nested/top-level duplicates, and trailing values. |
| `make docs-check && (cd runtime && GOCACHE=/tmp/mk-go-cache go test -race ./... && GOCACHE=/tmp/mk-go-cache go vet ./...) && while ...; do bash -n "$script"; done` | `PASS` | One invocation passed schema/link checks, the full Go race suite, `go vet`, and syntax checks for every Bash/sh-shebang file under `scripts/` and `guest/`. |
| `git ls-remote` for Multikernel `v7.0-mk2` and Kerf `v0.2.0` | exact peeled commits matched | Multikernel still resolves to `3bdd35b64413da0b4e089ce931bfc2e8b031cbf7`; Kerf still resolves to `8b72b3e9b266f8d32e707e2c1743ad7afc50b1ec`. |
| `git ls-remote https://github.com/multikernel/daxfs.git` | `HEAD` and `refs/heads/main` matched | DAXFS still resolves to `11ab401585b79b4a7c9164019852e0219e197d13`. |

The fixtures are indexed by
[`schema-cases.json`](../contracts/fixtures/schema-cases.json). Duplicate JSON
object names are rejected at parse time by both the fixture validator and the
shared Go `protocol.StrictDecode` function; schema validation is not relied on
to detect a representation that JSON Schema cannot distinguish after parsing.

## Normative contract coverage

This matrix annotates every normative section in the frozen contract set. An
assigned gate is not a pass claim: `open` means the requirement remains an exit
condition for that gate.

| Contract section | Implementing gate(s) | Status after this session |
| --- | --- | --- |
| Lifecycle identity and idempotency | G2, G6 | G2 contract implemented; generation, validation, exact replay, and absent-delete semantics have focused coverage. Broader clients remain G6 work. |
| Lifecycle states, journaling, reconciliation, and locks | G2, G8 | G2 contract implemented with full intent/checkpoint reconciliation; hostile fault campaigns remain G8 work. |
| Error taxonomy and secret-safe errors | G2, G3, G8 | G0-G3 boundary complete: stable operation IDs, backend-timeout classification, structured agent errors, focused leakage tests, and the safe live negative matrix pass. Broader hostile campaigns remain G8 work. |
| Ownership table: containerd/shim inputs | G6 | Future gate. |
| Ownership table: daemon/Kerf resources and protected host devices | G1, G2, G8 | G1/G2 boundary implemented; device allocation is absent and strict configuration prohibits protected-controller input. Adversarial proof remains G8 work. |
| Ownership table: agent/process/cgroups | G3, G8 | G3 process ownership/limits implemented; OCI cgroup application remains explicitly mandatory at G6. |
| Ownership table: writable roots/storage | G4 | Future gate. |
| Ownership table: cloud ledgers | Every live gate, audited again at G10 | Replacement proof and continuing remediation have before/after cleanup ledgers; both disposable VMs and their auto-delete disks are gone. |
| Threat statement and isolation boundary | G1, G8 | G1 pinned-source audit and resource matrix complete; hostile-workload hardening remains G8. |
| Agent authentication/session binding | G3, G8 | G3 implementation, focused tests, and the safe live negative/reconnect matrix pass; adversarial campaigns remain G8 work. |
| Kernel/image ownership and approved-manifest validation | G2, G3, G4, G6 | Strict production manifest resolution verifies artifacts, versions, relay, module, and exact transport roles before allocation. Image-root production work remains G4/G6. |
| OCI supported/rejected fields | G3, G6, G8 | G3 direct-agent and adapter boundaries fail closed with a 25-case matrix; expanded production support remains G6. |
| Host and sandbox configuration | G0 schema; G1/G2 enforcement | Strict root ownership, safe parents, daemon loading, socket/state permissions, and field bounds are implemented. |
| Daemon/agent wire envelopes and method set | G0 schema; G2/G3 implementation; G6 evolution | Schemas cover the implemented v1 method set and structured replies; remaining method semantics are gate work. |
| Evidence manifest, resource ledgers, and redaction | Every gate; G10 release audit | Separate schema-valid G0-G3 manifests, indexed raw evidence, checksums, cloud ledgers, and exact-token redaction proof exist; release-wide audit remains open. |

## Contract drift found by the 2026-09-02 synchronization audit

The schema and policy were synchronized with the implemented v1 state,
stdin/output, terminal-resize, network, and structured-reply behavior. The
first cross-gate issue found by that audit is now resolved:

- the G6 image builder performs strict 25-case subset validation before
  allocation and rewrites only `root.path`; the exec adapter has a focused
  unsupported-process matrix; and
- `make docs-check` runs those OCI validation tests in addition to schema
  fixtures, but it still does not validate every committed evidence
  manifest, so non-conforming historical manifests remain undetected by the
  default target.

Historical manifest migration remains release-audit work; it does not change
the frozen G0 evidence schema or the now-enforced OCI policy.

## Key learning

G0 prevented three live findings from leaking into higher layers as accidental
behavior: Kerf can commit despite a nonzero exit, transport direction matters,
and EOF/half-close is not a safe Multikernel AF_VSOCK framing contract.
