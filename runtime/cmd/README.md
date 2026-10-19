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

`mk-agentctl` requires `--token-file=/absolute/private-directory/token`.
The caller owns the0700directory and0600single-link file (64hex digits plus
newline). The file is retained for the lab harness to clean up. Plaintext
`--token-hex` is rejected; process dumpability is disabled before reading.
Run `python3 scripts/test-agentctl-credentials.py runtime/bin/mk-agentctl`
from the repository root as an ordinary Linux user for the bounded process
and authenticated Unix-request check.
