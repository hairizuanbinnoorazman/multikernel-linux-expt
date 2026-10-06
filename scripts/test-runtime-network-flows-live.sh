#!/usr/bin/env bash
set -euo pipefail

if [[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]]; then
	PS4='+${BASH_SOURCE}:${LINENO}: '
	set -x
fi

source_root=${1:?usage: test-runtime-network-flows-live.sh SOURCE_ROOT}
task_id=mk-network-flows
image=${MK_TEST_IMAGE:-docker.io/library/busybox:1.36}
runtime=${MK_RUNTIME:-io.containerd.multikernel.v2}
scratch=$(mktemp -d -p /var/tmp mk-network-flows.XXXXXX)
tcp_log=$scratch/tcp.json
udp_log=$scratch/udp.json
tcp_pid=
udp_pid=

observe() {
	local key=$1
	shift
	printf 'OBSERVATION_BEGIN key=%s\n%s\nOBSERVATION_END key=%s\n' "$key" "$*" "$key"
}

cleanup_task() {
	sudo ctr tasks kill --signal SIGKILL "$task_id" >/dev/null 2>&1 || true
	for _ in $(seq 1 120); do
		local state
		state=$(sudo ctr tasks list | awk -v id="$task_id" '$1 == id {print $3}')
		[[ $state != RUNNING && $state != PAUSED ]] && break
		sleep .25
	done
	sudo ctr tasks rm -f "$task_id" >/dev/null 2>&1 || true
	sudo ctr containers rm "$task_id" >/dev/null 2>&1 || true
}

cleanup() (
	set +e
	[[ -z $tcp_pid ]] || kill "$tcp_pid" >/dev/null 2>&1 || true
	[[ -z $udp_pid ]] || kill "$udp_pid" >/dev/null 2>&1 || true
	[[ -z $tcp_pid ]] || wait "$tcp_pid" >/dev/null 2>&1 || true
	[[ -z $udp_pid ]] || wait "$udp_pid" >/dev/null 2>&1 || true
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
assert value['state'] in ('READY','DEGRADED'), value
assert value['owner'] == 'runtime' and value['managed_namespace'] is True, value
assert re.fullmatch(r'[0-9a-f]{32}',value['generation']), value
assert value['address'].endswith('/30') and value['gateway'], value
assert value['dns']['nameservers'], value
print(json.dumps({
 'container_id':value['container_id'], 'sandbox_id':value['sandbox_id'],
 'sandbox_generation':value['sandbox_generation'], 'generation':value['generation'],
 'address':value['address'], 'gateway':value['gateway'], 'mtu':value['mtu'],
 'dns':value['dns'], 'state':value['state'],
 'rx_packets':value['rx_packets'], 'tx_packets':value['tx_packets'],
 'rx_drops':value['rx_drops'], 'tx_drops':value['tx_drops'], 'errors':value['errors'],
},sort_keys=True,separators=(',',':')))
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
qualifier_sha256=$(sha256sum "$0" | awk '{print $1}')"

guest_program='set -eu; touch /tmp/network-flow-ready; while [ ! -e /tmp/network-flow-release ]; do sleep 1; done'
sudo ctr run --detach --runtime "$runtime" "$image" "$task_id" /bin/sh -c "$guest_program"
wait_task_state RUNNING
for _ in $(seq 1 120); do
	sudo ctr task exec --exec-id "network-ready-$RANDOM" "$task_id" /bin/test -e /tmp/network-flow-ready >/dev/null 2>&1 && break
	sleep .25
done
sudo ctr task exec --exec-id network-ready-final "$task_id" /bin/test -e /tmp/network-flow-ready
wait_counts 'rootfs_records=1 live_exports=1 endpoints=1' >/dev/null

endpoint_before=$(endpoint_summary)
observe endpoint-before-flows "$endpoint_before"
primary_ip=$(ip -4 route get 8.8.8.8 | awk '{for(i=1;i<=NF;i++) if($i=="src") {print $(i+1); exit}}')
[[ $primary_ip =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]
tcp_port=18080
udp_port=18081

python3 - "$primary_ip" "$tcp_port" "$tcp_log" <<'PY' &
import json,socket,sys
host,port,path=sys.argv[1],int(sys.argv[2]),sys.argv[3]
with socket.socket() as server:
    server.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1)
    server.bind((host,port)); server.listen(1); server.settimeout(30)
    conn,peer=server.accept()
    with conn:
        conn.settimeout(10); payload=conn.recv(256)
        assert payload == b'child-tcp-token', payload
        conn.sendall(b'primary-tcp-reply')
with open(path,'w',encoding='utf-8') as stream:
    json.dump({'destination':f'{host}:{port}','peer':peer[0],'peer_port':peer[1],
               'request':payload.decode(),'response':'primary-tcp-reply','protocol':'tcp'},stream,sort_keys=True)
PY
tcp_pid=$!
python3 - "$primary_ip" "$udp_port" "$udp_log" <<'PY' &
import json,socket,sys
host,port,path=sys.argv[1],int(sys.argv[2]),sys.argv[3]
with socket.socket(socket.AF_INET,socket.SOCK_DGRAM) as server:
    server.bind((host,port)); server.settimeout(30)
    payload,peer=server.recvfrom(256)
    assert payload == b'child-udp-token', payload
    server.sendto(b'primary-udp-reply',peer)
with open(path,'w',encoding='utf-8') as stream:
    json.dump({'destination':f'{host}:{port}','peer':peer[0],'peer_port':peer[1],
               'request':payload.decode(),'response':'primary-udp-reply','protocol':'udp'},stream,sort_keys=True)
PY
udp_pid=$!
sleep .25
kill -0 "$tcp_pid"
kill -0 "$udp_pid"

tcp_reply=$(sudo ctr task exec --exec-id network-tcp "$task_id" /bin/sh -c "printf child-tcp-token | nc -w 10 '$primary_ip' '$tcp_port'")
[[ $tcp_reply = primary-tcp-reply ]]
wait "$tcp_pid"
tcp_pid=
observe child-primary-tcp "child_destination=$primary_ip:$tcp_port child_request=child-tcp-token child_response=$tcp_reply
primary_observation=$(cat "$tcp_log")"

udp_reply=$(sudo ctr task exec --exec-id network-udp "$task_id" /bin/sh -c "printf child-udp-token | nc -u -w 10 '$primary_ip' '$udp_port'")
[[ $udp_reply = primary-udp-reply ]]
wait "$udp_pid"
udp_pid=
observe child-primary-outbound-udp "child_destination=$primary_ip:$udp_port child_request=child-udp-token child_response=$udp_reply
primary_observation=$(cat "$udp_log")"

dns_output=$(sudo ctr task exec --exec-id network-dns "$task_id" /bin/nslookup example.com)
grep -Fq 'example.com' <<<"$dns_output"
grep -Eq 'Address[^:]*: [0-9a-fA-F:.]+' <<<"$dns_output"
observe outbound-dns-query-answer "question=example.com
$dns_output"

http_output=$(sudo ctr task exec --exec-id network-http "$task_id" /bin/sh -c 'set -eu; wget -T 20 -S -O /tmp/network-http.body http://example.com 2>/tmp/network-http.headers; printf "destination=http://example.com\nbody_bytes=%s\nbody_sha256=%s\n" "$(wc -c </tmp/network-http.body)" "$(sha256sum /tmp/network-http.body | cut -d" " -f1)"; cat /tmp/network-http.headers')
grep -Fq 'destination=http://example.com' <<<"$http_output"
grep -Eq 'HTTP/[0-9.]+ 200' <<<"$http_output"
grep -Eq 'body_bytes=[1-9][0-9]*' <<<"$http_output"
observe outbound-tcp-http-return "$http_output"

sleep 2
endpoint_after=$(endpoint_summary)
observe endpoint-after-flows "$endpoint_after"
python3 - "$endpoint_before" "$endpoint_after" <<'PY'
import json,sys
before,after=map(json.loads,sys.argv[1:])
for key in ('container_id','sandbox_id','sandbox_generation','generation','address','gateway','mtu','dns'):
    assert before[key] == after[key], (key,before,after)
assert after['rx_packets'] >= before['rx_packets'], (before,after)
assert after['tx_packets'] >= before['tx_packets'], (before,after)
assert after['errors'] == before['errors'], (before,after)
PY

sudo ctr task exec --exec-id network-release "$task_id" /bin/touch /tmp/network-flow-release
wait_task_state STOPPED
sudo ctr tasks rm "$task_id" >/dev/null
sudo ctr containers rm "$task_id"
observe clean-after-release "$(wait_counts 'rootfs_records=0 live_exports=0 endpoints=0')"

sudo systemctl restart mkruntimed
for _ in $(seq 1 60); do systemctl is-active --quiet mkruntimed && break; sleep 1; done
[[ $(systemctl is-active mkruntimed) = active ]]
kerf_ready=
for _ in $(seq 1 60); do
	if kerf_ready=$(sudo /opt/mkruntime/kerf-venv/bin/kerf show 2>&1) &&
		grep -Fq 'No memory pool configured' <<<"$kerf_ready" &&
		grep -Fq 'No instances found' <<<"$kerf_ready"; then
		break
	fi
	sleep 1
done
grep -Fq 'No memory pool configured' <<<"$kerf_ready"
grep -Fq 'No instances found' <<<"$kerf_ready"
observe post-restart-readiness "$kerf_ready"
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
echo G5_NETWORK_FLOWS_LIVE_PASS
