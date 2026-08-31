# Plan 10: Final build and developer-preview release

## Purpose

Turn the gated prototype into a reproducible, reviewable developer preview.

## Build outputs

- `mkruntimed`;
- `containerd-shim-multikernel-v2`;
- static `mk-agent`;
- host qualification tool;
- child kernel and initramfs manifest;
- configuration examples;
- containerd runtime fragment;
- Kubernetes `RuntimeClass` and deployment assets;
- checksums, SBOMs, licenses, and provenance; and
- an uninstall/cleanup command that defaults to inspection before mutation.

## Release checks

- Rebuild from a clean checkout using pinned dependencies.
- Verify artifacts on a fresh qualified host, not a development VM snapshot.
- Run unit, contract, privileged integration, containerd, Kubernetes, failure,
  and cleanup suites.
- Confirm no test depends on developer home-directory state.
- Verify upgrade and downgrade of daemon state formats or explicitly reject
  unsupported transitions before mutation.
- Test installation alongside ordinary `runc` without changing the default
  runtime.
- Validate documentation commands and configuration from scratch.
- Review retained GCE resources and costs.

## Release documentation

- Supported host/kernel/Kerf matrix.
- Supported OCI feature matrix.
- Storage and network limitations.
- Resource-allocation rules.
- Recovery and troubleshooting guide.
- Security and trusted-workload statement.
- Known failures and performance results.

## Gate G10

Pass when a fresh operator can install the preview, run and remove a containerd
and Kubernetes workload, recover from documented failures, and uninstall it
without leaked resources by following only the released documentation.

The preview must remain opt-in and must not become the node's default runtime.
