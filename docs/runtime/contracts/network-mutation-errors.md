# Network mutation error replies

Source `9362d5a4` removes backend and persistence error text from network mutation
`APIError.Message`. External command output, paths and credentials must not be
formatted into replies. This is a producer rule, not pattern-based sanitization.

| Failure stage | Code | Retryable |
| --- | --- | --- |
| ADD replay, CHECK or BIND backend check | `FAILED_PRECONDITION` | yes |
| Address allocation exhaustion | `RESOURCE_EXHAUSTED` | no |
| Generation, namespace creation/identity, endpoint creation | `INTERNAL` | yes |
| Allocation/readiness/deletion/binding/unbinding/counter persistence | `INTERNAL` | yes |
| Endpoint/namespace cleanup or allocation record removal | `INTERNAL` | yes |
| ATTACH device opening | `INTERNAL` | yes |

Each reply uses a fixed stage description. ADD failures after durable allocation
append either `allocation rolled back` or `rollback incomplete; reconciliation
required`. The latter applies when backend cleanup or durable record removal
fails; the existing allocation generation is retained for reconciliation.
No additional cleanup, retry or state transition is introduced by error formatting.
Codes, retry flags, owner checks and stale-generation rejection remain unchanged.
ATTACH opening failure still preserves its successful binding and omits the
endpoint from the response.

[Local proof](../../../evidence/runtime-20261104/g8-network-mutation-controls/README.md)
retains failing-before/fixed-after controls, network race tests, complete runtime
race tests and required checks. The synthetic payload is deliberately repeated
and includes a private path; assertions inspect API JSON and actual ATTACH wire
output without logging the payload. Rollback failure/reconciliation controls
check that bounded replies do not conceal ownership or change recovery. The
[GCE private fault fixture](../../../evidence/runtime-20261104/g8-network-mutation-gce/README.md)
passes for the same source with unchanged primary services and transport.

Internal reconciliation errors, other daemon/Task RPC producers, flag/inspector
output, aggregate pressure and complete G7/G8 acceptance retain separate scope.
This change does not establish containment after arbitrary child-kernel compromise.
