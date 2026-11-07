# Private-runtime crash during child-local synced writes

`MK_MODULE_TRIAL_SCENARIO=runtime-crash-during-io` selects the version-4 recovery
scope. It uses a source-pinned BuildKit RUN script that repeatedly overwrites a
fixed 4 MiB file and fsyncs it before publishing synced progress. Guest monotonic
uptime bounds the loop to approximately 45 seconds plus its last write; an
independent iteration ceiling bounds a frozen clock. Progress is published at
most once per second, keeping streams within the existing bounds. There is no
sleep-based overlap barrier in this variant.

The harness waits for the first synced write before killing its disposable
private runtime. After confirmed SIGKILL/-9 exit it records a new stderr baseline,
then requires a newer synced sequence while that runtime remains dead. Only then
may it start the replacement. It requires further synced progress after adoption.
No installed primary service is restarted and no retained failed generation is
recovered or signaled.

The version-2 active checkpoint contains four exact frames: before-kill,
confirmed-dead, after-kill and after-adoption. They bind old/new runtime PIDs,
confirmed exit status, strict host monotonic order, executed RUN/progress markers
and raw prefixes of the final original stream. Synced sequence numbers must be
canonical, increasing and advance strictly from confirmed-dead to after-kill,
then again after adoption. Existing child/engine/owner, original build completion,
archive preservation, rebuild and primary-isolation requirements still apply.

The first image includes the synced file. The workload reads it back through the
child-local engine and requires the known SHA-256 of 4 MiB zeros and exact size.
Final native written bytes must cover the total logical bytes of all completed
fsynced iterations. The exact owner's lease must be RELEASED with its clean
offline-check marker; ordinary postrun resource checks remain separate. The
inspector itself never claims cleanup, repairs ownership or acquires signal
authority. Executed bytes of both guest shell scripts must match the committed
archive and workload checkpoint in the independent audit.

Local controls cover no-progress during outage/adoption, data mismatch, native
underreporting, wrong owner, missing confirmed-dead frame/status/time and
cross-policy refusal with recomputed hashes. Removing only synced-progress
ordering produces detected false acceptance of both no-progress cases. Actual
GCE qualification is pending. Broader storage/network-server failures, abrupt
child loss, autonomous failure detection, concurrency, cold restoration and full
G7/G8 remain open. Direct children use no nested virtualization and share host
trust after arbitrary child-kernel compromise.
