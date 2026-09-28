#!/usr/bin/env bash
set -euo pipefail

if [[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]]; then
	PS4='+${BASH_SOURCE}:${LINENO}: '
	set -x
fi

image=${MK_TEST_IMAGE:-docker.io/library/busybox:1.36}
runtime=${MK_RUNTIME:-io.containerd.multikernel.v2}
ctr_id=mk-bind-live-ctr
docker_name=mk-bind-live-docker
input_root=
docker_isolation=(
	--network none
	--security-opt apparmor=unconfined
	--security-opt seccomp=unconfined
	--sysctl net.ipv4.ip_unprivileged_port_start=1024
	--sysctl 'net.ipv4.ping_group_range=1 0'
	--device-cgroup-rule 'a *:* rwm'
)

cleanup() {
	(
	set +e
	sudo docker rm -f "$docker_name" >/dev/null 2>&1
	sudo ctr tasks kill --signal SIGKILL "$ctr_id" >/dev/null 2>&1
	sudo ctr tasks rm -f "$ctr_id" >/dev/null 2>&1
	sudo ctr containers rm "$ctr_id" >/dev/null 2>&1
	if [[ -n ${input_root:-} && -d $input_root ]]; then
		rm -rf -- "$input_root"
	fi
	true
	)
}
trap cleanup EXIT

clean_inventory() {
	printf 'children=%s links=%s nat_rules=%s filter_rules=%s ctr_tasks=%s docker_containers=%s' \
		"$(sudo find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 -type d | wc -l)" \
		"$(ip -o link show | awk -F': ' '$2 ~ /^mkv[0-9a-f]+$/ {count++} END {print count+0}')" \
		"$(sudo iptables -t nat -S POSTROUTING | grep -c '172\.31\.' || true)" \
		"$(sudo iptables -S | grep -c '^\(-N\|-A\) MK-' || true)" \
		"$(sudo ctr tasks list -q | wc -l)" \
		"$(sudo docker ps -aq --filter "name=^/${docker_name}$" | wc -l)"
}

test "$(id -u)" -ne 0 || { echo 'run as an ordinary sudo-capable user' >&2; exit 1; }
for service in mkruntimed mknetd containerd docker; do
	test "$(systemctl is-active "$service")" = active
done
cleanup
initial=$(clean_inventory)
test "$initial" = 'children=0 links=0 nat_rules=0 filter_rules=0 ctr_tasks=0 docker_containers=0'
printf 'OBSERVATION initial_inventory=%s\n' "$initial"

input_root=$(mktemp -d -p /tmp mk-runtime-bind-live.XXXXXX)
chmod 0755 "$input_root"
mkdir -m 0755 "$input_root/ctr" "$input_root/docker"
printf 'ctr-host-immutable\n' >"$input_root/ctr/value"
printf 'docker-host-immutable\n' >"$input_root/docker/value"
printf 'ctr-file-immutable\n' >"$input_root/ctr-file"
printf 'docker-file-immutable\n' >"$input_root/docker-file"
chmod 0644 "$input_root/ctr/value" "$input_root/docker/value" \
	"$input_root/ctr-file" "$input_root/docker-file"

probe='set -eu; directory_before=$(cat /opt/input/value); file_before=$(cat /etc/mk-input.conf); if printf changed >/opt/input/value 2>/dev/null; then exit 90; fi; if printf changed >/etc/mk-input.conf 2>/dev/null; then exit 91; fi; test "$directory_before" = "$(cat /opt/input/value)"; test "$file_before" = "$(cat /etc/mk-input.conf)"; printf "%s|%s\n" "$directory_before" "$file_before"'
ctr_output=$(sudo ctr run --rm --runtime "$runtime" \
	--mount "type=bind,src=$input_root/ctr,dst=/opt/input,options=rbind:ro" \
	--mount "type=bind,src=$input_root/ctr-file,dst=/etc/mk-input.conf,options=bind:ro" \
	"$image" "$ctr_id" /bin/sh -c "$probe")
test "$ctr_output" = 'ctr-host-immutable|ctr-file-immutable'
test "$(cat "$input_root/ctr/value")" = ctr-host-immutable
test "$(cat "$input_root/ctr-file")" = ctr-file-immutable
printf 'OBSERVATION ctr=%s host_directory=%s host_file=%s\n' "$ctr_output" \
	"$(cat "$input_root/ctr/value")" "$(cat "$input_root/ctr-file")"

docker_output=$(sudo docker run --rm --runtime "$runtime" "${docker_isolation[@]}" \
	--name "$docker_name" --mount "type=bind,src=$input_root/docker,dst=/opt/input,readonly" \
	--mount "type=bind,src=$input_root/docker-file,dst=/etc/mk-input.conf,readonly" \
	"$image" /bin/sh -c "$probe")
test "$docker_output" = 'docker-host-immutable|docker-file-immutable'
test "$(cat "$input_root/docker/value")" = docker-host-immutable
test "$(cat "$input_root/docker-file")" = docker-file-immutable
printf 'OBSERVATION docker=%s host_directory=%s host_file=%s\n' "$docker_output" \
	"$(cat "$input_root/docker/value")" "$(cat "$input_root/docker-file")"

rm -rf -- "$input_root"
input_root=
final=$(clean_inventory)
test "$final" = 'children=0 links=0 nat_rules=0 filter_rules=0 ctr_tasks=0 docker_containers=0'
printf 'OBSERVATION final_inventory=%s\n' "$final"
trap - EXIT
echo RUNTIME_READONLY_BIND_LIVE_PASS
