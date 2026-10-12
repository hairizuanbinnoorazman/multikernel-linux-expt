#!/usr/bin/env bash
# Disposable-host guest image-build probe; retains production agent restrictions.
set -euo pipefail
: "${MK_SECURITY_DISPOSABLE:?set MK_SECURITY_DISPOSABLE=yes on the disposable host}"
[[ $MK_SECURITY_DISPOSABLE == yes ]]
repo_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
service=docker-online
kerf=${KERF:?set KERF to the pinned executable}
kernel=${KERNEL:?set KERNEL to the pinned vmlinux}
module=${MK_TRANSPORT_MODULE:?}
relay=${MK_RELAY:?}
agent=${MK_AGENT:?}
ctl=${MK_ONLINE_CONTROLLER:?}
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
relay_pid=; console_pid=; token=
forward_before=$(cat /proc/sys/net/ipv4/ip_forward)
network_ready=no
cleanup_network() {
    [[ $network_ready == yes ]] || return 0
    sudo iptables -D INPUT -i mkonline0 -j DROP
    sudo iptables -D FORWARD -i mkonline0 -j MKONLINE7
    sudo iptables -D FORWARD -o mkonline0 -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
    sudo iptables -t nat -D POSTROUTING -s 10.231.7.2/32 -o "$egress" -j MASQUERADE
    sudo iptables -F MKONLINE7
    sudo iptables -X MKONLINE7
    sudo ip link delete mkonline0
    sudo sysctl -w net.ipv4.ip_forward="$forward_before"
    network_ready=no
}
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
        python3 - "$scratch" "$evidence" "${fixture:-setup}" "$token" "$status" <<'PY'
import pathlib,sys,json
scratch,out=map(pathlib.Path,sys.argv[1:3]);fixture,token,status=sys.argv[3:]
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
    cleanup_network
    sudo rm -rf "$scratch/load-bundle"
    rm -rf "$scratch"
    exit "$status"
}
trap cleanup_all EXIT
sudo modprobe vsock
lsmod | grep -q '^mk_transport ' || sudo insmod "$module"
sha256sum "$kernel" "$module" "$relay" "$agent" "$ctl" > "$evidence/component-hashes.txt"
uname -r > "$evidence/kernel-release.txt"
fixture_count=0
for fixture in prepared; do
    [[ $fixture == prepared ]]
    fixture_count=$((fixture_count + 1))
    generation=$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')
    token=$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')
    printf '%s\n' "$token" > "$scratch/token"
    chmod 0600 "$scratch/token"
    initrd=$scratch/$fixture.cpio.gz
    sudo bash "$repo_dir/scripts/build-$service-probe-initramfs.sh" "$agent" "$relay" "$module" "${MK_BUILDKIT_ROOTFS:?}" "$fixture" "$initrd" \
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
    sudo timeout 1500s script -qefc "$kerf exec $name --console" /dev/null > "$scratch/console.log" 2>&1 &
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
    egress=$(ip -4 route show default | awk 'NR==1 {print $5}')
    sudo ip tuntap add dev mkonline0 mode tun
    sudo ip address add 10.231.7.1/30 dev mkonline0
    sudo ip link set mkonline0 mtu 1500 up
    sudo iptables -N MKONLINE7
    sudo iptables -I INPUT -i mkonline0 -j DROP
    sudo iptables -I FORWARD -i mkonline0 -j MKONLINE7
    sudo iptables -I FORWARD -o mkonline0 -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
    sudo iptables -t nat -A POSTROUTING -s 10.231.7.2/32 -o "$egress" -j MASQUERADE
    network_ready=yes
    for blocked in 0.0.0.0/8 10.0.0.0/8 100.64.0.0/10 127.0.0.0/8 169.254.0.0/16 172.16.0.0/12 192.168.0.0/16 224.0.0.0/4; do
        sudo iptables -A MKONLINE7 -d "$blocked" -j REJECT
    done
    sudo iptables -A MKONLINE7 -p tcp --dport 443 -j ACCEPT
    sudo iptables -A MKONLINE7 -p udp -d 8.8.8.8 --dport 53 -j ACCEPT
    sudo iptables -A MKONLINE7 -j DROP
    sudo sysctl -w net.ipv4.ip_forward=1
    sudo iptables-save > "$evidence/primary-network-rules.txt"
    sudo timeout 1500s "$ctl" --socket="$scratch/relay.sock" \
        --generation="$generation" --token-file="$scratch/token" \
        > "$scratch/result.json" 2> "$scratch/control.err" || control_status=$?
    python3 - "$scratch" "$evidence" "$fixture" "$token" "$control_status" <<'PY'
import pathlib,sys,json
scratch,out,fixture=map(pathlib.Path,sys.argv[1:4]);token=sys.argv[4]
status=int(sys.argv[5])
assert status==0,status
raw=json.loads((scratch/'result.json').read_text())
result=raw['WaitProcess']
assert result['status']=='STOPPED',result['status']
# ProcessState omits a zero exit_code from its JSON representation.
assert result.get('exit_code',0)==0,result.get('exit_code')
assert 'ONLINE_DOCKER_PROBE_END' in result['stdout']
assert token not in json.dumps(raw), 'probe printed a credential'
assert 'NoNewPrivs:\t1' in result['stdout']
for field in ['KERNEL_COMMAND_LINE_TOKEN_VISIBLE','AGENT_ARGUMENT_TOKEN_VISIBLE',
              'CHILD_BOOTSTRAP_CANARY_READ','CHILD_BOOTSTRAP_CANARY_WRITE']:
    assert field+'=no' in result['stdout']
if str(fixture)=='prepared':
    assert 'ONLINE_PYTHON_BUILD_PASSED' in result['stdout']
    assert 'ONLINE_PYTHON_CACHE_PASSED' in result['stdout']
    assert 'ONLINE_PYTHON_RUN_PASSED' in result['stdout']
    assert 'CapEff:\t0000000000000000' in result['stdout']
(out/(str(fixture)+'-result.json')).write_text(json.dumps(raw,indent=2)+'\n')
for name in ['console.log','control.err','relay.log','kerf.log']:
    data=(scratch/name).read_text(errors='replace').replace(token,'[REDACTED_TEST_TOKEN]')
    (out/(str(fixture)+'-'+name)).write_text(data)
PY
    cleanup_fixture > "$evidence/$fixture-cleanup.txt" 2>&1
    cleanup_network > "$evidence/network-cleanup.txt" 2>&1
    sudo iptables-save > "$evidence/primary-network-rules-after.txt"
    ! ip link show mkonline0 2>/dev/null
    ! sudo iptables-save | grep -E "MKONLINE7|mkonline0|10\.231\.7\.2"
    rm -f "$scratch/relay.sock"
    [[ $(cat /sys/devices/system/cpu/online) == "$before_cpus" ]]
    [[ ! -d /sys/fs/multikernel/instances/$name ]]
    echo "ONLINE_DOCKER_FIXTURE_COMPLETED case=$fixture"
done
printf '{"online_cpus":"%s","active_test_children":0,"fixture_count":%s}\n' "$before_cpus" "$fixture_count" > "$evidence/final-inventory.json"
echo ONLINE_DOCKER_COLLECTION_COMPLETED
