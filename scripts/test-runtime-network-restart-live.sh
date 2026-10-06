#!/usr/bin/env bash
set -euo pipefail

if [[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]]; then
	PS4='+${BASH_SOURCE}:${LINENO}: '
	set -x
fi

source_root=${1:?usage: test-runtime-network-restart-live.sh SOURCE_ROOT}
task_id=mk-network-restart
image=${MK_TEST_IMAGE:-docker.io/library/busybox:1.36}
runtime=${MK_RUNTIME:-io.containerd.multikernel.v2}
scratch=$(mktemp -d -p /var/tmp mk-network-restart.XXXXXX)
listener_pid=
exchange_index=0

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
	[[ -z $listener_pid ]] || kill "$listener_pid" >/dev/null 2>&1 || true
	[[ -z $listener_pid ]] || wait "$listener_pid" >/dev/null 2>&1 || true
	cleanup_task
	rm -rf -- "$scratch"
)
trap cleanup EXIT

wait_task_state() {
	local expected=$1 state=
	for _ in $(seq 1 480); do
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
    if not os.path.exists(path): return 0
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
	for _ in $(seq 1 480); do
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
with open('/var/lib/mknetd/state.json', encoding='utf-8') as stream: state=json.load(stream)
assert state['version'] == 1 and len(state['endpoints']) == 1, state
value=next(iter(state['endpoints'].values()))
assert value['state'] in ('READY','DEGRADED'), value
assert value['owner'] == 'runtime' and value['managed_namespace'] is True, value
assert re.fullmatch(r'[0-9a-f]{32}',value['generation']), value
print(json.dumps({key:value[key] for key in (
 'container_id','sandbox_id','sandbox_generation','generation','address','gateway','mtu','state',
 'rx_packets','tx_packets','rx_drops','tx_drops','errors')},sort_keys=True,separators=(',',':')))
PY
}

parent_pid() { sudo awk '{print $4}' "/proc/$1/stat"; }
holder_pid() { sudo ctr tasks list | awk -v id="$task_id" '$1 == id {print $2}'; }

process_summary() {
	local holder worker supervisor
	holder=$(holder_pid); worker=$(parent_pid "$holder"); supervisor=$(parent_pid "$worker")
	[[ -d /proc/$holder && -d /proc/$worker && -d /proc/$supervisor ]]
	printf 'supervisor=%s worker=%s holder=%s' "$supervisor" "$worker" "$holder"
}

start_task() {
	sudo ctr run --detach --runtime "$runtime" "$image" "$task_id" /bin/sleep 900
	wait_task_state RUNNING
	wait_counts 'rootfs_records=1 live_exports=1 endpoints=1' >/dev/null
}

packet_exchange() {
	local label=$1 primary_ip token reply log port endpoint
	exchange_index=$((exchange_index + 1))
	primary_ip=$(ip -4 route get 8.8.8.8 | awk '{for(i=1;i<=NF;i++) if($i=="src") {print $(i+1); exit}}')
	[[ $primary_ip =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]
	port=$((18100 + exchange_index)); token="restart-token-$label"; log="$scratch/$label.json"
	python3 - "$primary_ip" "$port" "$token" "$log" <<'PY' &
import json,socket,sys
host,port,token,path=sys.argv[1],int(sys.argv[2]),sys.argv[3].encode(),sys.argv[4]
with socket.socket() as server:
    server.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1)
    server.bind((host,port)); server.listen(1); server.settimeout(30)
    conn,peer=server.accept()
    with conn:
        conn.settimeout(10); payload=conn.recv(512); assert payload == token,(payload,token)
        response=b'primary-reply-'+token; conn.sendall(response)
with open(path,'w',encoding='utf-8') as stream:
    json.dump({'destination':f'{host}:{port}','peer':peer[0],'request':token.decode(),
               'response':response.decode()},stream,sort_keys=True)
PY
	listener_pid=$!
	sleep .25
	reply=$(sudo ctr task exec --exec-id "network-$label" "$task_id" /bin/sh -c "printf '%s' '$token' | nc -w 10 '$primary_ip' '$port'")
	[[ $reply = "primary-reply-$token" ]]
	wait "$listener_pid"; listener_pid=
	endpoint=$(endpoint_summary)
	observe "packet-$label" "child_reply=$reply
primary_observation=$(cat "$log")
endpoint=$endpoint"
}

same_endpoint_identity() {
	python3 - "$1" "$2" <<'PY'
import json,sys
a,b=map(json.loads,sys.argv[1:])
for key in ('container_id','sandbox_id','sandbox_generation','generation','address','gateway','mtu'):
    assert a[key] == b[key], (key,a,b)
assert b['errors'] == a['errors'] and b['rx_drops'] == a['rx_drops'] and b['tx_drops'] == a['tx_drops'],(a,b)
PY
}

[[ $(id -u) -ne 0 ]] || { echo 'run as an ordinary sudo-capable user' >&2; exit 1; }
test -x "$source_root/scripts/audit-runtime-final-resources-live.sh"
for service in mkruntimed mknetd containerd docker; do [[ $(systemctl is-active "$service") = active ]]; done
cleanup_task
observe clean-before "$(wait_counts 'rootfs_records=0 live_exports=0 endpoints=0')"
observe provenance "boot_id=$(cat /proc/sys/kernel/random/boot_id)
selector=$(readlink -f /usr/local/lib/multikernel/current)
mkruntimed_sha256=$(sudo sha256sum /proc/$(systemctl show -p MainPID --value mkruntimed)/exe | awk '{print $1}')
mknetd_sha256=$(sudo sha256sum /proc/$(systemctl show -p MainPID --value mknetd)/exe | awk '{print $1}')
qualifier_sha256=$(sha256sum "$0" | awk '{print $1}')"
sudo ctr images pull "$image" >/dev/null

start_task
boot_initial=$(sudo ctr task exec --exec-id boot-initial "$task_id" /bin/cat /proc/sys/kernel/random/boot_id)
endpoint_initial=$(endpoint_summary); processes_initial=$(process_summary)
packet_exchange baseline
observe initial-identities "child_boot=$boot_initial processes=$processes_initial endpoint=$endpoint_initial"

mknetd_before=$(systemctl show -p MainPID --value mknetd)
sudo systemctl restart mknetd
[[ $(systemctl is-active mknetd) = active ]]
mknetd_after=$(systemctl show -p MainPID --value mknetd); [[ $mknetd_after != "$mknetd_before" ]]
packet_exchange after-mknetd
endpoint_after_mknetd=$(endpoint_summary); same_endpoint_identity "$endpoint_initial" "$endpoint_after_mknetd"
[[ $(sudo ctr task exec --exec-id boot-after-mknetd "$task_id" /bin/cat /proc/sys/kernel/random/boot_id) = "$boot_initial" ]]
observe mknetd-restart "pid_before=$mknetd_before pid_after=$mknetd_after child_boot=$boot_initial endpoint=$endpoint_after_mknetd"

mkruntimed_before=$(systemctl show -p MainPID --value mkruntimed)
sudo systemctl restart mkruntimed
[[ $(systemctl is-active mkruntimed) = active ]]
mkruntimed_after=$(systemctl show -p MainPID --value mkruntimed); [[ $mkruntimed_after != "$mkruntimed_before" ]]
packet_exchange after-mkruntimed
endpoint_after_mkruntimed=$(endpoint_summary); same_endpoint_identity "$endpoint_initial" "$endpoint_after_mkruntimed"
[[ $(sudo ctr task exec --exec-id boot-after-mkruntimed "$task_id" /bin/cat /proc/sys/kernel/random/boot_id) = "$boot_initial" ]]
observe mkruntimed-restart "pid_before=$mkruntimed_before pid_after=$mkruntimed_after child_boot=$boot_initial endpoint=$endpoint_after_mkruntimed"

holder_before=$(holder_pid); worker_before=$(parent_pid "$holder_before"); supervisor=$(parent_pid "$worker_before")
sudo kill -KILL "$worker_before"
holder_after=; worker_after=
for _ in $(seq 1 480); do
	holder_after=$(holder_pid); [[ -z $holder_after ]] || worker_after=$(parent_pid "$holder_after" 2>/dev/null || true)
	if [[ -n $holder_after && -n $worker_after && $holder_after != "$holder_before" && $worker_after != "$worker_before" ]] &&
	   [[ $(parent_pid "$worker_after" 2>/dev/null || true) = "$supervisor" ]]; then break; fi
	sleep .25
done
[[ -n $holder_after && -n $worker_after && $holder_after != "$holder_before" && $worker_after != "$worker_before" ]]
packet_exchange after-shim-worker
endpoint_after_shim=$(endpoint_summary); same_endpoint_identity "$endpoint_initial" "$endpoint_after_shim"
boot_after_shim=$(sudo ctr task exec --exec-id boot-after-shim "$task_id" /bin/cat /proc/sys/kernel/random/boot_id); [[ $boot_after_shim = "$boot_initial" ]]
observe shim-worker-restart "supervisor=$supervisor old_worker=$worker_before replacement_worker=$worker_after old_holder=$holder_before replacement_holder=$holder_after child_boot=$boot_after_shim endpoint=$endpoint_after_shim"

sudo ctr tasks kill --signal SIGKILL "$task_id" >/dev/null
wait_task_state STOPPED
sudo ctr tasks rm "$task_id" >/dev/null
sudo ctr containers rm "$task_id"
observe between-child-generations "$(wait_counts 'rootfs_records=0 live_exports=0 endpoints=0')"
start_task
boot_replacement=$(sudo ctr task exec --exec-id boot-replacement "$task_id" /bin/cat /proc/sys/kernel/random/boot_id)
endpoint_replacement=$(endpoint_summary)
[[ $boot_replacement != "$boot_initial" ]]
python3 - "$endpoint_initial" "$endpoint_replacement" <<'PY'
import json,sys
a,b=map(json.loads,sys.argv[1:])
assert a['sandbox_generation'] != b['sandbox_generation'],(a,b)
assert a['generation'] != b['generation'],(a,b)
PY
packet_exchange after-child-replacement
observe child-replacement "old_child_boot=$boot_initial replacement_child_boot=$boot_replacement old_endpoint=$endpoint_initial replacement_endpoint=$endpoint_replacement"

sudo ctr tasks kill --signal SIGKILL "$task_id" >/dev/null
wait_task_state STOPPED
sudo ctr tasks rm "$task_id" >/dev/null
sudo ctr containers rm "$task_id"
observe clean-after "$(wait_counts 'rootfs_records=0 live_exports=0 endpoints=0')"

idle_mkruntimed_before=$(systemctl show -p MainPID --value mkruntimed)
sudo systemctl restart mkruntimed
[[ $(systemctl is-active mkruntimed) = active ]]
idle_mkruntimed_after=$(systemctl show -p MainPID --value mkruntimed)
[[ $idle_mkruntimed_after != "$idle_mkruntimed_before" ]]
observe idle-pool-release "mkruntimed_pid_before=$idle_mkruntimed_before mkruntimed_pid_after=$idle_mkruntimed_after"

audit_output=
for _ in $(seq 1 60); do
	if audit_output=$(MK_EVIDENCE_XTRACE=0 "$source_root/scripts/audit-runtime-final-resources-live.sh" 2>&1); then break; fi
	sleep 1
done
grep -Fq G6_FINAL_RESOURCE_RETURN_PASS <<<"$audit_output"
printf '%s\n' "$audit_output"
for service in mkruntimed mknetd containerd docker; do [[ $(systemctl is-active "$service") = active ]]; done

trap - EXIT
rm -rf -- "$scratch"
echo G5_NETWORK_RESTART_LIVE_PASS
