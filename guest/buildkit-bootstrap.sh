#!/bin/busybox sh
# Experimental approved-builder-image bootstrap. Run before accepting workloads.
# UID mappings are established by trusted bootstrap; builder exec enables NNP.
set -eu
root=/bundle/rootfs
# runc changes propagation on its '/' before pivot_root. Give the builder
# a mount point, retaining the restricted /dev, /proc, /sys and /run mounts.
mount -o rbind "$root" "$root"
# The production /sys bind is deliberately non-recursive. runc still needs
# to discover cgroup2; expose the guest hierarchy read-only, without delegation.
mount --bind /sys/fs/cgroup "$root/sys/fs/cgroup"
mount -o remount,bind,ro,nosuid,nodev,noexec "$root/sys/fs/cgroup"
mkdir -p "$root/run/buildkit" "$root/run/user/1000"
chown 1000:1000 "$root/run/buildkit" "$root/run/user/1000"
chmod 0700 "$root/run/user/1000"
chmod 0755 "$root/run/buildkit"
# Create the user namespace before chroot: Linux rejects CLONE_NEWUSER when
# the caller's filesystem root differs from its mount namespace root.
/bin/busybox su user -s /bin/sh -c \
    'export TMPDIR=/home/user/.local/tmp XDG_RUNTIME_DIR=/run/user/1000; exec rootlesskit /bin/busybox chroot /bundle/rootfs /bin/busybox setpriv --nnp buildkitd --rootless --oci-worker-snapshotter=native --oci-worker-net=host --addr unix:///run/buildkit/buildkitd.sock --root /home/user/.local/share/buildkit' \
    >/buildkit-bootstrap.log 2>&1 &
builder_pid=$!
for attempt in $(seq 1 100); do
    if [ -S "$root/run/buildkit/buildkitd.sock" ]; then
        # Local to this dedicated workload guest; never exposed on TCP/primary.
        chmod 0666 "$root/run/buildkit/buildkitd.sock"
        echo MK_GUEST_BUILDER_AUTHORITY_BEGIN
        for status in /proc/[0-9]*/status; do
            grep -q '^Name:.*buildkitd$' "$status" || continue
            grep -E '^(Uid|Gid|CapEff|NoNewPrivs):' "$status"
            cat "${status%status}uid_map"
            cat "${status%status}gid_map"
        done
        echo MK_GUEST_BUILDER_AUTHORITY_END
        echo MK_GUEST_BUILDER_READY
        exit 0
    fi
    kill -0 "$builder_pid" 2>/dev/null || break
    sleep 0.1
done
cat /buildkit-bootstrap.log
echo MK_GUEST_BUILDER_FAILED
exit 1
