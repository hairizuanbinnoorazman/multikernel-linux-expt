# Sandbox lifecycle and errors (v1)

## Identity

A sandbox ID matches `^[a-z][a-z0-9-]{0,62}$` and is never interpreted as a
path. Each successful create assigns a random 128-bit lowercase hexadecimal
generation. The pair `(sandbox_id, generation)` is the identity used by every
API and transport. Reusing an ID after deletion creates a new generation;
requests carrying an older generation fail with `STALE_GENERATION`.

Every mutating request also carries an idempotency key, 1–128 printable ASCII
characters. The daemon durably records the request fingerprint and result.
Repeating a key with the same operation and input returns the recorded result;
reusing it for different input fails with `IDEMPOTENCY_CONFLICT`.

## States

```text
ABSENT -> ALLOCATING -> CREATED -> LOADED -> RUNNING
                                      ^         |
                                      |         v
                                  STOPPED <- STOPPING
                                      |
                                      v
                                  RELEASING -> ABSENT
```

`CreateSandbox` covers `ALLOCATING -> CREATED`; `LoadSandbox` covers
`CREATED -> LOADED`; `StartSandbox` covers `LOADED|STOPPED -> RUNNING`;
`StopSandbox` covers `RUNNING -> STOPPING -> STOPPED`; and `DeleteSandbox`
covers any non-running durable state through `RELEASING -> ABSENT`. Deleting a
running sandbox first performs the stop transition.

The write-ahead journal records `intent` before external mutation and
`complete` after observation. A crash leaves a visible incomplete operation.
Reconciliation observes Kerf and resumes or records `OPERATOR_ACTION`; it does
not guess that an unobserved resource is safe to release. `ERROR` is metadata
on the last durable state, not a separate state.

## Idempotent results

- Create on an existing ID returns it only when generation and request match.
- Load/Start/Stop at the requested terminal state succeeds without mutation.
- Delete of an absent sandbox succeeds only by exact replay of its durably
  recorded delete result. A new delete key for an absent ID returns `NOT_FOUND`;
  the runtime does not retain a separate unbounded tombstone namespace.
  Unknown external resources are never deleted. This v1 choice keeps absent
  identity bounded while preserving retry safety through idempotent replay.
- One global allocation lock prevents overlapping CPU, memory, port, or image
  assignments. One per-sandbox lock serializes its transitions.

## Error taxonomy

| Code | Meaning | Retry |
| --- | --- | --- |
| `INVALID_ARGUMENT` | Syntax, range, path, or unsupported field | After correction |
| `UNSUPPORTED` | Required OCI/kernel feature is not implemented | No |
| `NOT_FOUND` | Known resource does not exist | After discovery |
| `ALREADY_EXISTS` | Conflicting live identity | No |
| `STALE_GENERATION` | ID is valid but generation is obsolete | Rediscover |
| `IDEMPOTENCY_CONFLICT` | Key was reused with different input | New key |
| `FAILED_PRECONDITION` | State or host cannot perform operation | After state change |
| `RESOURCE_EXHAUSTED` | Safe allocation is unavailable | After release |
| `BACKEND_TIMEOUT` | Kerf/agent exceeded a bounded deadline | Reconcile first |
| `BACKEND_FAILURE` | Kerf/agent returned invalid or failed output | Reconcile first |
| `UNAUTHENTICATED` | Transport proof is absent or wrong | No |
| `OPERATOR_ACTION` | Ownership/outcome cannot be proved safely | Manual review |
| `INTERNAL` | Runtime invariant failed | Reconcile and investigate |

Errors include a stable code, safe message, operation ID, and retryable flag.
Messages and logs never include auth tokens, OCI credentials, or environment
values named as secrets.
