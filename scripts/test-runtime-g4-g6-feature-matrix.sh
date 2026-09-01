#!/usr/bin/env bash
set -euo pipefail

# Full shared ctr/Docker feature audit for the G4-G6 implementation. Run only
# on an otherwise idle, disposable, qualified Multikernel host. Every positive
# row is exercised through both clients; unsupported Task v2 operations are
# recorded separately so an expected rejection is never presented as support.
image=${MK_TEST_IMAGE:-docker.io/library/busybox:1.36}
runtime=${MK_RUNTIME:-io.containerd.multikernel.v2}
ctr_id=mk-matrix-ctr
docker_name=mk-matrix-docker

cleanup() {
	(
	set +e
	sudo docker rm -f "$docker_name" >/dev/null 2>&1
	sudo ctr tasks kill --signal SIGKILL "$ctr_id" >/dev/null 2>&1
	for _ in $(seq 1 100); do
		[[ $(sudo ctr tasks list | awk -v id="$ctr_id" '$1==id {print $3}') != RUNNING ]] && break
		sleep .1
	done
	sudo ctr tasks rm -f "$ctr_id" >/dev/null 2>&1
	sudo ctr containers rm "$ctr_id" >/dev/null 2>&1
	true
	)
}
trap cleanup EXIT

wait_for_clean_host() {
	for _ in $(seq 1 180); do
		if test -z "$(sudo find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 -print -quit)" &&
		   test -z "$(ip -o link show | awk -F': ' '$2 ~ /^mkn[0-9]+$/ {print $2}')" &&
		   ! sudo iptables -t nat -S POSTROUTING | grep -q '172\.30\.' &&
		   ! sudo iptables -S FORWARD | grep -q 'mkn[0-9]'; then
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

test "$(id -u)" -ne 0 || {
	echo 'run as an ordinary sudo-capable user' >&2
	exit 1
}
for service in mkruntimed containerd docker; do
	test "$(systemctl is-active "$service")" = active
done
mountpoint -q /sys/fs/multikernel
test -x /usr/local/bin/containerd-shim-multikernel-v2
test -c /dev/net/tun
cleanup
wait_for_clean_host

sudo ctr images pull "$image" >/dev/null
sudo docker image inspect "$image" >/dev/null 2>&1 || sudo docker pull "$image" >/dev/null
sudo ctr images list -q | grep -Fxq "$image"
sudo docker image inspect "$image" >/dev/null
row image-pull-and-inspect

# Split create/start proves that support is not limited to the combined `run`
# convenience commands. Keep both children alive for the shared state, exec,
# isolation, networking, signal, and restart assertions below.
sudo ctr containers create --runtime "$runtime" "$image" "$ctr_id" /bin/sh -c \
	'trap "exit 42" TERM; while :; do sleep 1; done'
sudo ctr tasks start --detach "$ctr_id"
sudo docker create --runtime "$runtime" --network none --name "$docker_name" \
	"$image" /bin/sh -c 'trap "exit 42" TERM; while :; do sleep 1; done' >/dev/null
sudo docker start "$docker_name" >/dev/null
wait_for_state ctr RUNNING
wait_for_state docker running
row split-create-and-start

test "$(sudo ctr tasks list | awk -v id="$ctr_id" '$1==id {print $3}')" = RUNNING
test "$(sudo docker inspect --format '{{.State.Running}}' "$docker_name")" = true
row state-and-inspect

ctr_boot=$(sudo ctr task exec --exec-id matrix-ctr-boot "$ctr_id" \
	/bin/cat /proc/sys/kernel/random/boot_id)
docker_boot=$(sudo docker exec "$docker_name" \
	/bin/cat /proc/sys/kernel/random/boot_id)
host_boot=$(cat /proc/sys/kernel/random/boot_id)
test -n "$ctr_boot"
test -n "$docker_boot"
test "$ctr_boot" != "$docker_boot"
test "$ctr_boot" != "$host_boot"
test "$docker_boot" != "$host_boot"
row exec-and-distinct-kernel-identity

ctr_stdio=$(sudo ctr task exec --exec-id matrix-ctr-stdio "$ctr_id" \
	/bin/sh -c 'echo ctr-stdout; echo ctr-stderr >&2' 2>&1)
docker_stdio=$(sudo docker exec "$docker_name" \
	/bin/sh -c 'echo docker-stdout; echo docker-stderr >&2' 2>&1)
printf '%s\n' "$ctr_stdio" | grep -Fxq ctr-stdout
printf '%s\n' "$ctr_stdio" | grep -Fxq ctr-stderr
printf '%s\n' "$docker_stdio" | grep -Fxq docker-stdout
printf '%s\n' "$docker_stdio" | grep -Fxq docker-stderr
row exec-stdout-and-stderr

sudo ctr task exec --exec-id matrix-ctr-write "$ctr_id" \
	/bin/sh -c 'printf ctr-private >/tmp/matrix-owner'
sudo docker exec "$docker_name" \
	/bin/sh -c 'printf docker-private >/tmp/matrix-owner'
test "$(sudo ctr task exec --exec-id matrix-ctr-read "$ctr_id" \
	/bin/cat /tmp/matrix-owner)" = ctr-private
test "$(sudo docker exec "$docker_name" \
	/bin/cat /tmp/matrix-owner)" = docker-private
row private-writable-root

ctr_network=$(sudo ctr task exec --exec-id matrix-ctr-network "$ctr_id" /bin/sh -c \
	'ip -4 address show dev mkn0; wget -T 15 -qO- http://example.com >/dev/null; echo network-ok')
docker_network=$(sudo docker exec "$docker_name" /bin/sh -c \
	'ip -4 address show dev mkn0; wget -T 15 -qO- http://example.com >/dev/null; echo network-ok')
printf '%s\n' "$ctr_network" | grep -q '172\.30\.30\.2/30'
printf '%s\n' "$docker_network" | grep -q '172\.30\.31\.2/30'
printf '%s\n' "$ctr_network" | grep -q network-ok
printf '%s\n' "$docker_network" | grep -q network-ok
row primary-mediated-network

if sudo ctr task exec --exec-id matrix-ctr-cross "$ctr_id" \
	/bin/ping -c 1 -W 2 172.30.31.2 >/dev/null 2>&1; then
	echo 'ctr child reached the Docker child link' >&2
	exit 1
fi
if sudo docker exec "$docker_name" \
	/bin/ping -c 1 -W 2 172.30.30.2 >/dev/null 2>&1; then
	echo 'Docker child reached the ctr child link' >&2
	exit 1
fi
row cross-sandbox-network-isolation

sudo systemctl restart mkruntimed
test "$(systemctl is-active mkruntimed)" = active
test "$(sudo ctr task exec --exec-id matrix-ctr-daemon-restart "$ctr_id" \
	/bin/cat /proc/sys/kernel/random/boot_id)" = "$ctr_boot"
test "$(sudo docker exec "$docker_name" \
	/bin/cat /proc/sys/kernel/random/boot_id)" = "$docker_boot"
row runtime-daemon-restart-continuity

# These calls reach unimplemented Task v2 methods and must fail without
# changing either running task. Their rejection is part of the audit, not a
# supported-feature row.
if sudo ctr tasks pause "$ctr_id" >/dev/null 2>&1; then
	echo 'ctr pause unexpectedly succeeded' >&2
	exit 1
fi
if sudo docker pause "$docker_name" >/dev/null 2>&1; then
	echo 'Docker pause unexpectedly succeeded' >&2
	exit 1
fi
wait_for_state ctr RUNNING
wait_for_state docker running
echo 'FEATURE_MATRIX_UNSUPPORTED feature=pause-resume ctr=REJECTED docker=REJECTED'

sudo ctr tasks kill --signal SIGTERM "$ctr_id"
sudo docker kill --signal TERM "$docker_name" >/dev/null
wait_for_state ctr STOPPED
wait_for_state docker exited
test "$(sudo docker wait "$docker_name")" = 42
row signal-and-exit-observation

sudo ctr tasks rm "$ctr_id" >/dev/null
sudo ctr containers rm "$ctr_id"
sudo docker rm "$docker_name" >/dev/null
wait_for_clean_host
row delete-and-resource-cleanup

# Foreground run, init stdout/stderr, and nonzero exit are proved using the
# same reusable names twice. This also catches stale generation/idempotency
# state that a single lifecycle cannot expose.
for cycle in 1 2; do
	set +e
	ctr_output=$(sudo ctr run --runtime "$runtime" "$image" "$ctr_id" \
		/bin/sh -c "echo ctr-cycle-$cycle; echo ctr-error-$cycle >&2; exit 17" 2>&1)
	ctr_rc=$?
	docker_output=$(sudo docker run --runtime "$runtime" --network none \
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
	sudo ctr tasks rm "$ctr_id" >/dev/null 2>&1 || true
	sudo ctr containers rm "$ctr_id"
	sudo docker rm "$docker_name" >/dev/null
	wait_for_clean_host
done
row foreground-wait-stdio-and-nonzero-exit
row name-reuse

ctr_tty=$(sudo ctr run --tty --runtime "$runtime" "$image" "$ctr_id" \
	/bin/sh -c 'test -t 0; test -t 1; echo ctr-terminal-ok')
docker_tty=$(sudo docker run --tty --runtime "$runtime" --network none \
	--name "$docker_name" "$image" \
	/bin/sh -c 'test -t 0; test -t 1; echo docker-terminal-ok')
printf '%s\n' "$ctr_tty" | tr -d '\r' | grep -Fxq ctr-terminal-ok
printf '%s\n' "$docker_tty" | tr -d '\r' | grep -Fxq docker-terminal-ok
sudo ctr tasks rm "$ctr_id" >/dev/null 2>&1 || true
sudo ctr containers rm "$ctr_id"
sudo docker rm "$docker_name" >/dev/null
wait_for_clean_host
row terminal-mode

trap - EXIT
echo G4_G6_CTR_DOCKER_FEATURE_MATRIX_PASS
