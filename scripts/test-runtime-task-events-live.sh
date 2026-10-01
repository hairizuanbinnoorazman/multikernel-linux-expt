#!/usr/bin/env bash
set -euo pipefail
[[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]] && { PS4='+${BASH_SOURCE}:${LINENO}: '; set -x; }

image=${MK_TEST_IMAGE:-docker.io/library/busybox:1.36}
runtime=${MK_RUNTIME:-io.containerd.multikernel.v2}
kerf=${MK_KERF:-/opt/mkruntime/kerf-venv/bin/kerf}
task_id=mk-task-events
nonzero_id=event-nonzero
signal_id=event-signal
scratch=$(mktemp -d)
events=$scratch/events
signal_output=$scratch/signal-output
init_output=$scratch/init-output
event_pid=
signal_pid=
attach_pid=

stop_reader() {
	local pid=${1:-}
	[[ -z $pid ]] || { kill "$pid" >/dev/null 2>&1 || true; wait "$pid" >/dev/null 2>&1 || true; }
}
cleanup_task() {
	sudo ctr tasks kill --signal SIGKILL "$task_id" >/dev/null 2>&1 || true
	sudo ctr tasks rm -f "$task_id" >/dev/null 2>&1 || true
	sudo ctr containers rm "$task_id" >/dev/null 2>&1 || true
}
cleanup() {
	set +e
	stop_reader "$signal_pid"
	stop_reader "$attach_pid"
	stop_reader "$event_pid"
	cleanup_task
	rm -rf -- "$scratch"
}
trap cleanup EXIT
observe() {
	local key=$1 value=$2
	printf 'OBSERVATION_BEGIN key=%s\n%s\nOBSERVATION_END key=%s\n' "$key" "$value" "$key"
}
pool_configured() {
	local value
	value=$(sudo "$kerf" show)
	if grep -Fq 'No memory pool configured' <<<"$value"; then printf 0; else printf 1; fi
}
inventory() {
	printf 'pool_configured=%s children=%s ctr_tasks=%s ctr_containers=%s moby_tasks=%s moby_containers=%s docker_containers=%s runtime_artifacts=%s rootfs_records=%s endpoints=%s shim_processes=%s helper_processes=%s' \
		"$(pool_configured)" \
		"$(sudo find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 -type d | wc -l)" \
		"$(sudo ctr tasks list -q | wc -l)" "$(sudo ctr containers list -q | wc -l)" \
		"$(sudo ctr -n moby tasks list -q | wc -l)" "$(sudo ctr -n moby containers list -q | wc -l)" \
		"$(sudo docker ps -aq | wc -l)" \
		"$(sudo find /srv/multikernel-storage/runtime -mindepth 1 -print | wc -l)" \
		"$(sudo python3 -c 'import json; print(len(json.load(open("/var/lib/mkruntimed/rootfs/state.json"))["records"]))')" \
		"$(sudo python3 -c 'import json; print(len(json.load(open("/var/lib/mknetd/state.json"))["endpoints"]))')" \
		"$( (pgrep -f '^/usr/local/lib/multikernel/.*/containerd-shim-multikernel-v2' || true) | wc -l)" \
		"$( (pgrep -f '^/usr/local/libexec/multikernel/(mkvsock-nbd|mk-agent-relay)' || true) | wc -l)"
}
wait_inventory() {
	local expected=$1 observed=
	for _ in $(seq 1 480); do
		observed=$(inventory)
		[[ $observed = "$expected" ]] && { printf %s "$observed"; return; }
		sleep .25
	done
	printf %s "$observed"
	return 1
}
wait_event() {
	local topic=$1 id=$2
	for _ in $(seq 1 240); do
		grep -F "$topic" "$events" | grep -Fq "\"$id\"" && return
		sleep .25
	done
	return 1
}

clean='children=0 ctr_tasks=0 ctr_containers=0 moby_tasks=0 moby_containers=0 docker_containers=0 runtime_artifacts=0 rootfs_records=0 endpoints=0 shim_processes=0 helper_processes=0'
clean_released="pool_configured=0 $clean"
clean_retained="pool_configured=1 $clean"
[[ $(id -u) -ne 0 ]] || { echo 'run as an ordinary sudo-capable user' >&2; exit 1; }
for service in mkruntimed mknetd containerd docker; do [[ $(systemctl is-active "$service") = active ]]; done
cleanup_task
observe initial-inventory "$(wait_inventory "$clean_released")"
observe provenance "selector=$(readlink -f /usr/local/lib/multikernel/current) boot_id=$(cat /proc/sys/kernel/random/boot_id) mkruntimed_pid=$(systemctl show -p MainPID --value mkruntimed) containerd_pid=$(systemctl show -p MainPID --value containerd)"
sudo ctr images pull "$image" >/dev/null
sudo timeout 900 stdbuf -oL ctr events >"$events" 2>&1 & event_pid=$!
sleep 1

sudo ctr run --detach --runtime "$runtime" "$image" "$task_id" /bin/sh -c 'while :; do sleep 1; done'
wait_event /tasks/start "$task_id"

set +e
nonzero_output=$(sudo ctr task exec --exec-id "$nonzero_id" "$task_id" /bin/sh -c 'echo exec-nonzero-out; echo exec-nonzero-err >&2; exit 17' 2>&1)
nonzero_status=$?
set -e
[[ $nonzero_status -eq 17 ]]
grep -Fq exec-nonzero-out <<<"$nonzero_output"
grep -Fq exec-nonzero-err <<<"$nonzero_output"
wait_event /tasks/delete "$nonzero_id"
observe exec-nonzero "client_exit=$nonzero_status output=$nonzero_output"

set +e
sudo ctr task exec --exec-id "$signal_id" "$task_id" /bin/sh -c 'echo exec-signal-ready; trap "" TERM; while :; do sleep 1; done' >"$signal_output" 2>&1 & signal_pid=$!
set -e
wait_event /tasks/exec-started "$signal_id"
for _ in $(seq 1 120); do grep -Fq exec-signal-ready "$signal_output" && break; sleep .25; done
grep -Fq exec-signal-ready "$signal_output"
sudo ctr tasks kill --exec-id "$signal_id" --signal SIGKILL "$task_id"
set +e
wait "$signal_pid"; signal_status=$?; signal_pid=
set -e
[[ $signal_status -eq 137 ]]
wait_event /tasks/delete "$signal_id"
observe exec-signaled "signal=SIGKILL client_exit=$signal_status output=$(<"$signal_output")"

set +e
sudo ctr tasks attach "$task_id" >"$init_output" 2>&1 & attach_pid=$!
set -e
sleep 1
sudo ctr tasks kill --signal SIGKILL "$task_id"
set +e
wait "$attach_pid"; init_status=$?; attach_pid=
set -e
[[ $init_status -eq 137 ]]
wait_event /tasks/delete "$task_id"
observe init-signaled "signal=SIGKILL client_exit=$init_status output=$(<"$init_output")"

sudo ctr containers rm "$task_id"
sleep 2
stop_reader "$event_pid"; event_pid=
event_summary=$(python3 - "$events" "$task_id" "$nonzero_id" "$signal_id" <<'PY'
import json,re,sys
p,task,nonzero,signal=sys.argv[1:]
rows=[]
rx=re.compile(r'^(\S+ \S+ \+0000 UTC) \S+ (/tasks/\S+) (\{.*\})$')
for line in open(p):
    m=rx.match(line.rstrip())
    if not m: continue
    body=json.loads(m.group(3)); ident=body.get("id") or body.get("exec_id") or body.get("container_id")
    if body.get("container_id") != task and ident != task: continue
    wall,fraction_zone=m.group(1).split(".",1)
    fraction=fraction_zone.split(" ",1)[0]
    assert fraction.isdigit() and len(fraction) <= 9, f"invalid fractional timestamp: {m.group(1)!r}"
    stamp=(wall,int(fraction.ljust(9,"0")))
    rows.append((stamp,m.group(2),ident,line.rstrip()))
assert rows and all(a[0] <= b[0] for a,b in zip(rows,rows[1:])), "non-monotonic event timestamps"
topics=[(topic,ident) for _,topic,ident,_ in rows]
expected=[
 ("/tasks/create",task),("/tasks/start",task),
 ("/tasks/exec-added",nonzero),("/tasks/exec-started",nonzero),("/tasks/exit",nonzero),("/tasks/delete",nonzero),
 ("/tasks/exec-added",signal),("/tasks/exec-started",signal),("/tasks/exit",signal),("/tasks/delete",signal),
 ("/tasks/exit",task),("/tasks/delete",task),
]
positions=[]; cursor=0
for wanted in expected:
    while cursor < len(topics) and topics[cursor] != wanted: cursor += 1
    assert cursor < len(topics), f"missing ordered event {wanted!r}: {topics!r}"
    positions.append(cursor); cursor += 1
print(json.dumps({"event_count":len(rows),"ordered":True,"monotonic_timestamps":True,"required_positions":positions},sort_keys=True,separators=(",",":")))
for *_,line in rows: print(line)
PY
)
observe task-events "$event_summary"
observe retained-final "$(wait_inventory "$clean_retained")"
idle_pid=$(systemctl show -p MainPID --value mkruntimed)
sudo systemctl restart mkruntimed
released_pid=$(systemctl show -p MainPID --value mkruntimed)
[[ $released_pid != "$idle_pid" ]]
observe released-final "$(wait_inventory "$clean_released")"
trap - EXIT
rm -rf -- "$scratch"
echo G6_TASK_EVENT_ORDER_PASS
