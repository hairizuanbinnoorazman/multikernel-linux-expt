#!/usr/bin/env bash
set -euo pipefail
[[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]] && { PS4='+${BASH_SOURCE}:${LINENO}: '; set -x; }

image=${MK_TEST_IMAGE:-docker.io/library/busybox:1.36}
runtime=${MK_RUNTIME:-io.containerd.multikernel.v2}
kerf=${MK_KERF:-/opt/mkruntime/kerf-venv/bin/kerf}
host_config=${MK_HOST_CONFIG:-/etc/mkruntime/config.json}
ctr_id=mk-concurrent-ctr
docker_name=mk-concurrent-docker
scratch=$(mktemp -d)
ctr_churn=$scratch/ctr-churn
docker_churn=$scratch/docker-churn
docker_isolation=(--network none --security-opt apparmor=unconfined --security-opt seccomp=unconfined --sysctl net.ipv4.ip_unprivileged_port_start=1024 --sysctl 'net.ipv4.ping_group_range=1 0' --device-cgroup-rule 'a *:* rwm')

cleanup_workloads() {
	set +e
	sudo ctr tasks kill --signal SIGKILL "$ctr_id" >/dev/null 2>&1 || true
	sudo ctr tasks rm -f "$ctr_id" >/dev/null 2>&1 || true
	sudo ctr containers rm "$ctr_id" >/dev/null 2>&1 || true
	sudo docker rm -f "$docker_name" >/dev/null 2>&1 || true
}
cleanup() {
	cleanup_workloads
	rm -rf -- "$scratch"
}
trap cleanup EXIT
observe() { local key=$1 value=$2; printf 'OBSERVATION_BEGIN key=%s\n%s\nOBSERVATION_END key=%s\n' "$key" "$value" "$key"; }
pool_configured() { local value; value=$(sudo "$kerf" show); if grep -Fq 'No memory pool configured' <<<"$value"; then printf 0; else printf 1; fi; }
inventory() {
	printf 'pool_configured=%s children=%s links=%s nat_rules=%s filter_rules=%s ctr_tasks=%s ctr_containers=%s moby_tasks=%s moby_containers=%s docker_containers=%s runtime_artifacts=%s rootfs_records=%s endpoints=%s shim_processes=%s helper_processes=%s' \
		"$(pool_configured)" "$(sudo find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 -type d | wc -l)" \
		"$(ip -o link show | awk -F': ' '$2 ~ /^mkv[0-9a-f]+$/ {n++} END {print n+0}')" \
		"$(sudo iptables -t nat -S POSTROUTING | grep -c '172\.31\.' || true)" "$(sudo iptables -S | grep -c '^\(-N\|-A\) MK-' || true)" \
		"$(sudo ctr tasks list -q | wc -l)" "$(sudo ctr containers list -q | wc -l)" "$(sudo ctr -n moby tasks list -q | wc -l)" "$(sudo ctr -n moby containers list -q | wc -l)" "$(sudo docker ps -aq | wc -l)" \
		"$( (sudo find /srv/multikernel-storage/runtime -mindepth 1 -print 2>/dev/null || true) | wc -l)" \
		"$(sudo python3 -c 'import json; print(len(json.load(open("/var/lib/mkruntimed/rootfs/state.json"))["records"]))')" "$(sudo python3 -c 'import json; print(len(json.load(open("/var/lib/mknetd/state.json"))["endpoints"]))')" \
		"$( (pgrep -f '^/usr/local/lib/multikernel/.*/containerd-shim-multikernel-v2' || true) | wc -l)" "$( (pgrep -f '^/usr/local/libexec/multikernel/(mkvsock-nbd|mk-agent-relay)' || true) | wc -l)"
}
wait_inventory() { local expected=$1 observed=; for _ in $(seq 1 600); do observed=$(inventory); [[ $observed = "$expected" ]] && { printf %s "$observed"; return; }; sleep .25; done; printf %s "$observed"; return 1; }
parent_pid() { sudo awk '{print $4}' "/proc/$1/stat"; }

clean='children=0 links=0 nat_rules=0 filter_rules=0 ctr_tasks=0 ctr_containers=0 moby_tasks=0 moby_containers=0 docker_containers=0 runtime_artifacts=0 rootfs_records=0 endpoints=0 shim_processes=0 helper_processes=0'
clean_released="pool_configured=0 $clean"; clean_retained="pool_configured=1 $clean"
[[ $(id -u) -ne 0 ]] || { echo 'run as an ordinary sudo-capable user' >&2; exit 1; }
for service in mkruntimed mknetd containerd docker; do [[ $(systemctl is-active "$service") = active ]]; done
cleanup_workloads
set -e
observe initial-inventory "$(wait_inventory "$clean_released")"
observe provenance "selector=$(readlink -f /usr/local/lib/multikernel/current) boot_id=$(cat /proc/sys/kernel/random/boot_id) mkruntimed_pid=$(systemctl show -p MainPID --value mkruntimed) containerd_pid=$(systemctl show -p MainPID --value containerd) docker_pid=$(systemctl show -p MainPID --value docker)"
sudo ctr images pull "$image" >/dev/null

sudo ctr run --detach --runtime "$runtime" "$image" "$ctr_id" /bin/sh -c 'trap "exit 0" TERM; while :; do sleep 1; done' & ctr_create_pid=$!
sudo docker run --detach --runtime "$runtime" "${docker_isolation[@]}" --name "$docker_name" "$image" /bin/sh -c 'trap "exit 0" TERM; while :; do sleep 1; done' >"$scratch/docker-id" & docker_create_pid=$!
wait "$ctr_create_pid"; wait "$docker_create_pid"
docker_id=$(<"$scratch/docker-id")
for _ in $(seq 1 600); do
	ctr_state=$(sudo ctr tasks list | awk -v id="$ctr_id" '$1 == id {print $3}')
	docker_state=$(sudo docker inspect --format '{{.State.Status}}' "$docker_name")
	[[ $ctr_state = RUNNING && $docker_state = running ]] && break
	sleep .25
done
[[ $ctr_state = RUNNING && $docker_state = running ]]
ctr_holder=$(sudo ctr tasks list | awk -v id="$ctr_id" '$1 == id {print $2}')
docker_holder=$(sudo ctr -n moby tasks list | awk -v id="$docker_id" '$1 == id {print $2}')
ctr_worker=$(parent_pid "$ctr_holder"); ctr_supervisor=$(parent_pid "$ctr_worker")
docker_worker=$(parent_pid "$docker_holder"); docker_supervisor=$(parent_pid "$docker_worker")

state_dir=$(sudo python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["state_directory"])' "$host_config")
identity_summary=$(sudo python3 - "$state_dir/state.json" "/proc/$ctr_supervisor/cwd/.multikernel/sandbox.json" "/proc/$docker_supervisor/cwd/.multikernel/sandbox.json" <<'PY'
import json,os,sys
state=json.load(open(sys.argv[1])); recoveries=[json.load(open(x)) for x in sys.argv[2:]]
boxes=list(state["sandboxes"].values()); assert len(boxes)==2
boxes.sort(key=lambda x:x["id"]); configs=[x["config"] for x in boxes]
assert set(configs[0]["cpus"]).isdisjoint(configs[1]["cpus"])
for field in ("id","bundle","agent_port","child_cid"):
 assert configs[0][field] != configs[1][field], field
assert configs[0]["memory_bytes"] > 0 and configs[1]["memory_bytes"] > 0
assert boxes[0]["generation"] != boxes[1]["generation"]
assert configs[0]["storage"]["path"] != configs[1]["storage"]["path"]
assert configs[0]["storage"]["port"] != configs[1]["storage"]["port"]
byid={x["id"]:x for x in boxes}; rec=[]
for r in recoveries:
 b=byid[r["id"]]; assert r["generation"]==b["generation"]
 c=b["config"]; sock=f'/run/mk-agent-{c["agent_port"]}-{b["generation"][:12]}.sock'; st=os.stat(sock)
 rec.append({"id":b["id"],"generation":b["generation"],"state":b["state"],"cpus":c["cpus"],"memory_bytes":c["memory_bytes"],"bundle":c["bundle"],"agent_port":c["agent_port"],"child_cid":c["child_cid"],"storage_path":c["storage"]["path"],"storage_port":c["storage"]["port"],"task_identity":r["task_identity"],"network_generation":r["network"]["generation"],"network_address":r["network"]["address"],"agent_socket":{"path":sock,"inode":st.st_ino,"mode":oct(st.st_mode&0o777)}})
assert rec[0]["task_identity"] != rec[1]["task_identity"]
assert rec[0]["network_generation"] != rec[1]["network_generation"]
assert rec[0]["network_address"] != rec[1]["network_address"]
print(json.dumps(rec,sort_keys=True,separators=(",",":")))
PY
)
observe concurrent-identities "ctr_supervisor=$ctr_supervisor ctr_worker=$ctr_worker ctr_holder=$ctr_holder docker_supervisor=$docker_supervisor docker_worker=$docker_worker docker_holder=$docker_holder identities=$identity_summary"

(for i in $(seq 1 12); do sudo ctr task exec --exec-id "ctr-churn-$i" "$ctr_id" /bin/echo "ctr-$i"; done) >"$ctr_churn" & ctr_churn_pid=$!
(for i in $(seq 1 12); do sudo docker exec "$docker_name" /bin/echo "docker-$i"; done) >"$docker_churn" & docker_churn_pid=$!
wait "$ctr_churn_pid"; wait "$docker_churn_pid"
[[ $(wc -l <"$ctr_churn") -eq 12 && $(wc -l <"$docker_churn") -eq 12 ]]
sudo ctr tasks pause "$ctr_id" & ctr_pause_pid=$!; sudo docker pause "$docker_name" >/dev/null & docker_pause_pid=$!; wait "$ctr_pause_pid"; wait "$docker_pause_pid"
sudo ctr tasks resume "$ctr_id" & ctr_resume_pid=$!; sudo docker unpause "$docker_name" >/dev/null & docker_resume_pid=$!; wait "$ctr_resume_pid"; wait "$docker_resume_pid"
ctr_boot=$(sudo ctr task exec --exec-id churn-boot "$ctr_id" /bin/cat /proc/sys/kernel/random/boot_id)
docker_boot=$(sudo docker exec "$docker_name" /bin/cat /proc/sys/kernel/random/boot_id)
[[ $ctr_boot != "$docker_boot" ]]
observe concurrent-churn "ctr_execs=12 docker_execs=12 pause_resume=both ctr_boot=$ctr_boot docker_boot=$docker_boot ctr_output=$(tr '\n' ',' <"$ctr_churn") docker_output=$(tr '\n' ',' <"$docker_churn")"

sudo ctr tasks kill --signal SIGTERM "$ctr_id" & ctr_stop_pid=$!; sudo docker stop -t 30 "$docker_name" >/dev/null & docker_stop_pid=$!; wait "$ctr_stop_pid"; wait "$docker_stop_pid"
for _ in $(seq 1 240); do ctr_state=$(sudo ctr tasks list | awk -v id="$ctr_id" '$1 == id {print $3}'); [[ $ctr_state = STOPPED ]] && break; sleep .25; done
[[ $ctr_state = STOPPED ]]
sudo ctr tasks rm "$ctr_id" >/dev/null; sudo ctr containers rm "$ctr_id"; sudo docker rm "$docker_name" >/dev/null
observe retained-final "$(wait_inventory "$clean_retained")"
idle_pid=$(systemctl show -p MainPID --value mkruntimed); sudo systemctl restart mkruntimed; released_pid=$(systemctl show -p MainPID --value mkruntimed); [[ $released_pid != "$idle_pid" ]]
observe released-final "$(wait_inventory "$clean_released")"
trap - EXIT; rm -rf -- "$scratch"
echo G6_CONCURRENT_CHURN_PASS
