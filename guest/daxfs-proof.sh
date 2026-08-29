#!/bin/busybox sh

set -u

getarg() {
    key=$1
    for argument in $(cat /proc/cmdline); do
        case "$argument" in
            "$key"=*) echo "${argument#*=}"; return ;;
        esac
    done
}

test_name=$(getarg daxfs.test)
role=$(getarg daxfs.role)

echo "DAXFS_ROOT_PROOF_BEGIN"
echo "release=$(uname -r)"
echo "cmdline=$(cat /proc/cmdline)"
echo "online_cpus=$(cat /sys/devices/system/cpu/online)"
echo "cpu_count=$(grep -c '^processor' /proc/cpuinfo)"
echo "mem_total=$(awk '/MemTotal/ { print $2 " " $3 }' /proc/meminfo)"
echo "pid1=$(readlink /proc/1/exe)"
echo "self=$(readlink /proc/self/exe)"
echo "root_mount=$(awk '$2 == "/" { print $0 }' /proc/mounts)"
if [ -e /INITRAMFS_ONLY ]; then
    echo "initramfs_only_marker=present"
else
    echo "initramfs_only_marker=absent"
fi
cat /DAXFS-MARKER
sha256sum /DAXFS-MARKER /opt/proof/nested/payload.txt
if (cd / && sha256sum -c /MANIFEST.sha256); then
    echo "DAXFS_MANIFEST_READY"
else
    echo "DAXFS_MANIFEST_ERROR"
    while true; do sleep 3600; done
fi

case "$test_name" in
    shared-ro)
        if echo should-fail > /read-only-write 2>/dev/null; then
            echo "SHARED_RO_WRITE_REJECTED=no"
        else
            echo "SHARED_RO_WRITE_REJECTED=yes"
        fi
        echo "SHARED_RO_READY role=$role"
        ;;
    shared-rw)
        echo "writer=$role" > "/shared-${role}.txt"
        other=A
        [ "$role" = A ] && other=B
        attempt=0
        while [ ! -f "/shared-${other}.txt" ] && [ "$attempt" -lt 200 ]; do
            sleep 0.05
            attempt=$((attempt + 1))
        done
        if [ -f "/shared-${other}.txt" ]; then
            echo "SHARED_RW_PEER_VISIBLE=yes role=$role peer=$other"
            cat "/shared-${other}.txt"
        else
            echo "SHARED_RW_PEER_VISIBLE=no role=$role peer=$other"
        fi
        i=1
        while [ "$i" -le 100 ]; do
            echo "$role-$i" >> /contended.log
            i=$((i + 1))
        done
        sync
        sleep 2
        echo "SHARED_RW_CONTENDED role=$role lines=$(wc -l < /contended.log) sha256=$(sha256sum /contended.log | awk '{print $1}')"
        echo "SHARED_RW_READY role=$role"
        ;;
    restart)
        echo "RESTART_PERSISTED_A=$(cat /shared-A.txt 2>/dev/null || echo missing)"
        echo "RESTART_PERSISTED_B=$(cat /shared-B.txt 2>/dev/null || echo missing)"
        echo "DAXFS_RESTART_READY"
        ;;
esac

echo "DAXFS_ROOT_READY"
while true; do sleep 3600; done
