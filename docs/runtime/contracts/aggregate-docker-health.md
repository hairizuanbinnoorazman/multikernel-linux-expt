# Aggregate Docker-mode health observations

`mkruntimed --doctor-all --doctor-format=9 --engine-targets=/private/targets.json`
adds owner-bound child engine samples to the complete v8 operational health
observation. `--diagnostic-bundle=/private/new.json --diagnostic-format=3` with
the same target flag retains that report and the compiled component view through
the existing private, exclusive, bounded diagnostic publisher. Versions 5–8 and
bundles 1–2 keep their existing scopes and defaults.

This is an opt-in Docker deployment mode: every RUNNING lifecycle owner must
support the existing owning-shim engine observation. Ordinary non-Docker or older
agents refuse as unsupported/inconclusive rather than being silently excluded.
Non-ABSENT owners in other states require rechecking. Matching observations do
not establish admission, current storage writability, atomicity, process/kernel
incarnation, compatibility or hostile-kernel containment.

The operator supplies one private snapshot mapping current owners to their
existing protected shim sockets, using trusted current runtime/containerd state.
No socket discovery, legacy serialization or credential transfer is performed.
The file is caller-owned mode 0600, single-link, in a caller-owned mode-0700
parent with descriptor-anchored no-symlink traversal. Its canonical absolute path
and one-MiB/256-target bounds are enforced. Unknown/duplicate/case-variant keys,
null lists, unsafe sockets, invalid owner tuples and duplicate sandbox entries
refuse. A valid empty list covers only a lifecycle with no RUNNING owners.

```json
{"version":1,"targets":[{"socket":"unix:///private/current-shim.sock","owner":{"version":1,"namespace":"moby","container_id":"current-container-id","sandbox_id":"current-sandbox-id","generation":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}]}
```

Replace the example values with the actual current tuple and socket. The map is
input, not lifetime authority: exact sandbox/generation coverage is checked
against the complete lifecycle before any engine query, and each actual shim
matches the full namespace/container/sandbox/generation tuple. Missing, extra,
retired or stale owners fail closed. An administrative snapshot that becomes
stale must be replaced by a fresh private snapshot; inspection never repairs it.

One original caller context includes map loading, complete initial v8 inspection,
two complete engine sample sets, and a complete final v8 inspection. Each socket
query additionally caps itself at five seconds. No request reconnects or replays.
Any observed health/ownership/engine drift suppresses all payloads and yields
inconclusive. Unsupported/unavailable/invalid observations retain fixed finding
codes without raw errors. Stable nonresponding engine states yield degraded.
Aggregate failure never exposes usable CPU or memory candidates; observers and
shared component snapshots are not mutated.

Version 9 reports scope
`operational-health-and-all-running-child-engine-observations`, a separate nested
v8 report, redacted engine rows (sandbox ID/generation and fixed observation),
and `all_running_owners_observed`. Target socket paths, namespace/container
names and credentials are excluded. The nested v8 report keeps its narrower
scope and unobserved list. The outer list removes Docker engine only after all
running owners have been observed in matching samples. Admission remains false.
A complete idle report carries an empty engine array, never an invented sample.

Bundle v3 uses scope
`operational-health-child-engine-and-component-observation-snapshot`; its typed
collector rejects incomplete coverage or forged aggregate status. The existing
8-MiB cap, exclusive synced mode-0600 publication, optional directory budgets,
redaction exclusions and SHA-256/byte-count receipt still apply. A captured
inconclusive/degraded diagnostic is evidence, not a readiness success.

Local controls cover coverage, drift, cancellation, callback snapshot mutation,
redaction, capacity suppression, target file safety and production RPC idle
coverage. Installed CLI/native and actual child-build qualification are separate
requirements; this implementation does not close complete G7/G8. Direct child
kernels require no nested virtualization and share primary trust after arbitrary
child-kernel compromise.

## Receipt-bound real-child pressure trial

The ordinary pressure fixture can opt in with `--aggregate-docker-health` and
`--engine-observation`. Its guarded module-trial parent requires
`MK_MODULE_TRIAL_AGGREGATE_DOCKER_HEALTH=1` together with
`MK_MODULE_TRIAL_ENGINE_OBSERVATION=1`. Restart, concurrency and native-record
fault scenarios do not qualify through this mode.

At the full-filesystem, built-image and released holds, the fixture writes a
fresh private target snapshot from its exact Docker container/owner and saved
protected shim address. Active holds require the one running owner; the released
hold requires no running owners. The existing v8/v2 pair and added v9/v3 pair
share the original five-second health deadline and original hold deadline. The
old pair retains its own two-report timing, and a separate four-report timing
records the entire sequence with exactly the same start and deadline. Neither
timeout nor inconclusive/degraded output can publish a successful checkpoint.
Command streams are retained privately as exclusive synced mode-0600 files;
stdout is capped at one MiB and stderr at 4096 bytes. Failed commands retain
fixed failure/truncation metadata with `qualified=false`, without publishing
a successful phase timing. These streams are diagnostics, not gate evidence.

Collection also enforces these limits while reading both pipes, retaining at
most one extra overflow byte per stream in memory. It never collects an entire
unbounded stream before truncating it. Overflow refuses immediately, preserving
only observed truncation flags; unread output is not claimed as captured. EOF
does not grant a new process-exit budget. Deadline/overflow refusal kills the
command's private process group while its leader remains unreaped, closes both
pipes and allows at most one second for reaping. That cleanup allowance never
extends checkpoint acceptance. An already exhausted deadline spawns no command.
The ordinary v8 health doctor and v2 bundle command also use this shared
collector and pass their original `health_deadline_ns` directly. This applies
even when the aggregate opt-in is disabled. Other trial subprocesses retain
their existing collection policies; this boundary covers the four health reports.
The [shared-pair regression and restarted-boot proof](../../../evidence/runtime-20261112/g8-health-command-bounds/README.md)
execute both actual v8/v2 command expressions from committed source: all four
baseline stdout/stderr bursts complete before timeout, while candidate capture
refuses before the writer finishes. Eight health and nine aggregate controls
qualify on GCE with current service epochs, retained-state parity and successful
own-unit/cgroup cleanup. Historical baseline files and PID authority are unchanged.

The [actual capture baseline and GCE controls](../../../evidence/runtime-20261109/g8-aggregate-command-bounds/README.md)
show the prior capture consuming complete synthetic eight-MiB stdout/stderr
bursts before timeout, and the corrected capture refusing before either writer
finishes. Nine source-pinned native controls, required checks and independent
preservation/unit cleanup qualify this collection boundary. They do not qualify
the actual child aggregate pressure lifecycle.

The receipt binds every target/report/bundle/receipt/file-identity/timing
artifact. `audit-runtime-docker-trial.py --require-aggregate-docker-health`
also requires the existing engine proof. Acceptance requalifies the complete
agent approval, original unfinished RUN, all engine phases, all independent
v8 health references and both bundle products. Aggregate owner generation must
match that engine proof. Its observations must occur after their corresponding
engine samples and before the following build/removal phase. Bundle identity,
source binary/build label, phase order and the unchanged shared deadline are
checked again. Raw targets, outputs and artifact hashes remain private.

The local checkpoint tests include recomputed-hash semantic attacks, real
subprocess capture/timeout/stderr refusal, and propagation of independent engine
qualification failure. Their archived fixture isolates that separately tested
engine validator; it is not a real-child pass. Actual aggregate pressure-trial
execution and its independent audit/cold-restoration evidence are still required.
