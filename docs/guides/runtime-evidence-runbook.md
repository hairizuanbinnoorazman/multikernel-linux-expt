# Runtime evidence runbook

This guide turns a runtime test into reviewable evidence. The normative field
requirements remain in the [evidence contract](../runtime/contracts/evidence.md)
and its JSON Schemas; this guide supplies the operator workflow.

Evidence does not make a broad gate pass merely because a script exited zero.
Claim only the assertions directly supported by retained output, and record
every known limitation.

## 1. Define the run before mutation

Choose one gate or narrowly named feature audit. Record:

- the exact repository commit and whether the checkout is dirty;
- the runtime component versions and revisions;
- host kernel release, boot ID, machine identity, and relevant service states;
- the command sequence and expected assertions;
- known exclusions and unsupported behavior; and
- the expected cleanup state, including intentionally retained cloud assets.

Use a new directory under `evidence/runtime-YYYYMMDD/`. Never reuse or rewrite
a historical run directory.

```bash
run_dir=evidence/runtime-YYYYMMDD/meaningful-run-id
mkdir -m 0700 "$run_dir"
```

## 2. Capture the preflight state

On GCE, capture the project-wide resource ledger before creating or changing
resources:

```bash
python3 scripts/collect-gce-resource-ledger.py \
  "$MK_PROJECT" "$run_dir/resources-before.json"
```

On the test host, retain the read-only qualification report using the exact
proposed pool:

```bash
scripts/capture-evidence-command.py "$run_dir/host-check.log" -- \
  runtime/bin/mk-host-check \
  --kerf=/absolute/path/to/kerf \
  --probe-pool-cpus=8,10,12,14,9,11,13,15 \
  --probe-pool-memory=16GB
```

Also capture component `--version` output, release/deployment inspection,
mount identity, and the initial container, child, network, and firewall
inventories. A preflight failure ends the run; it must not be edited out.

## 3. Capture commands without losing their status

Run every material live command through the capture helper:

```bash
scripts/capture-evidence-command.py "$run_dir/feature-matrix.log" -- \
  bash scripts/test-runtime-g4-g6-feature-matrix.sh
```

The helper exclusive-creates a mode-0600 transcript, streams combined output,
records UTC boundaries and argv, preserves the command exit status, and fsyncs
the result. Use a different output file for each material command.

The helper redacts values of visibly sensitive command-line options. It cannot
recognize secrets printed by the child command. Test scripts must suppress
metadata tokens, registry credentials, agent tokens, private keys, and full
environments at their source. Do not enable shell tracing around secret
material.

## 4. Preserve failures and observations

Keep failed attempts when they explain the final result. Do not normalize,
truncate, or reformat raw transcripts after capture. Terminal output may
legitimately contain CRLF, backspaces, or padding.

For every assertion, retain the actual observed value rather than only a
`PASS` marker. Examples include:

- host and child boot IDs;
- process exit status and signal result;
- before/after terminal dimensions;
- component hashes and image digest;
- storage identity and mount record; and
- initial and final resource counts.

If the harness does not print the value behind an assertion, improve the
harness before treating the run as evidence-grade.

## 5. Clean up and capture the final state

Allow the harness's bounded cleanup trap to run even after a test failure.
Then independently inventory the host. A clean runtime host has no test
containers or tasks, no Multikernel children, no generated TUN links, and no
runtime firewall rules.

For GCE, collect the after ledger only after the intended deletion or retention
actions complete:

```bash
python3 scripts/collect-gce-resource-ledger.py \
  "$MK_PROJECT" "$run_dir/resources-after.json"
```

Every retained instance, disk, snapshot, address, or firewall rule needs an
explicit reason and operator acknowledgment in the manifest. A stopped VM
still has billable disks; absence of running instances is not a complete cloud
cleanup claim.

## 6. Write the manifest and index

Create `manifest.json` using
[`evidence-manifest.valid.json`](../runtime/contracts/fixtures/evidence-manifest.valid.json)
as a structural example and
[`evidence-manifest-v1.schema.json`](../runtime/contracts/schemas/evidence-manifest-v1.schema.json)
as the authority. Record:

- schema version, gate, scoped result, and UTC interval;
- repository commit, dirty state, and the dirty diff when applicable;
- every component identity;
- host and cloud identity;
- every command's argv, exit status, and output path;
- assertions mapped to the exact supporting paths;
- limitations; and
- cleanup result.

Add a short `README.md` that explains the purpose, result, evidence map,
limitations, and resource disposition. Link the run from the nearest dated
evidence index and from [`evidence/README.md`](../../evidence/README.md) when a
new date group is introduced.

## 7. Validate before committing

From the repository root, run:

```bash
python3 scripts/check-runtime-schemas.py
python3 scripts/check-runtime-evidence.py
make docs-check
```

The evidence checker recognizes historical exceptions recorded in
`evidence/runtime-manifest-exceptions.json`. Do not add a new exception merely
to make a current run pass validation; correct or accurately classify the new
manifest.

## Review checklist

- Does the claim match the narrow operation actually exercised?
- Can every assertion be traced to retained raw output?
- Are exit statuses present, including failures?
- Are source and component revisions exact?
- Is dirty source accompanied by the required diff?
- Are secrets and operator identity sanitized without destroying meaning?
- Do before/after ledgers show the complete cloud disposition?
- Are limitations explicit and excluded features not presented as passes?
