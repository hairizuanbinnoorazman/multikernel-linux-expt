# `mkruntimed`

Privileged node daemon planned by
[`../../../docs/runtime/plans/02-control-plane.md`](../../../docs/runtime/plans/02-control-plane.md).

This command is the sole runtime owner of Kerf mutations. Keep command-line and
service setup here; lifecycle and adapter logic belongs under `internal/`.
