#!/usr/bin/env bash
# Privileged, disposable-host-only diagnostic. No hostile-kernel memory probe.
set -euo pipefail
: "${MK_SECURITY_DISPOSABLE:?set MK_SECURITY_DISPOSABLE=yes on the disposable host}"
[[ $MK_SECURITY_DISPOSABLE == yes ]]
repo_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
kerf=${KERF:?set KERF to the pinned executable}
kernel=${KERNEL:?set KERNEL to the pinned vmlinux}
module=${MK_TRANSPORT_MODULE:?}
relay=${MK_RELAY:?}
agent=${MK_AGENT:?}
ctl=${MK_AGENTCTL:?}
load_helper=${MK_SECURITY_LOAD:?provide the production credential-handoff diagnostic helper}
evidence=${EVIDENCE_DIR:?}
mkdir -p "$evidence"
[[ $(uname -r) == 7.0.0-mk2-gce-lab ]]
[[ -d /sys/fs/multikernel/instances ]]
[[ -z $(find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 -type d -print -quit) ]]
before_cpus=$(cat /sys/devices/system/cpu/online)
[[ $before_cpus == 0-15 ]]
scratch=$(mktemp -d)
chmod 0700 "$scratch"
name=runtime-g7-authority
port=7717
relay_pid=; console_pid=
cleanup_fixture() {
    local status
    if [[ -n $relay_pid ]]; then kill "$relay_pid" 2>/dev/null || true; wait "$relay_pid" 2>/dev/null || true; relay_pid=; fi
    if [[ -d /sys/fs/multikernel/instances/$name ]]; then
        status=$(cat "/sys/fs/multikernel/instances/$name/status")
        [[ $status != active ]] || sudo "$kerf" kill "$name" --force
        status=$(cat "/sys/fs/multikernel/instances/$name/status")
        [[ $status != loaded ]] || sudo "$kerf" unload "$name"
        sudo "$kerf" delete "$name"
    fi
    if [[ -n $console_pid ]]; then wait "$console_pid" 2>/dev/null || true; console_pid=; fi
    sudo "$kerf" init --cpus=none --memory=none --devices=none
}
cleanup_all() {
    local status=$?
    trap - EXIT
    set +e
    if [[ $status != 0 ]]; then
        python3 - "$scratch" "$evidence" "${fixture:-setup}" "$scratch/token" "$status" <<'PY'
import pathlib,sys,json
scratch,out=map(pathlib.Path,sys.argv[1:3]);fixture,token_path,status=sys.argv[3:]
token_file=pathlib.Path(token_path)
token=token_file.read_text().strip() if token_file.exists() else ''
for name in ['console.log','control.err','relay.log','kerf.log','result.json']:
    if (scratch/name).exists():
        data=(scratch/name).read_text(errors='replace')
        if token:
            data=data.replace(token,'[REDACTED_TEST_TOKEN]')
        (out/(fixture+'-failed-'+name)).write_text(data)
(out/'failed-attempt.json').write_text(json.dumps({'fixture':fixture,'exit_status':int(status)})+'\n')
PY
    fi
    cleanup_fixture
    sudo rm -rf "$scratch/load-bundle"
    rm -rf "$scratch"
    exit "$status"
}
trap cleanup_all EXIT
sudo modprobe vsock
lsmod | grep -q '^mk_transport ' || sudo insmod "$module"
sha256sum "$kernel" "$module" "$relay" "$agent" "$ctl" > "$evidence/component-hashes.txt"
uname -r > "$evidence/kernel-release.txt"
for fixture in nonroot root-empty-caps root-default root-ordinary-caps root-unsafe-cap root-disable-nnp; do
    generation=$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')
    od -An -N32 -tx1 /dev/urandom | tr -d ' \n' > "$scratch/token"
    printf '\n' >> "$scratch/token"
    chmod 0600 "$scratch/token"
    initrd=$scratch/$fixture.cpio.gz
    bash "$repo_dir/scripts/build-security-authority-initramfs.sh" "$agent" "$relay" "$module" "$fixture" "$initrd" \
        > "$evidence/$fixture-initramfs.txt"
    sudo "$kerf" init --cpus=8,10,12,14 --memory=8GB --devices=none > "$scratch/kerf.log" 2>&1
    sudo "$kerf" create "$name" --id=27 --cpus=8,10 --memory=4GB >> "$scratch/kerf.log" 2>&1
    sudo install -d -m 0700 "$scratch/load-bundle/.multikernel"
    sudo install -m 0600 "$initrd" "$scratch/load-bundle/.multikernel/initramfs.cpio.gz"
    sudo install -m 0600 "$scratch/token" "$scratch/load-bundle/.multikernel/token"
    printf '%s\n' "$scratch/load-bundle/.multikernel/initramfs.cpio.gz" | sudo tee "$scratch/load-bundle/.multikernel/initramfs.path" >/dev/null
    sudo chmod 0600 "$scratch/load-bundle/.multikernel/initramfs.path"
    # Hash the actual sealed, credential-bearing image passed to pinned Kerf.
    {
        printf '%s\n' '#!/bin/sh' 'set -eu'
        printf 'if [ "$1" = load ]; then sha256sum /proc/self/fd/3 > "%s"; fi\n' "$evidence/$fixture-loaded-initramfs.sha256"
        printf 'exec "%s" "$@"\n' "$kerf"
    } > "$scratch/load-wrapper"
    sudo install -m 0700 "$scratch/load-wrapper" "$scratch/load-bundle/load-wrapper"
    sudo env MK_SECURITY_DISPOSABLE=yes "$load_helper" --kerf="$scratch/load-bundle/load-wrapper" \
        --kernel="$kernel" --bundle="$scratch/load-bundle" --generation="$generation" >> "$scratch/kerf.log" 2>&1
    "$relay" server "$port" "$scratch/relay.sock" > "$scratch/relay.log" 2>&1 &
    relay_pid=$!
    sudo timeout 60s script -qefc "$kerf exec $name --console" /dev/null > "$scratch/console.log" 2>&1 &
    console_pid=$!
    # The relay creates its Unix listener after the child connects over vsock.
    for ((attempt=0; attempt<300; attempt++)); do
        [[ ! -S $scratch/relay.sock ]] || break
        kill -0 "$relay_pid" 2>/dev/null || break
        sleep 0.1
    done
    [[ -S $scratch/relay.sock ]]
    # The probe emits only booleans for credential visibility. Raw console can
    # contain the boot command line; redact before moving it into evidence.
    control_status=0
    timeout 60s "$ctl" --port="$port" --unix-socket="$scratch/relay.sock" \
        --sandbox-id="$name" --generation="$generation" --token-file="$scratch/token" --bundle=/bundle \
        > "$scratch/result.json" 2> "$scratch/control.err" || control_status=$?
    python3 - "$scratch" "$evidence" "$fixture" "$scratch/token" "$control_status" <<'PY'
import pathlib,sys,json
scratch,out,fixture=map(pathlib.Path,sys.argv[1:4]);token=pathlib.Path(sys.argv[4]).read_text().strip()
status=int(sys.argv[5])
if str(fixture) in ['root-unsafe-cap','root-disable-nnp']:
    assert status==1,status
    assert 'CreateProcess: INTERNAL: guest process creation failed at bundle-load' in (scratch/'control.err').read_text()
    (out/(str(fixture)+'-rejection.json')).write_text(json.dumps({'case':str(fixture),'client_exit_status':status,'rejected_before_start':True})+'\n')
    for name in ['console.log','control.err','relay.log','kerf.log']:
        (out/(str(fixture)+'-'+name)).write_text((scratch/name).read_text(errors='replace').replace(token,'[REDACTED_TEST_TOKEN]'))
    sys.exit(0)
assert status==0,status
raw=json.loads((scratch/'result.json').read_text())
result=raw['WaitProcess']
assert result['status']=='STOPPED',result['status']
# ProcessState omits a zero exit_code from its JSON representation.
assert result.get('exit_code',0)==0,result.get('exit_code')
assert 'SECURITY_AUTHORITY_END' in result['stdout']
assert token not in json.dumps(raw), 'probe printed a credential'
assert 'KERNEL_COMMAND_LINE_TOKEN_VISIBLE=no' in result['stdout']
assert 'AGENT_ARGUMENT_TOKEN_VISIBLE=no' in result['stdout']
assert 'CHILD_BOOTSTRAP_CANARY_READ=no' in result['stdout']
assert 'CHILD_BOOTSTRAP_CANARY_WRITE=no' in result['stdout']
assert 'NoNewPrivs:\t1' in result['stdout']
(out/(str(fixture)+'-result.json')).write_text(json.dumps(raw,indent=2)+'\n')
for name in ['console.log','control.err','relay.log','kerf.log']:
    data=(scratch/name).read_text(errors='replace').replace(token,'[REDACTED_TEST_TOKEN]')
    (out/(str(fixture)+'-'+name)).write_text(data)
PY
    cleanup_fixture > "$evidence/$fixture-cleanup.txt" 2>&1
    rm -f "$scratch/relay.sock"
    [[ $(cat /sys/devices/system/cpu/online) == "$before_cpus" ]]
    [[ ! -d /sys/fs/multikernel/instances/$name ]]
    echo "SECURITY_AUTHORITY_FIXTURE_COMPLETED case=$fixture"
done
printf '{"online_cpus":"%s","active_test_children":0,"fixture_count":6}\n' "$before_cpus" > "$evidence/final-inventory.json"
echo SECURITY_AUTHORITY_COLLECTION_COMPLETED
