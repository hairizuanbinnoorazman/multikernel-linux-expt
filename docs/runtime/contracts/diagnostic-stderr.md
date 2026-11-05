# Shim diagnostic stderr transport

Source `4f6a4810` protects the default process/network/event/supervisor diagnostic
sink against stderr reader loss. Direct writes through Go's os.Stderr can trigger
SIGPIPE termination on descriptor 2. The sink worker now duplicates stderr with
F_DUPFD_CLOEXEC into a private descriptor >=3, writes through that descriptor and
closes it on return. EPIPE, EBADF, descriptor exhaustion and other write/duplicate
errors disable the lossy sink through its existing failure path. There is no retry,
raw-error fallback, process-wide signal change or inherited flag modification.

Each worker/supervisor still has one lazily allocated writer goroutine and a
64-record queue, with at most 1024 bytes per encoded record and one in-flight
record. A full pipe can retain one private descriptor and one blocked writer until
reader progress or process exit. Enqueue and close do not join blocked I/O. No
per-attempt descriptors or goroutines are created by rejected enqueue attempts.
The duplicate is CLOEXEC so it cannot leak into subsequently executed workloads.

This is a per-process diagnostic transport limit, not a node-wide admission or
retention quota. It does not make stderr delivery durable, change journald policy,
bound all inherited stdout/stderr writers, attest readiness, or replace Task output
forwarding. Other startup/flag/RPC producers keep their own contracts. Diagnostic
loss must preserve durable process/event ownership and exact lifecycle completion.

[Local proof](../../../evidence/runtime-20261105/g8-diagnostic-stderr-controls/README.md)
contains six parent-source SIGPIPE failures and green real-pipe default-sink cases
across all four typed producers. Additional controls preserve exact process exit,
supervisor crash recovery/PID cleanup and ordered event journal reconstruction/
acknowledgement. Full runtime race and required checks pass. The separate
[GCE fixture](../../../evidence/runtime-20261105/g8-diagnostic-stderr-gce/README.md)
also passes success/zero with real pipes and controlled processes, preserving all
219 prior artifacts and failure generations. Actual child-Docker
fault injection and node aggregate pressure/retention remain separate.

No nested virtualization is used; direct child kernels share host trust after
child-kernel compromise.
