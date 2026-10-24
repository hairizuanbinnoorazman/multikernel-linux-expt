#!/bin/sh
# Disposable qualification workload for the approved production Docker profile.
# Supply this script and a static x86-64 BusyBox through copied /work inputs.
set -eu
[ "$#" -eq 0 ] || { [ "$#" -eq 1 ] && { [ "$1" = --overlap-proof ] || [ "$1" = --storage-pressure ]; }; }
export DOCKER_HOST=unix:///var/run/docker.sock
work=/home/rootless/.local/tmp/g8-build
test ! -e "$work"
mkdir -m 700 "$work"
mkdir "$work/context"
cp /work/input/busybox "$work/context/busybox"
chmod 755 "$work/context/busybox"
echo G8_BUILD_WORKLOAD_BEGIN
id
uname -r
grep -E '^(CapEff|CapBnd|NoNewPrivs|Seccomp):' /proc/self/status
docker version
docker info --format '{{json .}}'
if [ "${1:-}" = --storage-pressure ]; then
    pressure_release=0
    trap 'pressure_release=1' USR1
    trap 'rm -f "$work/pressure.bin"' EXIT
    before_kib=$(df -Pk "$work" | awk 'NR == 2 {print $4}')
    test "$before_kib" -gt 65536
    echo "G8_STORAGE_PRESSURE_BEFORE_KIB=$before_kib"
    echo "G8_STORAGE_PRESSURE_FS_MAGIC=$(stat -f -c %t "$work")"
    set +e
    LC_ALL=C dd if=/dev/zero of="$work/pressure.bin" bs=1048576 count=2304
    coarse_status=$?
    set -e
    echo "G8_STORAGE_PRESSURE_COARSE_WRITE_EXIT=$coarse_status"
    test "$coarse_status" -eq 1
    # A failed 1 MiB allocation can leave a smaller usable tail. Consume at
    # most one additional MiB in filesystem-sized writes; keep the host's
    # zero-available-space requirement unchanged.
    set +e
    LC_ALL=C dd if=/dev/zero bs=4096 count=256 >> "$work/pressure.bin"
    write_status=$?
    set -e
    echo "G8_STORAGE_PRESSURE_WRITE_EXIT=$write_status"
    set +e
    sync
    sync_status=$?
    set -e
    echo "G8_STORAGE_PRESSURE_SYNC_EXIT=$sync_status"
    test "$sync_status" -eq 0
    echo "G8_STORAGE_PRESSURE_FULL_KIB=$(df -Pk "$work" | awk 'NR == 2 {print $4}')"
    echo "G8_STORAGE_PRESSURE_FILE_BYTES=$(stat -c %s "$work/pressure.bin")"
    test "$write_status" -ne 0
    echo G8_STORAGE_PRESSURE_FULL
    # Hold the full filesystem while the primary records the exact backing
    # identity and geometry. Release is a signal, requiring no new guest file.
    while [ "$pressure_release" -eq 0 ]; do sleep 1 || :; done
    rm -f "$work/pressure.bin"
    echo "G8_STORAGE_PRESSURE_RESTORED_KIB=$(df -Pk "$work" | awk 'NR == 2 {print $4}')"
    echo G8_STORAGE_PRESSURE_FREED
fi
cat > "$work/context/Dockerfile" <<'DOCKERFILE'
FROM scratch AS build
COPY busybox /bin/busybox
RUN ["/bin/busybox", "sh", "-ec", "/bin/busybox mkdir -p /out; echo multikernel-production-docker-build > /out/result"]
FROM scratch
COPY --from=build /out/result /result
COPY --from=build /bin/busybox /bin/busybox
CMD ["/bin/busybox", "cat", "/result"]
DOCKERFILE
if [ "${1:-}" = --overlap-proof ]; then
    sed -i 's@/bin/busybox mkdir -p /out; @/bin/busybox mkdir -p /out; echo G8_OVERLAP_RUN_BEGIN; /bin/busybox sleep 45; echo G8_OVERLAP_RUN_END; @' "$work/context/Dockerfile"
fi
docker build --progress=plain --network=none --pull=false -t g8-proof:latest "$work/context"
echo G8_MULTISTAGE_BUILD_PASS
docker build --progress=plain --network=none --pull=false -t g8-proof:cached "$work/context"
echo G8_REPEAT_BUILD_PASS
test "$(docker run --rm --network=none g8-proof:latest)" = multikernel-production-docker-build
echo G8_BUILT_IMAGE_RUN_PASS
echo G8_NESTED_AUTHORITY_BEGIN
docker run --rm --network=none g8-proof:latest /bin/busybox sh -ec \
    '/bin/busybox id; /bin/busybox grep -E "^(CapEff|NoNewPrivs|Seccomp):" /proc/self/status; /bin/busybox cat /proc/self/uid_map'
echo G8_NESTED_AUTHORITY_END
docker image inspect g8-proof:latest
set +e
docker save -o "$work/image.tar" g8-proof:latest
save_status=$?
set -e
echo "G8_IMAGE_SAVE_EXIT=$save_status"
if [ "$save_status" -ne 0 ] || [ ! -s "$work/image.tar" ]; then
    echo G8_IMAGE_SAVE_ARTIFACT_MISSING
    ls -lain "$work"
    df -k "$work"
    exit 71
fi
sha256sum "$work/image.tar"
sed -i 's@echo G8_OVERLAP_RUN_BEGIN; /bin/busybox sleep 45; echo G8_OVERLAP_RUN_END; @@' "$work/context/Dockerfile"
sed -i 's/echo multikernel-production-docker-build/exit 37; echo multikernel-production-docker-build/' "$work/context/Dockerfile"
if docker build --progress=plain --network=none --pull=false -t g8-proof:must-fail "$work/context"; then
    echo G8_FAILED_BUILD_UNEXPECTEDLY_PASSED
    exit 1
fi
echo G8_FAILED_BUILD_REJECTED
docker image rm g8-proof:latest g8-proof:cached
docker builder prune --all --force
test -z "$(docker ps -aq)"
echo G8_BUILD_RESOURCES_REMOVED
if [ "${1:-}" = --storage-pressure ]; then
    docker info --format '{{json .}}'
    echo G8_STORAGE_PRESSURE_BUILD_RECOVERED
fi
echo G8_BUILD_WORKLOAD_PASS
