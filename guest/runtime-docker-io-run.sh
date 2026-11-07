#!/bin/sh
# Executes inside one original BuildKit RUN, on its child-local writable layer.
set -eu
bb=/bin/busybox
"$bb" mkdir -p /out
start=$("$bb" cut -d. -f1 /proc/uptime)
deadline=$((start + 45))
sequence=0
last_tick=-1
echo G8_OVERLAP_RUN_BEGIN
while :; do
    # Fixed live footprint; fsync completes before any progress is published.
    "$bb" dd if=/dev/zero of=/out/io.bin bs=1048576 count=4 conv=fsync >/dev/null 2>&1
    test "$("$bb" stat -c %s /out/io.bin)" = 4194304
    sequence=$((sequence + 1))
    tick=$("$bb" cut -d. -f1 /proc/uptime)
    if [ "$tick" -ne "$last_tick" ]; then
        echo "G8_IO_SYNCED_SEQ=$sequence"
        last_tick=$tick
    fi
    [ "$tick" -lt "$deadline" ] || break
    # An independent count ceiling also bounds a frozen guest clock.
    [ "$sequence" -lt 1048576 ] || exit 78
done
echo "G8_IO_FINAL_SEQ=$sequence"
echo "G8_IO_FINAL_SHA256=$("$bb" sha256sum /out/io.bin | "$bb" cut -d' ' -f1)"
echo G8_OVERLAP_RUN_END
echo multikernel-production-docker-build > /out/result
