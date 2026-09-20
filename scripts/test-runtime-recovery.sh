#!/usr/bin/env bash
set -uo pipefail

if [[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]]; then
	PS4='+${BASH_SOURCE}:${LINENO}: '
	set -x
fi

# Live G6 recovery audit. Run only on a disposable, otherwise idle qualified
# host. Cases use distinct names and always attempt bounded cleanup so one
# recovery failure does not hide the remaining results.
image=${MK_TEST_IMAGE:-docker.io/library/busybox:1.36}
runtime=${MK_RUNTIME:-io.containerd.multikernel.v2}
id=
failures=0

cleanup() {
	(
	set +e
	test -n "$id" || exit 0
	sudo ctr tasks kill --signal SIGKILL "$id" >/dev/null 2>&1
	sudo ctr tasks rm -f "$id" >/dev/null 2>&1
	sudo ctr containers rm "$id" >/dev/null 2>&1
	true
	)
}
trap cleanup EXIT

wait_clean() {
	for _ in $(seq 1 180); do
		if ! sudo ctr containers info "$id" >/dev/null 2>&1 &&
		   test ! -d "/sys/fs/multikernel/instances/mk-$id" &&
		   ! ip -o link show | grep -qE 'mkn[0-9]+' &&
		   ! sudo iptables -t nat -S POSTROUTING | grep -q '172\.30\.' &&
		   ! sudo iptables -S FORWARD | grep -qE 'mkn[0-9]+'; then
			return 0
		fi
		sleep .5
	done
	echo "CLEANUP_TIMEOUT id=$id" >&2
	return 1
}

start_task() {
	cleanup
	wait_clean || return
	sudo ctr run -d --runtime "$runtime" "$image" "$id" /bin/sleep 300 || return
	test "$(sudo ctr tasks list | awk -v id="$id" '$1==id {print $3}')" = RUNNING
}

child_boot() {
	sudo ctr task exec --exec-id "boot-$1" "$id" /bin/cat /proc/sys/kernel/random/boot_id
}

record() {
	local name=$1 rc=$2
	cleanup
	wait_clean || rc=1
	if [[ $rc -eq 0 ]]; then
		echo "${name}_PASS"
	else
		echo "${name}_FAIL"
		failures=$((failures + 1))
	fi
}

containerd_restart() {
	start_task || return
	local before status=
	before=$(child_boot before-containerd) || return
	sudo systemctl restart containerd || return
	for _ in $(seq 1 40); do
		status=$(sudo ctr tasks list | awk -v id="$id" '$1==id {print $3}')
		[[ $status = RUNNING ]] && break
		sleep .25
	done
	[[ $status = RUNNING ]] || return 1
	[[ $(child_boot after-containerd) = "$before" ]]
}

daemon_restart() {
	start_task || return
	local before
	before=$(child_boot before-daemon) || return
	sudo systemctl restart mkruntimed || return
	[[ $(systemctl is-active mkruntimed) = active ]] || return
	[[ $(child_boot after-daemon) = "$before" ]]
}

shim_crash_reconnect() {
	start_task || return
	local before supervisor_pid worker_pid replacement_pid= status=
	before=$(child_boot before-shim-crash) || return
	supervisor_pid=$(sudo ctr tasks list | awk -v id="$id" '$1==id {print $2}')
	[[ -n $supervisor_pid ]] || return
	worker_pid=$(sudo cat "/proc/$supervisor_pid/cwd/.multikernel-worker.pid") || return
	echo "SHIM_CRASH_BEFORE id=$id boot_id=$before supervisor_pid=$supervisor_pid worker_pid=$worker_pid status=RUNNING"
	sudo kill -KILL "$worker_pid" || return
	for _ in $(seq 1 40); do
		replacement_pid=$(sudo cat "/proc/$supervisor_pid/cwd/.multikernel-worker.pid" 2>/dev/null || true)
		status=$(sudo ctr tasks list | awk -v id="$id" '$1==id {print $3}')
		[[ $status = RUNNING && -n $replacement_pid && $replacement_pid != "$worker_pid" ]] && break
		sleep .25
	done
	[[ $status = RUNNING ]] || return 1
	[[ -n $replacement_pid && $replacement_pid != "$worker_pid" ]] || return 1
	local after
	after=$(child_boot after-shim-crash) || return
	echo "SHIM_CRASH_AFTER id=$id boot_id=$after supervisor_pid=$supervisor_pid old_worker_pid=$worker_pid replacement_worker_pid=$replacement_pid status=$status"
	[[ $after = "$before" ]]
}

if [[ $(id -u) -eq 0 ]]; then
	echo 'run as an ordinary sudo-capable user' >&2
	exit 1
fi
for service in mkruntimed containerd; do
	[[ $(systemctl is-active "$service") = active ]] || exit 1
done
mountpoint -q /sys/fs/multikernel || exit 1
mountpoint -q /srv/multikernel-storage || exit 1
[[ -z $(sudo find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 -print -quit) ]] || exit 1
! ip -o link show | grep -qE 'mkn[0-9]+' || exit 1
! sudo iptables -t nat -S POSTROUTING | grep -q '172\.30\.' || exit 1
! sudo iptables -S FORWARD | grep -qE 'mkn[0-9]+' || exit 1
sudo ctr images pull "$image" >/dev/null || exit 1

id=mk-recovery-containerd
containerd_restart
record CONTAINERD_RESTART_RECONNECT $?

id=mk-recovery-daemon
daemon_restart
record MKRUNTIMED_RESTART_RECONNECT $?

id=mk-recovery-shim
shim_crash_reconnect
record SHIM_CRASH_RECONNECT $?

trap - EXIT
if [[ $failures -ne 0 ]]; then
	echo "G6_RESTART_RECOVERY_AUDIT_FAIL failures=$failures"
	exit 1
fi
echo G6_RESTART_RECOVERY_AUDIT_PASS
