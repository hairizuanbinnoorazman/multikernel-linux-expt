#!/bin/sh
set -eu
echo BUILD_GUEST_PROBE_BEGIN
id
uname -r
grep -E '^(CapEff|CapBnd|NoNewPrivs):' /proc/self/status
cat /proc/self/uid_map
buildkitd --version
rootlesskit --version
if [ -f /authority-probe.sh ]; then
    /bin/sh /authority-probe.sh
fi
if [ "${MK_BUILD_PROBE_MODE:-baseline}" = baseline ]; then
    set +e
    timeout 15 rootlesskit buildkitd --rootless --oci-worker-snapshotter=native \
        --oci-worker-net=host --addr unix:///run/user/1000/buildkit/buildkitd.sock \
        >/work/baseline.log 2>&1
    result=$?
    set -e
    cat /work/baseline.log
    echo "BASELINE_START_EXIT=$result"
    echo BUILD_GUEST_PROBE_END
    exit 0
fi
mkdir -p /work/context
cp /usr/local/bin/build-probe-busybox /work/context/busybox
cat > /work/context/Dockerfile <<'DOCKERFILE'
FROM scratch AS build
COPY busybox /bin/busybox
RUN ["/bin/busybox", "sh", "-ec", "/bin/busybox mkdir -p /out; echo multikernel-guest-build > /out/result; /bin/busybox chmod 0644 /out/result"]
FROM scratch
COPY --from=build /out/result /result
DOCKERFILE
mk-build /work/context /work/image.tar
test -s /work/image.tar
sha256sum /work/image.tar
echo OCI_IMAGE_BASE64_BEGIN
base64 /work/image.tar
echo OCI_IMAGE_BASE64_END
echo MULTISTAGE_BUILD_PASSED
mk-build /work/context /work/image-cached.tar
echo REPEAT_BUILD_PASSED
sed -i 's/echo multikernel-guest-build/exit 37; echo multikernel-guest-build/' /work/context/Dockerfile
if mk-build /work/context /work/should-fail.tar; then
    echo FAILED_BUILD_UNEXPECTEDLY_PASSED
    exit 1
fi
echo FAILED_BUILD_REJECTED
test ! -e /dev/mem
test ! -e /dev/port
test ! -e /dev/nbd0
echo RESTRICTED_DEVICES_PASSED
echo BUILD_GUEST_PROBE_END
