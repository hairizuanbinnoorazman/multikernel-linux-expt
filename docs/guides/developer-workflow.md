# Developer workflow

This guide covers unprivileged local development of the Multikernel runtime.
Local tests should not require root, GCE, Kerf, or a Multikernel-enabled
kernel. Privileged and billable tests are separate, explicit workflows.

## Prerequisites

The runtime module declares Go 1.26. Repository checks also use Bash, Python 3,
and the Python `jsonschema` package. A C compiler is needed for the helper tools
but not for the default Go unit suite.

Confirm the available entry points from the repository root:

```bash
make help
go version
python3 --version
```

## Repository map

- `runtime/cmd/` contains thin executable entry points.
- `runtime/internal/` contains lifecycle, state, Kerf, storage, and network
  implementations.
- `runtime/agent/` and `runtime/protocol/` contain the child service and shared
  wire contracts.
- `docs/runtime/contracts/` contains normative contracts and schemas.
- `docs/runtime/plans/` describes future gates; it is not proof that a feature
  is implemented.
- `docs/runtime/learnings/` records demonstrated behavior and known gaps.
- `scripts/` contains local checks and explicitly selected privileged harnesses.
- `evidence/` contains immutable historical output.

Preserve the dependency direction documented in
[`runtime/README.md`](../../runtime/README.md): the shim uses daemon and network
interfaces, `mkruntimed` owns lifecycle policy, and protocol packages do not
depend on executable packages.

## Fast local loop

Run the default unit and contract suite:

```bash
make runtime-test
```

During focused development, run the affected package directly:

```bash
cd runtime
GOCACHE=/tmp/mk-go-cache go test ./internal/lifecycle
GOCACHE=/tmp/mk-go-cache go test ./cmd/containerd-shim-multikernel-v2
```

Before handing off a runtime change, run the broader local checks:

```bash
cd runtime
GOCACHE=/tmp/mk-go-cache go test -race ./...
GOCACHE=/tmp/mk-go-cache go vet ./...
cd ..
make runtime-build
make docs-check
```

`make docs-check` is broader than a link checker: it validates schemas,
fixtures, OCI and rootfs policy, storage validation, deployment managers,
release manifests, GCE ledgers, evidence capture, containerd configuration,
and evidence audits.

## Contracts and schemas

When changing a wire format or configuration:

1. update the normative Markdown contract;
2. update the corresponding JSON Schema;
3. add or adjust both accepted and rejected fixtures;
4. update Go parsing and validation;
5. add focused unit or contract tests; and
6. run `python3 scripts/check-runtime-schemas.py` plus `make docs-check`.

Parsing is strict. Unknown fields, duplicate JSON names, unsafe relative
paths, overflow, and unsupported OCI fields must continue to fail closed.
Avoid broadening a schema merely because one client emitted an unexamined
field.

## Build identity and manifests

Normal dirty-tree builds use revision `unknown`. An evidence or installation
build must use a clean checkout and explicit identity:

```bash
runtime_revision=$(git rev-parse HEAD)
test -z "$(git status --porcelain --untracked-files=normal)"
make runtime-manifest \
  RUNTIME_VERSION=0.1.0-dev \
  RUNTIME_REVISION="$runtime_revision"
```

The generated manifest is exclusive-create. Preserve an existing manifest
instead of overwriting it. Each binary's `--version`, size, and SHA-256 digest
must agree with the manifest.

## Adding or changing a command

Keep command packages thin: flags, service setup, dependency construction, and
exit reporting belong there; reusable behavior belongs in an importable
package. Update `runtime/cmd/README.md`, relevant deployment units, and the
top-level build if a new installed component is introduced.

Every new mutation path needs tests for validation-before-mutation, bounded
timeouts, idempotency or generation handling, rollback, restart
reconstruction, and cleanup. Privileged integration coverage supplements these
tests; it does not replace them.

## Documentation placement

- Put repeatable operator procedures in `docs/guides/`.
- Put completed experimental interpretation in `docs/experiments/`.
- Put normative runtime behavior in `docs/runtime/contracts/`.
- Put future gate design in `docs/runtime/plans/`.
- Put implementation observations and remediation audits in
  `docs/runtime/learnings/`.
- Put raw output and a manifest in a new immutable `evidence/` run directory.

Use repository-relative links and run `make docs-check` after moving or adding
Markdown files.

## Privileged and GCE tests

The runtime harnesses under `scripts/test-runtime-g*.sh` use sudo, mutate host
state, and may require a disposable GCE instance. Do not run them as part of a
normal local loop.

Before a live run:

- read the script completely;
- use an otherwise idle qualified host;
- confirm serial recovery and cloud billing scope;
- capture a project-wide before ledger;
- use the [evidence runbook](runtime-evidence-runbook.md); and
- confirm the script has bounded cleanup on both success and failure.

The shared feature matrix is available through `make runtime-g4-g6-matrix`.
The recovery audit deliberately restarts services and kills a shim worker and
must be invoked directly as `scripts/test-runtime-recovery.sh`.

## Change review checklist

- Does validation happen before host mutation?
- Is resource ownership unambiguous and generation-bound?
- Are error paths bounded and cleanup attempted?
- Are unsupported operations rejected explicitly?
- Do logs avoid secrets and authentication tokens?
- Do local tests cover success, rejection, rollback, and reconstruction?
- Does documentation distinguish implemented, live-proven, and planned work?
