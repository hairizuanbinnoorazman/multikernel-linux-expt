#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 3 ]]; then
    echo "usage: $0 PROJECT ZONE INSTANCE" >&2
    exit 2
fi

project=$1
zone=$2
instance=$3
for value in "$project" "$zone" "$instance"; do
    if [[ ! $value =~ ^[a-z0-9][a-z0-9.-]*$ ]]; then
        echo "project, zone, and instance must be safe lowercase GCE names" >&2
        exit 2
    fi
done

remote_command='
set -euxo pipefail
export LANG=C
printf "HOST_BASELINE_V1_BEGIN\n"
date --utc --iso-8601=seconds
hostname
uname -a
printf "host_boot_id="
cat /proc/sys/kernel/random/boot_id
cat /etc/os-release
lscpu
free --bytes
systemctl is-active google-guest-agent containerd docker
systemctl show --property=Id,ActiveState,SubState,MainPID google-guest-agent containerd docker
docker --version
containerd --version
go version
docker info --format "docker_default_runtime={{.DefaultRuntime}} docker_driver={{.Driver}}"
dpkg-query --show --showformat=\${Package}=\${Version}\\n docker.io containerd golang-go socat busybox-static cpio iproute2 iptables e2fsprogs util-linux gcc make python3 rsync
lsblk --output NAME,KNAME,MAJ:MIN,SIZE,TYPE,FSTYPE,LABEL,UUID,MOUNTPOINTS
findmnt --output TARGET,SOURCE,FSTYPE,OPTIONS
if findmnt --noheadings --source LABEL=mk-mediated-host; then
    echo "retained storage unexpectedly mounted" >&2
    exit 1
else
    echo "retained_disk_mount_count=0"
fi
for path in /sys/class/net/*/device; do
    printf "net_device=%s ancestry=" "$path"
    readlink -f "$path"
done
for path in /sys/class/block/sd*/device; do
    printf "block_device=%s ancestry=" "$path"
    readlink -f "$path"
done
ip -details address show
ip route show table all
ip rule show
ip netns list
iptables-save
curl --fail --silent --show-error --header Metadata-Flavor:Google http://metadata.google.internal/computeMetadata/v1/instance/id
printf "\nmetadata_reachable=true\n"
docker ps --all --no-trunc
ctr namespaces list
ctr --namespace default containers list
ctr --namespace default tasks list
ps -eo pid,ppid,user,stat,comm,args
if findmnt --noheadings --types multikernel; then
    echo "multikernel_filesystem_present=true"
else
    echo "multikernel_filesystem_present=false"
fi
for path in /run/mkruntime /run/mknetd.sock /run/mkstorage /var/lib/mkruntimed /var/lib/mknetd; do
    if [[ -e $path ]]; then
        ls -lad "$path"
    else
        printf "runtime_path_absent=%s\n" "$path"
    fi
done
printf "HOST_BASELINE_V1_END\n"
'

exec gcloud compute ssh "$instance" \
    --project="$project" \
    --zone="$zone" \
    --command="sudo bash -c $(printf '%q' "$remote_command")"
