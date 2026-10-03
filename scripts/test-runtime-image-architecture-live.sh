#!/usr/bin/env bash
set -euo pipefail
set -x

source_root=${1:?usage: test-runtime-image-architecture-live.sh SOURCE_ROOT [KERNEL_MANIFEST]}
kernel_manifest=${2:-/etc/mkruntime/kernels/gce-mk2.json}
bootstrap_validator=$source_root/scripts/validate-runtime-bootstrap.py
image_validator=$source_root/scripts/validate-runtime-image.py
image_suite=$source_root/scripts/test-runtime-image-validation.py
for path in "$bootstrap_validator" "$image_validator" "$image_suite"; do
	test -f "$path" && test ! -L "$path"
done

work_root=${MK_IMAGE_ARCH_WORK_ROOT:-/var/tmp}
work=$(mktemp -d "$work_root"/mk-image-architecture.XXXXXX)
cleanup() {
	rm -rf -- "$work"
}
trap cleanup EXIT

bootstrap=$work/bootstrap.json
"$bootstrap_validator" "$kernel_manifest" gce-mk2 | tee "$bootstrap"
python3 - "$bootstrap" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as stream:
    value = json.load(stream)
assert value["architecture"] == "amd64"
assert len(value["manifest_sha256"]) == 64
assert value["kernel_release"]
assert value["required_config"]
assert value["oci_features"]
print("SELECTED_KERNEL architecture=amd64 manifest_sha256=%s kernel_release=%s required_config_count=%d oci_feature_count=%d" % (
    value["manifest_sha256"], value["kernel_release"], len(value["required_config"]), len(value["oci_features"])
))
PY

root=$work/root
mkdir -p "$root/bin"
busybox=$(command -v busybox)
install -m 0755 "$busybox" "$root/bin/busybox"
config=$work/config.json
printf '{"process":{"args":["/bin/busybox"]}}\n' >"$config"
image_result=$work/image-result.json
"$image_validator" "$root" "$config" "$bootstrap" | tee "$image_result"
python3 - "$bootstrap" "$image_result" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as stream:
    bootstrap = json.load(stream)
with open(sys.argv[2], encoding="utf-8") as stream:
    image = json.load(stream)
assert image["architecture"] == "amd64"
assert image["kernel_manifest_sha256"] == bootstrap["manifest_sha256"]
assert image["kernel_release"] == bootstrap["kernel_release"]
assert image["required_kernel_config"] == bootstrap["required_config"]
assert image["supported_oci_features"] == bootstrap["oci_features"]
assert len(image["entrypoint_sha256"]) == 64
print("IMAGE_KERNEL_BINDING_PASS entrypoint_sha256=%s kernel_manifest_sha256=%s" % (
    image["entrypoint_sha256"], image["kernel_manifest_sha256"]
))
PY

python3 - "$root/bin/wrong-architecture" <<'PY'
import os
import sys

with open(sys.argv[1], "wb") as stream:
    stream.write(b"\x7fELF\x02\x01" + b"\0" * 12 + (183).to_bytes(2, "little"))
os.chmod(sys.argv[1], 0o755)
PY
printf '{"process":{"args":["/bin/wrong-architecture"]}}\n' >"$config"
set +e
wrong_output=$("$image_validator" "$root" "$config" "$bootstrap" 2>&1)
wrong_status=$?
set -e
[[ $wrong_status -ne 0 ]]
grep -Fq 'not a little-endian x86-64 ELF image' <<<"$wrong_output"
printf 'WRONG_ARCHITECTURE_REJECTED status=%s diagnostic=%s\n' "$wrong_status" "$wrong_output"

TMPDIR=$work python3 "$image_suite"
echo G4_IMAGE_ARCHITECTURE_LIVE_PASS
