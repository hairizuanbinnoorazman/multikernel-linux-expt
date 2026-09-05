# Commands

Planned commands:

- `mk-host-check`: read-only host qualification and JSON report.
- `mkruntimed`: privileged node daemon and sole Kerf mutator.
- `mknetd`: privileged primary-owned endpoint, route, and firewall authority.
- `mk-cni`: CNI v1.0.0 adapter installed under the network type name
  `multikernel`.
- `containerd-shim-multikernel-v2`: unprivileged containerd Runtime v2 shim.

Command packages should remain thin. Lifecycle, state, and adapter behavior
belongs in importable internal packages with unit tests.
