#!/usr/bin/env bash
set -euo pipefail

if [[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]]; then
	PS4='+${BASH_SOURCE}:${LINENO}: '
	set -x
fi

source_root=${1:?usage: test-runtime-network-stress-live.sh SOURCE_ROOT}
task_id=mk-network-stress
image=${MK_TEST_IMAGE:-docker.io/library/busybox:1.36}
runtime=${MK_RUNTIME:-io.containerd.multikernel.v2}
scratch=$(mktemp -d -p /var/tmp mk-network-stress.XXXXXX)
server_log=$scratch/server.json
server_pid=

observe() {
	local key=$1
	shift
	printf 'OBSERVATION_BEGIN key=%s\n%s\nOBSERVATION_END key=%s\n' "$key" "$*" "$key"
}

cleanup_task() {
	local state=
	sudo ctr tasks kill --signal SIGKILL "$task_id" >/dev/null 2>&1 || true
	for _ in $(seq 1 120); do
		state=$(sudo ctr tasks list | awk -v id="$task_id" '$1 == id {print $3}')
		[[ $state != RUNNING && $state != PAUSED ]] && break
		sleep .25
	done
	sudo ctr tasks rm -f "$task_id" >/dev/null 2>&1 || true
	sudo ctr containers rm "$task_id" >/dev/null 2>&1 || true
}

cleanup() (
	set +e
	[[ -z $server_pid ]] || kill "$server_pid" >/dev/null 2>&1 || true
	[[ -z $server_pid ]] || wait "$server_pid" >/dev/null 2>&1 || true
	cleanup_task
	rm -rf -- "$scratch"
)
trap cleanup EXIT

wait_task_state() {
	local expected=$1 state=
	for _ in $(seq 1 240); do
		state=$(sudo ctr tasks list | awk -v id="$task_id" '$1 == id {print $3}')
		[[ $state = "$expected" ]] && return 0
		sleep .25
	done
	return 1
}

live_counts() {
	sudo python3 - <<'PY'
import json, os
def count(path, key, predicate=lambda value: True):
    if not os.path.exists(path):
        return 0
    with open(path, encoding='utf-8') as stream:
        return sum(predicate(value) for value in json.load(stream)[key].values())
print('rootfs_records=%d live_exports=%d endpoints=%d' % (
    count('/var/lib/mkruntimed/rootfs/state.json','records'),
    count('/var/lib/mkruntimed/storage/state.json','exports',lambda v:v['state']!='RELEASED'),
    count('/var/lib/mknetd/state.json','endpoints')))
PY
}

wait_counts() {
	local expected=$1 observed=
	for _ in $(seq 1 240); do
		observed=$(live_counts)
		[[ $observed = "$expected" ]] && { printf '%s' "$observed"; return 0; }
		sleep .25
	done
	printf '%s' "$observed"
	return 1
}

endpoint_summary() {
	sudo python3 - <<'PY'
import json,re
with open('/var/lib/mknetd/state.json', encoding='utf-8') as stream:
    state=json.load(stream)
assert state['version'] == 1 and len(state['endpoints']) == 1, state
value=next(iter(state['endpoints'].values()))
assert value['container_id'] == 'mk-network-stress', value
assert value['state'] in ('READY','DEGRADED'), value
assert value['owner'] == 'runtime' and value['managed_namespace'] is True, value
assert re.fullmatch(r'[0-9a-f]{32}',value['generation']), value
print(json.dumps({key:value[key] for key in (
 'container_id','sandbox_id','sandbox_generation','generation','address','gateway','mtu','dns',
 'state','rx_packets','tx_packets','rx_drops','tx_drops','errors')},sort_keys=True,separators=(',',':')))
PY
}

[[ $(id -u) -ne 0 ]] || { echo 'run as an ordinary sudo-capable user' >&2; exit 1; }
test -x "$source_root/scripts/audit-runtime-final-resources-live.sh"
for service in mkruntimed mknetd containerd docker; do
	[[ $(systemctl is-active "$service") = active ]]
	[[ $(systemctl show -p NRestarts --value "$service") = 0 ]]
done
cleanup_task
observe clean-before "$(wait_counts 'rootfs_records=0 live_exports=0 endpoints=0')"
observe provenance "boot_id=$(cat /proc/sys/kernel/random/boot_id)
selector=$(readlink -f /usr/local/lib/multikernel/current)
mkruntimed_sha256=$(sudo sha256sum /proc/$(systemctl show -p MainPID --value mkruntimed)/exe | awk '{print $1}')
mknetd_sha256=$(sudo sha256sum /proc/$(systemctl show -p MainPID --value mknetd)/exe | awk '{print $1}')
source_test_sha256=$(sha256sum "$source_root/runtime/cmd/containerd-shim-multikernel-v2/main_test.go" | awk '{print $1}')
qualifier_sha256=$(sha256sum "$0" | awk '{print $1}')"

guest_program='set -eu; touch /tmp/network-stress-ready; while [ ! -e /tmp/network-stress-release ]; do sleep 1; done'
sudo ctr run --detach --runtime "$runtime" "$image" "$task_id" /bin/sh -c "$guest_program"
wait_task_state RUNNING
for _ in $(seq 1 120); do
	sudo ctr task exec --exec-id "stress-ready-$RANDOM" "$task_id" /bin/test -e /tmp/network-stress-ready >/dev/null 2>&1 && break
	sleep .25
done
sudo ctr task exec --exec-id stress-ready-final "$task_id" /bin/test -e /tmp/network-stress-ready
wait_counts 'rootfs_records=1 live_exports=1 endpoints=1' >/dev/null

endpoint_before=$(endpoint_summary)
observe endpoint-before-stress "$endpoint_before"
python3 - "$endpoint_before" <<'PY'
import json,sys
value=json.loads(sys.argv[1])
assert value['mtu'] == 1400, value
assert value['errors'] == value['rx_drops'] == value['tx_drops'] == 0, value
PY
primary_ip=$(ip -4 route get 8.8.8.8 | awk '{for(i=1;i<=NF;i++) if($i=="src") {print $(i+1); exit}}')
[[ $primary_ip =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]

exact_mtu=$(sudo ctr task exec --exec-id stress-exact-mtu "$task_id" /bin/ping -c 5 -W 2 -s 1372 "$primary_ip")
grep -Eq '5 packets transmitted, 5 packets received|5 packets transmitted, 5 received' <<<"$exact_mtu"
grep -Fq '0% packet loss' <<<"$exact_mtu"
observe exact-negotiated-mtu-icmp "destination=$primary_ip payload_bytes=1372 ipv4_total_bytes=1400
$exact_mtu"

fragmented=$(sudo ctr task exec --exec-id stress-fragmented "$task_id" /bin/ping -c 3 -W 3 -s 3000 "$primary_ip")
grep -Eq '3 packets transmitted, 3 packets received|3 packets transmitted, 3 received' <<<"$fragmented"
grep -Fq '0% packet loss' <<<"$fragmented"
observe fragmented-icmp-reassembly "destination=$primary_ip payload_bytes=3000 exceeds_mtu=1400
$fragmented"

burst=$(sudo ctr task exec --exec-id stress-burst "$task_id" /bin/ping -c 256 -i 0.01 -W 2 -s 64 "$primary_ip")
grep -Eq '256 packets transmitted, 256 packets received|256 packets transmitted, 256 received' <<<"$burst"
grep -Fq '0% packet loss' <<<"$burst"
! grep -Fq 'DUP!' <<<"$burst"
observe burst-loss-order "destination=$primary_ip count=256 interval_seconds=0.01 payload_bytes=64 duplicate_marker=absent
$burst"

server_port=18083
python3 - "$primary_ip" "$server_port" "$server_log" <<'PY' &
import hashlib,json,socket,sys,time
host,port,path=sys.argv[1],int(sys.argv[2]),sys.argv[3]
expected=1<<20
with socket.socket() as server:
    server.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1)
    server.bind((host,port)); server.listen(1); server.settimeout(45)
    conn,peer=server.accept()
    with conn:
        conn.settimeout(30)
        time.sleep(1.0)
        data=bytearray()
        while len(data) < expected:
            block=conn.recv(min(65536,expected-len(data)))
            assert block, len(data)
            data.extend(block)
        assert len(data) == expected and set(data) == {0}, (len(data),set(data))
        digest=hashlib.sha256(data).hexdigest()
        reply=f'PRIMARY_LOAD_OK bytes={len(data)} sha256={digest}'
        conn.sendall(reply.encode())
with open(path,'w',encoding='utf-8') as stream:
    json.dump({'destination':f'{host}:{port}','peer':peer[0],'peer_port':peer[1],
      'bytes':len(data),'sha256':digest,'slow_reader_delay_seconds':1.0,
      'response':reply},stream,sort_keys=True)
PY
server_pid=$!
sleep .25
kill -0 "$server_pid"
load_reply=$(sudo ctr task exec --exec-id stress-tcp-load "$task_id" /bin/sh -c "dd if=/dev/zero bs=4096 count=256 2>/dev/null | nc -w 30 '$primary_ip' '$server_port'")
grep -Eq '^PRIMARY_LOAD_OK bytes=1048576 sha256=[0-9a-f]{64}$' <<<"$load_reply"
wait "$server_pid"
server_pid=
observe sustained-tcp-slow-reader-integrity "child_destination=$primary_ip:$server_port child_response=$load_reply
primary_observation=$(cat "$server_log")"

endpoint_after=
for _ in $(seq 1 60); do
	endpoint_after=$(endpoint_summary)
	if python3 - "$endpoint_before" "$endpoint_after" <<'PY'
import json,sys
before,after=map(json.loads,sys.argv[1:])
assert after['rx_packets'] > before['rx_packets'], (before,after)
assert after['tx_packets'] > before['tx_packets'], (before,after)
PY
	then
		break
	fi
	sleep 1
done
python3 - "$endpoint_before" "$endpoint_after" <<'PY'
import json,sys
before,after=map(json.loads,sys.argv[1:])
for key in ('container_id','sandbox_id','sandbox_generation','generation','address','gateway','mtu','dns'):
    assert before[key] == after[key], (key,before,after)
assert after['rx_packets'] > before['rx_packets'] and after['tx_packets'] > before['tx_packets'], (before,after)
for key in ('rx_drops','tx_drops','errors'):
    assert after[key] == before[key], (key,before,after)
PY
observe endpoint-after-stress "$endpoint_after"

sudo ctr task exec --exec-id stress-release "$task_id" /bin/touch /tmp/network-stress-release
wait_task_state STOPPED
sudo ctr tasks rm "$task_id" >/dev/null
sudo ctr containers rm "$task_id"
observe clean-after-release "$(wait_counts 'rootfs_records=0 live_exports=0 endpoints=0')"

sudo systemctl restart mkruntimed
for _ in $(seq 1 60); do systemctl is-active --quiet mkruntimed && break; sleep 1; done
[[ $(systemctl is-active mkruntimed) = active ]]
audit_output=
for _ in $(seq 1 60); do
	if audit_output=$(MK_EVIDENCE_XTRACE=0 "$source_root/scripts/audit-runtime-final-resources-live.sh" 2>&1); then
		break
	fi
	sleep 1
done
grep -Fq G6_FINAL_RESOURCE_RETURN_PASS <<<"$audit_output"
printf '%s\n' "$audit_output"
for service in mkruntimed mknetd containerd docker; do
	[[ $(systemctl is-active "$service") = active ]]
	[[ $(systemctl show -p NRestarts --value "$service") = 0 ]]
done

trap - EXIT
rm -rf -- "$scratch"
echo G5_NETWORK_STRESS_LIVE_PASS
