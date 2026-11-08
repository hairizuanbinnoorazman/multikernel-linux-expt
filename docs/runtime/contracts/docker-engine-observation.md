# Child Docker API and launcher observation

`mk-agent --docker-guest` advertises `docker-engine-observation-v1` only when
its production supervisor has completed startup and the read-only observer is
installed. Ordinary agents do not advertise it. `InspectDockerEngine` accepts
an empty request through the existing authenticated identity/generation/endpoint
and increasing-sequence protocol. It has no command, path, PID, cgroup or Docker
endpoint argument. Unknown request fields refuse before probing.

The version-1 result has exactly `version`, `source` and `status`, with source
`owned-child-docker-api-and-launcher`. The statuses are:

| Status | Observed condition |
| --- | --- |
| `responding` | Held launcher pidfd was live before and after fixed API probes, with validated rootless profile and owned live socket peers |
| `unavailable` | No launched supervisor authority, unverifiable launcher identity, or API/peer/profile observation failed |
| `exited` | Held launcher has exited, including before log reaping completes |
| `stopping` | A stop has begun and cleanup ownership remains retained |
| `stopped` | Supervisor cleanup completed |

The supervisor shares its context-aware stop gate with observation. A 750 ms
budget includes queue admission and both API requests. It probes only the socket
captured at trusted launch, with no proxy/TCP/host-socket fallback. Existing
UID-1000, `SO_PEERPIDFD`, owned cgroup, bounded HTTP header/body, redirect refusal,
Linux/vfs/rootless data-directory/profile and final cancellation checks remain.
The held launcher pidfd is polled directly: a still-open log-reaping channel is
not proof of process liveness. Context failure returns no successful report.
Synchronous kernel/filesystem operations retain their existing latency limits.

The agent refuses observations while its quiescence gate is busy or after
quiescence. It caps callback execution cooperatively at 750 ms and validates
returned classifications and cancellation before publishing. Errors and reports
contain no engine output, paths, tokens, raw errors, IDs or resource inventories.
The callback does not stop, restart, adopt, repair or release anything. Scope,
mount and pidfd authority remain owned by normal teardown.

`Client.InspectDockerEngine(ctx)` negotiates the capability and validates the
strict bounded result over the caller's existing connection under one five-second
budget. It never reconnects/replays or executes a workload command as fallback.
Existing frame and sequence failure rules continue to apply.

Operators with an already approved private relay and credential can use:

```sh
mk-agentctl --inspect-docker-engine \
  --sandbox-id="$SANDBOX_ID" --generation="$GENERATION" --port="$AGENT_PORT" \
  --unix-socket="$APPROVED_RELAY_SOCKET" --token-file="$PRIVATE_TOKEN_FILE"
```

This mode performs only negotiation and inspection, exits zero for `responding`,
and exits one for other observed states or failure. Unsupported/malformed/failed
observations produce no success JSON. It is mutually exclusive with `--auth-matrix`.
It has no connection retry loop and an overall five-second caller budget. The
existing private token-file requirements are unchanged; never put tokens in argv.

This is a sampled API/profile/launcher observation, not a reservation or a claim
that a build will succeed. It does not measure storage writability/free bytes,
network reachability, BuildKit progress or rootless engine honesty after compromise.
It does not change doctor v8, diagnostic bundle v2, startup admission or the
installed host service defaults. Real child Docker builds, fault/churn cases,
full readiness/admission and complete G7/G8 require separate evidence.

Local tests include strict decoding/refusal, authenticated negotiation, foreign
identities, replay, argument refusal, quiescence, callback errors/redaction,
queue cancellation, unreaped launcher death and CLI success/failure behavior.
Installed opt-in tests exercise real UID-1000 sockets and owned cgroups with a
synthetic API, API degradation/recovery, foreign socket replacement, authenticated
quiescence and release. They do not run real Docker or prove child-kernel health.

Direct child kernels require no nested virtualization. Arbitrary child-kernel
compromise shares primary trust; this observation introduces no containment claim.

[Local controls](../../../evidence/runtime-20261108/g8-engine-observation-controls/README.md)
and [native GCE synthetic controls/mutation/preservation](../../../evidence/runtime-20261108/g8-engine-observation-gce/README.md)
pass. Actual Docker and child-kernel observations remain unqualified by these tests.

The corrected [committed-source real-child GCE trial](../../../evidence/runtime-20261108/g8-live-engine-observation-aa173749/README.md)
now qualifies responding samples at filesystem-full, original unfinished build
and built-image holds, then refusal of the removed owner. Independent audit
also verifies the full build matrix, primary parity, native release/resource
return and five live epoch/source/build refusals while preserving the earlier
failed attempt. This closes that sampled engine/build-lifecycle subcase only;
post-trial cold restoration, broader readiness/admission and complete G7/G8
remain separate requirements.

Its [separate cold-restoration proof](../../../evidence/runtime-20261108/g8-live-engine-observation-aa173749-cold-restoration/README.md)
also passes with original idle transport, returned resources, selected artifact
and failed-history preservation, archived frames confined to their original
epoch, old completion refused on the new boot and a fresh primary baseline.
Aggregate doctor/readiness integration and full G7/G8 remain open.
