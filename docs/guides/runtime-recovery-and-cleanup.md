# Runtime recovery and cleanup

Use this guide when a runtime test, child kernel, service, or host cleanup does
not complete normally. Recovery must preserve evidence first and mutate only
resources whose ownership is established.

These procedures are for a disposable, trusted test host. They are not a
general disaster-recovery promise: the broader G7 security, reliability, and
soak matrix remains open. The target uses direct child kernels because nested
virtualization is unavailable under its cloud constraints. Recovery checks do
not establish containment after arbitrary child-kernel compromise.

## First response

1. Stop issuing new create or start requests.
2. Keep SSH open and confirm that serial-console recovery is available.
3. Record the failing command, exit status, UTC time, host boot ID, and service
   state.
4. Capture logs and inventories before restarting or deleting anything.
5. Identify whether containerd, the shim, `mkruntimed`, Kerf, or the kernel
   currently owns the resource.

Useful read-only capture commands are:

```bash
date -u +%FT%TZ
uname -a
cat /proc/sys/kernel/random/boot_id
systemctl --no-pager --full status mkruntimed mknetd containerd docker
journalctl -u mkruntimed -u mknetd --since '-30 minutes' --no-pager
sudo ctr tasks list
sudo ctr containers list
sudo find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 2 -print
ip -o link show
sudo iptables -t nat -S
sudo iptables -S
```

Do not publish journals until they have been checked for credentials, tokens,
cloud identity, and operator paths.

## Classify the failure

| Symptom | First authority to inspect | Safe first action |
| --- | --- | --- |
| Create fails before a child appears | shim and `mkruntimed` journals | Preserve the error; validate configuration, bootstrap artifacts, and storage |
| Task exists but is stopped | `ctr tasks list` or Docker inspect | Record exit state, then remove it through the same client |
| Child exists but the client object is missing | `mkruntimed` state and Kerf/sysfs inventory | Restart only `mkruntimed` and let reconciliation classify it |
| Network link or firewall rule remains | `mknetd` journal and state | Request normal task deletion, then restart `mknetd` if needed |
| Storage validator fails | mount, device identity, and `/etc/fstab` | Keep `mkruntimed` stopped and correct the mount identity |
| Service restart loses a running task | shim/containerd/daemon journals | Preserve the child boot ID and run no further destructive recovery |
| Host becomes unreachable | GCE serial output | Capture serial logs before rebooting, stopping, or deleting the VM |

## Normal task cleanup

Prefer the client that created the task. For a known `ctr` test ID:

```bash
sudo ctr tasks kill --signal SIGKILL TEST_ID
sudo ctr tasks rm -f TEST_ID
sudo ctr containers rm TEST_ID
```

For a known Docker test name:

```bash
sudo docker rm -f TEST_NAME
```

Replace only explicit test identifiers. Never bulk-remove unrelated host
workloads. Wait for runtime reconciliation before escalating.

## Service recovery

Check whether the child boot ID and task state are part of the assertion before
restarting anything. The implemented recovery path is expected to preserve a
running child across an `mkruntimed` restart, but current live evidence may lag
the implementation. The [approved Docker guest recovery proof](../../evidence/runtime-20261022/g8-host-docker-runtime-recovery/README.md)
now qualifies one graceful private candidate-runtime restart: the live child boot
ID, engine ID, saved-image digest and running ownership generation stay unchanged,
then cleanup passes. It does not qualify crash/power loss, other service restarts
or the installed service capability restrictions.

Restart one authority at a time and capture before/after state:

```bash
systemctl show -p MainPID --value mkruntimed
sudo systemctl restart mkruntimed
systemctl is-active mkruntimed
systemctl show -p MainPID --value mkruntimed
```

Do not restart containerd or Docker on a host with unrelated tasks. Do not
delete daemon, shim, network, or storage state directories by hand. Those files
carry generations and ownership needed for safe reconciliation.

The repository's bounded reconnect audit is
`scripts/test-runtime-recovery.sh`. Run it only as an ordinary sudo-capable
user on an otherwise idle, disposable, already qualified host. It deliberately
restarts services and kills a shim worker.

## Storage startup failures

If `mkruntimed` refuses to start, confirm that the dedicated filesystem is
actually mounted rather than allowing the directory to fall through to the
primary root disk:

```bash
findmnt /srv/multikernel-storage
lsblk -o NAME,SERIAL,SIZE,FSTYPE,LABEL,UUID,MOUNTPOINTS
```

Run `scripts/validate-runtime-storage-mount.py` with the exact expected
mountpoint, persistent by-id path, serial, byte size, label, and UUID. A failed
identity check is a hard stop. Do not change expected values simply to match an
unexpected device.

Offline filesystem repair requires the runtime services and all exports using
the image to be stopped. Preserve the server log and identify the exact image
before invoking `e2fsck`; do not run it against a mounted filesystem.

## Host inventory after cleanup

Cleanup is complete only when all scoped inventories agree:

```bash
sudo ctr tasks list
sudo docker ps -a
sudo find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 -print
ip -o link show | awk -F': ' '$2 ~ /^mkv[0-9a-f]+$/ {print $2}'
sudo iptables -t nat -S POSTROUTING | grep '172\.31\.' || true
sudo iptables -S | grep '^\(-N\|-A\) MK-' || true
```

Compare the primary CPU and memory inventory with the preflight report. An
empty container list alone does not prove that CPUs, memory, links, storage
servers, or firewall rules were returned.

## GCE recovery and cleanup

Before a reboot, stop, or deletion, capture serial output and the project-wide
resource ledger. If SSH is lost:

```bash
make serial MK_PROJECT="$MK_PROJECT" INSTANCE="$MK_VM" ZONE="$MK_ZONE"
```

Stopping a VM ends vCPU/RAM charges but leaves disks and possibly other assets
billable. Deleting a VM deletes only disks whose auto-delete policy says so.
After cleanup, collect a second resource ledger and explicitly account for
instances, disks, snapshots, addresses, and firewall rules.

## Escalation boundary

Stop recovery and retain the host when any of these is true:

- the target child or state generation cannot be tied to the failed test;
- primary CPU, memory, disk, or NIC ownership is ambiguous;
- a child remains active after normal client deletion and daemon
  reconciliation;
- the dedicated storage identity changed unexpectedly;
- cleanup would require editing runtime state or raw Multikernel sysfs; or
- the failure is the behavior under investigation.

In those cases, preserve the VM or disk long enough to collect evidence rather
than converting a diagnosable failure into an unreviewable cleanup.

The later [service-policy recovery qualification](../../evidence/runtime-20261022/g8-host-docker-service-policy/README.md)
repeats the Docker guest build and graceful private runtime restart with the
runtime/network capability, NNP and ambient settings read from the committed
service units. Raw process masks and executable/unit hashes are retained for
both runtime processes and the network daemon. Other systemd restrictions,
crash/power loss and other service restarts remain separate checks.

## Native export process death

A server lost after client acceptance requires operator action. Process absence
and an apparently clean filesystem are insufficient to recreate the session or
release its image: a generation-bound synced terminal close is required. The
runtime retains unsafe PREPARING/ACTIVE/QUIESCING evidence and returns sanitized,
non-retryable OPERATOR_ACTION on unsafe lifecycle deletion. Repeating the same
idempotency key returns the retained failure without repeating mutation. Missing
kernel presence alone does not free unsafe allocation claims.

Keep the image, process record, native log, lifecycle journal and all ownership
generations. Do not edit a lease to RELEASED, guess a process from a reused PID,
or run filesystem repair against any live/exported image. The guarded private
export-crash harness uses expected exit 77 for retained fault completion; it does
not claim normal cleanup. The [native identity contract](../runtime/contracts/native-resource-identity.md)
and [controls](../../evidence/runtime-20261026/g8-native-death-controls/README.md)
describe the checked scope. Direct children use no nested virtualization and
remain in the shared host trust domain; full G7/G8 remains open.

The [live crash case](../../evidence/runtime-20261026/g8-live-native-export-crash/README.md)
now exercises this retained failure after an inner build. Its
[operator quarantine](../../evidence/runtime-20261026/g8-native-export-crash-quarantine/README.md)
uses guarded Kerf operations on the exact stopped generation and returns its pool
while preserving the image and original failed ledgers. Read-only e2fsck exits 4;
no filesystem repair or automatic storage release is claimed. The subsequent
[cold restoration](../../evidence/runtime-20261026/g8-native-export-crash-restoration/README.md)
restores the original transport and confirms all four quarantined generations
remain unchanged. Do not hot-unload the candidate transport after traffic; that
unregister lifetime remains unqualified.

## Shadowed endpoint firewall guards

An existing DROP or endpoint-chain jump is insufficient if an earlier rule can
accept or return the packet. Linux endpoint CHECK now inspects filter-table
IPv4/IPv6 INPUT/FORWARD prefixes as well as the complete private chain. It
conservatively refuses prefixes without one distinct positive literal ingress
interface. A same-boot reconciliation failure preserves ownership and requires
operator diagnosis; this change does not repair rules or continuously enforce
policy. Preserve the endpoint generation and bounded rule observations. Do not
flush host firewall tables to make CHECK pass. The
[GCE namespace/TCP controls](../../evidence/runtime-20261026/g8-ingress-guard-precedence/README.md)
explain the tested scope; live Docker requalification remains separate.


## Pressure wait failure and retained orphan helpers

The [017c52f live failure](../../evidence/runtime-20261026/g8-live-network-check-failure/README.md)
shows that a terminal systemd MainPID does not establish that the owned task,
NBD server, relay or backing file is gone. Forced Docker removal can fail on a
zombie task PID. Preserve the failed ownership generations and inspect the
original unit cgroup, executable/incarnation and backing descriptors before
any scoped recovery. Do not retry old PID-based commands after a restart.

The user restarted this instance before operator quarantine completed. The
[retention proof](../../evidence/runtime-20261027/g8-failed-network-post-restart/README.md)
confirms idle resources and preserved ACTIVE/RUNNING claims. Read-only e2fsck
status 0 is not a substitute for synced close or a lifecycle storage release.
This private generation must remain retained and excluded from reuse; no broad
state reset or automatic restart is qualified. The fifth image has only a
post-restart content hash, so pre/post-restart byte equality cannot be claimed.

New [phase outcome checkpoints](../../evidence/runtime-20261027/g8-pressure-wait-outcomes/README.md)
record process exit/return code separately from deadline expiry and bounded-read
output overflow, before teardown. They identify the observed wait outcome, not
the underlying storage/transport cause. Their successful receipt remains a
sampled hold point, separate from final normal cleanup evidence.
