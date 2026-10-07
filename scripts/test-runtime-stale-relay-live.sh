#!/usr/bin/env bash
set -euo pipefail
[[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]] && { PS4='+${BASH_SOURCE}:${LINENO}: '; set -x; }

source_root=${1:?usage: test-runtime-stale-relay-live.sh SOURCE_ROOT}
image=${MK_TEST_IMAGE:-docker.io/library/busybox:1.36}
runtime=${MK_RUNTIME:-io.containerd.multikernel.v2}
task_id=mk-stale-relay-live
task_root=/run/containerd/io.containerd.runtime.v2.task/default
host_config=/etc/mkruntime/config.json
kerf=/opt/mkruntime/kerf-venv/bin/kerf
scratch=$(mktemp -d -p /var/tmp mk-stale-relay.XXXXXX)
helper=$scratch/runtime-task-barrier
ready=$scratch/task-created.json
continuation=$scratch/start-task
helper_log=$scratch/helper.log
helper_pid=
relay_path=

observe() { local key=$1; shift; printf 'OBSERVATION_BEGIN key=%s\n%s\nOBSERVATION_END key=%s\n' "$key" "$*" "$key"; }
cleanup_task() {
	local state
	state=$(sudo ctr tasks list 2>/dev/null | awk -v id="$task_id" '$1 == id {print tolower($3)}')
	if [[ $state = created && -x $helper ]]; then
		sudo "$helper" --id "$task_id" --start-existing >/dev/null 2>&1 || true
	fi
	sudo ctr tasks kill --signal SIGKILL "$task_id" >/dev/null 2>&1 || true
	sudo ctr tasks rm -f "$task_id" >/dev/null 2>&1 || true
	sudo ctr containers rm "$task_id" >/dev/null 2>&1 || true
}
release_idle_pool() {
	local sandbox_count
	sandbox_count=$(sudo python3 - "$host_config" <<'PY'
import json, os, sys
config = json.load(open(sys.argv[1], encoding="utf-8"))
state = json.load(open(os.path.join(config["state_directory"], "state.json"), encoding="utf-8"))
print(len(state["sandboxes"]))
PY
)
	if [[ $sandbox_count = 0 ]] && ! sudo "$kerf" show 2>/dev/null | grep -Fq 'No memory pool configured'; then
		sudo systemctl restart mkruntimed
	fi
}
cleanup() {
	local status=$?
	set +e
	[[ -z $helper_pid ]] || { sudo kill "$helper_pid" >/dev/null 2>&1 || true; wait "$helper_pid" >/dev/null 2>&1 || true; }
	cleanup_task
	[[ -z $relay_path ]] || sudo rm -f -- "$relay_path"
	release_idle_pool
	rm -rf -- "$scratch"
	exit "$status"
}
trap cleanup EXIT

[[ $(id -u) -ne 0 ]] || { echo 'run as an ordinary sudo-capable user' >&2; exit 1; }
for service in mkruntimed mknetd containerd docker; do
	[[ $(systemctl is-active "$service") = active ]]
	[[ $(systemctl show -p NRestarts --value "$service") = 0 ]]
done
for path in "$source_root/runtime/go.mod" "$source_root/scripts/runtime-task-barrier.go" "$source_root/scripts/audit-runtime-final-resources-live.sh"; do test -f "$path"; done
cleanup_task
"$source_root/scripts/audit-runtime-final-resources-live.sh"

mkdir -p "$scratch/go-cache" "$scratch/go-tmp"
(
	cd "$source_root/runtime"
	GOCACHE="$scratch/go-cache" GOTMPDIR="$scratch/go-tmp" go build -trimpath -o "$helper" ../scripts/runtime-task-barrier.go
)
chmod 0755 "$helper"
observe provenance "boot_id=$(cat /proc/sys/kernel/random/boot_id)
selector=$(readlink -f /usr/local/lib/multikernel/current)
daemon_pid=$(systemctl show -p MainPID --value mkruntimed)
containerd_pid=$(systemctl show -p MainPID --value containerd)
helper_source_sha256=$(sha256sum "$source_root/scripts/runtime-task-barrier.go" | awk '{print $1}')
helper_binary_sha256=$(sha256sum "$helper" | awk '{print $1}')
qualifier_sha256=$(sha256sum "$0" | awk '{print $1}')"

sudo ctr images list -q | grep -Fxq "$image"
sudo ctr containers create --runtime "$runtime" "$image" "$task_id" /bin/sh -c 'while :; do sleep 1; done'
sudo "$helper" --id "$task_id" --ready "$ready" --continue "$continuation" >"$helper_log" 2>&1 &
helper_pid=$!
for _ in $(seq 1 2400); do sudo test -f "$ready" && break; kill -0 "$helper_pid" 2>/dev/null || break; sleep .25; done
sudo test -f "$ready"
created=$(sudo cat "$ready")
grep -Fq '"phase":"created"' <<<"$created"
grep -Fq '"status":"created"' <<<"$created"

recovery="$task_root/$task_id/.multikernel/sandbox.json"
sudo test -f "$recovery"
relay=$(sudo python3 - "$host_config" "$recovery" <<'PY'
import json, os, sys
config = json.load(open(sys.argv[1], encoding="utf-8"))
recovery = json.load(open(sys.argv[2], encoding="utf-8"))
state = json.load(open(os.path.join(config["state_directory"], "state.json"), encoding="utf-8"))
matches = [value for value in state["sandboxes"].values()
           if value.get("id") == recovery["id"] and value.get("generation") == recovery["generation"]]
if len(matches) != 1:
    raise SystemExit(f"sandbox identity matches={len(matches)}")
port = matches[0]["config"]["agent_port"]
generation = recovery["generation"]
path = f"/run/mk-agent-{port}-{generation[:12]}.sock"
print(json.dumps({"sandbox_id": recovery["id"], "generation": generation,
                  "agent_port": port, "path": path}, sort_keys=True, separators=(",", ":")))
PY
)
relay_path=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["path"])' "$relay")
sudo test ! -e "$relay_path"

stale=$(sudo python3 - "$relay_path" <<'PY'
import errno, json, os, socket, stat, sys
path = sys.argv[1]
listener = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
listener.bind(path)
os.chmod(path, 0o755)
before = os.lstat(path)
listener.close()
probe = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
observed = None
try:
    probe.connect(path)
except OSError as error:
    observed = error.errno
finally:
    probe.close()
if observed != errno.ECONNREFUSED:
    raise SystemExit(f"stale socket probe errno={observed}")
print(json.dumps({"path": path, "inode": before.st_ino, "device": before.st_dev,
                  "uid": before.st_uid, "gid": before.st_gid, "mode": oct(stat.S_IMODE(before.st_mode)),
                  "links": before.st_nlink, "connect_errno": observed}, sort_keys=True, separators=(",", ":")))
PY
)
stale_inode=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["inode"])' "$stale")
observe created-barrier-and-stale-socket "task=$created
relay=$relay
stale=$stale"

sudo touch "$continuation"
set +e
wait "$helper_pid"
helper_status=$?
set -e
helper_pid=
[[ $helper_status -eq 0 ]]
helper_output=$(<"$helper_log")
grep -Fq '"phase":"started"' <<<"$helper_output"
grep -Fq '"status":"running"' <<<"$helper_output"

replacement=$(sudo python3 - "$relay_path" "$stale_inode" <<'PY'
import json, os, socket, stat, sys
path, stale = sys.argv[1], int(sys.argv[2])
value = os.lstat(path)
if not stat.S_ISSOCK(value.st_mode) or value.st_ino == stale or value.st_uid != 0 or value.st_nlink != 1:
    raise SystemExit(f"invalid replacement identity mode={value.st_mode:o} inode={value.st_ino} uid={value.st_uid} links={value.st_nlink}")
probe = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
probe.settimeout(2)
probe.connect(path)
probe.close()
print(json.dumps({"path": path, "inode": value.st_ino, "device": value.st_dev,
                  "uid": value.st_uid, "gid": value.st_gid, "mode": oct(stat.S_IMODE(value.st_mode)),
                  "links": value.st_nlink, "connect": "accepted"}, sort_keys=True, separators=(",", ":")))
PY
)
workload=$(sudo ctr task exec --exec-id stale-relay-proof "$task_id" /bin/sh -c 'printf STALE_RELAY_REPLACEMENT_PASS')
[[ $workload = STALE_RELAY_REPLACEMENT_PASS ]]
child_boot=$(sudo ctr task exec --exec-id stale-relay-boot "$task_id" /bin/cat /proc/sys/kernel/random/boot_id)
relay_process=$(sudo python3 - "$relay_path" <<'PY'
import json, os, sys
path = sys.argv[1]
matches = []
for name in os.listdir("/proc"):
    if not name.isdigit():
        continue
    try:
        argv = open(f"/proc/{name}/cmdline", "rb").read().split(b"\0")
        if argv and os.fsdecode(argv[-2]) == path:
            matches.append({"pid": int(name), "argv": [os.fsdecode(value) for value in argv if value]})
    except (FileNotFoundError, PermissionError, IndexError):
        pass
if len(matches) != 1:
    raise SystemExit(f"relay process matches={len(matches)}")
print(json.dumps(matches[0], sort_keys=True, separators=(",", ":")))
PY
)
observe live-replacement "helper_status=$helper_status
helper_output=$helper_output
replacement=$replacement
relay_process=$relay_process
child_boot=$child_boot
workload=$workload"

cleanup_task
for _ in $(seq 1 240); do sudo test ! -e "$relay_path" && break; sleep .25; done
sudo test ! -e "$relay_path"
old_daemon=$(systemctl show -p MainPID --value mkruntimed)
sudo systemctl restart mkruntimed
new_daemon=$(systemctl show -p MainPID --value mkruntimed)
[[ $new_daemon != "$old_daemon" ]]
"$source_root/scripts/audit-runtime-final-resources-live.sh"
observe final-cleanup "stale_inode=$stale_inode relay_path_absent=true old_daemon=$old_daemon new_daemon=$new_daemon"

trap - EXIT
rm -rf -- "$scratch"
echo G6_STALE_RELAY_LIVE_PASS
