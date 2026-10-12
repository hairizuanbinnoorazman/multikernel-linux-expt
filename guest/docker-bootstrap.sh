#!/bin/busybox sh
# Experimental approved Docker guest. No primary Docker socket or TCP listener.
set -eu
root=/bundle/rootfs
for module in llc stp bridge veth; do
    insmod "/bootstrap-modules/$module.ko"
done
mount -o rbind "$root" "$root"
mount --bind /sys/fs/cgroup "$root/sys/fs/cgroup"
mount -o remount,bind,ro,nosuid,nodev,noexec "$root/sys/fs/cgroup"
mkdir -p "$root/run/user/1000" "$root/etc/docker"
chown 1000:1000 "$root/run" "$root/run/user/1000"
chmod 0755 "$root/run"
chmod 0700 "$root/run/user/1000"
cat > "$root/etc/docker/daemon.json" <<'JSON'
{"rootless":true,"storage-driver":"vfs","features":{"containerd-snapshotter":false},
 "iptables":false,"ip6tables":false,"ip-forward":false,"ip-masq":false,
 "hosts":["unix:///var/run/docker.sock"],"data-root":"/home/rootless/.local/share/docker",
 "exec-root":"/run/user/1000/docker","pidfile":"/run/user/1000/docker.pid"}
JSON
/bin/busybox su rootless -s /bin/sh -c \
    'export PATH=/usr/local/bin:/usr/bin:/bin TMPDIR=/home/rootless/.local/tmp XDG_RUNTIME_DIR=/run/user/1000; exec /usr/local/bin/rootlesskit --net=none /bin/busybox chroot /bundle/rootfs /bin/busybox setpriv --nnp /usr/local/bin/dockerd --config-file=/etc/docker/daemon.json' \
    >/docker-bootstrap.log 2>&1 &
daemon_pid=$!
for attempt in $(seq 1 300); do
    if [ -S "$root/run/docker.sock" ]; then
        chmod 0666 "$root/run/docker.sock"
        echo MK_GUEST_DOCKER_AUTHORITY_BEGIN
        for status in /proc/[0-9]*/status; do
            grep -q '^Name:.*dockerd$' "$status" || continue
            grep -E '^(Uid|Gid|CapEff|NoNewPrivs):' "$status"
            cat "${status%status}uid_map"
            cat "${status%status}gid_map"
        done
        echo MK_GUEST_DOCKER_AUTHORITY_END
        echo MK_GUEST_DOCKER_READY
        exit 0
    fi
    kill -0 "$daemon_pid" 2>/dev/null || break
    sleep 0.1
done
cat /docker-bootstrap.log
echo MK_GUEST_DOCKER_FAILED
exit 1
