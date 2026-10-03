#!/usr/bin/env bash
set -euo pipefail
set -x

source_root=${1:?usage: test-runtime-initramfs-repro-live.sh SOURCE_ROOT}
builder=$source_root/scripts/build-runtime-rootfs.py
verifier=$source_root/scripts/verify-runtime-rootfs.py
suite=$source_root/scripts/test-runtime-rootfs-build.py

for path in "$builder" "$verifier" "$suite"; do
	test -f "$path" && test ! -L "$path"
done

work_root=${MK_REPRO_WORK_ROOT:-/var/tmp}
work=$(mktemp -d "$work_root"/mk-initramfs-repro.XXXXXX)
cleanup() {
	rm -rf -- "$work"
}
trap cleanup EXIT

root=$work/root
mkdir -p "$root/bin" "$root/etc"
printf 'payload\n' >"$root/bin/program"
chmod 0751 "$root/bin/program"
ln "$root/bin/program" "$root/bin/program-link"
ln -s bin/program "$root/program"
printf 'configuration\n' >"$root/etc/value"
chmod 0640 "$root/etc/value"
python3 - "$root/sparse" <<'PY'
import os
import sys

with open(sys.argv[1], "wb") as stream:
    stream.write(b"prefix")
    stream.seek((1 << 20) + len(b"prefix"))
    stream.write(b"suffix")
os.utime(sys.argv[1], ns=(1_000_000_000, 1_000_000_000))
PY

first_archive=$work/first.cpio.gz
first_manifest=$work/first.manifest.json
second_archive=$work/second.cpio.gz
second_manifest=$work/second.manifest.json
changed_archive=$work/changed.cpio.gz
changed_manifest=$work/changed.manifest.json

first_result=$("$builder" "$root" "$first_archive" "$first_manifest")
printf 'FIRST_RESULT %s\n' "$first_result"
first_verified=$("$verifier" "$first_archive" "$first_manifest")
printf 'FIRST_VERIFIED %s\n' "$first_verified"

python3 - "$root/sparse" <<'PY'
import os
import sys

path = sys.argv[1]
with open(path, "rb") as stream:
    data = stream.read()
with open(path, "wb") as stream:
    stream.write(data)
os.utime(path, ns=(9_000_000_000, 9_000_000_000))
PY
touch -d '@17' "$root/bin/program" "$root/etc/value"

second_result=$("$builder" "$root" "$second_archive" "$second_manifest")
printf 'SECOND_RESULT %s\n' "$second_result"
second_verified=$("$verifier" "$second_archive" "$second_manifest")
printf 'SECOND_VERIFIED %s\n' "$second_verified"

cmp "$first_archive" "$second_archive"
cmp "$first_manifest" "$second_manifest"
first_archive_sha=$(sha256sum "$first_archive" | awk '{print $1}')
second_archive_sha=$(sha256sum "$second_archive" | awk '{print $1}')
first_manifest_sha=$(sha256sum "$first_manifest" | awk '{print $1}')
second_manifest_sha=$(sha256sum "$second_manifest" | awk '{print $1}')
[[ $first_archive_sha = "$second_archive_sha" ]]
[[ $first_manifest_sha = "$second_manifest_sha" ]]
python3 - "$first_manifest" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as stream:
    value = json.load(stream)
normalization = value["normalization"]
assert normalization == {
    "archive": "cpio-newc",
    "device_nodes": "rejected",
    "inode_assignment": "lexical-path-with-hardlink-groups",
    "mtime": 0,
    "overlay_opacity": "materialized-view-normalized",
    "sparse_extents": "normalized-to-regular-bytes",
    "xattrs": "rejected-except-realized-overlay-opacity",
}
entries = {item["path"]: item for item in value["entries"]}
assert entries["bin/program"]["mode"] & 0o777 == 0o751
assert entries["etc/value"]["mode"] & 0o777 == 0o640
assert entries["bin/program"]["hardlink"] == entries["bin/program-link"]["hardlink"]
assert entries["program"]["target"] == "bin/program"
print("MANIFEST_NORMALIZATION_PASS entries=%d" % len(entries))
PY
printf 'REPRODUCIBLE archive_sha256=%s manifest_sha256=%s\n' "$first_archive_sha" "$first_manifest_sha"

printf 'changed\n' >"$root/etc/value"
"$builder" "$root" "$changed_archive" "$changed_manifest"
changed_archive_sha=$(sha256sum "$changed_archive" | awk '{print $1}')
changed_manifest_sha=$(sha256sum "$changed_manifest" | awk '{print $1}')
[[ $changed_archive_sha != "$first_archive_sha" ]]
[[ $changed_manifest_sha != "$first_manifest_sha" ]]
printf 'CHANGED_CONTROL archive_sha256=%s manifest_sha256=%s\n' "$changed_archive_sha" "$changed_manifest_sha"

TMPDIR=$work python3 "$suite"
echo G4_INITRAMFS_REPRO_LIVE_PASS
