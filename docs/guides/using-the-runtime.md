# Using the Multikernel runtime

This guide provides copyable `ctr` and Docker workflows after the runtime has
been installed and verified with the
[operator quickstart](runtime-operator-quickstart.md).

Every command selects `io.containerd.multikernel.v2` explicitly. Docker must
also use `--network none` because the runtime, rather than Docker's bridge,
owns the child link. Keep the runtime opt-in.

The examples use `docker.io/library/busybox:1.36`, matching the live feature
matrix. Pin a digest as well when reproducibility or evidence quality requires
immutable image identity.

## Preconditions

```bash
systemctl is-active mkruntimed mknetd containerd
mountpoint -q /sys/fs/multikernel
mountpoint -q /srv/multikernel-storage
test -x /usr/local/bin/containerd-shim-multikernel-v2
test -c /dev/net/tun
```

Run as an ordinary sudo-capable user, not as a root login shell. Use an idle
host for feature or recovery testing.

## Pull and run with `ctr`

```bash
sudo ctr images pull docker.io/library/busybox:1.36
sudo ctr run --rm \
  --runtime io.containerd.multikernel.v2 \
  docker.io/library/busybox:1.36 mk-ctr-once \
  /bin/sh -c 'uname -r; echo hello-from-child'
```

For a split create/start lifecycle:

```bash
sudo ctr containers create \
  --runtime io.containerd.multikernel.v2 \
  docker.io/library/busybox:1.36 mk-ctr-demo \
  /bin/sh -c 'trap "exit 42" TERM; while :; do sleep 1; done'
sudo ctr tasks start --detach mk-ctr-demo
sudo ctr tasks list
```

Execute another process and verify that the boot ID belongs to a child:

```bash
cat /proc/sys/kernel/random/boot_id
sudo ctr task exec --exec-id show-boot mk-ctr-demo \
  /bin/cat /proc/sys/kernel/random/boot_id
```

Signal, observe, and delete the task:

```bash
sudo ctr tasks kill --signal SIGTERM mk-ctr-demo
sudo ctr tasks list
sudo ctr tasks rm mk-ctr-demo
sudo ctr containers rm mk-ctr-demo
```

## Pull and run with Docker

Docker may require the opt-in registration described in
[`deploy/README.md`](../../deploy/README.md). Verify that registration did not
change Docker's default runtime.

```bash
sudo docker pull docker.io/library/busybox:1.36
sudo docker run --rm \
  --runtime io.containerd.multikernel.v2 \
  --network none \
  docker.io/library/busybox:1.36 \
  /bin/sh -c 'uname -r; echo hello-from-child'
```

For a split lifecycle:

```bash
sudo docker create \
  --runtime io.containerd.multikernel.v2 \
  --network none \
  --name mk-docker-demo \
  docker.io/library/busybox:1.36 \
  /bin/sh -c 'trap "exit 42" TERM; while :; do sleep 1; done'
sudo docker start mk-docker-demo
sudo docker inspect --format '{{.State.Status}}' mk-docker-demo
sudo docker exec mk-docker-demo cat /proc/sys/kernel/random/boot_id
sudo docker kill --signal TERM mk-docker-demo
sudo docker wait mk-docker-demo
sudo docker rm mk-docker-demo
```

## Stdin, attach, and terminals

Foreground stdin, detach/reattach, `CloseIO`, PTYs, and initial terminal-size
propagation have passed live through both clients. A deliberate post-start
resize still requires current live revalidation.

For an interactive Docker shell:

```bash
sudo docker run --rm -it \
  --runtime io.containerd.multikernel.v2 \
  --network none \
  docker.io/library/busybox:1.36 /bin/sh
```

For `ctr`, supply `--tty` only to a process that expects a terminal:

```bash
sudo ctr run --rm --tty \
  --runtime io.containerd.multikernel.v2 \
  docker.io/library/busybox:1.36 mk-ctr-tty /bin/sh
```

## Pause, resume, and stats

Pause, resume, and stats are implemented with focused tests. Treat them as
requiring current-revision live evidence when making a release claim.

```bash
sudo ctr tasks pause mk-ctr-demo
sudo ctr tasks resume mk-ctr-demo
sudo ctr tasks metrics mk-ctr-demo

sudo docker pause mk-docker-demo
sudo docker unpause mk-docker-demo
sudo docker stats --no-stream mk-docker-demo
```

## Read-only bind inputs

Only the admitted read-only bind forms are supported. Writable, propagating,
or otherwise unsupported mounts fail closed.

```bash
input_dir=$(mktemp -d -p /tmp mk-runtime-input.XXXXXX)
chmod 0755 "$input_dir"
printf 'immutable input\n' >"$input_dir/value"
chmod 0644 "$input_dir/value"

sudo ctr run --rm \
  --runtime io.containerd.multikernel.v2 \
  --mount "type=bind,src=$input_dir,dst=/opt/input,options=rbind:ro" \
  docker.io/library/busybox:1.36 mk-ctr-bind \
  cat /opt/input/value
```

Remove the temporary input only after the task has exited:

```bash
rm -r "$input_dir"
```

Docker's equivalent is:

```bash
sudo docker run --rm \
  --runtime io.containerd.multikernel.v2 \
  --network none \
  --mount type=bind,src=/absolute/host/input,dst=/opt/input,readonly \
  docker.io/library/busybox:1.36 cat /opt/input/value
```

## Networking

The child receives a primary-mediated interface named `mkn0`. Outbound DNS and
HTTP passed the shared feature matrix, while sibling child links were isolated.
Docker's own bridge must remain disabled for these workloads.

```bash
sudo ctr task exec --exec-id network-check mk-ctr-demo \
  /bin/sh -c 'ip -4 address show dev mkn0; nslookup example.com'
```

Network availability depends on `mknetd`, the configured egress interface,
DNS policy, and host firewall/NAT state.

## Current support boundary

The runtime supports the demonstrated create/start/exec/signal/wait/delete,
private root, primary-mediated network, stdin/attach, and terminal paths.
Capabilities, rlimits, hostname/path policy, read-only root, standard mounts,
and materialized read-only binds have narrower implementation coverage.

Dynamic resource `Update` and checkpoint/restore are not implemented.
Unsupported seccomp, hooks, writable or propagating mounts, and unapproved OCI
fields fail closed. Kubernetes `RuntimeClass` is a future G7 gate, not a
supported workflow in this guide.

For the current claim-by-claim status, consult the root
[`README.md`](../../README.md) and the
[G4-G6 remediation checklist](../runtime/learnings/g4-g6-remediation-checklist.md).

## End-of-session cleanup

Always remove named tasks and containers through their client, then confirm
the host inventory is clean:

```bash
sudo ctr tasks list
sudo ctr containers list
sudo docker ps -a
sudo find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 -print
```

If client deletion fails or a child remains, follow
[Runtime recovery and cleanup](runtime-recovery-and-cleanup.md) rather than
deleting state files manually.
