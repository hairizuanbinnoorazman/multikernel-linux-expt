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
