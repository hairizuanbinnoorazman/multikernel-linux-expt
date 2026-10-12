#!/bin/sh
set -eu
echo BUILD_GUEST_PROBE_BEGIN
id
uname -r
grep -E '^(CapEff|CapBnd|NoNewPrivs):' /proc/self/status
/bin/sh /authority-probe.sh
docker --version
if [ "${MK_BUILD_PROBE_MODE:-baseline}" = baseline ]; then
    set +e
    timeout 15 dockerd --storage-driver=vfs --iptables=false --ip6tables=false \
        --bridge=none --host=unix:///var/run/docker.sock >/work/baseline.log 2>&1
    result=$?
    set -e
    cat /work/baseline.log
    echo "BASELINE_START_EXIT=$result"
    echo BUILD_GUEST_PROBE_END
    exit 0
fi
for attempt in $(seq 1 100); do
    docker info >/work/docker-info.txt 2>/work/docker-info.err && break
    sleep 0.1
done
docker info --format '{{json .}}'
echo DOCKER_DAEMON_INFO_PASSED
mkdir -p /work/context
cp /usr/local/bin/build-probe-busybox /work/context/busybox
cat > /work/context/Dockerfile <<'DOCKERFILE'
FROM scratch AS build
COPY busybox /bin/busybox
RUN ["/bin/busybox", "sh", "-ec", "/bin/busybox mkdir -p /out; echo multikernel-docker-build > /out/result; /bin/busybox chmod 0644 /out/result"]
FROM scratch
COPY --from=build /out/result /result
COPY --from=build /bin/busybox /bin/busybox
CMD ["/bin/busybox", "cat", "/result"]
DOCKERFILE
docker build -t guest-proof:latest /work/context
echo MULTISTAGE_BUILD_PASSED
docker build -t guest-proof:cached /work/context
echo REPEAT_BUILD_PASSED
result=$(docker run --rm guest-proof:latest)
[ "$result" = multikernel-docker-build ]
echo NESTED_CONTAINER_RUN_PASSED
echo NESTED_CONTAINER_AUTHORITY_BEGIN
docker run --rm guest-proof:latest /bin/busybox sh -ec \
    '/bin/busybox id; /bin/busybox grep -E "^(CapEff|NoNewPrivs|Seccomp):" /proc/self/status; /bin/busybox cat /proc/self/uid_map'
echo NESTED_CONTAINER_AUTHORITY_END
echo DOCKER_IMAGE_INSPECT_BEGIN
docker image inspect guest-proof:latest
echo DOCKER_IMAGE_INSPECT_END
docker save -o /work/image.tar guest-proof:latest
gzip -c /work/image.tar > /work/image.tar.gz
sha256sum /work/image.tar.gz
echo DOCKER_IMAGE_BASE64_BEGIN
base64 /work/image.tar.gz
echo DOCKER_IMAGE_BASE64_END
sed -i 's/echo multikernel-docker-build/exit 37; echo multikernel-docker-build/' /work/context/Dockerfile
if docker build -t guest-proof:must-fail /work/context; then
    echo FAILED_BUILD_UNEXPECTEDLY_PASSED
    exit 1
fi
echo FAILED_BUILD_REJECTED
docker image rm guest-proof:latest guest-proof:cached
docker builder prune --all --force
test -z "$(docker ps -aq)"
echo DOCKER_TEST_RESOURCES_REMOVED
echo BUILD_GUEST_PROBE_END
