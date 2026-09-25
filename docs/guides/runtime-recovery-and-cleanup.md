# Runtime recovery and cleanup

Use this guide when a runtime test, child kernel, service, or host cleanup does
not complete normally. Recovery must preserve evidence first and mutate only
resources whose ownership is established.

These procedures are for a disposable, trusted test host. They are not a
general disaster-recovery promise: the full G8 failure matrix and several G6
reconnect cases remain open.

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
the implementation.

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
