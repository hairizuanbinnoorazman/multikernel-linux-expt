# Disposable Docker workload output policy

The four profile/concurrency attach call sites (ordinary, restart, pressure and concurrent trials)
exec the trusted CLI through `/usr/bin/prlimit` with soft/hard `RLIMIT_FSIZE`
131072 and child `RLIMIT_CORE` zero. The kernel therefore caps each attached regular stdout/stderr file
before a polling reader runs. The controller, runtime, Docker daemon and storage
helpers do not inherit this child-only change; two-GiB image writes remain under
their existing policies. No threaded Python `preexec_fn` is used.

Current workload readers read at most 131073 bytes and refuse at or above
131072. The phase waiter records `output-limit` at the exact cap even if expected
markers are present. The engine build-window capture applies the same reader.
Reaching the cap cannot publish a complete workload pass; nonzero attach exits
still fail and existing owned cleanup proceeds. Existing raw streams remain
private diagnostics, not public metadata.

The disposable Docker daemon defaults, and each explicit container create,
select the `local` logging driver with `max-size=1m`, `max-file=2`,
`compress=false`, `mode=non-blocking` and `max-buffer-size=1m`. Live inspection
must match the selected driver and exact container options. The primary Docker
configuration is not edited or restarted. These options follow Docker's
[local driver](https://docs.docker.com/engine/logging/drivers/local/) and
[nonblocking delivery](https://docs.docker.com/engine/logging/configure/)
contracts. Nonblocking logging can lose log records; rotated daemon copies are
not used as complete build evidence. Direct attached streams must independently
satisfy the workload checks.

The resource dependency `/usr/bin/prlimit` must be present. Config parsing can
be checked using [`dockerd --validate`](https://docs.docker.com/reference/cli/dockerd/)
without starting a daemon. Neither that check nor native file-cap fixtures prove
live Docker rotation or the real-child lifecycle. Pipe-based commands, daemon/
helper logs, peak CPU/memory and all other output producers retain their existing
policies. These limits do not establish arbitrary hostile-kernel containment or
close full G7/G8. Direct children use no nested virtualization and share primary
trust after child-kernel compromise.

The [committed regression and GCE proof](../../../evidence/runtime-20261112/g8-attached-output/README.md)
qualifies all four attach expressions with controlled writers, exact-boundary
refusal, private logging policy agreement and installed Docker configuration
validation: 21 native controls pass. Independent observation verifies source
pins, primary/retained-state parity, tool preservation and own-unit/cgroup cleanup.
Actual container log rotation and the aggregate real-child trial remain open.

`scripts/test-runtime-docker-logging-live.py` is an opt-in native logging trial
for a source-pinned, delegated lab unit with private network/mount namespaces
and an exclusive runtime directory. An outer guard must check boot, primary
preservation, protected tools and reserves. It starts private containerd/Docker
instances, imports only a protected static BusyBox fixture, and runs two native
runc containers with inherited versus explicit logging policy. Both containers
have no network, host mounts or capabilities and use a read-only rootfs.
Their cgroups stay below the delegated unit. The script requires successful
container exits and clean daemon shutdown, retaining private state for audit.
Native CLI commands retain separate bounded stdout/stderr and fixed status
records exclusively under the private trial directory. Warnings cannot corrupt
machine-readable identities; expiry, overflow and nonzero status refuse, leaving
diagnostics private. Existing evidence refuses before command execution.
The supervisor runs in the unit's `supervisor` leaf and Docker uses the sibling
`workloads` subtree, keeping processes out of the delegated domain root before
enabling cgroup-v2 controllers. It never adopts or moves primary service tasks.

`runtime_docker_log_rotation.py` checks only metadata of the two expected local
log files, without following directory/file aliases or reading contents. It
derives the lab-only internal path from the owned data root and container ID;
Docker deliberately leaves the local driver's API `LogPath` empty. This
internal layout is a version-specific observation, not a stable Docker API.
See the [upstream local logger setup](https://github.com/moby/moby/blob/master/daemon/container/container.go).
It
requires a rotated file reaching the decimal 1m threshold and each observed
file at most 1,000,000 + 65,536 bytes, allowing record overhead. That conservative
sample bound is not an exact disk quota or a peak measurement. Missing,
additional, unrotated, aliased, shared or oversized samples refuse. A successful
native trial would qualify installed daemon behavior on that fixture, while
real-child rotation, logging loss behavior and full G7/G8 remain open.
