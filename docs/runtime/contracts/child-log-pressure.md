# Child builds and host log rotation

The private trial scenario `logging` selects `--child-log-pressure` with one
direct child kernel and the exact staged agent. It derives and normally
validates the bootstrap without changing the installed approval or injecting
credentials. It cannot be combined with pressure, concurrency, recovery or
engine-observation scenarios. Same-boot candidate reuse from this new scenario
is not yet supported.

The host Docker container starts detached, with the existing concurrent
trial's 180-second cold-start window. Ordinary exec captures keep their
30-second command deadline; build/writer polling has a separate 300-second
window within the unchanged 1200-second outer trial limit. Polling, command
collection and readiness acceptance share that one monotonic deadline. Each
probe retains the ordinary 30-second cap and is clipped to the remaining
window. Expired windows launch no new probe; even a successful result arriving
at or after the deadline refuses before build/rotation qualification. Its fixed unprivileged init runs the
complete child-local rootless Docker build/cache/run/save/failure matrix into
private child files, then writes a controlled 16 MiB to stdout. A completion
file is created only after the build succeeds and the fixed writer finishes.
The original init holds until its authenticated Docker signal releases it.

Bounded private Docker exec captures retain the complete build output, archive
hash readback and child engine observation. Primary Docker must retain its
engine/inventory and lack the exact child-built image. The stopped original
container must exit zero and retain the expected runtime, user, capabilities,
security options, exact fixed command and bounded local log policy.

Two metadata-only observations require exactly the local driver's current and
one rotated file, each protected and within the configured one-million-byte
limit plus bounded record overhead. The rotated file must reach that threshold;
the conservative observed total is 2,131,072 bytes. Neither observation opens
log contents. The private checkpoint binds source-agent approval, epoch, build
streams, command exits, primary observation and rotation sample. The independent
audit requalifies it and still requires normal native release/offline checks,
resource return and primary file/service/Docker/network parity. Cold restoration
remains a separate requirement after candidate traffic.

This observes rotation in the disposable host engine, using the actual child
runtime. It does not measure every produced/dropped byte, peak disk/RSS/CPU use,
slow-reader fairness, durable delivery or all production logging paths. Detached
output deliberately does not pass through an attached CLI; existing attached
stream limits remain unchanged. Direct child kernels use no nested
virtualization and share primary trust after arbitrary child-kernel compromise.
The [corrected real-child GCE proof](../../../evidence/runtime-20261121/g8-child-log-pressure-corrected/README.md)
now qualifies source `75e5e188`: both exact units exit zero, the full child
build matrix and current-agent approval requalify, and two rotated local files
total 1,503,596 bytes within the original 2,131,072-byte sample bound. Primary
state and native cleanup pass, including five altered-identity refusals. Its
failed 30-second predecessor stays preserved and unqualified. Independent cold restoration also passes: 264 artifacts/107 absences and all
earlier sets survive, archived success retains its original boot, old unit
completion refuses the new boot, original resources return idle and fresh
primary state plus the durable 200 GiB/5% reserve qualify. Loss/peak accounting and complete G7/G8 remain open.

The [shared readiness-deadline proof](../../../evidence/runtime-20261122/g8-child-log-budget/README.md)
reproduces actual source `75e5e188` accepting a controlled success at 301 seconds
following a 300-second polling window. Source `506c145c` rejects the identical
sequence and clips the final probe to the original deadline. Six source-pinned
GCE control programs pass without skips, including real subprocess timeout and
success capture, with primary/history/idle and scratch preservation. Required
checks pass. This closes that command-budget acceptance boundary; it does not
relabel the earlier successful child lifecycle as new-source execution. Existing
owned-process cleanup and private artifact fsync retain their separate contracts
and never grant readiness success or another probe.

## Finite writer completion

Current checkpoint/view version 2 uses a finite BusyBox `dd` writer reading
`/dev/zero` with 256 blocks of 65,536 bytes. Its exit status must be zero before
the private exact-byte record and done marker are published. The host captures
writer status, byte record and diagnostics separately; the checkpoint binds
those original command results to the exact fixed container command. Short
producer failure, closed output, nonzero status, wrong/float byte records and
legacy envelopes refuse even after receipt hashes are recomputed. This binds
completion of the configured 16 MiB producer; delivered/dropped logging bytes,
peak resource use and full G7/G8 remain separate requirements.

Earlier version-1 proofs retain their configured-writer and observed-rotation
scope. Their `yes | head` pipeline can return success on short producer EOF;
version 2 does not reclassify those archived observations as finite-writer proof.
Actual new-source child execution requires its own source/epoch audit and cold
restoration. No nested virtualization or hostile-kernel boundary is added.

The [finite-writer actual-child proof](../../../evidence/runtime-20261122/g8-finite-child-writer/README.md)
now qualifies source `2ad6cda1` in exact successful parent/workload epochs. The
independent v2 audit binds zero writer exit, exact 16 MiB completion, full child
build matrix, saved archive readback and primary image/engine separation. Two
log files total 1,797,083 bytes below the original 2,131,072-byte bound; primary
parity, native offline release/resource return and five identity refusals pass.
Independent cold restoration also passes with 280 artifacts/111 absences and
all historical/native control sets preserved, exact current-source module
binding, archived original-boot frame requalification and fresh primary state.
Original resources and the durable 200 GiB/5% reserve return verified. Older v1
proofs retain their own scopes;
loss/peak accounting and full G7/G8 remain open.
