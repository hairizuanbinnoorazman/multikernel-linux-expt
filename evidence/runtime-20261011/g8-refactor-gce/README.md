# G8 shim-layout refactor: GCE validation

This directory records the portable validation of the first G8 maintainability
slice on a fresh disposable Google Compute Engine instance. It does not claim a
privileged Multikernel lifecycle or service-hardening pass.

## Environment

- Project: `new-demo-project-462517`
- Zone: `asia-southeast1-b`
- Instance: `mklinux-g8-refactor-20261011`
- Machine type: `e2-standard-4`
- Image family: `ubuntu-2404-lts-amd64`
- Toolchain: copied Go `1.26.1` artifact, verified before use
- Source archive SHA-256:
  `c083d34d3b022004d0470256f566e26404a2fbb44d25270fa95398f902f33a4f`

The exact resolved host and toolchain identity is in
[`mklinux-g8-host.txt`](mklinux-g8-host.txt). The pre-cleanup Compute Engine
description is retained as [`instance-before-delete.json`](instance-before-delete.json).

## Results

- `go test -race ./...`: pass; see
  [`mklinux-g8-go-test-race.log`](mklinux-g8-go-test-race.log).
- `go vet ./...`: pass; see [`mklinux-g8-vet.log`](mklinux-g8-vet.log).
- `make docs-check`: pass after installing its required `rg` command; see
  [`mklinux-g8-docs-check.log`](mklinux-g8-docs-check.log).
- `make runtime-build`: pass; see
  [`mklinux-g8-build.log`](mklinux-g8-build.log).

## Cleanup

The instance was deleted after evidence collection. Its boot disk used
`autoDelete=true`. Both the exact-name instance and disk inventories are empty
in [`instance-after-delete.json`](instance-after-delete.json) and
[`disk-after-delete.json`](disk-after-delete.json).
