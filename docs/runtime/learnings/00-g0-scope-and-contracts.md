# G0 learnings: scope and contracts

## Result and current status

The 2026-08-31 work is retained as a **provisional G0 milestone**, not a closed
gate. The v1 contract set is under
[`../contracts/`](../contracts/README.md), with strict schemas for sandbox
and host configuration, approved kernel artifacts, daemon and agent envelopes,
and evidence manifests. The 2026-08-31 remediation added the missing schema
coverage and executable fixtures, but G0 remains provisional until its open
implementation mappings and replacement per-gate evidence are complete.

Go was selected for `mk-host-check`, `mkruntimed`, and `mk-agent`. Existing C
transport code remains appropriate at the AF_VSOCK boundary; the G3 run later
made that boundary a compatibility requirement rather than merely a migration
convenience.

The frozen MVP uses a runtime-owned kernel/initramfs/agent and a
containerd-owned OCI bundle/rootfs. It is `linux/amd64`, trusted-workload only,
and requires fail-closed handling for OCI fields not in the advertised feature
set. Sandbox identity is `(id, random generation)` and every mutation has an
idempotency key. Error, ownership, lifecycle, configuration, authentication,
kernel selection, and cloud evidence rules are explicit. Later G4-G6 code does
not yet satisfy every one of those rules: the image builder and exec translation
can discard unsupported OCI fields before the agent validates them.

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

| Command | Result | What it proves |
| --- | --- | --- |
| `python3 scripts/check-runtime-schemas.py` | `PASS (6 schemas, 15 cases)` | Every schema is a valid Draft 2020-12 schema; positive fixtures pass; unknown fields, duplicate fields, overflow, relative/unsafe paths, malformed digests, missing pins, invalid labels, and missing evidence exit status fail. |
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
| Lifecycle identity and idempotency | G2, G6 | Partial; core generation/replay exists, validation and tombstones remain open. |
| Lifecycle states, journaling, reconciliation, and locks | G2, G8 | Partial; crash windows and documented intermediate states remain open. |
| Error taxonomy and secret-safe errors | G2, G3, G8 | Partial; operation IDs, timeout classification, agent structured errors, and redaction tests remain open. |
| Ownership table: containerd/shim inputs | G6 | Future gate. |
| Ownership table: daemon/Kerf resources and protected host devices | G1, G2, G8 | Partial; allocation-path controller enforcement remains open. |
| Ownership table: agent/process/cgroups | G3, G8 | Partial; full process semantics and limits remain open. |
| Ownership table: writable roots/storage | G4 | Future gate. |
| Ownership table: cloud ledgers | Every live gate, audited again at G10 | Open for replacement G0-G3 evidence. |
| Threat statement and isolation boundary | G1, G8 | Partial; pinned-source audit and resource matrix remain open. |
| Agent authentication/session binding | G3, G8 | Partial; fresh randomness, reconnect, reply integrity, and live negative matrix remain open. |
| Kernel/image ownership and approved-manifest validation | G2, G3, G4, G6 | Schema complete; production manifest resolution and architecture checks remain open. |
| OCI supported/rejected fields | G3, G6, G8 | Partial; see the G3 feature matrix and open fail-closed tests. |
| Host and sandbox configuration | G0 schema; G1/G2 enforcement | Schema complete; root ownership, safe parents, and daemon loading remain open. |
| Daemon/agent wire envelopes and method set | G0 schema; G2/G3 implementation; G6 evolution | Envelope schemas complete; method semantics and missing methods remain open. |
| Evidence manifest, resource ledgers, and redaction | Every gate; G10 release audit | Schema complete; historical G0-G3 evidence is non-conforming and replacement runs remain open. |

## Contract drift found by the 2026-09-02 synchronization audit

The schema work completed the original G0 artifact set, but later runtime work
evolved beyond parts of frozen v1 without updating the contracts together:

- the current agent implements stdin, attach-oriented output reads, terminal
  resize, process state, and static-network methods that are not represented in
  the v1 agent-protocol schema's method enum;
- the kernel/image policy still lists `terminal=false` as the G3 boundary even
  though terminal support was subsequently implemented and live-tested for
  initial-size propagation;
- the G6 image builder and exec adapter select a supported OCI subset instead
  of rejecting every unsupported caller field, contrary to the fail-closed
  contract; and
- `make docs-check` validates schema fixtures but not every committed evidence
  manifest, so non-conforming historical manifests remain undetected by the
  default target.

These are open G0/G3/G6 contract-evolution items. Closing them requires either
a compatible schema/policy revision with tests and migration notes or code that
conforms to the existing v1 contracts; documentation alone must not normalize
the mismatch.

## Key learning

G0 prevented three live findings from leaking into higher layers as accidental
behavior: Kerf can commit despite a nonzero exit, transport direction matters,
and EOF/half-close is not a safe Multikernel AF_VSOCK framing contract.
