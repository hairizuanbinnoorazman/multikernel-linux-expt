#!/usr/bin/env bash
set -euo pipefail

if [[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]]; then
	PS4='+${BASH_SOURCE}:${LINENO}: '
	set -x
fi

# Privileged GCE integration proof for the G4-G6 MVP path. Run on a qualified
# Multikernel host after mkruntimed, the Runtime v2 shim, and Docker are active.
image=${MK_TEST_IMAGE:-docker.io/library/busybox:1.36}
runtime=${MK_RUNTIME:-io.containerd.multikernel.v2}
ctr_id=mk-proof-ctr
docker_name=mk-proof-docker

cleanup() {
	(
	set +e
	sudo docker rm -f "$docker_name" >/dev/null 2>&1
	sudo ctr tasks kill --signal SIGKILL "$ctr_id" >/dev/null 2>&1
	sudo ctr tasks rm -f "$ctr_id" >/dev/null 2>&1
	sudo ctr containers rm "$ctr_id" >/dev/null 2>&1
	true
	)
}
trap cleanup EXIT
cleanup

test "$(id -u)" -ne 0 || { echo 'run as an ordinary sudo-capable user' >&2; exit 1; }
for service in mkruntimed mknetd containerd docker; do
	test "$(systemctl is-active "$service")" = active
done
mountpoint -q /sys/fs/multikernel || {
	echo '/sys/fs/multikernel is not mounted; install sys-fs-multikernel.mount' >&2
	exit 1
}
mountpoint -q /srv/multikernel-storage || {
	echo '/srv/multikernel-storage is not a distinct mounted filesystem' >&2
	exit 1
}
test -x /usr/local/bin/containerd-shim-multikernel-v2
test -c /dev/net/tun
test -z "$(sudo find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 -print -quit)"

# A fresh disposable host has no image cache. Pull explicitly so a missing
# precondition is not mistaken for a Runtime v2 failure.
sudo ctr images pull "$image" >/dev/null
sudo docker image inspect "$image" >/dev/null 2>&1 || sudo docker pull "$image" >/dev/null

host_boot=$(cat /proc/sys/kernel/random/boot_id)
host_release=$(uname -r)
echo "HOST_BOOT_ID=$host_boot"
echo "HOST_KERNEL=$host_release"
echo "IMAGE=$image"
echo "RUNTIME=$runtime"

# ctr and Docker each request a container while the other is still alive. The
# two boot IDs and addresses prove distinct kernels and disjoint network links.
sudo ctr run -d --runtime "$runtime" "$image" "$ctr_id" /bin/sleep 300
test "$(sudo ctr tasks list | awk -v id="$ctr_id" '$1==id {print $3}')" = RUNNING
docker_id=$(sudo docker run -d --runtime "$runtime" --name "$docker_name" "$image" /bin/sleep 300)
for _ in $(seq 1 100); do
	docker_status=$(sudo docker inspect --format '{{.State.Status}}' "$docker_name")
	[[ $docker_status = running ]] && break
	sleep .1
done
test "$docker_status" = running

ctr_state=$(sudo ctr task exec --exec-id ctr-state "$ctr_id" /bin/sh -c \
	'cat /proc/sys/kernel/random/boot_id; ip -4 address show dev mkn0; wget -T 15 -qO- http://example.com >/dev/null; echo CTR_NETWORK_PASS')
docker_state=$(sudo docker exec "$docker_name" /bin/sh -c \
	'cat /proc/sys/kernel/random/boot_id; ip -4 address show dev mkn0; wget -T 15 -qO- http://example.com >/dev/null; echo DOCKER_NETWORK_PASS')
test -n "$ctr_state"
test -n "$docker_state"
printf '%s\n' "$ctr_state"
printf '%s\n' "$docker_state"

ctr_boot=$(printf '%s\n' "$ctr_state" | head -n1)
docker_boot=$(printf '%s\n' "$docker_state" | head -n1)
test "$ctr_boot" != "$docker_boot"
test "$ctr_boot" != "$host_boot"
test "$docker_boot" != "$host_boot"
ctr_ip=$(printf '%s\n' "$ctr_state" | awk '/inet / {sub("/.*", "", $2); print $2; exit}')
docker_ip=$(printf '%s\n' "$docker_state" | awk '/inet / {sub("/.*", "", $2); print $2; exit}')
test -n "$ctr_ip"
test -n "$docker_ip"
test "$ctr_ip" != "$docker_ip"

# Neither sandbox may route directly into its sibling's point-to-point link.
if sudo ctr task exec --exec-id ctr-isolation "$ctr_id" /bin/ping -c 1 -W 2 "$docker_ip" >/dev/null 2>&1; then
	echo 'cross-sandbox packet unexpectedly succeeded' >&2
	exit 1
fi
echo CROSS_SANDBOX_ISOLATION_PASS

# Exercise exec, signal, wait, and delete through each client.
sudo ctr task exec --exec-id ctr-exec "$ctr_id" /bin/echo CTR_EXEC_PASS
sudo docker exec "$docker_name" /bin/echo DOCKER_EXEC_PASS
sudo ctr tasks kill --signal SIGKILL "$ctr_id"
sudo docker kill --signal KILL "$docker_name" >/dev/null
for _ in $(seq 1 100); do
	ctr_status=$(sudo ctr tasks list | awk -v id="$ctr_id" '$1==id {print $3}')
	docker_status=$(sudo docker inspect --format '{{.State.Status}}' "$docker_name" 2>/dev/null || true)
	[[ $ctr_status != RUNNING && $docker_status = exited ]] && break
	sleep .1
done
test "$ctr_status" = STOPPED
test "$(sudo docker inspect --format '{{.State.ExitCode}}' "$docker_name")" = 137
sudo ctr tasks rm "$ctr_id" >/dev/null
sudo ctr containers rm "$ctr_id"
sudo docker rm "$docker_name" >/dev/null

# Normal teardown must release every per-container resource and network rule.
for _ in $(seq 1 100); do
	test -z "$(sudo find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 -print -quit)" && break
	sleep .1
done
test -z "$(sudo find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 -print -quit)"
test "$(cat /proc/sys/kernel/random/boot_id)" = "$host_boot"
test -z "$(ip -o link show | awk -F': ' '$2 ~ /^mkv[0-9a-f]+$/ {print $2}')"
! sudo iptables -t nat -S POSTROUTING | grep -q '172\.31\.'
! sudo iptables -S | grep -q '^\(-N\|-A\) MK-'
test -z "$(sudo ctr containers list -q)"
test -z "$(sudo ctr tasks list -q)"
test -z "$(sudo docker ps -aq --filter name="^/${docker_name}$")"

trap - EXIT
echo DISTINCT_CHILD_KERNEL_BOOT_IDS_PASS
echo CTR_DOCKER_LIFECYCLE_PASS
echo PRIMARY_MEDIATED_NETWORK_PASS
echo G4_G5_G6_MVP_PROOF_PASS
