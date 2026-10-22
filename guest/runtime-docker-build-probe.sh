#!/bin/sh
# Disposable qualification workload for the approved production Docker profile.
# Supply this script and a static x86-64 BusyBox through copied /work inputs.
set -eu
[ "$#" -eq 0 ] || { [ "$#" -eq 1 ] && [ "$1" = --overlap-proof ]; }
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
docker save -o "$work/image.tar" g8-proof:latest
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
echo G8_BUILD_WORKLOAD_PASS
