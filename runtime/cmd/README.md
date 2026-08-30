# Commands

Planned commands:

- `mk-host-check`: read-only host qualification and JSON report.
- `mkruntimed`: privileged node daemon and sole Kerf mutator.
- `containerd-shim-multikernel-v2`: unprivileged containerd Runtime v2 shim.

Command packages should remain thin. Lifecycle, state, and adapter behavior
belongs in importable internal packages with unit tests.
