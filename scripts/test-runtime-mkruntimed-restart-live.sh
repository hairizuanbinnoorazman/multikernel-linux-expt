#!/usr/bin/env bash
set -euo pipefail
[[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]] && { PS4='+${BASH_SOURCE}:${LINENO}: '; set -x; }
image=${MK_TEST_IMAGE:-docker.io/library/busybox:1.36}
runtime=${MK_RUNTIME:-io.containerd.multikernel.v2}
kerf=${MK_KERF:-/opt/mkruntime/kerf-venv/bin/kerf}
host_config=${MK_HOST_CONFIG:-/etc/mkruntime/config.json}
task_id=mk-mkruntimed-restart
scratch=$(mktemp -d); stream=$scratch/stream; events=$scratch/events
attach_pid=; event_pid=

stop_reader() { local pid=${1:-}; [[ -z $pid ]] || { kill "$pid" >/dev/null 2>&1 || true; wait "$pid" >/dev/null 2>&1 || true; }; }
cleanup_task() {
	sudo ctr tasks kill --signal SIGKILL "$task_id" >/dev/null 2>&1 || true
	for _ in $(seq 1 120); do
		state=$(sudo ctr tasks list | awk -v id="$task_id" '$1 == id {print $3}')
		[[ $state != RUNNING && $state != PAUSED ]] && break; sleep .25
	done
	sudo ctr tasks rm -f "$task_id" >/dev/null 2>&1 || true
	sudo ctr containers rm "$task_id" >/dev/null 2>&1 || true
}
cleanup() { set +e; stop_reader "$attach_pid"; stop_reader "$event_pid"; cleanup_task; rm -rf -- "$scratch"; }
trap cleanup EXIT
observe() { local key=$1 value=$2; printf 'OBSERVATION_BEGIN key=%s\n%s\nOBSERVATION_END key=%s\n' "$key" "$value" "$key"; }
pool_configured() { local value; value=$(sudo "$kerf" show); if grep -Fq 'No memory pool configured' <<<"$value"; then printf 0; else printf 1; fi; }
inventory() {
	printf 'pool_configured=%s children=%s links=%s nat_rules=%s filter_rules=%s ctr_tasks=%s ctr_containers=%s moby_tasks=%s moby_containers=%s docker_containers=%s runtime_artifacts=%s rootfs_records=%s endpoints=%s shim_processes=%s helper_processes=%s' \
		"$(pool_configured)" "$(sudo find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 -type d | wc -l)" \
		"$(ip -o link show | awk -F': ' '$2 ~ /^mkv[0-9a-f]+$/ {n++} END {print n+0}')" \
		"$(sudo iptables -t nat -S POSTROUTING | grep -c '172\.31\.' || true)" "$(sudo iptables -S | grep -c '^\(-N\|-A\) MK-' || true)" \
		"$(sudo ctr tasks list -q | wc -l)" "$(sudo ctr containers list -q | wc -l)" \
		"$(sudo ctr -n moby tasks list -q | wc -l)" "$(sudo ctr -n moby containers list -q | wc -l)" "$(sudo docker ps -aq | wc -l)" \
		"$(sudo find /srv/multikernel-storage/runtime -mindepth 1 -print | wc -l)" \
		"$(sudo python3 -c 'import json; print(len(json.load(open("/var/lib/mkruntimed/rootfs/state.json"))["records"]))')" \
		"$(sudo python3 -c 'import json; print(len(json.load(open("/var/lib/mknetd/state.json"))["endpoints"]))')" \
		"$( (pgrep -f '^/usr/local/lib/multikernel/.*/containerd-shim-multikernel-v2' || true) | wc -l)" \
		"$( (pgrep -f '^/usr/local/libexec/multikernel/(mkvsock-nbd|mk-agent-relay)' || true) | wc -l)"
}
wait_inventory() { local expected=$1 observed=; for _ in $(seq 1 480); do observed=$(inventory); [[ $observed = "$expected" ]] && { printf %s "$observed"; return; }; sleep .25; done; printf %s "$observed"; return 1; }
parent_pid() { sudo awk '{print $4}' "/proc/$1/stat"; }
task_holder_pid() { sudo ctr tasks list | awk -v id="$1" '$1 == id {print $2}'; }
recovery_summary() {
	sudo python3 - "/proc/$1/cwd/.multikernel/sandbox.json" <<'PY'
import json,os,sys
p=sys.argv[1]; d=json.load(open(p)); s=os.stat(p)
o={"file":{"uid":s.st_uid,"mode":oct(s.st_mode&0o777),"links":s.st_nlink},"id":d["id"],"generation":d["generation"],"task_identity":d["task_identity"],"network":{k:d["network"].get(k) for k in ("sandbox_id","sandbox_generation","generation","address","mtu")},"processes":[{k:x.get(k) for k in ("id","pid","status","stdin_offset","stdout_offset","stderr_offset")} for x in d["processes"]]}
print(json.dumps(o,sort_keys=True,separators=(",",":")))
PY
}
daemon_state_summary() {
	sudo python3 - "$1" <<'PY'
import hashlib,json,os,sys
b=sys.argv[1]
def dig(p):
 d=open(p,"rb").read(); s=os.stat(p); return {"sha256":hashlib.sha256(d).hexdigest(),"size":len(d),"mode":oct(s.st_mode&0o777),"uid":s.st_uid}
sp=os.path.join(b,"state.json"); jp=os.path.join(b,"journal.jsonl"); s=json.load(open(sp)); es=[json.loads(x) for x in open(jp) if x.strip()]
boxes=[{"key":k,"id":v.get("id"),"generation":v.get("generation"),"state":v.get("state")} for k,v in sorted(s.get("sandboxes",{}).items())]
safe=[{k:x.get(k) for k in ("sequence","sandbox_id","generation","method","phase","state")} for x in es]
o={"snapshot_file":dig(sp),"journal_file":dig(jp),"snapshot":{"version":s.get("version"),"sequence":s.get("sequence"),"sandboxes":boxes,"result_count":len(s.get("results",{}))},"journal":{"count":len(safe),"last":safe[-8:]}}
for n,p in (("rootfs","/var/lib/mkruntimed/rootfs/state.json"),("storage","/var/lib/mkruntimed/storage/state.json")):
 v=json.load(open(p)); r=v.get("records",{}); o[n]={"file":dig(p),"version":v.get("version"),"record_count":len(r),"record_keys":sorted(r)}
print(json.dumps(o,sort_keys=True,separators=(",",":")))
PY
}
service_journal_summary() {
	sudo journalctl -u mkruntimed --since "$1" --no-pager -o json | python3 -c '
import hashlib,json,sys
r=[json.loads(x) for x in sys.stdin if x.strip()]; e="\n".join(json.dumps(x,sort_keys=True,separators=(",",":")) for x in r).encode(); p=[int(x.get("PRIORITY",6)) for x in r]
print(json.dumps({"entries":len(r),"sha256":hashlib.sha256(e).hexdigest(),"first_realtime":r[0].get("__REALTIME_TIMESTAMP") if r else None,"last_realtime":r[-1].get("__REALTIME_TIMESTAMP") if r else None,"priorities":{str(x):p.count(x) for x in sorted(set(p))},"error_or_higher":sum(x<=3 for x in p)},sort_keys=True,separators=(",",":")))'
}

clean='children=0 links=0 nat_rules=0 filter_rules=0 ctr_tasks=0 ctr_containers=0 moby_tasks=0 moby_containers=0 docker_containers=0 runtime_artifacts=0 rootfs_records=0 endpoints=0 shim_processes=0 helper_processes=0'
clean_released="pool_configured=0 $clean"; clean_retained="pool_configured=1 $clean"
[[ $(id -u) -ne 0 ]] || { echo 'run as an ordinary sudo-capable user' >&2; exit 1; }
for service in mkruntimed mknetd containerd docker; do [[ $(systemctl is-active "$service") = active ]]; done
cleanup_task; observe initial-inventory "$(wait_inventory "$clean_released")"
state_dir=$(sudo python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["state_directory"])' "$host_config")
host_boot=$(cat /proc/sys/kernel/random/boot_id); daemon_before=$(systemctl show -p MainPID --value mkruntimed); containerd_pid=$(systemctl show -p MainPID --value containerd)
observe host-before "boot_id=$host_boot mkruntimed_pid=$daemon_before containerd_pid=$containerd_pid selector=$(readlink -f /usr/local/lib/multikernel/current)"
sudo ctr images pull "$image" >/dev/null
sudo timeout 900 stdbuf -oL ctr events >"$events" 2>&1 & event_pid=$!; sleep 1
sudo ctr run --detach --runtime "$runtime" "$image" "$task_id" /bin/sh -c 'while [ ! -e /tmp/restart-ready ]; do sleep 1; done; echo daemon-before; echo daemon-before-err >&2; while [ ! -e /tmp/restart-release ]; do sleep 1; done; echo daemon-after; echo daemon-after-err >&2'
for _ in $(seq 1 480); do state=$(sudo ctr tasks list | awk -v id="$task_id" '$1 == id {print $3}'); [[ $state = RUNNING ]] && break; sleep .25; done; [[ $state = RUNNING ]]
sudo timeout 600 ctr tasks attach "$task_id" >"$stream" 2>&1 & attach_pid=$!; sleep 1
sudo ctr task exec --exec-id daemon-ready "$task_id" /bin/touch /tmp/restart-ready
for _ in $(seq 1 120); do grep -Fxq daemon-before "$stream" && grep -Fxq daemon-before-err "$stream" && break; sleep .25; done
grep -Fxq daemon-before "$stream"; grep -Fxq daemon-before-err "$stream"
holder_before=$(task_holder_pid "$task_id"); worker=$(parent_pid "$holder_before"); supervisor=$(parent_pid "$worker")
child_boot_before=$(sudo ctr task exec --exec-id daemon-boot-before "$task_id" /bin/cat /proc/sys/kernel/random/boot_id)
exec_before=$(sudo ctr task exec --exec-id daemon-exec-before "$task_id" /bin/sh -c 'echo exec-before; echo exec-before-err >&2' 2>&1)
grep -Fxq exec-before <<<"$exec_before"; grep -Fxq exec-before-err <<<"$exec_before"
recovery_before=$(recovery_summary "$supervisor"); durable_before=$(daemon_state_summary "$state_dir")
observe task-before "supervisor_pid=$supervisor worker_pid=$worker namespace_holder_pid=$holder_before state=$state child_boot=$child_boot_before exec=$exec_before recovery=$recovery_before"; observe daemon-durable-before "$durable_before"
restart_since=$(date -u '+%Y-%m-%d %H:%M:%S UTC'); sudo systemctl restart mkruntimed; [[ $(systemctl is-active mkruntimed) = active ]]
daemon_after=$(systemctl show -p MainPID --value mkruntimed); [[ $daemon_after != "$daemon_before" ]]; [[ $(systemctl show -p MainPID --value containerd) = "$containerd_pid" ]]; [[ $(cat /proc/sys/kernel/random/boot_id) = "$host_boot" ]]
state=$(sudo ctr tasks list | awk -v id="$task_id" '$1 == id {print $3}'); [[ $state = RUNNING ]]
holder_after=$(task_holder_pid "$task_id"); [[ $holder_after = "$holder_before" ]]
child_boot_after=$(sudo ctr task exec --exec-id daemon-boot-after "$task_id" /bin/cat /proc/sys/kernel/random/boot_id); [[ $child_boot_after = "$child_boot_before" ]]
exec_after=$(sudo ctr task exec --exec-id daemon-exec-after "$task_id" /bin/sh -c 'echo exec-after; echo exec-after-err >&2' 2>&1); grep -Fxq exec-after <<<"$exec_after"; grep -Fxq exec-after-err <<<"$exec_after"
recovery_after=$(recovery_summary "$supervisor"); durable_after=$(daemon_state_summary "$state_dir"); [[ $recovery_after = "$recovery_before" ]]; [[ $durable_after = "$durable_before" ]]
journal_after=$(service_journal_summary "$restart_since"); grep -Fq '"error_or_higher":0' <<<"$journal_after"
observe host-after "boot_id=$host_boot mkruntimed_pid_before=$daemon_before mkruntimed_pid_after=$daemon_after containerd_pid=$containerd_pid"
observe task-after "supervisor_pid=$supervisor namespace_holder_pid=$holder_after state=$state child_boot=$child_boot_after exec=$exec_after recovery=$recovery_after"
observe daemon-durable-after "$durable_after"; observe mkruntimed-service-journal "$journal_after"
sudo ctr task exec --exec-id daemon-release "$task_id" /bin/touch /tmp/restart-release
wait "$attach_pid"; attach_pid=; stream_value=$(<"$stream")
for marker in daemon-before daemon-before-err daemon-after daemon-after-err; do grep -Fxq "$marker" <<<"$stream_value"; done; observe continuous-stream "$stream_value"
for _ in $(seq 1 120); do state=$(sudo ctr tasks list | awk -v id="$task_id" '$1 == id {print $3}'); [[ $state = STOPPED ]] && break; sleep .25; done; [[ $state = STOPPED ]]
sudo ctr tasks rm "$task_id" >/dev/null; sudo ctr containers rm "$task_id"; sleep 2; stop_reader "$event_pid"; event_pid=
for topic in /tasks/create /tasks/start /tasks/exit /tasks/delete; do grep -Fq "$topic" "$events"; done; observe task-events "$(grep -F "$task_id" "$events")"
observe retained-final "$(wait_inventory "$clean_retained")"
idle_pid=$(systemctl show -p MainPID --value mkruntimed); sudo systemctl restart mkruntimed; released_pid=$(systemctl show -p MainPID --value mkruntimed); [[ $released_pid != "$idle_pid" ]]
observe released-final "$(wait_inventory "$clean_released")"
trap - EXIT; rm -rf -- "$scratch"; echo G6_MKRUNTIMED_RESTART_CONTINUITY_PASS
