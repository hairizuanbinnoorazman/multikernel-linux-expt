#!/usr/bin/env bash
set -euxo pipefail

archive=/tmp/mksrc-d7c5301.tar
source_root=/var/tmp/mksrc-d7c5301
test_tmp=/var/tdd
revision=d7c530103b43c63d97518b0d6f5aa5442e7b15a3
archive_sha=d8dae4a4f7669c600a42921433918d84d9d6b881d7574f8e0a0fa0f4d7ed25d5

echo G6_STREAM_KEEPER_CANDIDATE_BUILD_BEGIN
date -u +%FT%TZ
printf 'boot_id=%s\n' "$(cat /proc/sys/kernel/random/boot_id)"
test "$(stat -c %a "$archive")" = 600
test "$(stat -c %s "$archive")" = 7321600
test "$(sha256sum "$archive" | awk '{print $1}')" = "$archive_sha"
test "$(findmnt -n -o FSTYPE -T /var)" = ext4
test ! -e "$source_root"
test ! -e "$test_tmp"
test ! -e "/usr/local/lib/multikernel/releases/0.1.0-dev-$revision"
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
echo G6_STREAM_KEEPER_CANDIDATE_BUILD_PASS
