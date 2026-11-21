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
window within the unchanged 1200-second outer trial limit. Its fixed unprivileged init runs the
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
Actual execution and complete G7/G8 qualification require live evidence.
