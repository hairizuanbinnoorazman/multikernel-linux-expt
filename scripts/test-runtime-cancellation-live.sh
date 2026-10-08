#!/usr/bin/env bash
set -euo pipefail
[[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]] && { PS4='+${BASH_SOURCE}:${LINENO}: '; set -x; }

source_root=${1:?usage: test-runtime-cancellation-live.sh SOURCE_ROOT}
runtime=${MK_RUNTIME:-io.containerd.multikernel.v2}
image=${MK_TEST_IMAGE:-docker.io/library/busybox:1.36}
task_id=mk-cancellation-live
unit=mkruntimed-cancel-qualification.service
install_root=/opt/mkruntime/qualification-cancel
blocker=$install_root/blocker
config=$install_root/config.json
control=/run/mkruntime-cancel-qualification
scratch=$(mktemp -d -p /var/tmp mk-cancel.XXXXXX)
official_running=1
client_pid=

observe() { local key=$1; shift; printf 'OBSERVATION_BEGIN key=%s\n%s\nOBSERVATION_END key=%s\n' "$key" "$*" "$key"; }

wait_official() {
	for _ in $(seq 1 120); do
		systemctl is-active --quiet mkruntimed && sudo test -S /run/mkruntimed.sock && return
		sleep .25
	done
	return 1
}

cleanup_task() {
	sudo ctr tasks kill --signal SIGKILL "$task_id" >/dev/null 2>&1 || true
	for _ in $(seq 1 120); do
		state=$(sudo ctr tasks list | awk -v id="$task_id" '$1 == id {print $3}')
		[[ $state != RUNNING && $state != PAUSED ]] && break
		sleep .25
	done
	sudo ctr tasks rm -f "$task_id" >/dev/null 2>&1 || true
	sudo ctr containers rm "$task_id" >/dev/null 2>&1 || true
}

restore() {
	status=$?
	set +e
	[[ -z $client_pid ]] || sudo kill -KILL "$client_pid" >/dev/null 2>&1 || true
	cleanup_task
	sudo systemctl stop "$unit" >/dev/null 2>&1 || true
	if (( ! official_running )); then
		sudo systemctl start mkruntimed >/dev/null 2>&1 || true
		wait_official >/dev/null 2>&1 || true
	fi
	sudo rm -rf -- "$install_root" "$control"
	rm -rf -- "$scratch"
	exit "$status"
}
trap restore EXIT

wait_clean() {
	for _ in $(seq 1 240); do
		if output=$(MK_EVIDENCE_XTRACE=0 "$source_root/scripts/audit-runtime-final-resources-live.sh" 2>&1); then
			printf '%s\n' "$output"
			return
		fi
		sleep .25
	done
	printf '%s\n' "$output"
	return 1
}

start_qualification_daemon() {
	local operation=$1 real=$2 use_config=$3 arg replaced=0
	mapfile -d '' original_argv <"/proc/$(systemctl show -p MainPID --value mkruntimed)/cmdline"
	qualification_argv=()
	for arg in "${original_argv[@]}"; do
		case "$operation:$arg" in
			rootfs-build:--rootfs-builder=*) qualification_argv+=("--rootfs-builder=$blocker"); replaced=1 ;;
			kerf-load:--config=*) qualification_argv+=("--config=$config"); replaced=1 ;;
			*) qualification_argv+=("$arg") ;;
		esac
	done
	[[ $replaced = 1 ]]
	sudo systemctl stop mkruntimed
	official_running=0
	sudo rm -rf -- "$control"
	sudo systemd-run --unit="$unit" --collect --property=Type=simple \
		--setenv="MK_CANCEL_REAL=$real" --setenv="MK_CANCEL_OPERATION=$operation" \
		--setenv="MK_CANCEL_CONTROL=$control" -- "${qualification_argv[@]}"
	for _ in $(seq 1 120); do
		systemctl is-active --quiet "$unit" && sudo test -S /run/mkruntimed.sock && return
		sleep .25
	done
	return 1
}

stop_qualification_daemon() {
	sudo systemctl stop "$unit"
	sudo systemctl start mkruntimed
	wait_official
	official_running=1
}

cancel_blocked_create() {
	local label=$1 status parent child
	sudo timeout --foreground --kill-after=10 300 ctr run --rm --runtime "$runtime" "$image" "$task_id" /bin/true >"$scratch/$label.client" 2>&1 &
	client_pid=$!
	for _ in $(seq 1 240); do sudo test -f "$control/ready" && break; kill -0 "$client_pid" 2>/dev/null || break; sleep .25; done
	sudo test -f "$control/ready"
	parent=$(sudo cat "$control/parent"); child=$(sudo cat "$control/child")
	sudo kill -TERM "$client_pid"
	set +e; wait "$client_pid"; status=$?; set -e
	client_pid=
	[[ $status -ne 0 ]]
	for _ in $(seq 1 240); do [[ ! -d /proc/$parent && ! -d /proc/$child ]] && break; sleep .25; done
	[[ ! -d /proc/$parent && ! -d /proc/$child ]]
	observe "$label-cancellation" "client_status=$status blocker_parent=$parent blocker_child=$child parent_reaped=true child_reaped=true"
	observe "$label-cleanup" "$(wait_clean)"
}

[[ $(id -u) -ne 0 ]] || { echo 'run as an ordinary sudo-capable user' >&2; exit 1; }
for path in "$source_root/scripts/runtime-cancellation-blocker.sh" "$source_root/scripts/audit-runtime-final-resources-live.sh"; do test -x "$path"; done
for service in mkruntimed mknetd containerd docker; do [[ $(systemctl is-active "$service") = active ]]; done
cleanup_task
observe clean-before "$(wait_clean)"
observe provenance "boot_id=$(cat /proc/sys/kernel/random/boot_id)
selector=$(readlink -f /usr/local/lib/multikernel/current)
qualifier_sha256=$(sha256sum "$0" | awk '{print $1}')
blocker_sha256=$(sha256sum "$source_root/scripts/runtime-cancellation-blocker.sh" | awk '{print $1}')"
sudo ctr images pull "$image" >/dev/null
sudo install -d -m 0700 "$install_root"
sudo install -m 0755 "$source_root/scripts/runtime-cancellation-blocker.sh" "$blocker"
real_builder=$(systemctl show -p ExecStart --value mkruntimed | grep -o -- '--rootfs-builder=[^ ;}]*' | head -1 | cut -d= -f2-)
[[ -x $real_builder ]]
start_qualification_daemon rootfs-build "$real_builder" false
cancel_blocked_create rootfs-build
stop_qualification_daemon

real_kerf=$(sudo jq -r .kerf_executable /etc/mkruntime/config.json)
sudo jq --arg blocker "$blocker" '.kerf_executable=$blocker' /etc/mkruntime/config.json | sudo tee "$config" >/dev/null
sudo chmod 0600 "$config"
start_qualification_daemon kerf-load "$real_kerf" true
cancel_blocked_create child-boot
stop_qualification_daemon

sudo ctr run --detach --runtime "$runtime" "$image" "$task_id" /bin/sh -c 'echo cancellation-ready; while :; do sleep 1; done'
before_pid=$(sudo ctr tasks list | awk -v id="$task_id" '$1 == id {print $2}')
set +e; sudo timeout --foreground --signal=TERM 2 ctr tasks wait "$task_id" >"$scratch/wait" 2>&1; wait_status=$?; set -e
[[ $wait_status = 124 ]]
[[ $(sudo ctr tasks list | awk -v id="$task_id" '$1 == id {print $3}') = RUNNING ]]
[[ $(sudo ctr tasks list | awk -v id="$task_id" '$1 == id {print $2}') = "$before_pid" ]]
set +e; sudo timeout --foreground --signal=TERM 2 ctr tasks attach "$task_id" >"$scratch/attach" 2>&1; attach_status=$?; set -e
[[ $attach_status = 124 ]]
[[ $(sudo ctr tasks list | awk -v id="$task_id" '$1 == id {print $3}') = RUNNING ]]
probe=$(sudo ctr task exec --exec-id cancellation-probe "$task_id" /bin/echo cancellation-still-usable)
[[ $probe = cancellation-still-usable ]]
observe task-rpc-cancellation "wait_status=$wait_status attach_status=$attach_status task_pid=$before_pid state=RUNNING post_cancel_exec=$probe"
cleanup_task
observe clean-after "$(wait_clean)"
for service in mkruntimed mknetd containerd docker; do [[ $(systemctl is-active "$service") = active ]]; done

trap - EXIT
sudo rm -rf -- "$install_root" "$control"
rm -rf -- "$scratch"
echo G6_CANCELLATION_LIVE_PASS
