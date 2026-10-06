#!/usr/bin/env bash
set -euo pipefail

if [[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]]; then
	PS4='+${BASH_SOURCE}:${LINENO}: '
	set -x
fi

source_root=${1:?usage: test-runtime-network-isolation-live.sh SOURCE_ROOT}
image=${MK_TEST_IMAGE:-docker.io/library/busybox:1.36}
runtime=${MK_RUNTIME:-io.containerd.multikernel.v2}
task_a=mk-net-isolation-a
task_b=mk-net-isolation-b
shared_hostname=shared-internal-name
scratch=$(mktemp -d -p /var/tmp mk-network-isolation.XXXXXX)
listener_log=$scratch/primary-listener.json
listener_pid=

observe() {
	local key=$1
	shift
	printf 'OBSERVATION_BEGIN key=%s\n%s\nOBSERVATION_END key=%s\n' "$key" "$*" "$key"
}

cleanup_one() {
	local task=$1 state=
	sudo ctr tasks kill --signal SIGKILL "$task" >/dev/null 2>&1 || true
	for _ in $(seq 1 120); do
		state=$(sudo ctr tasks list | awk -v id="$task" '$1 == id {print $3}')
		[[ $state != RUNNING && $state != PAUSED ]] && break
		sleep .25
	done
	sudo ctr tasks rm -f "$task" >/dev/null 2>&1 || true
	sudo ctr containers rm "$task" >/dev/null 2>&1 || true
}

cleanup() (
	set +e
	[[ -z $listener_pid ]] || kill "$listener_pid" >/dev/null 2>&1 || true
	[[ -z $listener_pid ]] || wait "$listener_pid" >/dev/null 2>&1 || true
	cleanup_one "$task_a"
	cleanup_one "$task_b"
	rm -rf -- "$scratch"
)
trap cleanup EXIT

wait_task_state() {
	local task=$1 expected=$2 state=
	for _ in $(seq 1 240); do
		state=$(sudo ctr tasks list | awk -v id="$task" '$1 == id {print $3}')
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

endpoint_inventory() {
	sudo python3 - "$task_a" "$task_b" <<'PY'
import json,re,sys
wanted=set(sys.argv[1:])
with open('/var/lib/mknetd/state.json', encoding='utf-8') as stream:
    state=json.load(stream)
assert state['version'] == 1 and len(state['endpoints']) == 2, state
result={}
for value in state['endpoints'].values():
    assert value['container_id'] in wanted and value['container_id'] not in result, value
    assert value['state'] in ('READY','DEGRADED'), value
    assert value['owner'] == 'runtime' and value['managed_namespace'] is True, value
    assert re.fullmatch(r'[0-9a-f]{32}',value['generation']), value
    result[value['container_id']]={
      'sandbox_id':value['sandbox_id'], 'sandbox_generation':value['sandbox_generation'],
      'generation':value['generation'], 'address':value['address'],
      'gateway':value['gateway'], 'mtu':value['mtu'], 'dns':value['dns'],
      'state':value['state'], 'errors':value['errors']}
assert set(result) == wanted, result
assert len({v['address'] for v in result.values()}) == 2, result
assert len({v['generation'] for v in result.values()}) == 2, result
print(json.dumps(result,sort_keys=True,separators=(',',':')))
PY
}

[[ $(id -u) -ne 0 ]] || { echo 'run as an ordinary sudo-capable user' >&2; exit 1; }
test -x "$source_root/scripts/audit-runtime-final-resources-live.sh"
for service in mkruntimed mknetd containerd docker; do
	[[ $(systemctl is-active "$service") = active ]]
	[[ $(systemctl show -p NRestarts --value "$service") = 0 ]]
done
cleanup_one "$task_a"
cleanup_one "$task_b"
observe clean-before "$(wait_counts 'rootfs_records=0 live_exports=0 endpoints=0')"
observe provenance "boot_id=$(cat /proc/sys/kernel/random/boot_id)
selector=$(readlink -f /usr/local/lib/multikernel/current)
mkruntimed_sha256=$(sudo sha256sum /proc/$(systemctl show -p MainPID --value mkruntimed)/exe | awk '{print $1}')
mknetd_sha256=$(sudo sha256sum /proc/$(systemctl show -p MainPID --value mknetd)/exe | awk '{print $1}')
qualifier_sha256=$(sha256sum "$0" | awk '{print $1}')"

guest_program='set -eu; touch /tmp/isolation-ready; while [ ! -e /tmp/isolation-release ]; do sleep 1; done'
sudo ctr run --detach --runtime "$runtime" --hostname "$shared_hostname" "$image" "$task_a" /bin/sh -c "$guest_program"
sudo ctr run --detach --runtime "$runtime" --hostname "$shared_hostname" "$image" "$task_b" /bin/sh -c "$guest_program"
wait_task_state "$task_a" RUNNING
wait_task_state "$task_b" RUNNING
for task in "$task_a" "$task_b"; do
	for _ in $(seq 1 120); do
		sudo ctr task exec --exec-id "isolation-ready-$RANDOM" "$task" /bin/test -e /tmp/isolation-ready >/dev/null 2>&1 && break
		sleep .25
	done
	sudo ctr task exec --exec-id "isolation-ready-final-$task" "$task" /bin/test -e /tmp/isolation-ready
done
wait_counts 'rootfs_records=2 live_exports=2 endpoints=2' >/dev/null

endpoints=$(endpoint_inventory)
identity_a=$(sudo ctr task exec --exec-id isolation-identity-a "$task_a" /bin/sh -c 'printf "hostname=%s\n" "$(hostname)"; ip -4 address show dev mkn0')
identity_b=$(sudo ctr task exec --exec-id isolation-identity-b "$task_b" /bin/sh -c 'printf "hostname=%s\n" "$(hostname)"; ip -4 address show dev mkn0')
grep -Fxq "hostname=$shared_hostname" <<<"$identity_a"
grep -Fxq "hostname=$shared_hostname" <<<"$identity_b"
ip_a=$(awk '/inet / {sub("/.*", "", $2); print $2; exit}' <<<"$identity_a")
ip_b=$(awk '/inet / {sub("/.*", "", $2); print $2; exit}' <<<"$identity_b")
[[ -n $ip_a && -n $ip_b && $ip_a != "$ip_b" ]]
python3 - "$endpoints" "$task_a" "$ip_a" "$task_b" "$ip_b" <<'PY'
import json,sys
state=json.loads(sys.argv[1])
for task,address in ((sys.argv[2],sys.argv[3]),(sys.argv[4],sys.argv[5])):
    assert state[task]['address'].split('/')[0] == address, (state,task,address)
PY
observe overlapping-name-distinct-identity "endpoint_inventory=$endpoints
task_a=$task_a $identity_a
task_b=$task_b $identity_b"

primary_ip=$(ip -4 route get 8.8.8.8 | awk '{for(i=1;i<=NF;i++) if($i=="src") {print $(i+1); exit}}')
[[ $primary_ip =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]
primary_port=18082
python3 - "$primary_ip" "$primary_port" "$listener_log" <<'PY' &
import json,socket,sys
host,port,path=sys.argv[1],int(sys.argv[2]),sys.argv[3]
records=[]
with socket.socket() as server:
    server.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1)
    server.bind((host,port)); server.listen(2); server.settimeout(30)
    for _ in range(2):
        conn,peer=server.accept()
        with conn:
            conn.settimeout(10); payload=conn.recv(256)
            token=payload.decode()
            assert token in ('route-token-a','route-token-b'), token
            response='primary-route-ok-'+token[-1]
            conn.sendall(response.encode())
            records.append({'destination':f'{host}:{port}','peer':peer[0],
              'peer_port':peer[1],'request':token,'response':response})
assert {r['request'] for r in records} == {'route-token-a','route-token-b'}, records
with open(path,'w',encoding='utf-8') as stream:
    json.dump(records,stream,sort_keys=True)
PY
listener_pid=$!
sleep .25
kill -0 "$listener_pid"
reply_a=$(sudo ctr task exec --exec-id isolation-route-a "$task_a" /bin/sh -c "printf route-token-a | nc -w 10 '$primary_ip' '$primary_port'")
reply_b=$(sudo ctr task exec --exec-id isolation-route-b "$task_b" /bin/sh -c "printf route-token-b | nc -w 10 '$primary_ip' '$primary_port'")
[[ $reply_a = primary-route-ok-a && $reply_b = primary-route-ok-b ]]
wait "$listener_pid"
listener_pid=
observe positive-allowed-routing "task_a_destination=$primary_ip:$primary_port request=route-token-a response=$reply_a
task_b_destination=$primary_ip:$primary_port request=route-token-b response=$reply_b
primary_observation=$(cat "$listener_log")"

set +e
ping_a_to_b=$(sudo ctr task exec --exec-id isolation-ping-a-b "$task_a" /bin/ping -c 1 -W 2 "$ip_b" 2>&1)
ping_a_to_b_rc=$?
ping_b_to_a=$(sudo ctr task exec --exec-id isolation-ping-b-a "$task_b" /bin/ping -c 1 -W 2 "$ip_a" 2>&1)
ping_b_to_a_rc=$?
set -e
[[ $ping_a_to_b_rc -ne 0 && $ping_b_to_a_rc -ne 0 ]]
observe negative-default-isolation "task_a_to_task_b=$ip_b exit_status=$ping_a_to_b_rc output=$ping_a_to_b
task_b_to_task_a=$ip_a exit_status=$ping_b_to_a_rc output=$ping_b_to_a"

for task in "$task_a" "$task_b"; do
	sudo ctr task exec --exec-id "isolation-release-$task" "$task" /bin/touch /tmp/isolation-release
	wait_task_state "$task" STOPPED
	sudo ctr tasks rm "$task" >/dev/null
	sudo ctr containers rm "$task"
done
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
echo G5_NETWORK_ISOLATION_LIVE_PASS
