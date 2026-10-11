# Plan 09: Compatibility contract and release engineering

## Purpose

Freeze a narrow standalone support contract and make every supported artifact
reproducible before performance qualification or release validation.

## Compatibility contract

- Publish the exact supported Multikernel kernel, Kerf, containerd, Docker,
  OCI runtime-spec, CNI, host distribution, machine-family, and storage matrix.
- Test the selected containerd and Docker versions through the same lifecycle,
  restart, I/O, and cleanup assertions.
- Publish supported and rejected OCI fields, mounts, namespaces, resource
  controls, and writable-state semantics.
- Keep checkpoint/restore explicitly unsupported for the standalone preview.
- Decide whether runtime resource `Update` is required. If it remains out of
  scope, document resources as immutable after sandbox creation and retain the
  fail-closed behavior.
- Require explicit runtime selection and preserve the host's ordinary `runc`
  default.

## Build and automation

- Add continuous integration for unit, race, vet, schema, documentation,
  deployment, reproducibility, and evidence-auditor tests.
- Keep privileged qualified-host and cloud suites separately selected, cost
  bounded, revision pinned, and evidence producing.
- Generate deterministic release archives, checksums, dependency and license
  inventories, an SBOM, and build provenance.
- Add a top-level license, security policy, support statement, and changelog.
- Test fresh install, upgrade, rollback, inactive-generation removal, dry-run
  uninstall, applied uninstall, and refusal of unsafe state transitions.

## Gate G9

Pass when the supported compatibility matrix is explicit and green, unsupported
behavior fails closed, CI protects every unprivileged gate, and one clean
revision reproducibly produces all standalone release artifacts and metadata.
