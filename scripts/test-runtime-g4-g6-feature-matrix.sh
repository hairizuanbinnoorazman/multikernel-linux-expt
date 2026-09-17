#!/usr/bin/env bash
set -euo pipefail

if [[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]]; then
	PS4='+${BASH_SOURCE}:${LINENO}: '
	set -x
fi

# Full shared ctr/Docker feature audit for the G4-G6 implementation. Run only
# on an otherwise idle, disposable, qualified Multikernel host. Every positive
# row is exercised through both clients; unsupported Task v2 operations are
# recorded separately so an expected rejection is never presented as support.
image=${MK_TEST_IMAGE:-docker.io/library/busybox:1.36}
runtime=${MK_RUNTIME:-io.containerd.multikernel.v2}
ctr_id=mk-matrix-ctr
docker_name=mk-matrix-docker
ctr_attach_id=mk-matrix-ctr-attach
docker_attach_name=mk-matrix-docker-attach
ctr_bind_id=mk-matrix-ctr-bind
docker_bind_name=mk-matrix-docker-bind
bind_root=

cleanup() {
	(
	set +e
	sudo docker rm -f "$docker_name" >/dev/null 2>&1
	sudo docker rm -f "$docker_attach_name" >/dev/null 2>&1
	sudo docker rm -f "$docker_bind_name" >/dev/null 2>&1
	sudo ctr tasks kill --signal SIGKILL "$ctr_id" >/dev/null 2>&1
	sudo ctr tasks kill --signal SIGKILL "$ctr_attach_id" >/dev/null 2>&1
	sudo ctr tasks kill --signal SIGKILL "$ctr_bind_id" >/dev/null 2>&1
	for _ in $(seq 1 100); do
		[[ $(sudo ctr tasks list | awk -v id="$ctr_id" '$1==id {print $3}') != RUNNING ]] && break
		sleep .1
	done
	sudo ctr tasks rm -f "$ctr_id" >/dev/null 2>&1
	sudo ctr containers rm "$ctr_id" >/dev/null 2>&1
	sudo ctr tasks rm -f "$ctr_attach_id" >/dev/null 2>&1
	sudo ctr containers rm "$ctr_attach_id" >/dev/null 2>&1
	sudo ctr tasks rm -f "$ctr_bind_id" >/dev/null 2>&1
	sudo ctr containers rm "$ctr_bind_id" >/dev/null 2>&1
	if [[ -n ${bind_root:-} && -d $bind_root ]]; then
		rm -rf -- "$bind_root"
	fi
	true
	)
}
trap cleanup EXIT

wait_for_clean_host() {
	for _ in $(seq 1 180); do
		if test -z "$(sudo find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 -print -quit)" &&
		   test -z "$(ip -o link show | awk -F': ' '$2 ~ /^mkv[0-9a-f]+$/ {print $2}')" &&
		   ! sudo iptables -t nat -S POSTROUTING | grep -q '172\.31\.' &&
		   ! sudo iptables -S | grep -q '^\(-N\|-A\) MK-'; then
			return 0
		fi
		sleep .5
	done
	echo 'runtime resources did not return before the timeout' >&2
	return 1
}

wait_for_state() {
	local client=$1 wanted=$2 observed=
	for _ in $(seq 1 120); do
		if [[ $client = ctr ]]; then
			observed=$(sudo ctr tasks list | awk -v id="$ctr_id" '$1==id {print $3}')
		else
			observed=$(sudo docker inspect --format '{{.State.Status}}' "$docker_name" 2>/dev/null || true)
		fi
		[[ $observed = "$wanted" ]] && return 0
		sleep .25
	done
	echo "$client state is '$observed', wanted '$wanted'" >&2
	return 1
}

row() {
	printf 'FEATURE_MATRIX_PASS feature=%s ctr=PASS docker=PASS\n' "$1"
}

# Retain the values behind assertions. Fixed delimiters preserve multiline
# command output without lossy shell quoting.
observe() {
	local key=$1 value=$2
	printf 'OBSERVATION_BEGIN key=%s\n%s\nOBSERVATION_END key=%s\n' "$key" "$value" "$key"
}

clean_inventory() {
	printf 'children=%s links=%s nat_rules=%s filter_rules=%s ctr_tasks=%s docker_containers=%s' \
		"$(sudo find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 -type d | wc -l)" \
		"$(ip -o link show | awk -F': ' '$2 ~ /^mkv[0-9a-f]+$/ {count++} END {print count+0}')" \
		"$(sudo iptables -t nat -S POSTROUTING | grep -c '172\.31\.' || true)" \
		"$(sudo iptables -S | grep -c '^\(-N\|-A\) MK-' || true)" \
		"$(sudo ctr tasks list -q | wc -l)" \
		"$(sudo docker ps -aq | wc -l)"
}

test "$(id -u)" -ne 0 || {
	echo 'run as an ordinary sudo-capable user' >&2
	exit 1
}
for service in mkruntimed mknetd containerd docker; do
	test "$(systemctl is-active "$service")" = active
done
observe host "kernel=$(uname -r) boot_id=$(cat /proc/sys/kernel/random/boot_id) services=$(systemctl is-active mkruntimed mknetd containerd docker | paste -sd,)"
mountpoint -q /sys/fs/multikernel
test -x /usr/local/bin/containerd-shim-multikernel-v2
test -c /dev/net/tun
cleanup
wait_for_clean_host
observe initial-clean-inventory "$(clean_inventory)"

sudo ctr images pull "$image" >/dev/null
sudo docker image inspect "$image" >/dev/null 2>&1 || sudo docker pull "$image" >/dev/null
sudo ctr images list -q | grep -Fxq "$image"
sudo docker image inspect "$image" >/dev/null
observe ctr-image "$(sudo ctr images list | awk -v image="$image" 'NR == 1 || $1 == image')"
observe docker-image "$(sudo docker image inspect --format 'id={{.Id}} repo_digests={{json .RepoDigests}} architecture={{.Architecture}}' "$image")"
row image-pull-and-inspect

# Split create/start proves that support is not limited to the combined `run`
# convenience commands. Keep both children alive for the shared state, exec,
# isolation, networking, signal, and restart assertions below.
sudo ctr containers create --runtime "$runtime" "$image" "$ctr_id" /bin/sh -c \
	'trap "exit 42" TERM; while :; do sleep 1; done'
sudo ctr tasks start --detach "$ctr_id"
sudo docker create --runtime "$runtime" --name "$docker_name" \
	"$image" /bin/sh -c 'trap "exit 42" TERM; while :; do sleep 1; done' >/dev/null
sudo docker start "$docker_name" >/dev/null
wait_for_state ctr RUNNING
wait_for_state docker running
observe split-create-state "ctr=$(sudo ctr tasks list | awk -v id="$ctr_id" '$1==id {print $3}') docker=$(sudo docker inspect --format '{{.State.Status}}' "$docker_name")"
row split-create-and-start

test "$(sudo ctr tasks list | awk -v id="$ctr_id" '$1==id {print $3}')" = RUNNING
test "$(sudo docker inspect --format '{{.State.Running}}' "$docker_name")" = true
row state-and-inspect

ctr_boot=$(sudo ctr task exec --exec-id matrix-ctr-boot "$ctr_id" \
	/bin/cat /proc/sys/kernel/random/boot_id)
docker_boot=$(sudo docker exec "$docker_name" \
	/bin/cat /proc/sys/kernel/random/boot_id)
ctr_kernel=$(sudo ctr task exec --exec-id matrix-ctr-kernel "$ctr_id" /bin/uname -r)
docker_kernel=$(sudo docker exec "$docker_name" /bin/uname -r)
host_boot=$(cat /proc/sys/kernel/random/boot_id)
test -n "$ctr_boot"
test -n "$docker_boot"
test "$ctr_boot" != "$docker_boot"
test "$ctr_boot" != "$host_boot"
test "$docker_boot" != "$host_boot"
observe kernel-identities "host_boot=$host_boot ctr_boot=$ctr_boot docker_boot=$docker_boot host_kernel=$(uname -r) ctr_kernel=$ctr_kernel docker_kernel=$docker_kernel"
row exec-and-distinct-kernel-identity

ctr_stdio=$(sudo ctr task exec --exec-id matrix-ctr-stdio "$ctr_id" \
	/bin/sh -c 'echo ctr-stdout; echo ctr-stderr >&2' 2>&1)
docker_stdio=$(sudo docker exec "$docker_name" \
	/bin/sh -c 'echo docker-stdout; echo docker-stderr >&2' 2>&1)
printf '%s\n' "$ctr_stdio" | grep -Fxq ctr-stdout
printf '%s\n' "$ctr_stdio" | grep -Fxq ctr-stderr
printf '%s\n' "$docker_stdio" | grep -Fxq docker-stdout
printf '%s\n' "$docker_stdio" | grep -Fxq docker-stderr
observe exec-stdio "ctr=$ctr_stdio
docker=$docker_stdio"
row exec-stdout-and-stderr

sudo ctr task exec --exec-id matrix-ctr-write "$ctr_id" \
	/bin/sh -c 'printf ctr-private >/tmp/matrix-owner'
sudo docker exec "$docker_name" \
	/bin/sh -c 'printf docker-private >/tmp/matrix-owner'
ctr_private=$(sudo ctr task exec --exec-id matrix-ctr-read "$ctr_id" /bin/cat /tmp/matrix-owner)
docker_private=$(sudo docker exec "$docker_name" /bin/cat /tmp/matrix-owner)
test "$ctr_private" = ctr-private
test "$docker_private" = docker-private
observe private-root-values "ctr=$ctr_private docker=$docker_private"
row private-writable-root

# Standard clients emit different read-only bind option sets: ctr supplies an
# explicit rbind/ro pair, while Docker adds rprivate. The adapter must admit
# both, remove the host source from the guest projection, preserve the admitted
# bytes, and enforce a read-only guest mount without changing either host tree.
bind_root=$(mktemp -d -p /tmp mk-runtime-bind-matrix.XXXXXX)
chmod 0755 "$bind_root"
mkdir -m 0755 "$bind_root/ctr" "$bind_root/docker"
printf 'ctr-host-immutable\n' >"$bind_root/ctr/value"
printf 'docker-host-immutable\n' >"$bind_root/docker/value"
printf 'ctr-file-immutable\n' >"$bind_root/ctr-file"
printf 'docker-file-immutable\n' >"$bind_root/docker-file"
chmod 0644 "$bind_root/ctr/value" "$bind_root/docker/value" "$bind_root/ctr-file" "$bind_root/docker-file"
bind_probe='set -eu; directory_before=$(cat /opt/input/value); file_before=$(cat /etc/mk-input.conf); if printf changed >/opt/input/value 2>/dev/null; then echo writable-directory-bind >&2; exit 90; fi; if printf changed >/etc/mk-input.conf 2>/dev/null; then echo writable-file-bind >&2; exit 91; fi; test "$directory_before" = "$(cat /opt/input/value)"; test "$file_before" = "$(cat /etc/mk-input.conf)"; printf "%s|%s\n" "$directory_before" "$file_before"'
ctr_bind_output=$(sudo ctr run --rm --runtime "$runtime" \
	--mount "type=bind,src=$bind_root/ctr,dst=/opt/input,options=rbind:ro" \
	--mount "type=bind,src=$bind_root/ctr-file,dst=/etc/mk-input.conf,options=bind:ro" \
	"$image" "$ctr_bind_id" /bin/sh -c "$bind_probe")
docker_bind_output=$(sudo docker run --rm --runtime "$runtime" \
	--name "$docker_bind_name" --mount "type=bind,src=$bind_root/docker,dst=/opt/input,readonly" \
	--mount "type=bind,src=$bind_root/docker-file,dst=/etc/mk-input.conf,readonly" \
	"$image" /bin/sh -c "$bind_probe")
test "$ctr_bind_output" = 'ctr-host-immutable|ctr-file-immutable'
test "$docker_bind_output" = 'docker-host-immutable|docker-file-immutable'
test "$(cat "$bind_root/ctr/value")" = ctr-host-immutable
test "$(cat "$bind_root/docker/value")" = docker-host-immutable
test "$(cat "$bind_root/ctr-file")" = ctr-file-immutable
test "$(cat "$bind_root/docker-file")" = docker-file-immutable
observe readonly-bind-inputs "ctr=$ctr_bind_output docker=$docker_bind_output ctr_host=$(cat "$bind_root/ctr/value") ctr_file=$(cat "$bind_root/ctr-file") docker_host=$(cat "$bind_root/docker/value") docker_file=$(cat "$bind_root/docker-file")"
rm -rf -- "$bind_root"
bind_root=
row readonly-bind-inputs

ctr_network=$(sudo ctr task exec --exec-id matrix-ctr-network "$ctr_id" /bin/sh -c \
	'ip -4 address show dev mkn0; nslookup example.com; wget -T 15 -qO- http://example.com | sha256sum; echo network-ok')
docker_network=$(sudo docker exec "$docker_name" /bin/sh -c \
	'ip -4 address show dev mkn0; nslookup example.com; wget -T 15 -qO- http://example.com | sha256sum; echo network-ok')
printf '%s\n' "$ctr_network" | grep -q network-ok
printf '%s\n' "$docker_network" | grep -q network-ok
ctr_ip=$(printf '%s\n' "$ctr_network" | awk '/inet / {sub("/.*", "", $2); print $2; exit}')
docker_ip=$(printf '%s\n' "$docker_network" | awk '/inet / {sub("/.*", "", $2); print $2; exit}')
test -n "$ctr_ip"
test -n "$docker_ip"
test "$ctr_ip" != "$docker_ip"
observe mediated-network "ctr=$ctr_network
docker=$docker_network"
row primary-mediated-network

set +e
sudo ctr task exec --exec-id matrix-ctr-cross "$ctr_id" /bin/ping -c 1 -W 2 "$docker_ip" >/dev/null 2>&1
ctr_cross_rc=$?
sudo docker exec "$docker_name" /bin/ping -c 1 -W 2 "$ctr_ip" >/dev/null 2>&1
docker_cross_rc=$?
set -e
if test "$ctr_cross_rc" -eq 0; then
	echo 'ctr child reached the Docker child link' >&2
	exit 1
fi
if test "$docker_cross_rc" -eq 0; then
	echo 'Docker child reached the ctr child link' >&2
	exit 1
fi
observe sibling-isolation "ctr_to_docker_ip=$docker_ip exit_status=$ctr_cross_rc docker_to_ctr_ip=$ctr_ip exit_status=$docker_cross_rc"
row cross-sandbox-network-isolation

runtime_pid_before=$(systemctl show -p MainPID --value mkruntimed)
sudo systemctl restart mkruntimed
runtime_pid_after=$(systemctl show -p MainPID --value mkruntimed)
test "$(systemctl is-active mkruntimed)" = active
test "$(sudo ctr task exec --exec-id matrix-ctr-daemon-restart "$ctr_id" \
	/bin/cat /proc/sys/kernel/random/boot_id)" = "$ctr_boot"
test "$(sudo docker exec "$docker_name" \
	/bin/cat /proc/sys/kernel/random/boot_id)" = "$docker_boot"
test "$runtime_pid_before" != "$runtime_pid_after"
observe runtime-restart "pid_before=$runtime_pid_before pid_after=$runtime_pid_after ctr_boot=$ctr_boot docker_boot=$docker_boot"
row runtime-daemon-restart-continuity

# Pause and resume are guest process-group signals. Verify the externally
# visible Task v2 states as well as successful requests through both clients.
sudo ctr tasks pause "$ctr_id"
sudo docker pause "$docker_name" >/dev/null
wait_for_state ctr PAUSED
wait_for_state docker paused
sudo ctr tasks resume "$ctr_id"
sudo docker unpause "$docker_name" >/dev/null
wait_for_state ctr RUNNING
wait_for_state docker running
observe pause-resume-states "ctr=$(sudo ctr tasks list | awk -v id="$ctr_id" '$1==id {print $3}') docker=$(sudo docker inspect --format '{{.State.Status}}' "$docker_name")"
row pause-resume

sudo ctr tasks kill --signal SIGTERM "$ctr_id"
sudo docker kill --signal TERM "$docker_name" >/dev/null
wait_for_state ctr STOPPED
wait_for_state docker exited
docker_signal_exit=$(sudo docker wait "$docker_name")
test "$docker_signal_exit" = 42
ctr_signal_state=$(sudo ctr tasks list | awk -v id="$ctr_id" '$1==id {print $0}')
observe signal-exit "ctr_task=$ctr_signal_state docker_status=exited docker_exit=$docker_signal_exit"
row signal-and-exit-observation

sudo ctr tasks rm "$ctr_id" >/dev/null
sudo ctr containers rm "$ctr_id"
sudo docker rm "$docker_name" >/dev/null
wait_for_clean_host
observe post-delete-clean-inventory "$(clean_inventory)"
row delete-and-resource-cleanup

# Foreground run, init stdout/stderr, and nonzero exit are proved using the
# same reusable names twice. This also catches stale generation/idempotency
# state that a single lifecycle cannot expose.
for cycle in 1 2; do
	set +e
	ctr_output=$(sudo ctr run --runtime "$runtime" "$image" "$ctr_id" \
		/bin/sh -c "echo ctr-cycle-$cycle; echo ctr-error-$cycle >&2; exit 17" 2>&1)
	ctr_rc=$?
	docker_output=$(sudo docker run --runtime "$runtime" \
		--name "$docker_name" "$image" /bin/sh -c \
		"echo docker-cycle-$cycle; echo docker-error-$cycle >&2; exit 17" 2>&1)
	docker_rc=$?
	set -e
	test "$ctr_rc" -eq 17
	test "$docker_rc" -eq 17
	printf '%s\n' "$ctr_output" | grep -q "ctr-cycle-$cycle"
	printf '%s\n' "$ctr_output" | grep -q "ctr-error-$cycle"
	printf '%s\n' "$docker_output" | grep -q "docker-cycle-$cycle"
	printf '%s\n' "$docker_output" | grep -q "docker-error-$cycle"
	test "$(sudo docker inspect --format '{{.State.ExitCode}}' "$docker_name")" = 17
	test "$(sudo docker wait "$docker_name")" = 17
	observe "nonzero-cycle-$cycle" "ctr_exit=$ctr_rc docker_exit=$docker_rc
ctr_output=$ctr_output
docker_output=$docker_output"
	sudo ctr tasks rm "$ctr_id" >/dev/null 2>&1 || true
	sudo ctr containers rm "$ctr_id"
	sudo docker rm "$docker_name" >/dev/null
	wait_for_clean_host
done
row foreground-wait-stdio-and-nonzero-exit
row name-reuse

# Stdin is carried to the already-running guest process rather than being
# consumed by the primary-host shim. Exercise the ordinary foreground path
# independently from reattachment so either regression is visible.
ctr_stdin=$(printf 'ctr-stdin\n' | sudo ctr run --runtime "$runtime" "$image" "$ctr_id" \
	/bin/sh -c 'read line; echo guest-$line')
docker_stdin=$(printf 'docker-stdin\n' | sudo docker run --interactive \
	--runtime "$runtime" --name "$docker_name" "$image" \
	/bin/sh -c 'read line; echo guest-$line')
printf '%s\n' "$ctr_stdin" | grep -Fxq guest-ctr-stdin
printf '%s\n' "$docker_stdin" | grep -Fxq guest-docker-stdin
observe guest-stdin "ctr=$ctr_stdin docker=$docker_stdin"
sudo ctr tasks rm "$ctr_id" >/dev/null 2>&1 || true
sudo ctr containers rm "$ctr_id"
sudo docker rm "$docker_name" >/dev/null
wait_for_clean_host
row guest-stdin

# Detach the creating clients, then reopen the task's existing FIFO set and
# drive the guest through that attachment. The process exits only after the
# newly attached client supplies its line, proving that attach is live I/O.
sudo ctr run --detach --runtime "$runtime" "$image" "$ctr_attach_id" \
	/bin/sh -c 'read line; echo ctr-attached-$line'
sudo docker run --detach --interactive --runtime "$runtime" \
	--name "$docker_attach_name" "$image" \
	/bin/sh -c 'read line; echo docker-attached-$line' >/dev/null
ctr_attached=$(printf 'stdin\n' | sudo ctr tasks attach "$ctr_attach_id")
# Docker detaches as soon as the attaching client's stdin reaches EOF. Keep
# that stream open briefly so the same attachment can receive the guest reply.
docker_attached=$({ printf 'stdin\n'; sleep 2; } | sudo docker attach "$docker_attach_name")
printf '%s\n' "$ctr_attached" | grep -Fxq ctr-attached-stdin
printf '%s\n' "$docker_attached" | grep -Fxq docker-attached-stdin
observe guest-attach "ctr=$ctr_attached docker=$docker_attached"
sudo ctr tasks rm "$ctr_attach_id" >/dev/null 2>&1 || true
sudo ctr containers rm "$ctr_attach_id"
sudo docker rm "$docker_attach_name" >/dev/null
wait_for_clean_host
row guest-attach

# ctr deliberately connects a terminal task to /dev/tty, which bypasses shell
# command substitution. Give each client a nested controlling PTY and capture
# that PTY's transcript. A nonzero stty size proves live ResizePty delivery,
# not only successful terminal allocation.
ctr_tty=$(script -q -e -c \
	"stty rows 37 cols 91; sudo ctr run --tty --runtime '$runtime' '$image' '$ctr_id' /bin/sh -c 'set -e; test -t 0; test -t 1; sleep 1; stty size; echo ctr-terminal-ok'" /dev/null)
docker_tty=$(script -q -e -c \
	"stty rows 37 cols 91; sudo docker run --tty --runtime '$runtime' --name '$docker_name' '$image' /bin/sh -c 'set -e; test -t 0; test -t 1; sleep 1; stty size; echo docker-terminal-ok'" /dev/null)
printf '%s\n' "$ctr_tty" | tr -d '\r' | grep -Fxq ctr-terminal-ok
printf '%s\n' "$docker_tty" | tr -d '\r' | grep -Fxq docker-terminal-ok
printf '%s\n' "$ctr_tty" | tr -d '\r' | grep -Fxq '37 91'
printf '%s\n' "$docker_tty" | tr -d '\r' | grep -Fxq '37 91'
observe terminal-mode "ctr=$ctr_tty
docker=$docker_tty"
sudo ctr tasks rm "$ctr_id" >/dev/null 2>&1 || true
sudo ctr containers rm "$ctr_id"
sudo docker rm "$docker_name" >/dev/null
wait_for_clean_host
row terminal-mode

# The earlier terminal row verifies allocation and initial sizing. These runs
# wait for the guest to print its initial size, then mutate the already-live
# client PTY and retain both values from inside the guest.
resize_guest='trap '\''echo resized:$(stty size); exit 0'\'' WINCH; echo ready:$(stty size); while :; do sleep 1; done'
"$(dirname "$0")/test-runtime-live-resize.py" -- \
	sudo ctr run --tty --runtime "$runtime" "$image" "$ctr_id" /bin/sh -c "$resize_guest"
sudo ctr tasks rm "$ctr_id" >/dev/null 2>&1 || true
sudo ctr containers rm "$ctr_id"
wait_for_clean_host
"$(dirname "$0")/test-runtime-live-resize.py" -- \
	sudo docker run --tty --runtime "$runtime" --name "$docker_name" "$image" /bin/sh -c "$resize_guest"
sudo docker rm "$docker_name" >/dev/null
wait_for_clean_host
observe final-clean-inventory "$(clean_inventory)"
row post-start-terminal-resize

trap - EXIT
echo G4_G6_CTR_DOCKER_FEATURE_MATRIX_PASS
