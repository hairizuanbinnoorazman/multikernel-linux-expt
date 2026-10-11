#!/usr/bin/env bash
set -euxo pipefail

archive=/tmp/mksrc-87ef4fb.tar
source_root=/var/tmp/mksrc-87ef4fb-v2
test_tmp=/var/tmp/mk-go-tmp-87ef4fb
revision=87ef4fbcf47f7dc41c17c774a5490b3fa2b1312a
archive_sha=4072bbe1c6f2b79bf93074c5276b9466a1288f819b95217eb8c8981bde924cf5

echo G6_RECONNECT_CANDIDATE_BUILD_V2_BEGIN
date -u +%FT%TZ
printf 'boot_id=%s\n' "$(cat /proc/sys/kernel/random/boot_id)"
test "$(stat -c %a "$archive")" = 600
test "$(stat -c %s "$archive")" = 7311360
test "$(sha256sum "$archive" | awk '{print $1}')" = "$archive_sha"
test "$(findmnt -n -o FSTYPE -T /var/tmp)" = ext4
test ! -e "$source_root"
test ! -e "$test_tmp"
install -d -m 0755 "$source_root"
install -d -m 0700 -o "$SUDO_UID" -g "$SUDO_GID" "$test_tmp"
trap 'rm -rf -- "$test_tmp"' EXIT
tar -xf "$archive" -C "$source_root"
chown -R "$SUDO_UID:$SUDO_GID" "$source_root"
chmod -R go-w "$source_root"

sudo -u "$SUDO_USER" env TMPDIR="$test_tmp" GOCACHE=/tmp/mk-go-cache bash -c \
  "cd '$source_root/runtime' && go test -race ./..."
sudo -u "$SUDO_USER" env TMPDIR="$test_tmp" make -C "$source_root" runtime-manifest \
  RUNTIME_VERSION=0.1.0-dev RUNTIME_REVISION="$revision"

test "$(stat -c %a "$source_root/runtime/release-manifest.json")" = 644
python3 "$source_root/scripts/runtime-release-manifest.py" \
  "$source_root/runtime/bin" "$test_tmp/release-manifest-duplicate.json" \
  --expected-version=0.1.0-dev --expected-revision="$revision"
cmp "$source_root/runtime/release-manifest.json" "$test_tmp/release-manifest-duplicate.json"
for binary in "$source_root"/runtime/bin/*; do
  "$binary" --version
done
sha256sum "$source_root"/runtime/bin/* "$source_root/runtime/release-manifest.json"
echo G6_RECONNECT_CANDIDATE_BUILD_V2_PASS
